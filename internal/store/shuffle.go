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

// ErrShortRun means a run names fewer than two songs, or one twice.
var ErrShortRun = errors.New("a run takes two or more different songs")

// ShuffleRun is songs that always play together in shuffle, in order.
type ShuffleRun struct {
	ID     string   // The run's ULID.
	Tracks []string // Track ULIDs, in the order they play.
}

// ShuffleRules are a person's rules touching some songs.
type ShuffleRules struct {
	Runs     []ShuffleRun // Whole, even where only some of their songs were asked about.
	Excluded []string     // The songs asked about that shuffle leaves out, by ULID.
}

// ShuffleRulesOf returns the user's runs holding any of tracks, whole, and which
// of tracks they excluded. Unknown tracks are ignored.
func (s *Store) ShuffleRulesOf(ctx context.Context, userID int64, tracks []string) (ShuffleRules, error) {
	out := ShuffleRules{Runs: []ShuffleRun{}, Excluded: []string{}}
	if len(tracks) == 0 {
		return out, nil
	}
	rows, err := s.rq.ListRunsOf(ctx, sqlcdb.ListRunsOfParams{UserID: userID, Tracks: tracks})
	if err != nil {
		return ShuffleRules{}, fmt.Errorf("shuffle runs: %w", err)
	}
	for _, r := range rows {
		if n := len(out.Runs); n == 0 || out.Runs[n-1].ID != r.Run {
			out.Runs = append(out.Runs, ShuffleRun{ID: r.Run})
		}
		last := &out.Runs[len(out.Runs)-1]
		last.Tracks = append(last.Tracks, r.Track)
	}
	excluded, err := s.rq.ListExcludedOf(ctx, sqlcdb.ListExcludedOfParams{UserID: userID, Tracks: tracks})
	if err != nil {
		return ShuffleRules{}, fmt.Errorf("excluded songs: %w", err)
	}
	out.Excluded = append(out.Excluded, excluded...)
	return out, nil
}

// trackRows returns the row of each track, by ULID, in order. It returns an
// error wrapping [ErrNotFound] that names the first unknown one.
func trackRows(ctx context.Context, q *sqlcdb.Queries, tracks []string) ([]int64, error) {
	rows := make([]int64, len(tracks))
	for i, id := range tracks {
		row, err := q.TrackRowID(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("track %s: %w", id, ErrNotFound)
		}
		if err != nil {
			return nil, fmt.Errorf("track %s: %w", id, err)
		}
		rows[i] = row
	}
	return rows, nil
}

// SetRun makes tracks one of the user's runs, in that order, and returns it. A
// track in another run leaves it, and a run left with fewer than two songs goes.
// The run keeps the ID of the run its first song was in, when that run has gone,
// so a reorder is the same run; otherwise it takes newID, a 26-character ULID.
//
// It returns an error wrapping [ErrShortRun] for fewer than two tracks or one
// named twice, and one wrapping [ErrNotFound] for an unknown track.
func (s *Store) SetRun(ctx context.Context, userID int64, newID string, tracks []string) (ShuffleRun, error) {
	if len(tracks) < 2 || len(slices.Compact(slices.Sorted(slices.Values(tracks)))) != len(tracks) {
		return ShuffleRun{}, fmt.Errorf("set run: %w", ErrShortRun)
	}
	run := ShuffleRun{ID: newID, Tracks: slices.Clone(tracks)}
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		rows, err := trackRows(ctx, q, tracks)
		if err != nil {
			return err
		}
		kept, err := leaveRuns(ctx, q, userID, rows)
		if err != nil {
			return err
		}
		run.ID = cmp.Or(kept, run.ID)
		return insertRun(ctx, q, userID, &run, rows, ms(s.now()))
	})
	if err != nil {
		return ShuffleRun{}, fmt.Errorf("set run: %w", err)
	}
	return run, nil
}

// leaveRuns takes rows out of the user's runs. It returns the ULID of the run the
// first of them was in when that run has gone, for the new run to keep.
func leaveRuns(ctx context.Context, q *sqlcdb.Queries, userID int64, rows []int64) (string, error) {
	prior, err := firstRun(ctx, q, userID, rows)
	if err != nil {
		return "", err
	}
	for _, row := range rows {
		if err = q.LeaveRun(ctx, sqlcdb.LeaveRunParams{UserID: userID, TrackID: row}); err != nil {
			return "", fmt.Errorf("leave run: %w", err)
		}
	}
	if prior.Ulid == "" {
		return "", nil
	}
	still, err := q.RunExists(ctx, prior.ID)
	if err != nil || still {
		return "", err //nolint:wrapcheck // The caller wraps it once, as set run.
	}
	return prior.Ulid, nil
}

// firstRun returns the run the first of rows that is in one is in, or a zero
// row when none is.
func firstRun(ctx context.Context, q *sqlcdb.Queries, userID int64, rows []int64) (sqlcdb.RunOfTrackRow, error) {
	for _, row := range rows {
		run, err := q.RunOfTrack(ctx, sqlcdb.RunOfTrackParams{UserID: userID, TrackID: row})
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return sqlcdb.RunOfTrackRow{}, fmt.Errorf("run of track: %w", err)
		}
		return run, nil
	}
	return sqlcdb.RunOfTrackRow{}, nil
}

// insertRun stores run, its songs being rows.
func insertRun(ctx context.Context, q *sqlcdb.Queries, userID int64, run *ShuffleRun, rows []int64, now int64) error {
	id, err := q.InsertRun(ctx, sqlcdb.InsertRunParams{Ulid: run.ID, UserID: userID, CreatedAt: now})
	if err != nil {
		return fmt.Errorf("insert run: %w", err)
	}
	for position, row := range rows {
		err = q.InsertRunTrack(ctx, sqlcdb.InsertRunTrackParams{RunID: id, UserID: userID, Position: int64(position), TrackID: row})
		if err != nil {
			return fmt.Errorf("insert run track: %w", err)
		}
	}
	return nil
}

// DeleteRun deletes the user's run id, leaving its songs alone. It returns an
// error wrapping [ErrNotFound] when the user has no such run.
func (s *Store) DeleteRun(ctx context.Context, userID int64, id string) error {
	n, err := s.wq.DeleteRun(ctx, sqlcdb.DeleteRunParams{UserID: userID, Ulid: id})
	if err != nil {
		return fmt.Errorf("delete run %s: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("delete run %s: %w", id, ErrNotFound)
	}
	return nil
}

// SetExcluded leaves tracks out of the user's shuffle, or with on false lets them
// back in. Either is idempotent. It returns an error wrapping [ErrNotFound], and
// changes nothing, for an unknown track.
func (s *Store) SetExcluded(ctx context.Context, userID int64, tracks []string, on bool) error {
	now := ms(s.now())
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		rows, err := trackRows(ctx, q, tracks)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if on {
				err = q.InsertExcluded(ctx, sqlcdb.InsertExcludedParams{UserID: userID, TrackID: row, ExcludedAt: now})
			} else {
				err = q.DeleteExcluded(ctx, sqlcdb.DeleteExcludedParams{UserID: userID, TrackID: row})
			}
			if err != nil {
				return fmt.Errorf("track %d: %w", row, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("set excluded: %w", err)
	}
	return nil
}
