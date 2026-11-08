package library

import (
	"sync"
	"time"
)

const (
	// notifyEvery is how often a running scan or pass tells its [Notifier]
	// what it has done so far. The start and the end are told at once.
	notifyEvery = time.Second
	// maxListedAlbums is the most albums one change lists; past it, a change
	// says everything changed, as a full scan's does.
	maxListedAlbums = 500
)

// Notifier hears what the scanner changes, as it changes it. The scanner calls
// it as it works, from its own goroutines, so it must be safe for concurrent
// use and return at once.
type Notifier interface {
	// LibraryChanged says the albums with these public IDs were added, changed,
	// or left the index. all says too many did to list: read it all again.
	LibraryChanged(albums []string, all bool)
	// ScanChanged gives a scan's progress: at its claim, about every second
	// while it runs, and at its end.
	ScanChanged(p Progress)
	// FingerprintsChanged gives the fingerprint pass's progress, the same way.
	FingerprintsChanged(p FingerprintProgress)
}

// noNotify hears nothing.
type noNotify struct{}

func (noNotify) LibraryChanged([]string, bool)           {}
func (noNotify) ScanChanged(Progress)                    {}
func (noNotify) FingerprintsChanged(FingerprintProgress) {}

// throttle says when a running thing may tell again.
type throttle struct {
	mu   sync.Mutex
	last time.Time
}

// due reports whether notifyEvery has passed since the last time it said yes,
// or force is set, and if so starts the wait again.
func (t *throttle) due(force bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if !force && now.Sub(t.last) < notifyEvery {
		return false
	}
	t.last = now
	return true
}

// notifyLibrary tells the notifier which albums the store's writes touched
// since it last did, unless it told less than a second ago and force is unset.
func (s *Scanner) notifyLibrary(force bool) {
	if !s.libraryNote.due(force) {
		return
	}
	ids := s.st.TakeChangedAlbums()
	switch {
	case len(ids) == 0:
	case len(ids) > maxListedAlbums:
		s.notify.LibraryChanged(nil, true)
	default:
		s.notify.LibraryChanged(ids, false)
	}
}

// notifyScan tells the notifier the scan's progress, at most once a second
// unless force is set.
func (s *Scanner) notifyScan(force bool) {
	if s.scanNote.due(force) {
		s.notify.ScanChanged(s.Progress())
	}
}

// notifyPass tells the notifier the pass's progress, at most once a second
// unless force is set.
func (s *Scanner) notifyPass(force bool) {
	if s.passNote.due(force) {
		s.notify.FingerprintsChanged(s.Fingerprints())
	}
}
