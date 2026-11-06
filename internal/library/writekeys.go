package library

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/simonhull/audiometa"
)

// ErrUnsupportedEdit means a file can't take an edit, whatever its permissions.
var ErrUnsupportedEdit = errors.New("the file can't take this edit")

// Field names an editable tag, the same in every format.
type Field string

// The fields of the table. Album, artists, album artists and compilation can be
// read but not edited yet: they decide which album a track groups into.
const (
	FieldTitle           Field = "title"
	FieldAlbum           Field = "album"
	FieldArtists         Field = "artists"
	FieldAlbumArtists    Field = "albumArtists"
	FieldCompilation     Field = "compilation"
	FieldTrack           Field = "track"
	FieldTrackTotal      Field = "trackTotal"
	FieldDisc            Field = "disc"
	FieldDiscTotal       Field = "discTotal"
	FieldDate            Field = "date"
	FieldGenres          Field = "genres"
	FieldComposers       Field = "composers"
	FieldConductor       Field = "conductor"
	FieldLyricist        Field = "lyricist"
	FieldPerformers      Field = "performers"
	FieldRemixer         Field = "remixer"
	FieldGrouping        Field = "grouping"
	FieldLabel           Field = "label"
	FieldCatalogNumber   Field = "catalogNumber"
	FieldBarcode         Field = "barcode"
	FieldISRC            Field = "isrc"
	FieldOriginalDate    Field = "originalDate"
	FieldLanguage        Field = "language"
	FieldBPM             Field = "bpm"
	FieldKey             Field = "key"
	FieldCopyright       Field = "copyright"
	FieldComment         Field = "comment"
	FieldLyrics          Field = "lyrics"
	FieldSortTitle       Field = "sortTitle"
	FieldSortArtist      Field = "sortArtist"
	FieldSortAlbum       Field = "sortAlbum"
	FieldSortAlbumArtist Field = "sortAlbumArtist"
)

// Edit is a change to a file's tags.
type Edit struct {
	// Fields holds new values. A field it doesn't name stays as it is, and one
	// named with no values is removed.
	Fields map[Field][]string
	// Tags does the same for raw tags, by the keys [TrackTags] shows.
	Tags map[string][]string
}

// Empty reports whether the edit names nothing.
func (e *Edit) Empty() bool { return len(e.Fields) == 0 && len(e.Tags) == 0 }

// over returns e with own on top: what both name takes own's value.
func (e *Edit) over(own *Edit) Edit {
	return Edit{Fields: merged(e.Fields, own.Fields), Tags: merged(e.Tags, own.Tags)}
}

func merged[K comparable](base, top map[K][]string) map[K][]string {
	out := make(map[K][]string, len(base)+len(top))
	maps.Copy(out, base)
	maps.Copy(out, top)
	return out
}

// lyricTimestamp matches the time code that starts a line of synced lyrics.
var lyricTimestamp = regexp.MustCompile(`(?m)^\s*\[\d+:\d{2}([.:]\d+)?\]`)

// family is a way of naming tags: the formats that share one have the same keys.
type family int

const (
	vorbis family = iota
	// id3v23 also covers ID3v2.2, whose frames audiometa reports by their ID3v2.3
	// names. It rewrites such a tag as ID3v2.4, so a month, a day or an original
	// date is refused there.
	id3v23
	id3v24
	mp4
)

// field is one row of the table: where a field lives in each format, and what
// follows from changing it.
type field struct {
	name Field
	// flac, mp3 and m4a are the keys in each format. The first is the one
	// written; the others are mirrors, kept in step only where they exist.
	flac, mp3, m4a []string
	// mp3v23 replaces mp3 on an ID3v2.3 tag, which lacks some frames.
	mp3v23 []string
	// dependents are deleted on a change, in every format: they were made for the old value.
	dependents []string
	// matching are copies that a change follows only while they equal the old value.
	matching []string
	identity bool // It decides which album a track groups into, so it can't be edited yet.
	multi    bool // It may hold several values.
	required bool // It can't be removed.
	// frozen reports values a client may see but not replace.
	frozen func([]string) bool
	// valid says what a value must look like. Nil takes any.
	valid func(family, string) error
	// read returns the file's values. Nil means the first of the keys with any.
	read func(t *audiometa.Tags, keys []string) []string
	// write saves values, or with none removes the field. Nil means [writeKeys].
	write func(t *audiometa.Tags, fam family, keys, values []string)
}

// The numbers a track or disc tag can hold in every format: M4A's are 16 bits.
const (
	minNumber = 1
	maxNumber = 65535
)

// The years the MP3 reader accepts.
const (
	id3MinYear = 1900
	id3MaxYear = 2100
)

// Keys the table's functions treat apart.
const (
	keyYear    = "YEAR"
	keyTYER    = "TYER"
	keyTDAT    = "TDAT"
	keyTORY    = "TORY"
	keyComment = "COMM" // MP3: audiometa keeps it out of the raw tags.
	keyLyrics  = "USLT" // MP3: audiometa takes it through the typed field.
)

// fields is the table, in the order an edit applies: a field comes before those
// among its dependents, so that one request can change both. Raw names are checked
// by TestEveryFieldRoundTrips and TestEditLandsInEveryOwnedKey, on files carrying
// the duplicate keys of a real library. Only formats audiometa can write have a
// family.
//
//nolint:gochecknoglobals // A fixed table, never written.
var fields = []field{
	{
		name: FieldTitle, flac: keys("TITLE"), mp3: keys("TIT2"), m4a: keys("\xA9nam"), required: true,
		dependents: []string{"TITLESORT", "TITLESORTEN", "TSOT", "sonm"},
	},
	{
		name: FieldAlbum, flac: keys("ALBUM"), mp3: keys("TALB"), m4a: keys("\xA9alb"), identity: true,
		dependents: []string{"ALBUMSORT", "ALBUMSORTEN", "TSOA", "soal"},
	},
	{
		name: FieldArtists, flac: keys("ARTIST", "ARTISTS"), mp3: keys("TPE1", "ARTISTS"), m4a: keys("\xA9ART", "ARTISTS"),
		identity: true, multi: true,
		dependents: slices.Concat(keysArtistSort, keysArtistMBID, keys("ARTISTS_SORT", "ARTIST_CREDIT", "ARTISTS_CREDIT", "ARTIST CREDIT")),
	},
	{
		name:     FieldAlbumArtists,
		flac:     keys("ALBUMARTIST", "ALBUM ARTIST", "ALBUM_ARTIST", "ALBUMARTISTS", "ALBUM_ARTISTS"),
		mp3:      keys("TPE2", "ALBUMARTIST", "ALBUM ARTIST", "ALBUM_ARTIST", "ALBUMARTISTS", "ALBUM_ARTISTS"),
		m4a:      keys("aART"),
		identity: true, multi: true, read: candidates(keysAlbumArtist),
		dependents: slices.Concat(keysAlbumArtistSort, keysAlbumArtistMBID, keys("ALBUMARTISTS_SORT", "ALBUMARTIST_CREDIT", "ALBUMARTISTS_CREDIT", "ALBUM ARTIST CREDIT")),
	},
	{name: FieldCompilation, flac: keys("COMPILATION"), mp3: keys("TCMP"), m4a: keys("cpil"), identity: true},
	number(FieldTrack, trackNumber, keys("TRACKNUMBER", "TRACK"), keys("TRCK"), keys("trkn")),
	number(FieldTrackTotal, trackTotal, keys("TRACKTOTAL", "TOTALTRACKS", "TRACKC"), keys("TRCK"), keys("trkn")),
	number(FieldDisc, discNumber, keys("DISCNUMBER", "DISC"), keys("TPOS"), keys("disk")),
	number(FieldDiscTotal, discTotal, keys("DISCTOTAL", "TOTALDISCS", "DISCC"), keys("TPOS"), keys("disk")),
	{
		name: FieldDate, flac: keys("DATE", keyYear), mp3: keys("TDRC"), mp3v23: keys(keyTYER, keyTDAT), m4a: keys("\xA9day"),
		dependents: []string{"TDRC", keyTYER, keyTDAT, "TIME"},
		valid:      validReleaseDate, read: readDate, write: writeDate,
	},
	{
		name: FieldGenres, flac: keys("GENRE"), mp3: keys("TCON"), m4a: keys("\xA9gen"), multi: true,
		dependents: []string{"gnre"},
	},
	{
		name: FieldComposers, flac: keys("COMPOSER"), mp3: keys("TCOM"), m4a: keys("\xA9wrt"), multi: true,
		dependents: []string{"COMPOSERSORT"},
	},
	{name: FieldConductor, flac: keys("CONDUCTOR"), mp3: keys("TPE3"), m4a: keys("CONDUCTOR"), multi: true},
	{name: FieldLyricist, flac: keys("LYRICIST"), mp3: keys("TEXT"), m4a: keys("LYRICIST"), multi: true},
	// MP3 and M4A have no standard home for performers: these are custom keys.
	{name: FieldPerformers, flac: keys("PERFORMER"), mp3: keys("PERFORMER"), m4a: keys("PERFORMER"), multi: true},
	{name: FieldRemixer, flac: keys("REMIXER"), mp3: keys("TPE4"), m4a: keys("REMIXER"), multi: true},
	{name: FieldGrouping, flac: keys("GROUPING"), mp3: keys("GRP1", "TIT1"), m4a: keys("\xA9grp")},
	{name: FieldLabel, flac: keys("LABEL", "PUBLISHER"), mp3: keys("TPUB"), m4a: keys("LABEL"), multi: true},
	{name: FieldCatalogNumber, flac: keys("CATALOGNUMBER"), mp3: keys("CATALOGNUMBER"), m4a: keys("CATALOGNUMBER"), multi: true},
	{name: FieldBarcode, flac: keys("BARCODE"), mp3: keys("BARCODE"), m4a: keys("BARCODE"), multi: true},
	{name: FieldISRC, flac: keys("ISRC"), mp3: keys("TSRC"), m4a: keys("ISRC"), multi: true},
	{
		name: FieldOriginalDate, flac: keys("ORIGINALDATE"), mp3: keys("TDOR"), mp3v23: keys(keyTORY), m4a: keys("ORIGINALDATE"),
		dependents: []string{"TDOR", keyTORY},
		valid:      validDate, read: candidates(keysOriginalDate), write: writeOriginalDate,
	},
	{name: FieldLanguage, flac: keys("LANGUAGE"), mp3: keys("TLAN"), m4a: keys("LANGUAGE"), multi: true},
	{name: FieldBPM, flac: keys("BPM"), mp3: keys("TBPM"), m4a: keys("tmpo"), valid: validBPM},
	{name: FieldKey, flac: keys("INITIALKEY"), mp3: keys("TKEY"), m4a: keys("INITIALKEY")},
	{name: FieldCopyright, flac: keys("COPYRIGHT"), mp3: keys("TCOP"), m4a: keys("cprt")},
	{name: FieldComment, flac: keys("COMMENT", "DESCRIPTION"), mp3: keys(keyComment), m4a: keys("\xA9cmt")},
	{
		name: FieldLyrics, flac: keys("LYRICS"), mp3: keys(keyLyrics), m4a: keys("\xA9lyr"),
		matching: []string{"UNSYNCEDLYRICS"}, frozen: timestamped,
	},
	{name: FieldSortTitle, flac: keys("TITLESORT"), mp3: keys("TSOT"), m4a: keys("sonm")},
	{name: FieldSortArtist, flac: keys("ARTISTSORT"), mp3: keys("TSOP"), m4a: keys("soar")},
	{name: FieldSortAlbum, flac: keys("ALBUMSORT"), mp3: keys("TSOA"), m4a: keys("soal")},
	{
		name: FieldSortAlbumArtist, flac: keys("ALBUMARTISTSORT"), mp3: keys("TSO2", "ALBUMARTISTSORT"), m4a: keys("soaa"),
		read: candidates(keysAlbumArtistSort),
	},
}

func keys(names ...string) []string { return names }

func trackNumber(t *audiometa.Tags) *int { return &t.TrackNumber }
func trackTotal(t *audiometa.Tags) *int  { return &t.TrackTotal }
func discNumber(t *audiometa.Tags) *int  { return &t.DiscNumber }
func discTotal(t *audiometa.Tags) *int   { return &t.DiscTotal }

// candidates reads a field through the same keys, in the same order, as a scan.
func candidates(names []string) func(*audiometa.Tags, []string) []string {
	return func(t *audiometa.Tags, _ []string) []string { return firstValues(t, names) }
}

// timestamped reports lyrics with time codes, which a plain text box would ruin.
func timestamped(values []string) bool {
	return slices.ContainsFunc(values, lyricTimestamp.MatchString)
}

// number is the row of a numeric field. Its typed field reads every format, since
// a Vorbis value can be "3/12", and writes the two that keep a number and a total
// together. Vorbis keeps them apart, under the raw keys, and audiometa reads only
// some: a value held only by a mirror is still the field's.
func number(name Field, pick func(*audiometa.Tags) *int, flac, mp3, m4a []string) field {
	return field{
		name: name, flac: flac, mp3: mp3, m4a: m4a, valid: validNumber,
		read: func(t *audiometa.Tags, keys []string) []string {
			if n := *pick(t); n != 0 {
				return []string{strconv.Itoa(n)}
			}
			return firstValues(t, keys[1:])
		},
		write: func(t *audiometa.Tags, fam family, keys, values []string) {
			if fam == vorbis {
				writeKeys(t, keys, values)
				return
			}
			*pick(t) = 0
			if len(values) > 0 {
				*pick(t), _ = strconv.Atoi(values[0]) // validNumber read it.
			}
		},
	}
}

func validNumber(_ family, v string) error {
	n, err := strconv.Atoi(v)
	if err != nil || n < minNumber || n > maxNumber {
		return fmt.Errorf("%q is not a number from %d to %d: %w", v, minNumber, maxNumber, ErrUnsupportedEdit)
	}
	return nil
}

// validBPM refuses what M4A's integer can't hold.
func validBPM(fam family, v string) error {
	if _, err := strconv.ParseInt(v, 10, 16); err != nil && fam == mp4 {
		return fmt.Errorf("%q is not a whole number: %w", v, ErrUnsupportedEdit)
	}
	return nil
}

// validDate takes a year, or a year and month, or a whole date.
func validDate(_ family, v string) error {
	for _, layout := range []string{"2006", "2006-01", time.DateOnly} {
		if _, err := time.Parse(layout, v); err == nil && leadingYear(v) != 0 {
			return nil
		}
	}
	return fmt.Errorf("%q is not a date: %w", v, ErrUnsupportedEdit)
}

// validReleaseDate is [validDate], within what the format's reader takes back as a year.
func validReleaseDate(fam family, v string) error {
	if err := validDate(fam, v); err != nil {
		return err
	}
	if y := leadingYear(v); (fam == id3v23 || fam == id3v24) && (y < id3MinYear || y > id3MaxYear) {
		return fmt.Errorf("year %d: %w", y, ErrUnsupportedEdit)
	}
	return nil
}

// readDate returns the date, joining an ID3v2.3 year to its TDAT, which holds
// the day and month as DDMM.
func readDate(t *audiometa.Tags, _ []string) []string {
	date := firstValues(t, keysDate)
	if len(date) == 0 {
		return nil
	}
	if ddmm := t.GetFirst(keyTDAT); len(date[0]) == yearDigits && len(ddmm) == yearDigits {
		return []string{date[0] + "-" + ddmm[2:] + "-" + ddmm[:2]}
	}
	return date
}

// writeDate replaces the whole date, in the keys the format holds one in. A year
// key takes the year alone.
func writeDate(t *audiometa.Tags, _ family, keys, values []string) {
	if len(values) == 0 {
		writeKeys(t, keys, nil)
		return
	}
	date := values[0]
	year := date[:yearDigits]
	switch {
	case keys[0] != keyTYER:
		t.Set(keys[0], date)
	case len(date) == len(time.DateOnly):
		t.Set(keyTYER, year)
		t.Set(keyTDAT, date[8:]+date[5:7])
	default:
		t.Set(keyTYER, year)
	}
	if t.Get(keyYear) != nil {
		t.Set(keyYear, year)
	}
}

// writeOriginalDate saves the date, or its year where the frame holds no more.
func writeOriginalDate(t *audiometa.Tags, _ family, keys, values []string) {
	if len(values) == 0 {
		writeKeys(t, keys, nil)
		return
	}
	date := values[0]
	if keys[0] == keyTORY {
		date = date[:yearDigits]
	}
	t.Set(keys[0], date)
}

// writeKeys saves values to the first of keys, and to each of the others that
// exists. With no values, that removes them all.
func writeKeys(t *audiometa.Tags, keys, values []string) {
	set(t, keys[0], values)
	for _, key := range keys[1:] {
		if t.Get(key) != nil {
			t.Set(key, values...)
		}
	}
}

// get returns the values of key, which may be one of the keys audiometa keeps
// behind a typed field.
func get(t *audiometa.Tags, key string) []string {
	switch key {
	case keyComment:
		return []string{t.Comment}
	case keyLyrics:
		return []string{t.Lyrics}
	}
	return t.Get(key)
}

// set is the writing side of [get].
func set(t *audiometa.Tags, key string, values []string) {
	switch key {
	case keyComment:
		t.Comment = strings.Join(values, "")
	case keyLyrics:
		t.Lyrics = strings.Join(values, "")
	default:
		t.Set(key, values...)
	}
}

// keys returns the field's keys in fam.
func (f *field) keys(fam family) []string {
	switch fam {
	case vorbis:
		return f.flac
	case id3v23:
		if f.mp3v23 != nil {
			return f.mp3v23
		}
		return f.mp3
	case id3v24:
		return f.mp3
	case mp4:
		return f.m4a
	}
	return nil
}

// values returns what the file holds. Empty values read as absent.
func (f *field) values(t *audiometa.Tags, fam family) []string {
	if f.read != nil {
		return f.read(t, f.keys(fam))
	}
	return firstValues(t, f.keys(fam))
}

// check refuses values the field can't hold, whatever the file has now.
func (f *field) check(fam family, values []string) error {
	if f.identity {
		return fmt.Errorf("%s can't be edited yet: %w", f.name, ErrUnsupportedEdit)
	}
	if f.required && len(values) == 0 {
		return fmt.Errorf("empty %s: %w", f.name, ErrUnsupportedEdit)
	}
	if !f.multi && len(values) > 1 {
		return fmt.Errorf("%s takes one value: %w", f.name, ErrUnsupportedEdit)
	}
	for _, v := range values {
		if strings.ContainsRune(v, 0) {
			return fmt.Errorf("%s holds a NUL: %w", f.name, ErrUnsupportedEdit)
		}
		if f.valid != nil {
			if err := f.valid(fam, v); err != nil {
				return fmt.Errorf("%s: %w", f.name, err)
			}
		}
	}
	return nil
}

// change saves values. A field that already holds them is left alone, so saving
// an unchanged form keeps a hand-made sort title.
func (f *field) change(t *audiometa.Tags, fam family, values []string) error {
	values = clean(values)
	if err := f.check(fam, values); err != nil {
		return err
	}
	old := f.values(t, fam)
	if slices.Equal(values, old) {
		return nil
	}
	if f.frozen != nil && f.frozen(old) {
		return fmt.Errorf("%s is read-only: %w", f.name, ErrUnsupportedEdit)
	}
	f.save(t, fam, old, values)
	return nil
}

// save replaces old with values.
func (f *field) save(t *audiometa.Tags, fam family, old, values []string) {
	t.Delete(f.dependents...)
	for _, key := range f.matching {
		if v := t.Get(key); v != nil && slices.Equal(clean(v), old) {
			t.Set(key, values...)
		}
	}
	if f.write != nil {
		f.write(t, fam, f.keys(fam), values)
		return
	}
	writeKeys(t, f.keys(fam), values)
}

// clean returns values trimmed, without the empty ones, as valid UTF-8.
func clean(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(strings.ToValidUTF8(v, "\uFFFD")); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// firstValues returns the cleaned values of the first of keys that has any.
func firstValues(t *audiometa.Tags, keys []string) []string {
	for _, key := range keys {
		if v := clean(get(t, key)); len(v) > 0 {
			return v
		}
	}
	return nil
}

// familyOf returns the family of f's format. Its errors wrap [ErrUnsupportedEdit]
// for a format audiometa can't write.
func familyOf(f *audiometa.File) (family, error) {
	switch f.Format {
	case audiometa.FormatFLAC:
		return vorbis, nil
	case audiometa.FormatMP3:
		return id3Family(f.Path)
	case audiometa.FormatM4A, audiometa.FormatM4B:
		return mp4, nil
	case audiometa.FormatUnknown, audiometa.FormatOgg, audiometa.FormatOpus, audiometa.FormatWAV, audiometa.FormatAIFF:
	}
	return 0, fmt.Errorf("%s: %w", f.Format, ErrUnsupportedEdit)
}

// id3Family reads the version of the ID3v2 tag at the start of the file at path,
// which audiometa doesn't report. A file without one gets ID3v2.4.
func id3Family(path string) (family, error) {
	file, err := os.Open(path) //nolint:gosec // G304: audiometa just opened this path.
	if err != nil {
		return 0, fmt.Errorf("read the ID3 version: %w", err)
	}
	defer func() { _ = file.Close() }() // Only read.
	header := make([]byte, len("ID3")+1)
	if _, err = io.ReadFull(file, header); err != nil {
		return 0, fmt.Errorf("read the ID3 version: %w", err)
	}
	if string(header[:3]) == "ID3" && header[3] < 4 {
		return id3v23, nil
	}
	return id3v24, nil
}

// apply makes the edit on f's tags, for [audiometa.Write] to save. Its errors
// wrap [ErrUnsupportedEdit], unless the file can't be read.
func (e *Edit) apply(f *audiometa.File) error {
	fam, err := familyOf(f)
	if err != nil {
		return err
	}
	if readsFromID3v1(f) {
		return fmt.Errorf("the tags are ID3v1 only: %w", ErrUnsupportedEdit)
	}
	for name := range e.Fields {
		if !slices.ContainsFunc(fields, func(f field) bool { return f.name == name }) {
			return fmt.Errorf("no field %q: %w", name, ErrUnsupportedEdit)
		}
	}
	for i := range fields {
		row := &fields[i]
		if values, named := e.Fields[row.name]; named {
			if changeErr := row.change(&f.Tags, fam, values); changeErr != nil {
				return changeErr
			}
		}
	}
	return e.applyTags(&f.Tags, fam)
}

// readsFromID3v1 reports whether f's names came from an ID3v1 trailer. Any
// ID3v2 title or artist switches that fallback off, so the first edit would
// silently take the other trailer fields away.
func readsFromID3v1(f *audiometa.File) bool {
	t := &f.Tags
	return f.Format == audiometa.FormatMP3 && (t.Title != "" || t.Artist != "") && multi(t, keysID3v2Names) == nil
}
