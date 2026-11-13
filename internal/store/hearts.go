package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// HeartsSummary is what a person kept of their hearts from before stars were
// the only like, for the one-time change that turns them into stars. A person
// with none has the zero value.
type HeartsSummary struct {
	// ByStars counts the hearted tracks rated each whole star, 1★ to 5★: a half
	// rounds up, so 4½★ counts as 5★.
	ByStars [rating.Max / 2]int
	// Unrated counts the hearted tracks with no rating.
	Unrated int
	// Albums and Artists count the album and artist hearts.
	Albums, Artists int
	// Preview is a few of the unrated hearted tracks, the latest hearted first.
	Preview []HeartedTrack
}

// Rated counts the hearted tracks that have a rating.
func (h *HeartsSummary) Rated() int {
	n := 0
	for _, c := range h.ByStars {
		n += c
	}
	return n
}

// HeartedTrack is a hearted track in a [HeartsSummary]'s preview.
type HeartedTrack struct {
	ID     string // The track's ULID.
	Title  string
	Artist string // Its first artist, or "".
}

// previewSize is how many tracks a [HeartsSummary] previews.
const previewSize = 5

// HeartsSummary counts the user's hearts by kind, and the hearted tracks by
// their rating.
func (s *Store) HeartsSummary(ctx context.Context, userID int64) (HeartsSummary, error) {
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return HeartsSummary{}, fmt.Errorf("hearts summary: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Nothing was written: there is nothing to undo.
	q := s.rq.WithTx(tx)
	var out HeartsSummary
	counts, err := q.CountHeartedTracksByRating(ctx, userID)
	if err != nil {
		return HeartsSummary{}, fmt.Errorf("hearts summary: tracks: %w", err)
	}
	for _, c := range counts {
		r := ratingOf(c.Halves)
		if !r.Valid() {
			out.Unrated += int(c.Songs)
			continue
		}
		whole := int(math.Ceil(r.Stars())) // 1 to 5, a half rounding up.
		out.ByStars[whole-1] += int(c.Songs)
	}
	albums, err := q.CountAlbumHearts(ctx, userID)
	if err != nil {
		return HeartsSummary{}, fmt.Errorf("hearts summary: albums: %w", err)
	}
	artists, err := q.CountArtistHearts(ctx, userID)
	if err != nil {
		return HeartsSummary{}, fmt.Errorf("hearts summary: artists: %w", err)
	}
	out.Albums, out.Artists = int(albums), int(artists)
	preview, err := q.ListUnratedHeartedTracks(ctx, sqlcdb.ListUnratedHeartedTracksParams{UserID: userID, Limit: previewSize})
	if err != nil {
		return HeartsSummary{}, fmt.Errorf("hearts summary: preview: %w", err)
	}
	for _, p := range preview {
		out.Preview = append(out.Preview, HeartedTrack{ID: p.Ulid, Title: p.Title, Artist: p.Artist})
	}
	return out, nil
}

// HeartsConverted is what [Store.ConvertHearts] wrote.
type HeartsConverted struct {
	// Rated are the tracks given stars, by ULID.
	Rated []string
	// Tracks, Albums and Artists count the hearts taken off.
	Tracks, Albums, Artists int
}

// ConvertHearts turns the user's hearts into stars, once, in one transaction:
// every hearted track with no rating gets stars, or stays unrated when stars is
// [rating.None], and every track, album and artist heart of theirs goes.
// Ratings they had stay as they are. Nothing reaches Last.fm: those tracks were
// loved through their hearts already. A second call finds nothing to do.
func (s *Store) ConvertHearts(ctx context.Context, userID int64, stars rating.Rating) (HeartsConverted, error) {
	var out HeartsConverted
	at := ms(s.now())
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		out = HeartsConverted{}
		if stars.Valid() {
			unrated, err := q.ListUnratedHeartedTracks(ctx, sqlcdb.ListUnratedHeartedTracksParams{UserID: userID, Limit: -1})
			if err != nil {
				return fmt.Errorf("unrated hearts: %w", err)
			}
			for _, t := range unrated {
				err = q.UpsertTrackRating(ctx, sqlcdb.UpsertTrackRatingParams{UserID: userID, TrackID: t.ID, Halves: stars.Halves(), RatedAt: at})
				if err != nil {
					return fmt.Errorf("rate %s: %w", t.Ulid, err)
				}
				out.Rated = append(out.Rated, t.Ulid)
			}
		}
		for _, del := range []struct {
			n    *int
			kind Kind
			run  func(context.Context, int64) (int64, error)
		}{
			{&out.Tracks, KindTrack, q.DeleteTrackHearts},
			{&out.Albums, KindAlbum, q.DeleteAlbumHearts},
			{&out.Artists, KindArtist, q.DeleteArtistHearts},
		} {
			n, err := del.run(ctx, userID)
			if err != nil {
				return fmt.Errorf("%s hearts: %w", del.kind, err)
			}
			*del.n = int(n)
		}
		return nil
	})
	if err != nil {
		return HeartsConverted{}, fmt.Errorf("convert hearts: %w", err)
	}
	return out, nil
}
