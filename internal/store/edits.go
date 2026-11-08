package store

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// AlbumRekey gives an album row a new group key, so that the tracks an edit
// renames land back on the row they left, keeping its ULID, first-seen time and
// cover.
type AlbumRekey struct {
	Album int64  // The album's row.
	To    string // Its new group key.
}

// RekeyAlbums gives each album its new group key, in order and in one
// transaction. A key another album holds, or one an earlier rekey took, is left
// alone: tracks written there join that album. It returns the rekeys that undo
// what it did, newest first.
func (s *Store) RekeyAlbums(ctx context.Context, rekeys []AlbumRekey) ([]AlbumRekey, error) {
	if len(rekeys) == 0 {
		return nil, nil
	}
	var undo []AlbumRekey
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		undo = undo[:0]
		for _, r := range rekeys {
			taken, err := q.GroupKeyTaken(ctx, r.To)
			if err != nil {
				return fmt.Errorf("album %d: %w", r.Album, err)
			}
			if taken {
				continue
			}
			from, err := q.AlbumGroupKey(ctx, r.Album)
			if err != nil {
				return fmt.Errorf("album %d: %w", r.Album, err)
			}
			if err = q.SetAlbumGroupKey(ctx, sqlcdb.SetAlbumGroupKeyParams{GroupKey: r.To, ID: r.Album}); err != nil {
				return fmt.Errorf("album %d: %w", r.Album, err)
			}
			undo = append([]AlbumRekey{{Album: r.Album, To: from}}, undo...)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("rekey albums: %w", err)
	}
	return undo, nil
}

// ArtistRename renames an artist row before an edit writes, so the tracks it
// renames the artist on land back on the row, keeping its ULID and hearts.
type ArtistRename struct {
	From, To       string // Match keys.
	Name, SortName string // The new names. An empty SortName is the name.
	MBID           string // Empty clears it: an MBID was the old name's.
	// Links is how many links the edit renames. The row is renamed only when that
	// is every link it has, so the artist no track still names keeps its name.
	// Zero takes any.
	Links int
}

// RenameArtists renames each artist the edit renames on every one of its links,
// to a key no artist holds, in order and in one transaction. It returns the
// renames that undo what it did, newest first.
func (s *Store) RenameArtists(ctx context.Context, renames []ArtistRename) ([]ArtistRename, error) {
	if len(renames) == 0 {
		return nil, nil
	}
	var undo []ArtistRename
	err := s.inLibraryTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		undo = undo[:0]
		for i := range renames {
			back, err := renameArtist(ctx, q, &renames[i])
			if errors.Is(err, errNoRename) {
				continue
			}
			if err != nil {
				return fmt.Errorf("artist %q: %w", renames[i].From, err)
			}
			undo = append([]ArtistRename{back}, undo...)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("rename artists: %w", err)
	}
	return undo, nil
}

// errNoRename means a rename does not apply: the artist is gone, keeps a link
// the edit leaves, or its new key is another artist's.
var errNoRename = errors.New("the rename does not apply")

// renameArtist does r and returns the rename that undoes it. It returns
// [errNoRename] when r does not apply.
func renameArtist(ctx context.Context, q *sqlcdb.Queries, r *ArtistRename) (ArtistRename, error) {
	row, err := q.ArtistToRename(ctx, r.From)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ArtistRename{}, errNoRename
	case err != nil:
		return ArtistRename{}, err //nolint:wrapcheck // The caller names the artist.
	case r.Links != 0 && row.Links != int64(r.Links):
		return ArtistRename{}, errNoRename
	}
	taken, err := q.MatchKeyTaken(ctx, r.To)
	if err != nil {
		return ArtistRename{}, err //nolint:wrapcheck // The caller names the artist.
	}
	if taken {
		return ArtistRename{}, errNoRename
	}
	err = q.RenameArtist(ctx, sqlcdb.RenameArtistParams{
		MatchKey: r.To, Name: r.Name, SortName: cmp.Or(r.SortName, r.Name), Mbid: nullStr(r.MBID), ID: row.ID,
	})
	if err != nil {
		return ArtistRename{}, err //nolint:wrapcheck // The caller names the artist.
	}
	back := ArtistRename{From: r.To, To: r.From, Name: row.Name, SortName: row.SortName}
	if row.Mbid != nil {
		back.MBID = *row.Mbid
	}
	return back, nil
}

// AlbumMove says which tracks an edit took off an album.
type AlbumMove struct {
	From   int64   // The album's row.
	Tracks []int64 // The rows of the tracks the edit wrote.
}

// MergeEmptied merges each album an edit left with no tracks into the album its
// tracks went to, when they all went to one: that album keeps its own cover, or
// takes the other's, its first-seen time becomes the older of the two, and the
// other's hearts move to it. The emptied album is left for [Store.PruneOrphans].
// It returns how many albums it merged.
func (s *Store) MergeEmptied(ctx context.Context, moves []AlbumMove) (int, error) {
	merged := 0
	err := s.inLibraryTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		merged = 0
		for i := range moves {
			into, err := mergeTarget(ctx, q, &moves[i])
			if err != nil {
				return fmt.Errorf("album %d: %w", moves[i].From, err)
			}
			if into == 0 {
				continue
			}
			arg := sqlcdb.MergeAlbumIntoParams{FromID: moves[i].From, IntoID: into}
			if err = q.MergeAlbumInto(ctx, arg); err != nil {
				return fmt.Errorf("album %d: %w", arg.FromID, err)
			}
			if err = q.MoveAlbumHearts(ctx, sqlcdb.MoveAlbumHeartsParams{IntoID: into, FromID: arg.FromID}); err != nil {
				return fmt.Errorf("album %d: hearts: %w", arg.FromID, err)
			}
			merged++
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("merge albums: %w", err)
	}
	return merged, nil
}

// mergeTarget returns the row of the one album m's tracks went to, or 0 when m's
// album still has tracks or they went to several.
func mergeTarget(ctx context.Context, q *sqlcdb.Queries, m *AlbumMove) (int64, error) {
	left, err := q.CountAlbumTracks(ctx, m.From)
	if err != nil || left != 0 {
		return 0, err //nolint:wrapcheck // The caller names the album.
	}
	var into int64
	for _, track := range m.Tracks {
		album, rowErr := q.TrackAlbumRow(ctx, track)
		if rowErr != nil {
			return 0, rowErr //nolint:wrapcheck // The caller names the album.
		}
		if into != 0 && album != into {
			return 0, nil
		}
		into = album
	}
	if into == m.From {
		return 0, nil
	}
	return into, nil
}

// TrackAlbumRows returns the album rows of the tracks with the given rows, each
// once, in the order the tracks first name them.
func (s *Store) TrackAlbumRows(ctx context.Context, tracks []int64) ([]int64, error) {
	var out []int64
	for _, track := range tracks {
		album, err := s.rq.TrackAlbumRow(ctx, track)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("album of track %d: %w", track, err)
		}
		if !slices.Contains(out, album) {
			out = append(out, album)
		}
	}
	return out, nil
}

// SettleAlbums gives each album the year and compilation flag of its first
// track, by disc, track number and path, as covers are chosen: with the tracks
// of an edit written one at a time, the last one written must not decide. An
// album that is gone is skipped.
func (s *Store) SettleAlbums(ctx context.Context, albums []int64) error {
	if len(albums) == 0 {
		return nil
	}
	err := s.inLibraryTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		for _, album := range albums {
			if err := q.SettleAlbum(ctx, album); err != nil {
				return fmt.Errorf("album %d: %w", album, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("settle albums: %w", err)
	}
	return nil
}

// CoverFile is a file an album's cover may come from.
type CoverFile struct{ Root, RelPath string }

// AlbumCoverFiles returns the files of an album's live tracks, first track
// first, as [Store.SettleAlbums] orders them.
func (s *Store) AlbumCoverFiles(ctx context.Context, album int64) ([]CoverFile, error) {
	rows, err := s.rq.ListAlbumCoverFiles(ctx, album)
	if err != nil {
		return nil, fmt.Errorf("album %d: cover files: %w", album, err)
	}
	out := make([]CoverFile, len(rows))
	for i := range rows {
		out[i] = CoverFile{Root: rows[i].Root, RelPath: rows[i].RelPath}
	}
	return out, nil
}

// SetAlbumArtwork makes art the album's cover.
//
// It returns an error wrapping [ErrInvalidWrite], and writes nothing, for art
// that [Batch] would refuse.
func (s *Store) SetAlbumArtwork(ctx context.Context, album int64, art *ArtworkWrite) error {
	if err := art.validate(); err != nil {
		return fmt.Errorf("album %d artwork: %w", album, err)
	}
	err := s.inLibraryTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		err := q.UpsertArtwork(ctx, sqlcdb.UpsertArtworkParams{
			Hash: art.Hash[:], Width: int64(art.Width), Height: int64(art.Height),
			Accent: art.Accent, Ground: art.Ground, Source: string(art.Source),
		})
		if err != nil {
			return fmt.Errorf("upsert artwork: %w", err)
		}
		return q.SetAlbumArtwork(ctx, sqlcdb.SetAlbumArtworkParams{Hash: art.Hash[:], ID: album})
	})
	if err != nil {
		return fmt.Errorf("album %d artwork: %w", album, err)
	}
	return nil
}
