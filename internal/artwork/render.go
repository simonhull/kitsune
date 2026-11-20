package artwork

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"slices"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // Registers the WebP decoder for image.Decode.
)

// jpegQuality is the quality of every JPEG rendition.
const jpegQuality = 82

// The UI's ground, #0A0908: what transparent pixels become.
const (
	groundR = 0x0A
	groundG = 0x09
	groundB = 0x08
	opaque  = 0xFF
)

// background returns the color transparent pixels are flattened onto.
func background() color.RGBA { return color.RGBA{R: groundR, G: groundG, B: groundB, A: opaque} }

// rendition is one rung to write: its file name's size and its pixel size.
type rendition struct {
	rung, w, h int
}

// plan lists the renditions of a w x h source on the ladder rungs (smallest
// first), largest first. Rungs below the source's longest side are written at
// rung size; the smallest rung at or above it is written at the source's own
// size, so nothing is upscaled; larger rungs are not written.
func plan(rungs []int, w, h int) []rendition {
	longest := max(w, h)
	var out []rendition
	for i, rung := range slices.Backward(rungs) {
		switch {
		case rung < longest:
			out = append(out, fit(rung, w, h))
		case i == 0 || rungs[i-1] < longest:
			out = append(out, rendition{rung: rung, w: w, h: h})
		}
	}
	return out
}

// fit scales w x h so its longest side is rung, keeping the shape.
func fit(rung, w, h int) rendition {
	longest := max(w, h)
	scale := func(v int) int { return max(1, (v*rung+longest/2)/longest) }
	return rendition{rung: rung, w: scale(w), h: scale(h)}
}

// readConfig checks the header of data without decoding the pixels.
func readConfig(data []byte) (image.Config, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return image.Config{}, fmt.Errorf("%w: %w", ErrBadImage, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return image.Config{}, fmt.Errorf("%w: empty", ErrBadImage)
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return image.Config{}, fmt.Errorf("%w: %d x %d", ErrTooLarge, cfg.Width, cfg.Height)
	}
	return cfg, nil
}

// hasAlpha reports whether any pixel of img is less than opaque. An image type
// that can't say is looked at pixel by pixel.
func hasAlpha(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return !o.Opaque()
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != opaque16 {
				return true
			}
		}
	}
	return false
}

// rendered is what render wrote: the encoded smallest rung, and whether the
// renditions are PNG.
type rendered struct {
	small []byte
	alpha bool
}

// render decodes data once and writes every rung of kind's ladder, scaling each
// from the one before it. It returns the encoded smallest rung, and whether the
// renditions are PNG: those of a kind that keeps alpha, from a source with
// transparency. Every other is JPEG, flattened onto the ground. EXIF orientation
// is ignored: images are stored as the file's pixels have them.
func (c *Cache) render(ctx context.Context, hash [32]byte, kind Kind, data []byte) (rendered, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return rendered{}, fmt.Errorf("%w: %w", ErrBadImage, err)
	}
	alpha := kind.keepsAlpha() && hasAlpha(src)
	ext := extJPEG
	if alpha {
		ext = extPNG
	}
	b := src.Bounds()
	var small []byte
	for _, st := range plan(kind.Ladder(), b.Dx(), b.Dy()) {
		if err = ctx.Err(); err != nil {
			return rendered{}, fmt.Errorf("artwork: render: %w", err)
		}
		dst := scaled(src, st, alpha)
		src = dst // Drops the source (and, later, the larger rung).
		encoded, eerr := encode(dst, alpha)
		if eerr != nil {
			return rendered{}, fmt.Errorf("artwork: encode %d px rendition: %w", st.rung, eerr)
		}
		if werr := c.write(c.file(hash, st.rung, ext), encoded); werr != nil {
			return rendered{}, werr
		}
		small = encoded
	}
	return rendered{small: small, alpha: alpha}, nil
}

// scaled draws src at the rendition's size. Without alpha the first step
// flattens transparency onto the ground; later steps scale an opaque image.
func scaled(src image.Image, st rendition, alpha bool) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, st.w, st.h))
	op := draw.Src
	if !alpha {
		draw.Draw(dst, dst.Bounds(), image.NewUniform(background()), image.Point{}, draw.Src)
		op = draw.Over
	}
	if st.w == b.Dx() && st.h == b.Dy() {
		draw.Draw(dst, dst.Bounds(), src, b.Min, op)
	} else {
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, op, nil)
	}
	return dst
}

// encode is img as PNG, keeping alpha, or as JPEG.
func encode(img image.Image, alpha bool) ([]byte, error) {
	var buf bytes.Buffer
	var err error
	if alpha {
		err = png.Encode(&buf, img)
	} else {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality})
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // render wraps it with the rung.
	}
	return buf.Bytes(), nil
}

// write stores data at path through a synced temporary file and a rename, so a
// reader never sees a partial rendition.
func (c *Cache) write(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("artwork: %w", err)
	}
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("artwork: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, f.Close(), os.Remove(f.Name()))
		}
	}()
	if _, err = f.Write(data); err != nil {
		return fmt.Errorf("artwork: write: %w", err)
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("artwork: sync: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("artwork: close: %w", err)
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("artwork: %w", err)
	}
	return nil
}
