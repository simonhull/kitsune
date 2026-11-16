package server

import (
	"encoding/json/v2"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/m3u"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	mixFile  = "Playlists/Mix.m3u8"
	savePath = "/admin/playlists/"
)

// mixLines is a playlist file of the fixture's two songs, four times, and three
// lines that find none: a web address, an absolute path outside the library,
// and a path with another extension than the song's.
func mixLines(root string) string {
	return "#EXTM3U\n#EXTINF:1,Sample - One\n../album/one.flac\n../album/two.flac\n../album/one.flac\n" +
		"#EXTINF:30,Radio\nhttps://radio.example/live.mp3\n/elsewhere/Dawn Escapes/07 Moonlit.mp3\n" +
		"../album/one.mp3\n" + root + "/album/two.flac\n"
}

// fileFixture is the two-song album with mixFile beside it, scanned, and the
// file playlist read from it.
type fileFixture struct {
	*editFixture

	other string // Another person's token.
	mix   api.PlaylistDetail
}

func newFileFixture(t *testing.T, tagWrites bool) *fileFixture {
	t.Helper()
	fx := newEditFixture(t, &testServerOptions{TagWrites: tagWrites}, twoFLACs())
	writeUnder(t, fx.root, mixFile, mixLines(fx.root))
	if _, err := fx.Scanner.Scan(t.Context(), library.Quick); err != nil {
		t.Fatal(err)
	}
	ff := &fileFixture{editFixture: fx, other: fx.tokenFor(t, store.RoleUser)}
	ff.mix = ff.fileList(t, fx.token)
	return ff
}

// writeUnder writes text to the file at rel under root, making its folders.
func writeUnder(t *testing.T, root, rel, text string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil { //nolint:gosec // G703: under the test's temp root.
		t.Fatal(err)
	}
}

// fileList reads token's playlists, wants one file playlist among them, and
// reads it whole.
func (fx *fileFixture) fileList(t *testing.T, token string) api.PlaylistDetail {
	t.Helper()
	var list api.Playlists
	fx.getJSON(t, token, playlistsPath, &list)
	i := slices.IndexFunc(list.Playlists, func(p api.Playlist) bool { return p.Source == api.PlaylistFromFile })
	if i < 0 {
		t.Fatalf("playlists = %+v, want a file playlist", list.Playlists)
	}
	var d api.PlaylistDetail
	fx.getJSON(t, token, playlistPath(list.Playlists[i].ID), &d)
	return d
}

// wantOneTwoTwice wants songs to be the album's two songs, then the same two
// again. The two share their tags, so the album's order between them is no
// guide to which file is which; the export test ties each to its path.
func wantOneTwoTwice(t *testing.T, songs, album []string) {
	t.Helper()
	if len(songs) != 4 || songs[0] == songs[1] || !slices.Equal(songs[:2], songs[2:]) ||
		!slices.Contains(album, songs[0]) || !slices.Contains(album, songs[1]) {
		t.Errorf("songs = %v, want one, two, one, two of %v", songs, album)
	}
}

// A playlist file in the library is a playlist everyone sees: the file's songs
// in its order with its repeats, and the lines that found none, with no path
// that names a root.
func TestFilePlaylistIsEveryones(t *testing.T) {
	t.Parallel()
	fx := newFileFixture(t, false)
	d := fx.mix
	if d.Name != mixName || d.Smart || d.File == nil || d.File.Path != mixFile || d.File.Format != "m3u8" {
		t.Fatalf("file playlist = %+v / %+v", d.Playlist, d.File)
	}
	wantOneTwoTwice(t, entrySongs(&d), []string{fx.album.Tracks[0].ID, fx.album.Tracks[1].ID})
	want := []api.MissingLine{
		{Line: 7, Text: "https://…", Title: "Radio", Seconds: 30, Reason: api.MissingNotFile},
		{Line: 8, Text: "…/Dawn Escapes/07 Moonlit.mp3", Reason: api.MissingOutside},
		{Line: 9, Text: "../album/one.mp3", Reason: api.MissingNoSong, OtherExtension: "one.flac"},
	}
	if !slices.Equal(d.Missing, want) || d.File.NotFound != 3 || d.SongCount != 4 {
		t.Errorf("missing = %+v (%d not found, %d songs), want %+v", d.Missing, d.File.NotFound, d.SongCount, want)
	}
}

// entrySongs returns the track of each entry of d, in order.
func entrySongs(d *api.PlaylistDetail) []string {
	out := make([]string, len(d.Entries))
	for i := range d.Entries {
		out[i] = d.Entries[i].Track.ID
	}
	return out
}

// Another person sees the same file playlist, which names no root and no
// absolute path, and which Add to playlist never lists.
func TestFilePlaylistIsSharedAndNamesNoRoot(t *testing.T) {
	t.Parallel()
	fx := newFileFixture(t, false)
	d := fx.mix
	theirs := fx.fileList(t, fx.other)
	if theirs.ID != d.ID || len(theirs.Entries) != 4 {
		t.Errorf("another person sees %+v, want the same playlist", theirs.Playlist)
	}
	resp := fx.send(t, call{method: http.MethodGet, path: playlistPath(d.ID), header: bearer(fx.other)})
	if strings.Contains(string(resp.Body), fx.root) || strings.Contains(string(resp.Body), "/elsewhere") {
		t.Errorf("GET %s names a root or an absolute path: %s", playlistPath(d.ID), resp.Body)
	}
	var held api.Playlists
	fx.getJSON(t, fx.token, playlistsPath+"?holding="+fx.album.Tracks[0].ID, &held)
	if slices.ContainsFunc(held.Playlists, func(p api.Playlist) bool { return p.ID == d.ID }) {
		t.Error("holding lists a file playlist: nothing is added to one")
	}
}

// Nothing a person does to a playlist changes a file's: it belongs to its file.
// A copy is how one makes it their own.
func TestFilePlaylistRefusesChanges(t *testing.T) {
	t.Parallel()
	fx := newFileFixture(t, false)
	id := fx.mix.ID
	entry := fx.mix.Entries[0].ID
	for _, c := range []call{
		{method: http.MethodPatch, path: playlistPath(id), body: `{"name":"Mine"}`},
		{method: http.MethodDelete, path: playlistPath(id)},
		{method: http.MethodPost, path: playlistPath(id, "tracks"), body: tracksBody(fx.album.Tracks[0].ID)},
		{method: http.MethodDelete, path: playlistPath(id, "entries"), body: `{"entries":["` + entry + `"]}`},
		{method: http.MethodPost, path: playlistPath(id, "move"), body: `{"entry":"` + entry + `","after":"` + fx.mix.Entries[1].ID + `"}`},
		{method: http.MethodPost, path: playlistPath(id, "seed")},
	} {
		c.header = jsonHeader(fx.other)
		fx.send(t, c).problem(t, http.StatusConflict)
	}
	resp := fx.send(t, call{method: http.MethodPost, path: playlistPath(id, "copy"), header: bearer(fx.other)})
	var copied api.Playlist
	if resp.Status != http.StatusCreated || json.Unmarshal(resp.Body, &copied) != nil {
		t.Fatalf("copy = %d: %s", resp.Status, resp.Body)
	}
	if copied.Source != api.PlaylistMade || copied.Name != mixName+" (copy)" || copied.SongCount != 4 || copied.File != nil {
		t.Errorf("copy = %+v, want a regular playlist of the four songs", copied)
	}
	if again := fx.fileList(t, fx.other); len(again.Entries) != 4 {
		t.Errorf("the file playlist after the refusals holds %d songs, want 4", len(again.Entries))
	}
}

// m3uCall downloads a playlist as token.
func m3uCall(id, token string) call {
	return call{method: http.MethodGet, path: playlistPath(id, "m3u"), header: bearer(token)}
}

// An export is the playlist's songs with paths from their own library folder,
// so it names no root, under the playlist's name.
func TestPlaylistM3U(t *testing.T) {
	t.Parallel()
	fx := newFileFixture(t, false)
	resp := fx.send(t, m3uCall(fx.mix.ID, fx.other))
	if resp.Status != http.StatusOK {
		t.Fatalf("GET m3u = %d: %s", resp.Status, resp.Body)
	}
	if got := resp.Header.Get("Content-Type"); got != m3uMediaType {
		t.Errorf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); got != "attachment; filename*=UTF-8''Mix.m3u8" {
		t.Errorf("Content-Disposition = %q", got)
	}
	body := string(resp.Body)
	if strings.Contains(body, fx.root) || !strings.HasPrefix(body, "#EXTM3U\n#EXTINF:") {
		t.Errorf("body = %q", body)
	}
	read := m3u.Parse(resp.Body, true, false)
	paths := make([]string, 0, len(read.Lines))
	for _, l := range read.Lines {
		paths = append(paths, l.Path)
	}
	if want := []string{fileOne, fileTwo, fileOne, fileTwo}; !slices.Equal(paths, want) {
		t.Errorf("paths = %q, want %q", paths, want)
	}
	mine := fx.makePlaylist(t, fx.token, &api.PlaylistCreate{Name: "Été / mine", Tracks: []string{fx.album.Tracks[1].ID}})
	resp = fx.send(t, m3uCall(mine.Playlist.ID, fx.token))
	if got := resp.Header.Get("Content-Disposition"); got != "attachment; filename*=UTF-8''%C3%89t%C3%A9%20%20mine.m3u8" {
		t.Errorf("Content-Disposition = %q", got)
	}
	fx.send(t, m3uCall(mine.Playlist.ID, fx.other)).problem(t, http.StatusNotFound)
}

// saveCall saves playlist id as token.
func saveCall(id, token, body string) call {
	return call{method: http.MethodPost, path: savePath + id + "/save", body: body, header: jsonHeader(token)}
}

const (
	savedDir = "Saved"
	outName  = "Out"
)

// saveFixture is the file fixture with tag writes on and a playlist of the
// album's two songs, second first, named outName.
type saveFixture struct {
	*fileFixture

	id string // The playlist's.
}

func newSaveFixture(t *testing.T) *saveFixture {
	t.Helper()
	fx := newFileFixture(t, true)
	mine := fx.makePlaylist(t, fx.token, &api.PlaylistCreate{Name: outName, Tracks: []string{fx.album.Tracks[1].ID, fx.album.Tracks[0].ID}})
	return &saveFixture{fileFixture: fx, id: mine.Playlist.ID}
}

// body is a save into folder under the fixture's root.
func (fx *saveFixture) body(t *testing.T, folder, name string, replace bool) string {
	t.Helper()
	return jsonBody(t, api.PlaylistSave{Root: fx.root, Folder: folder, Name: name, Replace: replace})
}

// save saves id as the admin and wants a 200.
func (fx *saveFixture) save(t *testing.T, id, folder, name string, replace bool) api.PlaylistSaved {
	t.Helper()
	var saved api.PlaylistSaved
	fx.send(t, saveCall(id, fx.token, fx.body(t, folder, name, replace))).decode(t, &saved)
	return saved
}

// A save is an administrator's, of a playlist they see, into a folder in a
// root, under a name with no path in it.
func TestPlaylistSaveRefusals(t *testing.T) {
	t.Parallel()
	fx := newSaveFixture(t)
	fx.send(t, saveCall(fx.id, fx.other, fx.body(t, savedDir, outName, false))).problem(t, http.StatusForbidden)
	bad := saveCall(fx.id, fx.token, `{"folder":"x"}`)
	bad.invalid = true
	fx.send(t, bad).problem(t, http.StatusBadRequest)
	fx.send(t, saveCall("01J0000000000000000000000Z", fx.token, fx.body(t, savedDir, outName, false))).problem(t, http.StatusNotFound)
	for _, body := range []string{
		fx.body(t, "../x", outName, false), fx.body(t, savedDir, "a/b", false), fx.body(t, savedDir, ".hidden", false),
		fx.body(t, ".hidden", outName, false), jsonBody(t, api.PlaylistSave{Root: t.TempDir(), Folder: savedDir, Name: outName}),
	} {
		fx.send(t, saveCall(fx.id, fx.token, body)).problem(t, http.StatusUnprocessableEntity)
	}
}

// A save writes the playlist into a library folder with paths from that
// folder, reads it at once as a file playlist and tells everyone.
func TestPlaylistSave(t *testing.T) {
	t.Parallel()
	fx := newSaveFixture(t)
	tap := fx.Events.Subscribe(events.Viewer{UserID: 99}, "")
	defer fx.Events.Unsubscribe(tap)
	if saved := fx.save(t, fx.id, savedDir, outName, false); saved != (api.PlaylistSaved{Name: "Out.m3u8", Folder: savedDir, SongCount: 2}) {
		t.Errorf("saved = %+v", saved)
	}
	written, err := os.ReadFile(filepath.Join(fx.root, savedDir, "Out.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "\n../album/two.flac\n") || strings.Contains(string(written), fx.root) {
		t.Errorf("file = %q, want paths from its folder", written)
	}
	if heard := drainTap(tap); !slices.ContainsFunc(heard, func(ev string) bool { return strings.Contains(ev, "playlists") }) {
		t.Errorf("another person heard %v, want a playlists event", heard)
	}
	if p := fx.savedList(t); p.SongCount != 2 || p.File.NotFound != 0 {
		t.Errorf("read back = %+v, want its two songs", p)
	}
	if saved := fx.save(t, fx.mix.ID, "", "Root mix", false); saved.Folder != "" || saved.SongCount != 4 {
		t.Errorf("saved at the root = %+v", saved)
	}
}

// savedList returns the file playlist read from Saved/Out.m3u8.
func (fx *saveFixture) savedList(t *testing.T) api.Playlist {
	t.Helper()
	var list api.Playlists
	fx.getJSON(t, fx.other, playlistsPath, &list)
	i := slices.IndexFunc(list.Playlists, func(p api.Playlist) bool { return p.File != nil && p.File.Path == "Saved/Out.m3u8" })
	if i < 0 {
		t.Fatalf("playlists = %+v, want the saved file", list.Playlists)
	}
	return list.Playlists[i]
}

// A save never overwrites without replace: it names the file playlist that is
// there, and the next free name.
func TestPlaylistSaveNeverOverwrites(t *testing.T) {
	t.Parallel()
	fx := newSaveFixture(t)
	fx.save(t, fx.id, savedDir, outName, false)
	there := fx.savedList(t)
	resp := fx.send(t, saveCall(fx.id, fx.token, fx.body(t, savedDir, "Out.m3u8", false)))
	var exists api.PlaylistExists
	if resp.Status != http.StatusConflict || json.Unmarshal(resp.Body, &exists) != nil {
		t.Fatalf("second save = %d: %s", resp.Status, resp.Body)
	}
	if e := exists.Exists; e.Name != "Out.m3u8" || e.FreeName != "Out 2.m3u8" || e.Playlist != there.ID ||
		e.PlaylistName != outName || e.SongCount == nil || *e.SongCount != 2 {
		t.Errorf("exists = %+v", e)
	}
	fx.save(t, fx.id, savedDir, outName, true)
	if again := fx.savedList(t); again.ID != there.ID {
		t.Errorf("replaced as %s, want %s kept", again.ID, there.ID)
	}
}

// With tag writes off, nothing is saved into the library.
func TestPlaylistSaveNeedsTagWrites(t *testing.T) {
	t.Parallel()
	fx := newFileFixture(t, false)
	body := jsonBody(t, api.PlaylistSave{Root: fx.root, Folder: savedDir, Name: outName})
	fx.send(t, saveCall(fx.mix.ID, fx.token, body)).problem(t, http.StatusConflict)
	if _, err := os.Stat(filepath.Join(fx.root, savedDir)); !os.IsNotExist(err) {
		t.Errorf("stat Saved = %v, want nothing written", err)
	}
}

// A file playlist goes with its file, but not when its root looks down.
func TestFilePlaylistGoesWithItsFile(t *testing.T) {
	t.Parallel()
	fx := newFileFixture(t, false)
	moved := filepath.Join(t.TempDir(), "root")
	if err := os.Rename(fx.root, moved); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.Scanner.Scan(t.Context(), library.Quick); err != nil {
		t.Fatal(err)
	}
	if again := fx.fileList(t, fx.token); again.ID != fx.mix.ID {
		t.Errorf("after the root went missing the file playlist is %s, want %s kept", again.ID, fx.mix.ID)
	}
	if err := os.Rename(moved, fx.root); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(fx.root, mixFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.Scanner.Scan(t.Context(), library.Quick); err != nil {
		t.Fatal(err)
	}
	fx.send(t, call{method: http.MethodGet, path: playlistPath(fx.mix.ID), header: bearer(fx.token)}).problem(t, http.StatusNotFound)
}
