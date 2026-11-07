package library

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// hashTime is how long the slow fingerprints below take, in a bubble's time:
// a batch then holds two files, since the second ends past passBatchTime.
const hashTime = passBatchTime * 3 / 4

// undigested counts the live tracks waiting for a fingerprint.
func (l *testLib) undigested() int {
	l.t.Helper()
	n, err := l.st.CountUndigested(l.t.Context())
	if err != nil {
		l.t.Fatal(err)
	}
	return n
}

// flac names the nth file of fourFLACs.
func flac(n int) string { return fmt.Sprintf("%d.flac", n) }

// fourFLACs is a library of four FLACs, flac(0) to flac(3), scanned.
func fourFLACs(t *testing.T) *testLib {
	t.Helper()
	l := newTestLib(t, 1)
	for i := range 4 {
		l.copyTo(l.roots[0], flac(i), sampleFLAC)
	}
	l.scan(Quick)
	return l
}

// passLog records what a fingerprint hook saw, in order.
type passLog struct {
	mu     sync.Mutex
	events []string
}

func (p *passLog) add(event string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
}

func (p *passLog) all() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.events)
}

// slowly makes the scanner's fingerprints take hashTime each, logging each
// file's base name and what note says about that moment.
func (l *testLib) slowly(log *passLog, note func() string) {
	read := l.scanner.fingerprint
	l.scanner.fingerprint = func(ctx context.Context, path string) ([32]byte, error) {
		log.add(filepath.Base(path) + note())
		time.Sleep(hashTime)
		return read(ctx, path)
	}
}

func TestFirstScanReadsNoAudio(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, aFLAC, sampleFLAC)
	l.copyTo(root, keepMP3, sampleMP3)
	l.copyTo(root, "c.m4a", sampleM4A)

	if stats := l.scan(Quick); stats.Added != 3 || stats.Hashed != 0 {
		t.Errorf("stats = %+v, want Added 3 and Hashed 0", stats)
	}
	if got := l.undigested(); got != 3 {
		t.Fatalf("undigested = %d after the scan, want 3", got)
	}
	if stats := l.fingerprint(); stats != (FingerprintStats{Stored: 3}) {
		t.Errorf("pass = %+v, want Stored 3", stats)
	}
	if got := l.undigested(); got != 0 {
		t.Errorf("undigested = %d after the pass, want 0", got)
	}
	if stats := l.fingerprint(); stats != (FingerprintStats{}) {
		t.Errorf("second pass = %+v, want nothing to do", stats)
	}
}

func TestNewFilesAreHashedOnlyWhenAMoveCouldMatch(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "old/one.flac", sampleFLAC)
	l.copyTo(root, keepMP3, sampleMP3)
	l.copyTo(root, "changed.flac", sampleFLAC)
	l.scan(Quick)
	l.fingerprint()
	id := l.id(root, "old/one.flac")
	remove(t, root, "old/one.flac")
	l.copyTo(root, "new/one.flac", sampleFLAC)
	l.copyTo(root, "fresh.m4a", sampleM4A)
	bump(t, filepath.Join(root, "changed.flac"), 1)

	stats := l.scan(Quick)

	if stats.Moved != 1 || stats.Added != 1 || stats.Updated != 1 || stats.Hashed != 2 {
		t.Errorf("stats = %+v, want Moved 1, Added 1, Updated 1 and Hashed 2: the new files only", stats)
	}
	if got := l.id(root, "new/one.flac"); got != id {
		t.Errorf("the moved file has ID %d, want %d", got, id)
	}
	if got := l.undigested(); got != 1 {
		t.Errorf("undigested = %d, want 1: the changed file lost its fingerprint", got)
	}
}

// TestAMoveBeforeThePassIsNew is the known limit: with no fingerprint to
// match, a moved file is a new track.
func TestAMoveBeforeThePassIsNew(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, "old/one.flac", sampleFLAC)
	l.copyTo(root, keepMP3, sampleMP3)
	l.scan(Quick)
	id := l.id(root, "old/one.flac")
	remove(t, root, "old/one.flac")
	l.copyTo(root, "new/one.flac", sampleFLAC)

	stats := l.scan(Quick)

	if stats.Moved != 0 || stats.Added != 1 || stats.Hashed != 0 {
		t.Errorf("stats = %+v, want Added 1, Moved 0 and Hashed 0", stats)
	}
	if got := l.id(root, "new/one.flac"); got == id {
		t.Error("the file kept its ID without a fingerprint")
	}
}

func TestFullScanKeepsFingerprints(t *testing.T) {
	t.Parallel()
	l := fourFLACs(t)
	l.fingerprint()
	if stats := l.scan(Full); stats.Read != 4 || stats.Hashed != 0 {
		t.Errorf("stats = %+v, want Read 4 and Hashed 0", stats)
	}
	if got := l.undigested(); got != 0 {
		t.Errorf("undigested = %d after a full scan, want 0: the files did not change", got)
	}
}

func TestEditClearsTheFingerprintAndAsksForAPass(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	l.fingerprint()
	<-l.scanner.wantPass // The scan's.
	if err := l.editAlbum(); err != nil {
		t.Fatal(err)
	}
	if got := l.undigested(); got != 2 {
		t.Errorf("undigested = %d after the edit, want 2", got)
	}
	select {
	case <-l.scanner.wantPass:
	default:
		t.Error("the edit did not ask for a pass")
	}
	if stats := l.fingerprint(); stats.Stored != 2 {
		t.Errorf("pass = %+v, want Stored 2", stats)
	}
}

func TestPassBatchesByTime(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l := fourFLACs(t)
		var log passLog
		start := time.Now()
		l.slowly(&log, func() string {
			p := l.scanner.Fingerprints()
			return fmt.Sprintf(" %d/%d at %s", p.Done, p.Total, time.Since(start))
		})
		ends := make(chan time.Duration, 4)
		read := l.scanner.fingerprint
		l.scanner.fingerprint = func(ctx context.Context, path string) ([32]byte, error) {
			d, err := read(ctx, path)
			if !l.scanner.lock.tryLock(heldByEdit) {
				ends <- time.Since(start) // Still the pass's.
			} else {
				l.scanner.lock.unlock()
			}
			return d, err
		}

		if stats := l.fingerprint(); stats.Stored != 4 {
			t.Errorf("pass = %+v, want Stored 4", stats)
		}

		want := []string{flac(0) + " 0/4 at 0s", flac(1) + " 1/4 at 1.5s", flac(2) + " 2/4 at 3s", flac(3) + " 3/4 at 4.5s"}
		if got := log.all(); !slices.Equal(got, want) {
			t.Errorf("hashes = %q, want %q", got, want)
		}
		if got := l.scanner.Fingerprints(); got != (FingerprintProgress{}) {
			t.Errorf("progress after the pass = %+v, want zero", got)
		}
		if len(ends) != 4 {
			t.Errorf("the pass held the lock for %d of 4 files", len(ends))
		}
	})
}

func TestPassYieldsToAnEdit(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l := fourFLACs(t)
		var log passLog
		l.slowly(&log, func() string { return "" })
		passed := make(chan error, 1)
		go func() {
			_, err := l.scanner.Fingerprint(t.Context())
			passed <- err
		}()
		time.Sleep(hashTime / 2) // Inside the first batch.

		err := l.scanner.exclusive(t.Context(), func() error {
			log.add("edit")
			return nil
		})
		if err != nil {
			t.Errorf("exclusive = %v, want the edit to wait out the batch", err)
		}
		if err = <-passed; err != nil {
			t.Fatal(err)
		}
		want := []string{flac(0), flac(1), "edit", flac(2), flac(3)}
		if got := log.all(); !slices.Equal(got, want) {
			t.Errorf("order = %q, want %q", got, want)
		}
	})
}

func TestPassPausesForAScan(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l := fourFLACs(t)
		var log passLog
		l.slowly(&log, func() string {
			if l.scanner.Progress().Running {
				return " during a scan"
			}
			return ""
		})
		passed := make(chan error, 1)
		go func() {
			_, err := l.scanner.Fingerprint(t.Context())
			passed <- err
		}()
		time.Sleep(hashTime / 2)
		run, err := l.scanner.Claim(Quick)
		if err != nil {
			t.Fatal(err)
		}

		if _, err = run(t.Context()); err != nil {
			t.Fatal(err)
		}
		log.add("scan ended")

		if err = <-passed; err != nil {
			t.Fatal(err)
		}
		// The batch the claim met ends, and the next waits for the scan.
		want := []string{flac(0), flac(1) + " during a scan", "scan ended", flac(2), flac(3)}
		if got := log.all(); !slices.Equal(got, want) {
			t.Errorf("order = %q, want %q", got, want)
		}
	})
}

func TestPassLeavesAFileThatChanged(t *testing.T) {
	t.Parallel()
	l := fourFLACs(t)
	read := l.scanner.fingerprint
	l.scanner.fingerprint = func(ctx context.Context, path string) ([32]byte, error) {
		d, err := read(ctx, path)
		if filepath.Base(path) == flac(1) {
			bump(t, path, 1) // A tag edit by another program, mid-read.
		}
		return d, err
	}
	if stats := l.fingerprint(); stats != (FingerprintStats{Stored: 3, Changed: 1}) {
		t.Errorf("pass = %+v, want Stored 3 and Changed 1", stats)
	}
	if got := l.undigested(); got != 1 {
		t.Errorf("undigested = %d, want 1", got)
	}
	if stats := l.scan(Quick); stats.Updated != 1 || stats.Hashed != 0 {
		t.Errorf("scan = %+v, want the changed file read again, not hashed", stats)
	}
	if stats := l.fingerprint(); stats != (FingerprintStats{Stored: 1}) {
		t.Errorf("next pass = %+v, want Stored 1", stats)
	}
}

func TestPassSkipsAnUnreadableFile(t *testing.T) {
	t.Parallel()
	l := fourFLACs(t)
	errNoRead := errors.New("no read")
	read := l.scanner.fingerprint
	var tries int
	l.scanner.fingerprint = func(ctx context.Context, path string) ([32]byte, error) {
		if filepath.Base(path) == flac(2) {
			tries++
			return [32]byte{}, errNoRead
		}
		return read(ctx, path)
	}
	if stats := l.fingerprint(); stats != (FingerprintStats{Stored: 3, Unreadable: 1}) {
		t.Errorf("pass = %+v, want Stored 3 and Unreadable 1", stats)
	}
	if tries != 1 {
		t.Errorf("the unreadable file was tried %d times in one pass, want 1", tries)
	}
	if stats := l.fingerprint(); stats != (FingerprintStats{Unreadable: 1}) || tries != 2 {
		t.Errorf("next pass = %+v after %d tries, want it tried again", stats, tries)
	}
}

func TestRestartMidPassFinishesTheRest(t *testing.T) {
	t.Parallel()
	l := fourFLACs(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	read := l.scanner.fingerprint
	l.scanner.fingerprint = func(ctx context.Context, path string) ([32]byte, error) {
		if filepath.Base(path) == flac(2) {
			cancel() // The server stops.
			return [32]byte{}, ctx.Err()
		}
		return read(ctx, path)
	}
	if _, err := l.scanner.Fingerprint(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Fingerprint = %v, want context.Canceled", err)
	}
	if got := l.undigested(); got != 2 {
		t.Fatalf("undigested = %d after the stop, want 2", got)
	}
	l.use(l.roots...) // A new server on the same database.
	if stats := l.fingerprint(); stats != (FingerprintStats{Stored: 2}) {
		t.Errorf("pass after the restart = %+v, want Stored 2", stats)
	}
}

func TestRunFingerprintsAtStartAndAfterScans(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		l := fourFLACs(t)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			defer close(done)
			l.scanner.RunFingerprints(ctx)
		}()
		synctest.Wait()
		if got := l.undigested(); got != 0 {
			t.Fatalf("undigested = %d after start, want 0", got)
		}
		l.copyTo(l.roots[0], flac(4), sampleFLAC)
		l.scan(Quick)
		synctest.Wait()
		if got := l.undigested(); got != 0 {
			t.Errorf("undigested = %d after a scan, want 0", got)
		}
		cancel()
		<-done
	})
}
