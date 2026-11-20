package library

import (
	"cmp"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/store"
)

// The file names a scan reads in artist and album folders are Kodi's and
// Jellyfin's, compared ignoring case. Earlier names win, then earlier
// extensions, then the name sorted first. The album's own cover is not here:
// [covers] finds it.

// nfoName is the artist.nfo a bio is read from.
const nfoName = "artist.nfo"

// The image extensions a scan reads, beside the audio.
const (
	extJPG  = ".jpg"
	extJPEG = ".jpeg"
	extPNG  = ".png"
	extWebP = ".webp"
)

// extraKind is one kind of image a scan reads from a folder: its stems, best
// first, and its extensions, best first. A numbered stem ("fanart1") follows
// every plain one.
type extraKind struct {
	stems    []string
	numbered []string // Stems that also come numbered: fanart1, fanart-2.
	exts     []string
	art      artwork.Kind
}

// photoExts and clearExts are the extensions read, best first: a logo or disc
// art is best as a PNG, which can be transparent.
//
//nolint:gochecknoglobals // Fixed lookup tables; Go has no const slices.
var (
	photoExts = []string{extJPG, extJPEG, extPNG, extWebP}
	clearExts = []string{extPNG, extWebP, extJPG, extJPEG}
)

// artistKinds are what an artist folder holds.
//
//nolint:gochecknoglobals // A fixed lookup table.
var artistKinds = map[store.ArtistImageKind]extraKind{
	store.ArtistThumb:      {stems: []string{"artist", "thumb", "folder"}, exts: photoExts, art: artwork.Thumb},
	store.ArtistBackground: {stems: []string{"fanart", "backdrop", "background"}, numbered: []string{"fanart", "backdrop"}, exts: photoExts, art: artwork.Background},
	store.ArtistLogo:       {stems: []string{"clearlogo", "logo"}, exts: clearExts, art: artwork.Logo},
	store.ArtistBanner:     {stems: []string{"banner"}, exts: photoExts, art: artwork.Banner},
}

// The album folder's extras. Disc art for every disc is cdart, discart or
// disc; for one disc, those or cd with its number: cd2.png, discart-2.png.
//
//nolint:gochecknoglobals // Fixed lookup tables.
var (
	backKind  = extraKind{stems: []string{"back"}, exts: photoExts, art: artwork.Back}
	discKind  = extraKind{stems: []string{"cdart", "discart", "disc"}, exts: clearExts, art: artwork.Disc}
	discStems = []string{"cdart", "discart", "disc", "cd"}
	// numberedStem is a stem with a number after it, perhaps after a space, a
	// hyphen or an underscore.
	numberedStem = regexp.MustCompile(`^([a-z]+)[ _-]?(\d{1,3})$`)
)

// mostDiscs is the highest disc number a file name may carry.
const mostDiscs = 999

// fileName is a file's name split for matching: its stem and extension, both
// lower case.
type fileName struct{ stem, ext string }

func splitName(name string) fileName {
	ext := path.Ext(name)
	return fileName{stem: strings.ToLower(strings.TrimSuffix(name, ext)), ext: strings.ToLower(ext)}
}

// numberedName is a stem with a number: "fanart" and 2 of "fanart-2".
type numberedName struct {
	base string
	n    int
}

// splitNumber splits a numbered stem, and reports false for any other.
func splitNumber(stem string) (numberedName, bool) {
	m := numberedStem.FindStringSubmatch(stem)
	if m == nil {
		return numberedName{}, false
	}
	n, err := strconv.Atoi(m[2])
	if err != nil || n < 1 || n > mostDiscs {
		return numberedName{}, false
	}
	return numberedName{base: m[1], n: n}, true
}

// rank is how good name is as the kind, lower being better, and false when it
// is not one.
func (k *extraKind) rank(name string) (int, bool) {
	f := splitName(name)
	x := slices.Index(k.exts, f.ext)
	if x < 0 {
		return 0, false
	}
	if s := slices.Index(k.stems, f.stem); s >= 0 {
		return s*len(k.exts) + x, true
	}
	if num, ok := splitNumber(f.stem); ok {
		if s := slices.Index(k.numbered, num.base); s >= 0 {
			return (len(k.stems)+s)*len(k.exts) + x, true
		}
	}
	return 0, false
}

// discArt is what a disc-art name says: the disc it is for ([store.AllDiscs]
// for every disc) and its rank among that disc's names.
type discArt struct{ disc, rank int }

// discOf reads a disc-art name, and reports false for any other.
func discOf(name string) (discArt, bool) {
	if r, ok := discKind.rank(name); ok {
		return discArt{disc: store.AllDiscs, rank: r}, true
	}
	f := splitName(name)
	x := slices.Index(clearExts, f.ext)
	num, ok := splitNumber(f.stem)
	if x < 0 || !ok {
		return discArt{}, false
	}
	s := slices.Index(discStems, num.base)
	if s < 0 {
		return discArt{}, false
	}
	return discArt{disc: num.n, rank: s*len(clearExts) + x}, true
}

// isExtraFile reports whether a walk keeps the file name for the extras step:
// it may be an artist's or an album's image, or an artist.nfo.
func isExtraFile(name string) bool {
	if strings.EqualFold(name, nfoName) {
		return true
	}
	for _, k := range artistKinds {
		if _, ok := k.rank(name); ok {
			return true
		}
	}
	if _, ok := backKind.rank(name); ok {
		return true
	}
	_, ok := discOf(name)
	return ok
}

// ranked is a file and how good it is.
type ranked struct {
	file *seenFile
	rank int
}

// best orders candidates best first: by rank, then by name.
func best(found []ranked) []*seenFile {
	slices.SortFunc(found, func(a, b ranked) int {
		return cmp.Or(cmp.Compare(a.rank, b.rank), strings.Compare(a.file.rel, b.file.rel))
	})
	out := make([]*seenFile, len(found))
	for i := range found {
		out[i] = found[i].file
	}
	return out
}

// candidates returns the files that are the kind, best first.
func (k *extraKind) candidates(files []*seenFile) []*seenFile {
	var found []ranked
	for _, f := range files {
		if r, ok := k.rank(path.Base(f.rel)); ok {
			found = append(found, ranked{f, r})
		}
	}
	return best(found)
}

// discCandidates returns the disc art among files by disc, best first.
func discCandidates(files []*seenFile) map[int][]*seenFile {
	found := map[int][]ranked{}
	for _, f := range files {
		if d, ok := discOf(path.Base(f.rel)); ok {
			found[d.disc] = append(found[d.disc], ranked{f, d.rank})
		}
	}
	out := make(map[int][]*seenFile, len(found))
	for disc, fs := range found {
		out[disc] = best(fs)
	}
	return out
}

// nfoCandidates returns the artist.nfo among files, if there is one; of two
// that differ only in case, the one sorted first.
func nfoCandidates(files []*seenFile) []*seenFile {
	var found []ranked
	for _, f := range files {
		if strings.EqualFold(path.Base(f.rel), nfoName) {
			found = append(found, ranked{f, 0})
		}
	}
	if len(found) == 0 {
		return nil
	}
	return best(found)[:1]
}
