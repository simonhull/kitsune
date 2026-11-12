package library

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json/v2"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	coverJPG   = "cover.jpg"
	albumName  = "Cover Album"
	embeddedIs = "embedded"
	folderIs   = "folder"
	coverPNG   = "cover.png"
)

// artLib is a test library whose scanner renders album art.
type artLib struct {
	*testLib

	cache *artwork.Cache
}

func newArtLib(t *testing.T) *artLib {
	t.Helper()
	l := &artLib{testLib: newTestLib(t, 1), cache: artwork.NewCache(filepath.Join(t.TempDir(), "art"))}
	l.opts.Artwork = l.cache
	l.scanner = NewScanner(l.st, l.opts)
	return l
}

// sequential makes the scan read one file at a time, in survey order.
func (l *artLib) sequential() {
	l.opts.Workers = 1
	l.scanner = NewScanner(l.st, l.opts)
}

// testPNG returns a size x size PNG of one color.
func testPNG(t *testing.T, c color.Color) []byte {
	t.Helper()
	const size = 64
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < len(img.Pix); i += 4 {
		r, g, b, a := c.RGBA()
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = byte(r>>8), byte(g>>8), byte(b>>8), byte(a>>8) //nolint:gosec // Channels are 8-bit.
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func red() color.RGBA  { return color.RGBA{0xC8, 0x38, 0x38, 0xFF} }
func blue() color.RGBA { return color.RGBA{0x38, 0x38, 0xC8, 0xFF} }

// embed replaces all of the file's pictures with the given ones.
func embed(t *testing.T, path string, pics ...audiometa.Artwork) {
	t.Helper()
	err := audiometa.Write(path, func(f *audiometa.File) error {
		f.RemoveAllArtwork()
		for _, p := range pics {
			if err := f.AddArtwork(p); err != nil {
				return fmt.Errorf("add artwork: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
}

func pic(data []byte, typ audiometa.ArtworkType) audiometa.Artwork {
	return audiometa.Artwork{MIMEType: "image/png", Type: typ, Data: data}
}

// album is what the database holds about the album of the track at rel.
type album struct {
	hash     []byte
	source   string
	warnings []string
}

func (l *artLib) albumOf(rel string) album {
	l.t.Helper()
	db, err := sql.Open("sqlite3", "file:"+l.dbPath+"?mode=ro")
	if err != nil {
		l.t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var a album
	var raw string
	err = db.QueryRowContext(l.t.Context(), `
		SELECT a.artwork_hash, coalesce(w.source, ''), t.scan_warnings
		FROM tracks t JOIN albums a ON a.id = t.album_id LEFT JOIN artwork w ON w.hash = a.artwork_hash
		WHERE t.rel_path = ?`, rel).Scan(&a.hash, &a.source, &raw)
	if err != nil {
		l.t.Fatalf("album of %q: %v", rel, err)
	}
	if err = json.Unmarshal([]byte(raw), &a.warnings); err != nil {
		l.t.Fatal(err)
	}
	return a
}

func (l *artLib) artworkRows() int {
	l.t.Helper()
	db, err := sql.Open("sqlite3", "file:"+l.dbPath+"?mode=ro")
	if err != nil {
		l.t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err = db.QueryRowContext(l.t.Context(), "SELECT count(*) FROM artwork").Scan(&n); err != nil {
		l.t.Fatal(err)
	}
	return n
}

func wantHash(t *testing.T, got, data []byte) {
	t.Helper()
	want := sha256.Sum256(data)
	if !bytes.Equal(got, want[:]) {
		t.Errorf("artwork hash = %x, want %x", got, want)
	}
}

func hasWarning(a album, parts ...string) bool {
	for _, w := range a.warnings {
		ok := true
		for _, p := range parts {
			ok = ok && strings.Contains(w, p)
		}
		if ok {
			return true
		}
	}
	return false
}

func TestEmbeddedArtworkIsStored(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "a.mp3", sampleMP3)
	stats := l.scan(Quick)
	if stats.Covers != 1 {
		t.Errorf("Covers = %d, want 1", stats.Covers)
	}
	a := l.albumOf("a.mp3")
	if a.hash == nil || a.source != embeddedIs {
		t.Fatalf("album = %+v, want an embedded cover", a)
	}
	if len(a.warnings) != 0 {
		t.Errorf("warnings = %q, want none", a.warnings)
	}
	if _, err := l.cache.Path([32]byte(a.hash), artwork.Size512); err != nil {
		t.Errorf("the cache has no rendition: %v", err)
	}
}

func TestFrontCoverBeatsOtherEmbeddedPictures(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "a.flac", sampleFLAC)
	other, front := testPNG(t, blue()), testPNG(t, red())
	embed(t, filepath.Join(root, "a.flac"), pic(other, audiometa.ArtworkOther), pic(front, audiometa.ArtworkFrontCover))
	l.scan(Quick)
	wantHash(t, l.albumOf("a.flac").hash, front)
}

func TestOtherEmbeddedPictureIsUsedWithoutAFrontCover(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "a.flac", sampleFLAC)
	other := testPNG(t, blue())
	embed(t, filepath.Join(root, "a.flac"), pic(other, audiometa.ArtworkOther))
	l.scan(Quick)
	wantHash(t, l.albumOf("a.flac").hash, other)
}

func TestEmbeddedBeatsFolder(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "a.flac", sampleFLAC)
	emb := testPNG(t, red())
	embed(t, filepath.Join(root, "a.flac"), pic(emb, audiometa.ArtworkFrontCover))
	l.write(t, root, coverJPG, testPNG(t, blue()))
	l.scan(Quick)
	a := l.albumOf("a.flac")
	wantHash(t, a.hash, emb)
	if a.source != embeddedIs {
		t.Errorf("source = %q, want embedded", a.source)
	}
}

func TestFolderArtwork(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		files map[string]byte // File name to color seed: 0 red, 1 blue.
		want  string          // The file whose image must win.
	}{
		{"cover", map[string]byte{coverJPG: 0}, coverJPG},
		{"case insensitive", map[string]byte{"Cover.PNG": 0}, "Cover.PNG"},
		{"folder", map[string]byte{"folder.png": 0}, "folder.png"},
		{"front", map[string]byte{"front.webp.png": 1, "front.jpeg": 0}, "front.jpeg"},
		{"album", map[string]byte{"album.png": 0}, "album.png"},
		{"name beats extension", map[string]byte{"folder.jpg": 1, coverPNG: 0}, coverPNG},
		{"extension order", map[string]byte{coverPNG: 1, coverJPG: 0}, coverJPG},
		{"other names are ignored", map[string]byte{"back.jpg": 1, "cover.txt": 1, "cover-1.jpg": 1}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := newArtLib(t)
			root := l.roots[0]
			l.copyTo(root, "a.m4a", sampleM4A)
			imgs := map[string][]byte{}
			for name, seed := range tt.files {
				c := color.Color(red())
				if seed == 1 {
					c = blue()
				}
				imgs[name] = testPNG(t, c)
				l.write(t, root, name, imgs[name])
			}
			stats := l.scan(Quick)
			a := l.albumOf("a.m4a")
			if tt.want == "" {
				if a.hash != nil || stats.Covers != 0 {
					t.Errorf("album = %+v, covers %d; want no cover", a, stats.Covers)
				}
				return
			}
			if a.source != folderIs {
				t.Errorf("source = %q, want folder", a.source)
			}
			wantHash(t, a.hash, imgs[tt.want])
		})
	}
}

func TestDiscFolderLooksInItsParent(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "Album/CD1/a.m4a", sampleM4A)
	l.copyTo(root, "Album/CD2/b.m4a", sampleM4A)
	parent := testPNG(t, red())
	l.write(t, root, "Album/"+coverJPG, parent)
	l.scan(Quick)
	wantHash(t, l.albumOf("Album/CD1/a.m4a").hash, parent)
	wantHash(t, l.albumOf("Album/CD2/b.m4a").hash, parent)

	// A cover in the disc folder itself wins over the parent's.
	l2 := newArtLib(t)
	l2.copyTo(l2.roots[0], "Album/CD1/a.m4a", sampleM4A)
	disc := testPNG(t, blue())
	l2.write(t, l2.roots[0], "Album/CD1/"+coverJPG, disc)
	l2.write(t, l2.roots[0], "Album/"+coverJPG, parent)
	l2.scan(Quick)
	wantHash(t, l2.albumOf("Album/CD1/a.m4a").hash, disc)
}

func TestParentOfAPlainFolderIsNotSearched(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "Artist/Album/a.m4a", sampleM4A)
	l.write(t, root, "Artist/"+coverJPG, testPNG(t, red()))
	l.scan(Quick)
	if a := l.albumOf("Artist/Album/a.m4a"); a.hash != nil {
		t.Errorf("album took the artist folder's image: %+v", a)
	}
}

// sameAlbum gives the files at rels (under root) one album.
func sameAlbum(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		retag(t, filepath.Join(root, rel), albumName)
	}
}

func TestOneCoverPerAlbum(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	for _, rel := range []string{"1.flac", "2.flac", "3.flac", "4.flac"} {
		l.copyTo(root, rel, sampleFLAC)
	}
	sameAlbum(t, root, "1.flac", "2.flac", "3.flac", "4.flac")
	if got := l.scan(Quick).Covers; got != 1 {
		t.Errorf("Covers = %d for one album of four tracks, want 1", got)
	}
	// Two albums, two covers.
	l.copyTo(root, "x/5.flac", sampleFLAC)
	retag(t, filepath.Join(root, "x", "5.flac"), "Second")
	if got := l.scan(Quick).Covers; got != 1 {
		t.Errorf("Covers = %d after a new album, want 1", got)
	}
}

// numbered gives the file at rel, under root, its own art and these numbers.
func numbered(t *testing.T, root, rel string, disc, track int, art []byte) {
	t.Helper()
	path := filepath.Join(root, rel)
	embed(t, path, pic(art, audiometa.ArtworkFrontCover))
	err := audiometa.Write(path, func(f *audiometa.File) error {
		f.Tags.DiscNumber, f.Tags.TrackNumber = disc, track
		return nil
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func TestCoverComesFromTheFirstTrack(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		first, other [2]int // Disc and track of a.flac, read first, and of b.flac.
		wantFirst    bool
	}{
		{"the file read first is first", [2]int{1, 1}, [2]int{1, 2}, true},
		{"the file read last is first", [2]int{1, 2}, [2]int{1, 1}, false},
		{"disc beats track", [2]int{2, 1}, [2]int{1, 9}, false},
		{"no disc comes after every disc", [2]int{0, 1}, [2]int{1, 2}, false},
		{"with no disc anywhere, it is one disc", [2]int{0, 1}, [2]int{0, 2}, true},
		{"no number comes last", [2]int{1, 0}, [2]int{1, 7}, false},
		{"a tie goes to the path", [2]int{1, 1}, [2]int{1, 1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := newArtLib(t)
			l.sequential()
			root := l.roots[0]
			l.copyTo(root, "a.flac", sampleFLAC)
			l.copyTo(root, "b.flac", sampleFLAC)
			sameAlbum(t, root, "a.flac", "b.flac")
			art, otherArt := testPNG(t, red()), testPNG(t, blue())
			numbered(t, root, "a.flac", tt.first[0], tt.first[1], art)
			numbered(t, root, "b.flac", tt.other[0], tt.other[1], otherArt)
			want := otherArt
			if tt.wantFirst {
				want = art
			}
			l.scan(Full)
			wantHash(t, l.albumOf("a.flac").hash, want)
		})
	}
}

// On an album with discs and a song with no disc number, the cover is the first
// listed song's: disc 1's track 2, not the untagged track 1, which a rule that
// counts no disc as disc 1 would put first.
func TestCoverOfAMixedDiscAlbumIsTheFirstListedSongs(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	l.sequential()
	root := l.roots[0]
	rels := []string{"untagged.flac", "disc-1.flac", "disc-2.flac"}
	for _, rel := range rels {
		l.copyTo(root, rel, sampleFLAC)
	}
	sameAlbum(t, root, rels...)
	untagged, disc1, disc2 := testPNG(t, red()), testPNG(t, blue()), testPNG(t, color.RGBA{0x10, 0x80, 0x40, 0xFF})
	numbered(t, root, rels[0], 0, 1, untagged)
	numbered(t, root, rels[1], 1, 2, disc1)
	numbered(t, root, rels[2], 2, 1, disc2)
	l.scan(Full)
	wantHash(t, l.albumOf(rels[0]).hash, disc1)
}

func TestCoverIsTheSameEveryFullScan(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	l.opts.Workers = 8
	l.scanner = NewScanner(l.st, l.opts)
	root := l.roots[0]
	const tracks = 8
	rels := make([]string, tracks)
	for i := range rels {
		rels[i] = fmt.Sprintf("%d.flac", i)
		l.copyTo(root, rels[i], sampleFLAC)
	}
	sameAlbum(t, root, rels...)
	want := testPNG(t, red())
	numbered(t, root, rels[tracks-1], 1, 1, want) // The last file read, by path.
	for i, rel := range rels[:tracks-1] {
		numbered(t, root, rel, 1, i+2, testPNG(t, color.RGBA{0x10, byte(i * 0x20), 0x40, 0xFF})) //nolint:gosec // i is below 8.
	}
	for range 5 {
		l.scan(Full)
		wantHash(t, l.albumOf(rels[0]).hash, want)
	}
	if got := l.artworkRows(); got != 1 {
		t.Errorf("%d artwork rows after the scans, want the one cover", got)
	}
}

func TestLateCoverDoesNotReplaceAnEarlierOne(t *testing.T) {
	t.Parallel()
	c := newCovers(nil, nil)
	track := func(n int) *store.TrackWrite {
		return &store.TrackWrite{TrackNo: n, RelPath: "a", Album: store.AlbumWrite{GroupKey: "g", Artwork: &store.ArtworkWrite{}}}
	}
	early, late := track(1), track(5)
	if !c.take("g", store.PlaceOf(late)) || !c.take("g", store.PlaceOf(early)) {
		t.Fatal("a track ahead of the album's best did not take it")
	}
	if c.take("g", store.PlaceOf(late)) {
		t.Error("a later track took the album from an earlier one")
	}
	c.settle(early) // The writer sees them in either order.
	c.settle(late)
	if early.Album.Artwork == nil || late.Album.Artwork != nil {
		t.Errorf("early cover %v, late cover %v; want only the early one kept", early.Album.Artwork, late.Album.Artwork)
	}
}

func TestTrackWithoutArtDoesNotCostItsAlbumTheCover(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	l.sequential()
	root := l.roots[0]
	l.copyTo(root, "a.m4a", sampleM4A) // Read first, has no art.
	l.copyTo(root, "b.flac", sampleFLAC)
	sameAlbum(t, root, "a.m4a", "b.flac")
	if got := l.scan(Quick).Covers; got != 1 {
		t.Errorf("Covers = %d, want 1", got)
	}
	if a := l.albumOf("a.m4a"); a.hash == nil {
		t.Error("the album lost its cover to a track without art")
	}
}

func TestCorruptCoverGivesWarningAndTrack(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "a.m4a", sampleM4A)
	l.write(t, root, coverJPG, garbage())
	stats := l.scan(Quick)
	if stats.Added != 1 || stats.Issues != 0 || stats.Covers != 0 {
		t.Errorf("stats = %+v, want the track added with no issue and no cover", stats)
	}
	a := l.albumOf("a.m4a")
	if a.hash != nil || !hasWarning(a, coverJPG) {
		t.Errorf("album = %+v, want no cover and a warning naming %s", a, coverJPG)
	}
}

func TestCorruptCandidateFallsThroughToTheNext(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "a.flac", sampleFLAC)
	embed(t, filepath.Join(root, "a.flac"), pic(garbage(), audiometa.ArtworkFrontCover))
	good := testPNG(t, blue())
	l.write(t, root, "folder.png", good)
	l.write(t, root, coverJPG, garbage())
	l.scan(Quick)
	a := l.albumOf("a.flac")
	wantHash(t, a.hash, good)
	if a.source != folderIs || len(a.warnings) != 2 || !hasWarning(a, "embedded") || !hasWarning(a, coverJPG) {
		t.Errorf("album = %+v, want the folder cover and two warnings", a)
	}
}

func TestTooLargeCoverGivesWarning(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "a.m4a", sampleM4A)
	l.write(t, root, coverJPG, pngHeaderOf(8001, 8001))
	l.scan(Quick)
	a := l.albumOf("a.m4a")
	if a.hash != nil || !hasWarning(a, coverJPG) {
		t.Errorf("album = %+v, want no cover and a warning", a)
	}
}

func TestDirectoryNamedCoverIsIgnored(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "a.m4a", sampleM4A)
	if err := os.Mkdir(filepath.Join(root, coverJPG), dirMode); err != nil {
		t.Fatal(err)
	}
	good := testPNG(t, red())
	l.write(t, root, "folder.png", good)
	stats := l.scan(Quick)
	a := l.albumOf("a.m4a")
	if stats.Added != 1 || len(a.warnings) != 0 {
		t.Errorf("stats = %+v, warnings = %q; want a plain add", stats, a.warnings)
	}
	wantHash(t, a.hash, good)
}

func TestNonRegularOrHugeCoverIsIgnored(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "a.m4a", sampleM4A)
	big, err := os.Create(filepath.Join(root, coverJPG))
	if err != nil {
		t.Fatal(err)
	}
	if err = big.Truncate(maxArtworkBytes + 1); err != nil { // Sparse.
		t.Fatal(err)
	}
	if err = big.Close(); err != nil {
		t.Fatal(err)
	}
	stats := l.scan(Quick)
	if a := l.albumOf("a.m4a"); a.hash != nil || stats.Covers != 0 || len(a.warnings) != 0 {
		t.Errorf("album = %+v, covers %d; want it ignored without a warning", a, stats.Covers)
	}
}

func TestSymlinkOutOfTheRootIsIgnored(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "a.m4a", sampleM4A)
	outside := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(outside, testPNG(t, red()), fileMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, coverPNG)); err != nil {
		t.Fatal(err)
	}
	stats := l.scan(Quick)
	if a := l.albumOf("a.m4a"); a.hash != nil || stats.Covers != 0 {
		t.Errorf("album = %+v; a symlink leaving the root was followed", a)
	}
}

func TestSymlinkInsideTheRootIsFollowed(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "a.m4a", sampleM4A)
	img := testPNG(t, red())
	l.write(t, root, "art/real.png", img)
	if err := os.Symlink(filepath.Join("art", "real.png"), filepath.Join(root, coverPNG)); err != nil {
		t.Fatal(err)
	}
	l.scan(Quick)
	wantHash(t, l.albumOf("a.m4a").hash, img)
}

func TestNonUTF8NamesKeepTheBatchValid(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	dir := "bad\xffdir"
	l.copyTo(root, dir+"/a.m4a", sampleM4A)
	l.write(t, root, dir+"/"+coverJPG, garbage())
	stats := l.scan(Quick)
	if stats.Added != 1 {
		t.Fatalf("stats = %+v, want the track added", stats)
	}
	a := l.albumOf(dir + "/a.m4a")
	if !hasWarning(a, coverJPG) {
		t.Fatalf("warnings = %q, want one naming %s", a.warnings, coverJPG)
	}
	for _, w := range a.warnings {
		if !utf8.ValidString(w) {
			t.Errorf("warning %q is not valid UTF-8", w)
		}
	}
}

func TestReindexKeepsTheCover(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, aFLAC, sampleFLAC)
	path := filepath.Join(root, aFLAC)
	embed(t, path, pic(testPNG(t, red()), audiometa.ArtworkFrontCover))
	l.scan(Quick)
	before := l.albumOf(aFLAC)
	embed(t, path, pic(testPNG(t, blue()), audiometa.ArtworkFrontCover))
	if err := l.scanner.reindex(t.Context(), l.states(root)[aFLAC].TrackID, root, aFLAC); err != nil {
		t.Fatal(err)
	}
	if got := l.albumOf(aFLAC); !bytes.Equal(got.hash, before.hash) {
		t.Errorf("artwork hash = %x, want %x: an edit must not touch the cover", got.hash, before.hash)
	}
}

func TestNoArtGivesNil(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	l.copyTo(l.roots[0], "a.m4a", sampleM4A)
	stats := l.scan(Quick)
	if a := l.albumOf("a.m4a"); a.hash != nil || stats.Covers != 0 || l.artworkRows() != 0 {
		t.Errorf("album = %+v, covers %d; want no cover and no rows", a, stats.Covers)
	}
}

func TestNilCacheSkipsArtwork(t *testing.T) {
	t.Parallel()
	l := &artLib{testLib: newTestLib(t, 1)}
	l.copyTo(l.roots[0], "a.mp3", sampleMP3)
	stats := l.scan(Quick)
	if a := l.albumOf("a.mp3"); a.hash != nil || stats.Covers != 0 {
		t.Errorf("album = %+v, covers %d; want art skipped without a cache", a, stats.Covers)
	}
}

// safeBuffer is a log sink that survives concurrent writers.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, _ = b.buf.Write(p) // A bytes.Buffer never fails.
	return len(p), nil
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestCacheFailureLogsAndKeepsTheTrack(t *testing.T) {
	t.Parallel()
	l := &artLib{testLib: newTestLib(t, 1)}
	// A cache directory that is a file: every write fails with a plain I/O error.
	blocked := filepath.Join(t.TempDir(), "art")
	if err := os.WriteFile(blocked, nil, fileMode); err != nil {
		t.Fatal(err)
	}
	var logs safeBuffer
	l.opts.Artwork = artwork.NewCache(blocked)
	l.opts.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	l.scanner = NewScanner(l.st, l.opts)
	l.copyTo(l.roots[0], "a.mp3", sampleMP3)
	stats := l.scan(Quick)
	if stats.Added != 1 || stats.Issues != 0 || stats.Covers != 0 {
		t.Errorf("stats = %+v, want the track added and no cover", stats)
	}
	if a := l.albumOf("a.mp3"); a.hash != nil {
		t.Errorf("album = %+v, want no cover", a)
	}
	if !strings.Contains(logs.String(), `"level":"WARN"`) {
		t.Errorf("logs = %s, want a warning", logs.String())
	}
}

func TestPrunedAlbumsLoseTheirFiles(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	l.copyTo(root, "keep.m4a", sampleM4A)
	l.copyTo(root, "gone.mp3", sampleMP3)
	retag(t, filepath.Join(root, "gone.mp3"), otherName)
	l.scan(Quick)
	hash := [32]byte(l.albumOf("gone.mp3").hash)
	if _, err := l.cache.Path(hash, artwork.Size512); err != nil {
		t.Fatal(err)
	}
	remove(t, root, "gone.mp3")
	l.scan(Quick)
	l.clock.Advance(testGrace + time.Minute)
	stats := l.scan(Quick)
	if len(stats.Pruned.Artwork) != 1 || stats.Pruned.Artwork[0] != hash {
		t.Fatalf("Pruned = %+v, want the album's image", stats.Pruned)
	}
	if _, err := l.cache.Path(hash, artwork.Size512); err == nil {
		t.Error("the pruned album's renditions are still in the cache")
	}
}

func TestSharedImageSurvivesOnePrunedAlbum(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	root := l.roots[0]
	img := testPNG(t, red())
	l.copyTo(root, "a/one.m4a", sampleM4A)
	l.copyTo(root, "b/two.m4a", sampleM4A)
	retag(t, filepath.Join(root, "b", "two.m4a"), otherName)
	l.write(t, root, "a/"+coverJPG, img)
	l.write(t, root, "b/"+coverJPG, img)
	l.scan(Quick)
	hash := [32]byte(l.albumOf("a/one.m4a").hash)
	remove(t, root, "b/two.m4a")
	l.scan(Quick)
	l.clock.Advance(testGrace + time.Minute)
	l.scan(Quick)
	if _, err := l.cache.Path(hash, artwork.Size512); err != nil {
		t.Errorf("an image another album uses was deleted: %v", err)
	}
}

// pngHeaderOf is a PNG that is only a header claiming w x h pixels.
func pngHeaderOf(w, h uint32) []byte {
	var ihdr bytes.Buffer
	for _, v := range []uint32{w, h} {
		ihdr.Write([]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}) //nolint:gosec // Big-endian bytes.
	}
	ihdr.Write([]byte{8, 6, 0, 0, 0})
	chunk := append([]byte("IHDR"), ihdr.Bytes()...)
	out := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0d")
	out = append(out, chunk...)
	sum := crc32.ChecksumIEEE(chunk)
	return append(out, byte(sum>>24), byte(sum>>16), byte(sum>>8), byte(sum)) //nolint:gosec // Big-endian bytes.
}
