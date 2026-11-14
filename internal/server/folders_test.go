package server

import (
	"encoding/json/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	foldersPath = "/admin/folders"
	booksPath   = "/audiobooks"
)

// folderTree makes the server's "/" for a test: two folders, one holding
// audio, a hidden folder, a file, a link inside and a link out, and a folder
// beside it the links can't reach. It returns the "/".
func folderTree(t *testing.T) string {
	t.Helper()
	top := t.TempDir()
	root := filepath.Join(top, "root")
	outside := filepath.Join(top, "outside")
	for _, dir := range []string{
		"root/music", "root/audiobooks/Dune/CD1", "root/audiobooks/@eaDir", "root/.hidden", "root/data", "outside/secret",
		"root/audiobooks/.kitsune-trash/01J0000000000000000000TRSH",
	} {
		if err := os.MkdirAll(filepath.Join(top, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{
		"root/notes.txt", "root/audiobooks/Dune/01.m4b", "root/audiobooks/Dune/CD1/02.mp3",
		"root/audiobooks/Dune/cover.jpg", "root/audiobooks/@eaDir/x.mp3", "outside/secret/hidden.flac",
		"root/audiobooks/.kitsune-trash/01J0000000000000000000TRSH/t.flac",
	} {
		if err := os.WriteFile(filepath.Join(top, file), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range map[string]string{
		"books":  "audiobooks", // Relative, inside.
		"abs":    booksPath,    // Absolute: read as a path under the root.
		"out":    outside,      // Absolute, to a real folder outside: not under the root.
		"relout": "../outside", // Relative, out of the root.
		"deep":   "music/../../outside/secret",
	} {
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// folders lists path as the holder of token.
func (ts *testServer) folders(t *testing.T, token, path string) response {
	t.Helper()
	return ts.send(t, call{method: http.MethodGet, path: foldersPath + "?path=" + url.QueryEscape(path), header: bearer(token)})
}

func decodeListing(t *testing.T, r response) api.FolderListing {
	t.Helper()
	if r.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", r.Status, r.Body)
	}
	var l api.FolderListing
	if err := json.Unmarshal(r.Body, &l); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestFoldersListDirectoriesOnly(t *testing.T) {
	t.Parallel()
	root := folderTree(t)
	ts := newTestServer(t, testServerOptions{FolderRoot: root, DataDir: "/data"})
	sess := ts.sessionFor(t)
	ts.patchSettings(t, sess.token, `{"library.roots":["`+musicBase+`"]}`, false)

	r := ts.folders(t, sess.token, "/")
	got := decodeListing(t, r)
	want := []api.Folder{
		{Name: "abs", Path: booksPath, Kind: api.FolderFree}, // Links list the folder they name.
		{Name: "audiobooks", Path: booksPath, Kind: api.FolderFree},
		{Name: "books", Path: booksPath, Kind: api.FolderFree},
		{Name: "data", Path: "/data", Kind: api.FolderData},
		{Name: musicBase[1:], Path: musicBase, Kind: api.FolderLibrary},
	}
	if !slices.Equal(got.Folders, want) || got.Path != "/" || got.Parent != "" {
		t.Errorf("listing / = %+v, want %+v", got, want)
	}
	for _, never := range []string{"notes.txt", ".hidden", "out\"", "secret"} {
		if strings.Contains(string(r.Body), never) {
			t.Errorf("listing names %s: %s", never, r.Body)
		}
	}
}

func TestFoldersCountAudioAsAScanWould(t *testing.T) {
	t.Parallel()
	root := folderTree(t)
	ts := newTestServer(t, testServerOptions{FolderRoot: root})
	sess := ts.sessionFor(t)
	got := decodeListing(t, ts.folders(t, sess.token, booksPath))
	if got.Audio != (api.AudioCount{Files: 2, Folders: 2, Complete: true}) || got.Parent != "/" || got.Kind != api.FolderFree {
		t.Errorf("listing = %+v, want 2 audio files in 2 folders, @eaDir and the Trash left out", got)
	}
	if len(got.Folders) != 1 || got.Folders[0] != (api.Folder{Name: "Dune", Path: "/audiobooks/Dune", Kind: api.FolderFree}) {
		t.Errorf("folders = %+v, want Dune alone", got.Folders)
	}
	ts.patchSettings(t, sess.token, `{"library.roots":["/audiobooks/Dune"]}`, false)
	if got = decodeListing(t, ts.folders(t, sess.token, booksPath)); got.Kind != api.FolderHolds {
		t.Errorf("a folder holding a root = %q, want holds", got.Kind)
	}
	if got = decodeListing(t, ts.folders(t, sess.token, "/audiobooks/Dune/CD1")); got.Kind != api.FolderInside {
		t.Errorf("a folder inside a root = %q, want inside", got.Kind)
	}
	// A link inside the root is followed to its folder.
	if got = decodeListing(t, ts.folders(t, sess.token, "/books")); got.Audio.Files != 2 {
		t.Errorf("through the link = %+v", got)
	}
}

func TestFoldersNeverLeaveTheRoot(t *testing.T) {
	t.Parallel()
	root := folderTree(t)
	ts := newTestServer(t, testServerOptions{FolderRoot: root})
	sess := ts.sessionFor(t)
	for _, p := range []string{"/out", "/out/secret", "/relout", "/relout/secret", "/deep", "/../outside", "/music/../../outside/secret", "/notes.txt", "/nope"} {
		r := ts.folders(t, sess.token, p)
		if r.Status == http.StatusOK {
			got := decodeListing(t, r)
			if got.Path != "/" && !strings.HasPrefix(got.Path, "/music") || strings.Contains(string(r.Body), "secret") {
				t.Errorf("%s escaped: %s", p, r.Body)
			}
			continue
		}
		r.problem(t, http.StatusNotFound)
	}
	ts.folders(t, sess.token, "music").problem(t, http.StatusBadRequest)
	if got := decodeListing(t, ts.folders(t, sess.token, "/..")); got.Path != "/" {
		t.Errorf("/.. = %q, want /", got.Path)
	}
}

func TestFoldersAreForAdministrators(t *testing.T) {
	t.Parallel()
	root := folderTree(t)
	ts := newTestServer(t, testServerOptions{FolderRoot: root})
	ts.folders(t, ts.tokenFor(t, store.RoleUser), "/").problem(t, http.StatusForbidden)
	ts.send(t, call{method: http.MethodGet, path: foldersPath, invalid: true}).problem(t, http.StatusUnauthorized)
	// No path means /.
	if got := decodeListing(t, ts.send(t, call{method: http.MethodGet, path: foldersPath, header: bearer(ts.sessionFor(t).token)})); got.Path != "/" {
		t.Errorf("no path = %q, want /", got.Path)
	}
}
