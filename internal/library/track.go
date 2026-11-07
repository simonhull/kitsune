package library

import (
	"cmp"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	yearDigits = 4
	minYear    = 1000
	maxYear    = 9999
)

// discFolder matches a folder that holds one disc of a multi-disc album.
var discFolder = regexp.MustCompile(`(?i)^(cd|dis[ck])[ _.-]?\d{1,2}$`)

// FileInfo is what the scanner knows about a file besides its tags.
type FileInfo struct {
	Root, RelPath string
	Size          int64
	MTime         time.Time
	Digest        *[32]byte // Nil when the file was not hashed.
}

// NewTrackWrite maps an opened file and its file facts to a store write. It
// never fails: missing tags give zero values, and a missing title falls back to
// the file name. ID and ULID stay zero for the scanner to assign.
func NewTrackWrite(f *audiometa.File, fi *FileInfo) store.TrackWrite {
	tw := store.TrackWrite{
		Root: fi.Root, RelPath: fi.RelPath, Size: fi.Size, MTime: fi.MTime, Digest: fi.Digest,
		Format: f.Format.String(), Codec: f.Audio.Codec,
		Duration: f.Audio.Duration, SampleRate: f.Audio.SampleRate, BitDepth: f.Audio.BitDepth,
		Channels: f.Audio.Channels, Bitrate: f.Audio.Bitrate, Lossless: f.Audio.Lossless,
		Compilation:  isCompilation(&f.Tags),
		Artists:      artists(&f.Tags),
		AlbumArtists: albumArtists(&f.Tags),
		Tags:         rawTags(&f.Tags),
	}
	if rg := f.Audio.ReplayGain; rg != nil {
		tw.ReplayGain = store.ReplayGain{TrackGain: rg.TrackGain, TrackPeak: rg.TrackPeak, AlbumGain: rg.AlbumGain, AlbumPeak: rg.AlbumPeak}
	}
	setTitle(&tw, &f.Tags)
	setNumbers(&tw, &f.Tags)
	setAlbum(&tw, &f.Tags)
	for _, w := range f.Warnings {
		// A warning can embed a file path that is not UTF-8, which the store's JSON encoding rejects.
		tw.Warnings = append(tw.Warnings, strings.ToValidUTF8(w.String(), "\uFFFD"))
	}
	return tw
}

// setTitle sets the title and its sort form, falling back to the file name.
func setTitle(tw *store.TrackWrite, t *audiometa.Tags) {
	tw.Title = strings.TrimSpace(t.Title)
	if tw.Title == "" {
		base := filepath.Base(tw.RelPath)
		tw.Title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	tw.SortTitle = cmp.Or(single(t, keysTitleSort), tw.Title)
}

// setNumbers copies the track, disc and year fields.
func setNumbers(tw *store.TrackWrite, t *audiometa.Tags) {
	tw.TrackNo, tw.TrackTotal = t.TrackNumber, t.TrackTotal
	tw.DiscNo, tw.DiscTotal = t.DiscNumber, t.DiscTotal
	tw.Year = t.Year
	tw.OriginalYear = leadingYear(cmp.Or(t.OriginalDate, single(t, keysOriginalDate)))
}

// leadingYear returns the four-digit year a date starts with, or 0.
func leadingYear(date string) int {
	date = strings.TrimSpace(date)
	if len(date) < yearDigits {
		return 0
	}
	y, err := strconv.Atoi(date[:yearDigits])
	if err != nil || y < minYear || y > maxYear {
		return 0
	}
	return y
}

// isCompilation reports whether the compilation flag is set.
func isCompilation(t *audiometa.Tags) bool {
	v := single(t, keysCompilation)
	return v == "1" || strings.EqualFold(v, "true")
}

// setAlbum builds the album row from the tags and the already mapped track.
func setAlbum(tw *store.TrackWrite, t *audiometa.Tags) {
	tw.Album = store.AlbumWrite{
		Name:        strings.TrimSpace(t.Album),
		MBID:        cmp.Or(single(t, keysAlbumMBID), strings.TrimSpace(t.MusicBrainzAlbumID)),
		Year:        tw.Year,
		Compilation: tw.Compilation,
	}
	tw.Album.SortName = cmp.Or(single(t, keysAlbumSort), tw.Album.Name)
	tw.Album.GroupKey = groupKey(tw)
	// After grouping, which still sees the real (possibly empty) name.
	tw.Album.Name = cmp.Or(tw.Album.Name, unknownAlbum)
	tw.Album.SortName = cmp.Or(tw.Album.SortName, unknownAlbum)
	tw.Album.Artist = albumArtistName(tw)
}

// Display fallbacks for an album with no title or no artist tags.
const (
	unknownAlbum   = "Unknown Album"
	unknownArtist  = "Unknown Artist"
	variousArtists = "Various Artists"
)

// albumArtistName returns the album's display artist: the album artists, joined
// with ", "; else Various Artists for a compilation; else the first track
// artist, which is stable because [groupKey] uses it too.
func albumArtistName(tw *store.TrackWrite) string {
	switch {
	case len(tw.AlbumArtists) > 0:
		names := make([]string, len(tw.AlbumArtists))
		for i := range tw.AlbumArtists {
			names[i] = tw.AlbumArtists[i].Name
		}
		return strings.Join(names, ", ")
	case tw.Compilation:
		return variousArtists
	case len(tw.Artists) > 0:
		return tw.Artists[0].Name
	}
	return unknownArtist
}

// artists maps the track artists, falling back to the single Artist field.
func artists(t *audiometa.Tags) []store.ArtistWrite {
	names := t.Artists
	if len(names) == 0 && t.Artist != "" {
		names = []string{t.Artist}
	}
	return artistWrites(names, multi(t, keysArtistSort), multi(t, keysArtistMBID))
}

// albumArtists maps the album artists, falling back to the typed AlbumArtist.
func albumArtists(t *audiometa.Tags) []store.ArtistWrite {
	names := multi(t, keysAlbumArtist)
	if len(names) == 0 && t.AlbumArtist != "" {
		names = []string{t.AlbumArtist}
	}
	return artistWrites(names, multi(t, keysAlbumArtistSort), multi(t, keysAlbumArtistMBID))
}

// artistWrites cleans names (trimmed; empty and repeated match keys dropped,
// first kept) and pairs sort names and MBIDs with them. An empty sort name
// becomes the name.
func artistWrites(names, sorts, mbids []string) []store.ArtistWrite {
	sorts, mbids = trimAll(sorts), trimAll(mbids)
	type kept struct {
		idx       int
		name, key string
	}
	var keep []kept
	seen := make(map[string]struct{}, len(names))
	for i, name := range names {
		name = strings.TrimSpace(name)
		key := MatchKey(name)
		if _, dup := seen[key]; key == "" || dup {
			continue
		}
		seen[key] = struct{}{}
		keep = append(keep, kept{idx: i, name: name, key: key})
	}
	out := make([]store.ArtistWrite, 0, len(keep))
	for _, k := range keep {
		out = append(out, store.ArtistWrite{
			MatchKey: k.key,
			Name:     k.name,
			SortName: cmp.Or(pair(sorts, k.idx, len(names), len(keep)), k.name),
			MBID:     pair(mbids, k.idx, len(names), len(keep)),
		})
	}
	return out
}

// pair returns the value for the name at idx. Values pair by index when there
// is one per name before dropping (n); when one name remains after dropping
// and there is exactly one value, that value belongs to it; otherwise the
// result is empty.
func pair(vals []string, idx, n, kept int) string {
	switch {
	case len(vals) == n:
		return vals[idx]
	case kept == 1 && len(vals) == 1:
		return vals[0]
	}
	return ""
}

// trimAll returns vs with each value trimmed.
func trimAll(vs []string) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = strings.TrimSpace(v)
	}
	return out
}

// rawTags lists every raw tag with its keys sorted and each key's values in
// order, untrimmed.
func rawTags(t *audiometa.Tags) []store.Tag {
	all := maps.Collect(t.All())
	var out []store.Tag
	for _, k := range slices.Sorted(maps.Keys(all)) {
		key := tagKey(k)
		for _, v := range all[k] {
			out = append(out, store.Tag{Key: key, Value: v})
		}
	}
	return out
}

// tagKey returns k as valid UTF-8. M4A atom names hold Latin-1 bytes
// ("\xa9nam"), which JSON cannot carry, so such a key is decoded as Latin-1:
// lossless and reversible.
func tagKey(k string) string {
	if utf8.ValidString(k) {
		return k
	}
	var sb strings.Builder
	for i := range len(k) {
		sb.WriteRune(rune(k[i]))
	}
	return sb.String()
}

// discParent returns the parent of dir when dir is a disc folder ("CD1").
func discParent(dir string) (string, bool) {
	if dir != "." && discFolder.MatchString(filepath.Base(dir)) {
		return filepath.Dir(dir), true
	}
	return "", false
}

// groupKey returns the key that groups tw's tracks into an album: the album
// artist, album name and folder, with disc folders collapsed into their parent.
// The album MBID is deliberately not part of it. On a real library, grouping by
// MBID split partly tagged albums and merged editions that shared a release ID.
func groupKey(tw *store.TrackWrite) string {
	var artistKey string
	switch {
	case len(tw.AlbumArtists) > 0:
		artistKey = tw.AlbumArtists[0].MatchKey
	case tw.Compilation:
		artistKey = "\x00various"
	case len(tw.Artists) > 0:
		artistKey = tw.Artists[0].MatchKey
	}
	dir := filepath.Dir(tw.RelPath)
	if parent, ok := discParent(dir); ok {
		dir = parent
	}
	folder := filepath.Join(tw.Root, dir)
	return "tag:" + artistKey + "\x1f" + MatchKey(tw.Album.Name) + "\x1f" + folder
}
