package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// The organizer moves songs' files to where a path template puts them. The
// store keeps what it reads of the library for that (beside the index's rows),
// a journal of every move written before the move is made, and the runs.

// OrganizeSong is a live track as the organizer reads it: where its file is,
// and what a template reads of it.
type OrganizeSong struct {
	ID          int64
	ULID, Title string
	Root        string
	RelPath     string // Slash-separated on every system Kitsune runs on.
	Size        int64
	MTime       time.Time // With Size, what a run checks to know the file is as previewed.
	AlbumID     int64
	Album       string // The album's ULID.
	AlbumTitle  string
	// AlbumArtist is who the album files under, as the index heads it: its
	// owner, or its display artist when it has none or it is Various Artists'.
	AlbumArtist  string
	Artwork      *Cover
	Artists      []string
	Year         int // The album's.
	OriginalYear int
	Disc, Discs  int // Discs counts the album's discs, as the index does.
	Track        int
	DiscSongs    int // Songs on the song's disc.
	DiscName     string
	Format       string
	Compilation  bool
}

// OrganizePath is a track's row and where its file is, under its root.
type OrganizePath struct {
	ID            int64
	Root, RelPath string
}

// OrganizeLibrary is the library as the organizer reads it.
type OrganizeLibrary struct {
	// Songs are every live track, in the Library's order: albums as the index
	// orders them, each album's songs by disc and track.
	Songs []OrganizeSong
	// Missing are the tracks out of the library and not in the Trash, whose
	// rows still hold their paths.
	Missing []OrganizePath
}

// OrganizeLibrary reads the library as the organizer needs it, from the rows
// kept for the index where it can: no audio file is read.
func (s *Store) OrganizeLibrary(ctx context.Context) (*OrganizeLibrary, error) {
	mark := s.library.before()
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("organize library: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // Read-only: nothing to undo.
	q := s.rq.WithTx(tx)
	// The first read takes the snapshot, which the kept rows must come after.
	missing, err := q.ListOrganizeMissing(ctx)
	if err != nil {
		return nil, fmt.Errorf("organize library: missing: %w", err)
	}
	b, err := s.smartBasis(ctx, q, mark)
	if err != nil {
		return nil, fmt.Errorf("organize library: %w", err)
	}
	l := &s.library
	files, err := read(l, &l.files, mark, "", func() ([]sqlcdb.ListOrganizeTrackFilesRow, error) { return q.ListOrganizeTrackFiles(ctx) })
	if err != nil {
		return nil, fmt.Errorf("organize library: files: %w", err)
	}
	details, err := s.trackDetails(ctx, q, mark, b)
	if err != nil {
		return nil, fmt.Errorf("organize library: %w", err)
	}
	out := &OrganizeLibrary{Songs: organizeSongs(b, files, details), Missing: make([]OrganizePath, len(missing))}
	for i := range missing {
		out.Missing[i] = OrganizePath{ID: missing[i].ID, Root: missing[i].Root, RelPath: missing[i].RelPath}
	}
	return out, nil
}

// organizeSongs builds every live track's song, in the Library's order.
func organizeSongs(b *smartBasis, files []sqlcdb.ListOrganizeTrackFilesRow, details map[int64]*sqlcdb.ListIndexTrackDetailsRow) []OrganizeSong {
	fileOf := make(map[int64]*sqlcdb.ListOrganizeTrackFilesRow, len(files))
	for i := range files {
		fileOf[files[i].ID] = &files[i]
	}
	shape := albumShapes(b.tracks)
	names := b.names()
	out := make([]OrganizeSong, 0, len(b.tracks))
	for i := range b.tracks {
		t := &b.tracks[i]
		f, ok := fileOf[t.ID]
		at, okAlbum := b.album[t.AlbumID]
		if !ok || !okAlbum {
			continue // A write landed between the reads: the next preview has it.
		}
		a := &b.albums[at]
		disc := intOr0(t.DiscNo)
		sh := shape[t.AlbumID]
		song := OrganizeSong{
			ID: t.ID, ULID: t.Ulid, Title: t.Title, Root: f.Root, RelPath: f.RelPath, Size: f.Size, MTime: fromMS(f.Mtime),
			AlbumID: a.ID, Album: a.Ulid, AlbumTitle: a.Name, AlbumArtist: indexHeading(a).Name,
			Artwork: cover(a.ArtworkHash, a.ArtworkWidth, a.ArtworkHeight, a.ArtworkAccent, a.ArtworkGround),
			Artists: names[t.ID], Year: intOr0(a.Year), OriginalYear: intOr0(f.OriginalYear),
			Disc: disc, Discs: sh.discs, Track: intOr0(t.TrackNo), DiscSongs: sh.songs[disc], DiscName: sh.names[disc],
			Compilation: a.Compilation == 1,
		}
		if d := details[t.ID]; d != nil {
			song.Format = d.Format
		}
		out = append(out, song)
	}
	slices.SortStableFunc(out, func(x, y OrganizeSong) int { return b.rank[x.ID] - b.rank[y.ID] })
	return out
}

// albumShape is what an album's discs are: how many, and each one's songs and
// name, by disc number (0 for songs with none).
type albumShape struct {
	discs int
	songs map[int]int
	names map[int]string
}

// albumShapes returns each album's shape, by album row, from tracks in album
// order, counting discs as the index does.
func albumShapes(tracks []sqlcdb.ListIndexTracksRow) map[int64]albumShape {
	out := map[int64]albumShape{}
	for i := range tracks {
		t := &tracks[i]
		sh, ok := out[t.AlbumID]
		if !ok {
			sh = albumShape{songs: map[int]int{}, names: map[int]string{}}
		}
		disc := intOr0(t.DiscNo)
		if _, seen := sh.songs[disc]; !seen {
			sh.discs++
		}
		sh.songs[disc]++
		if sh.names[disc] == "" {
			sh.names[disc] = strOrEmpty(t.DiscSubtitle)
		}
		out[t.AlbumID] = sh
	}
	return out
}

// OrganizeTrackState is a track as a run finds it before moving it.
type OrganizeTrackState struct {
	ID, Size      int64
	ULID, Title   string
	Album         string // The album's ULID.
	Root, RelPath string
	MTime         time.Time
	Live          bool // Neither missing nor trashed.
}

// OrganizeTrackStates returns the tracks ids as they are now, by row. A row
// that is gone is not in the map.
func (s *Store) OrganizeTrackStates(ctx context.Context, ids []int64) (map[int64]OrganizeTrackState, error) {
	rows, err := s.rq.OrganizeTrackStates(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("organize track states: %w", err)
	}
	out := make(map[int64]OrganizeTrackState, len(rows))
	for i := range rows {
		r := &rows[i]
		out[r.ID] = OrganizeTrackState{
			ID: r.ID, Size: r.Size, ULID: r.Ulid, Title: r.Title, Album: r.Album, Root: r.Root, RelPath: r.RelPath,
			MTime: fromMS(r.Mtime), Live: r.MissingSince == nil && r.TrashedAt == nil,
		}
	}
	return out, nil
}

// OrganizeClaimant is a song in a conflict, with what tells it apart.
type OrganizeClaimant struct {
	ID             int64
	ULID, Title    string
	Album, AlbumID string // The album's title and ULID.
	Artist         string // The song's first artist.
	Root, RelPath  string
	Format         string
	Bitrate        int // In kbps; 0 when unknown.
	Duration       time.Duration
	AddedAt        time.Time
	Digest         *[32]byte
	Plays          int           // The person's own.
	Stars          rating.Rating // The person's own; zero for none.
}

// OrganizeClaimants returns the tracks ids, with the plays and stars of the
// user, by row.
func (s *Store) OrganizeClaimants(ctx context.Context, userID int64, ids []int64) (map[int64]OrganizeClaimant, error) {
	rows, err := s.rq.OrganizeClaimants(ctx, sqlcdb.OrganizeClaimantsParams{UserID: userID, Ids: ids})
	if err != nil {
		return nil, fmt.Errorf("organize claimants: %w", err)
	}
	out := make(map[int64]OrganizeClaimant, len(rows))
	for i := range rows {
		r := &rows[i]
		c := OrganizeClaimant{
			ID: r.ID, ULID: r.Ulid, Title: r.Title, Album: r.Album, AlbumID: r.AlbumUlid, Artist: r.Artist,
			Root: r.Root, RelPath: r.RelPath, Format: r.Format, Bitrate: intOr0(r.Bitrate),
			Duration: time.Duration(r.DurationMs) * time.Millisecond, AddedAt: fromMS(r.AddedAt),
			Plays: int(r.Plays),
		}
		if r.Halves > 0 {
			c.Stars, _ = rating.FromHalves(r.Halves) // A stored rating is a valid one.
		}
		if len(r.AudioDigest) == len(c.Digest) {
			d := [32]byte(r.AudioDigest)
			c.Digest = &d
		}
		out[r.ID] = c
	}
	return out, nil
}

// The kinds of organize run.
const (
	OrganizeKindRun  = "run"
	OrganizeKindUndo = "undo"
	OrganizeKindEdit = "edit" // One song moved after a tag edit.
)

// The states of an organize run.
const (
	OrganizeRunning = "running"
	OrganizeDone    = "done"
	OrganizeStopped = "stopped"
	OrganizeUndone  = "undone"
)

// The kinds of journal row, and their states.
const (
	MoveSong      = "song"
	MoveCompanion = "companion"
	MoveFolder    = "folder"   // A folder the run emptied and removed.
	MovePlaylist  = "playlist" // A playlist file it rewrote; To is the original's copy.

	MovePlanned = "planned"
	MoveMoved   = "moved"
	MoveSkipped = "skipped"
	MoveUndone  = "undone"
)

// OrganizeRunStart is a run about to start.
type OrganizeRunStart struct {
	ULID, Kind, Template string
	By                   int64 // 0 for none.
	At                   time.Time
	Total, Conflicts     int
	UndoOf               int64 // The run an undo undoes; 0 for none.
}

// StartOrganizeRun records a run as running and returns its row.
func (s *Store) StartOrganizeRun(ctx context.Context, r *OrganizeRunStart) (int64, error) {
	var id int64
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		var err error
		id, err = q.CreateOrganizeRun(ctx, sqlcdb.CreateOrganizeRunParams{
			Ulid: r.ULID, Kind: r.Kind, ByUser: nullID(r.By), StartedAt: ms(r.At), Template: r.Template,
			Total: int64(r.Total), Conflicts: int64(r.Conflicts), UndoOf: nullID(r.UndoOf),
		})
		return err //nolint:wrapcheck // Wrapped below.
	})
	if err != nil {
		return 0, fmt.Errorf("start organize run: %w", err)
	}
	return id, nil
}

// nullID returns id, or nil for 0.
func nullID(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}

// OrganizeMove is a file a run means to move, as its journal holds it.
type OrganizeMove struct {
	ID       int64 // The journal row; 0 before it is written.
	Run      int64
	Track    int64 // The song's row; 0 for a companion.
	Root     string
	From, To string // Under Root.
	Kind     string // MoveSong or MoveCompanion.
}

// JournalOrganize writes moves to run's journal as planned, in one
// transaction, and sets each one's ID.
func (s *Store) JournalOrganize(ctx context.Context, run int64, moves []OrganizeMove) error {
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		for i := range moves {
			m := &moves[i]
			id, err := q.PlanOrganizeMove(ctx, sqlcdb.PlanOrganizeMoveParams{
				RunID: run, TrackID: nullID(m.Track), Root: m.Root, FromRel: m.From, ToRel: m.To, Kind: m.Kind,
			})
			if err != nil {
				return fmt.Errorf("%s: %w", m.From, err)
			}
			m.ID, m.Run = id, run
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("journal organize: %w", err)
	}
	return nil
}

// OrganizeLanded is how one journaled move ended.
type OrganizeLanded struct {
	Move   OrganizeMove
	Album  string // A song's album, by ULID, which a move changes.
	State  string // MoveMoved, MoveSkipped or MoveUndone.
	Reason string // Why a move was skipped.
}

// OrganizeTally is what a batch adds to its run's counts.
type OrganizeTally struct {
	Moved, Left, Failed, Companions, Playlists int
}

// errTrackMoved means a song's row was not where the run found it.
var errTrackMoved = errors.New("the track is not at the path the run moved it from")

// LandOrganize settles a batch of run's moves in one transaction: each song
// that moved gets its new path (or its old one back, for an undo), each
// journal row its state, and the run its counts. The albums of the songs that
// moved are noted changed. A song whose row is not where its file was is an
// error, and nothing is settled.
func (s *Store) LandOrganize(ctx context.Context, run int64, landed []OrganizeLanded, t OrganizeTally) error {
	var touched []string
	err := s.inLibraryTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		touched = touched[:0] // The writer may retry a transaction.
		for i := range landed {
			l := &landed[i]
			if l.Move.Kind == MoveSong && l.State != MoveSkipped {
				n, err := q.MoveTrackPath(ctx, sqlcdb.MoveTrackPathParams{
					ToRel: l.Move.To, ID: l.Move.Track, Root: l.Move.Root, FromRel: l.Move.From,
				})
				if err != nil {
					return fmt.Errorf("%s: %w", l.Move.From, err)
				}
				if n != 1 {
					return fmt.Errorf("%s: %w", l.Move.From, errTrackMoved)
				}
				touched = append(touched, l.Album)
			}
			if l.Move.ID != 0 {
				if err := q.SetOrganizeMove(ctx, sqlcdb.SetOrganizeMoveParams{State: l.State, Reason: nullStr(l.Reason), ID: l.Move.ID}); err != nil {
					return fmt.Errorf("%s: %w", l.Move.From, err)
				}
			}
		}
		return q.CountOrganizeRun(ctx, sqlcdb.CountOrganizeRunParams{
			Moved: int64(t.Moved), LeftCount: int64(t.Left), Failed: int64(t.Failed),
			Companions: int64(t.Companions), Playlists: int64(t.Playlists), ID: run,
		})
	})
	if err != nil {
		return fmt.Errorf("land organize: %w", err)
	}
	s.changed.add(touched)
	return nil
}

// SetOrganizeMoves gives journal rows a state and a reason, outside any
// library write: a companion's, or what an undo put back.
func (s *Store) SetOrganizeMoves(ctx context.Context, ids []int64, state, reason string) error {
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		for _, id := range ids {
			if err := q.SetOrganizeMove(ctx, sqlcdb.SetOrganizeMoveParams{State: state, Reason: nullStr(reason), ID: id}); err != nil {
				return fmt.Errorf("move %d: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("set organize moves: %w", err)
	}
	return nil
}

// AddOrganizeDone journals something run already did: a folder it removed
// (MoveFolder, From the folder) or a playlist file it rewrote (MovePlaylist,
// To where the original was kept).
func (s *Store) AddOrganizeDone(ctx context.Context, run int64, kind, root, from, to string) error {
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		return q.AddOrganizeMove(ctx, sqlcdb.AddOrganizeMoveParams{RunID: run, Root: root, FromRel: from, ToRel: to, Kind: kind})
	})
	if err != nil {
		return fmt.Errorf("journal organize %s: %w", kind, err)
	}
	return nil
}

// FinishOrganizeRun ends run in state at the time given, with the folders its
// songs landed in.
func (s *Store) FinishOrganizeRun(ctx context.Context, run int64, state string, folders int, at time.Time) error {
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		return q.FinishOrganizeRun(ctx, sqlcdb.FinishOrganizeRunParams{
			State: state, FinishedAt: new(ms(at)), Folders: int64(folders), ID: run,
		})
	})
	if err != nil {
		return fmt.Errorf("finish organize run: %w", err)
	}
	return nil
}

// SetOrganizeRunState sets a run's state: an undo that put everything back
// makes its run undone.
func (s *Store) SetOrganizeRunState(ctx context.Context, run int64, state string) error {
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		return q.SetOrganizeRunState(ctx, sqlcdb.SetOrganizeRunStateParams{State: state, ID: run})
	})
	if err != nil {
		return fmt.Errorf("set organize run state: %w", err)
	}
	return nil
}

// OrganizeRunInfo is a run as the runs list shows it.
type OrganizeRunInfo struct {
	ID                          int64
	ULID, Kind, State, Template string
	By                          string // Empty once their account is gone.
	StartedAt                   time.Time
	FinishedAt                  *time.Time
	Total, Moved, Folders       int
	Conflicts, Left, Failed     int
	Companions, Playlists       int
	UndoOf, UndoneBy            string // ULIDs.
	// StillThere says a song the run moved is still where it put it, so an
	// undo would put something back.
	StillThere bool
}

// OrganizeRuns returns the latest limit runs, a running one first, then
// newest first.
func (s *Store) OrganizeRuns(ctx context.Context, limit int) ([]OrganizeRunInfo, error) {
	rows, err := s.rq.ListOrganizeRuns(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("organize runs: %w", err)
	}
	out := make([]OrganizeRunInfo, len(rows))
	for i := range rows {
		out[i] = runInfo((*sqlcdb.OrganizeRunRow)(&rows[i]))
	}
	return out, nil
}

// OrganizeRunByULID returns the run with the public ID id, or an error
// wrapping [ErrNotFound].
func (s *Store) OrganizeRunByULID(ctx context.Context, id string) (OrganizeRunInfo, error) {
	row, err := s.rq.OrganizeRun(ctx, id)
	if err != nil {
		return OrganizeRunInfo{}, fmt.Errorf("organize run %s: %w", id, notFound(err))
	}
	return runInfo(&row), nil
}

// runInfo converts a run's row.
func runInfo(r *sqlcdb.OrganizeRunRow) OrganizeRunInfo {
	info := OrganizeRunInfo{
		ID: r.ID, ULID: r.Ulid, Kind: r.Kind, State: r.State, Template: r.Template, By: strOrEmpty(r.ByName),
		StartedAt: fromMS(r.StartedAt), Total: int(r.Total), Moved: int(r.Moved), Folders: int(r.Folders),
		Conflicts: int(r.Conflicts), Left: int(r.LeftCount), Failed: int(r.Failed), Companions: int(r.Companions),
		Playlists: int(r.Playlists), UndoOf: strOrEmpty(r.UndoOf), UndoneBy: r.UndoneBy, StillThere: r.Undoable,
	}
	if r.FinishedAt != nil {
		info.FinishedAt = new(fromMS(*r.FinishedAt))
	}
	return info
}

// PlannedOrganize returns every journal row still planned, oldest first: the
// moves of runs a stopped server never settled.
func (s *Store) PlannedOrganize(ctx context.Context) ([]OrganizeMove, error) {
	rows, err := s.rq.PlannedOrganizeMoves(ctx)
	if err != nil {
		return nil, fmt.Errorf("planned organize moves: %w", err)
	}
	out := make([]OrganizeMove, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = OrganizeMove{ID: r.ID, Run: r.RunID, Track: int64OrZero(r.TrackID), Root: r.Root, From: r.FromRel, To: r.ToRel, Kind: r.Kind}
	}
	return out, nil
}

// int64OrZero returns *p, or 0 for NULL.
func int64OrZero(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// StopRunningOrganizeRuns marks every run still running stopped, as of at:
// what a server stopped midway left. It returns how many there were.
func (s *Store) StopRunningOrganizeRuns(ctx context.Context, at time.Time) (int64, error) {
	var n int64
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		var err error
		n, err = q.StopRunningOrganizeRuns(ctx, new(ms(at)))
		return err //nolint:wrapcheck // Wrapped below.
	})
	if err != nil {
		return 0, fmt.Errorf("stop running organize runs: %w", err)
	}
	return n, nil
}

// RunningOrganizeRuns returns the rows of the runs still marked running.
func (s *Store) RunningOrganizeRuns(ctx context.Context) ([]int64, error) {
	ids, err := s.rq.RunningOrganizeRuns(ctx)
	if err != nil {
		return nil, fmt.Errorf("running organize runs: %w", err)
	}
	return ids, nil
}

// OrganizeMoved is a move a run made, with where its song is now.
type OrganizeMoved struct {
	OrganizeMove

	ULID, Title string // The song's; empty for a companion, or a song deleted since.
	Album       string // The song's album, by ULID.
	// Here says the song is live and still where the run put it, so an undo may
	// put it back.
	Here bool
}

// MovedOrganize returns what run moved, in the order it moved it.
func (s *Store) MovedOrganize(ctx context.Context, run int64) ([]OrganizeMoved, error) {
	rows, err := s.rq.MovedOrganizeMoves(ctx, run)
	if err != nil {
		return nil, fmt.Errorf("moved organize: %w", err)
	}
	out := make([]OrganizeMoved, len(rows))
	for i := range rows {
		r := &rows[i]
		m := OrganizeMoved{
			ID: r.ID, Run: run, Track: int64OrZero(r.TrackID), Root: r.Root, From: r.FromRel, To: r.ToRel, Kind: r.Kind,
			ULID: strOrEmpty(r.TrackUlid), Title: strOrEmpty(r.Title), Album: strOrEmpty(r.Album),
		}
		m.Here = r.TrackID != nil && r.TrashedAt == nil && r.MissingSince == nil &&
			strOrEmpty(r.TrackRoot) == r.Root && strOrEmpty(r.TrackPath) == r.ToRel
		out[i] = m
	}
	return out, nil
}

// OrganizeLeft is a song a run left where it was.
type OrganizeLeft struct {
	ULID, Title string
	Root        string
	From, To    string
	Reason      string
}

// LeftOrganize returns up to limit songs run left where they were.
func (s *Store) LeftOrganize(ctx context.Context, run int64, limit int) ([]OrganizeLeft, error) {
	rows, err := s.rq.LeftOrganizeMoves(ctx, sqlcdb.LeftOrganizeMovesParams{RunID: run, Limit: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("left organize: %w", err)
	}
	out := make([]OrganizeLeft, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = OrganizeLeft{ULID: r.TrackUlid, Title: r.Title, Root: r.Root, From: r.FromRel, To: r.ToRel, Reason: strOrEmpty(r.Reason)}
	}
	return out, nil
}

// PruneOrganizeRuns deletes the runs, with their journals, that are past the
// latest keep and started before cutoff: whichever keeps more. It returns how
// many went.
func (s *Store) PruneOrganizeRuns(ctx context.Context, keep int, cutoff time.Time) (int64, error) {
	var n int64
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		var err error
		n, err = q.PruneOrganizeRuns(ctx, sqlcdb.PruneOrganizeRunsParams{Cutoff: ms(cutoff), Keep: int64(keep)})
		return err //nolint:wrapcheck // Wrapped below.
	})
	if err != nil {
		return 0, fmt.Errorf("prune organize runs: %w", err)
	}
	return n, nil
}
