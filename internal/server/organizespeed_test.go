package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/organize"
	"github.com/simonhull/kitsune/internal/store"
)

// The organize speed shapes: 50,000 songs in albums of ten, one album in five
// out of line; and a run of 1,000 songs in 100 albums.
const (
	organizeSpeedSongs   = 50_000
	organizeSpeedPerDisc = 10
	organizeOutOfLine    = 5 // One album in this many is out of line.
	organizeRunAlbums    = 100
)

// organizeSong is the track write of song t of album a by artist, filed at rel
// under root, of size bytes changed at mtime.
func organizeSong(root, rel string, a, t int, artist *store.ArtistWrite, size int64, mtime time.Time) store.TrackWrite {
	album := store.AlbumWrite{
		GroupKey: fmt.Sprintf("g%d", a), ULID: albumULID(a), Name: fmt.Sprintf("Album %d", a),
		SortName: fmt.Sprintf("Album %d", a), Artist: artist.Name, Year: 1960 + a%60,
	}
	return store.TrackWrite{
		ULID: fmt.Sprintf("01J%020d%03d", a, t), Root: root, RelPath: rel, Size: size, MTime: mtime,
		Format: streamFormat, Codec: flacFormat, Duration: 4 * time.Minute, SampleRate: 44100, BitDepth: 16,
		Lossless: true, Title: fmt.Sprintf("Track %d", t), TrackNo: t, Year: album.Year, Album: album,
		Artists: []store.ArtistWrite{*artist}, AlbumArtists: []store.ArtistWrite{*artist},
	}
}

// filed is where the default template files song t of album a by artist.
func filed(a, t int, artist string) string {
	return fmt.Sprintf("%s/%d - Album %d/%02d - Track %d.flac", artist, 1960+a%60, a, t, t)
}

// TestOrganizePreviewSpeed times a preview of the whole library at 50,000
// songs, a fifth of them to move: it reads the kept tags and lists only the
// folders the moves land in, none of them there.
func TestOrganizePreviewSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "organize-preview")
	ts := newTestServer(t, testServerOptions{TagWrites: true})
	if err := ts.Scanner.Configure(t.Context(), library.Live{Roots: []string{musicBase}, MissingGrace: time.Hour, TagWrites: true}); err != nil {
		t.Fatal(err)
	}
	tws := make([]store.TrackWrite, 0, organizeSpeedSongs)
	for a := range organizeSpeedSongs / organizeSpeedPerDisc {
		artist := person(a%4000+1, fmt.Sprintf("Artist %d", a%4000))
		for t := 1; t <= organizeSpeedPerDisc; t++ {
			rel := filed(a+1, t, artist.Name)
			if a%organizeOutOfLine == 0 {
				rel = fmt.Sprintf("old/a%d/%d.flac", a+1, t)
			}
			tws = append(tws, organizeSong(musicBase, rel, a+1, t, &artist, 1, time.UnixMilli(1)))
		}
	}
	if err := ts.Store.WriteBatch(t.Context(), &store.Batch{Tracks: tws}); err != nil {
		t.Fatal(err)
	}
	token := ts.tokenFor(t, store.RoleAdmin)
	p := ts.previewAs(t, token)
	if c := p.Counts; c.Songs != organizeSpeedSongs || c.Moves != organizeSpeedSongs/organizeOutOfLine || c.InPlace != c.Songs-c.Moves {
		t.Fatalf("counts = %+v, want a fifth of %d to move and the rest in place", c, organizeSpeedSongs)
	}
	holdSpeed(t, b, limit, ts.timeCalls(t, token, http.MethodPost, organizePreviewPath, `{}`))
}

// previewAs previews the whole library as the holder of token.
func (ts *testServer) previewAs(t *testing.T, token string) api.OrganizePreview {
	t.Helper()
	var p api.OrganizePreview
	resp := ts.send(t, call{method: http.MethodPost, path: organizePreviewPath, body: `{}`, header: jsonHeader(token)})
	resp.decode(t, &p)
	return p
}

// TestOrganizeRunSpeed times a run of 1,000 songs in 100 albums, every one
// moved to a new folder and journaled, as milliseconds per song. Each run
// files the songs by the other of two templates, so every run moves them all.
func TestOrganizeRunSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "organize-run")
	root := t.TempDir()
	ts := newTestServer(t, testServerOptions{LibraryRoot: root, TagWrites: true})
	at := time.UnixMilli(1_700_000_000_000)
	tws := make([]store.TrackWrite, 0, organizeRunAlbums*organizeSpeedPerDisc)
	for a := range organizeRunAlbums {
		artist := person(a+1, fmt.Sprintf("Artist %d", a))
		for t := 1; t <= organizeSpeedPerDisc; t++ {
			rel := filed(a+1, t, artist.Name)
			tws = append(tws, organizeSong(root, filepath.FromSlash(rel), a+1, t, &artist, 4096, at))
		}
	}
	for i := range tws {
		writeSized(t, root, tws[i].RelPath, at)
	}
	if err := ts.Store.WriteBatch(t.Context(), &store.Batch{Tracks: tws}); err != nil {
		t.Fatal(err)
	}
	templates := [2]string{"<Album Artist>/<Album>/<Track> <Title>", organize.DefaultTemplate}
	took := make([]time.Duration, speedRuns+1)
	for i := range took {
		took[i] = ts.timeRun(t, templates[i%2], len(tws))
	}
	holdSpeed(t, b, limit, took[1:]) // The first warms up.
}

// writeSized writes a 4 KiB file at rel under root, changed at mtime.
func writeSized(t *testing.T, root, rel string, mtime time.Time) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// timeRun previews the library under template, runs it, wants every one of
// songs moved, and returns the run's time per song.
func (ts *testServer) timeRun(t *testing.T, template string, songs int) time.Duration {
	t.Helper()
	main, err := organize.Parse(template)
	if err != nil {
		t.Fatal(err)
	}
	c := library.DefaultOrganize()
	c.Template = main
	p, err := ts.Scanner.PreviewOrganize(t.Context(), &c, library.OrganizeScope{})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	run, fn, err := ts.Scanner.StartOrganize(t.Context(), p, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	fn(t.Context())
	took := time.Since(start)
	info, err := ts.Store.OrganizeRunByULID(t.Context(), run.ID)
	if err != nil || info.Moved != songs {
		t.Fatalf("run = %+v, %v; want all %d moved", info, err, songs)
	}
	return took / time.Duration(songs)
}
