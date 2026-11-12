package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// NewPlay is the input to [Store.RecordPlay].
type NewPlay struct {
	UserID   int64
	TrackID  string // The track's ULID.
	PlayedAt time.Time
	Source   string
	ClientID string // Unique among the user's plays: a second report of it changes nothing.
}

// Play is a play as [Store.RecentPlays] returns it.
type Play struct {
	TrackID  string
	PlayedAt time.Time
	Source   string
}

// RecordPlay stores a play, once however often it is reported, and queues its
// scrobble when the user sends plays to Last.fm. A track missing from disk
// still takes plays: its file may be back. It returns an error
// wrapping [ErrNotFound] for a track that isn't in the library.
func (s *Store) RecordPlay(ctx context.Context, p *NewPlay) error {
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		trackID, err := q.TrackRowID(ctx, p.TrackID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err //nolint:wrapcheck // Wrapped once, below.
		}
		n, err := q.InsertPlay(ctx, sqlcdb.InsertPlayParams{
			UserID:   p.UserID,
			TrackID:  trackID,
			PlayedAt: ms(p.PlayedAt),
			Source:   p.Source,
			ClientID: p.ClientID,
		})
		if err != nil || n == 0 { // A report seen before was queued for Last.fm then.
			return err //nolint:wrapcheck // Wrapped once, below.
		}
		return enqueueLastfm(ctx, q, p.UserID, trackID, OutboxScrobble, p.PlayedAt, s.now())
	})
	if err != nil {
		return fmt.Errorf("record play: %w", err)
	}
	return nil
}

// Listening is what a person has done with the library, counted.
type Listening struct {
	Plays  int // Plays that counted.
	Hearts int // Hearts on tracks, albums and artists.
	Rated  int // Tracks with stars.
}

// CountListening counts the user's plays, hearts and rated tracks.
func (s *Store) CountListening(ctx context.Context, userID int64) (Listening, error) {
	row, err := s.rq.CountUserListening(ctx, userID)
	if err != nil {
		return Listening{}, fmt.Errorf("count listening: %w", err)
	}
	return Listening{Plays: int(row.Plays), Hearts: int(row.Hearts), Rated: int(row.Rated)}, nil
}

// RecentPlays returns up to limit of a user's plays, newest first.
func (s *Store) RecentPlays(ctx context.Context, userID, limit int64) ([]Play, error) {
	rows, err := s.rq.RecentPlays(ctx, sqlcdb.RecentPlaysParams{UserID: userID, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("recent plays: %w", err)
	}
	plays := make([]Play, len(rows))
	for i, row := range rows {
		plays[i] = Play{TrackID: row.Ulid, PlayedAt: time.UnixMilli(row.PlayedAt).UTC(), Source: row.Source}
	}
	return plays, nil
}

// SourceUndated is the source of a play whose time is only the latest it can
// have happened, not when it did, as an import writes it. Counts over a span of
// time leave such plays out.
const SourceUndated = "navidrome-undated"

// MaxHomeAlbums is the most albums each of Home's history rows holds.
const MaxHomeAlbums = 25

// AlbumPlays is an album and how many times its songs were played.
type AlbumPlays struct {
	Album AlbumSummary
	Plays int
}

// History is what Home shows of a person's plays: the albums they played most
// recently, newest first, and the ones they played most since a time, most
// first. Only plays of live tracks count, undated ones only for Recent, and
// each list holds at most [MaxHomeAlbums].
type History struct {
	Recent   []AlbumSummary
	OnRepeat []AlbumPlays
}

// recentAlbumsSQL lists the albums of the user's plays of live tracks, newest
// play first, once per play. It is hand-written because the caller stops
// reading at [MaxHomeAlbums] distinct albums, and sqlc reads every row. CROSS
// JOIN keeps plays outermost, so plays_recent gives the order and no sort runs.
const recentAlbumsSQL = `SELECT a.ulid
FROM plays p CROSS JOIN tracks t ON t.id = p.track_id
JOIN albums a ON a.id = t.album_id
WHERE p.user_id = ? AND t.missing_since IS NULL
ORDER BY p.played_at DESC`

// History returns the user's recently played albums, and the ones they played
// most from since on.
func (s *Store) History(ctx context.Context, userID int64, since time.Time) (History, error) {
	recentIDs, err := s.recentAlbums(ctx, userID)
	if err != nil {
		return History{}, fmt.Errorf("history: %w", err)
	}
	repeats, err := s.rq.AlbumsOnRepeat(ctx, sqlcdb.AlbumsOnRepeatParams{
		UserID: userID, Since: ms(since), Undated: SourceUndated, Lim: MaxHomeAlbums,
	})
	if err != nil {
		return History{}, fmt.Errorf("history: on repeat: %w", err)
	}
	ids := slices.Clone(recentIDs)
	for _, r := range repeats {
		ids = append(ids, r.Ulid)
	}
	byID, err := s.albumsByID(ctx, ids)
	if err != nil {
		return History{}, fmt.Errorf("history: %w", err)
	}
	h := History{Recent: inOrder(recentIDs, byID), OnRepeat: make([]AlbumPlays, 0, len(repeats))}
	for _, r := range repeats {
		if a, ok := byID[r.Ulid]; ok { // An album can go between the reads.
			h.OnRepeat = append(h.OnRepeat, AlbumPlays{Album: a, Plays: int(r.Plays)})
		}
	}
	return h, nil
}

// recentAlbums returns the IDs of the user's [MaxHomeAlbums] most recently
// played albums, newest first. It reads plays only until it has them.
func (s *Store) recentAlbums(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.r.QueryContext(ctx, recentAlbumsSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("recent albums: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err has the outcome.
	ids := make([]string, 0, MaxHomeAlbums)
	for len(ids) < MaxHomeAlbums && rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("recent albums: %w", err)
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("recent albums: %w", err)
	}
	return ids, nil
}
