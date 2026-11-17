package store

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// A ReplayGain run writes Kitsune's own loudness measure into the files of
// songs that carry no ReplayGain. The store says which songs a run may write
// and with what, from the measures the loudness pass keeps, and keeps a
// journal of every write, made before the write, and the runs.

// The kinds of ReplayGain run, and their states.
const (
	GainKindRun  = "run"
	GainKindUndo = "undo"

	GainRunning = "running"
	GainDone    = "done"
	GainStopped = "stopped"
	GainUndone  = "undone"
)

// The states of a journal row.
const (
	GainPlanned = "planned"
	GainWritten = "written"
	GainSkipped = "skipped"
	GainUndid   = "undone" // A write an undo took out again.
)

// GainMeasure is what Kitsune measured of a song and its album, as it plays
// them: Track in LUFS and TrackPeak linear; Album and AlbumPeak the album's
// pooled measure when HasAlbum is set. A run writes the gains these make.
type GainMeasure struct {
	Track, TrackPeak float64
	Album, AlbumPeak float64
	HasAlbum         bool
}

// GainSong is a song a run may write: where its file is, which file it is,
// and its measure.
type GainSong struct {
	ID            int64
	ULID, Title   string
	Root, RelPath string
	Size          int64
	MTime         time.Time
	Format        string
	Album         string // The album's ULID.
	Measure       GainMeasure
}

// GainAlbum is an album with songs a run may write, in disc and track order.
type GainAlbum struct {
	ID            string // ULID.
	Title, Artist string
	Year          int
	Artwork       *Cover
	Songs         []GainSong
}

// Formats returns the formats of a's songs, most songs first.
func (a *GainAlbum) Formats() []string {
	return formatsOf(func(yield func(string) bool) {
		for i := range a.Songs {
			if !yield(a.Songs[i].Format) {
				return
			}
		}
	})
}

// formatsOf returns the formats seq yields, each once, the most frequent
// first, then by name.
func formatsOf(seq func(func(string) bool)) []string {
	count := map[string]int{}
	for f := range seq {
		count[f]++
	}
	out := make([]string, 0, len(count))
	for f := range count {
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b string) int {
		return cmp.Or(count[b]-count[a], strings.Compare(a, b))
	})
	return out
}

// GainLibrary is what a run may write now, and what waits.
type GainLibrary struct {
	// Albums are the albums with songs a run may write, by artist then title.
	Albums []GainAlbum
	// Waiting counts the songs whose measure, or whose album's pool, the
	// loudness pass hasn't made yet; Unmeasurable those it can't measure.
	Waiting, Unmeasurable int
}

// Songs counts the songs of l a run may write.
func (l *GainLibrary) Songs() int {
	n := 0
	for i := range l.Albums {
		n += len(l.Albums[i].Songs)
	}
	return n
}

// Formats returns the formats of the songs of l a run may write, most first.
func (l *GainLibrary) Formats() []string {
	return formatsOf(func(yield func(string) bool) {
		for i := range l.Albums {
			for j := range l.Albums[i].Songs {
				if !yield(l.Albums[i].Songs[j].Format) {
					return
				}
			}
		}
	})
}

// GainWhy is why a song can't be written now, or nothing.
type GainWhy int

// Why a song can't be written now.
const (
	GainWritable     GainWhy = iota
	GainWaiting              // No measure of its file at the version, or its album isn't pooled from the files as they are.
	GainUnmeasurable         // Measured as unsupported or unreadable, or silent: Kitsune plays it without gain.
)

// gainWhy says whether a song with this measure of its file and its album's
// pool can be written, and with what. version is the measurement version.
func gainWhy(version int, mVersion *int64, outcome *string, integrated, peak, pool, poolPeak *float64, poolCurrent bool) (GainWhy, GainMeasure) {
	switch {
	case outcome == nil || mVersion == nil || *mVersion != int64(version):
		return GainWaiting, GainMeasure{}
	case LoudnessOutcome(*outcome) != LoudnessMeasured || integrated == nil || peak == nil:
		return GainUnmeasurable, GainMeasure{}
	case !poolCurrent:
		return GainWaiting, GainMeasure{}
	}
	m := GainMeasure{Track: *integrated, TrackPeak: *peak}
	if pool != nil && poolPeak != nil {
		m.Album, m.AlbumPeak, m.HasAlbum = *pool, *poolPeak, true
	}
	return GainWritable, m
}

// GainLibrary returns the songs a run may write now, by album, and counts what
// waits: every live song with no ReplayGain tagged whose measure of its file
// is current at measurement version, and whose album's pool is current, unless
// a run wrote that very file already. No file is read.
func (s *Store) GainLibrary(ctx context.Context, version int) (*GainLibrary, error) {
	rows, err := s.rq.GainCandidates(ctx)
	if err != nil {
		return nil, fmt.Errorf("gain library: %w", err)
	}
	out := &GainLibrary{}
	at := map[int64]int{} // Album row → its index in out.Albums.
	for i := range rows {
		r := &rows[i]
		if r.Written != 0 {
			continue // The gain a run wrote rounds to 0.00 dB, which reads as none.
		}
		why, m := gainWhy(version, r.MeasureVersion, r.MeasureOutcome, r.MeasureIntegrated, r.MeasurePeak,
			r.PoolIntegrated, r.PoolPeak, r.PoolCurrent != 0)
		switch why {
		case GainWaiting:
			out.Waiting++
			continue
		case GainUnmeasurable:
			out.Unmeasurable++
			continue
		case GainWritable:
		}
		j, ok := at[r.AlbumID]
		if !ok {
			j = len(out.Albums)
			at[r.AlbumID] = j
			out.Albums = append(out.Albums, GainAlbum{
				ID: r.Album, Title: r.AlbumTitle, Artist: r.AlbumArtist, Year: intOr0(r.AlbumYear),
				Artwork: cover(r.ArtworkHash, r.ArtworkWidth, r.ArtworkHeight, r.ArtworkAccent, r.ArtworkGround),
			})
		}
		out.Albums[j].Songs = append(out.Albums[j].Songs, GainSong{
			ID: r.ID, ULID: r.Ulid, Title: r.Title, Root: r.Root, RelPath: r.RelPath, Size: r.Size,
			MTime: fromMS(r.Mtime), Format: r.Format, Album: r.Album, Measure: m,
		})
	}
	slices.SortStableFunc(out.Albums, func(a, b GainAlbum) int {
		return cmp.Or(strings.Compare(strings.ToLower(a.Artist), strings.ToLower(b.Artist)),
			strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title)), strings.Compare(a.ID, b.ID))
	})
	return out, nil
}

// GainTrackState is a song as a batch finds it before writing it, or an undo
// before putting it back.
type GainTrackState struct {
	ID            int64
	Root, RelPath string
	Size          int64
	MTime         time.Time
	Missing       bool // Out of the library: missing, or trashed.
	Trashed       bool
	Ungained      bool // Its row reads no ReplayGain.
	Why           GainWhy
	Measure       GainMeasure // What it would be written with now, when Why is GainWritable.
}

// GainTrackStates returns the tracks ids as they are now, by row, at
// measurement version. A row that is gone is not in the map.
func (s *Store) GainTrackStates(ctx context.Context, ids []int64, version int) (map[int64]GainTrackState, error) {
	rows, err := s.rq.GainTrackStates(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("gain track states: %w", err)
	}
	out := make(map[int64]GainTrackState, len(rows))
	for i := range rows {
		r := &rows[i]
		why, m := gainWhy(version, r.MeasureVersion, r.MeasureOutcome, r.MeasureIntegrated, r.MeasurePeak,
			r.PoolIntegrated, r.PoolPeak, r.PoolCurrent != 0)
		out[r.ID] = GainTrackState{
			ID: r.ID, Root: r.Root, RelPath: r.RelPath, Size: r.Size, MTime: fromMS(r.Mtime),
			Missing: r.MissingSince != nil, Trashed: r.TrashedAt != nil, Ungained: r.Ungained != 0,
			Why: why, Measure: m,
		}
	}
	return out, nil
}

// GainRunStart is a run about to start.
type GainRunStart struct {
	ULID, Kind    string
	By            int64 // 0 for none.
	At            time.Time
	Total, Albums int
	UndoOf        int64 // The run an undo undoes; 0 for none.
}

// StartGainRun records a run as running and returns its row.
func (s *Store) StartGainRun(ctx context.Context, r *GainRunStart) (int64, error) {
	var id int64
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		var err error
		id, err = q.CreateGainRun(ctx, sqlcdb.CreateGainRunParams{
			Ulid: r.ULID, Kind: r.Kind, ByUser: nullID(r.By), StartedAt: ms(r.At),
			Total: int64(r.Total), Albums: int64(r.Albums), UndoOf: nullID(r.UndoOf),
		})
		return err //nolint:wrapcheck // Wrapped below.
	})
	if err != nil {
		return 0, fmt.Errorf("start gain run: %w", err)
	}
	return id, nil
}

// GainValues are the ReplayGain values a run writes into one file: gains in
// dB on ReplayGain 2's scale, peaks linear. The album's are left out when
// HasAlbum is unset.
type GainValues struct {
	TrackGain, TrackPeak float64
	AlbumGain, AlbumPeak float64
	HasAlbum             bool
}

// GainWrite is a journal row: a song a run means to write, or wrote.
type GainWrite struct {
	ID            int64 // 0 before it is written.
	Run           int64
	Track         int64 // 0 once the track is deleted for good.
	Album         string
	Ord           int // Its place in the run.
	Root, RelPath string
	Values        GainValues
	// Prior is the ReplayGain tags the file held before, as JSON of each key
	// as spelled and its values: "{}" for none.
	Prior string
	// Undoes is, for an undo, the row of the write it takes out.
	Undoes int64
	// State and Reason are a row's state, and why it was skipped.
	State, Reason string
}

// JournalGain writes rows to run's journal in one transaction, each in its
// State (planned, or skipped with its Reason), and sets each one's ID.
func (s *Store) JournalGain(ctx context.Context, run int64, rows []GainWrite) error {
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		for i := range rows {
			w := &rows[i]
			p := sqlcdb.PlanGainWriteParams{
				RunID: run, TrackID: nullID(w.Track), Album: w.Album, Ord: int64(w.Ord), Root: w.Root, RelPath: w.RelPath,
				State: cmp.Or(w.State, GainPlanned), Reason: nullStr(w.Reason), TrackGain: w.Values.TrackGain,
				TrackPeak: w.Values.TrackPeak, Prior: cmp.Or(w.Prior, "{}"), Undoes: nullID(w.Undoes),
			}
			if w.Values.HasAlbum {
				p.AlbumGain, p.AlbumPeak = &w.Values.AlbumGain, &w.Values.AlbumPeak
			}
			id, err := q.PlanGainWrite(ctx, p)
			if err != nil {
				return fmt.Errorf("%s: %w", w.RelPath, err)
			}
			w.ID, w.Run, w.State = id, run, p.State
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("journal gain: %w", err)
	}
	return nil
}

// GainSettled is how one journaled write ended.
type GainSettled struct {
	ID     int64
	State  string // GainWritten or GainSkipped.
	Reason string // Why it was skipped.
	// Size and MTime are the file's after the write.
	Size  int64
	MTime time.Time
	// Undoes is, for an undo that wrote, the row whose write it took out:
	// that row becomes undone.
	Undoes int64
}

// GainTally is what a batch adds to its run's counts.
type GainTally struct{ Written, Left, Failed int }

// SettleGain settles a batch of run's journal in one transaction, and adds t
// to its counts.
func (s *Store) SettleGain(ctx context.Context, run int64, settled []GainSettled, t GainTally) error {
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		for i := range settled {
			w := &settled[i]
			p := sqlcdb.SettleGainWriteParams{State: w.State, Reason: nullStr(w.Reason), ID: w.ID}
			if w.State == GainWritten {
				p.SizeAfter, p.MtimeAfter = &w.Size, new(ms(w.MTime))
			}
			if err := q.SettleGainWrite(ctx, p); err != nil {
				return fmt.Errorf("row %d: %w", w.ID, err)
			}
			if w.Undoes != 0 && w.State == GainWritten {
				if err := q.SetGainWriteState(ctx, sqlcdb.SetGainWriteStateParams{State: GainUndid, ID: w.Undoes}); err != nil {
					return fmt.Errorf("row %d: %w", w.Undoes, err)
				}
			}
		}
		return q.CountGainRun(ctx, sqlcdb.CountGainRunParams{
			Written: int64(t.Written), LeftCount: int64(t.Left), Failed: int64(t.Failed), ID: run,
		})
	})
	if err != nil {
		return fmt.Errorf("settle gain: %w", err)
	}
	return nil
}

// FinishGainRun ends run in state at the time given.
func (s *Store) FinishGainRun(ctx context.Context, run int64, state string, at time.Time) error {
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		return q.FinishGainRun(ctx, sqlcdb.FinishGainRunParams{State: state, FinishedAt: new(ms(at)), ID: run})
	})
	if err != nil {
		return fmt.Errorf("finish gain run: %w", err)
	}
	return nil
}

// SetGainRunState sets a run's state: an undo that took out everything makes
// its run undone.
func (s *Store) SetGainRunState(ctx context.Context, run int64, state string) error {
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		return q.SetGainRunState(ctx, sqlcdb.SetGainRunStateParams{State: state, ID: run})
	})
	if err != nil {
		return fmt.Errorf("set gain run state: %w", err)
	}
	return nil
}

// GainRunInfo is a run as the runs list shows it.
type GainRunInfo struct {
	ID                    int64
	ULID, Kind, State     string
	By                    string // Empty once their account is gone.
	StartedAt             time.Time
	FinishedAt            *time.Time
	Total, Albums         int
	AlbumsDone            int // Albums it settled a song of.
	Written, Left, Failed int
	UndoOf, UndoneBy      string // ULIDs.
	// StillWritten says a write of the run is still in its file, as far as the
	// journal knows, so an undo would take something out.
	StillWritten bool
}

// GainRuns returns the latest limit runs, a running one first, then newest
// first.
func (s *Store) GainRuns(ctx context.Context, limit int) ([]GainRunInfo, error) {
	rows, err := s.rq.ListGainRuns(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("gain runs: %w", err)
	}
	out := make([]GainRunInfo, len(rows))
	for i := range rows {
		out[i] = gainRunInfo((*sqlcdb.GainRunRow)(&rows[i]))
	}
	return out, nil
}

// GainRunByULID returns the run with the public ID id, or an error wrapping
// [ErrNotFound].
func (s *Store) GainRunByULID(ctx context.Context, id string) (GainRunInfo, error) {
	row, err := s.rq.GainRun(ctx, id)
	if err != nil {
		return GainRunInfo{}, fmt.Errorf("gain run %s: %w", id, notFound(err))
	}
	return gainRunInfo(&row), nil
}

// gainRunInfo converts a run's row.
func gainRunInfo(r *sqlcdb.GainRunRow) GainRunInfo {
	info := GainRunInfo{
		ID: r.ID, ULID: r.Ulid, Kind: r.Kind, State: r.State, By: strOrEmpty(r.ByName), StartedAt: fromMS(r.StartedAt),
		Total: int(r.Total), Albums: int(r.Albums), AlbumsDone: int(r.AlbumsDone), Written: int(r.Written), Left: int(r.LeftCount), Failed: int(r.Failed),
		UndoOf: strOrEmpty(r.UndoOf), UndoneBy: r.UndoneBy, StillWritten: r.Undoable,
	}
	if r.FinishedAt != nil {
		info.FinishedAt = new(fromMS(*r.FinishedAt))
	}
	return info
}

// PlannedGain returns every journal row still planned, oldest first: the
// writes of runs a stopped server never settled.
func (s *Store) PlannedGain(ctx context.Context) ([]GainWrite, error) {
	rows, err := s.rq.PlannedGainWrites(ctx)
	if err != nil {
		return nil, fmt.Errorf("planned gain writes: %w", err)
	}
	out := make([]GainWrite, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = GainWrite{
			ID: r.ID, Run: r.RunID, Track: int64OrZero(r.TrackID), Album: r.Album, Ord: int(r.Ord), Root: r.Root,
			RelPath: r.RelPath, Values: gainValues(r.TrackGain, r.TrackPeak, r.AlbumGain, r.AlbumPeak), Prior: r.Prior,
			Undoes: int64OrZero(r.Undoes), State: GainPlanned,
		}
	}
	return out, nil
}

// gainValues builds a row's values.
func gainValues(trackGain, trackPeak float64, albumGain, albumPeak *float64) GainValues {
	v := GainValues{TrackGain: trackGain, TrackPeak: trackPeak}
	if albumGain != nil && albumPeak != nil {
		v.AlbumGain, v.AlbumPeak, v.HasAlbum = *albumGain, *albumPeak, true
	}
	return v
}

// GainMade is a write a run made, for its undo.
type GainMade struct {
	GainWrite

	Title string // The song's; empty once it is deleted for good.
}

// WrittenGain returns what run wrote and no undo took out, in the run's order.
func (s *Store) WrittenGain(ctx context.Context, run int64) ([]GainMade, error) {
	rows, err := s.rq.WrittenGainWrites(ctx, run)
	if err != nil {
		return nil, fmt.Errorf("written gain: %w", err)
	}
	out := make([]GainMade, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = GainMade{
			ID: r.ID, Run: run, Track: int64OrZero(r.TrackID), Album: r.Album, Ord: int(r.Ord), Root: r.Root,
			RelPath: r.RelPath, Values: gainValues(r.TrackGain, r.TrackPeak, r.AlbumGain, r.AlbumPeak),
			Prior: r.Prior, State: GainWritten, Title: strOrEmpty(r.Title),
		}
	}
	return out, nil
}

// GainRunAlbum is one album of a run, as its journal holds it.
type GainRunAlbum struct {
	Album                string // ULID.
	Songs, Written, Left int
	Planned              int
}

// GainRunAlbums returns run's albums as its journal holds them, in the order
// it took them.
func (s *Store) GainRunAlbums(ctx context.Context, run int64) ([]GainRunAlbum, error) {
	rows, err := s.rq.GainRunAlbums(ctx, run)
	if err != nil {
		return nil, fmt.Errorf("gain run albums: %w", err)
	}
	out := make([]GainRunAlbum, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = GainRunAlbum{Album: r.Album, Songs: int(r.Songs), Written: int(r.Written), Left: int(r.LeftCount), Planned: int(r.Planned)}
	}
	return out, nil
}

// GainAlbumFacts is what a run's album list shows of an album.
type GainAlbumFacts struct {
	Title, Artist string
	Year          int
	Artwork       *Cover
	Formats       []string // Of its live songs, by name.
}

// GainAlbumFacts returns the albums ids, by ULID. An album gone is not in the
// map.
func (s *Store) GainAlbumFacts(ctx context.Context, ids []string) (map[string]GainAlbumFacts, error) {
	if len(ids) == 0 {
		return map[string]GainAlbumFacts{}, nil
	}
	rows, err := s.rq.GainAlbumFacts(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("gain album facts: %w", err)
	}
	out := make(map[string]GainAlbumFacts, len(rows))
	for i := range rows {
		r := &rows[i]
		f := GainAlbumFacts{
			Title: r.Name, Artist: r.Artist, Year: intOr0(r.Year),
			Artwork: cover(r.ArtworkHash, r.ArtworkWidth, r.ArtworkHeight, r.ArtworkAccent, r.ArtworkGround),
			Formats: []string{},
		}
		if r.Formats != "" {
			f.Formats = strings.Split(r.Formats, ",")
			slices.Sort(f.Formats)
		}
		out[r.Ulid] = f
	}
	return out, nil
}

// GainLeft is a song a run left as it was.
type GainLeft struct {
	ULID, Title   string // Empty once the song is deleted for good.
	Root, RelPath string
	Reason        string
}

// LeftGain returns up to limit songs run left as they were, in its order.
func (s *Store) LeftGain(ctx context.Context, run int64, limit int) ([]GainLeft, error) {
	rows, err := s.rq.LeftGainWrites(ctx, sqlcdb.LeftGainWritesParams{RunID: run, Limit: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("left gain: %w", err)
	}
	out := make([]GainLeft, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = GainLeft{ULID: r.TrackUlid, Title: r.Title, Root: r.Root, RelPath: r.RelPath, Reason: strOrEmpty(r.Reason)}
	}
	return out, nil
}

// PruneGainRuns deletes the runs, with their journals, that are past the
// latest keep and started before cutoff: whichever keeps more. It returns how
// many went.
func (s *Store) PruneGainRuns(ctx context.Context, keep int, cutoff time.Time) (int64, error) {
	var n int64
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		var err error
		n, err = q.PruneGainRuns(ctx, sqlcdb.PruneGainRunsParams{Cutoff: ms(cutoff), Keep: int64(keep)})
		return err //nolint:wrapcheck // Wrapped below.
	})
	if err != nil {
		return 0, fmt.Errorf("prune gain runs: %w", err)
	}
	return n, nil
}

// StopRunningGainRuns marks every run still running stopped, as of at: what a
// server stopped midway left. It returns how many there were.
func (s *Store) StopRunningGainRuns(ctx context.Context, at time.Time) (int64, error) {
	var n int64
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		var err error
		n, err = q.StopRunningGainRuns(ctx, new(ms(at)))
		return err //nolint:wrapcheck // Wrapped below.
	})
	if err != nil {
		return 0, fmt.Errorf("stop running gain runs: %w", err)
	}
	return n, nil
}
