package library

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	oldDate  = "2001-05-12"
	newTitle = "New Title"
	newYear  = 1987
	oldSort  = "Old Sort"
	keyDate  = "DATE"
	keyGenre = "GENRE"
	rock     = "Rock"
	pop      = "Pop"
)

// reread maps the file at path as a scan would.
func reread(t *testing.T, path string) store.TrackWrite {
	t.Helper()
	f, err := audiometa.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // Read-only use is done; nothing to report.
	digest, err := f.AudioDigest()
	if err != nil {
		t.Fatal(err)
	}
	return NewTrackWrite(f, &FileInfo{Root: filepath.Dir(path), RelPath: filepath.Base(path), Digest: digest})
}

// rawValues returns tw's raw values for key, as the store keeps it.
func rawValues(tw *store.TrackWrite, key string) []string {
	var out []string
	for _, tag := range tw.Tags {
		if tag.Key == key {
			out = append(out, tag.Value)
		}
	}
	return out
}

// seed writes raw tags into the file at path, as other taggers leave them.
func seed(t *testing.T, path string, tags map[string][]string) {
	t.Helper()
	err := audiometa.Write(path, func(f *audiometa.File) error {
		for k, v := range tags {
			f.Tags.Set(k, v...)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// ownedKeys says what an edit of every field must leave in one file.
type ownedKeys struct {
	year  []string // Keys that must hold the new year and nothing else.
	genre string
	gone  string // The sort key, which a new title deletes.
	kept  []string
}

// check compares what the edit left in got with the file before it.
func (k *ownedKeys) check(t *testing.T, before, got *store.TrackWrite, genres []string) {
	t.Helper()
	for _, key := range k.year {
		if v := rawValues(got, key); !slices.Equal(v, []string{"1987"}) {
			t.Errorf("%s = %q, want the year alone", key, v)
		}
	}
	if v := rawValues(got, k.genre); !slices.Equal(v, genres) {
		t.Errorf("%s = %q, want %q", k.genre, v, genres)
	}
	if v := rawValues(got, k.gone); v != nil {
		t.Errorf("%s = %q, want it gone", k.gone, v)
	}
	for _, key := range k.kept {
		if v := rawValues(got, key); len(v) == 0 || !slices.Equal(v, rawValues(before, key)) {
			t.Errorf("%s = %q, want it untouched", key, v)
		}
	}
}

func TestEditLandsInEveryOwnedKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, sample string
		seed         map[string][]string
		want         ownedKeys
	}{
		{
			name: formatFLAC, sample: sampleFLAC,
			seed: map[string][]string{
				keyDate: {oldDate}, "YEAR": {"2001"}, keysTitleSort[0]: {oldSort}, keyGenre: {rock, pop},
				"ALBUM ARTIST": {otherName}, "TOTALTRACKS": {"12"}, "TOTALDISCS": {"2"}, "ARTISTS": {otherName},
			},
			want: ownedKeys{
				year: []string{keyDate, "YEAR"}, genre: keyGenre, gone: keysTitleSort[0],
				kept: []string{"ALBUM ARTIST", "TOTALTRACKS", "TOTALDISCS", "ARTISTS"},
			},
		},
		{
			name: sampleMP3, sample: sampleMP3,
			seed: map[string][]string{
				"TDRC": {oldDate}, keysTitleSort[1]: {oldSort}, "TCON": {rock, pop}, keysArtistSort[1]: {otherName},
			},
			want: ownedKeys{year: []string{"TDRC"}, genre: "TCON", gone: keysTitleSort[1], kept: []string{keysArtistSort[1]}},
		},
		{
			name: formatM4A, sample: sampleM4A,
			seed: map[string][]string{
				"\xA9day": {oldDate}, keysTitleSort[2]: {oldSort}, "\xA9gen": {rock, pop}, keysArtistSort[2]: {otherName},
			},
			want: ownedKeys{year: []string{"©day"}, genre: "©gen", gone: keysTitleSort[2], kept: []string{keysArtistSort[2]}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, tt.sample)
			seed(t, path, tt.seed)
			before := reread(t, path)
			edit := Edit{
				Title: new(newTitle), TrackNo: new(7), DiscNo: new(2), Year: new(newYear),
				Genres: []string{"Jazz", "Blues"},
			}
			if err := audiometa.Write(path, edit.apply); err != nil {
				t.Fatalf("Write: %v", err)
			}
			got := reread(t, path)
			if got.Title != newTitle || got.SortTitle != newTitle || got.TrackNo != 7 || got.DiscNo != 2 || got.Year != newYear {
				t.Errorf("track = %q (sort %q) %d/%d, year %d", got.Title, got.SortTitle, got.TrackNo, got.DiscNo, got.Year)
			}
			if got.Digest != before.Digest {
				t.Error("the edit changed the audio digest")
			}
			tt.want.check(t, &before, &got, edit.Genres)
		})
	}
}

func TestEditWritesAYearAFileLacks(t *testing.T) {
	t.Parallel()
	for _, sample := range []string{sampleFLAC, sampleMP3, sampleM4A} {
		t.Run(sample, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, sample)
			err := audiometa.Write(path, func(f *audiometa.File) error {
				f.Tags.Year, f.Tags.Date = 0, ""
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			edit := Edit{Year: new(newYear)}
			if err = audiometa.Write(path, edit.apply); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if got := reread(t, path); got.Year != newYear {
				t.Errorf("Year = %d, want %d", got.Year, newYear)
			}
		})
	}
}

func TestSameValuesKeepWhatTheFileHas(t *testing.T) {
	t.Parallel()
	path := copySample(t, sampleFLAC)
	seed(t, path, map[string][]string{"TITLE": {"Same"}, keysTitleSort[0]: {"Same, The"}, keyDate: {oldDate}, keyGenre: {rock}})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edit := Edit{Title: new("Same"), Year: new(2001), Genres: []string{rock}}
	if err = audiometa.Write(path, edit.apply); err != nil {
		t.Fatalf("Write: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before, after) {
		t.Error("an edit to the same values rewrote the file")
	}
}

func TestEmptyGenresRemoveTheKey(t *testing.T) {
	t.Parallel()
	path := copySample(t, sampleFLAC)
	seed(t, path, map[string][]string{keyGenre: {rock}})
	edit := Edit{Genres: []string{}}
	if err := audiometa.Write(path, edit.apply); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := reread(t, path)
	if v := rawValues(&got, keyGenre); v != nil {
		t.Errorf("%s = %q, want it gone", keyGenre, v)
	}
}

// id3v1Only rewrites the MP3 at path so its only tag is an ID3v1 trailer.
func id3v1Only(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const headerLen = 10
	size := 0
	for _, c := range b[6:headerLen] { // Sync-safe: seven bits a byte.
		size = size<<7 | int(c)
	}
	trailer := make([]byte, 128)
	copy(trailer, "TAG")
	copy(trailer[3:], "Old Title")
	copy(trailer[33:], "Old Artist")
	//nolint:gosec // G703: path is a temp file from copySample.
	if err = os.WriteFile(path, append(b[headerLen+size:], trailer...), fileMode); err != nil {
		t.Fatal(err)
	}
}

func TestEditRefuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, sample string
		prepare      func(*testing.T, string)
		edit         Edit
	}{
		{name: "empty title", sample: sampleFLAC, edit: Edit{Title: new("")}},
		{name: "blank title", sample: sampleMP3, edit: Edit{Title: new(" \t ")}},
		{name: "ID3v1 only", sample: sampleMP3, prepare: id3v1Only, edit: Edit{Title: new(newTitle)}},
		{name: "a year MP3 can't read back", sample: sampleMP3, edit: Edit{Year: new(1850)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, tt.sample)
			if tt.prepare != nil {
				tt.prepare(t, path)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = audiometa.Write(path, tt.edit.apply); !errors.Is(err, ErrUnsupportedEdit) {
				t.Errorf("Write = %v, want ErrUnsupportedEdit", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(before, after) {
				t.Error("a refused edit changed the file")
			}
		})
	}
}

func TestFormatWithoutWriteKeysIsRefused(t *testing.T) {
	t.Parallel()
	edit := Edit{Title: new(newTitle)}
	err := edit.apply(&audiometa.File{Format: audiometa.FormatOgg})
	if !errors.Is(err, ErrUnsupportedEdit) || !strings.Contains(err.Error(), "Ogg") {
		t.Errorf("apply = %v, want ErrUnsupportedEdit naming the format", err)
	}
}
