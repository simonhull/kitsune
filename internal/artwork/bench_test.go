package artwork_test

import (
	"bytes"
	"image"
	"image/jpeg"
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
		if _, err := c.Put(b.Context(), data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPutIdempotent times Put of a cover whose renditions all exist: a
// hash, a header read and the decode of the smallest rendition.
func BenchmarkPutIdempotent(b *testing.B) {
	data := noisyJPEG(b)
	c := artwork.NewCache(b.TempDir())
	if _, err := c.Put(b.Context(), data); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if _, err := c.Put(b.Context(), data); err != nil {
			b.Fatal(err)
		}
	}
}
