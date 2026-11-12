package library

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/store"
)

// maxArtworkBytes caps an image read from a file or a tag.
const maxArtworkBytes = 32 << 20

// Folder art is a file named like one of coverNames with one of coverExts.
// Earlier names and extensions win.
var (
	coverNames = [...]string{"cover", "folder", "front", "album"} //nolint:gochecknoglobals // Fixed lookup tables; Go has no const arrays.
	coverExts  = [...]string{".jpg", ".jpeg", ".png", ".webp"}    //nolint:gochecknoglobals // As above.
)

// covers finds and stores one cover per album during a read: the one the first
// track of the album with art has, by [store.Place], whichever file a worker
// happens to finish first. A nil *covers finds nothing.
type covers struct {
	cache *artwork.Cache
	log   *slog.Logger
	mu    sync.Mutex
	best  map[string]store.Place // By group key: the first track found with a cover.
}

func newCovers(cache *artwork.Cache, log *slog.Logger) *covers {
	return &covers{cache: cache, log: log, best: make(map[string]store.Place)}
}

// ahead reports whether r comes before the album's best so far. The caller holds mu.
func (c *covers) ahead(album string, r store.Place) bool {
	best, ok := c.best[album]
	return !ok || r.Compare(best) < 0
}

// wants reports whether a cover from r would improve on the album's.
func (c *covers) wants(album string, r store.Place) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ahead(album, r)
}

// take makes r the album's best, and reports false when a track before it has
// got there first.
func (c *covers) take(album string, r store.Place) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ahead(album, r) {
		return false
	}
	c.best[album] = r
	return true
}

// count returns how many albums have a cover.
func (c *covers) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.best)
}

// settle drops tw's cover when a track before it has one by now. Workers finish in
// any order, and the album keeps the last cover written, so the writer must not
// let a later track's cover land after an earlier one's.
func (c *covers) settle(tw *store.TrackWrite) {
	if tw.Album.Artwork == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.best[tw.Album.GroupKey] != store.PlaceOf(tw) {
		tw.Album.Artwork = nil
	}
}

// attach looks for the cover of tw's album if tw comes before every track with
// one so far, and puts it on tw. f is open on the file tw was made from. It appends
// the warnings the search produced to tw. It returns an error only when ctx
// ended, so that the caller drops the result.
func (c *covers) attach(ctx context.Context, f *audiometa.File, j *job, tw *store.TrackWrite) error {
	if c == nil || c.cache == nil {
		return nil
	}
	r := store.PlaceOf(tw)
	if !c.wants(tw.Album.GroupKey, r) {
		return nil
	}
	seen := len(f.Warnings)
	s := search{covers: c, f: f, j: j}
	art := s.run(ctx)
	for _, w := range f.Warnings[seen:] {
		tw.Warnings = append(tw.Warnings, strings.ToValidUTF8(w.String(), "�"))
	}
	tw.Warnings = append(tw.Warnings, s.warnings...)
	if art == nil {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("find cover: %w", err)
		}
		return nil
	}
	if c.take(tw.Album.GroupKey, r) {
		tw.Album.Artwork = art
	}
	return nil
}

// search is one look for an album's cover.
type search struct {
	*covers

	f        *audiometa.File
	j        *job
	warnings []string // Fixed text and relative paths only, as valid UTF-8.
}

// run tries the embedded pictures, then the folders, and returns the first
// cover the cache accepts. It returns nil when there is none.
func (s *search) run(ctx context.Context) *store.ArtworkWrite {
	if art, stop := s.embedded(ctx); art != nil || stop {
		return art
	}
	art, _ := s.folders(ctx)
	return art
}

// embedded tries the file's pictures: the front cover first, then the others
// in file order. The bool says the search must end, empty-handed.
func (s *search) embedded(ctx context.Context) (*store.ArtworkWrite, bool) {
	pics, err := s.f.ExtractArtworkContext(ctx)
	if err != nil {
		// A damaged picture block never fails the file; folders come next.
		if ctx.Err() != nil {
			return nil, true
		}
		s.warn("embedded artwork: unreadable")
		return nil, false
	}
	front := slices.IndexFunc(pics, func(a audiometa.Artwork) bool { return a.Type == audiometa.ArtworkFrontCover })
	order := make([]int, 0, len(pics))
	if front >= 0 {
		order = append(order, front)
	}
	for i := range pics {
		if i != front {
			order = append(order, i)
		}
	}
	for _, i := range order {
		if len(pics[i].Data) == 0 {
			continue
		}
		if art, stop := s.put(ctx, pics[i].Data, fmt.Sprintf("embedded artwork %d", i+1), store.ArtworkEmbedded); art != nil || stop {
			return art, stop
		}
	}
	return nil, false
}

// put offers data to the cache. It returns the cover on success. When data is
// not a usable image it notes a warning and returns nothing, so that the next
// candidate is tried. The bool is true when the search must end: ctx ended, or
// the cache failed in a way another image would not fix.
func (s *search) put(ctx context.Context, data []byte, what string, src store.ArtworkSource) (*store.ArtworkWrite, bool) {
	img, err := s.cache.Put(ctx, data)
	switch {
	case err == nil:
		return &store.ArtworkWrite{
			Hash: img.Hash, Width: img.Width, Height: img.Height,
			Accent: img.Accent, Ground: img.Ground, Source: src,
		}, false
	case ctx.Err() != nil:
		return nil, true
	case errors.Is(err, artwork.ErrBadImage):
		s.warn(what + ": not a decodable image")
		return nil, false
	case errors.Is(err, artwork.ErrTooLarge):
		s.warn(what + ": too many pixels")
		return nil, false
	}
	s.log.WarnContext(ctx, "storing album artwork failed; the track is stored without it", "err", err)
	return nil, true
}

// warn records a warning, as valid UTF-8.
func (s *search) warn(msg string) {
	s.warnings = append(s.warnings, "artwork: "+strings.ToValidUTF8(msg, "�"))
}

// folders tries the cover files in the track's folder, then, when that is a
// disc folder, in its parent. The library root is opened as an [os.Root], so a
// symlink leading out of it is never followed.
func (s *search) folders(ctx context.Context) (*store.ArtworkWrite, bool) {
	root, err := os.OpenRoot(s.j.Root)
	if err != nil {
		// The track was just read from here; a vanished root is the next scan's business.
		return nil, false
	}
	defer func() { _ = root.Close() }() // Read-only.
	dir := filepath.Dir(s.j.RelPath)
	dirs := []string{dir}
	if parent, ok := discParent(dir); ok {
		dirs = append(dirs, parent)
	}
	for _, d := range dirs {
		for _, name := range coverFiles(root, d) {
			rel := filepath.Join(d, name)
			data := readRegular(root, rel)
			if data == nil {
				continue
			}
			if art, stop := s.put(ctx, data, rel, store.ArtworkFolder); art != nil || stop {
				return art, stop
			}
		}
	}
	return nil, false
}

// coverFiles lists the names in dir that look like cover files, best first.
// Ties between names that differ only in case go to the sorted first.
func coverFiles(root *os.Root, dir string) []string {
	// Not root.FS(): fs.FS rejects paths that are not valid UTF-8, which a
	// library may well hold.
	d, err := root.Open(dir)
	if err != nil {
		return nil
	}
	defer func() { _ = d.Close() }() // Read-only.
	entries, err := d.ReadDir(-1)
	if err != nil {
		return nil
	}
	type ranked struct {
		name string
		rank int
	}
	var found []ranked
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(path.Ext(e.Name()))
		stem := strings.ToLower(strings.TrimSuffix(e.Name(), path.Ext(e.Name())))
		n, x := slices.Index(coverNames[:], stem), slices.Index(coverExts[:], ext)
		if n >= 0 && x >= 0 {
			found = append(found, ranked{e.Name(), n*len(coverExts) + x})
		}
	}
	slices.SortFunc(found, func(a, b ranked) int { return cmp.Or(cmp.Compare(a.rank, b.rank), strings.Compare(a.name, b.name)) })
	names := make([]string, len(found))
	for i, r := range found {
		names[i] = r.name
	}
	return names
}

// readRegular reads rel under root when it is a regular file within
// [maxArtworkBytes], following symlinks that stay inside root. It returns nil
// for anything else.
func readRegular(root *os.Root, rel string) []byte {
	st, err := root.Stat(rel)
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxArtworkBytes {
		return nil
	}
	f, err := root.Open(rel)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }() // Read-only.
	data, err := io.ReadAll(io.LimitReader(f, maxArtworkBytes+1))
	if err != nil || len(data) > maxArtworkBytes {
		return nil
	}
	return data
}
