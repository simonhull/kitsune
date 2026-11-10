package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// artistIndex is [Store.LibraryIndex] for [IndexOptions.Artist]: the songs one
// artist is on, read in one transaction.
func (s *Store) artistIndex(ctx context.Context, userID int64, opts *IndexOptions) (LibraryIndex, error) {
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return LibraryIndex{}, fmt.Errorf("artist index: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Nothing was written: there is nothing to undo.
	q := s.rq.WithTx(tx)
	artist, err := q.GetArtist(ctx, opts.Artist)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && isVarious(artist.MatchKey, artist.Mbid)) {
		return LibraryIndex{}, fmt.Errorf("artist index %s: %w", opts.Artist, ErrNotFound)
	}
	if err != nil {
		return LibraryIndex{}, fmt.Errorf("artist index %s: %w", opts.Artist, err)
	}
	var rows indexRows
	if rows.marks, err = readLibraryMarks(ctx, q, userID); err != nil {
		return LibraryIndex{}, fmt.Errorf("artist index: %w", err)
	}
	albums, err := q.ListArtistAlbums(ctx, artist.ID)
	if err != nil {
		return LibraryIndex{}, fmt.Errorf("artist index: albums: %w", err)
	}
	if err = readArtistRows(ctx, q, userID, artist.ID, opts, &rows); err != nil {
		return LibraryIndex{}, fmt.Errorf("artist index: %w", err)
	}
	return LibraryIndex{Artists: []IndexArtist{artistHeading(&artist, albums, &rows)}}, nil
}

// artistHeading puts the songs rows holds under their albums, the artist's own
// first and then those it appears on, each run in the order albums has them.
func artistHeading(artist *sqlcdb.GetArtistRow, albums []sqlcdb.ListArtistAlbumsRow, rows *indexRows) IndexArtist {
	tracks := indexTracks(rows)
	discs := indexDiscs(rows.tracks)
	head := IndexArtist{
		ID: artist.Ulid, Name: artist.Name, SortName: artist.SortName, Hearted: rows.marks.artists[artist.Ulid],
		Albums: make([]IndexAlbum, 0, len(albums)),
	}
	for _, owned := range []bool{true, false} {
		for i := range albums {
			r := &albums[i]
			if (r.Owned != 0) != owned || len(tracks[r.ID]) == 0 {
				continue
			}
			head.Albums = append(head.Albums, IndexAlbum{
				ID: r.Ulid, Title: r.Name, SortName: r.SortName, Artist: r.Artist, Year: intOr0(r.Year),
				Compilation: r.Compilation != 0, Hearted: rows.marks.albums[r.Ulid], Tracks: tracks[r.ID], Discs: discs[r.ID],
				Cover: cover(r.ArtworkHash, r.ArtworkWidth, r.ArtworkHeight, r.ArtworkAccent, r.ArtworkGround),
			})
		}
	}
	return head
}

// readArtistRows reads the songs the artist with the row ID artist is on, and
// what opts asks for of them, through the index queries' by-artist twins.
func readArtistRows(ctx context.Context, q *sqlcdb.Queries, userID, artist int64, opts *IndexOptions, rows *indexRows) error {
	tracks, err := q.ListIndexTracksByArtist(ctx, artist)
	if err != nil {
		return fmt.Errorf("tracks: %w", err)
	}
	rows.tracks = convert(tracks, func(r sqlcdb.ListIndexTracksByArtistRow) sqlcdb.ListIndexTracksRow {
		return sqlcdb.ListIndexTracksRow(r)
	})
	artists, err := q.ListIndexTrackArtistsByArtist(ctx, artist)
	if err != nil {
		return fmt.Errorf("track artists: %w", err)
	}
	rows.artists = convert(artists, func(r sqlcdb.ListIndexTrackArtistsByArtistRow) sqlcdb.ListIndexTrackArtistsRow {
		return sqlcdb.ListIndexTrackArtistsRow(r)
	})
	rows.extra = opts.any()
	if detailErr := readArtistDetails(ctx, q, artist, opts, rows); detailErr != nil {
		return detailErr
	}
	return readArtistOwn(ctx, q, userID, artist, opts, rows)
}

// readArtistDetails runs, for an artist's index, the queries opts asks for of
// the files and their tags, and only those.
func readArtistDetails(ctx context.Context, q *sqlcdb.Queries, artist int64, opts *IndexOptions, rows *indexRows) error {
	if opts.Details {
		details, err := q.ListIndexTrackDetailsByArtist(ctx, artist)
		if err != nil {
			return fmt.Errorf("track details: %w", err)
		}
		rows.details = convert(details, func(r sqlcdb.ListIndexTrackDetailsByArtistRow) sqlcdb.ListIndexTrackDetailsRow {
			return sqlcdb.ListIndexTrackDetailsRow(r)
		})
	}
	if opts.AlbumArtists {
		names, err := q.ListIndexTrackAlbumArtistsByArtist(ctx, artist)
		if err != nil {
			return fmt.Errorf("track album artists: %w", err)
		}
		rows.albumArtists = convert(names, func(r sqlcdb.ListIndexTrackAlbumArtistsByArtistRow) sqlcdb.ListIndexTrackAlbumArtistsRow {
			return sqlcdb.ListIndexTrackAlbumArtistsRow(r)
		})
	}
	if len(opts.TagKeys) > 0 {
		tags, err := q.ListIndexTrackTagsByArtist(ctx, sqlcdb.ListIndexTrackTagsByArtistParams{ArtistID: artist, TagKeys: opts.TagKeys})
		if err != nil {
			return fmt.Errorf("track tags: %w", err)
		}
		rows.tags = convert(tags, func(r sqlcdb.ListIndexTrackTagsByArtistRow) sqlcdb.ListIndexTrackTagsRow {
			return sqlcdb.ListIndexTrackTagsRow(r)
		})
	}
	if opts.Measured {
		measures, err := q.ListIndexTrackMeasuresByArtist(ctx, artist)
		if err != nil {
			return fmt.Errorf("track measures: %w", err)
		}
		rows.measures = convert(measures, func(r sqlcdb.ListIndexTrackMeasuresByArtistRow) sqlcdb.ListIndexTrackMeasuresRow {
			return sqlcdb.ListIndexTrackMeasuresRow(r)
		})
	}
	return nil
}

// readArtistOwn reads, for an artist's index, what is the person's own: their
// shuffle rules and plays, when opts asks.
func readArtistOwn(ctx context.Context, q *sqlcdb.Queries, userID, artist int64, opts *IndexOptions, rows *indexRows) error {
	if opts.Shuffle {
		// The person's rules whole, as for some albums: a run's places count across albums.
		shuffle, err := q.ListIndexShuffle(ctx, userID)
		if err != nil {
			return fmt.Errorf("shuffle rules: %w", err)
		}
		rows.shuffle = shuffle
	}
	if opts.Plays {
		plays, err := q.ListIndexPlaysByArtist(ctx, sqlcdb.ListIndexPlaysByArtistParams{UserID: userID, ArtistID: artist})
		if err != nil {
			return fmt.Errorf("plays: %w", err)
		}
		rows.plays = convert(plays, func(r sqlcdb.ListIndexPlaysByArtistRow) sqlcdb.ListIndexPlaysRow { return sqlcdb.ListIndexPlaysRow(r) })
	}
	return nil
}
