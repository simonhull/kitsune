package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// Kind is what a heart is on.
type Kind uint8

// The kinds of item a person can heart.
const (
	KindTrack Kind = iota + 1
	KindAlbum
	KindArtist
)

// String names the kind, for errors.
func (k Kind) String() string {
	switch k {
	case KindTrack:
		return "track"
	case KindAlbum:
		return "album"
	case KindArtist:
		return "artist"
	}
	return fmt.Sprintf("Kind(%d)", uint8(k))
}

// The stars a rating may have.
const (
	MinStars = 1
	MaxStars = 5
)

var (
	// ErrBadStars means a rating is outside [MinStars] to [MaxStars].
	ErrBadStars = errors.New("stars must be from 1 to 5")
	// errBadKind means a [Kind] is none of the constants: a programming error.
	errBadKind = errors.New("unknown kind")
)

// Marks are one person's marks on a track. Stars is 0 when it is unrated.
type Marks struct {
	Hearted bool
	Stars   int
}

// AlbumMarks are one person's marks on an album and its tracks. Tracks holds
// only the tracks with a mark, by ULID.
type AlbumMarks struct {
	Hearted bool
	Tracks  map[string]Marks
}

// Heart hearts the item of kind with the ULID id for the user. Hearting it again
// changes nothing, not even when it was first hearted. A track missing from disk
// still takes a heart: its file may be back. It returns an error wrapping
// [ErrNotFound] for an item that isn't in the library.
func (s *Store) Heart(ctx context.Context, userID int64, kind Kind, id string) error {
	at := ms(s.now())
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		rowID, err := rowIDOf(ctx, q, kind, id)
		if err != nil {
			return err
		}
		switch kind {
		case KindTrack:
			return q.InsertTrackHeart(ctx, sqlcdb.InsertTrackHeartParams{UserID: userID, TrackID: rowID, HeartedAt: at})
		case KindAlbum:
			return q.InsertAlbumHeart(ctx, sqlcdb.InsertAlbumHeartParams{UserID: userID, AlbumID: rowID, HeartedAt: at})
		case KindArtist:
			return q.InsertArtistHeart(ctx, sqlcdb.InsertArtistHeartParams{UserID: userID, ArtistID: rowID, HeartedAt: at})
		}
		return errBadKind
	})
	if err != nil {
		return fmt.Errorf("heart %s %s: %w", kind, id, err)
	}
	return nil
}

// Unheart takes the user's heart off the item of kind with the ULID id, if it
// has one. It returns an error wrapping [ErrNotFound] for an item that isn't in
// the library.
func (s *Store) Unheart(ctx context.Context, userID int64, kind Kind, id string) error {
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		rowID, err := rowIDOf(ctx, q, kind, id)
		if err != nil {
			return err
		}
		switch kind {
		case KindTrack:
			return q.DeleteTrackHeart(ctx, sqlcdb.DeleteTrackHeartParams{UserID: userID, TrackID: rowID})
		case KindAlbum:
			return q.DeleteAlbumHeart(ctx, sqlcdb.DeleteAlbumHeartParams{UserID: userID, AlbumID: rowID})
		case KindArtist:
			return q.DeleteArtistHeart(ctx, sqlcdb.DeleteArtistHeartParams{UserID: userID, ArtistID: rowID})
		}
		return errBadKind
	})
	if err != nil {
		return fmt.Errorf("unheart %s %s: %w", kind, id, err)
	}
	return nil
}

// Rate gives the track with the ULID id the user's stars, in place of any it
// had. It returns an error wrapping [ErrBadStars] for stars outside [MinStars]
// to [MaxStars], and [ErrNotFound] for a track that isn't in the library.
func (s *Store) Rate(ctx context.Context, userID int64, id string, stars int) error {
	if stars < MinStars || stars > MaxStars {
		return fmt.Errorf("rate track %s: %w", id, ErrBadStars)
	}
	at := ms(s.now())
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		rowID, err := rowIDOf(ctx, q, KindTrack, id)
		if err != nil {
			return err
		}
		return q.UpsertTrackRating(ctx, sqlcdb.UpsertTrackRatingParams{UserID: userID, TrackID: rowID, Stars: int64(stars), RatedAt: at})
	})
	if err != nil {
		return fmt.Errorf("rate track %s: %w", id, err)
	}
	return nil
}

// Unrate clears the user's rating of the track with the ULID id, if it has one.
// It returns an error wrapping [ErrNotFound] for a track that isn't in the
// library.
func (s *Store) Unrate(ctx context.Context, userID int64, id string) error {
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		rowID, err := rowIDOf(ctx, q, KindTrack, id)
		if err != nil {
			return err
		}
		return q.DeleteTrackRating(ctx, sqlcdb.DeleteTrackRatingParams{UserID: userID, TrackID: rowID})
	})
	if err != nil {
		return fmt.Errorf("unrate track %s: %w", id, err)
	}
	return nil
}

// rowIDOf returns the row ID of the item of kind with the ULID id, or
// [ErrNotFound].
func rowIDOf(ctx context.Context, q *sqlcdb.Queries, kind Kind, id string) (int64, error) {
	var (
		rowID int64
		err   error
	)
	switch kind {
	case KindTrack:
		rowID, err = q.TrackRowID(ctx, id)
	case KindAlbum:
		rowID, err = q.AlbumRowID(ctx, id)
	case KindArtist:
		rowID, err = q.ArtistRowID(ctx, id)
	default:
		return 0, errBadKind
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return rowID, err //nolint:wrapcheck // Wrapped once, by the caller.
}

// AlbumMarks returns the user's marks on the album with the ULID id and on its
// tracks, read together. An unknown album has none.
func (s *Store) AlbumMarks(ctx context.Context, userID int64, id string) (AlbumMarks, error) {
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return AlbumMarks{}, fmt.Errorf("album %s marks: begin: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }() // Nothing was written: there is nothing to undo.
	q := s.rq.WithTx(tx)
	hearted, err := q.AlbumHearted(ctx, sqlcdb.AlbumHeartedParams{UserID: userID, Album: id})
	if err != nil {
		return AlbumMarks{}, fmt.Errorf("album %s marks: %w", id, err)
	}
	tracks, err := q.ListAlbumHeartedTracks(ctx, sqlcdb.ListAlbumHeartedTracksParams{UserID: userID, Album: id})
	if err != nil {
		return AlbumMarks{}, fmt.Errorf("album %s marks: hearts: %w", id, err)
	}
	ratings, err := q.ListAlbumTrackRatings(ctx, sqlcdb.ListAlbumTrackRatingsParams{UserID: userID, Album: id})
	if err != nil {
		return AlbumMarks{}, fmt.Errorf("album %s marks: ratings: %w", id, err)
	}
	marks := AlbumMarks{Hearted: hearted, Tracks: make(map[string]Marks, len(tracks)+len(ratings))}
	for _, t := range tracks {
		marks.Tracks[t] = Marks{Hearted: true}
	}
	for _, r := range ratings {
		m := marks.Tracks[r.Ulid]
		m.Stars = int(r.Stars)
		marks.Tracks[r.Ulid] = m
	}
	return marks, nil
}

// libraryMarks is everything one person has marked, by ULID.
type libraryMarks struct {
	tracks, albums, artists map[string]bool
	stars                   map[string]int
}

// readLibraryMarks reads every mark of the user through q.
func readLibraryMarks(ctx context.Context, q *sqlcdb.Queries, userID int64) (libraryMarks, error) {
	var m libraryMarks
	var err error
	if m.tracks, err = heartSet(q.ListHeartedTracks(ctx, userID)); err != nil {
		return libraryMarks{}, fmt.Errorf("track hearts: %w", err)
	}
	if m.albums, err = heartSet(q.ListHeartedAlbums(ctx, userID)); err != nil {
		return libraryMarks{}, fmt.Errorf("album hearts: %w", err)
	}
	if m.artists, err = heartSet(q.ListHeartedArtists(ctx, userID)); err != nil {
		return libraryMarks{}, fmt.Errorf("artist hearts: %w", err)
	}
	ratings, err := q.ListTrackRatings(ctx, userID)
	if err != nil {
		return libraryMarks{}, fmt.Errorf("ratings: %w", err)
	}
	m.stars = make(map[string]int, len(ratings))
	for _, r := range ratings {
		m.stars[r.Ulid] = int(r.Stars)
	}
	return m, nil
}

func heartSet(ids []string, err error) (map[string]bool, error) {
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set, nil
}
