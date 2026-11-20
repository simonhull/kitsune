package artwork_test

import (
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/artwork"
)

// Rendition files, as files lists them.
const (
	j128  = "128.jpg"
	j512  = "512.jpg"
	j640  = "640.jpg"
	j400  = "400.jpg"
	j800  = "800.jpg"
	j1024 = "1024.jpg"
	j1920 = "1920.jpg"
	j3840 = "3840.jpg"
	p128  = "128.png"
	p400  = "400.png"
	p800  = "800.png"
)

// logo returns a w x h image transparent but for an opaque red band across its
// middle third, as a clear logo is.
func logo(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := h / 3; y < 2*h/3; y++ {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{R: 0xD0, G: 0x20, B: 0x20, A: 0xFF})
		}
	}
	return img
}

// files lists the rendition files of hash in the cache at dir, as "<rung><ext>".
func files(t *testing.T, dir string, hash [32]byte) []string {
	t.Helper()
	name := hex.EncodeToString(hash[:])
	entries, err := os.ReadDir(filepath.Join(dir, name[:2]))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if rest, ok := strings.CutPrefix(e.Name(), name+"-"); ok {
			out = append(out, rest)
		}
	}
	slices.Sort(out)
	return out
}

func TestKindLadders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		kind artwork.Kind
		w, h int
		want []string // Rendition files, sorted.
	}{
		{"a 4K background", artwork.Background, 3840, 2160, []string{j128, j1920, j3840, j640}},
		{"a 1080p background never upscaled", artwork.Background, 1920, 1080, []string{j128, j1920, j640}},
		{"a small background at its own size", artwork.Background, 1000, 562, []string{j128, j1920, j640}},
		{"a banner", artwork.Banner, 1000, 185, []string{j128, j1920, j640}},
		{"a thumb", artwork.Thumb, 1000, 1000, []string{j1024, j128, j512}},
		{"a back cover", artwork.Back, 1400, 1400, []string{j1024, j128, j512}},
		{"an opaque disc", artwork.Disc, 1000, 1000, []string{j128, j400, j800}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			c := artwork.NewCache(dir)
			img, err := c.Put(t.Context(), tt.kind, encodeJPEG(t, noisy(tt.w, tt.h)))
			if err != nil {
				t.Fatal(err)
			}
			if got := files(t, dir, img.Hash); !slices.Equal(got, tt.want) {
				t.Errorf("renditions = %v, want %v", got, tt.want)
			}
			if img.Alpha {
				t.Error("an opaque image says it has alpha")
			}
			top := tt.kind.Ladder()[len(tt.kind.Ladder())-1]
			if d := dims(t, mustPath(t, c, img.Hash, top)); max(d[0], d[1]) > max(tt.w, tt.h) {
				t.Errorf("largest rendition %v is bigger than its %dx%d source", d, tt.w, tt.h)
			}
		})
	}
}

func TestAlphaKeptAsPNG(t *testing.T) {
	t.Parallel()
	for _, kind := range []artwork.Kind{artwork.Logo, artwork.Disc} {
		t.Run(kind.String(), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			c := artwork.NewCache(dir)
			img, err := c.Put(t.Context(), kind, encodePNG(t, logo(800, 310)))
			if err != nil {
				t.Fatal(err)
			}
			if !img.Alpha {
				t.Fatal("a transparent logo was flattened")
			}
			if got, want := files(t, dir, img.Hash), []string{p128, p400, p800}; !slices.Equal(got, want) {
				t.Errorf("renditions = %v, want %v", got, want)
			}
			p := mustPath(t, c, img.Hash, artwork.Size400)
			if !artwork.IsPNG(p) {
				t.Fatalf("Path = %s, want a PNG", p)
			}
			f, err := os.Open(p)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			dec, err := png.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, _, a := dec.At(5, 5).RGBA(); a != 0 {
				t.Errorf("a transparent corner has alpha %#x, want 0", a)
			}
			if _, _, _, a := dec.At(dec.Bounds().Dx()/2, dec.Bounds().Dy()/2).RGBA(); a != 0xFFFF {
				t.Errorf("the opaque band has alpha %#x, want opaque", a)
			}
			// The colors are the band's, not the transparency's.
			near(t, "accent", parseHex(t, img.Accent), [3]int{0xD0, 0x20, 0x20}, 12)
		})
	}
}

// A cover with transparency is still flattened: only logos and discs keep it.
func TestOnlyLogosAndDiscsKeepAlpha(t *testing.T) {
	t.Parallel()
	for _, kind := range []artwork.Kind{artwork.Cover, artwork.Thumb, artwork.Back, artwork.Background, artwork.Banner} {
		img, err := artwork.NewCache(t.TempDir()).Put(t.Context(), kind, encodePNG(t, logo(700, 400)))
		if err != nil {
			t.Fatal(err)
		}
		if img.Alpha {
			t.Errorf("%s kept alpha", kind)
		}
	}
}

// The colors come from the stored 128 rendition, so a fresh render and an
// idempotent Put agree, for a PNG ladder as for a JPEG one.
func TestColorsAgreeOnBothPaths(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		kind artwork.Kind
		data func(t *testing.T) []byte
	}{
		{artwork.Logo, func(t *testing.T) []byte { t.Helper(); return encodePNG(t, logo(800, 310)) }},
		{artwork.Background, func(t *testing.T) []byte { t.Helper(); return encodeJPEG(t, noisy(1920, 1080)) }},
	} {
		t.Run(tt.kind.String(), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			data := tt.data(t)
			first, err := artwork.NewCache(dir).Put(t.Context(), tt.kind, data)
			if err != nil {
				t.Fatal(err)
			}
			again, err := artwork.NewCache(dir).Put(t.Context(), tt.kind, data)
			if err != nil {
				t.Fatal(err)
			}
			if *first != *again {
				t.Errorf("fresh %+v, again %+v", *first, *again)
			}
		})
	}
}

// One image put as two kinds keeps both ladders, Path finds the nearest rung
// of either, and Delete takes them all.
func TestKindsShareAnImage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := artwork.NewCache(dir)
	data := encodePNG(t, logo(3840, 2160))
	if _, err := c.Put(t.Context(), artwork.Background, data); err != nil {
		t.Fatal(err)
	}
	img, err := c.Put(t.Context(), artwork.Logo, data)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		size int
		want string
	}{
		{artwork.Size3840, j3840},
		{artwork.Size1024, p800},
		{artwork.Size512, p400},
		{artwork.Size640, j640},
		{artwork.Size128, j128}, // Both have one; JPEG first.
		{64, j128},
	} {
		if p := mustPath(t, c, img.Hash, tt.size); !strings.HasSuffix(p, "-"+tt.want) {
			t.Errorf("Path(%d) = %s, want the %s", tt.size, filepath.Base(p), tt.want)
		}
	}
	if err = c.Delete(img.Hash); err != nil {
		t.Fatal(err)
	}
	if got := files(t, dir, img.Hash); len(got) != 0 {
		t.Errorf("Delete left %v", got)
	}
	if _, err = c.Path(img.Hash, artwork.Size512); !errors.Is(err, artwork.ErrNotFound) {
		t.Errorf("Path after Delete = %v, want ErrNotFound", err)
	}
}

func TestSizesHoldEveryLadder(t *testing.T) {
	t.Parallel()
	for k := artwork.Cover; k <= artwork.Disc; k++ {
		for _, rung := range k.Ladder() {
			if !slices.Contains(artwork.Sizes(), rung) {
				t.Errorf("%s's rung %d is not in Sizes", k, rung)
			}
		}
		if k.Ladder()[0] != artwork.Size128 {
			t.Errorf("%s's ladder starts at %d, want 128", k, k.Ladder()[0])
		}
	}
	if !slices.IsSorted(artwork.Sizes()) {
		t.Error("Sizes is not sorted")
	}
	if got := artwork.Kind(99).String(); got != "Kind("+strconv.Itoa(99)+")" {
		t.Errorf("unknown kind = %q", got)
	}
}
