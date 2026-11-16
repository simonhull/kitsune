package server

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/m3u"
)

// speedRuns is how many times a scanner speed test times its scan, after one to
// warm up.
const speedRuns = 20

// TestPlaylistFileSpeed times a read of a 10,000-song .m3u8 against 50,000
// songs as the scanner does it: the targeted scan of its folder that the
// watcher starts, which reads it, matches every line and stores the entries.
func TestPlaylistFileSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "m3u-read")
	ts := newTestServer(t)
	shape := benchShapes[len(benchShapes)-1]
	ts.seedBench(t, shape.tracks, shape.albums, shape.artists)
	fr := newFileRead(t, ts, shape.albums)
	fr.read(t)
	list, err := ts.Store.Playlists(t.Context(), 1, nil, nil)
	if err != nil || len(list) != 1 || list[0].Songs != m3u.MostPaths || list[0].File.NotFound != 0 {
		t.Fatalf("playlists = %+v, %v; want one of %d songs, all found", list, err, m3u.MostPaths)
	}
	took := make([]time.Duration, speedRuns)
	for i := range took {
		took[i] = fr.read(t)
	}
	holdSpeed(t, b, limit, took)
}

// fileRead is a 10,000-song playlist file naming the bench library's songs,
// absolutely, in a root of its own beside the bench's.
type fileRead struct {
	sc        *library.Scanner
	dir, path string
	at        time.Time // Its mtime.
}

func newFileRead(t *testing.T, ts *testServer, albums int) *fileRead {
	t.Helper()
	var file strings.Builder
	file.WriteString("#EXTM3U\n")
	for i := range m3u.MostPaths {
		fmt.Fprintf(&file, "#EXTINF:240,Artist - Track %d\n%s/a%d/%d.flac\n", i, musicBase, i%albums+1, i/albums)
	}
	dir := t.TempDir()
	fr := &fileRead{
		sc:  library.NewScanner(ts.Store, library.Options{Roots: []string{dir, musicBase}, MissingGrace: time.Hour}),
		dir: dir, path: filepath.Join(dir, "Everything.m3u8"), at: time.Now(),
	}
	if err := os.WriteFile(fr.path, []byte(file.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return fr
}

// read changes the file's mtime, so a scan reads it, and times the targeted
// scan of its folder.
func (fr *fileRead) read(t *testing.T) time.Duration {
	t.Helper()
	fr.at = fr.at.Add(time.Second)
	if err := os.Chtimes(fr.path, fr.at, fr.at); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	stats, err := fr.sc.ScanDirs(t.Context(), fr.dir, []string{"."})
	took := time.Since(start)
	if err != nil || stats.Playlists != 1 {
		t.Fatalf("ScanDirs = %+v, %v; want the file read", stats, err)
	}
	return took
}

// playlistFilesInScan is how many unchanged playlist files the quick scan
// speed test adds to a library.
const playlistFilesInScan = 50

// TestQuickScanWithPlaylistFilesSpeed times a quick scan of a library holding
// 50 unchanged playlist files, and holds it to the same scan without them: an
// unchanged file is only stat'd, so the two differ by noise.
func TestQuickScanWithPlaylistFilesSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "quick-scan-playlists")
	plain, with := scanLibrary(t, 0), scanLibrary(t, playlistFilesInScan)
	without, held := make([]time.Duration, 0, speedRuns), make([]time.Duration, 0, speedRuns)
	for range speedRuns {
		without = append(without, plain.quickScan(t))
		held = append(held, with.quickScan(t))
	}
	slices.Sort(without)
	slices.Sort(held)
	base, got := percentile(without, 50), percentile(held, 50)
	t.Logf("median quick scan: %v without playlist files, %v with %d", base, got, playlistFilesInScan)
	if noise := base/5 + 2*time.Millisecond; got-base > noise {
		t.Errorf("%d unchanged playlist files added %v to a quick scan of %v, past the %v noise allows", playlistFilesInScan, got-base, base, noise)
	}
	holdSpeed(t, b, limit, held)
}

// scanLibrary is a scanned library of 500 songs on 50 albums, with as many
// playlist files as playlists says, of 40 lines each.
func scanLibrary(t *testing.T, playlists int) *testServer {
	t.Helper()
	root := t.TempDir()
	sample, err := os.ReadFile(filepath.Join("..", "library", "testdata", "sample.flac"))
	if err != nil {
		t.Fatal(err)
	}
	const albums, perAlbum = 50, 10
	for a := range albums {
		for s := range perAlbum {
			writeUnder(t, root, fmt.Sprintf("Album %d/%02d.flac", a, s), string(sample))
		}
	}
	for p := range playlists {
		var lines strings.Builder
		for l := range 40 {
			fmt.Fprintf(&lines, "../Album %d/%02d.flac\n", (p+l)%albums, l%perAlbum)
		}
		writeUnder(t, root, fmt.Sprintf("Playlists/List %d.m3u8", p), lines.String())
	}
	ts := newTestServer(t, testServerOptions{LibraryRoot: root})
	if stats, scanErr := ts.Scanner.Scan(t.Context(), library.Quick); scanErr != nil || stats.Playlists != playlists {
		t.Fatalf("first scan = %+v, %v", stats, scanErr)
	}
	return ts
}

// quickScan times one quick scan, which must read nothing.
func (ts *testServer) quickScan(t *testing.T) time.Duration {
	t.Helper()
	start := time.Now()
	stats, err := ts.Scanner.Scan(t.Context(), library.Quick)
	took := time.Since(start)
	if err != nil || stats.Read != 0 || stats.Playlists != 0 {
		t.Fatalf("quick scan = %+v, %v; want nothing read", stats, err)
	}
	return took
}
