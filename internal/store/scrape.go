package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// ArtworkRemote is an image a source on the internet gave.
const ArtworkRemote ArtworkSource = "remote"

// ScrapeSource is a source the artist pass asks, as scrape_state names it.
type ScrapeSource string

// The sources the artist pass asks.
const (
	ScrapeFanart    ScrapeSource = "fanarttv"
	ScrapeWikimedia ScrapeSource = "wikimedia"
)

// ScrapeStatus is what a source's last answer about an artist was.
type ScrapeStatus string

// What an answer may be.
const (
	ScrapeFound  ScrapeStatus = "found"  // Something to show.
	ScrapeNone   ScrapeStatus = "none"   // A 404, or nothing usable.
	ScrapeFailed ScrapeStatus = "failed" // A server error, a timeout, a body that doesn't parse.
)

// ScrapeState is what a source last said about an artist, and when the pass
// asks it again.
type ScrapeState struct {
	Status            ScrapeStatus
	FetchedAt, NextAt time.Time
	Failures          int    // Failures in a row; 0 unless Status is ScrapeFailed.
	Error             string // The last failure in words, never with a URL or a key.
}

// ScrapeArtist is an artist the pass may ask about.
type ScrapeArtist struct {
	ID         int64
	ULID, Name string
	MBID       string // "" when unknown.
	Owner      bool   // Owns an album.
	Plays      int    // Everyone's plays of the albums it owns.
	Arted      bool   // Holds an image or a bio, from anywhere.
	States     map[ScrapeSource]ScrapeState
}

// ScrapeArtists returns every artist with an MBID and a live track, Various
// Artists left out, with what each source last said of it, read in one
// transaction.
func (s *Store) ScrapeArtists(ctx context.Context) ([]ScrapeArtist, error) {
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("scrape artists: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Nothing was written: there is nothing to undo.
	r, err := readScrapeRows(ctx, s.rq.WithTx(tx))
	if err != nil {
		return nil, fmt.Errorf("scrape artists: %w", err)
	}
	rows := r.artists
	out := make([]ScrapeArtist, 0, len(rows))
	for i := range rows {
		a := &rows[i]
		if isVarious(a.MatchKey, a.Mbid) {
			continue
		}
		mbid := strOrEmpty(a.Mbid)
		p, owner := r.plays[a.ID]
		out = append(out, ScrapeArtist{
			ID: a.ID, ULID: a.Ulid, Name: a.Name, MBID: mbid, Owner: owner, Plays: p,
			Arted: r.arted[a.ID], States: r.states[mbid],
		})
	}
	return out, nil
}

// ScrapeArtistByID returns the artist with the public ID id and what each
// source last said of it; its MBID may be "". It returns an error wrapping
// [ErrNotFound] for an unknown ID or Various Artists. Owner and Plays are not
// read.
func (s *Store) ScrapeArtistByID(ctx context.Context, id string) (ScrapeArtist, error) {
	r, err := s.rq.GetScrapeArtist(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && isVarious(r.MatchKey, r.Mbid)) {
		return ScrapeArtist{}, fmt.Errorf("scrape artist %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return ScrapeArtist{}, fmt.Errorf("scrape artist %s: %w", id, err)
	}
	a := ScrapeArtist{ID: r.ID, ULID: r.Ulid, Name: r.Name, MBID: strOrEmpty(r.Mbid)}
	if a.MBID == "" {
		return a, nil
	}
	states, err := s.rq.ListScrapeStatesFor(ctx, a.MBID)
	if err != nil {
		return ScrapeArtist{}, fmt.Errorf("scrape artist %s: states: %w", id, err)
	}
	for i := range states {
		st := &states[i]
		if a.States == nil {
			a.States = map[ScrapeSource]ScrapeState{}
		}
		a.States[ScrapeSource(st.Source)] = scrapeState(st.Status, st.FetchedAt, st.NextAt, st.Failures, st.Error)
	}
	return a, nil
}

// scrapeRows is what ScrapeArtists reads.
type scrapeRows struct {
	artists []sqlcdb.ListScrapeArtistsRow
	plays   map[int64]int // Owners only.
	arted   map[int64]bool
	states  map[string]map[ScrapeSource]ScrapeState
}

func readScrapeRows(ctx context.Context, q *sqlcdb.Queries) (*scrapeRows, error) {
	artists, err := q.ListScrapeArtists(ctx)
	if err != nil {
		return nil, fmt.Errorf("artists: %w", err)
	}
	owners, err := q.ListOwnersByPlays(ctx)
	if err != nil {
		return nil, fmt.Errorf("owners: %w", err)
	}
	states, err := q.ListScrapeStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("states: %w", err)
	}
	arted, err := q.ListArtedArtists(ctx)
	if err != nil {
		return nil, fmt.Errorf("art: %w", err)
	}
	r := &scrapeRows{artists: artists, plays: make(map[int64]int, len(owners)), arted: make(map[int64]bool, len(arted)), states: statesByMBID(states)}
	for _, id := range arted {
		r.arted[id] = true
	}
	for _, o := range owners {
		r.plays[o.ArtistID] = int(o.Plays)
	}
	return r, nil
}

// statesByMBID is every artist's states, by MBID and source.
func statesByMBID(rows []sqlcdb.ListScrapeStatesRow) map[string]map[ScrapeSource]ScrapeState {
	out := map[string]map[ScrapeSource]ScrapeState{}
	for i := range rows {
		st := &rows[i]
		if out[st.Mbid] == nil {
			out[st.Mbid] = map[ScrapeSource]ScrapeState{}
		}
		out[st.Mbid][ScrapeSource(st.Source)] = scrapeState(st.Status, st.FetchedAt, st.NextAt, st.Failures, st.Error)
	}
	return out
}

func scrapeState(status string, fetched, next, failures int64, msg *string) ScrapeState {
	return ScrapeState{
		Status: ScrapeStatus(status), FetchedAt: fromMS(fetched), NextAt: fromMS(next),
		Failures: int(failures), Error: strOrEmpty(msg),
	}
}

// HeldImage is what an artist holds of one kind of image.
type HeldImage struct {
	Source   string // "folder", "pinned", or a source's name.
	RemoteID string // The source's own ID for the image; "" for a folder's.
	Hash     [32]byte
}

// HeldBio is where an artist's bio came from.
type HeldBio struct {
	Source, URL, Lang string
}

// HeldArt is what an artist holds of images and a bio, by where each came from.
type HeldArt struct {
	Images map[ArtistImageKind]HeldImage
	Bio    *HeldBio // Nil when it has none.
}

// ArtistHeld returns what the artist with row ID id holds of images and a bio.
func (s *Store) ArtistHeld(ctx context.Context, id int64) (HeldArt, error) {
	rows, err := s.rq.ListHeldImages(ctx, id)
	if err != nil {
		return HeldArt{}, fmt.Errorf("artist %d held: %w", id, err)
	}
	h := HeldArt{Images: make(map[ArtistImageKind]HeldImage, len(rows))}
	for i := range rows {
		r := &rows[i]
		h.Images[ArtistImageKind(r.Kind)] = HeldImage{Source: r.Source, RemoteID: strOrEmpty(r.RemoteID), Hash: [32]byte(r.ArtworkHash)}
	}
	b, err := s.rq.GetHeldBio(ctx, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return HeldArt{}, fmt.Errorf("artist %d held: bio: %w", id, err)
	default:
		h.Bio = &HeldBio{Source: b.Source, URL: strOrEmpty(b.Url), Lang: strOrEmpty(b.Lang)}
	}
	return h, nil
}

// RemoteImage sets an artist's image of one kind from a source, or with a nil
// Image takes away the kind's image when Source gave it. Neither touches a
// folder's image or a pin.
type RemoteImage struct {
	Kind     ArtistImageKind
	Source   string // As artist_images names it: "fanart" or "wikimedia".
	RemoteID string
	Credit   *Credit
	Image    *ArtworkWrite // Its Source is ArtworkRemote.
}

// RemoteBio sets an artist's bio from a source, or with a nil Bio takes away
// the bio Source gave. Neither touches an artist.nfo's.
type RemoteBio struct {
	Source string // As artist_bios names it: "wikipedia".
	Bio    *Bio
}

// ScrapeAnswer is what one source said about the artist, as it is stored.
type ScrapeAnswer struct {
	Source ScrapeSource
	State  ScrapeState
}

// RemoteWrite is what the pass found for one artist.
type RemoteWrite struct {
	ArtistID int64
	MBID     string
	Images   []RemoteImage
	Bios     []RemoteBio
	Answers  []ScrapeAnswer
	// AskAgain are sources to ask about the artist again now, since what this
	// write took away one of them may have.
	AskAgain []ScrapeSource
}

// WriteRemote stores w in one transaction and reports whether the artist's
// images or bio changed.
func (s *Store) WriteRemote(ctx context.Context, w *RemoteWrite, now time.Time) (bool, error) {
	var changed bool
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		var err error
		changed, err = writeRemote(ctx, q, w, now) // The writer may retry: the last try says.
		return err
	})
	if err != nil {
		return false, fmt.Errorf("write remote art of artist %d: %w", w.ArtistID, err)
	}
	return changed, nil
}

func writeRemote(ctx context.Context, q *sqlcdb.Queries, w *RemoteWrite, now time.Time) (bool, error) {
	images, err := writeEach(w.Images, func(img *RemoteImage) (bool, int64, error) {
		n, werr := writeRemoteImage(ctx, q, w.ArtistID, img, now)
		return true, n, werr
	})
	if err != nil {
		return false, err
	}
	bios, err := writeEach(w.Bios, func(b *RemoteBio) (bool, int64, error) {
		n, werr := writeRemoteBio(ctx, q, w.ArtistID, b, now)
		return true, n, werr
	})
	if err != nil {
		return false, err
	}
	for _, a := range w.Answers {
		if serr := upsertScrapeState(ctx, q, a.Source, w.MBID, &a.State); serr != nil {
			return false, serr
		}
	}
	if _, err = askAgain(ctx, q, w.AskAgain, []string{w.MBID}, now); err != nil {
		return false, err
	}
	return len(images)+len(bios) > 0, nil
}

// Where a source's image or bio may come from, as artist_images and
// artist_bios name them.
const (
	ImageFromFanart    = "fanart"
	ImageFromWikimedia = "wikimedia"
	BioFromWikipedia   = "wikipedia"
)

// remoteImage reports whether source names a source's image: never a folder's
// or a pin, which a source's write must never take away.
func remoteImage(source string) bool {
	return source == ImageFromFanart || source == ImageFromWikimedia || source == "audiodb"
}

// remoteBio reports whether source names a source's bio: never an artist.nfo's.
func remoteBio(source string) bool {
	return source == BioFromWikipedia || source == "lastfm" || source == "audiodb"
}

func writeRemoteImage(ctx context.Context, q *sqlcdb.Queries, artistID int64, img *RemoteImage, now time.Time) (int64, error) {
	if !remoteImage(img.Source) {
		return 0, fmt.Errorf("%s from %q: %w", img.Kind, img.Source, ErrInvalidWrite)
	}
	if img.Image == nil {
		n, err := q.DeleteRemoteArtistImage(ctx, sqlcdb.DeleteRemoteArtistImageParams{ArtistID: artistID, Kind: string(img.Kind), Source: img.Source})
		if err != nil {
			return 0, fmt.Errorf("%s: %w", img.Kind, err)
		}
		return n, nil
	}
	if err := upsertImage(ctx, q, img.Image); err != nil {
		return 0, fmt.Errorf("%s: %w", img.Kind, err)
	}
	n, err := q.UpsertRemoteArtistImage(ctx, sqlcdb.UpsertRemoteArtistImageParams{
		ArtistID: artistID, Kind: string(img.Kind), ArtworkHash: img.Image.Hash[:], Source: img.Source,
		RemoteID: orNil(img.RemoteID), Credit: creditJSON(img.Credit), FoundAt: ms(now),
	})
	if err != nil {
		return 0, fmt.Errorf("%s: %w", img.Kind, err)
	}
	return n, nil
}

func writeRemoteBio(ctx context.Context, q *sqlcdb.Queries, artistID int64, b *RemoteBio, now time.Time) (int64, error) {
	if !remoteBio(b.Source) {
		return 0, fmt.Errorf("bio from %q: %w", b.Source, ErrInvalidWrite)
	}
	if b.Bio == nil {
		n, err := q.DeleteRemoteBio(ctx, sqlcdb.DeleteRemoteBioParams{ArtistID: artistID, Source: b.Source})
		if err != nil {
			return 0, fmt.Errorf("bio: %w", err)
		}
		return n, nil
	}
	n, err := q.UpsertRemoteBio(ctx, sqlcdb.UpsertRemoteBioParams{
		ArtistID: artistID, Source: b.Source, Lang: orNil(b.Bio.Lang), Summary: b.Bio.Summary, Body: b.Bio.Body,
		Url: orNil(b.Bio.URL), License: orNil(b.Bio.License), LicenseUrl: orNil(b.Bio.LicenseURL), FetchedAt: ms(now),
	})
	if err != nil {
		return 0, fmt.Errorf("bio: %w", err)
	}
	return n, nil
}

func upsertScrapeState(ctx context.Context, q *sqlcdb.Queries, src ScrapeSource, mbid string, st *ScrapeState) error {
	err := q.UpsertScrapeState(ctx, sqlcdb.UpsertScrapeStateParams{
		Source: string(src), Mbid: mbid, Status: string(st.Status), FetchedAt: ms(st.FetchedAt), NextAt: ms(st.NextAt),
		Failures: int64(st.Failures), Error: orNil(st.Error),
	})
	if err != nil {
		return fmt.Errorf("%s state: %w", src, err)
	}
	return nil
}

// orNil is s, or NULL for "".
func orNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// creditJSON is c as stored, or NULL.
func creditJSON(c *Credit) *string {
	if c == nil || *c == (Credit{}) {
		return nil
	}
	b, err := json.Marshal(c, json.Deterministic(true))
	if err != nil { // Four strings always encode.
		panic("store: credit: " + err.Error())
	}
	return new(string(b))
}

// AskAgain brings the next ask of sources about the artists with mbids forward
// to now, and returns how many it brought forward.
func (s *Store) AskAgain(ctx context.Context, sources []ScrapeSource, mbids []string, now time.Time) (int64, error) {
	var n int64
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		var err error
		n, err = askAgain(ctx, q, sources, mbids, now)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("ask again: %w", err)
	}
	return n, nil
}

func askAgain(ctx context.Context, q *sqlcdb.Queries, sources []ScrapeSource, mbids []string, now time.Time) (int64, error) {
	if len(sources) == 0 || len(mbids) == 0 {
		return 0, nil
	}
	src, err := json.Marshal(sources)
	if err != nil { // Strings always encode.
		return 0, fmt.Errorf("sources: %w", err)
	}
	ids, err := json.Marshal(mbids)
	if err != nil {
		return 0, fmt.Errorf("mbids: %w", err)
	}
	n, err := q.AskAgain(ctx, sqlcdb.AskAgainParams{Now: ms(now), Sources: string(src), Mbids: string(ids)})
	if err != nil {
		return 0, fmt.Errorf("ask again: %w", err)
	}
	return n, nil
}

// ScrapeFeedRead returns when the source's feed of changes was last read, and
// false when it never was.
func (s *Store) ScrapeFeedRead(ctx context.Context, src ScrapeSource) (time.Time, bool, error) {
	at, err := s.rq.GetScrapeFeed(ctx, string(src))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return time.Time{}, false, nil
	case err != nil:
		return time.Time{}, false, fmt.Errorf("%s feed: %w", src, err)
	}
	return fromMS(at), true, nil
}

// SetScrapeFeedRead stores when the source's feed of changes was read.
func (s *Store) SetScrapeFeedRead(ctx context.Context, src ScrapeSource, at time.Time) error {
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		return q.SetScrapeFeed(ctx, sqlcdb.SetScrapeFeedParams{Source: string(src), ReadAt: ms(at)})
	})
	if err != nil {
		return fmt.Errorf("%s feed: %w", src, err)
	}
	return nil
}
