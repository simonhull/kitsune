package store_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	listFile  = "Playlists/p.m3u8"
	fileRoot  = "/music"
	readAgain = "Read again"
)

// writeFile stores a read of the playlist file rel naming tracks, in order,
// with one line that found no song after them.
func (l *trashLib) writeFile(t *testing.T, rel, name string, tracks ...store.TrackWrite) string {
	t.Helper()
	paths := must(l.s.TrackPaths(t.Context()))(t)
	w := &store.FilePlaylistWrite{
		Root: fileRoot, RelPath: rel, Size: 10, MTime: time.UnixMilli(1000), Reader: 1,
		NewID: fmt.Sprintf("01FILE%020d", len(rel)), Name: name, Lines: len(tracks) + 1,
		Missing: []store.MissingLine{{Line: 2*len(tracks) + 2, Text: "…/a/b.mp3", Reason: store.MissingOutside}},
	}
	for i := range tracks {
		at := slices.IndexFunc(paths, func(p store.TrackPath) bool { return p.RelPath == tracks[i].RelPath })
		if at < 0 {
			t.Fatalf("no track path %s", tracks[i].RelPath)
		}
		w.Entries = append(w.Entries, store.FileEntry{Line: 2*i + 2, Text: tracks[i].RelPath, Track: paths[at].Row})
	}
	return must(l.s.WriteFilePlaylist(t.Context(), w))(t)
}

// readFilePlaylist reads a file playlist naming every song, live or trashed,
// as a list and as a page: its songs, its mosaic and its albums.
func readFilePlaylist(t *testing.T, l *trashLib) []string {
	t.Helper()
	id := l.writeFile(t, "every.m3u8", "Every", slices.Concat(l.live, l.trashed)...)
	var out []string
	list := must(l.s.Playlists(t.Context(), l.user, nil, nil))(t)
	for i := range list {
		for _, a := range list[i].Mosaic {
			out = append(out, a.ID)
		}
	}
	d := must(l.s.Playlist(t.Context(), l.user, id, nil))(t)
	for i := range d.Entries {
		out = append(out, d.Entries[i].Track.ID, d.Entries[i].Album)
	}
	for _, a := range d.Albums {
		out = append(out, a.ID)
	}
	return out
}

// songsOf returns the ULID of each song of d, in order.
func songsOf(d *store.PlaylistDetail) []string {
	out := make([]string, len(d.Entries))
	for i := range d.Entries {
		out[i] = d.Entries[i].Track.ID
	}
	return out
}

// A file playlist is everyone's, its songs counted live, and it is left out of
// what Add to playlist asks about.
func TestFilePlaylistIsListedForEveryone(t *testing.T) {
	t.Parallel()
	l := newTrashLib(t)
	id := l.writeFile(t, listFile, "P", l.live[0], l.trashed[0], l.live[1], l.live[0])
	for _, user := range []int64{l.user, l.user + 100} {
		list := must(l.s.Playlists(t.Context(), user, nil, nil))(t)
		if len(list) != 1 || list[0].ID != id || list[0].Source != store.PlaylistFromFile || list[0].Songs != 3 {
			t.Fatalf("user %d lists %+v, want the file playlist of 3 live songs", user, list)
		}
		if f := list[0].File; f == nil || f.RelPath != listFile || f.NotFound != 2 {
			t.Errorf("file = %+v, want 2 not found: the outside line and the trashed song", f)
		}
	}
	if held := must(l.s.Playlists(t.Context(), l.user, []string{l.live[0].ULID}, nil))(t); len(held) != 0 {
		t.Errorf("holding lists %+v: nothing is added to a file playlist", held)
	}
}

// A file playlist's page is its songs in the file's order with repeats, and the
// lines that found none, a song in the Trash among them.
func TestFilePlaylistPage(t *testing.T) {
	t.Parallel()
	l := newTrashLib(t)
	first, gone, third := l.live[0], l.trashed[0], l.live[1]
	id := l.writeFile(t, listFile, "P", first, gone, third, first)
	d := must(l.s.Playlist(t.Context(), l.user, id, nil))(t)
	if got, want := songsOf(&d), []string{first.ULID, third.ULID, first.ULID}; !slices.Equal(got, want) {
		t.Errorf("songs = %v, want %v", got, want)
	}
	want := []store.MissingLine{
		{Line: 4, Text: gone.RelPath, Reason: store.MissingTrashed},
		{Line: 10, Text: "…/a/b.mp3", Reason: store.MissingOutside},
	}
	if !slices.Equal(d.Missing, want) {
		t.Errorf("missing = %+v, want %+v", d.Missing, want)
	}
}

// A read again of the file keeps the playlist's ULID; its deletion takes it.
func TestFilePlaylistReadAgainAndDeleted(t *testing.T) {
	t.Parallel()
	l := newTrashLib(t)
	id := l.writeFile(t, listFile, "P", l.live...)
	if again := l.writeFile(t, listFile, readAgain, l.live[1]); again != id {
		t.Errorf("read again as %s, want %s kept", again, id)
	}
	if d := must(l.s.Playlist(t.Context(), l.user, id, nil))(t); d.Name != readAgain || len(d.Entries) != 1 {
		t.Errorf("after a read again: %q with %d songs", d.Name, len(d.Entries))
	}
	ids := must(l.s.DeleteFilePlaylists(t.Context(), fileRoot, []string{listFile, "nothing.m3u"}))(t)
	if !slices.Equal(ids, []string{id}) {
		t.Errorf("deleted %v, want %s", ids, id)
	}
	if _, err := l.s.Playlist(t.Context(), l.user, id, nil); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("read after delete: %v, want ErrNotFound", err)
	}
}

// Nobody changes a file playlist: it belongs to its file. Copying it is fine.
func TestFilePlaylistRefusesWrites(t *testing.T) {
	t.Parallel()
	l := newTrashLib(t)
	id := l.writeFile(t, "p.m3u", "P", l.live...)
	entry := must(l.s.Playlist(t.Context(), l.user, id, nil))(t).Entries[0].ID
	_, addErr := l.s.AddToPlaylist(t.Context(), l.user, id, &store.PlaylistAdd{Tracks: []string{l.live[0].ULID}})
	_, removeErr := l.s.RemoveFromPlaylist(t.Context(), l.user, id, []int64{entry})
	for name, err := range map[string]error{
		"rename":   l.s.RenamePlaylist(t.Context(), l.user, id, "Mine"),
		"delete":   l.s.DeletePlaylist(t.Context(), l.user, id),
		"add":      addErr,
		"take out": removeErr,
		"move":     l.s.MovePlaylistEntry(t.Context(), l.user, id, entry, entry, true),
		"seed":     l.s.SetPlaylistSeed(t.Context(), l.user, id, 1),
	} {
		if !errors.Is(err, store.ErrFilePlaylist) {
			t.Errorf("%s: %v, want ErrFilePlaylist", name, err)
		}
	}
	const copyID = "01COPY00000000000000000000"
	if err := l.s.CopyPlaylist(t.Context(), l.user, id, copyID, nil); err != nil {
		t.Fatal(err)
	}
	if c := must(l.s.Playlist(t.Context(), l.user, copyID, nil))(t); c.Source != store.PlaylistMade || len(c.Entries) != 2 {
		t.Errorf("copy = %+v", c.Playlist)
	}
}
