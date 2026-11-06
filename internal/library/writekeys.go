package library

import (
	"errors"
	"fmt"
	"io"
	"os"
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

// The fields of the table.
const (
	FieldTitle  Field = "title"
	FieldTrack  Field = "track"
	FieldDisc   Field = "disc"
	FieldDate   Field = "date"
	FieldGenres Field = "genres"
)

// Edit is a change to a file's tags.
type Edit struct {
	// Fields holds new values. A field it doesn't name stays as it is, and one
	// named with no values is removed.
	Fields map[Field][]string
}

// family is a way of naming tags: the formats that share one have the same keys.
type family int

const (
	vorbis family = iota
	id3v23        // Also ID3v2.2, whose frames audiometa reports by their ID3v2.3 names.
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
	multi      bool // It may hold several values.
	required   bool // It can't be removed.
	// valid says what a value must look like. Nil takes any.
	valid func(family, string) error
	// read returns the file's values. Nil means the first of the keys with any.
	read func(*audiometa.Tags) []string
	// write saves values, never empty. Nil means [writeKeys].
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

// Keys with a meaning of their own in the date's writers.
const (
	keyYear = "YEAR"
	keyTYER = "TYER"
	keyTDAT = "TDAT"
)

// fields is the table, in the order an edit applies: a field comes before those
// its dependents overlap, so that one request can change both. Raw names are
// checked by TestEditLandsInEveryOwnedKey on files carrying the duplicate keys
// of a real library. Only formats audiometa can write have a family.
//
//nolint:gochecknoglobals // A fixed table, never written.
var fields = []field{
	{
		name: FieldTitle, flac: keys("TITLE"), mp3: keys("TIT2"), m4a: keys("\xA9nam"), required: true,
		dependents: []string{"TITLESORT", "TITLESORTEN", "TSOT", "sonm"},
	},
	number(FieldTrack, trackNumber, keys("TRACKNUMBER", "TRACK"), keys("TRCK"), keys("trkn")),
	number(FieldDisc, discNumber, keys("DISCNUMBER", "DISC"), keys("TPOS"), keys("disk")),
	{
		name: FieldDate, flac: keys("DATE", keyYear), mp3: keys("TDRC"), mp3v23: keys(keyTYER), m4a: keys("\xA9day"),
		dependents: []string{"TDRC", keyTYER, keyTDAT, "TIME"},
		valid:      validReleaseDate, read: readDate, write: writeDate,
	},
	{
		name: FieldGenres, flac: keys("GENRE"), mp3: keys("TCON"), m4a: keys("\xA9gen"), multi: true,
		dependents: []string{"gnre"},
	},
}

func keys(names ...string) []string { return names }

func trackNumber(t *audiometa.Tags) *int { return &t.TrackNumber }
func discNumber(t *audiometa.Tags) *int  { return &t.DiscNumber }

// number is the row of a numeric field. Its typed field reads every format,
// where a Vorbis value can be "3/12", and writes the two that keep a number and
// a total together. Vorbis keeps them apart, under the raw keys.
func number(name Field, pick func(*audiometa.Tags) *int, flac, mp3, m4a []string) field {
	return field{
		name: name, flac: flac, mp3: mp3, m4a: m4a, valid: validNumber,
		read: func(t *audiometa.Tags) []string {
			if n := *pick(t); n != 0 {
				return []string{strconv.Itoa(n)}
			}
			return nil
		},
		write: func(t *audiometa.Tags, fam family, keys, values []string) {
			if fam == vorbis {
				writeKeys(t, keys, values)
				return
			}
			*pick(t), _ = strconv.Atoi(values[0]) // validNumber read it.
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
func readDate(t *audiometa.Tags) []string {
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

// writeKeys saves values to the first of keys, and to each of the others that exists.
func writeKeys(t *audiometa.Tags, keys, values []string) {
	t.Set(keys[0], values...)
	for _, key := range keys[1:] {
		if t.Get(key) != nil {
			t.Set(key, values...)
		}
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
		return f.read(t)
	}
	return firstValues(t, f.keys(fam))
}

// check refuses values the field can't hold, whatever the file has now.
func (f *field) check(fam family, values []string) error {
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
	if slices.Equal(values, f.values(t, fam)) {
		return nil
	}
	t.Delete(f.dependents...)
	keys := f.keys(fam)
	switch {
	case len(values) == 0:
		t.Delete(keys...)
	case f.write != nil:
		f.write(t, fam, keys, values)
	default:
		writeKeys(t, keys, values)
	}
	return nil
}

// clean returns values trimmed, without the empty ones.
func clean(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// firstValues returns the cleaned values of the first of keys that has any.
func firstValues(t *audiometa.Tags, keys []string) []string {
	for _, key := range keys {
		if v := clean(t.Get(key)); len(v) > 0 {
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
	return nil
}

// readsFromID3v1 reports whether f's names came from an ID3v1 trailer. Any
// ID3v2 title or artist switches that fallback off, so the first edit would
// silently take the other trailer fields away.
func readsFromID3v1(f *audiometa.File) bool {
	t := &f.Tags
	return f.Format == audiometa.FormatMP3 && (t.Title != "" || t.Artist != "") && multi(t, keysID3v2Names) == nil
}
