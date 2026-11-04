package artwork_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io/fs"
	"math"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/simonhull/kitsune/internal/artwork"
)

// noisy returns a w x h image of deterministic varied pixels, so scaling has
// something to chew on.
func noisy(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Pix[y*img.Stride+x*4+0] = uint8(x * 7 / 3)
			img.Pix[y*img.Stride+x*4+1] = uint8(y * 5 / 3)
			img.Pix[y*img.Stride+x*4+2] = uint8((x ^ y) * 3 & 0xFF)
			img.Pix[y*img.Stride+x*4+3] = 0xFF
		}
	}
	return img
}

func solid(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// dims decodes the rendition at the path and returns its size.
func dims(t *testing.T, path string) [2]int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" {
		t.Fatalf("rendition is %s, want jpeg", format)
	}
	return [2]int{cfg.Width, cfg.Height}
}

// putResult is a stored image and the pixel size of each of its rungs.
type putResult struct {
	img  *artwork.Image
	dims map[int][2]int
}

// rungs puts the image and returns the dimensions of each rung present, keyed
// by the size asked of Path with exact matching.
func rungs(t *testing.T, c *artwork.Cache, data []byte) putResult {
	t.Helper()
	img, err := c.Put(t.Context(), data)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	got := make(map[int][2]int)
	for _, size := range []int{artwork.Size128, artwork.Size512, artwork.Size1024} {
		p := mustPath(t, c, img.Hash, size)
		d := dims(t, p)
		// Path falls back to a smaller rung when this one is absent; tell them
		// apart by the file name.
		if want := hashSuffix(img.Hash, size); !strings.HasSuffix(p, want) {
			continue
		}
		got[size] = d
	}
	return putResult{img: img, dims: got}
}

func mustPath(t *testing.T, c *artwork.Cache, hash [32]byte, size int) string {
	t.Helper()
	p, err := c.Path(hash, size)
	if err != nil {
		t.Fatalf("Path(%d): %v", size, err)
	}
	return p
}

func mustStat(t *testing.T, path string) fs.FileInfo {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func hashSuffix(h [32]byte, size int) string {
	const hexdigits = "0123456789abcdef"
	b := make([]byte, 0, 64)
	for _, v := range h {
		b = append(b, hexdigits[v>>4], hexdigits[v&0xF])
	}
	return string(b) + "-" + itoa(size) + ".jpg"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var s []byte
	for ; n > 0; n /= 10 {
		s = append([]byte{byte('0' + n%10)}, s...)
	}
	return string(s)
}

func TestLadder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		w, h int
		want map[int][2]int
	}{
		{"large", 3000, 2000, map[int][2]int{128: {128, 85}, 512: {512, 341}, 1024: {1024, 683}}},
		{"large portrait", 1500, 3000, map[int][2]int{128: {64, 128}, 512: {256, 512}, 1024: {512, 1024}}},
		{"small", 100, 80, map[int][2]int{128: {100, 80}}},
		{"between rungs", 600, 600, map[int][2]int{128: {128, 128}, 512: {512, 512}, 1024: {600, 600}}},
		{"exactly 1024", 1024, 768, map[int][2]int{128: {128, 96}, 512: {512, 384}, 1024: {1024, 768}}},
		{"exactly 512", 512, 512, map[int][2]int{128: {128, 128}, 512: {512, 512}}},
		{"exactly 128", 128, 128, map[int][2]int{128: {128, 128}}},
		{"tiny", 16, 16, map[int][2]int{128: {16, 16}}},
		{"thin", 2000, 3, map[int][2]int{128: {128, 1}, 512: {512, 1}, 1024: {1024, 2}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := artwork.NewCache(t.TempDir())
			res := rungs(t, c, encodePNG(t, noisy(tt.w, tt.h)))
			img := res.img
			if len(res.dims) != len(tt.want) {
				t.Errorf("rungs = %v, want %v", res.dims, tt.want)
			}
			for size, want := range tt.want {
				if res.dims[size] != want {
					t.Errorf("rung %d = %v, want %v", size, res.dims[size], want)
				}
			}
			if img.Width != tt.w || img.Height != tt.h {
				t.Errorf("source size = %dx%d, want %dx%d", img.Width, img.Height, tt.w, tt.h)
			}
		})
	}
}

func TestPutHash(t *testing.T) {
	t.Parallel()
	data := encodePNG(t, noisy(40, 40))
	img, err := artwork.NewCache(t.TempDir()).Put(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	if img.Hash != sha256.Sum256(data) {
		t.Error("Hash is not the SHA-256 of the source bytes")
	}
}

func TestPathSelection(t *testing.T) {
	t.Parallel()
	c := artwork.NewCache(t.TempDir())
	// 600 px: rungs 128, 512 and 1024 (at 600).
	img, err := c.Put(t.Context(), encodePNG(t, noisy(600, 600)))
	if err != nil {
		t.Fatal(err)
	}
	small, err := c.Put(t.Context(), encodePNG(t, noisy(100, 100)))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		hash [32]byte
		size int
		want int // Rung in the file name.
	}{
		{img.Hash, 1024, 1024},
		{img.Hash, 700, 512},
		{img.Hash, 512, 512},
		{img.Hash, 130, 128},
		{img.Hash, 128, 128},
		{img.Hash, 64, 128}, // Below the ladder: the smallest that exists.
		{small.Hash, 1024, 128},
		{small.Hash, 512, 128},
		{small.Hash, 64, 128},
	}
	for _, tt := range tests {
		p := mustPath(t, c, tt.hash, tt.size)
		if want := hashSuffix(tt.hash, tt.want); !strings.HasSuffix(p, want) {
			t.Errorf("Path(%d) = %s, want rung %d", tt.size, p, tt.want)
		}
	}
	if _, err = c.Path([32]byte{1}, artwork.Size512); !errors.Is(err, artwork.ErrNotFound) {
		t.Errorf("Path of unknown hash = %v, want ErrNotFound", err)
	}
}

func TestAlphaIsFlattenedOntoGround(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 300, 300)) // Fully transparent.
	c := artwork.NewCache(t.TempDir())
	got, err := c.Put(t.Context(), encodePNG(t, img))
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{artwork.Size128, artwork.Size512} {
		dec := decodeJPEG(t, mustPath(t, c, got.Hash, size))
		r, g, b, _ := dec.At(dec.Bounds().Dx()/2, dec.Bounds().Dy()/2).RGBA()
		near(t, "flattened pixel", [3]int{int(r >> 8), int(g >> 8), int(b >> 8)}, [3]int{0x0A, 0x09, 0x08}, 4)
	}
}

func decodeJPEG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func near(t *testing.T, what string, got, want [3]int, tol int) {
	t.Helper()
	for i := range got {
		if d := got[i] - want[i]; d > tol || d < -tol {
			t.Errorf("%s = %v, want %v ±%d", what, got, want, tol)
			return
		}
	}
}

func TestBadImages(t *testing.T) {
	t.Parallel()
	good := encodePNG(t, noisy(200, 200))
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"garbage", []byte("this is not an image at all")},
		{"truncated png", good[:len(good)/2]},
		{"truncated jpeg", encodeJPEG(t, noisy(200, 200))[:400]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			_, err := artwork.NewCache(dir).Put(t.Context(), tt.data)
			if !errors.Is(err, artwork.ErrBadImage) {
				t.Errorf("Put = %v, want ErrBadImage", err)
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Errorf("a bad image left %d entries in the cache", len(entries))
			}
		})
	}
}

// pngHeader is a PNG that is only a signature and an IHDR claiming w x h:
// decoding it would need gigabytes, reading its header needs nothing.
func pngHeader(w, h uint32) []byte {
	var ihdr bytes.Buffer
	_ = binary.Write(&ihdr, binary.BigEndian, w)
	_ = binary.Write(&ihdr, binary.BigEndian, h)
	ihdr.Write([]byte{8, 6, 0, 0, 0}) // 8-bit RGBA.
	chunk := append([]byte("IHDR"), ihdr.Bytes()...)
	var out bytes.Buffer
	out.WriteString("\x89PNG\r\n\x1a\n")
	_ = binary.Write(&out, binary.BigEndian, uint32(ihdr.Len())) //nolint:gosec // Thirteen.
	out.Write(chunk)
	_ = binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return out.Bytes()
}

func TestTooLarge(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := artwork.NewCache(dir)
	_, err := c.Put(t.Context(), pngHeader(8001, 8001)) // 64.016 MP.
	if !errors.Is(err, artwork.ErrTooLarge) {
		t.Errorf("Put = %v, want ErrTooLarge", err)
	}
	if errors.Is(err, artwork.ErrBadImage) {
		t.Error("too large must not also be a bad image")
	}
	// Just inside the limit reaches the decoder, which finds no pixel data.
	_, err = c.Put(t.Context(), pngHeader(8000, 8000))
	if !errors.Is(err, artwork.ErrBadImage) {
		t.Errorf("Put at the limit = %v, want ErrBadImage (header only)", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a refused image left %d entries in the cache", len(entries))
	}
}

func TestWebP(t *testing.T) {
	t.Parallel()
	// A 1 x 1 lossless WebP.
	data, err := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	if err != nil {
		t.Fatal(err)
	}
	img, err := artwork.NewCache(t.TempDir()).Put(t.Context(), data)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if img.Width != 1 || img.Height != 1 {
		t.Errorf("size = %dx%d, want 1x1", img.Width, img.Height)
	}
}

func TestPutIsIdempotent(t *testing.T) {
	t.Parallel()
	c := artwork.NewCache(t.TempDir())
	data := encodePNG(t, noisy(700, 500))
	first, err := c.Put(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	sizes := []int{artwork.Size128, artwork.Size512, artwork.Size1024}
	before := make(map[int]fs.FileInfo)
	for _, size := range sizes {
		before[size] = mustStat(t, mustPath(t, c, first.Hash, size))
	}
	again, err := c.Put(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	if *again != *first {
		t.Errorf("second Put = %+v, want %+v", *again, *first)
	}
	for _, size := range sizes {
		if !os.SameFile(before[size], mustStat(t, mustPath(t, c, first.Hash, size))) {
			t.Errorf("rung %d was rewritten by an idempotent Put", size)
		}
	}
}

func TestMissingRungRerendersAll(t *testing.T) {
	t.Parallel()
	c := artwork.NewCache(t.TempDir())
	data := encodePNG(t, noisy(700, 500))
	first, err := c.Put(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	sizes := []int{artwork.Size128, artwork.Size512, artwork.Size1024}
	before := make(map[int]fs.FileInfo)
	for _, size := range sizes {
		before[size] = mustStat(t, mustPath(t, c, first.Hash, size))
	}
	if err = os.Remove(mustPath(t, c, first.Hash, artwork.Size512)); err != nil {
		t.Fatal(err)
	}
	healed, err := c.Put(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	if *healed != *first {
		t.Errorf("Put after a loss = %+v, want %+v", *healed, *first)
	}
	for _, size := range sizes {
		if os.SameFile(before[size], mustStat(t, mustPath(t, c, first.Hash, size))) {
			t.Errorf("rung %d was not re-rendered", size)
		}
	}
}

func TestZeroSizeRungCountsAsMissing(t *testing.T) {
	t.Parallel()
	c := artwork.NewCache(t.TempDir())
	data := encodePNG(t, noisy(300, 300))
	img, err := c.Put(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	p := mustPath(t, c, img.Hash, artwork.Size128)
	if err = os.Truncate(p, 0); err != nil {
		t.Fatal(err)
	}
	if got, perr := c.Path(img.Hash, artwork.Size128); perr == nil && got == p {
		t.Error("Path returned a zero-size file")
	}
	again, err := c.Put(t.Context(), data)
	if err != nil {
		t.Fatalf("Put over a zero-size rung: %v", err)
	}
	if *again != *img {
		t.Errorf("Put = %+v, want %+v", *again, *img)
	}
	if mustStat(t, p).Size() == 0 {
		t.Error("rung not repaired")
	}
}

func TestUndecodableSmallRungIsRerendered(t *testing.T) {
	t.Parallel()
	c := artwork.NewCache(t.TempDir())
	data := encodePNG(t, noisy(300, 300))
	img, err := c.Put(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	p := mustPath(t, c, img.Hash, artwork.Size128)
	if err = os.WriteFile(p, []byte("not a jpeg"), 0o600); err != nil {
		t.Fatal(err)
	}
	again, err := c.Put(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	if *again != *img {
		t.Errorf("Put = %+v, want %+v", *again, *img)
	}
}

func TestConcurrentPut(t *testing.T) {
	t.Parallel()
	c := artwork.NewCache(t.TempDir())
	data := encodePNG(t, noisy(1100, 900))
	const n = 8
	results := make([]*artwork.Image, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { results[i], errs[i] = c.Put(t.Context(), data) })
	}
	wg.Wait()
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("Put %d: %v", i, errs[i])
		}
		if *results[i] != *results[0] {
			t.Errorf("Put %d = %+v, want %+v", i, *results[i], *results[0])
		}
	}
	for _, size := range []int{artwork.Size128, artwork.Size512, artwork.Size1024} {
		dims(t, mustPath(t, c, results[0].Hash, size))
	}
}

func TestPutHonorsContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	dir := t.TempDir()
	_, err := artwork.NewCache(dir).Put(ctx, encodePNG(t, noisy(300, 300)))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Put = %v, want context.Canceled", err)
	}
	if errors.Is(err, artwork.ErrBadImage) {
		t.Error("a canceled Put must not look like a bad image")
	}
}

func TestNoTempFilesLeft(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := artwork.NewCache(dir)
	if _, err := c.Put(t.Context(), encodePNG(t, noisy(800, 800))); err != nil {
		t.Fatal(err)
	}
	var files []string
	err := fs.WalkDir(os.DirFS(dir), ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Errorf("cache holds %v, want exactly the three rungs", files)
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()
	c := artwork.NewCache(t.TempDir())
	img, err := c.Put(t.Context(), encodePNG(t, noisy(700, 700)))
	if err != nil {
		t.Fatal(err)
	}
	other, err := c.Put(t.Context(), encodePNG(t, noisy(710, 710)))
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Delete(img.Hash); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	for _, size := range []int{artwork.Size128, artwork.Size512, artwork.Size1024} {
		if _, err = c.Path(img.Hash, size); !errors.Is(err, artwork.ErrNotFound) {
			t.Errorf("Path(%d) after Delete = %v, want ErrNotFound", size, err)
		}
	}
	if _, err = c.Path(other.Hash, artwork.Size512); err != nil {
		t.Errorf("Delete removed another image: %v", err)
	}
	if err = c.Delete(img.Hash); err != nil {
		t.Errorf("second Delete = %v, want nil", err)
	}
	if err = c.Delete([32]byte{9}); err != nil {
		t.Errorf("Delete of an unknown hash = %v, want nil", err)
	}
}

func TestColors(t *testing.T) {
	t.Parallel()
	gray := color.RGBA{0x78, 0x78, 0x88, 0xFF} // Desaturated.
	green := color.RGBA{0x28, 0xC8, 0x48, 0xFF}
	two := solid(400, 400, gray)
	draw.Draw(two, image.Rect(0, 0, 400, 120), image.NewUniform(green), image.Point{}, draw.Src) // 30%.
	tiny := solid(400, 400, gray)
	draw.Draw(tiny, image.Rect(0, 0, 400, 4), image.NewUniform(green), image.Point{}, draw.Src) // 1%.
	dark := color.RGBA{0x18, 0x28, 0x38, 0xFF}

	tests := []struct {
		name           string
		img            image.Image
		accent, ground [3]int
	}{
		{"vivid", solid(300, 300, color.RGBA{0xC8, 0x38, 0x38, 0xFF}), [3]int{0xC8, 0x38, 0x38}, [3]int{0x74, 0x1F, 0x1F}},
		{"gray falls back to fox", solid(300, 300, gray), [3]int{0xF2, 0x8C, 0x28}, [3]int{0x6C, 0x6C, 0x7B}},
		{"accent is not the biggest bucket", two, [3]int{0x28, 0xC8, 0x48}, [3]int{0x6C, 0x6C, 0x7B}},
		{"under 2 percent is no accent", tiny, [3]int{0xF2, 0x8C, 0x28}, [3]int{0x6C, 0x6C, 0x7B}},
		{"dark ground is unchanged", solid(300, 300, dark), [3]int{0xF2, 0x8C, 0x28}, [3]int{0x18, 0x28, 0x38}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := artwork.NewCache(t.TempDir())
			data := encodePNG(t, tt.img)
			img, err := c.Put(t.Context(), data)
			if err != nil {
				t.Fatal(err)
			}
			near(t, "accent", parseHex(t, img.Accent), tt.accent, 2)
			ground := parseHex(t, img.Ground)
			if tt.name == "dark ground is unchanged" {
				near(t, "ground", ground, tt.ground, 2)
			} else if cr := contrast(ground); cr < 4.4 {
				t.Errorf("ground %s has contrast %.2f with the text color, want >= 4.5", img.Ground, cr)
			}
			// The colors are a function of the stored file, so a repeat Put
			// (the idempotent path) agrees exactly.
			again, err := c.Put(t.Context(), data)
			if err != nil {
				t.Fatal(err)
			}
			if again.Accent != img.Accent || again.Ground != img.Ground {
				t.Errorf("idempotent colors %s/%s differ from %s/%s", again.Accent, again.Ground, img.Accent, img.Ground)
			}
		})
	}
}

// TestAccentGathersShades covers the common real cover: one color spread over
// many shades, none of which alone reaches 2% of the pixels.
func TestAccentGathersShades(t *testing.T) {
	t.Parallel()
	img := solid(400, 400, color.RGBA{0x78, 0x78, 0x88, 0xFF})
	for x := range 160 { // 40% of the image, a band of reds from dark to bright.
		shade := color.RGBA{uint8(0x70 + x), 0x20, 0x18, 0xFF}
		draw.Draw(img, image.Rect(x, 0, x+1, 400), image.NewUniform(shade), image.Point{}, draw.Src)
	}
	got, err := artwork.NewCache(t.TempDir()).Put(t.Context(), encodePNG(t, img))
	if err != nil {
		t.Fatal(err)
	}
	accent := parseHex(t, got.Accent)
	if got.Accent == "#f28c28" || accent[0] < 2*accent[1] || accent[0] < 2*accent[2] {
		t.Errorf("accent %s, want the mean of the reds", got.Accent)
	}
}

// TestAccentTakesTheLargerHue checks that hue bins compete by size, and that a
// vivid area under 2% loses however saturated it is.
func TestAccentTakesTheLargerHue(t *testing.T) {
	t.Parallel()
	img := solid(400, 400, color.RGBA{0x78, 0x78, 0x88, 0xFF})
	draw.Draw(img, image.Rect(0, 0, 400, 40), image.NewUniform(color.RGBA{0x28, 0x48, 0xD8, 0xFF}), image.Point{}, draw.Src)   // Blue, 10%.
	draw.Draw(img, image.Rect(0, 40, 400, 160), image.NewUniform(color.RGBA{0x28, 0xC8, 0x48, 0xFF}), image.Point{}, draw.Src) // Green, 30%.
	got, err := artwork.NewCache(t.TempDir()).Put(t.Context(), encodePNG(t, img))
	if err != nil {
		t.Fatal(err)
	}
	near(t, "accent", parseHex(t, got.Accent), [3]int{0x28, 0xC8, 0x48}, 6)
}

func TestGroundKeepsHueOfLightCover(t *testing.T) {
	t.Parallel()
	img, err := artwork.NewCache(t.TempDir()).Put(t.Context(), encodePNG(t, solid(300, 300, color.RGBA{0xE8, 0xD8, 0x78, 0xFF})))
	if err != nil {
		t.Fatal(err)
	}
	g := parseHex(t, img.Ground)
	if cr := contrast(g); cr < 4.4 {
		t.Errorf("ground %s contrast %.2f, want >= 4.5", img.Ground, cr)
	}
	// Still yellowish: red and green well above blue.
	if g[0] < g[2]+20 || g[1] < g[2]+20 {
		t.Errorf("ground %s lost its hue", img.Ground)
	}
}

func parseHex(t *testing.T, s string) [3]int {
	t.Helper()
	if len(s) != 7 || s[0] != '#' {
		t.Fatalf("color %q is not #rrggbb", s)
	}
	var out [3]int
	for i := range out {
		for _, c := range s[1+2*i : 3+2*i] {
			var v int
			switch {
			case c >= '0' && c <= '9':
				v = int(c - '0')
			case c >= 'a' && c <= 'f':
				v = int(c-'a') + 10
			default:
				t.Fatalf("color %q is not lowercase hex", s)
			}
			out[i] = out[i]*16 + v
		}
	}
	return out
}

// contrast is the WCAG contrast of c against #F5F1EC.
func contrast(c [3]int) float64 {
	lum := func(c [3]int) float64 {
		f := func(v int) float64 {
			x := float64(v) / 255
			if x <= 0.03928 {
				return x / 12.92
			}
			return pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*f(c[0]) + 0.7152*f(c[1]) + 0.0722*f(c[2])
	}
	a, b := lum(c), lum([3]int{0xF5, 0xF1, 0xEC})
	if a > b {
		a, b = b, a
	}
	return (b + 0.05) / (a + 0.05)
}

func pow(x, y float64) float64 { return math.Pow(x, y) }
