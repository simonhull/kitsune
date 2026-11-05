package library

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/simonhull/audiometa"
)

// ErrUnsupportedEdit means a file can't take an edit, whatever its permissions.
var ErrUnsupportedEdit = errors.New("the file can't take this edit")

// Edit is a change to a file's tags. A nil field stays as it is. Empty Genres
// removes the genres.
type Edit struct {
	Title           *string
	TrackNo, DiscNo *int
	Year            *int
	Genres          []string
}

// writeKeys lists the raw keys the editable fields own in one format, beside
// the ones a typed field of [audiometa.Tags] writes for itself. Track and disc
// numbers need no entry: their typed fields own them. ID3 and M4A keep a total in
// the same tag, so it survives; a Vorbis "3/12" loses "/12", which Kitsune never read.
type writeKeys struct {
	// titleSort is the sort form of the title. A new title deletes it: the
	// form was made for the old title, and on ID3v2.3 the frame can't be written.
	titleSort string
	// year holds the keys that carry a year or a whole date. A new year replaces
	// each one that exists: a date keeps nothing of its month and day.
	year []string
	// dateParts holds what is left of a full date beside year. A new year deletes it.
	dateParts []string
	// minYear and maxYear are what the format's reader takes back.
	minYear, maxYear int
	// genres is the key genres are written to.
	genres string
	// genreCodes holds numeric genres, which genres replaces.
	genreCodes []string
}

// The years the MP3 reader accepts.
const (
	id3MinYear = 1900
	id3MaxYear = 2100
)

// Raw keys per format, checked by TestEditLandsInEveryOwnedKey on files carrying
// the duplicate keys of a real library. Only formats audiometa can write are in
// writeTable.
//
//nolint:gochecknoglobals // Fixed lookup tables, never written.
var (
	m4aWriteKeys = writeKeys{
		titleSort: "sonm",
		year:      []string{"\xA9day"},
		minYear:   minYear, maxYear: maxYear,
		genres:     "\xA9gen",
		genreCodes: []string{"gnre"},
	}
	writeTable = map[audiometa.Format]*writeKeys{
		audiometa.FormatFLAC: {
			titleSort: "TITLESORT",
			year:      []string{"DATE", "YEAR"},
			minYear:   minYear, maxYear: maxYear,
			genres: "GENRE",
		},
		audiometa.FormatMP3: {
			titleSort: "TSOT",
			year:      []string{"TDRC", "TYER"},
			dateParts: []string{"TDAT"},
			minYear:   id3MinYear, maxYear: id3MaxYear,
			genres: "TCON",
		},
		audiometa.FormatM4A: &m4aWriteKeys,
		audiometa.FormatM4B: &m4aWriteKeys,
	}
	// keysID3v2Names hold the two fields whose absence makes the MP3 reader fall
	// back to an ID3v1 trailer.
	keysID3v2Names = []string{"TIT2", "TPE1"}
)

// apply makes the edit on f's tags, for [audiometa.Write] to save. A field that
// already holds its new value is left alone, so saving an unchanged form keeps a
// full date and a hand-made sort title. Its errors wrap [ErrUnsupportedEdit].
func (e *Edit) apply(f *audiometa.File) error {
	keys, ok := writeTable[f.Format]
	if !ok {
		return fmt.Errorf("%s: %w", f.Format, ErrUnsupportedEdit)
	}
	if readsFromID3v1(f) {
		return fmt.Errorf("the tags are ID3v1 only: %w", ErrUnsupportedEdit)
	}
	t := &f.Tags
	if e.TrackNo != nil {
		t.TrackNumber = *e.TrackNo
	}
	if e.DiscNo != nil {
		t.DiscNumber = *e.DiscNo
	}
	keys.setGenres(t, e.Genres)
	if err := keys.setTitle(t, e.Title); err != nil {
		return err
	}
	return keys.setYear(t, e.Year)
}

// readsFromID3v1 reports whether f's names came from an ID3v1 trailer. Any
// ID3v2 title or artist switches that fallback off, so the first edit would
// silently take the other trailer fields away.
func readsFromID3v1(f *audiometa.File) bool {
	t := &f.Tags
	return f.Format == audiometa.FormatMP3 && (t.Title != "" || t.Artist != "") && multi(t, keysID3v2Names) == nil
}

func (k *writeKeys) setTitle(t *audiometa.Tags, title *string) error {
	if title == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*title)
	if trimmed == "" {
		return fmt.Errorf("empty title: %w", ErrUnsupportedEdit)
	}
	if trimmed != t.Title {
		t.Title = trimmed
		t.Delete(k.titleSort)
	}
	return nil
}

func (k *writeKeys) setGenres(t *audiometa.Tags, genres []string) {
	if genres != nil && !slices.Equal(genres, t.Genres) {
		t.Set(k.genres, genres...)
		t.Delete(k.genreCodes...)
	}
}

// setYear writes year over every key that holds one. The typed field covers a
// file with none, through the format's usual key; alone it would keep the old
// month and day, and only in the first key.
func (k *writeKeys) setYear(t *audiometa.Tags, year *int) error {
	if year == nil || *year == t.Year {
		return nil
	}
	if *year < k.minYear || *year > k.maxYear {
		return fmt.Errorf("year %d: %w", *year, ErrUnsupportedEdit)
	}
	t.Year = *year
	for _, key := range k.year {
		if t.Get(key) != nil {
			t.Set(key, strconv.Itoa(*year))
		}
	}
	t.Delete(k.dateParts...)
	return nil
}
