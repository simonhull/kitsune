package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// LoudnessOutcome is what came of measuring a file's loudness.
type LoudnessOutcome string

// The outcomes of a measurement.
const (
	LoudnessMeasured    LoudnessOutcome = "measured"
	LoudnessUnsupported LoudnessOutcome = "unsupported" // A format or codec nothing measures yet.
	LoudnessUnreadable  LoudnessOutcome = "unreadable"  // The file could not be read or decoded.
)

// LoudnessWaiting is a live track waiting for its file's loudness: it is on an
// album with a song that has no ReplayGain, and has no measure of its file at
// the measurement version asked about.
type LoudnessWaiting struct {
	ID            int64
	Root, RelPath string
	Size          int64
	MTime         time.Time
	AlbumID       int64
	Format, Codec string
	Channels      int // 0 is unknown.
}

// CountLoudnessWaiting counts the tracks waiting for a measure at version.
func (s *Store) CountLoudnessWaiting(ctx context.Context, version int) (int, error) {
	n, err := s.rq.CountLoudnessWaiting(ctx, int64(version))
	if err != nil {
		return 0, fmt.Errorf("count loudness waiting: %w", err)
	}
	return int(n), nil
}

// LoudnessWaiting returns up to limit tracks waiting for a measure at version
// that come after the file at after, in path order.
func (s *Store) LoudnessWaiting(ctx context.Context, after FileKey, version, limit int) ([]LoudnessWaiting, error) {
	rows, err := s.rq.LoudnessWaiting(ctx, sqlcdb.LoudnessWaitingParams{
		Root: after.Root, RelPath: after.RelPath, Version: int64(version), N: int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("loudness waiting: %w", err)
	}
	out := make([]LoudnessWaiting, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = LoudnessWaiting{
			ID: r.ID, Root: r.Root, RelPath: r.RelPath, Size: r.Size, MTime: fromMS(r.Mtime),
			AlbumID: r.AlbumID, Format: r.Format, Codec: r.Codec, Channels: intOr0(r.Channels),
		}
	}
	return out, nil
}

// TrackLoudness is a measure of one file. Integrated is in LUFS and means
// nothing when Silent is set; peaks are linear, 1 being full scale. Blocks is
// the encoded histogram an album pools. A file that was not measured has only
// its Outcome.
type TrackLoudness struct {
	Outcome              LoudnessOutcome
	Integrated           float64
	Silent               bool
	TruePeak, SamplePeak float64
	Blocks               []byte
}

// SetTrackLoudness stores track id's measure, read from a file of size and
// mtime by measurement version. It writes only while the track is live and
// still has that size and mtime, and reports whether it did. The track's album
// is measured again once all its tracks are.
func (s *Store) SetTrackLoudness(ctx context.Context, id, size int64, mtime time.Time, version int, m *TrackLoudness) (bool, error) {
	p := sqlcdb.SetTrackLoudnessParams{
		ID: id, Size: size, Mtime: ms(mtime), Version: int64(version), Outcome: string(m.Outcome),
	}
	if m.Outcome == LoudnessMeasured {
		p.TruePeak, p.SamplePeak, p.Blocks = &m.TruePeak, &m.SamplePeak, m.Blocks
		if !m.Silent {
			p.Integrated = &m.Integrated
		}
	}
	n, err := s.wq.SetTrackLoudness(ctx, p)
	if err != nil {
		return false, named(fmt.Errorf("set loudness of track %d: %w", id, err))
	}
	return n == 1, nil
}

// AlbumsToPool returns the albums whose every live track has a measure of
// its file, or was found unmeasurable, but whose own measure is missing or
// pools other tracks.
func (s *Store) AlbumsToPool(ctx context.Context) ([]int64, error) {
	ids, err := s.rq.AlbumsToPool(ctx)
	if err != nil {
		return nil, fmt.Errorf("albums to pool: %w", err)
	}
	return ids, nil
}

// PooledTrack is one measured track of an album being pooled.
type PooledTrack struct {
	TruePeak float64
	Blocks   []byte
}

// AlbumLoudness is an album's measure: its pooled loudness in LUFS, which
// means nothing when Silent is set, and its true peak, linear.
type AlbumLoudness struct {
	Integrated float64
	Silent     bool
	TruePeak   float64
}

// PoolAlbum measures album albumID from its live tracks' measures, in one
// transaction: pool gets the tracks that were measured, and its result is
// stored as the album's. It reports false, storing nothing, while a live track
// of the album waits for a measure of its file. An error from pool is returned
// as is.
func (s *Store) PoolAlbum(ctx context.Context, albumID int64, pool func([]PooledTrack) (AlbumLoudness, error)) (bool, error) {
	ready := false
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		rows, err := q.AlbumLoudnessInputs(ctx, albumID)
		if err != nil {
			return fmt.Errorf("inputs: %w", err)
		}
		measured, ok := pooledTracks(rows)
		if !ok {
			return nil
		}
		album, err := pool(measured)
		if err != nil {
			return err
		}
		ready = true
		return storePool(ctx, q, albumID, rows, len(measured) > 0, &album)
	})
	if err != nil {
		return false, fmt.Errorf("pool album %d: %w", albumID, err)
	}
	return ready, nil
}

// pooledTracks returns the measured tracks of an album's rows, and false when
// it has none, or one still waits for a measure.
func pooledTracks(rows []sqlcdb.AlbumLoudnessInputsRow) ([]PooledTrack, bool) {
	var measured []PooledTrack
	for i := range rows {
		r := &rows[i]
		if r.MeasuredID == nil {
			return nil, false
		}
		if r.Outcome != nil && LoudnessOutcome(*r.Outcome) == LoudnessMeasured {
			measured = append(measured, PooledTrack{TruePeak: floatOr0(r.TruePeak), Blocks: r.Blocks})
		}
	}
	return measured, len(rows) > 0
}

// storePool replaces album albumID's measure with album, pooling rows.
func storePool(ctx context.Context, q *sqlcdb.Queries, albumID int64, rows []sqlcdb.AlbumLoudnessInputsRow, measured bool, album *AlbumLoudness) error {
	if err := q.DeleteAlbumLoudness(ctx, albumID); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	p := sqlcdb.InsertAlbumLoudnessParams{AlbumID: albumID, Tracks: int64(len(rows))}
	if measured {
		p.TruePeak = &album.TruePeak
		if !album.Silent {
			p.Integrated = &album.Integrated
		}
	}
	id, err := q.InsertAlbumLoudness(ctx, p)
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	for i := range rows {
		if err = q.PoolTrackLoudness(ctx, sqlcdb.PoolTrackLoudnessParams{Pool: &id, TrackID: rows[i].ID}); err != nil {
			return fmt.Errorf("pool track %d: %w", rows[i].ID, err)
		}
	}
	return nil
}

// LoudnessCounts counts the live songs with no ReplayGain, and of those, how
// many have Kitsune's measure of their file and how many can't be measured.
type LoudnessCounts struct{ Ungained, Measured, Unmeasurable int }

// CountLoudness returns the library's [LoudnessCounts].
func (s *Store) CountLoudness(ctx context.Context) (LoudnessCounts, error) {
	r, err := s.rq.LoudnessCounts(ctx)
	if err != nil {
		return LoudnessCounts{}, fmt.Errorf("count loudness: %w", err)
	}
	return LoudnessCounts{Ungained: int(r.Ungained), Measured: int(r.Measured), Unmeasurable: int(r.Unmeasurable)}, nil
}
