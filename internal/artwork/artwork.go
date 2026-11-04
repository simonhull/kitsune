// Package artwork renders album covers into a content-addressed cache.
//
// A [Cache] turns one source image into a ladder of JPEG renditions (see
// [Size128]) while the library is scanned, so serving a cover is only ever a
// file read. Files are named after the SHA-256 of the source bytes, so equal
// covers are stored once and a rendition never changes. The package knows
// nothing of the store or the scanner.
//
// Limits: JPEG, PNG and WebP sources up to 64 megapixels; EXIF orientation is
// ignored; at most two images are rendered at once, because a large source
// needs hundreds of megabytes to decode.
package artwork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Sizes of the rendition ladder, as longest side in pixels.
const (
	Size128  = 128
	Size512  = 512
	Size1024 = 1024
)

// Sentinels callers match with [errors.Is].
var (
	// ErrBadImage means the data is not an image Put can decode.
	ErrBadImage = errors.New("not a decodable image")
	// ErrTooLarge means the image has more pixels than Put will decode.
	ErrTooLarge = errors.New("image has too many pixels")
	// ErrNotFound means the cache holds no rendition of the image.
	ErrNotFound = errors.New("artwork not found")
)

const (
	// maxPixels is the most pixels Put decodes; 64 MP peaks at 400 to 900 MB.
	maxPixels = 64_000_000
	// maxRenders is how many images render at once.
	maxRenders = 2
	// dirMode is the mode of the cache's directories.
	dirMode = 0o700
)

// Image is a stored cover.
type Image struct {
	Hash          [32]byte // SHA-256 of the source image bytes.
	Width, Height int      // Of the source.
	Accent        string   // "#rrggbb": the cover's most populous vivid color.
	Ground        string   // "#rrggbb": its dark ground, for backdrops.
}

// Cache is a directory of renditions. It is safe for concurrent use.
type Cache struct {
	dir string
	sem chan struct{} // Holds one token per image being rendered.
}

// NewCache returns a cache that stores its files under dir, which is created
// when the first rendition is written.
func NewCache(dir string) *Cache {
	return &Cache{dir: dir, sem: make(chan struct{}, maxRenders)}
}

// ladder returns the rung sizes, smallest first.
func ladder() [3]int { return [3]int{Size128, Size512, Size1024} }

// Put stores renditions of the image in data and returns its description. It
// is idempotent: when every rendition already exists and decodes, nothing is
// rendered. A missing or damaged rendition makes it render them all again.
//
// Put returns [ErrBadImage] for data that is not a decodable JPEG, PNG or
// WebP, [ErrTooLarge] for more than 64 million pixels (the image is then not
// decoded), an error wrapping ctx.Err() when ctx ends while it waits to render
// or while it renders, and otherwise the file system's error.
func (c *Cache) Put(ctx context.Context, data []byte) (*Image, error) {
	cfg, err := readConfig(data)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(data)
	if cols, ok := c.existing(hash, cfg.Width, cfg.Height); ok {
		return &Image{Hash: hash, Width: cfg.Width, Height: cfg.Height, Accent: cols.accent, Ground: cols.ground}, nil
	}
	if aerr := c.acquire(ctx); aerr != nil {
		return nil, aerr
	}
	defer c.release()
	small, err := c.render(ctx, hash, data)
	if err != nil {
		return nil, err
	}
	cols, err := colorsOf(small)
	if err != nil {
		return nil, fmt.Errorf("artwork: colors of the %d px rendition: %w", Size128, err)
	}
	return &Image{Hash: hash, Width: cfg.Width, Height: cfg.Height, Accent: cols.accent, Ground: cols.ground}, nil
}

// Path returns the file of the largest rendition at or below size, or the
// smallest that exists when size is below them all. It returns [ErrNotFound]
// when the cache has no rendition of hash.
func (c *Cache) Path(hash [32]byte, size int) (string, error) {
	rungs := ladder()
	var smallest string
	for i := len(rungs) - 1; i >= 0; i-- {
		p := c.file(hash, rungs[i])
		if !usable(p) {
			continue
		}
		if rungs[i] <= size {
			return p, nil
		}
		smallest = p
	}
	if smallest == "" {
		return "", ErrNotFound
	}
	return smallest, nil
}

// Delete removes every rendition of hash. A rendition that is already gone is
// not an error.
func (c *Cache) Delete(hash [32]byte) error {
	var errs []error
	for _, rung := range ladder() {
		if err := os.Remove(c.file(hash, rung)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("artwork: delete: %w", err)
	}
	return nil
}

// file is where the rung of hash lives: <dir>/<hh>/<hash>-<rung>.jpg.
func (c *Cache) file(hash [32]byte, rung int) string {
	name := hex.EncodeToString(hash[:])
	return filepath.Join(c.dir, name[:2], fmt.Sprintf("%s-%d.jpg", name, rung))
}

// usable reports whether path is a non-empty regular file. An empty file is
// missing: a year-long immutable cache header would pin it in clients.
func usable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular() && st.Size() > 0
}

// existing reports whether every rendition of a w x h source exists and the
// smallest decodes, with its accent and ground.
func (c *Cache) existing(hash [32]byte, w, h int) (palette, bool) {
	for _, r := range plan(w, h) {
		if !usable(c.file(hash, r.rung)) {
			return palette{}, false
		}
	}
	small, err := os.ReadFile(c.file(hash, Size128))
	if err != nil {
		return palette{}, false
	}
	p, err := colorsOf(small)
	return p, err == nil
}

func (c *Cache) acquire(ctx context.Context) error {
	select {
	case c.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("artwork: waiting to render: %w", ctx.Err())
	}
}

func (c *Cache) release() { <-c.sem }
