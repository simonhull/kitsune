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
	r := &albumExtraRows{
		plays: make(map[int64]int, len(plays)), discs: map[int64][]int{},
		held: map[int64]map[AlbumImageKey]HeldAlbumImage{}, states: map[ScrapeSource]map[string]ScrapeState{},
	}
	for _, p := range plays {
		r.plays[p.AlbumID] = int(p.Plays)
	}
	for _, d := range discs {
		r.discs[d.AlbumID] = append(r.discs[d.AlbumID], int(d.Disc))
	}
	for id := range r.discs {
		slices.Sort(r.discs[id])
	}
	for i := range held {
		h := &held[i]
		if r.held[h.AlbumID] == nil {
			r.held[h.AlbumID] = map[AlbumImageKey]HeldAlbumImage{}
		}
		key := AlbumImageKey{AlbumID: h.AlbumID, Kind: AlbumImageKind(h.Kind), Disc: int(h.Disc)}
		r.held[h.AlbumID][key] = HeldAlbumImage{Source: h.Source, RemoteID: strOrEmpty(h.RemoteID)}
	}
	for i := range states {
		st := &states[i]
		src := ScrapeSource(st.Source)
		if r.states[src] == nil {
			r.states[src] = map[string]ScrapeState{}
		}
		r.states[src][st.Mbid] = scrapeState(st.Status, st.FetchedAt, st.NextAt, st.Failures, st.Error)
	}
	return r, nil
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

// WriteAlbumRemote stores w in one transaction and reports whether the album's
// cover or extras changed: what a library event says. A cover is the index's,
// so a write that changes one is a library write.
func (s *Store) WriteAlbumRemote(ctx context.Context, w *AlbumRemoteWrite, now time.Time) (bool, error) {
	var changed bool
	err := s.inLibraryTxIf(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) (bool, error) {
		var cover bool
		var err error
		changed, cover, err = writeAlbumRemote(ctx, q, w, now) // The writer may retry: the last try says.
		return cover, err
	})
	if err != nil {
		return false, fmt.Errorf("write remote art of album %d: %w", w.AlbumID, err)
	}
	return changed, nil
}

// writeAlbumRemote reports whether anything shown changed, and whether the
// album's cover did.
func writeAlbumRemote(ctx context.Context, q *sqlcdb.Queries, w *AlbumRemoteWrite, now time.Time) (changed, cover bool, err error) {
	if w.GroupMBID != "" {
		if _, err = q.SetLookedUpReleaseGroup(ctx, sqlcdb.SetLookedUpReleaseGroupParams{Mbid: &w.GroupMBID, ID: w.AlbumID}); err != nil {
			return false, false, fmt.Errorf("release group: %w", err)
		}
	}
	for i := range w.Images {
		img := &w.Images[i]
		var n int64
		var c bool
		if img.Kind == AlbumCover {
			n, c, err = writeArchiveCover(ctx, q, w.AlbumID, img, now)
			cover = cover || c
		} else {
			n, err = writeRemoteAlbumImage(ctx, q, w.AlbumID, img, now)
		}
		if err != nil {
			return false, false, err
		}
		changed = changed || n > 0
	}
	for _, a := range w.Answers {
		err = q.UpsertAlbumScrapeState(ctx, sqlcdb.UpsertAlbumScrapeStateParams{
			Source: string(a.Source), Mbid: a.MBID, Status: string(a.State.Status), FetchedAt: ms(a.State.FetchedAt),
			NextAt: ms(a.State.NextAt), Failures: int64(a.State.Failures), Error: orNil(a.State.Error),
		})
		if err != nil {
			return false, false, fmt.Errorf("%s state: %w", a.Source, err)
		}
	}
	return changed || cover, cover, nil
}

// remoteAlbumSource reports whether source names a source's album image: never
// a folder's.
func remoteAlbumSource(source string) bool {
	return source == ImageFromArchive || source == ImageFromFanart || source == "audiodb"
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
// Archive, never over a cover of the album's own. It reports the rows it
// changed, and whether the album's cover did.
func writeArchiveCover(ctx context.Context, q *sqlcdb.Queries, albumID int64, img *RemoteAlbumImage, now time.Time) (int64, bool, error) {
	if img.Source != ImageFromArchive || img.Disc != AllDiscs {
		return 0, false, fmt.Errorf("album cover from %q: %w", img.Source, ErrInvalidWrite)
	}
	cur, err := q.GetAlbumCover(ctx, albumID)
	if err != nil {
		return 0, false, fmt.Errorf("album cover: %w", err)
	}
	ours := cur.ArchiveHash != nil && bytes.Equal(cur.ArtworkHash, cur.ArchiveHash)
	if cur.ArtworkHash != nil && !ours {
		return 0, false, nil // The files gave it one: they win.
	}
	if img.Image == nil {
		if cur.ArchiveHash == nil {
			return 0, false, nil
		}
		// Taking the cover away takes the row with it, by albums_cover_from_files.
		n, serr := q.SetArchiveCover(ctx, sqlcdb.SetArchiveCoverParams{Hash: nil, ID: albumID, Was: cur.ArtworkHash})
		if serr != nil {
			return 0, false, fmt.Errorf("album cover: %w", serr)
		}
		return n, n > 0, nil
	}
	if err = upsertImage(ctx, q, img.Image); err != nil {
		return 0, false, fmt.Errorf("album cover: %w", err)
	}
	n, err := q.UpsertRemoteAlbumImage(ctx, sqlcdb.UpsertRemoteAlbumImageParams{
		AlbumID: albumID, Kind: string(AlbumCover), Disc: AllDiscs, ArtworkHash: img.Image.Hash[:], Source: ImageFromArchive,
		RemoteID: orNil(img.RemoteID), Credit: creditJSON(img.Credit), FoundAt: ms(now),
	})
	if err != nil {
		return 0, false, fmt.Errorf("album cover: %w", err)
	}
	if bytes.Equal(cur.ArtworkHash, img.Image.Hash[:]) {
		return n, false, nil
	}
	set, err := q.SetArchiveCover(ctx, sqlcdb.SetArchiveCoverParams{Hash: img.Image.Hash[:], ID: albumID, Was: cur.ArtworkHash})
	if err != nil {
		return 0, false, fmt.Errorf("album cover: %w", err)
	}
	return n + set, set > 0, nil
}
