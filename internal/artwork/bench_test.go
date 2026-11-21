package artwork_test

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/simonhull/kitsune/internal/artwork"
)

// benchSide is the side of the benchmark cover: a typical large embedded scan.
const benchSide = 3000

// noisyJPEG returns a benchSide x benchSide JPEG of random pixels, the worst
// case for both decoding and scaling.
func noisyJPEG(b *testing.B) []byte {
	b.Helper()
	img := image.NewRGBA(image.Rect(0, 0, benchSide, benchSide))
	state := uint32(1) // A fixed LCG: the same pixels every run.
	for i := 0; i < len(img.Pix); i += 4 {
		state = state*1664525 + 1013904223
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = byte(state>>24&0xFF), byte(state>>16&0xFF), byte(state>>8&0xFF), 0xFF
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		b.Fatal(err)
	}
	return buf.Bytes()
}

// BenchmarkPut times rendering a 3000 x 3000 cover into an empty cache.
func BenchmarkPut(b *testing.B) {
	data := noisyJPEG(b)
	b.ReportMetric(float64(len(data)), "source-bytes")
	for b.Loop() {
		b.StopTimer()
		c := artwork.NewCache(b.TempDir())
		b.StartTimer()
		if _, err := c.Put(b.Context(), artwork.Cover, data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPutIdempotent times Put of a cover whose renditions all exist: a
// hash, a header read and the decode of the smallest rendition.
func BenchmarkPutIdempotent(b *testing.B) {
	data := noisyJPEG(b)
	c := artwork.NewCache(b.TempDir())
	if _, err := c.Put(b.Context(), artwork.Cover, data); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if _, err := c.Put(b.Context(), artwork.Cover, data); err != nil {
			b.Fatal(err)
		}
	}
}

// photo returns a w x h image of smooth gradients with a little grain: closer
// to a photograph's size on disk than noise, which no encoder can shrink.
func photo(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	state := uint32(7)
	for y := range h {
		for x := range w {
			state = state*1664525 + 1013904223
			grain := byte(state >> 28)
			i := img.PixOffset(x, y)
			img.Pix[i] = byte(x*255/w&0xFF) + grain
			img.Pix[i+1] = byte(y*255/h&0xFF) + grain
			img.Pix[i+2] = byte((x+y)*255/(w+h)&0xFF) + grain
			img.Pix[i+3] = 0xFF
		}
	}
	return img
}

// clearBand returns photo with all but its middle third transparent, as a logo or
// a disc is.
func clearBand(w, h int) *image.NRGBA {
	img := photo(w, h)
	for y := range h {
		if y >= h/3 && y < 2*h/3 {
			continue
		}
		for x := range w {
			img.Pix[img.PixOffset(x, y)+3] = 0
		}
	}
	return img
}

// diskBytes sums the size of every file under dir.
func diskBytes(b *testing.B, dir string) int64 {
	b.Helper()
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
		b.Fatal(err)
	}
	return n
}

// BenchmarkPutKinds times rendering each kind of artist image and album extra
// at the size its sources come in, and reports what its renditions take on disk.
func BenchmarkPutKinds(b *testing.B) {
	for _, bc := range []struct {
		name string
		kind artwork.Kind
		img  image.Image
		png  bool
	}{
		{"thumb-1000", artwork.Thumb, photo(1000, 1000), false},
		{"background-1920", artwork.Background, photo(1920, 1080), false},
		{"background-3840", artwork.Background, photo(3840, 2160), false},
		{"banner-1000", artwork.Banner, photo(1000, 185), false},
		{"logo-800", artwork.Logo, clearBand(800, 310), true},
		{"disc-1000", artwork.Disc, clearBand(1000, 1000), true},
	} {
		b.Run(bc.name, func(b *testing.B) {
			var buf bytes.Buffer
			var err error
			if bc.png {
				err = png.Encode(&buf, bc.img)
			} else {
				err = jpeg.Encode(&buf, bc.img, &jpeg.Options{Quality: 90})
			}
			if err != nil {
				b.Fatal(err)
			}
			data := buf.Bytes()
			var dir string
			for b.Loop() {
				b.StopTimer()
				dir = b.TempDir()
				c := artwork.NewCache(dir)
				b.StartTimer()
				if _, err = c.Put(b.Context(), bc.kind, data); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(diskBytes(b, dir)), "disk-bytes")
		})
	}
}
