package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	oneFLAC  = "one.flac"
	playSrc  = "web"
	mostRead = 10
)

// listener is a user with a play of one.flac, in a library that also holds keepMP3.
type listener struct {
	*testLib

	root   string
	userID int64
	track  string // The ULID of one.flac.
	album  string // The ULID of its album.
}

func newListener(t *testing.T) *listener {
	t.Helper()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, oneFLAC, sampleFLAC)
	l.copyTo(root, keepMP3, sampleMP3)
	retag(t, filepath.Join(root, keepMP3), otherName)
	l.scan(Quick)
	u, err := l.st.CreateUser(t.Context(), store.NewUser{Name: "simon", PasswordHash: "h", Role: store.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	a := &listener{testLib: l, root: root, userID: u.ID}
	found := a.find()
	a.album, a.track = found.ID, found.Tracks[0].ID
	a.report("first")
	return a
}

// find returns the album outside otherName.
func (a *listener) find() store.AlbumDetail {
	a.t.Helper()
	page, err := a.st.Albums(a.t.Context(), store.SortAdded, mostRead, "")
	if err != nil {
		a.t.Fatal(err)
	}
	for i := range page.Albums {
		if page.Albums[i].Artist == otherName {
			continue
		}
		d, albumErr := a.st.Album(a.t.Context(), page.Albums[i].ID)
		if albumErr != nil {
			a.t.Fatal(albumErr)
		}
		return d
	}
	a.t.Fatal("no album outside otherName")
	return store.AlbumDetail{}
}

// report records a play of the track.
func (a *listener) report(clientID string) {
	a.t.Helper()
	err := a.st.RecordPlay(a.t.Context(), &store.NewPlay{
		UserID: a.userID, TrackID: a.track, PlayedAt: a.clock.Now(), Source: playSrc, ClientID: clientID,
	})
	if err != nil {
		a.t.Fatal(err)
	}
}

// wantPlays checks how many plays the user has, all of the track.
func (a *listener) wantPlays(want int, when string) {
	a.t.Helper()
	plays, err := a.st.RecentPlays(a.t.Context(), a.userID, mostRead)
	if err != nil {
		a.t.Fatal(err)
	}
	if len(plays) != want {
		a.t.Fatalf("%s: plays = %+v, want %d", when, plays, want)
	}
	for _, p := range plays {
		if p.TrackID != a.track {
			a.t.Errorf("%s: a play moved to %s, want %s", when, p.TrackID, a.track)
		}
	}
}

func TestPlaysSurviveARetag(t *testing.T) {
	t.Parallel()
	a := newListener(t)
	path := filepath.Join(a.root, oneFLAC)
	retag(t, path, "Changed")
	bump(t, path, 1)
	a.scan(Quick)
	found := a.find()
	if found.ID == a.album || found.Tracks[0].ID != a.track {
		t.Fatalf("album %s, track %s: want a new album and the same track", found.ID, found.Tracks[0].ID)
	}
	a.wantPlays(1, "after a retag")
}

func TestPlaysSurviveAMove(t *testing.T) {
	t.Parallel()
	a := newListener(t)
	if err := os.MkdirAll(filepath.Join(a.root, "new"), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(a.root, oneFLAC), filepath.Join(a.root, "new", oneFLAC)); err != nil {
		t.Fatal(err)
	}
	if stats := a.scan(Quick); stats.Moved != 1 {
		t.Fatalf("stats = %+v, want Moved 1", stats)
	}
	a.wantPlays(1, "after a move")
}

func TestPlaysSurviveAFullScan(t *testing.T) {
	t.Parallel()
	a := newListener(t)
	if stats := a.scan(Full); stats.Updated != 2 {
		t.Fatalf("stats = %+v, want Updated 2", stats)
	}
	a.wantPlays(1, "after a Full scan")
}

func TestPlaysWaitOutTheGrace(t *testing.T) {
	t.Parallel()
	a := newListener(t)
	path := filepath.Join(a.root, oneFLAC)
	aside := filepath.Join(t.TempDir(), oneFLAC)
	if err := os.Rename(path, aside); err != nil {
		t.Fatal(err)
	}
	if stats := a.scan(Quick); stats.Missing != 1 {
		t.Fatalf("stats = %+v, want Missing 1", stats)
	}
	a.report("while missing")
	if err := os.Rename(aside, path); err != nil {
		t.Fatal(err)
	}
	a.scan(Quick)
	a.wantPlays(2, "after coming back")
}

func TestScanDeletesATrackWithPlays(t *testing.T) {
	t.Parallel()
	a := newListener(t)
	remove(t, a.root, oneFLAC)
	a.scan(Quick)
	a.clock.Advance(testGrace + time.Minute)
	if stats := a.scan(Quick); stats.Deleted != 1 {
		t.Fatalf("stats = %+v, want Deleted 1", stats)
	}
	a.wantPlays(0, "past the grace")
	a.scan(Quick) // The next one mustn't trip over anything left behind.
}

func TestRecordPlayRefusesATrackPastTheGrace(t *testing.T) {
	t.Parallel()
	a := newListener(t)
	remove(t, a.root, oneFLAC)
	a.scan(Quick)
	a.clock.Advance(testGrace + time.Minute)
	a.scan(Quick)
	err := a.st.RecordPlay(t.Context(), &store.NewPlay{UserID: a.userID, TrackID: a.track, PlayedAt: a.clock.Now(), Source: playSrc, ClientID: "late"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
