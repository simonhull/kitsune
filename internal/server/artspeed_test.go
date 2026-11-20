package server

import (
	"crypto/sha256"
	"fmt"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

// Speed tests of artist art and album extras read from local files (plan 072,
// Phase 1).

// listedArtists is how many artists the list's budget reads, each with a thumb.
const listedArtists = 2000

// folderArt is a stored folder image for an artist, its hash from seed.
func folderArt(seed string) *store.ArtworkWrite {
	return &store.ArtworkWrite{
		Hash: sha256.Sum256([]byte(seed)), Width: 1000, Height: 1000,
		Accent: "#c83838", Ground: "#741f1f", Source: store.ArtworkFolder,
	}
}

// artistRows returns the row and public ID of each artist, by name.
func artistRows(t *testing.T, ts *testServer) map[string]store.LayoutArtist {
	t.Helper()
	l, err := ts.Store.FolderLayout(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	artists, err := ts.Store.Artists(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	byULID := map[string]store.LayoutArtist{}
	for _, owners := range l.Owners {
		for _, o := range owners {
			byULID[o.ULID] = o
		}
	}
	out := make(map[string]store.LayoutArtist, len(artists))
	for i := range artists {
		out[artists[i].Name] = byULID[artists[i].ID]
	}
	return out
}

// giveArt stores, for each artist, every kind of image wanted and a bio when
// bio is set, as a scan of their folders would.
func giveArt(t *testing.T, ts *testServer, artists []store.LayoutArtist, kinds []store.ArtistImageKind, bio bool) {
	t.Helper()
	var w store.ExtrasWrite
	for _, a := range artists {
		for _, k := range kinds {
			w.ArtistImages = append(w.ArtistImages, store.ArtistImageWrite{
				ArtistID: a.ID, Kind: k, Image: folderArt(fmt.Sprintf("%d-%s", a.ID, k)),
				File: store.FileRef{Root: musicBase, RelPath: fmt.Sprintf("%d/%s.jpg", a.ID, k), Size: 1, MTime: time.UnixMilli(1)},
			})
		}
		if bio {
			w.Bios = append(w.Bios, store.BioWrite{
				ArtistID: a.ID, Bio: &store.NfoBio{Summary: nfoBio, Body: nfoBio + "\n\nThey split in 1994."},
				File: store.FileRef{Root: musicBase, RelPath: fmt.Sprintf("%d/artist.nfo", a.ID), Size: 1, MTime: time.UnixMilli(1)},
			})
		}
	}
	if _, err := ts.Store.WriteExtras(t.Context(), &w, time.Now()); err != nil {
		t.Fatal(err)
	}
}

// everyKind is every kind of artist image.
func everyKind() []store.ArtistImageKind {
	return []store.ArtistImageKind{store.ArtistThumb, store.ArtistBackground, store.ArtistLogo, store.ArtistBanner}
}

// TestArtistDetailSpeed times an artist's page with all four images and a bio,
// of a library as large as the index's budget reads, and logs the same page
// without them: the art costs two small reads beside the albums'.
func TestArtistDetailSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "artist-detail")
	ts := newTestServer(t)
	shape := benchShapes[len(benchShapes)-1]
	token := ts.seedBench(t, shape.tracks, shape.albums, shape.artists)
	rows := artistRows(t, ts)
	bare, dressed := rows["Artist 1"], rows["Artist 0"]
	giveArt(t, ts, []store.LayoutArtist{dressed}, everyKind(), true)
	without := ts.timeGets(t, token, artistsPath+"/"+bare.ULID)
	with := ts.timeGets(t, token, artistsPath+"/"+dressed.ULID)
	slices.Sort(without)
	t.Logf("an artist without art: median %v, p95 %v", percentile(without, 50), percentile(without, 95))
	holdSpeed(t, b, limit, with)
}

// TestArtistsListSpeed times GET /artists at 2,000 artists, each with a thumb.
func TestArtistsListSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "artists-list")
	ts := newTestServer(t)
	shape := benchShapes[len(benchShapes)-1]
	token := ts.seedBench(t, shape.tracks, shape.albums, listedArtists)
	without := ts.timeGets(t, token, artistsPath)
	all := artistRows(t, ts)
	artists := make([]store.LayoutArtist, 0, len(all))
	for _, a := range all {
		artists = append(artists, a)
	}
	giveArt(t, ts, artists, []store.ArtistImageKind{store.ArtistThumb}, false)
	slices.Sort(without)
	t.Logf("%d artists without thumbs: median %v, p95 %v", len(artists), percentile(without, 50), percentile(without, 95))
	holdSpeed(t, b, limit, ts.timeGets(t, token, artistsPath))
}

// artLibraryAlbums and artLibrarySongs are the shape of the art scan's library:
// as many songs as the playlist files' scan reads, each album an artist's own.
const (
	artLibraryAlbums = 50
	artLibrarySongs  = 10
)

// artLibrary is a scanned library of 50 artists' albums of 10 songs, each in
// <Artist>/<Album>/; with art, every artist folder holds a thumb, a background,
// a logo, a banner and an artist.nfo, and every album folder a back cover and
// disc art. It returns the server and how long its first scan took.
func artLibrary(t *testing.T, art bool) (*testServer, time.Duration) {
	t.Helper()
	root := t.TempDir()
	sample, err := os.ReadFile(filepath.Join("..", "library", "testdata", sampleFLAC))
	if err != nil {
		t.Fatal(err)
	}
	for a := range artLibraryAlbums {
		folder := fmt.Sprintf("Artist %d", a)
		first := filepath.Join(root, folder, albumFolder, "00.flac")
		writeUnder(t, root, filepath.Join(folder, albumFolder, "00.flac"), string(sample))
		if err = audiometa.Write(first, func(f *audiometa.File) error {
			f.Tags.Album, f.Tags.AlbumArtist, f.Tags.Artists = albumFolder, folder, []string{folder}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		tagged, rerr := os.ReadFile(first)
		if rerr != nil {
			t.Fatal(rerr)
		}
		for s := 1; s < artLibrarySongs; s++ {
			writeUnder(t, root, filepath.Join(folder, albumFolder, fmt.Sprintf("%02d.flac", s)), string(tagged))
		}
		if art {
			writeArt(t, root, folder, uint8(a))
		}
	}
	cache := t.TempDir()
	ts := newTestServer(t, testServerOptions{LibraryRoot: root, Artwork: artwork.NewCache(cache)})
	start := time.Now()
	stats, err := ts.Scanner.Scan(t.Context(), library.Quick)
	took := time.Since(start)
	if want := map[bool]int{true: 7 * artLibraryAlbums}[art]; err != nil || stats.Extras != want {
		t.Fatalf("first scan = %+v, %v; want %d extras", stats, err, want)
	}
	t.Logf("art cache after the first scan: %.1f MB", float64(diskUse(t, cache))/(1<<20))
	return ts, took
}

// writeArt writes an artist's folder and its album's, every image its own, so
// a first scan renders each: a 1000 px thumb, a 1080p background, an 800 px
// logo, a banner and an artist.nfo, and a back cover and disc art.
func writeArt(t *testing.T, root, folder string, n uint8) {
	t.Helper()
	c := func(r, g, b uint8) color.NRGBA { return color.NRGBA{R: r, G: g + n, B: b, A: 0xFF} }
	for name, data := range map[string][]byte{
		thumbFile:         paint(t, 1000, 1000, c(0xC0, 0x40, 0x40), false),
		"fanart.jpg":      paint(t, 1920, 1080, c(0x40, 0x40, 0xC0), false),
		"clearlogo.png":   paint(t, 800, 310, c(0xF0, 0xA0, 0x20), true),
		"banner.jpg":      paint(t, 1000, 185, c(0x20, 0x90, 0x40), false),
		nfoFile:           []byte("<artist><biography>" + nfoBio + "</biography></artist>"),
		"Album/back.jpg":  paint(t, 1000, 1000, c(0x80, 0x80, 0x80), false),
		"Album/cdart.png": paint(t, 1000, 1000, c(0x10, 0x10, 0x10), true),
	} {
		writeUnder(t, root, filepath.Join(folder, name), string(data))
	}
}

// diskUse sums the size of every file under dir.
func diskUse(t *testing.T, dir string) int64 {
	t.Helper()
	var n int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, ierr := d.Info()
		if ierr != nil {
			return fmt.Errorf("%s: %w", d.Name(), ierr)
		}
		n += info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestQuickScanWithArtSpeed times a quick scan of a library whose every artist
// and album folder holds art, and holds it to the same scan without: an
// unchanged file is only compared with what the store holds, so the two differ
// by noise. It logs what the first scans took, renders included.
func TestQuickScanWithArtSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "quick-scan-art")
	plain, plainFirst := artLibrary(t, false)
	with, withFirst := artLibrary(t, true)
	t.Logf("first scan of %d songs: %v without art, %v with %d images and %d bios", artLibraryAlbums*artLibrarySongs,
		plainFirst, withFirst, 6*artLibraryAlbums, artLibraryAlbums)
	without, held := make([]time.Duration, 0, speedRuns), make([]time.Duration, 0, speedRuns)
	for range speedRuns {
		without = append(without, plain.quickScan(t))
		held = append(held, with.quickScan(t))
	}
	slices.Sort(without)
	slices.Sort(held)
	base, got := percentile(without, 50), percentile(held, 50)
	t.Logf("median quick scan: %v without art, %v with", base, got)
	if noise := base/5 + 2*time.Millisecond; got-base > noise {
		t.Errorf("unchanged art added %v to a quick scan of %v, past the %v noise allows", got-base, base, noise)
	}
	holdSpeed(t, b, limit, held)
}
