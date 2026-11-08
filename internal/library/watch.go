package library

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	// defaultSettle is how long a folder must go without a change before it is
	// scanned, so copying an album in is one scan, not one per file.
	defaultSettle = 10 * time.Second
	// defaultMaxWait bounds the settle: a folder that never stops changing is
	// scanned this long after its first change, and again once it settles.
	defaultMaxWait = time.Minute
	// eventBuffer is how many file events wait for the watcher's loop.
	eventBuffer = 1024
)

// WatchMode says how the watcher follows a library root.
type WatchMode int

const (
	// WatchOff means the root isn't followed: it is missing or unreadable. The
	// next scan that ends looks again.
	WatchOff WatchMode = iota
	// Watching means the OS reports every change under the root.
	Watching
)

// String returns the mode's name on the wire.
func (m WatchMode) String() string {
	if m == Watching {
		return "watching"
	}
	return "off"
}

// WatchReason says why a root is not watched.
type WatchReason int

const (
	// ReasonNone is a watched root's.
	ReasonNone WatchReason = iota
	// ReasonMissing means the root is missing, unreadable, or empty while the
	// library has tracks there.
	ReasonMissing
)

// String returns the reason's name on the wire, "" for none.
func (r WatchReason) String() string {
	if r == ReasonMissing {
		return "missing"
	}
	return ""
}

// RootWatch is how the watcher follows one root.
type RootWatch struct {
	Root   string
	Mode   WatchMode
	Reason WatchReason
}

// WatchOptions configures [NewWatcher]. The zero value is the server's.
type WatchOptions struct {
	Settle  time.Duration // Zero means 10 s.
	MaxWait time.Duration // Zero means a minute.
	// OnChange is called, on the watcher's goroutine, after a root's mode
	// changes. Nil means nothing is told.
	OnChange func()

	notify func() (notifier, error) // Nil means fsnotify; tests replace it.
}

// notifier is what the watcher needs of fsnotify.
type notifier interface {
	Add(path string) error
	Remove(path string) error
	Events() <-chan fsnotify.Event
	Errors() <-chan error
	Close() error
}

// fsNotifier is an fsnotify watcher as a [notifier].
type fsNotifier struct{ w *fsnotify.Watcher }

func newFSNotifier() (notifier, error) { //nolint:ireturn // The seam tests replace: a fake stands where fsnotify does.
	w, err := fsnotify.NewBufferedWatcher(eventBuffer)
	if err != nil {
		return nil, err //nolint:wrapcheck // The watcher logs it and polls.
	}
	return fsNotifier{w: w}, nil
}

func (n fsNotifier) Add(path string) error         { return n.w.Add(path) }    //nolint:wrapcheck // The caller classifies it.
func (n fsNotifier) Remove(path string) error      { return n.w.Remove(path) } //nolint:wrapcheck // As above.
func (n fsNotifier) Events() <-chan fsnotify.Event { return n.w.Events }
func (n fsNotifier) Errors() <-chan error          { return n.w.Errors }
func (n fsNotifier) Close() error                  { return n.w.Close() } //nolint:wrapcheck // Logged as is.

// Watcher follows the library roots and asks the scanner for a targeted scan
// of the folders that changed. It only ever marks folders dirty: what a change
// means, an outage included, is the scan's to decide.
type Watcher struct {
	s     *Scanner
	opts  WatchOptions
	log   *slog.Logger
	modes atomic.Pointer[[]RootWatch]
}

// NewWatcher returns a watcher over s's roots. Run starts it; one watcher per
// scanner.
func NewWatcher(s *Scanner, opts WatchOptions) *Watcher {
	if opts.Settle <= 0 {
		opts.Settle = defaultSettle
	}
	if opts.MaxWait <= 0 {
		opts.MaxWait = defaultMaxWait
	}
	if opts.notify == nil {
		opts.notify = newFSNotifier
	}
	w := &Watcher{s: s, opts: opts, log: s.log}
	w.modes.Store(&[]RootWatch{})
	return w
}

// Modes returns how each root is followed now, in the scanner's order. A root
// added a moment ago may not be there yet.
func (w *Watcher) Modes() []RootWatch { return slices.Clone(*w.modes.Load()) }

// rootState is one root as the watcher's loop follows it.
type rootState struct {
	root     string // As configured.
	resolved string // Symlinks resolved: what events name.
	mode     WatchMode
	reason   WatchReason
	dirs     map[string]struct{} // Watched folders, absolute.
}

// watchLoop is the state of one Run, owned by its goroutine.
type watchLoop struct {
	w      *Watcher
	n      notifier
	roots  map[string]*rootState // By configured root.
	dirty  map[string]map[string]struct{}
	first  time.Time // The oldest change waiting, or zero.
	last   time.Time // The newest.
	rescan bool      // Events were lost: scan everything.
	// moved is set once a watched folder has moved in this batch. Until the
	// batch settles, new folders wait to be watched: one may be that folder
	// arriving, and a watch added before the OS has dropped its old one would
	// keep the old path.
	moved    bool
	deferred map[string]*rootState // New folders to watch once the batch settles, absolute.
	scans    sync.WaitGroup
	running  bool
	done     chan scanOutcome
	timer    *time.Timer
}

// scanOutcome is what a round of targeted scans found.
type scanOutcome struct {
	unavailable []string // Roots that looked down.
}

// Run follows the roots until ctx ends, and waits for a scan it started to
// end. It follows root changes made with [Scanner.Configure].
func (w *Watcher) Run(ctx context.Context) {
	n, err := w.opts.notify()
	if err != nil {
		w.log.ErrorContext(ctx, "can't watch the library folders; new music waits for a scan", "err", err)
		return
	}
	defer func() {
		if cerr := n.Close(); cerr != nil {
			w.log.WarnContext(ctx, "closing the folder watcher", "err", cerr)
		}
	}()
	l := &watchLoop{
		w: w, n: n, roots: map[string]*rootState{}, dirty: map[string]map[string]struct{}{},
		deferred: map[string]*rootState{}, done: make(chan scanOutcome, 1), timer: time.NewTimer(time.Hour),
	}
	l.timer.Stop()
	defer l.scans.Wait()
	l.reroot(ctx)
	l.loop(ctx)
}

// loop handles events, settles and scans until ctx ends. Only Close closes the
// notifier's channels, after the loop.
func (l *watchLoop) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			l.timer.Stop()
			return
		case ev := <-l.n.Events():
			l.event(ctx, ev)
		case err := <-l.n.Errors():
			l.failed(ctx, err)
		case <-l.w.s.rerooted:
			l.reroot(ctx)
		case <-l.w.s.scanEnded:
			l.rearm(ctx)
		case <-l.timer.C:
			l.startScan(ctx)
		case out := <-l.done:
			l.ended(ctx, out)
		}
	}
}

// ended takes in a finished round of scans: the roots it found down are off,
// and changes that came meanwhile get their settle.
func (l *watchLoop) ended(ctx context.Context, out scanOutcome) {
	l.running = false
	for _, root := range out.unavailable {
		l.off(ctx, root)
	}
	l.schedule()
}

// failed handles an error from the notifier. A lost event could be anything,
// so every root is scanned.
func (l *watchLoop) failed(ctx context.Context, err error) {
	if errors.Is(err, fsnotify.ErrEventOverflow) {
		l.w.log.WarnContext(ctx, "too many folder changes at once; scanning the library")
		l.rescan = true
		l.touch()
		return
	}
	l.w.log.WarnContext(ctx, "folder watcher", "err", err)
}

// reroot makes the watched roots the scanner's: removed roots stop being
// watched, and new ones start.
func (l *watchLoop) reroot(ctx context.Context) {
	roots := l.w.s.Roots()
	for root, rs := range l.roots {
		if !slices.Contains(roots, root) {
			l.release(rs)
			delete(l.roots, root)
			delete(l.dirty, root)
		}
	}
	for _, root := range roots {
		if _, ok := l.roots[root]; !ok {
			rs := &rootState{root: root}
			l.roots[root] = rs
			l.arm(ctx, rs)
		}
	}
	l.publish()
}

// rearm tries again to watch the roots that are off, after a scan.
func (l *watchLoop) rearm(ctx context.Context) {
	changed := false
	for _, root := range slices.Sorted(maps.Keys(l.roots)) {
		if rs := l.roots[root]; rs.mode == WatchOff {
			l.arm(ctx, rs)
			changed = changed || rs.mode != WatchOff
		}
	}
	if changed {
		l.publish()
	}
}

// arm starts following rs: it watches every folder a scan reads.
func (l *watchLoop) arm(ctx context.Context, rs *rootState) {
	resolved, err := filepath.EvalSymlinks(rs.root)
	if err == nil {
		var info fs.FileInfo
		if info, err = os.Stat(resolved); err == nil && !info.IsDir() {
			err = errRootNotDir
		}
	}
	if err != nil {
		if rs.mode != WatchOff || rs.reason != ReasonMissing {
			l.w.log.WarnContext(ctx, "library folder is missing; not watching it until a scan finds it", "root", rs.root, "err", err)
		}
		rs.mode, rs.reason = WatchOff, ReasonMissing
		return
	}
	rs.resolved, rs.dirs = resolved, map[string]struct{}{}
	l.watchTree(ctx, rs, resolved, false)
	rs.mode, rs.reason = Watching, ReasonNone
	l.w.log.InfoContext(ctx, "watching library folder", "root", rs.root, "folders", len(rs.dirs))
}

// watchTree watches dir and every folder below it a scan reads. With dirty,
// each one is marked changed too: files may have landed before its watch did.
func (l *watchLoop) watchTree(ctx context.Context, rs *rootState, dir string, dirty bool) {
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		if err != nil || !d.IsDir() {
			return nil //nolint:nilerr // An unreadable folder is the scan's to report; the walk goes on.
		}
		if path != rs.resolved && SkipsFolder(d.Name()) {
			return filepath.SkipDir
		}
		if addErr := l.n.Add(path); addErr != nil {
			l.w.log.WarnContext(ctx, "can't watch a library folder", "root", rs.root, "err", addErr)
			return nil
		}
		rs.dirs[path] = struct{}{}
		if dirty {
			l.mark(rs, path)
		}
		return nil
	})
}

// release stops watching rs's folders.
func (l *watchLoop) release(rs *rootState) {
	for dir := range rs.dirs {
		_ = l.n.Remove(dir) // A folder already gone has no watch left to remove.
	}
	rs.dirs = nil
}

// off stops following root, which looked down, until a scan ends.
func (l *watchLoop) off(ctx context.Context, root string) {
	rs, ok := l.roots[root]
	if !ok || rs.mode == WatchOff {
		return
	}
	l.release(rs)
	delete(l.dirty, root)
	rs.mode, rs.reason = WatchOff, ReasonMissing
	l.w.log.WarnContext(ctx, "library folder is unavailable; not watching it until a scan finds it", "root", root)
	l.publish()
}

// rootOf returns the watched root that path is at or under.
func (l *watchLoop) rootOf(path string) *rootState {
	var best *rootState
	for _, rs := range l.roots {
		if rs.mode != Watching {
			continue
		}
		if path == rs.resolved || strings.HasPrefix(path, rs.resolved+string(filepath.Separator)) {
			if best == nil || len(rs.resolved) > len(best.resolved) {
				best = rs
			}
		}
	}
	return best
}

// event marks the folder a change happened in dirty. New folders are watched
// and marked whole; a folder that went away marks its parent, whose read
// decides what went with it. The root itself going away turns it off.
func (l *watchLoop) event(ctx context.Context, ev fsnotify.Event) {
	rs := l.rootOf(ev.Name)
	switch {
	case rs == nil:
	case ev.Name == rs.resolved:
		if ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
			l.off(ctx, rs.root)
		}
	case l.ignores(rs, ev.Name):
	case ev.Has(fsnotify.Create):
		l.created(ctx, rs, ev.Name)
	default:
		l.changed(rs, ev)
	}
}

// ignores reports whether a scan would leave path out: it is in a skipped
// folder, or a temp file of an edit.
func (*watchLoop) ignores(rs *rootState, path string) bool {
	rel, err := filepath.Rel(rs.resolved, path)
	return err != nil || skippedPath(rel) || isWriteTemp(filepath.Base(path))
}

// created handles a new entry at path: a folder is watched, unless a move
// makes it wait, and either it or an audio file marks the parent.
func (l *watchLoop) created(ctx context.Context, rs *rootState, path string) {
	info, err := os.Lstat(path)
	switch {
	case err == nil && info.IsDir() && l.moved:
		l.deferred[path] = rs
	case err == nil && info.IsDir():
		l.watchTree(ctx, rs, path, true)
	case !IsAudio(filepath.Base(path)):
		return
	}
	l.mark(rs, filepath.Dir(path))
}

// changed handles a change to an existing entry: a watched folder that went
// away is forgotten, and it or an audio file marks the parent.
func (l *watchLoop) changed(rs *rootState, ev fsnotify.Event) {
	_, watched := rs.dirs[ev.Name]
	switch {
	case watched && (ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename)):
		l.moved = l.moved || ev.Has(fsnotify.Rename)
		l.forget(rs, ev.Name)
	case !IsAudio(filepath.Base(ev.Name)):
		return
	}
	l.mark(rs, filepath.Dir(ev.Name))
}

// forget drops dir and the folders below it from rs's watches. A moved
// folder's subfolders keep their watches under their old paths unless they are
// removed here.
func (l *watchLoop) forget(rs *rootState, dir string) {
	prefix := dir + string(filepath.Separator)
	for d := range rs.dirs {
		if d == dir || strings.HasPrefix(d, prefix) {
			_ = l.n.Remove(d) // The OS may have removed it already.
			delete(rs.dirs, d)
		}
	}
}

// watchDeferred watches the new folders that waited for the batch to settle,
// marking them changed.
func (l *watchLoop) watchDeferred(ctx context.Context) {
	for _, dir := range slices.Sorted(maps.Keys(l.deferred)) {
		rs := l.deferred[dir]
		if _, err := os.Lstat(dir); err == nil && l.roots[rs.root] == rs && rs.mode == Watching {
			l.watchTree(ctx, rs, dir, true)
		}
	}
	clear(l.deferred)
	l.moved = false
}

// mark records that the absolute folder dir under rs changed.
func (l *watchLoop) mark(rs *rootState, dir string) {
	rel, err := filepath.Rel(rs.resolved, dir)
	if err != nil {
		return
	}
	set, ok := l.dirty[rs.root]
	if !ok {
		set = map[string]struct{}{}
		l.dirty[rs.root] = set
	}
	set[rel] = struct{}{}
	l.touch()
}

// touch records a change now and sets the timer for when it has settled.
func (l *watchLoop) touch() {
	now := time.Now()
	if l.first.IsZero() {
		l.first = now
	}
	l.last = now
	l.schedule()
}

// schedule sets the timer for when the changes waiting have settled: the
// settle after the last, but no later than the longest wait after the first.
// While a round of scans runs, it waits for that round's end.
func (l *watchLoop) schedule() {
	l.timer.Stop()
	if l.running || (len(l.dirty) == 0 && !l.rescan) {
		return
	}
	at := l.last.Add(l.w.opts.Settle)
	if limit := l.first.Add(l.w.opts.MaxWait); limit.Before(at) {
		at = limit
	}
	l.timer.Reset(max(time.Until(at), 0))
}

// startScan hands what changed to a round of scans on its own goroutine, so
// events keep being read while it runs.
func (l *watchLoop) startScan(ctx context.Context) {
	if l.running {
		return
	}
	l.watchDeferred(ctx)
	batch, since, rescan := l.dirty, l.last, l.rescan
	l.dirty, l.first, l.last, l.rescan = map[string]map[string]struct{}{}, time.Time{}, time.Time{}, false
	if len(batch) == 0 && !rescan {
		return
	}
	l.running = true
	l.scans.Go(func() { l.done <- l.w.scanRound(ctx, batch, since, rescan) })
}

// scanRound runs the targeted scans of one settled batch, root by root. A
// running scan that began walking after the newest change has seen them all;
// otherwise the round waits for it and scans after. Lost events mean a scan of
// everything instead.
func (w *Watcher) scanRound(ctx context.Context, batch map[string]map[string]struct{}, since time.Time, rescan bool) scanOutcome {
	var out scanOutcome
	if rescan {
		w.s.ScanWhenFree(ctx)
		return out
	}
	for _, root := range slices.Sorted(maps.Keys(batch)) {
		dirs := slices.Sorted(maps.Keys(batch[root]))
		for ctx.Err() == nil {
			_, err := w.s.ScanDirs(ctx, root, dirs)
			if errors.Is(err, ErrRootUnavailable) {
				out.unavailable = append(out.unavailable, root)
			}
			if !errors.Is(err, ErrScanRunning) {
				break // ScanDirs has logged how it went.
			}
			if w.s.claim.waitFree(ctx) != nil || w.s.walkedSince(since) {
				break
			}
		}
	}
	return out
}

// publish stores the roots' modes and tells OnChange when they changed.
func (l *watchLoop) publish() {
	roots := l.w.s.Roots()
	modes := make([]RootWatch, 0, len(roots))
	for _, root := range roots {
		if rs, ok := l.roots[root]; ok {
			modes = append(modes, RootWatch{Root: root, Mode: rs.mode, Reason: rs.reason})
		}
	}
	if slices.Equal(modes, *l.w.modes.Load()) {
		return
	}
	l.w.modes.Store(&modes)
	if l.w.opts.OnChange != nil {
		l.w.opts.OnChange()
	}
}
