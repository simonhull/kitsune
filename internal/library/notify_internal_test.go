package library

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

// notes are what a Notifier heard.
type notes struct {
	albums [][]string
	all    int
	scans  []Progress
	passes []FingerprintProgress
	loud   []LoudnessProgress
}

// heard is a Notifier that keeps what it hears.
type heard struct {
	notes

	mu sync.Mutex
}

func (h *heard) LibraryChanged(albums []string, all bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if all {
		h.all++
		return
	}
	h.albums = append(h.albums, albums)
}

func (h *heard) ScanChanged(p Progress) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.scans = append(h.scans, p)
}

func (h *heard) FingerprintsChanged(p FingerprintProgress) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.passes = append(h.passes, p)
}

func (h *heard) LoudnessChanged(p LoudnessProgress) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.loud = append(h.loud, p)
}

// take returns what h heard and forgets it.
func (h *heard) take() notes {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := h.notes
	h.notes = notes{}
	return out
}

// changedAlbums returns every album h heard of, once each, sorted.
func (h *notes) changedAlbums() []string {
	var out []string
	for _, ids := range h.albums {
		out = append(out, ids...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// notifiedLib is a test library whose scanner tells n.
func notifiedLib(t *testing.T) (*testLib, *heard) {
	t.Helper()
	l := newTestLib(t, 1)
	n := &heard{}
	l.opts.Notify = n
	l.scanner = NewScanner(l.st, l.opts)
	return l, n
}

func TestScanTellsWhatChanged(t *testing.T) {
	t.Parallel()
	l, n := notifiedLib(t)
	root := l.roots[0]
	l.copyTo(root, "a/one.flac", sampleFLAC)
	l.scan(Quick)
	got := n.take()
	album := l.albumDetail().ID
	if ids := got.changedAlbums(); !slices.Equal(ids, []string{album}) {
		t.Errorf("albums heard = %v, want %s", ids, album)
	}
	if len(got.scans) < 2 || !got.scans[0].Running || got.scans[len(got.scans)-1].Running ||
		got.scans[len(got.scans)-1].Last == nil {
		t.Errorf("scans heard = %+v, want the claim first and the end last", got.scans)
	}
	l.scan(Quick)
	if again := n.take(); len(again.albums) != 0 || again.all != 0 {
		t.Errorf("an unchanged library told %+v", again)
	}
	remove(t, root, "a/one.flac")
	l.copyTo(root, "a/two.mp3", sampleMP3)
	l.scanDirs(root, "a")
	if targeted := n.take(); len(targeted.changedAlbums()) == 0 {
		t.Error("a targeted scan told nothing")
	}
}

func TestPassTellsItsProgress(t *testing.T) {
	t.Parallel()
	l, n := notifiedLib(t)
	l.copyTo(l.roots[0], "a/one.flac", sampleFLAC)
	l.scan(Quick)
	n.take()
	l.fingerprint()
	got := n.take().passes
	if len(got) < 2 || got[0].Total != 1 || got[len(got)-1] != (FingerprintProgress{}) {
		t.Errorf("passes heard = %+v, want the total first and nothing running last", got)
	}
}

func TestManyChangedAlbumsAreAll(t *testing.T) {
	t.Parallel()
	l, n := notifiedLib(t)
	var b store.Batch
	for i := range maxListedAlbums + 1 {
		tw := l.trackAt(l.roots[0], flac(i), 0, time.UnixMilli(1))
		tw.Album.GroupKey, tw.Album.ULID = flac(i), fmt.Sprintf("%026d", i)
		b.Tracks = append(b.Tracks, tw)
	}
	if err := l.st.WriteBatch(t.Context(), &b); err != nil {
		t.Fatal(err)
	}
	l.scanner.notifyLibrary(true)
	if got := n.take(); got.all != 1 || len(got.albums) != 0 {
		t.Errorf("heard %d lists and %d alls, want one all", len(got.albums), got.all)
	}
}
