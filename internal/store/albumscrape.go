package store

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// The sources the album pass asks, and Last.fm, which the artist pass asks for
// bios.
const (
	ScrapeLastfm      ScrapeSource = "lastfm"
	ScrapeArchive     ScrapeSource = "coverartarchive" // The Cover Art Archive, by an album's release.
	ScrapeMusicBrainz ScrapeSource = "musicbrainz"     // An album's release group, from its release.
)

// Where the album pass's images and Last.fm's bios come from, as album_images
// and artist_bios name them.
const (
	ImageFromArchive = "coverartarchive"
	BioFromLastfm    = "lastfm"
)

// AlbumCover is an album's front from the Cover Art Archive: the album's cover
// while its files hold none. Its row in album_images says where it came from.
const AlbumCover AlbumImageKind = "cover"

// HeldAlbumImage is what an album holds of one extra.
type HeldAlbumImage struct {
	Source   string // "folder", or a source's name.
	RemoteID string // The source's own ID; "" for a folder's.
}

// ScrapeAlbum is an album the pass may ask about.
type ScrapeAlbum struct {
	ID         int64
	ULID, Name string
	Artist     string // Its display artist, for the card.
	MBID       string // The release's; "" when unknown.
	GroupMBID  string // The release group's; "" when unknown.
	// OwnCover says the album has a cover of its own, from its files: one the
	// Cover Art Archive gave doesn't count.
	OwnCover bool
	Plays    int   // Everyone's plays of its live tracks.
	Discs    []int // Its discs, from 1, sorted.
	Held     map[AlbumImageKey]HeldAlbumImage
	// States are what each source last said: the Cover Art Archive's and
	// MusicBrainz's of MBID, fanart.tv's of GroupMBID.
	States map[ScrapeSource]ScrapeState
}

// StateKey is the MBID src keys its state about a by: fanart.tv asks by the
// release group, everyone else by the release.
func (a *ScrapeAlbum) StateKey(src ScrapeSource) string {
	if src == ScrapeFanart {
		return a.GroupMBID
	}
	return a.MBID
}

// ScrapeAlbums returns every album with a live track, with what it holds and
// what each source last said of it, read in one transaction.
func (s *Store) ScrapeAlbums(ctx context.Context) ([]ScrapeAlbum, error) {
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("scrape albums: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Nothing was written: there is nothing to undo.
	q := s.rq.WithTx(tx)
	rows, err := q.ListScrapeAlbums(ctx)
	if err != nil {
		return nil, fmt.Errorf("scrape albums: %w", err)
	}
	extra, err := readAlbumExtras(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("scrape albums: %w", err)
	}
	out := make([]ScrapeAlbum, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		a := ScrapeAlbum{
			ID: r.ID, ULID: r.Ulid, Name: r.Name, Artist: r.Artist, MBID: strOrEmpty(r.Mbid),
			GroupMBID: strOrEmpty(r.ReleaseGroupMbid), Plays: extra.plays[r.ID], Discs: extra.discs[r.ID],
			OwnCover: r.ArtworkHash != nil && !bytes.Equal(r.ArtworkHash, r.ArchiveHash),
			Held:     extra.held[r.ID],
		}
		for _, src := range []ScrapeSource{ScrapeArchive, ScrapeMusicBrainz, ScrapeFanart} {
			if st, ok := extra.states[src][a.StateKey(src)]; ok && a.StateKey(src) != "" {
				if a.States == nil {
					a.States = map[ScrapeSource]ScrapeState{}
				}
				a.States[src] = st
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// albumExtraRows is what ScrapeAlbums reads beside the albums.
type albumExtraRows struct {
	plays  map[int64]int
	discs  map[int64][]int
	held   map[int64]map[AlbumImageKey]HeldAlbumImage
	states map[ScrapeSource]map[string]ScrapeState // By source, then MBID.
}

func readAlbumExtras(ctx context.Context, q *sqlcdb.Queries) (*albumExtraRows, error) {
	plays, err := q.ListAlbumPlays(ctx)
	if err != nil {
		return nil, fmt.Errorf("plays: %w", err)
	}
	discs, err := q.ListAlbumDiscs(ctx)
	if err != nil {
		return nil, fmt.Errorf("discs: %w", err)
	}
	held, err := q.ListHeldAlbumImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("extras: %w", err)
	}
	states, err := q.ListAlbumScrapeStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("states: %w", err)
	}
	r := &albumExtraRows{plays: make(map[int64]int, len(plays)), discs: discsByAlbum(discs), held: heldByAlbum(held), states: albumStates(states)}
	for _, p := range plays {
		r.plays[p.AlbumID] = int(p.Plays)
	}
	return r, nil
}

// discsByAlbum is every album's discs, sorted.
func discsByAlbum(rows []sqlcdb.ListAlbumDiscsRow) map[int64][]int {
	out := map[int64][]int{}
	for _, d := range rows {
		out[d.AlbumID] = append(out[d.AlbumID], int(d.Disc))
	}
	for id := range out {
		slices.Sort(out[id])
	}
	return out
}

// heldByAlbum is what every album holds of its extras.
func heldByAlbum(rows []sqlcdb.ListHeldAlbumImagesRow) map[int64]map[AlbumImageKey]HeldAlbumImage {
	out := map[int64]map[AlbumImageKey]HeldAlbumImage{}
	for i := range rows {
		h := &rows[i]
		if out[h.AlbumID] == nil {
			out[h.AlbumID] = map[AlbumImageKey]HeldAlbumImage{}
		}
		key := AlbumImageKey{AlbumID: h.AlbumID, Kind: AlbumImageKind(h.Kind), Disc: int(h.Disc)}
		out[h.AlbumID][key] = HeldAlbumImage{Source: h.Source, RemoteID: strOrEmpty(h.RemoteID)}
	}
	return out
}

// albumStates is every album state, by source and MBID.
func albumStates(rows []sqlcdb.ListAlbumScrapeStatesRow) map[ScrapeSource]map[string]ScrapeState {
	out := map[ScrapeSource]map[string]ScrapeState{}
	for i := range rows {
		st := &rows[i]
		src := ScrapeSource(st.Source)
		if out[src] == nil {
			out[src] = map[string]ScrapeState{}
		}
		out[src][st.Mbid] = scrapeState(st.Status, st.FetchedAt, st.NextAt, st.Failures, st.Error)
	}
	return out
}

// RemoteAlbumImage sets an album's extra of one kind and disc from a source, or
// with a nil Image takes away the one Source gave. Neither touches a folder's.
// Kind [AlbumCover] is the album's cover from the Cover Art Archive, which is
// set only while the album has none of its own.
type RemoteAlbumImage struct {
	Kind     AlbumImageKind
	Disc     int // [AllDiscs], or a disc number; always AllDiscs for a cover or a back.
	Source   string
	RemoteID string
	Credit   *Credit
	Image    *ArtworkWrite // Its Source is ArtworkRemote.
}

// AlbumAnswer is what one source said about an album, as it is stored, keyed
// by the MBID it was asked about.
type AlbumAnswer struct {
	Source ScrapeSource
	MBID   string
	State  ScrapeState
}

// AlbumRemoteWrite is what the pass found for one album.
type AlbumRemoteWrite struct {
	AlbumID int64
	// GroupMBID is the release group a lookup found, written only while the
	// album has none from its files; "" writes nothing.
	GroupMBID string
	Images    []RemoteAlbumImage
	Answers   []AlbumAnswer
}

// albumWritten is what a write changed: anything shown, and the cover.
type albumWritten struct {
	shown, cover bool
}

// WriteAlbumRemote stores w in one transaction and reports whether the album's
// cover or extras changed: what a library event says. A cover is the index's,
// so a write that changes one is a library write.
func (s *Store) WriteAlbumRemote(ctx context.Context, w *AlbumRemoteWrite, now time.Time) (bool, error) {
	var did albumWritten
	err := s.inLibraryTxIf(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) (bool, error) {
		var err error
		did, err = writeAlbumRemote(ctx, q, w, now) // The writer may retry: the last try says.
		return did.cover, err
	})
	if err != nil {
		return false, fmt.Errorf("write remote art of album %d: %w", w.AlbumID, err)
	}
	return did.shown, nil
}

func writeAlbumRemote(ctx context.Context, q *sqlcdb.Queries, w *AlbumRemoteWrite, now time.Time) (albumWritten, error) {
	if w.GroupMBID != "" {
		if _, err := q.SetLookedUpReleaseGroup(ctx, sqlcdb.SetLookedUpReleaseGroupParams{Mbid: &w.GroupMBID, ID: w.AlbumID}); err != nil {
			return albumWritten{}, fmt.Errorf("release group: %w", err)
		}
	}
	did, err := writeAlbumImages(ctx, q, w, now)
	if err != nil {
		return albumWritten{}, err
	}
	for _, a := range w.Answers {
		err = q.UpsertAlbumScrapeState(ctx, sqlcdb.UpsertAlbumScrapeStateParams{
			Source: string(a.Source), Mbid: a.MBID, Status: string(a.State.Status), FetchedAt: ms(a.State.FetchedAt),
			NextAt: ms(a.State.NextAt), Failures: int64(a.State.Failures), Error: orNil(a.State.Error),
		})
		if err != nil {
			return albumWritten{}, fmt.Errorf("%s state: %w", a.Source, err)
		}
	}
	return did, nil
}

func writeAlbumImages(ctx context.Context, q *sqlcdb.Queries, w *AlbumRemoteWrite, now time.Time) (albumWritten, error) {
	var did albumWritten
	for i := range w.Images {
		img := &w.Images[i]
		if img.Kind == AlbumCover {
			c, err := writeArchiveCover(ctx, q, w.AlbumID, img, now)
			if err != nil {
				return albumWritten{}, err
			}
			did.shown, did.cover = did.shown || c.shown, did.cover || c.cover
			continue
		}
		n, err := writeRemoteAlbumImage(ctx, q, w.AlbumID, img, now)
		if err != nil {
			return albumWritten{}, err
		}
		did.shown = did.shown || n > 0
	}
	return did, nil
}

// remoteAlbumSource reports whether source names a source's album image: never
// a folder's.
func remoteAlbumSource(source string) bool {
	return source == ImageFromArchive || source == ImageFromFanart || source == fromAudioDB
}

func writeRemoteAlbumImage(ctx context.Context, q *sqlcdb.Queries, albumID int64, img *RemoteAlbumImage, now time.Time) (int64, error) {
	if !remoteAlbumSource(img.Source) || (img.Kind != AlbumDisc && img.Disc != AllDiscs) {
		return 0, fmt.Errorf("album %s %d from %q: %w", img.Kind, img.Disc, img.Source, ErrInvalidWrite)
	}
	disc := int64(img.Disc)
	if img.Image == nil {
		n, err := q.DeleteRemoteAlbumImage(ctx, sqlcdb.DeleteRemoteAlbumImageParams{AlbumID: albumID, Kind: string(img.Kind), Disc: disc, Source: img.Source})
		if err != nil {
			return 0, fmt.Errorf("album %s %d: %w", img.Kind, img.Disc, err)
		}
		return n, nil
	}
	if err := upsertImage(ctx, q, img.Image); err != nil {
		return 0, fmt.Errorf("album %s %d: %w", img.Kind, img.Disc, err)
	}
	n, err := q.UpsertRemoteAlbumImage(ctx, sqlcdb.UpsertRemoteAlbumImageParams{
		AlbumID: albumID, Kind: string(img.Kind), Disc: disc, ArtworkHash: img.Image.Hash[:], Source: img.Source,
		RemoteID: orNil(img.RemoteID), Credit: creditJSON(img.Credit), FoundAt: ms(now),
	})
	if err != nil {
		return 0, fmt.Errorf("album %s %d: %w", img.Kind, img.Disc, err)
	}
	return n, nil
}

// writeArchiveCover sets or takes away the album's cover from the Cover Art
// Archive, never over a cover of the album's own.
func writeArchiveCover(ctx context.Context, q *sqlcdb.Queries, albumID int64, img *RemoteAlbumImage, now time.Time) (albumWritten, error) {
	if img.Source != ImageFromArchive || img.Disc != AllDiscs {
		return albumWritten{}, fmt.Errorf("album cover from %q: %w", img.Source, ErrInvalidWrite)
	}
	cur, err := q.GetAlbumCover(ctx, albumID)
	if err != nil {
		return albumWritten{}, fmt.Errorf("album cover: %w", err)
	}
	ours := cur.ArchiveHash != nil && bytes.Equal(cur.ArtworkHash, cur.ArchiveHash)
	switch {
	case cur.ArtworkHash != nil && !ours:
		return albumWritten{}, nil // The files gave it one: they win.
	case img.Image == nil && cur.ArchiveHash == nil:
		return albumWritten{}, nil
	case img.Image == nil:
		// Taking the cover away takes the row with it, by albums_cover_from_files.
		return setArchiveCover(ctx, q, albumID, nil, cur.ArtworkHash)
	}
	return putArchiveCover(ctx, q, albumID, img, cur.ArtworkHash, now)
}

// putArchiveCover stores img as the album's archive cover and makes it the
// album's cover while that is still cur.
func putArchiveCover(ctx context.Context, q *sqlcdb.Queries, albumID int64, img *RemoteAlbumImage, cur []byte, now time.Time) (albumWritten, error) {
	if err := upsertImage(ctx, q, img.Image); err != nil {
		return albumWritten{}, fmt.Errorf("album cover: %w", err)
	}
	n, err := q.UpsertRemoteAlbumImage(ctx, sqlcdb.UpsertRemoteAlbumImageParams{
		AlbumID: albumID, Kind: string(AlbumCover), Disc: AllDiscs, ArtworkHash: img.Image.Hash[:], Source: ImageFromArchive,
		RemoteID: orNil(img.RemoteID), Credit: creditJSON(img.Credit), FoundAt: ms(now),
	})
	if err != nil {
		return albumWritten{}, fmt.Errorf("album cover: %w", err)
	}
	if bytes.Equal(cur, img.Image.Hash[:]) {
		return albumWritten{shown: n > 0}, nil
	}
	return setArchiveCover(ctx, q, albumID, img.Image.Hash[:], cur)
}

// setArchiveCover makes hash (nil for none) the album's cover while it is was.
func setArchiveCover(ctx context.Context, q *sqlcdb.Queries, albumID int64, hash, was []byte) (albumWritten, error) {
	n, err := q.SetArchiveCover(ctx, sqlcdb.SetArchiveCoverParams{Hash: hash, ID: albumID, Was: was})
	if err != nil {
		return albumWritten{}, fmt.Errorf("album cover: %w", err)
	}
	return albumWritten{shown: n > 0, cover: n > 0}, nil
}

// AlbumCounts counts the library's albums for Health's card.
type AlbumCounts struct {
	Albums       int // With a live track.
	WithMBID     int // With a release MBID, which the Cover Art Archive is asked by.
	FromArchive  int // Whose cover is the Cover Art Archive's.
	WithoutCover int // With no cover at all.
}

// CountAlbums counts the library's albums by their covers.
func (s *Store) CountAlbums(ctx context.Context) (AlbumCounts, error) {
	r, err := s.rq.CountScrapeAlbums(ctx)
	if err != nil {
		return AlbumCounts{}, fmt.Errorf("count albums: %w", err)
	}
	return AlbumCounts{Albums: int(r.Albums), WithMBID: int(r.WithMbid), FromArchive: int(r.FromArchive), WithoutCover: int(r.WithoutCover)}, nil
}
