package library

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/simonhull/kitsune/internal/store"
)

// both is an edit of one track on each album of l.
func (l *pairLib) both(all Edit) *TracksEdit {
	return &TracksEdit{
		Albums: []AlbumETag{{ID: l.first.ID, ETag: l.first.ETag}, {ID: l.second.ID, ETag: l.second.ETag}},
		All:    all,
		Tracks: map[string]Edit{l.first.Tracks[0].ID: {}, l.second.Tracks[0].ID: {}},
	}
}

func TestEditAcrossAlbumsWritesEveryTrack(t *testing.T) {
	t.Parallel()
	l := newPairLib(t)
	if err := l.scanner.EditTracks(t.Context(), l.both(*yearEdit())); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{aFLAC, bystander} {
		if got := reread(t, filepath.Join(l.root, rel)); got.Year != newYear {
			t.Errorf("%s year = %d, want %d", rel, got.Year, newYear)
		}
	}
	for _, id := range []string{l.first.ID, l.second.ID} {
		if d, err := l.st.Album(t.Context(), id); err != nil || d.Year != newYear {
			t.Errorf("album %s = year %d, %v; want %d", id, d.Year, err, newYear)
		}
	}
}

func TestEditAcrossAlbumsWithAStaleETagWritesNothing(t *testing.T) {
	t.Parallel()
	l := newPairLib(t)
	edit := l.both(*yearEdit())
	edit.Albums[1].ETag = `"stale"`
	err := l.scanner.EditTracks(t.Context(), edit)
	stale, ok := errors.AsType[*StaleAlbumError](err)
	if !ok || stale.Album != l.second.ID || !errors.Is(err, ErrStale) {
		t.Fatalf("EditTracks = %v, want a StaleAlbumError naming %s", err, l.second.ID)
	}
	l.wantUnedited()
}

func TestEditAcrossAlbumsOfATrackElsewhereIsRefused(t *testing.T) {
	t.Parallel()
	l := newPairLib(t)
	edit := l.both(*yearEdit())
	edit.Albums = edit.Albums[:1]
	wantRefused(t, l.scanner.EditTracks(t.Context(), edit), ErrUnsupportedEdit)
	l.wantUnedited()
}

func TestEditAcrossAlbumsOfAnUnknownAlbum(t *testing.T) {
	t.Parallel()
	l := newPairLib(t)
	edit := l.both(*yearEdit())
	edit.Albums[0].ID = "nope"
	if err := l.scanner.EditTracks(t.Context(), edit); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("EditTracks = %v, want ErrNotFound", err)
	}
}

func TestMovingTracksAcrossAlbumsIntoOneMergesThem(t *testing.T) {
	t.Parallel()
	l := newPairLib(t)
	moved := l.second.Tracks[0].ID
	if err := l.scanner.EditTracks(t.Context(), l.both(Edit{Fields: map[Field][]string{FieldAlbum: {firstAlbum}}})); err != nil {
		t.Fatal(err)
	}
	if _, err := l.st.Album(t.Context(), l.second.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the emptied album answers %v, want ErrNotFound", err)
	}
	if album, err := l.st.TrackAlbum(t.Context(), moved); err != nil || album != l.first.ID {
		t.Errorf("track %s is on %s, %v; want %s, keeping its ID", moved, album, err, l.first.ID)
	}
}

func TestEditAcrossAlbumsStoppedPartwaySaysHowFar(t *testing.T) {
	t.Parallel()
	l := newPairLib(t)
	boom := l.failWrite(2)
	err := l.scanner.EditTracks(t.Context(), l.both(*yearEdit()))
	stopped, ok := errors.AsType[*EditError](err)
	if !ok || !errors.Is(err, boom) {
		t.Fatalf("EditTracks = %v, want an EditError wrapping the cause", err)
	}
	if len(stopped.Changed) != 1 || len(stopped.Remaining) != 1 || stopped.Changed[0] != l.first.Tracks[0].ID {
		t.Errorf("changed %q, remaining %q: want the first album's track changed", stopped.Changed, stopped.Remaining)
	}
	wantStats(t, l.scan(Quick), &Stats{Seen: 2})
}
