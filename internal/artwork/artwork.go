// Package artwork renders album covers and artist images into a
// content-addressed cache.
//
// A [Cache] turns one source image into a ladder of renditions while the
// library is scanned, so serving an image is only ever a file read. Each
// [Kind] has its own ladder (see [Kind.Ladder]): covers, thumbs and back covers
// go to 1024, backgrounds and banners to 3840, logos and disc art to 800. Every
// ladder starts at 128, the rendition the colors come from. Renditions are
// JPEG, flattened onto the UI's ground, except a logo's or a disc's with
// transparency, which stay PNG. Files are named after the SHA-256 of the source
// bytes, so equal images are stored once and a rendition never changes. The
// package knows nothing of the store or the scanner.
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
	"slices"
	"strings"
)

// Sizes of the rendition ladders, as longest side in pixels.
const (
	Size128  = 128
	Size400  = 400
	Size512  = 512
	Size640  = 640
	Size800  = 800
	Size1024 = 1024
	Size1920 = 1920
	Size3840 = 3840
)

// Sizes lists every rung of every ladder, smallest first.
func Sizes() []int {
	return []int{Size128, Size400, Size512, Size640, Size800, Size1024, Size1920, Size3840}
}

// Kind says what an image is for, which decides its ladder and its format.
type Kind int

const (
	// Cover is an album's front cover.
	Cover Kind = iota
	// Thumb is an artist's portrait.
	Thumb
	// Back is an album's back cover.
	Back
	// Background is an artist's wide backdrop, 16:9 as a rule.
	Background
	// Banner is an artist's strip, about 5.4:1.
	Banner
	// Logo is an artist's name as a mark, usually on a transparent ground.
	Logo
	// Disc is an album's disc art, usually on a transparent ground.
	Disc
)

// Ladder returns the kind's rung sizes, smallest first.
func (k Kind) Ladder() []int {
	switch k {
	case Background, Banner:
		return []int{Size128, Size640, Size1920, Size3840}
	case Logo, Disc:
		return []int{Size128, Size400, Size800}
	case Cover, Thumb, Back:
	}
	return []int{Size128, Size512, Size1024}
}

// String names the kind.
func (k Kind) String() string {
	switch k {
	case Cover:
		return "cover"
	case Thumb:
		return "thumb"
	case Back:
		return "back"
	case Background:
		return "background"
	case Banner:
		return "banner"
	case Logo:
		return "logo"
	case Disc:
		return "disc"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// keepsAlpha reports whether the kind's renditions stay PNG when the source has
// transparency, rather than being flattened onto the ground.
func (k Kind) keepsAlpha() bool { return k == Logo || k == Disc }

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

// Formats of a rendition, as its file's extension.
const (
	extJPEG = ".jpg"
	extPNG  = ".png"
)

// Image is a stored image.
type Image struct {
	Hash          [32]byte // SHA-256 of the source image bytes.
	Width, Height int      // Of the source.
	Accent        string   // "#rrggbb": the image's most populous vivid color.
	Ground        string   // "#rrggbb": its dark ground, for backdrops.
	Alpha         bool     // The renditions are PNG, with the source's transparency.
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

// Put stores renditions of the image in data, on kind's ladder, and returns
// its description. It is idempotent: when every rendition already exists and
// decodes, nothing is rendered. A missing or damaged rendition makes it render
// them all again. The same image put as two kinds has both ladders.
//
// Put returns [ErrBadImage] for data that is not a decodable JPEG, PNG or
// WebP, [ErrTooLarge] for more than 64 million pixels (the image is then not
// decoded), an error wrapping ctx.Err() when ctx ends while it waits to render
// or while it renders, and otherwise the file system's error.
func (c *Cache) Put(ctx context.Context, kind Kind, data []byte) (*Image, error) {
	cfg, err := readConfig(data)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(data)
	if img, ok := c.existing(hash, kind, cfg.Width, cfg.Height); ok {
		return img, nil
	}
	if aerr := c.acquire(ctx); aerr != nil {
		return nil, aerr
	}
	defer c.release()
	r, err := c.render(ctx, hash, kind, data)
	if err != nil {
		return nil, err
	}
	cols, err := colorsOf(r.small)
	if err != nil {
		return nil, fmt.Errorf("artwork: colors of the %d px rendition: %w", Size128, err)
	}
	return &Image{Hash: hash, Width: cfg.Width, Height: cfg.Height, Accent: cols.accent, Ground: cols.ground, Alpha: r.alpha}, nil
}

// Path returns the file of the largest rendition at or below size, or the
// smallest that exists when size is below them all, of any kind's ladder the
// image was put on. [IsPNG] says its format. It returns [ErrNotFound] when the
// cache has no rendition of hash.
func (c *Cache) Path(hash [32]byte, size int) (string, error) {
	rungs := Sizes()
	at := sizeIndex(rungs, size)
	for i := at; i >= 0; i-- { // The largest at or below size...
		if p, ok := c.rendition(hash, rungs[i]); ok {
			return p, nil
		}
	}
	for _, rung := range rungs[at+1:] { // ...else the smallest above it.
		if p, ok := c.rendition(hash, rung); ok {
			return p, nil
		}
	}
	return "", ErrNotFound
}

// IsPNG reports whether the rendition at path, as [Cache.Path] returns it, is
// PNG; any other is JPEG.
func IsPNG(path string) bool { return strings.HasSuffix(path, extPNG) }

// sizeIndex is the index of the largest rung at or below size, or -1.
func sizeIndex(rungs []int, size int) int {
	i, found := slices.BinarySearch(rungs, size)
	if found {
		return i
	}
	return i - 1
}

// Delete removes every rendition of hash, of every kind. A rendition that is
// already gone is not an error.
func (c *Cache) Delete(hash [32]byte) error {
	var errs []error
	for _, rung := range Sizes() {
		for _, ext := range [...]string{extJPEG, extPNG} {
			if err := os.Remove(c.file(hash, rung, ext)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("artwork: delete: %w", err)
	}
	return nil
}

// rendition returns the usable file of hash's rung, JPEG first.
func (c *Cache) rendition(hash [32]byte, rung int) (string, bool) {
	for _, ext := range [...]string{extJPEG, extPNG} {
		if p := c.file(hash, rung, ext); usable(p) {
			return p, true
		}
	}
	return "", false
}

// file is where the rung of hash lives: <dir>/<hh>/<hash>-<rung><ext>.
func (c *Cache) file(hash [32]byte, rung int, ext string) string {
	name := hex.EncodeToString(hash[:])
	return filepath.Join(c.dir, name[:2], fmt.Sprintf("%s-%d%s", name, rung, ext))
}

// usable reports whether path is a non-empty regular file. An empty file is
// missing: a year-long immutable cache header would pin it in clients.
func usable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular() && st.Size() > 0
}

// existing returns the image when every rendition of a w x h source exists on
// kind's ladder, in one format, and the smallest decodes. A kind that keeps
// alpha may have been rendered either way, by the source's pixels.
func (c *Cache) existing(hash [32]byte, kind Kind, w, h int) (*Image, bool) {
	exts := []string{extJPEG}
	if kind.keepsAlpha() {
		exts = []string{extPNG, extJPEG}
	}
	for _, ext := range exts {
		if p, ok := c.existingAs(hash, kind, w, h, ext); ok {
			return &Image{Hash: hash, Width: w, Height: h, Accent: p.accent, Ground: p.ground, Alpha: ext == extPNG}, true
		}
	}
	return nil, false
}

// existingAs reports whether every rendition exists with the extension ext,
// with the colors of the smallest.
func (c *Cache) existingAs(hash [32]byte, kind Kind, w, h int, ext string) (palette, bool) {
	for _, r := range plan(kind.Ladder(), w, h) {
		if !usable(c.file(hash, r.rung, ext)) {
			return palette{}, false
		}
	}
	small, err := os.ReadFile(c.file(hash, Size128, ext))
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
