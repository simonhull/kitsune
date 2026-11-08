package library

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

const renamed = "Renamed"

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
	if err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, renameAlbum()); err != nil {
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
	if err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, renameAlbum()); !errors.Is(err, boom) {
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
	if err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, renameAlbum()); !errors.Is(err, boom) {
		t.Fatalf("EditAlbum = %v, want the write's failure", err)
	}
	half := l.albumDetail()
	if half.ID != l.album.ID || len(half.Tracks) != len(l.album.Tracks) {
		t.Fatalf("after a stop the album is %s with %d tracks, want %s with all %d", half.ID, len(half.Tracks), l.album.ID, len(l.album.Tracks))
	}
	l.scanner.write = func(ctx context.Context, path string, edit func(*audiometa.File) error) error {
		return audiometa.WriteContext(ctx, path, edit)
	}
	if err := l.scanner.EditAlbum(t.Context(), half.ID, half.ETag, renameAlbum()); err != nil {
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
