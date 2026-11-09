package store

import (
	"context"
	"fmt"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// readAlbumRows reads, for a partial index, the tracks of opts.Albums only,
// with what opts asks for of them. Every album is read all the same, which is
// cheap: the order of the whole index decides where each one goes.
func readAlbumRows(ctx context.Context, q *sqlcdb.Queries, userID int64, opts *IndexOptions, rows *indexRows) error {
	albums := opts.Albums
	tracks, err := q.ListIndexTracksOf(ctx, albums)
	if err != nil {
		return fmt.Errorf("tracks: %w", err)
	}
	rows.tracks = convert(tracks, func(r sqlcdb.ListIndexTracksOfRow) sqlcdb.ListIndexTracksRow { return sqlcdb.ListIndexTracksRow(r) })
	artists, err := q.ListIndexTrackArtistsOf(ctx, albums)
	if err != nil {
		return fmt.Errorf("track artists: %w", err)
	}
	rows.artists = convert(artists, func(r sqlcdb.ListIndexTrackArtistsOfRow) sqlcdb.ListIndexTrackArtistsRow {
		return sqlcdb.ListIndexTrackArtistsRow(r)
	})
	return readAlbumOptions(ctx, q, userID, opts, rows)
}

// readAlbumOptions runs, for a partial index, the queries opts asks for, and
// only those.
func readAlbumOptions(ctx context.Context, q *sqlcdb.Queries, userID int64, opts *IndexOptions, rows *indexRows) error {
	rows.extra = opts.any()
	albums := opts.Albums
	if opts.Details {
		details, err := q.ListIndexTrackDetailsOf(ctx, albums)
		if err != nil {
			return fmt.Errorf("track details: %w", err)
		}
		rows.details = convert(details, func(r sqlcdb.ListIndexTrackDetailsOfRow) sqlcdb.ListIndexTrackDetailsRow {
			return sqlcdb.ListIndexTrackDetailsRow(r)
		})
	}
	if opts.AlbumArtists {
		names, err := q.ListIndexTrackAlbumArtistsOf(ctx, albums)
		if err != nil {
			return fmt.Errorf("track album artists: %w", err)
		}
		rows.albumArtists = convert(names, func(r sqlcdb.ListIndexTrackAlbumArtistsOfRow) sqlcdb.ListIndexTrackAlbumArtistsRow {
			return sqlcdb.ListIndexTrackAlbumArtistsRow(r)
		})
	}
	if len(opts.TagKeys) > 0 {
		tags, err := q.ListIndexTrackTagsOf(ctx, sqlcdb.ListIndexTrackTagsOfParams{TagKeys: opts.TagKeys, Albums: albums})
		if err != nil {
			return fmt.Errorf("track tags: %w", err)
		}
		rows.tags = convert(tags, func(r sqlcdb.ListIndexTrackTagsOfRow) sqlcdb.ListIndexTrackTagsRow {
			return sqlcdb.ListIndexTrackTagsRow(r)
		})
	}
	if opts.Measured {
		measures, err := q.ListIndexTrackMeasuresOf(ctx, albums)
		if err != nil {
			return fmt.Errorf("track measures: %w", err)
		}
		rows.measures = convert(measures, func(r sqlcdb.ListIndexTrackMeasuresOfRow) sqlcdb.ListIndexTrackMeasuresRow {
			return sqlcdb.ListIndexTrackMeasuresRow(r)
		})
	}
	return readAlbumOwn(ctx, q, userID, opts, rows)
}

// readAlbumOwn reads, for a partial index, what is the person's own: their
// shuffle rules and plays, when opts asks.
func readAlbumOwn(ctx context.Context, q *sqlcdb.Queries, userID int64, opts *IndexOptions, rows *indexRows) error {
	albums := opts.Albums
	if opts.Shuffle {
		// The person's rules whole: a run spans albums, and its places count across them. They
		// are a person's few, not the library's many.
		shuffle, err := q.ListIndexShuffle(ctx, userID)
		if err != nil {
			return fmt.Errorf("shuffle rules: %w", err)
		}
		rows.shuffle = shuffle
	}
	if opts.Plays {
		plays, err := q.ListIndexPlaysOf(ctx, sqlcdb.ListIndexPlaysOfParams{UserID: userID, Albums: albums})
		if err != nil {
			return fmt.Errorf("plays: %w", err)
		}
		rows.plays = convert(plays, func(r sqlcdb.ListIndexPlaysOfRow) sqlcdb.ListIndexPlaysRow { return sqlcdb.ListIndexPlaysRow(r) })
	}
	return nil
}

// convert maps every row of in through f.
func convert[T, U any](in []T, f func(T) U) []U {
	out := make([]U, len(in))
	for i, r := range in {
		out[i] = f(r)
	}
	return out
}

// onlyAlbums keeps, of the whole ordered index, the albums whose public IDs are
// in ids, under their headings, each with the album before it in the whole
// index as its After.
func onlyAlbums(artists []IndexArtist, ids []string) []IndexArtist {
	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	out := []IndexArtist{}
	before := ""
	for i := range artists {
		a := &artists[i]
		var kept []IndexAlbum
		for j := range a.Albums {
			album := a.Albums[j]
			if _, ok := want[album.ID]; ok {
				album.After = before
				kept = append(kept, album)
			}
			before = album.ID
		}
		if len(kept) > 0 {
			heading := *a
			heading.Albums = kept
			out = append(out, heading)
		}
	}
	return out
}
