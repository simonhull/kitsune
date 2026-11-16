package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"fmt"
	"slices"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// Duplicates are copies of one recording. The store reads what finding them
// takes, from the rows kept for the index where it can, keeps the groups an
// administrator said are not duplicates, and merges: everything people did
// with the copies being trashed moves onto the one kept, in the transaction
// that marks those copies trashed.

// DuplicateSong is a live track as finding duplicates reads it.
type DuplicateSong struct {
	ID       int64
	ULID     string
	Title    string
	Folded   string // The title folded as a search folds it.
	Artist   string // The first artist, as shown; "" for none.
	AlbumID  int64
	Disc     int
	Duration time.Duration
	Format   string
	Digest   []byte // nil before the fingerprint pass reads the file.
}

// DuplicateAlbum is an album as finding duplicates reads it.
type DuplicateAlbum struct {
	ID          int64
	ULID, Title string
	Artist      string // Who it files under, as the index heads it.
	Year        int
	Cover       *Cover
	Compilation bool
	// Owner is the match key of the album's owner, as AGENTS.md defines it,
	// or "" when it has none or it is Various Artists.
	Owner string
}

// DuplicateLibrary is the library as finding duplicates reads it.
type DuplicateLibrary struct {
	Songs  []DuplicateSong // Every live track, in the Library's order.
	Albums map[int64]*DuplicateAlbum
	// Tags are each song's values of the keys asked for, by row.
	Tags map[int64][]Tag
}

// DuplicatesMark is where the writes finding duplicates reads stood at one
// moment: a library write, or a fingerprint stored, moves it.
type DuplicatesMark struct {
	Writes, Digests uint64
	Settled         bool // No library write was under way.
}

// DuplicatesMark returns where the writes finding duplicates reads stand now.
// What was read between two equal settled marks is as the library is.
func (s *Store) DuplicatesMark() DuplicatesMark {
	l := &s.library
	return DuplicatesMark{Writes: l.gen.Load(), Digests: l.digests.Load(), Settled: l.writing.Load() == 0}
}

// DuplicateLibrary reads every live song as finding duplicates needs it, with
// its values of tagKeys, from the rows kept for the index where it can: no
// audio file is read.
func (s *Store) DuplicateLibrary(ctx context.Context, tagKeys []string) (*DuplicateLibrary, error) {
	rows, err := s.duplicateRows(ctx, tagKeys)
	if err != nil {
		return nil, fmt.Errorf("duplicate library: %w", err)
	}
	b := rows.basis
	out := &DuplicateLibrary{Songs: duplicateSongs(rows), Albums: duplicateAlbums(b.albums), Tags: map[int64][]Tag{}}
	for i := range rows.tags {
		r := &rows.tags[i]
		out.Tags[r.TrackID] = append(out.Tags[r.TrackID], Tag{Key: r.Key, Value: r.Value})
	}
	return out, nil
}

// duplicateRows is what finding duplicates reads, in one snapshot.
type duplicateRows struct {
	digests []sqlcdb.ListTrackDigestsRow
	basis   *smartBasis
	details map[int64]*sqlcdb.ListIndexTrackDetailsRow
	tags    []sqlcdb.ListIndexTrackTagsRow
}

// duplicateRows reads what finding duplicates needs, from the kept rows where
// it can.
func (s *Store) duplicateRows(ctx context.Context, tagKeys []string) (*duplicateRows, error) {
	mark := s.library.before()
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // Read-only: nothing to undo.
	q := s.rq.WithTx(tx)
	l := &s.library
	var rows duplicateRows
	// The first read takes the snapshot, which the kept rows must come after.
	// Fingerprints are read every time: the pass stores them outside library
	// writes, and one landing is what makes a find worth working out again.
	if rows.digests, err = q.ListTrackDigests(ctx); err != nil {
		return nil, fmt.Errorf("digests: %w", err)
	}
	if rows.basis, err = s.smartBasis(ctx, q, mark); err != nil {
		return nil, err
	}
	if rows.details, err = s.trackDetails(ctx, q, mark, rows.basis); err != nil {
		return nil, err
	}
	rows.tags, err = read(l, &l.duplicateTags, mark, tagArgs(tagKeys), func() ([]sqlcdb.ListIndexTrackTagsRow, error) {
		return q.ListIndexTrackTags(ctx, tagKeys)
	})
	if err != nil {
		return nil, fmt.Errorf("tags: %w", err)
	}
	return &rows, nil
}

// duplicateAlbums returns every album as finding duplicates reads it, by row.
func duplicateAlbums(albums []sqlcdb.ListIndexAlbumsRow) map[int64]*DuplicateAlbum {
	out := make(map[int64]*DuplicateAlbum, len(albums))
	for i := range albums {
		a := &albums[i]
		owner := ""
		if a.OwnerMatchKey != nil && !isVarious(*a.OwnerMatchKey, a.OwnerMbid) {
			owner = *a.OwnerMatchKey
		}
		out[a.ID] = &DuplicateAlbum{
			ID: a.ID, ULID: a.Ulid, Title: a.Name, Artist: indexHeading(a).Name, Year: intOr0(a.Year),
			Cover:       cover(a.ArtworkHash, a.ArtworkWidth, a.ArtworkHeight, a.ArtworkAccent, a.ArtworkGround),
			Compilation: a.Compilation == 1, Owner: owner,
		}
	}
	return out
}

// duplicateSongs returns every live track as finding duplicates reads it, in
// the Library's order.
func duplicateSongs(rows *duplicateRows) []DuplicateSong {
	b := rows.basis
	digestOf := make(map[int64][]byte, len(rows.digests))
	for i := range rows.digests {
		digestOf[rows.digests[i].ID] = rows.digests[i].AudioDigest
	}
	names, folded := b.names(), b.titles()
	out := make([]DuplicateSong, 0, len(b.tracks))
	for i := range b.tracks {
		t := &b.tracks[i]
		song := DuplicateSong{
			ID: t.ID, ULID: t.Ulid, Title: t.Title, Folded: folded[i], AlbumID: t.AlbumID, Disc: intOr0(t.DiscNo),
			Duration: time.Duration(t.DurationMs) * time.Millisecond, Digest: digestOf[t.ID],
		}
		if n := names[t.ID]; len(n) > 0 {
			song.Artist = n[0]
		}
		if d := rows.details[t.ID]; d != nil {
			song.Format = d.Format
		}
		out = append(out, song)
	}
	slices.SortStableFunc(out, func(x, y DuplicateSong) int { return b.rank[x.ID] - b.rank[y.ID] })
	return out
}

// DuplicateCopy is one copy of a recording as a group's page compares it.
type DuplicateCopy struct {
	ID                   int64
	ULID, Title          string
	Root, RelPath        string
	Format               string
	Bitrate              int // In kb/s; 0 when unknown.
	SampleRate, BitDepth int // 0 when unknown.
	Lossless             bool
	Duration             time.Duration
	AddedAt              time.Time
	Disc, Track          int
	Album, AlbumTitle    string // The album's ULID and title.
	AlbumYear            int
	Cover                *Cover
	Plays                int           // Everyone's.
	Raters               int           // How many people rated it.
	Stars                rating.Rating // The person's own; None for none.
	Playlists            int           // How many playlists hold it.
}

// DuplicateCopies returns the live tracks ids as a group's page compares them,
// with the stars of the user, by row. A track that isn't live is left out.
func (s *Store) DuplicateCopies(ctx context.Context, userID int64, ids []int64) (map[int64]DuplicateCopy, error) {
	rows, err := s.rq.DuplicateCopies(ctx, sqlcdb.DuplicateCopiesParams{UserID: userID, Ids: ids})
	if err != nil {
		return nil, fmt.Errorf("duplicate copies: %w", err)
	}
	out := make(map[int64]DuplicateCopy, len(rows))
	for i := range rows {
		r := &rows[i]
		out[r.ID] = DuplicateCopy{
			ID: r.ID, ULID: r.Ulid, Title: r.Title, Root: r.Root, RelPath: r.RelPath, Format: r.Format,
			Bitrate: intOr0(r.Bitrate), SampleRate: intOr0(r.SampleRate), BitDepth: intOr0(r.BitDepth),
			Lossless: r.Lossless == 1, Duration: time.Duration(r.DurationMs) * time.Millisecond, AddedAt: fromMS(r.AddedAt),
			Disc: intOr0(r.DiscNo), Track: intOr0(r.TrackNo), Album: r.AlbumUlid, AlbumTitle: r.Album,
			AlbumYear: intOr0(r.AlbumYear),
			Cover:     cover(r.ArtworkHash, r.ArtworkWidth, r.ArtworkHeight, r.ArtworkAccent, r.ArtworkGround),
			Plays:     int(r.Plays), Raters: int(r.Raters), Stars: ratingOf(r.Halves), Playlists: int(r.Playlists),
		}
	}
	return out, nil
}

// DuplicateDismissals returns the groups said not to be duplicates: each
// one's key, with the public IDs it was said of.
func (s *Store) DuplicateDismissals(ctx context.Context) (map[string][]string, error) {
	rows, err := s.rq.ListDuplicateDismissals(ctx)
	if err != nil {
		return nil, fmt.Errorf("duplicate dismissals: %w", err)
	}
	out := make(map[string][]string, len(rows))
	for _, r := range rows {
		var ids []string
		if bad := json.Unmarshal([]byte(r.Tracks), &ids); bad != nil {
			return nil, fmt.Errorf("duplicate dismissal %s: %w", r.GroupKey, bad)
		}
		out[r.GroupKey] = ids
	}
	return out, nil
}

// DismissDuplicates notes that the group key, of the songs (or albums) ids,
// is not duplicates, as the user by said.
func (s *Store) DismissDuplicates(ctx context.Context, key string, ids []string, by int64) error {
	list, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("dismiss duplicates: %w", err)
	}
	err = s.wq.DismissDuplicates(ctx, sqlcdb.DismissDuplicatesParams{
		GroupKey: key, Tracks: string(list), ByUser: nullID(by), DismissedAt: ms(s.now()),
	})
	if err != nil {
		return fmt.Errorf("dismiss duplicates: %w", err)
	}
	return nil
}

// BringBackDuplicates forgets that the group key was said not to be
// duplicates. It returns an error wrapping [ErrNotFound] when it wasn't.
func (s *Store) BringBackDuplicates(ctx context.Context, key string) error {
	n, err := s.wq.BringBackDuplicates(ctx, key)
	if err != nil {
		return fmt.Errorf("bring back duplicates: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("bring back duplicates: %w", ErrNotFound)
	}
	return nil
}

// playTotalsChunk is how many tracks one read of play totals names, well
// under SQLite's limit on a statement's arguments.
const playTotalsChunk = 4096

// PlayTotals returns everyone's plays of each of the tracks ids, by row. A
// track never played is left out.
func (s *Store) PlayTotals(ctx context.Context, ids []int64) (map[int64]int, error) {
	out := make(map[int64]int, len(ids))
	for chunk := range slices.Chunk(ids, playTotalsChunk) {
		rows, err := s.rq.MergePlays(ctx, chunk)
		if err != nil {
			return nil, fmt.Errorf("play totals: %w", err)
		}
		for _, r := range rows {
			out[r.TrackID] += int(r.N)
		}
	}
	return out, nil
}
