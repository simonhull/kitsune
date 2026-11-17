package library

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/simonhull/kitsune/internal/organize"
	"github.com/simonhull/kitsune/internal/store"
)

// A run moves songs one batch at a time: one album's songs, at most
// [organizeBatchSongs] of them, under the library lock, which it holds as a
// background pass does, so an edit or a scan waits out at most one batch and
// goes first. The run holds the claim from start to end, so no scan starts
// while it goes. For each batch it
//
//  1. checks each song is still where the preview found it, as it was;
//  2. journals the batch's moves as planned, in one transaction;
//  3. renames each file under [os.Root], never over anything;
//  4. sets each moved song's rel_path and its journal row in one transaction,
//     so a scan, the watcher or a stream never sees a song at neither path;
//  5. brings the companions along, journaled the same way; and
//  6. removes the folders it emptied, journaled too.
//
// A server stopped anywhere in that leaves planned rows, which
// [Scanner.RecoverOrganize] settles at the next start.

const (
	// organizeBatchSongs is the most songs one batch moves.
	organizeBatchSongs = 64
	// organizeKeep and organizeKeepFor are how many runs, and how long, the
	// journal keeps for Undo: whichever keeps more.
	organizeKeep    = 10
	organizeKeepFor = 90 * 24 * time.Hour
	// organizeEvery is how often a run tells its progress, at most.
	organizeEvery = 250 * time.Millisecond
)

var (
	// ErrRootDown means a run would touch a library root that looks down:
	// missing, unreadable, or empty while the store has songs under it.
	ErrRootDown = errors.New("a library folder looks down")
	// ErrNothingToUndo means a run has nothing an undo could put back, or is
	// running, or an undo.
	ErrNothingToUndo = errors.New("nothing to undo")
	// ErrNotRunning means a stop named a run that isn't running.
	ErrNotRunning = errors.New("the run isn't running")
)

// OrganizeProgress is a run's progress, as its events carry it.
type OrganizeProgress struct {
	Run   string // Its public ID.
	State string // store.OrganizeRunning, Done, Stopped or Undone.
	// Done counts the songs dealt with, moved or not, of Total.
	Done, Total int
	// AlbumsDone counts the batches finished, in the order the run takes them.
	AlbumsDone int
	Album      string // The album in hand, by public ID.
	Stopping   bool
}

// organizeItem is one file a batch moves: a song or a companion.
type organizeItem struct {
	track    int64 // 0 for a companion.
	title    string
	album    string // The song's album, by public ID.
	root     string
	from, to string // In the store's form, under root.
	caseOnly bool
	// size and mtime (Unix ms) are what the store says the file is: a file
	// that isn't, changed since the last scan, and its tags may say otherwise.
	size, mtime int64
	// check says whether the song, as it is now, may move; else why not.
	check  func(st *store.OrganizeTrackState) organize.Reason
	undoes int64 // For an undo: the journal row of the move it reverses.
}

// organizeBatch is one album's moves, or a part of a big album's.
type organizeBatch struct {
	album      string
	songs      []organizeItem
	companions []organizeItem
}

// organizeTask is a run under way.
type organizeTask struct {
	id               int64
	ulid, kind       string
	undoOf           int64  // The run an undo reverses.
	undoOfULID       string // Its public ID.
	batches          []organizeBatch
	total            int
	rewritePlaylists bool

	release func() // Gives the claim back, once.

	stop     atomic.Bool
	mu       sync.Mutex
	progress OrganizeProgress
	folders  map[string]struct{}          // Folders songs landed in, root and path.
	moved    map[string]map[string]string // Root → from → to, of the songs moved, for playlists.
}

// snapshot returns the task's progress.
func (t *organizeTask) snapshot() OrganizeProgress {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.progress
	p.Stopping = t.stop.Load()
	return p
}

// update changes the task's progress.
func (t *organizeTask) update(fn func(p *OrganizeProgress)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fn(&t.progress)
}

// The steps of a run its fault hook hears.
const (
	stepJournaled  = "journaled"  // A batch's moves are journaled; none made.
	stepMove       = "move"       // A file is about to move: an error here fails the move.
	stepMoved      = "moved"      // A file moved, and the batch hasn't landed.
	stepLanded     = "landed"     // The batch's songs landed; its companions haven't moved.
	stepCompanions = "companions" // The companions landed; no folder is removed yet.
	stepBatch      = "batch"      // A batch is done.
)

// fault tells the fault hook a run reached step, at rel. A hook that panics
// stops the run there, as a crash would; an error it returns is ignored.
func (s *Scanner) fault(step, rel string) {
	if s.organizeFault != nil {
		_ = s.organizeFault(step, rel) // Only a move's step may fail: see faultErr.
	}
}

// faultErr is fault for a step that may fail: the hook's error is the step's.
func (s *Scanner) faultErr(step, rel string) error {
	if s.organizeFault == nil {
		return nil
	}
	return s.organizeFault(step, rel)
}

// OrganizeRun is a run that was started: its public ID.
type OrganizeRun struct {
	ID string
}

// OrganizeProgress returns the progress of the run going now, or nil.
func (s *Scanner) OrganizeProgress() *OrganizeProgress {
	t := s.organizing.Load()
	if t == nil {
		return nil
	}
	p := t.snapshot()
	return &p
}

// StopOrganize asks the run with the public ID id to stop once its batch in
// hand has landed. It returns [ErrNotRunning] when that run isn't going.
func (s *Scanner) StopOrganize(id string) error {
	t := s.organizing.Load()
	if t == nil || t.ulid != id {
		return ErrNotRunning
	}
	t.stop.Store(true)
	s.notifyOrganize(t, true)
	return nil
}

// StartOrganize claims the scanner for a run of p's moves, those pick keeps
// (nil keeps every one), by the user by, and returns the run and the func that
// runs it. The caller must call that func once, on any goroutine, as the
// app's background work: it gives the claim back when the run ends. A canceled
// ctx ends the run after the batch in hand, as Stop does.
//
// It returns [ErrTagWritesOff] while tag writes are off; [ErrBusy] while a
// scan, an edit or another run holds the library; and [ErrRootDown] when a root
// the run would touch looks down.
func (s *Scanner) StartOrganize(ctx context.Context, p *OrganizePreview, pick func(i int) bool, by int64) (OrganizeRun, func(context.Context), error) {
	t := &organizeTask{kind: store.OrganizeKindRun, rewritePlaylists: s.Organize().RewritePlaylists, batches: runBatches(p, pick)}
	template := p.Templates.Template.String()
	return s.startTask(ctx, t, &store.OrganizeRunStart{Kind: store.OrganizeKindRun, Template: template, By: by, Conflicts: conflictsOf(p, pick)})
}

// startTask claims the scanner for t, records it and returns its run func.
func (s *Scanner) startTask(ctx context.Context, t *organizeTask, start *store.OrganizeRunStart) (OrganizeRun, func(context.Context), error) {
	release, err := s.claimRun(ctx, "organizing", t.roots())
	if err != nil {
		return OrganizeRun{}, nil, err
	}
	for i := range t.batches {
		t.total += len(t.batches[i].songs)
	}
	ulid, err := NewID(s.now())
	if err != nil {
		release()
		return OrganizeRun{}, nil, err
	}
	start.ULID, start.At, start.Total, start.UndoOf = ulid, s.now(), t.total, t.undoOf
	if t.id, err = s.st.StartOrganizeRun(ctx, start); err != nil {
		release()
		return OrganizeRun{}, nil, err //nolint:wrapcheck // The store's error already says what failed.
	}
	t.ulid, t.folders, t.moved = ulid, map[string]struct{}{}, map[string]map[string]string{}
	t.progress = OrganizeProgress{Run: ulid, State: store.OrganizeRunning, Total: t.total}
	s.organizing.Store(t)
	s.notifyOrganize(t, true)
	return OrganizeRun{ID: ulid}, func(ctx context.Context) {
		t.release = sync.OnceFunc(release)
		defer t.release()
		s.runTask(ctx, t)
	}, nil
}

// cmpErr returns err when it isn't nil, else fallback.
func cmpErr(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}

// conflictsOf counts the songs of p that pick keeps that are in conflict.
func conflictsOf(p *OrganizePreview, pick func(i int) bool) int {
	n := 0
	for i := range p.Plan.Songs {
		if p.Plan.Songs[i].Kind == organize.Conflict && (pick == nil || pick(i)) {
			n++
		}
	}
	return n
}

// runBatches cuts p's moves that pick keeps into batches, album by album in
// the preview's order, each at most organizeBatchSongs songs, with their
// companions in the batch of the song they follow.
func runBatches(p *OrganizePreview, pick func(i int) bool) []organizeBatch {
	companions := map[int64][]organize.Companion{}
	for _, c := range p.Plan.Companions {
		companions[c.Song] = append(companions[c.Song], c)
	}
	var batches []organizeBatch
	for i := range p.Songs {
		pl := &p.Plan.Songs[i]
		if !pl.Kind.Moves() || (pick != nil && !pick(i)) {
			continue
		}
		song := &p.Songs[i]
		last := len(batches) - 1
		if last < 0 || batches[last].album != song.Album || len(batches[last].songs) == organizeBatchSongs {
			batches = append(batches, organizeBatch{album: song.Album})
			last++
		}
		b := &batches[last]
		b.songs = append(b.songs, songItem(song, pl))
		for _, c := range companions[song.ID] {
			b.companions = append(b.companions, organizeItem{
				root: c.Root, from: slashPath(c.From), to: slashPath(c.To), caseOnly: foldEqual(c.From, c.To),
			})
		}
	}
	return batches
}

// songItem is the move of song that pl plans: it may go while it is live and
// as the preview found it, at the same path with the same size and mtime.
func songItem(song *store.OrganizeSong, pl *organize.Planned) organizeItem {
	from, size, mtime := song.RelPath, song.Size, song.MTime.UnixMilli()
	return organizeItem{
		track: song.ID, title: song.Title, album: song.Album, root: song.Root,
		from: from, to: slashPath(pl.To), caseOnly: pl.CaseOnly,
		check: func(st *store.OrganizeTrackState) organize.Reason {
			if !st.Live || st.Root != song.Root || st.RelPath != from || st.Size != size || st.MTime.UnixMilli() != mtime {
				return organize.Changed
			}
			return organize.NoReason
		},
	}
}

// foldEqual reports whether a and b differ only in case.
func foldEqual(a, b string) bool { return a != b && strings.EqualFold(a, b) }

// roots returns the roots t would touch, each once.
func (t *organizeTask) roots() []string {
	var out []string
	for i := range t.batches {
		for j := range t.batches[i].songs {
			if root := t.batches[i].songs[j].root; !slices.Contains(out, root) {
				out = append(out, root)
			}
		}
	}
	return out
}

// claimRun claims the scanner for a run that writes under roots, and returns
// the func that gives the claim back. The run holds the claim from start to
// end, so no scan starts meanwhile, and takes the library lock one batch at a
// time. doing names the run for the log.
//
// It returns [ErrTagWritesOff] while tag writes are off; [ErrBusy] while a
// scan, an edit or another run holds the library; and [ErrRootDown] when one
// of roots looks down.
func (s *Scanner) claimRun(ctx context.Context, doing string, roots []string) (func(), error) {
	if !s.live().TagWrites {
		return nil, ErrTagWritesOff
	}
	if !s.claim.tryLock(heldByScan) {
		return nil, ErrBusy
	}
	release := s.claim.unlock
	ok, err := s.lock.lockUnlessBusy(ctx, heldByPass) // A scan or an edit holding it now is a refusal.
	if err != nil || !ok {
		release()
		return nil, cmpErr(err, ErrBusy)
	}
	s.lock.unlock()
	for _, root := range roots {
		if rootLooksDown(root) {
			release()
			s.log.WarnContext(ctx, "not "+doing+": a library root looks down", "root", root)
			return nil, ErrRootDown
		}
	}
	return release, nil
}

// rootLooksDown reports whether root looks down by the full scan's test: it
// can't be resolved or read, or it shows nothing, which is what an unmounted
// disk leaves behind. A run only asks of a root it has songs under.
func rootLooksDown(root string) bool {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return true
	}
	return rootDown(resolved, map[string]store.FileState{"": {TrackID: 1}}) != ""
}

// runTask runs t's batches, then finishes it.
func (s *Scanner) runTask(ctx context.Context, t *organizeTask) {
	start := time.Now()
	state := store.OrganizeDone
	for i := range t.batches {
		if t.stop.Load() || ctx.Err() != nil {
			state = store.OrganizeStopped
			break
		}
		b := &t.batches[i]
		t.update(func(p *OrganizeProgress) { p.Album = b.album })
		if err := s.runBatch(ctx, t, b); err != nil {
			if ctx.Err() == nil {
				s.log.ErrorContext(ctx, "an organize run stopped", "run", t.ulid, "err", err)
			}
			state = store.OrganizeStopped
			break
		}
		t.update(func(p *OrganizeProgress) { p.AlbumsDone = i + 1 })
		s.fault(stepBatch, b.album)
		s.notifyOrganize(t, false)
		s.notifyLibrary(false)
	}
	s.finishTask(context.WithoutCancel(ctx), t, state, time.Since(start))
}

// finishTask records how t ended and tells everyone.
func (s *Scanner) finishTask(ctx context.Context, t *organizeTask, state string, took time.Duration) {
	if t.rewritePlaylists && len(t.moved) > 0 {
		s.rewritePlaylists(ctx, t)
	}
	// Stop is refused from here on: the run is no longer the one going.
	t.update(func(p *OrganizeProgress) { p.State, p.Album = state, "" })
	s.organizing.CompareAndSwap(t, nil)
	t.release() // The library is whole: a scan or the next run may go.
	if err := s.st.FinishOrganizeRun(ctx, t.id, state, len(t.folders), s.now()); err != nil {
		s.log.ErrorContext(ctx, "recording the end of an organize run failed", "run", t.ulid, "err", err)
	}
	if t.kind == store.OrganizeKindUndo && t.undoOf != 0 {
		s.settleUndone(ctx, t)
	}
	if _, err := s.st.PruneOrganizeRuns(ctx, organizeKeep, s.now().Add(-organizeKeepFor)); err != nil {
		s.log.WarnContext(ctx, "pruning old organize runs failed", "err", err)
	}
	s.notifyOrganize(t, true)
	s.notifyLibrary(true)
	p := t.snapshot()
	s.log.InfoContext(ctx, "organize run ended", "run", t.ulid, "kind", t.kind, "state", state,
		"songs", p.Done, "of", p.Total, "folders", len(t.folders), "duration", took)
}

// notifyOrganize tells the notifier t's progress, at most every organizeEvery
// unless force is set.
func (s *Scanner) notifyOrganize(t *organizeTask, force bool) {
	if s.organizeNote.dueEvery(force, organizeEvery) {
		p := t.snapshot()
		s.notify.OrganizeChanged(&p)
	}
}

// runBatch moves one batch, holding the library lock. Once its files start
// moving it finishes, whatever ctx does.
func (s *Scanner) runBatch(ctx context.Context, t *organizeTask, b *organizeBatch) error {
	if err := s.lock.lock(ctx, heldByPass); err != nil {
		return err
	}
	defer s.lock.unlock()
	if !s.live().TagWrites {
		return ErrTagWritesOff
	}
	if rootLooksDown(b.songs[0].root) {
		return fmt.Errorf("%w: %s", ErrRootDown, b.songs[0].root)
	}
	bg := context.WithoutCancel(ctx)
	ready, left, err := s.checkBatch(bg, b.songs)
	if err != nil {
		return err
	}
	moves, err := s.journalBatch(bg, t, ready, left, b.companions)
	if err != nil {
		return err
	}
	s.fault(stepJournaled, "")
	roots := openRoots{}
	defer roots.close()
	landed, emptied := s.moveSongs(bg, &roots, ready, moves[:len(ready)])
	for i := range left {
		landed = append(landed, store.OrganizeLanded{Move: moves[len(ready)+i], State: store.MoveSkipped, Reason: string(left[i].reason)})
	}
	if landErr := s.landSongs(bg, t, &roots, landed); landErr != nil {
		return landErr
	}
	s.fault(stepLanded, "")
	cLanded, cEmptied := s.moveSongs(bg, &roots, b.companions, moves[len(ready)+len(left):])
	if landErr := s.landCompanions(bg, t, cLanded); landErr != nil {
		return landErr
	}
	s.fault(stepCompanions, "")
	s.removeEmptied(bg, t, &roots, append(emptied, cEmptied...))
	t.update(func(p *OrganizeProgress) { p.Done += len(b.songs) })
	return nil
}

// journalBatch journals a batch's moves as planned, in one transaction: the
// songs that may move, the songs left, then the companions, in that order.
func (s *Scanner) journalBatch(ctx context.Context, t *organizeTask, ready []organizeItem, left []leftItem, companions []organizeItem) ([]store.OrganizeMove, error) {
	moves := make([]store.OrganizeMove, 0, len(ready)+len(left)+len(companions))
	for i := range ready {
		moves = append(moves, ready[i].journal(store.MoveSong))
	}
	for i := range left {
		moves = append(moves, left[i].item.journal(store.MoveSong))
	}
	for i := range companions {
		moves = append(moves, companions[i].journal(store.MoveCompanion))
	}
	if err := s.st.JournalOrganize(ctx, t.id, moves); err != nil {
		return nil, err //nolint:wrapcheck // The store's error already says what failed.
	}
	return moves, nil
}

// journal is the item's journal row.
func (it *organizeItem) journal(kind string) store.OrganizeMove {
	return store.OrganizeMove{Track: it.track, Root: it.root, From: it.from, To: it.to, Kind: kind}
}

// leftItem is a song a batch leaves where it is, and why.
type leftItem struct {
	item   organizeItem
	reason organize.Reason
}

// checkBatch reads songs as they are now and returns those that may move and
// those left where they are, with why.
func (s *Scanner) checkBatch(ctx context.Context, songs []organizeItem) ([]organizeItem, []leftItem, error) {
	ids := make([]int64, len(songs))
	for i := range songs {
		ids[i] = songs[i].track
	}
	states, err := s.st.OrganizeTrackStates(ctx, ids)
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // The store's error already says what failed.
	}
	var ready []organizeItem
	var left []leftItem
	for i := range songs {
		it := &songs[i]
		st, ok := states[it.track]
		reason := organize.Changed
		if ok {
			reason = it.check(&st)
		}
		if reason == organize.NoReason {
			taken, takenErr := s.st.PathTaken(ctx, it.root, it.to)
			if takenErr != nil {
				return nil, nil, takenErr //nolint:wrapcheck // As above.
			}
			if taken && !it.caseOnly {
				reason = it.takenReason()
			}
		}
		if reason != organize.NoReason {
			left = append(left, leftItem{item: *it, reason: reason})
			continue
		}
		next := *it
		next.size, next.mtime = st.Size, st.MTime.UnixMilli()
		ready = append(ready, next)
	}
	return ready, left, nil
}

// emptiedDir is a folder a move left, which may now be empty.
type emptiedDir struct{ root, dir string }

// moveSongs moves each of items, songs or companions, whose journal rows are
// moves, and returns how each landed and the folders they left.
func (s *Scanner) moveSongs(ctx context.Context, roots *openRoots, items []organizeItem, moves []store.OrganizeMove) ([]store.OrganizeLanded, []emptiedDir) {
	landed := make([]store.OrganizeLanded, 0, len(items))
	var left []emptiedDir
	for i := range items {
		it := &items[i]
		var made []string
		err := crossed(s.faultErr(stepMove, it.from))
		if err == nil {
			err = roots.unchanged(it)
		}
		if err == nil {
			made, err = roots.organizeMove(it.root, it.from, it.to, it.caseOnly)
		}
		l := store.OrganizeLanded{Move: moves[i], Album: it.album, State: store.MoveMoved}
		if err != nil {
			reason := it.failedReason(err)
			s.log.WarnContext(ctx, "an organize move failed; the file stays where it is",
				"track", it.track, "reason", string(reason), "err", StripRoot(err.Error(), it.root))
			l.State, l.Reason = store.MoveSkipped, string(reason)
			landed = append(landed, l)
			left = append(left, madeDirs(it.root, made)...)
			continue
		}
		landed = append(landed, l)
		s.fault(stepMoved, it.from)
		left = append(left, emptiedDir{it.root, filepath.Dir(it.from)})
		if it.undoes != 0 {
			landed = append(landed, store.OrganizeLanded{Move: store.OrganizeMove{ID: it.undoes}, State: store.MoveUndone})
		}
	}
	return landed, left
}

// takenReason is why an item stays when a song's row is at its target.
func (it *organizeItem) takenReason() organize.Reason {
	if it.undoes != 0 {
		return organize.Taken
	}
	return organize.Claimed
}

// failedReason is why an item stays after its move failed with err.
func (it *organizeItem) failedReason(err error) organize.Reason {
	r := reasonOf(err)
	if r == organize.UnknownFile && it.undoes != 0 {
		return organize.Taken
	}
	return r
}

// madeDirs are the folders a failed move made, deepest first, to take away.
func madeDirs(root string, made []string) []emptiedDir {
	out := make([]emptiedDir, 0, len(made))
	for _, d := range slices.Backward(made) {
		out = append(out, emptiedDir{root, d})
	}
	return out
}

// landSongs settles a batch's songs in the store. When that fails it moves
// every file it moved back, so the store and the disk agree again, and returns
// the error: the run stops. What can't be moved back stays journaled as
// planned, for recovery.
func (s *Scanner) landSongs(ctx context.Context, t *organizeTask, roots *openRoots, landed []store.OrganizeLanded) error {
	err := s.st.LandOrganize(ctx, t.id, landed, tallySongs(landed))
	if err == nil {
		s.noteMoved(t, landed)
		return nil
	}
	for i := range landed {
		l := &landed[i]
		if l.Move.Kind == store.MoveSong && l.State == store.MoveMoved {
			if _, back := roots.organizeMove(l.Move.Root, l.Move.To, l.Move.From, foldEqual(l.Move.To, l.Move.From)); back != nil {
				s.log.ErrorContext(ctx, "moving a file back after a failed organize write failed; recovery will settle it",
					"track", l.Move.Track, "err", StripRoot(back.Error(), l.Move.Root))
			}
		}
	}
	return err //nolint:wrapcheck // The store's error already says what failed.
}

// tallySongs counts how a batch's songs landed.
func tallySongs(landed []store.OrganizeLanded) store.OrganizeTally {
	var tally store.OrganizeTally
	for i := range landed {
		l := &landed[i]
		switch {
		case l.Move.Kind != store.MoveSong:
		case l.State == store.MoveMoved:
			tally.Moved++
		case organize.Reason(l.Reason) == organize.Failed:
			tally.Failed++
		default:
			tally.Left++
		}
	}
	return tally
}

// noteMoved remembers where t's moved songs landed.
func (*Scanner) noteMoved(t *organizeTask, landed []store.OrganizeLanded) {
	for i := range landed {
		l := &landed[i]
		if l.Move.Kind != store.MoveSong || l.State != store.MoveMoved {
			continue
		}
		t.folders[l.Move.Root+"\x00"+filepath.Dir(l.Move.To)] = struct{}{}
		if t.moved[l.Move.Root] == nil {
			t.moved[l.Move.Root] = map[string]string{}
		}
		t.moved[l.Move.Root][filepath.ToSlash(l.Move.From)] = filepath.ToSlash(l.Move.To)
	}
}

// landCompanions settles companions in the journal and counts them.
func (s *Scanner) landCompanions(ctx context.Context, t *organizeTask, landed []store.OrganizeLanded) error {
	if len(landed) == 0 {
		return nil
	}
	var tally store.OrganizeTally
	for i := range landed {
		if landed[i].Move.Kind == store.MoveCompanion && landed[i].State == store.MoveMoved {
			tally.Companions++
		}
	}
	return s.st.LandOrganize(ctx, t.id, landed, tally) //nolint:wrapcheck // The store's error already says what failed.
}

// removeEmptied removes the folders the batch emptied, each once, deepest
// first, and journals each.
func (s *Scanner) removeEmptied(ctx context.Context, t *organizeTask, roots *openRoots, dirs []emptiedDir) {
	slices.SortFunc(dirs, func(a, b emptiedDir) int {
		return strings.Count(b.dir, string(filepath.Separator)) - strings.Count(a.dir, string(filepath.Separator))
	})
	seen := map[emptiedDir]bool{}
	for _, d := range dirs {
		if seen[d] {
			continue
		}
		seen[d] = true
		for _, gone := range roots.removeEmpty(d.root, d.dir) {
			if err := s.st.AddOrganizeDone(ctx, t.id, store.MoveFolder, d.root, gone, ""); err != nil {
				s.log.WarnContext(ctx, "journaling a removed folder failed", "err", err)
			}
		}
	}
}
