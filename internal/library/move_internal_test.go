package library

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

const renamed = "Renamed"

// errOf drops the album an edit landed on.
func errOf(_ string, err error) error { return err }

// renameAlbum renames every track's album.
func renameAlbum() *AlbumEdit {
	return &AlbumEdit{All: Edit{Fields: map[Field][]string{FieldAlbum: {renamed}}}}
}

// failWrite makes the nth write fail with the error it returns.
func (l *editLib) failWrite(n int) error {
	boom := errors.New("disk full")
	writes := 0
	l.scanner.write = func(ctx context.Context, path string, edit func(*audiometa.File) error) error {
		writes++
		if writes == n {
			return boom
		}
		return audiometa.WriteContext(ctx, path, edit)
	}
	return boom
}

func TestRenamingAnAlbumKeepsItsRow(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	if _, err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, renameAlbum()); err != nil {
		t.Fatal(err)
	}
	after := l.albumDetail()
	if after.ID != l.album.ID || after.Title != renamed || !after.AddedAt.Equal(l.album.AddedAt) {
		t.Errorf("album = %s %q added %v, want %s renamed, added %v", after.ID, after.Title, after.AddedAt, l.album.ID, l.album.AddedAt)
	}
	if got := l.scan(Full); got.Added != 0 {
		t.Errorf("a full scan added %d tracks", got.Added)
	}
	if again := l.albumDetail(); again.ID != l.album.ID {
		t.Errorf("after a full scan the album is %s, want %s", again.ID, l.album.ID)
	}
}

func TestARenameThatWritesNothingIsUndone(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	boom := l.failWrite(1)
	if _, err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, renameAlbum()); !errors.Is(err, boom) {
		t.Fatalf("EditAlbum = %v, want the write's failure", err)
	}
	l.scan(Full)
	if after := l.albumDetail(); after.ID != l.album.ID || after.Title != l.album.Title {
		t.Errorf("album = %s %q, want %s %q as before", after.ID, after.Title, l.album.ID, l.album.Title)
	}
}

func TestARetryFinishesARename(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	boom := l.failWrite(2)
	if _, err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, renameAlbum()); !errors.Is(err, boom) {
		t.Fatalf("EditAlbum = %v, want the write's failure", err)
	}
	half := l.albumDetail()
	if half.ID != l.album.ID || len(half.Tracks) != len(l.album.Tracks) {
		t.Fatalf("after a stop the album is %s with %d tracks, want %s with all %d", half.ID, len(half.Tracks), l.album.ID, len(l.album.Tracks))
	}
	l.scanner.write = func(ctx context.Context, path string, edit func(*audiometa.File) error) error {
		return audiometa.WriteContext(ctx, path, edit)
	}
	if _, err := l.scanner.EditAlbum(t.Context(), half.ID, half.ETag, renameAlbum()); err != nil {
		t.Fatal(err)
	}
	l.scan(Full)
	after := l.albumDetail()
	if after.ID != l.album.ID || after.Title != renamed {
		t.Errorf("album = %s %q, want %s renamed", after.ID, after.Title, l.album.ID)
	}
	ids := func(d *store.AlbumDetail) []string {
		out := make([]string, len(d.Tracks))
		for i := range d.Tracks {
			out[i] = d.Tracks[i].ID
		}
		slices.Sort(out)
		return out
	}
	if !slices.Equal(ids(&after), ids(&l.album)) {
		t.Errorf("tracks %q, want %q", ids(&after), ids(&l.album))
	}
}

// marker is a person whose plays and marks a move must keep.
type marker struct {
	l      *testLib
	userID int64
}

func newMarker(l *testLib) *marker {
	l.t.Helper()
	u, err := l.st.CreateUser(l.t.Context(), store.NewUser{Name: "simon", PasswordHash: "h", Role: store.RoleUser})
	if err != nil {
		l.t.Fatal(err)
	}
	return &marker{l: l, userID: u.ID}
}

// mark plays, hearts and rates the track id.
func (m *marker) mark(id string) {
	m.l.t.Helper()
	ctx := m.l.t.Context()
	err := m.l.st.RecordPlay(ctx, &store.NewPlay{UserID: m.userID, TrackID: id, PlayedAt: m.l.clock.Now(), Source: playSrc, ClientID: id})
	if err == nil {
		err = m.l.st.Heart(ctx, m.userID, store.KindTrack, id)
	}
	if err == nil {
		err = m.l.st.Rate(ctx, m.userID, id, 4)
	}
	if err != nil {
		m.l.t.Fatal(err)
	}
}

// wantMarked checks the track id, on album, kept what [marker.mark] gave it.
func (m *marker) wantMarked(album, id string) {
	m.l.t.Helper()
	ctx := m.l.t.Context()
	plays, err := m.l.st.RecentPlays(ctx, m.userID, mostRead)
	if err != nil {
		m.l.t.Fatal(err)
	}
	if !slices.ContainsFunc(plays, func(p store.Play) bool { return p.TrackID == id }) {
		m.l.t.Errorf("plays = %+v, want one of %s", plays, id)
	}
	marks, err := m.l.st.AlbumMarks(ctx, m.userID, album)
	if err != nil {
		m.l.t.Fatal(err)
	}
	if got := marks.Tracks[id]; !got.Hearted || got.Stars != 4 {
		m.l.t.Errorf("marks of %s on %s = %+v, want hearted with 4 stars", id, album, got)
	}
}

func TestMovingATrackKeepsItsIDAndMarks(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	m := newMarker(l.testLib)
	moved := l.album.Tracks[0].ID
	m.mark(moved)
	edit := AlbumEdit{Tracks: map[string]Edit{moved: {Fields: map[Field][]string{FieldAlbum: {renamed}}}}}
	landed, err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, &edit)
	if err != nil {
		t.Fatal(err)
	}
	if landed != l.album.ID {
		t.Errorf("landed on %s, want %s: a track is still there", landed, l.album.ID)
	}
	to, err := l.st.TrackAlbum(t.Context(), moved)
	if err != nil {
		t.Fatal(err)
	}
	album, err := l.st.Album(t.Context(), to)
	if err != nil {
		t.Fatal(err)
	}
	if to == l.album.ID || album.Title != renamed || len(album.Tracks) != 1 {
		t.Errorf("the track is on %s %q with %d tracks, want a new album %q of 1", to, album.Title, len(album.Tracks), renamed)
	}
	m.wantMarked(to, moved)
	if left, _ := l.st.Album(t.Context(), l.album.ID); len(left.Tracks) != 1 {
		t.Errorf("the old album has %d tracks, want 1", len(left.Tracks))
	}
}

// pairLib is a library of two albums in one folder.
type pairLib struct {
	*editLib

	first, second store.AlbumDetail // Of a.flac and of bystander.flac.
}

func newPairLib(t *testing.T) *pairLib {
	t.Helper()
	l := &pairLib{editLib: &editLib{testLib: newTestLib(t, 1)}}
	l.root = l.roots[0]
	l.copyTo(l.root, aFLAC, sampleFLAC)
	l.copyTo(l.root, bystander, sampleFLAC)
	seed(t, filepath.Join(l.root, aFLAC), map[string][]string{keyAlbum: {"First"}})
	seed(t, filepath.Join(l.root, bystander), map[string][]string{keyAlbum: {"Second"}})
	l.opts.TagWrites = true
	l.scanner = NewScanner(l.st, l.opts)
	l.scan(Quick)
	page, err := l.st.Albums(t.Context(), store.SortName, mostRead, "")
	if err != nil || len(page.Albums) != 2 {
		t.Fatalf("albums = %+v, %v; want 2", page.Albums, err)
	}
	if l.first, err = l.st.Album(t.Context(), page.Albums[0].ID); err != nil {
		t.Fatal(err)
	}
	if l.second, err = l.st.Album(t.Context(), page.Albums[1].ID); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestMovingEveryTrackMergesTheAlbum(t *testing.T) {
	t.Parallel()
	pl := newPairLib(t)
	l, first, second := pl.editLib, pl.first, pl.second
	m := newMarker(l.testLib)
	track := second.Tracks[0].ID
	m.mark(track)
	if err := l.st.Heart(t.Context(), m.userID, store.KindAlbum, second.ID); err != nil {
		t.Fatal(err)
	}
	edit := AlbumEdit{All: Edit{Fields: map[Field][]string{FieldAlbum: {"First"}}}}
	landed, err := l.scanner.EditAlbum(t.Context(), second.ID, second.ETag, &edit)
	if err != nil {
		t.Fatal(err)
	}
	if landed != first.ID {
		t.Errorf("landed on %s, want the album merged into, %s", landed, first.ID)
	}
	if _, err = l.st.Album(t.Context(), second.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the merged album answers %v, want ErrNotFound", err)
	}
	merged, err := l.st.Album(t.Context(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Tracks) != 2 || !merged.AddedAt.Equal(first.AddedAt) {
		t.Errorf("merged album has %d tracks, added %v; want 2, added %v", len(merged.Tracks), merged.AddedAt, first.AddedAt)
	}
	m.wantMarked(first.ID, track)
	if marks, _ := l.st.AlbumMarks(t.Context(), m.userID, first.ID); !marks.Hearted {
		t.Error("the merged album's heart did not move")
	}
}
