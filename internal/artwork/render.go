package artwork

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"

	_ "image/png" // Registers the PNG decoder for image.Decode.

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // Registers the WebP decoder for image.Decode.
)

// jpegQuality is the quality of every rendition.
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

// plan lists the renditions of a w x h source, largest first. Rungs below the
// source's longest side are written at rung size; the smallest rung at or
// above it is written at the source's own size, so nothing is upscaled; larger
// rungs are not written.
func plan(w, h int) []rendition {
	longest := max(w, h)
	var out []rendition
	rungs := ladder()
	for i := len(rungs) - 1; i >= 0; i-- {
		switch {
		case rungs[i] < longest:
			out = append(out, fit(rungs[i], w, h))
		case i == 0 || rungs[i-1] < longest:
			out = append(out, rendition{rung: rungs[i], w: w, h: h})
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

// render decodes data once and writes every rung, scaling each from the one
// before it. It returns the encoded smallest rung. EXIF orientation is
// ignored: covers are stored as the file's pixels have them.
func (c *Cache) render(ctx context.Context, hash [32]byte, data []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadImage, err)
	}
	b := src.Bounds()
	steps := plan(b.Dx(), b.Dy())
	var small []byte
	for _, st := range steps {
		if err = ctx.Err(); err != nil {
			return nil, fmt.Errorf("artwork: render: %w", err)
		}
		// The first step flattens alpha onto the ground; later steps scale an
		// opaque image.
		dst := image.NewRGBA(image.Rect(0, 0, st.w, st.h))
		draw.Draw(dst, dst.Bounds(), image.NewUniform(background()), image.Point{}, draw.Src)
		if st.w == b.Dx() && st.h == b.Dy() {
			draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Over)
		} else {
			draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		}
		src = dst // Drops the source (and, later, the larger rung).
		var buf bytes.Buffer
		if err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return nil, fmt.Errorf("artwork: encode %d px rendition: %w", st.rung, err)
		}
		if werr := c.write(c.file(hash, st.rung), buf.Bytes()); werr != nil {
			return nil, werr
		}
		small = buf.Bytes()
	}
	return small, nil
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
