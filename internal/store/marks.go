package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// Kind is what a heart is on.
type Kind uint8

// The kinds of item a person could heart, before stars were the only like.
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

// errBadKind means a [Kind] is none of the constants: a programming error.
var errBadKind = errors.New("unknown kind")

// Heart hearts the item of kind with the ULID id for the user, as people did
// before stars were the only like. No route makes a heart any more, and nothing
// reads one but [Store.HeartsSummary] and [Store.ConvertHearts]: Heart stays so
// tests can stand in for the hearts people kept, until the tables go. Hearting
// again changes nothing, and nothing reaches Last.fm. It returns an error
// wrapping [ErrNotFound] for an item that isn't in the library.
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

// Rate gives the track with the ULID id the user's rating, in place of any it
// had. A rating that crosses the user's Last.fm love line queues a love or an
// unlove, as [loveCrossed] says. It returns an error wrapping [rating.ErrBad]
// for [rating.None] or anything else that isn't a rating, and [ErrNotFound]
// for a track that isn't in the library.
func (s *Store) Rate(ctx context.Context, userID int64, id string, r rating.Rating) error {
	if !r.Valid() {
		return fmt.Errorf("rate track %s: %w", id, rating.ErrBad)
	}
	now := s.now()
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		rowID, err := rowIDOf(ctx, q, KindTrack, id)
		if err != nil {
			return err
		}
		was, err := trackRating(ctx, q, userID, rowID)
		if err != nil {
			return err
		}
		err = q.UpsertTrackRating(ctx, sqlcdb.UpsertTrackRatingParams{UserID: userID, TrackID: rowID, Halves: r.Halves(), RatedAt: ms(now)})
		if err != nil {
			return err //nolint:wrapcheck // Wrapped once, below.
		}
		return loveCrossed(ctx, q, userID, rowID, was, r, now)
	})
	if err != nil {
		return fmt.Errorf("rate track %s: %w", id, err)
	}
	return nil
}

// Unrate clears the user's rating of the track with the ULID id, if it has one,
// and queues an unlove when the rating was at or over the love line, as Rate
// does. It returns an error wrapping [ErrNotFound] for a track that isn't in the
// library.
func (s *Store) Unrate(ctx context.Context, userID int64, id string) error {
	now := s.now()
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		rowID, err := rowIDOf(ctx, q, KindTrack, id)
		if err != nil {
			return err
		}
		was, err := trackRating(ctx, q, userID, rowID)
		if err != nil {
			return err
		}
		err = q.DeleteTrackRating(ctx, sqlcdb.DeleteTrackRatingParams{UserID: userID, TrackID: rowID})
		if err != nil {
			return err //nolint:wrapcheck // Wrapped once, below.
		}
		return loveCrossed(ctx, q, userID, rowID, was, rating.None, now)
	})
	if err != nil {
		return fmt.Errorf("unrate track %s: %w", id, err)
	}
	return nil
}

// trackRating is the user's rating of the track row, or [rating.None].
func trackRating(ctx context.Context, q *sqlcdb.Queries, userID, trackRow int64) (rating.Rating, error) {
	halves, err := q.GetTrackRating(ctx, sqlcdb.GetTrackRatingParams{UserID: userID, TrackID: trackRow})
	if errors.Is(err, sql.ErrNoRows) {
		return rating.None, nil
	}
	if err != nil {
		return rating.None, fmt.Errorf("rating: %w", err)
	}
	return ratingOf(halves), nil
}

// loveCrossed queues a love when a track's rating went from under the user's
// Last.fm love line, unrated included, to at or over it, and an unlove for the
// reverse. A change that stays on one side sends nothing, and neither does a
// user with no account; [enqueueLastfm] leaves out one with loves off.
func loveCrossed(ctx context.Context, q *sqlcdb.Queries, userID, trackRow int64, was, is rating.Rating, now time.Time) error {
	halves, err := q.GetLastfmLoveFrom(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("love line: %w", err)
	}
	line := ratingOf(halves)
	loved := func(r rating.Rating) bool { return r.Valid() && r >= line }
	switch {
	case !loved(was) && loved(is):
		return enqueueLastfm(ctx, q, userID, trackRow, OutboxLove, now, now)
	case loved(was) && !loved(is):
		return enqueueLastfm(ctx, q, userID, trackRow, OutboxUnlove, now, now)
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

// AlbumRatings returns the user's ratings of the tracks of the album with the
// ULID id, by track ULID: the rated ones only. An unknown album has none.
func (s *Store) AlbumRatings(ctx context.Context, userID int64, id string) (map[string]rating.Rating, error) {
	rows, err := s.rq.ListAlbumTrackRatings(ctx, sqlcdb.ListAlbumTrackRatingsParams{UserID: userID, Album: id})
	if err != nil {
		return nil, fmt.Errorf("album %s ratings: %w", id, err)
	}
	out := make(map[string]rating.Rating, len(rows))
	for _, r := range rows {
		out[r.Ulid] = ratingOf(r.Halves)
	}
	return out, nil
}

// readRatings reads every rating of the user through q, by track ULID.
func readRatings(ctx context.Context, q *sqlcdb.Queries, userID int64) (map[string]rating.Rating, error) {
	rows, err := q.ListTrackRatings(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("ratings: %w", err)
	}
	out := make(map[string]rating.Rating, len(rows))
	for _, r := range rows {
		out[r.Ulid] = ratingOf(r.Halves)
	}
	return out, nil
}

// ratingOf is a stored rating in halves. The table's CHECK keeps it from 1 to
// 10; anything else, 0 from an outer join included, is no rating.
func ratingOf(halves int64) rating.Rating {
	r, err := rating.FromHalves(halves)
	if err != nil {
		return rating.None
	}
	return r
}
