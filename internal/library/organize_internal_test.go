package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/organize"
	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store"
)

// The organizer's songs: an album out of line (no " - ", a stray name), one in
// place, and one that has no album folder yet.
const (
	fallingUp = "Falling Up"
	fangs     = "Fangs!"
	crowder   = "David Crowder Band"
	church    = "Church Music"

	fangsDir   = "Falling Up/2009 - Fangs!"
	arrowsRel  = "Falling Up/2009 - Fangs!/05 - Golden Arrows.flac"
	phosOld    = "David Crowder Band/2009 - Church Music/01 Phos Hilaron.flac"
	phosNew    = "David Crowder Band/2009 - Church Music/01 - Phos Hilaron.flac"
	nearOld    = "David Crowder Band/2009 - Church Music/03 The Nearness.flac"
	nearNew    = "David Crowder Band/2009 - Church Music/03 - The Nearness.flac"
	narniaOld  = "Harry Gregson-Williams/13 Only the Beginning.flac"
	narniaNew  = "Harry Gregson-Williams/2005 - Narnia/13 - Only the Beginning.flac"
	narniaDir  = "Harry Gregson-Williams/2005 - Narnia"
	desktopINI = "desktop.ini"
)

// tagged is a song file to write: where, and its tags.
type tagged struct {
	rel, artist, album, title string
	year, track               int
}

// organizeLib is a library with tag writes on, the owner's shapes in it, scanned.
func organizeLib(t *testing.T) *testLib {
	t.Helper()
	l := newTestLib(t, 1)
	l.opts.TagWrites = true
	l.scanner = NewScanner(l.st, l.opts)
	l.songs(
		tagged{arrowsRel, fallingUp, fangs, "Golden Arrows", 2009, 5},
		tagged{phosOld, crowder, church, "Phos Hilaron", 2009, 1},
		tagged{nearOld, crowder, church, "The Nearness", 2009, 3},
		tagged{narniaOld, "Harry Gregson-Williams", "Narnia", "Only the Beginning", 2005, 13},
	)
	l.scan(Quick)
	return l
}

// songs writes each song: a FLAC at its place, tagged.
func (l *testLib) songs(list ...tagged) {
	l.t.Helper()
	root := l.roots[0]
	for _, s := range list {
		l.copyTo(root, s.rel, sampleFLAC)
		err := audiometa.Write(filepath.Join(root, s.rel), func(f *audiometa.File) error {
			f.Tags.Album, f.Tags.AlbumArtist, f.Tags.Artists = s.album, s.artist, []string{s.artist}
			f.Tags.Title, f.Tags.Year, f.Tags.TrackNumber = s.title, s.year, s.track
			return nil
		})
		if err != nil {
			l.t.Fatalf("Write %s: %v", s.rel, err)
		}
	}
}

// preview previews scope with the default templates and companions.
func (l *testLib) preview(scope OrganizeScope) *OrganizePreview {
	l.t.Helper()
	c := DefaultOrganize()
	p, err := l.scanner.PreviewOrganize(l.t.Context(), &c, scope)
	if err != nil {
		l.t.Fatalf("PreviewOrganize: %v", err)
	}
	return p
}

// run runs every move of p to its end and returns the run's public ID.
func (l *testLib) run(p *OrganizePreview) string {
	l.t.Helper()
	run, fn, err := l.scanner.StartOrganize(l.t.Context(), p, nil, 0)
	if err != nil {
		l.t.Fatalf("StartOrganize: %v", err)
	}
	fn(l.t.Context())
	return run.ID
}

// runInfo returns the run with the public ID id.
func (l *testLib) runInfo(id string) store.OrganizeRunInfo {
	l.t.Helper()
	info, err := l.st.OrganizeRunByULID(l.t.Context(), id)
	if err != nil {
		l.t.Fatal(err)
	}
	return info
}

// files lists every file and folder under root, slash-separated, sorted.
func files(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != root {
			rel, relErr := filepath.Rel(root, p)
			if relErr != nil {
				return fmt.Errorf("rel: %w", relErr)
			}
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

// ids returns every track's row by its path under the only root, live or not.
func (l *testLib) ids() map[string]int64 {
	l.t.Helper()
	out := map[string]int64{}
	for rel, st := range l.states(l.roots[0]) {
		if st.TrackID != 0 {
			out[rel] = st.TrackID
		}
	}
	return out
}

// sameSongs fails unless before and after hold the same tracks, under the
// paths moved says (old → new), and none other.
func sameSongs(t *testing.T, before, after map[string]int64, moved map[string]string) {
	t.Helper()
	want := map[string]int64{}
	for rel, id := range before {
		if to, ok := moved[rel]; ok {
			rel = to
		}
		want[rel] = id
	}
	if !maps.Equal(after, want) {
		t.Errorf("tracks after = %v, want %v", after, want)
	}
}

// wantFiles fails unless every one of present is under root and none of absent.
func wantFiles(t *testing.T, root string, present, absent []string) {
	t.Helper()
	got := files(t, root)
	for _, f := range present {
		if !slices.Contains(got, f) {
			t.Errorf("files = %v, want %s among them", got, f)
		}
	}
	for _, f := range absent {
		if slices.Contains(got, f) {
			t.Errorf("files = %v, want %s gone", got, f)
		}
	}
}

// wantContent fails unless each file under root holds what want says.
func wantContent(t *testing.T, root string, want map[string]string) {
	t.Helper()
	for rel, content := range want {
		if b, err := os.ReadFile(filepath.Join(root, rel)); err != nil || string(b) != content {
			t.Errorf("%s = %q, %v; want %q", rel, b, err, content)
		}
	}
}

// wantNoChange fails unless quick and full scans, and a targeted scan of dirs,
// find nothing added, missing or moved.
func (l *testLib) wantNoChange(dirs ...string) {
	l.t.Helper()
	for _, mode := range []Mode{Quick, Full} {
		if stats := l.scan(mode); stats.Added+stats.Missing+stats.Moved != 0 {
			l.t.Errorf("%s scan = %+v, want no change", mode, stats)
		}
	}
	if len(dirs) == 0 {
		return
	}
	if stats := l.scanDirs(l.roots[0], dirs...); stats.Added+stats.Missing != 0 {
		l.t.Errorf("targeted scan = %+v, want no change", stats)
	}
}

// leftReasons returns why the run with row id left each song, by key.
func (l *testLib) leftReasons(id int64, key func(*store.OrganizeLeft) string) map[string]string {
	l.t.Helper()
	left, err := l.st.LeftOrganize(l.t.Context(), id, 100)
	if err != nil {
		l.t.Fatal(err)
	}
	out := make(map[string]string, len(left))
	for i := range left {
		out[key(&left[i])] = left[i].Reason
	}
	return out
}

// byFrom keys a song left by where it was.
func byFrom(s *store.OrganizeLeft) string { return s.From }

// byTo keys a song left by where it would have gone.
func byTo(s *store.OrganizeLeft) string { return s.To }

// A run moves the songs out of line, keeping every track's row, brings the
// cover along, removes only the folders it emptied, and a scan after it, full
// or of the folders it touched, changes nothing.
func TestOrganizeRunMovesSongsAndCompanions(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	root := l.roots[0]
	l.write(t, root, "Harry Gregson-Williams/"+coverJPG, []byte("jpeg"))
	l.write(t, root, "David Crowder Band/2009 - Church Music/"+desktopINI, []byte("[.ShellClassInfo]"))
	before := l.ids()
	p := l.preview(OrganizeScope{})
	if c := p.Plan.Counts; c != (organize.Counts{Songs: 4, InPlace: 1, Renames: 2, Moves: 1}) {
		t.Fatalf("counts = %+v", c)
	}
	id := l.run(p)
	moved := map[string]string{phosOld: phosNew, nearOld: nearNew, narniaOld: narniaNew}
	sameSongs(t, before, l.ids(), moved)
	wantFiles(t, root,
		[]string{arrowsRel, phosNew, nearNew, narniaNew, narniaDir + "/" + coverJPG, "David Crowder Band/2009 - Church Music/" + desktopINI},
		[]string{phosOld, narniaOld, "Harry Gregson-Williams/" + coverJPG})
	if info := l.runInfo(id); info.State != store.OrganizeDone || info.Moved != 3 || info.Companions != 1 || info.Folders != 2 || !info.StillThere {
		t.Errorf("run = %+v, want done with 3 moved into 2 folders and a cover", info)
	}
	l.wantNoChange(".", "Harry Gregson-Williams", narniaDir, "David Crowder Band/2009 - Church Music")
	sameSongs(t, before, l.ids(), moved)
}

// A folder the run empties goes, and so does its parent once empty; one that
// keeps a file Kitsune doesn't move stays.
func TestOrganizeRemovesOnlyFoldersItEmptied(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	l.opts.TagWrites = true
	l.scanner = NewScanner(l.st, l.opts)
	root := l.roots[0]
	l.songs(
		tagged{"old/inbox/one.flac", fallingUp, fangs, "Ascend", 2009, 1},
		tagged{"kept/two.flac", fallingUp, "Crashings", "Descend", 2004, 2},
	)
	l.write(t, root, "kept/"+desktopINI, []byte("x"))
	l.write(t, root, "empty already/.keep", nil)
	l.scan(Quick)
	l.run(l.preview(OrganizeScope{}))
	got := files(t, root)
	if slices.Contains(got, "old") || slices.Contains(got, "old/inbox") {
		t.Errorf("files = %v, want old/ and old/inbox gone", got)
	}
	if !slices.Contains(got, "kept/"+desktopINI) || !slices.Contains(got, "empty already/.keep") {
		t.Errorf("files = %v, want kept/ and the folder the run never touched still there", got)
	}
}

// A change of case alone is made in two steps, and lands.
func TestOrganizeCaseOnlyRename(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	l.opts.TagWrites = true
	l.scanner = NewScanner(l.st, l.opts)
	root := l.roots[0]
	const old, want = "Sleep Token/2025 - Even In Arcadia/02 - Emergence.flac", "Sleep Token/2025 - Even in Arcadia/02 - Emergence.flac"
	l.songs(tagged{old, "Sleep Token", "Even in Arcadia", "Emergence", 2025, 2})
	l.scan(Quick)
	before := l.ids()
	p := l.preview(OrganizeScope{})
	if pl := p.Plan.Songs[0]; !pl.CaseOnly || pl.Kind != organize.Move {
		t.Fatalf("plan = %+v, want a case-only move", pl)
	}
	l.run(p)
	sameSongs(t, before, l.ids(), map[string]string{old: want})
	if got := files(t, root); !slices.Contains(got, want) || slices.Contains(got, old) || slices.ContainsFunc(got, func(f string) bool { return strings.Contains(f, "kitsune-organize") }) {
		t.Errorf("files = %v, want the new case only, and no temp", got)
	}
}

// Conflicts stay where they are: two songs that want one path, and a file
// Kitsune doesn't play at the path, seen by the preview or only by the run.
func TestOrganizeConflictsStay(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	l.opts.TagWrites = true
	l.scanner = NewScanner(l.st, l.opts)
	root := l.roots[0]
	const plainRel = "Celldweller/2012 - Wish Upon a Blackstar/12 - The Best.flac"
	const deluxeRel = "Celldweller/2012 - Wish Upon a Blackstar (Deluxe)/12 - The Best.flac"
	const takenRel = "Other/x.flac"
	const lateRel = "Late/y.flac"
	l.songs(
		tagged{plainRel, "Celldweller", "Wish Upon a Blackstar", "The Best", 2012, 12},
		tagged{deluxeRel, "Celldweller", "Wish Upon a Blackstar", "The Best", 2012, 12},
		tagged{takenRel, "Other", "Taken Here", "Taken", 2001, 1},
		tagged{lateRel, "Late", "Late Here", "Late", 2002, 1},
	)
	l.write(t, root, "Other/2001 - Taken Here/01 - taken.FLAC", []byte("not ours"))
	l.scan(Quick)
	before := l.ids()
	p := l.preview(OrganizeScope{})
	if c := p.Plan.Counts; c.Conflicts != 2 || c.Moves != 1 || c.InPlace != 1 {
		t.Fatalf("counts = %+v, want the deluxe twin and the taken path in conflict", c)
	}
	l.write(t, root, "Late/2002 - Late Here/01 - Late.flac", []byte("arrived after the preview"))
	id := l.run(p)
	sameSongs(t, before, l.ids(), nil)
	wantContent(t, root, map[string]string{"Other/2001 - Taken Here/01 - taken.FLAC": "not ours", "Late/2002 - Late Here/01 - Late.flac": "arrived after the preview"})
	info := l.runInfo(id)
	if info.Moved != 0 || info.Left != 1 || info.Conflicts != 2 {
		t.Errorf("run = %+v, want nothing moved, the late file's song left and 2 conflicts", info)
	}
	if got, want := l.leftReasons(info.ID, byFrom), map[string]string{lateRel: string(organize.UnknownFile)}; !maps.Equal(got, want) {
		t.Errorf("left = %v, want %v", got, want)
	}
}

// A song that changed since the preview, one whose file is gone and one that
// would cross to another filesystem stay where they are, each named.
func TestOrganizeLeavesWhatChangedMissingOrCrosses(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	root := l.roots[0]
	p := l.preview(OrganizeScope{})
	bump(t, filepath.Join(root, phosOld), 1)
	remove(t, root, nearOld)
	l.scanner.organizeFault = func(step, rel string) error {
		if step == stepMove && rel == narniaOld {
			return &os.LinkError{Op: "link", Old: rel, New: narniaNew, Err: syscall.EXDEV}
		}
		return nil
	}
	id := l.run(p)
	info := l.runInfo(id)
	if info.Moved != 0 || info.Left != 3 {
		t.Errorf("run = %+v, want none moved and 3 left", info)
	}
	reasons := l.leftReasons(info.ID, byFrom)
	want := map[string]string{phosOld: string(organize.Changed), nearOld: string(organize.Missing), narniaOld: string(organize.OtherFilesystem)}
	if !maps.Equal(reasons, want) {
		t.Errorf("left = %v, want %v", reasons, want)
	}
	if !exists(t, root, narniaOld) || exists(t, root, narniaDir) {
		t.Errorf("the song that would cross stays, and no folder is made for it")
	}
}

// Stop finishes the album in hand and stops: the next album stays.
func TestOrganizeStopBetweenAlbums(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	p := l.preview(OrganizeScope{})
	run, fn, err := l.scanner.StartOrganize(t.Context(), p, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	l.scanner.organizeFault = func(step, _ string) error {
		if step == stepBatch {
			if stopErr := l.scanner.StopOrganize(run.ID); stopErr != nil {
				t.Error(stopErr)
			}
		}
		return nil
	}
	fn(t.Context())
	info := l.runInfo(run.ID)
	if info.State != store.OrganizeStopped || info.Moved != 2 {
		t.Errorf("run = %+v, want stopped after Church Music's two songs", info)
	}
	if !exists(t, l.roots[0], narniaOld) || !exists(t, l.roots[0], phosNew) {
		t.Error("want the first album moved and the second where it was")
	}
	if err = l.scanner.StopOrganize(run.ID); !errors.Is(err, ErrNotRunning) {
		t.Errorf("stop after the end = %v, want ErrNotRunning", err)
	}
}

// crash stops the run at step, the n-th time it is reached, as a server killed
// there would: nothing after it happens.
type crash struct{}

// crashAt runs p, crashing at step's n-th call, and returns the run's row.
func (l *testLib) crashAt(p *OrganizePreview, step string, n int) int64 {
	l.t.Helper()
	run, fn, err := l.scanner.StartOrganize(l.t.Context(), p, nil, 0)
	if err != nil {
		l.t.Fatal(err)
	}
	calls := 0
	l.scanner.organizeFault = func(at, _ string) error {
		if at != step {
			return nil
		}
		calls++
		if calls == n {
			panic(crash{})
		}
		return nil
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(crash); !ok {
					panic(r)
				}
			}
		}()
		fn(l.t.Context())
	}()
	return l.runInfo(run.ID).ID
}

// A run killed at each step is settled at the next start: every song's row
// names a file that is there, nothing is journaled as planned, the run is
// stopped, and a full scan finds nothing missing or new.
func TestOrganizeRecoversFromACrashAtEachStep(t *testing.T) {
	t.Parallel()
	for _, step := range []struct {
		name string
		n    int
	}{{stepJournaled, 1}, {stepMoved, 1}, {stepMoved, 2}, {stepLanded, 1}, {stepCompanions, 1}, {stepJournaled, 2}} {
		t.Run(step.name, func(t *testing.T) {
			t.Parallel()
			l := organizeLib(t)
			l.write(t, l.roots[0], "Harry Gregson-Williams/"+coverJPG, []byte("jpeg"))
			before := l.ids()
			l.crashAt(l.preview(OrganizeScope{}), step.name, step.n)
			// The next start: a new scanner over the same store and disk.
			l.scanner = NewScanner(l.st, l.opts)
			l.wantRecovered(len(before))
		})
	}
}

// wantRecovered recovers, then fails unless nothing is journaled as planned,
// there are songs tracks, each at a file that is there, scans change nothing,
// and the run is stopped.
func (l *testLib) wantRecovered(songs int) {
	l.t.Helper()
	ctx := l.t.Context()
	if err := l.scanner.RecoverOrganize(ctx); err != nil {
		l.t.Fatal(err)
	}
	if planned, err := l.st.PlannedOrganize(ctx); err != nil || len(planned) != 0 {
		l.t.Errorf("planned after recovery = %v, %v; want none", planned, err)
	}
	after := l.ids()
	if len(after) != songs {
		l.t.Errorf("tracks = %v, want the %d there were", after, songs)
	}
	for rel := range after {
		if !exists(l.t, l.roots[0], rel) {
			l.t.Errorf("a track's row names %s, which isn't there", rel)
		}
	}
	l.wantNoChange()
	if runs, err := l.st.OrganizeRuns(ctx, 1); err != nil || len(runs) != 1 || runs[0].State != store.OrganizeStopped {
		l.t.Errorf("runs = %+v, %v; want the run stopped", runs, err)
	}
}

// journalOne journals one planned move of the track at from to to, under a
// run of its own, as a run does just before it moves the file.
func (l *testLib) journalOne(from, to string) {
	l.t.Helper()
	ulid, err := NewID(l.clock.Now())
	if err != nil {
		l.t.Fatal(err)
	}
	run, err := l.st.StartOrganizeRun(l.t.Context(), &store.OrganizeRunStart{ULID: ulid, Kind: store.OrganizeKindRun, Template: "t", At: l.clock.Now()})
	if err != nil {
		l.t.Fatal(err)
	}
	m := []store.OrganizeMove{{Track: l.id(l.roots[0], from), Root: l.roots[0], From: from, To: to, Kind: store.MoveSong}}
	if err = l.st.JournalOrganize(l.t.Context(), run, m); err != nil {
		l.t.Fatal(err)
	}
}

// recoveryCase is a state a crash can leave one move in, and what recovery
// should make of it.
type recoveryCase struct {
	name       string
	from, to   string
	leave      func(t *testing.T, root string) // What the crash left on disk.
	wantAt     string
	wantGone   []string
	wantReason organize.Reason
}

// linkFile hard-links root/a at root/b, making b's folder.
func linkFile(t *testing.T, root, a, b string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, b)), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, a), filepath.Join(root, b)); err != nil {
		t.Fatal(err)
	}
}

// recoveryCases are every state a move can be left in, a change of case's two
// steps included.
func recoveryCases() []recoveryCase {
	const from, to = "a/one.flac", "b/one.flac"
	const caseFrom, caseTo = "a/One.flac", "a/one.flac"
	return []recoveryCase{
		{name: "never moved", from: from, to: to, leave: func(*testing.T, string) {}, wantAt: from, wantReason: organize.Failed},
		{name: "linked, not removed", from: from, to: to, leave: func(t *testing.T, root string) {
			t.Helper()
			linkFile(t, root, from, to)
		}, wantAt: to, wantGone: []string{from}},
		{name: "moved, not landed", from: from, to: to, leave: func(t *testing.T, root string) {
			t.Helper()
			linkFile(t, root, from, to)
			remove(t, root, from)
		}, wantAt: to},
		{name: "two files", from: from, to: to, leave: func(t *testing.T, root string) {
			t.Helper()
			if err := os.MkdirAll(filepath.Join(root, "b"), dirMode); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, to), []byte("another"), fileMode); err != nil {
				t.Fatal(err)
			}
		}, wantAt: from, wantReason: organize.Failed},
		{name: "case: first step linked", from: caseFrom, to: caseTo, leave: func(t *testing.T, root string) {
			t.Helper()
			linkFile(t, root, caseFrom, organizeTemp(caseFrom))
		}, wantAt: caseFrom, wantGone: []string{organizeTemp(caseFrom)}, wantReason: organize.Failed},
		{name: "case: halfway", from: caseFrom, to: caseTo, leave: func(t *testing.T, root string) {
			t.Helper()
			linkFile(t, root, caseFrom, organizeTemp(caseFrom))
			remove(t, root, caseFrom)
		}, wantAt: caseTo, wantGone: []string{organizeTemp(caseFrom)}},
		{name: "case: done, not landed", from: caseFrom, to: caseTo, leave: func(t *testing.T, root string) {
			t.Helper()
			if err := os.Rename(filepath.Join(root, caseFrom), filepath.Join(root, caseTo)); err != nil {
				t.Fatal(err)
			}
		}, wantAt: caseTo},
	}
}

// Recovery settles each state a move can be left in, by where the file is,
// and never removes the only copy.
func TestOrganizeRecoveryByWhereTheFileIs(t *testing.T) {
	t.Parallel()
	for _, tc := range recoveryCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := newTestLib(t, 1)
			root := l.roots[0]
			l.copyTo(root, tc.from, sampleFLAC)
			l.scan(Quick)
			id := l.id(root, tc.from)
			l.journalOne(tc.from, tc.to)
			tc.leave(t, root)
			if err := l.scanner.RecoverOrganize(t.Context()); err != nil {
				t.Fatal(err)
			}
			if got := l.id(root, tc.wantAt); got != id || !exists(t, root, tc.wantAt) {
				t.Errorf("the track is at %s with row %d, want %d there with its file", tc.wantAt, got, id)
			}
			wantFiles(t, root, nil, tc.wantGone)
			l.wantSettled(&tc)
		})
	}
}

// wantSettled fails unless the only run counts tc's move as moved, or names it
// left for tc's reason.
func (l *testLib) wantSettled(tc *recoveryCase) {
	l.t.Helper()
	runs, err := l.st.OrganizeRuns(l.t.Context(), 1)
	if err != nil || len(runs) != 1 {
		l.t.Fatalf("runs = %+v, %v", runs, err)
	}
	left := l.leftReasons(runs[0].ID, byFrom)
	switch {
	case tc.wantReason == organize.NoReason && (len(left) != 0 || runs[0].Moved != 1):
		l.t.Errorf("run = %+v, left = %v; want the move counted", runs[0], left)
	case tc.wantReason != organize.NoReason && (len(left) != 1 || left[tc.from] != string(tc.wantReason)):
		l.t.Errorf("left = %v, want one, for %s", left, tc.wantReason)
	}
}

// Undo puts every song back, with its folders, unless it moved again since or
// something is at its old path: those stay, named. A second undo has nothing
// left to do, once none is where the run put it.
func TestOrganizeUndo(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	root := l.roots[0]
	l.write(t, root, "Harry Gregson-Williams/"+coverJPG, []byte("jpeg"))
	before := l.ids()
	id := l.run(l.preview(OrganizeScope{}))
	// Since the run: one song went to the Trash, and a file took another's old path.
	if _, err := l.scanner.TrashTracks(t.Context(), 0, []string{l.ulidOf(nearNew)}); err != nil {
		t.Fatal(err)
	}
	l.write(t, root, phosOld, []byte("someone else's"))
	undo, fn, err := l.scanner.StartUndo(t.Context(), id, 0)
	if err != nil {
		t.Fatal(err)
	}
	fn(t.Context())
	after := l.ids()
	if after[narniaOld] != before[narniaOld] || after[phosNew] != before[phosOld] {
		t.Errorf("tracks = %v; want Narnia back where it was, and Phos where the run put it", after)
	}
	if !exists(t, root, "Harry Gregson-Williams/"+coverJPG) || exists(t, root, narniaDir) {
		t.Errorf("files = %v; want the cover back and the run's folder gone", files(t, root))
	}
	l.wantUndid(id, undo.ID, map[string]string{phosOld: string(organize.Taken), nearOld: string(organize.MovedSince)})
	if stats := l.scan(Full); stats.Missing+stats.Added != 0 {
		t.Errorf("scan after undo = %+v, want nothing missing or added", stats)
	}
}

// wantUndid fails unless the undo with the public ID undo put back the run id's
// songs but those left, by where they would have gone, names the run it undid
// and can't itself be undone.
func (l *testLib) wantUndid(id, undo string, left map[string]string) {
	l.t.Helper()
	info := l.runInfo(undo)
	if info.Kind != store.OrganizeKindUndo || info.Left != len(left) || info.UndoOf != id {
		l.t.Errorf("undo = %+v, want %d left", info, len(left))
	}
	if got := l.leftReasons(info.ID, byTo); !maps.Equal(got, left) {
		l.t.Errorf("left = %v, want %v", got, left)
	}
	if got := l.runInfo(id); got.UndoneBy != undo {
		l.t.Errorf("run = %+v, want it to name its undo", got)
	}
	if _, _, err := l.scanner.StartUndo(l.t.Context(), undo, 0); !errors.Is(err, ErrNothingToUndo) {
		l.t.Errorf("undo of an undo = %v, want ErrNothingToUndo", err)
	}
}

// ulidOf returns the public ID of the track at rel under the only root.
func (l *testLib) ulidOf(rel string) string {
	l.t.Helper()
	lib, err := l.st.OrganizeLibrary(l.t.Context())
	if err != nil {
		l.t.Fatal(err)
	}
	for i := range lib.Songs {
		if lib.Songs[i].RelPath == rel {
			return lib.Songs[i].ULID
		}
	}
	l.t.Fatalf("no live song at %s", rel)
	return ""
}

// An undo that puts every song back leaves its run undone.
func TestOrganizeUndoWholeLeavesTheRunUndone(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	before := l.ids()
	id := l.run(l.preview(OrganizeScope{}))
	_, fn, err := l.scanner.StartUndo(t.Context(), id, 0)
	if err != nil {
		t.Fatal(err)
	}
	fn(t.Context())
	sameSongs(t, before, l.ids(), nil)
	if got := l.runInfo(id); got.State != store.OrganizeUndone || got.StillThere {
		t.Errorf("run = %+v, want undone", got)
	}
	if _, _, err = l.scanner.StartUndo(t.Context(), id, 0); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("a second undo = %v, want ErrNothingToUndo", err)
	}
}

// Playlist entries, plays and stars hang off the track, so they follow it.
func TestOrganizeKeepsPlaylistsPlaysAndStars(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	ctx := t.Context()
	u, err := l.st.CreateUser(ctx, store.NewUser{Name: "organizer", PasswordHash: "x", Role: store.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	song := l.ulidOf(narniaOld)
	if _, err = l.st.CreatePlaylist(ctx, u.ID, strings.Repeat("P", 26), "Mix", &store.PlaylistAdd{Tracks: []string{song}}); err != nil {
		t.Fatal(err)
	}
	if err = l.st.RecordPlay(ctx, &store.NewPlay{UserID: u.ID, TrackID: song, PlayedAt: l.clock.Now(), Source: "test", ClientID: "c1"}); err != nil {
		t.Fatal(err)
	}
	if err = l.st.Rate(ctx, u.ID, song, rating.Rating(8)); err != nil {
		t.Fatal(err)
	}
	l.run(l.preview(OrganizeScope{}))
	if got := l.ulidOf(narniaNew); got != song {
		t.Fatalf("the moved song's ID = %s, want %s", got, song)
	}
	l.wantKept(u.ID, song)
}

// wantKept fails unless the user's playlist holds song, and their play of it
// is kept.
func (l *testLib) wantKept(user int64, song string) {
	l.t.Helper()
	ctx := l.t.Context()
	pl, err := l.st.Playlist(ctx, user, strings.Repeat("P", 26), nil)
	if err != nil || len(pl.Entries) != 1 || pl.Entries[0].Track.ID != song {
		l.t.Errorf("playlist = %+v, %v; want the moved song in it", pl.Entries, err)
	}
	plays, err := l.st.RecentPlays(ctx, user, 10)
	if err != nil || len(plays) != 1 || plays[0].TrackID != song {
		l.t.Errorf("plays = %+v, %v; want the play kept", plays, err)
	}
}

// With tag writes off a preview still works, and a run or an undo is refused.
func TestOrganizeTagWritesOffRefusesEverything(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	id := l.run(l.preview(OrganizeScope{}))
	l.opts.TagWrites = false
	if err := l.scanner.Configure(t.Context(), Live{Roots: l.roots, MissingGrace: testGrace}); err != nil {
		t.Fatal(err)
	}
	p := l.preview(OrganizeScope{})
	if _, _, err := l.scanner.StartOrganize(t.Context(), p, nil, 0); !errors.Is(err, ErrTagWritesOff) {
		t.Errorf("run = %v, want ErrTagWritesOff", err)
	}
	if _, _, err := l.scanner.StartUndo(t.Context(), id, 0); !errors.Is(err, ErrTagWritesOff) {
		t.Errorf("undo = %v, want ErrTagWritesOff", err)
	}
}

// A root that looks down refuses a run, before and during it.
func TestOrganizeRootDownRefusesARun(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	root := l.roots[0]
	p := l.preview(OrganizeScope{})
	hidden := root + ".away"
	if err := os.Rename(root, hidden); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.scanner.StartOrganize(t.Context(), p, nil, 0); !errors.Is(err, ErrRootDown) {
		t.Errorf("run with the root gone = %v, want ErrRootDown", err)
	}
	if err := os.Mkdir(root, dirMode); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.scanner.StartOrganize(t.Context(), p, nil, 0); !errors.Is(err, ErrRootDown) {
		t.Errorf("run with the root empty = %v, want ErrRootDown", err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(hidden, root); err != nil {
		t.Fatal(err)
	}
	if stats := l.scan(Full); stats.Missing != 0 {
		t.Errorf("scan = %+v, want nothing lost", stats)
	}
}

// A run claims the scanner: a scan meets it as busy, and so does another run.
func TestOrganizeHoldsTheClaim(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	p := l.preview(OrganizeScope{})
	_, fn, err := l.scanner.StartOrganize(t.Context(), p, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.scanner.Scan(t.Context(), Quick); !errors.Is(err, ErrScanRunning) {
		t.Errorf("scan during a run = %v, want ErrScanRunning", err)
	}
	if _, _, err = l.scanner.StartOrganize(t.Context(), p, nil, 0); !errors.Is(err, ErrBusy) {
		t.Errorf("a second run = %v, want ErrBusy", err)
	}
	fn(t.Context())
	if _, err = l.scanner.Scan(t.Context(), Quick); err != nil {
		t.Errorf("scan after the run = %v", err)
	}
}

// With on-edit on, an edit moves a song that sat where the template put it,
// and leaves one someone filed by hand.
func TestOrganizeAfterAnEdit(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	root := l.roots[0]
	c := DefaultOrganize()
	c.OnEdit = true
	l.scanner.SetOrganize(&c)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { l.scanner.RunOrganizeEdits(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	fangsID, crowderID := l.albumOf(arrowsRel), l.albumOf(phosOld)
	l.retitle(fangsID, "Golden Arrows (Remastered)")
	l.retitle(crowderID, "Retitled")
	eventually(t, "the in-place song is moved", func() bool {
		runs, err := l.st.OrganizeRuns(t.Context(), 1)
		return err == nil && len(runs) == 1 && runs[0].State == store.OrganizeDone
	})
	if !exists(t, root, fangsDir+"/05 - Golden Arrows (Remastered).flac") {
		t.Errorf("files = %v; want Golden Arrows under its new title", files(t, root))
	}
	if !exists(t, root, phosOld) || !exists(t, root, nearOld) {
		t.Errorf("files = %v; want the songs out of line left where they are", files(t, root))
	}
	runs, err := l.st.OrganizeRuns(t.Context(), 1)
	if err != nil || len(runs) != 1 || runs[0].Kind != store.OrganizeKindEdit || runs[0].Moved != 1 {
		t.Errorf("runs = %+v, %v; want one edit run that moved one song", runs, err)
	}
}

// albumOf returns the public ID of the album of the song at rel.
func (l *testLib) albumOf(rel string) string {
	l.t.Helper()
	lib, err := l.st.OrganizeLibrary(l.t.Context())
	if err != nil {
		l.t.Fatal(err)
	}
	for i := range lib.Songs {
		if lib.Songs[i].RelPath == rel {
			return lib.Songs[i].Album
		}
	}
	l.t.Fatalf("no song at %s", rel)
	return ""
}

// retitle sets every song of the album id to title, through an edit.
func (l *testLib) retitle(id, title string) {
	l.t.Helper()
	_, etag, err := l.st.AlbumEditFiles(l.t.Context(), id)
	if err != nil {
		l.t.Fatal(err)
	}
	if _, err = l.scanner.EditAlbum(l.t.Context(), id, etag, &AlbumEdit{All: Edit{Fields: map[Field][]string{FieldTitle: {title}}}}); err != nil {
		l.t.Fatal(err)
	}
}

// The watcher sees a run's renames and its targeted scans find every song
// where the store says it is: nothing goes missing, nothing is new.
func TestOrganizeWatcherSeesNoDeletes(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	root := l.roots[0]
	before := l.ids()
	l.realWatcher()
	l.run(l.preview(OrganizeScope{}))
	moved := map[string]string{phosOld: phosNew, nearOld: nearNew, narniaOld: narniaNew}
	time.Sleep(600 * time.Millisecond) // Past the watcher's settle, so its scan has run.
	eventually(t, "the watcher's scans are over", func() bool { return l.scanner.claim.tryLock(heldByScan) })
	l.scanner.claim.unlock()
	sameSongs(t, before, l.ids(), moved)
	if got := l.live(root); len(got) != len(before) {
		t.Errorf("live = %v, want all %d songs live", got, len(before))
	}
}

// With playlist rewriting on, a playlist file's lines that named a moved song
// name where it went, the rest byte for byte; the original is kept, and the
// undo puts the lines back and takes the copy away.
func TestOrganizeRewritesPlaylistFiles(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	root := l.roots[0]
	const list = "Playlists/mix.m3u8"
	original := "#EXTM3U\r\n#EXTINF:1,Only\r\n../" + narniaOld + "\r\n" + root + "/" + phosOld + "\r\n../" + arrowsRel + "\r\nhttp://radio/x\r\n"
	l.write(t, root, list, []byte(original))
	l.scan(Quick)
	c := DefaultOrganize()
	c.RewritePlaylists = true
	l.scanner.SetOrganize(&c)
	id := l.run(l.preview(OrganizeScope{}))
	got, err := os.ReadFile(filepath.Join(root, list))
	if err != nil {
		t.Fatal(err)
	}
	want := "#EXTM3U\r\n#EXTINF:1,Only\r\n../" + narniaNew + "\r\n" + root + "/" + phosNew + "\r\n../" + arrowsRel + "\r\nhttp://radio/x\r\n"
	if string(got) != want {
		t.Errorf("playlist =\n%q\nwant\n%q", got, want)
	}
	if kept, readErr := os.ReadFile(filepath.Join(root, list+playlistBackup)); readErr != nil || string(kept) != original {
		t.Errorf("kept copy = %q, %v; want the original", kept, readErr)
	}
	if info := l.runInfo(id); info.Playlists != 1 {
		t.Errorf("run = %+v, want one playlist rewritten", info)
	}
	_, fn, err := l.scanner.StartUndo(t.Context(), id, 0)
	if err != nil {
		t.Fatal(err)
	}
	fn(t.Context())
	if back, readErr := os.ReadFile(filepath.Join(root, list)); readErr != nil || string(back) != original {
		t.Errorf("playlist after undo = %q, %v; want the original", back, readErr)
	}
	if exists(t, root, list+playlistBackup) {
		t.Error("the kept copy outlived the undo")
	}
}

// Picked songs only: a run of some of a preview's songs moves just those.
func TestOrganizeRunsPickedSongsOnly(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	p := l.preview(OrganizeScope{})
	run, fn, err := l.scanner.StartOrganize(t.Context(), p, func(i int) bool { return p.Songs[i].RelPath == narniaOld }, 0)
	if err != nil {
		t.Fatal(err)
	}
	fn(t.Context())
	if info := l.runInfo(run.ID); info.Moved != 1 || info.Total != 1 {
		t.Errorf("run = %+v, want one song moved of one", info)
	}
	if !exists(t, l.roots[0], phosOld) || !exists(t, l.roots[0], narniaNew) {
		t.Error("want Narnia moved and Church Music left")
	}
}

// A preview of some songs plans just them, and an ID that names nothing is not
// found.
func TestOrganizePreviewScope(t *testing.T) {
	t.Parallel()
	l := organizeLib(t)
	p := l.preview(OrganizeScope{Albums: []string{l.albumOf(phosOld)}})
	if p.Plan.Counts.Songs != 2 || !p.Scoped {
		t.Errorf("scoped counts = %+v, want Church Music's two", p.Plan.Counts)
	}
	c := DefaultOrganize()
	if _, err := l.scanner.PreviewOrganize(t.Context(), &c, OrganizeScope{Tracks: []string{strings.Repeat("Z", 26)}}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown track = %v, want ErrNotFound", err)
	}
}
