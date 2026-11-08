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
	keyAlbum = "ALBUM"
	keyGenre = "GENRE"
	rock     = "Rock"
	pop      = "Pop"
	jazz     = "Jazz"

	oldValue    = "Old"
	newValue    = "New"
	newDate     = "1987-03-04"
	aComment    = "A comment"
	someLyrics  = "Some lyrics"
	sortedTitle = "Sorted"
	albumValue  = "Album"
	artistValue = "Artist"

	keyTrackTotal   = "TRACKTOTAL"
	keyTrackC       = "TRACKC"
	keyComposer     = "COMPOSER"
	keyComposerSort = "COMPOSERSORT"
	keyTotalTracks  = "TOTALTRACKS"
	keyTotalDiscs   = "TOTALDISCS"
	keyVorbisLyrics = "LYRICS"
	keyUnsynced     = "UNSYNCEDLYRICS"
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
	return NewTrackWrite(f, &FileInfo{Root: filepath.Dir(path), RelPath: filepath.Base(path), Digest: &digest})
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
				"ALBUM ARTIST": {otherName}, keyTotalTracks: {"12"}, keyTotalDiscs: {"2"}, "ARTISTS": {otherName},
			},
			want: ownedKeys{
				year: []string{keyDate, "YEAR"}, genre: keyGenre, gone: keysTitleSort[0],
				kept: []string{"ALBUM ARTIST", keyTotalTracks, keyTotalDiscs, "ARTISTS"},
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
			genres := []string{jazz, "Blues"}
			edit := Edit{Fields: map[Field][]string{
				FieldTitle: {newTitle}, FieldTrack: {"7"}, FieldDisc: {"2"}, FieldDate: {"1987"}, FieldGenres: genres,
			}}
			if err := audiometa.Write(path, edit.apply); err != nil {
				t.Fatalf("Write: %v", err)
			}
			got := reread(t, path)
			if got.Title != newTitle || got.SortTitle != newTitle || got.TrackNo != 7 || got.DiscNo != 2 || got.Year != newYear {
				t.Errorf("track = %q (sort %q) %d/%d, year %d", got.Title, got.SortTitle, got.TrackNo, got.DiscNo, got.Year)
			}
			if *got.Digest != *before.Digest {
				t.Error("the edit changed the audio digest")
			}
			tt.want.check(t, &before, &got, genres)
		})
	}
}

func TestEditUpdatesNumbersRepeatedUnderOtherKeys(t *testing.T) {
	t.Parallel()
	path := copySample(t, sampleFLAC)
	seed(t, path, map[string][]string{"TRACK": {"3"}, "DISC": {"1"}})
	edit := Edit{Fields: map[Field][]string{FieldTrack: {"7"}, FieldDisc: {"2"}}}
	if err := audiometa.Write(path, edit.apply); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := reread(t, path)
	if v := rawValues(&got, "TRACK"); !slices.Equal(v, []string{"7"}) {
		t.Errorf("TRACK = %q, want the new number", v)
	}
	if v := rawValues(&got, "DISC"); !slices.Equal(v, []string{"2"}) {
		t.Errorf("DISC = %q, want the new number", v)
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
			if err = audiometa.Write(path, yearEdit().apply); err != nil {
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
	edit := Edit{Fields: map[Field][]string{FieldTitle: {"Same"}, FieldDate: {oldDate}, FieldGenres: {rock}}}
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
	edit := Edit{Fields: map[Field][]string{FieldGenres: {}}}
	if err := audiometa.Write(path, edit.apply); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := reread(t, path)
	if v := rawValues(&got, keyGenre); v != nil {
		t.Errorf("%s = %q, want it gone", keyGenre, v)
	}
}

// everyField holds a value for each field that can be edited.
func everyField() map[Field][]string {
	return map[Field][]string{
		FieldTitle: {newTitle}, FieldTrack: {"7"}, FieldTrackTotal: {"12"}, FieldDisc: {"2"}, FieldDiscTotal: {"3"},
		FieldDiscSubtitle: {discTwo}, FieldDate: {newDate}, FieldGenres: {jazz, "Blues"}, FieldComposers: {"Com One", "Com Two"},
		FieldConductor: {"Con One", "Con Two"}, FieldLyricist: {"Lyr One", "Lyr Two"},
		FieldPerformers: {"Per One", "Per Two"}, FieldRemixer: {"Rem One", "Rem Two"}, FieldGrouping: {"Group"},
		FieldLabel: {"Label One", "Label Two"}, FieldCatalogNumber: {"CAT-1", "CAT-2"}, FieldBarcode: {"123", "456"},
		FieldISRC: {"USABC1234567"}, FieldOriginalDate: {"1977-05-06"}, FieldLanguage: {"eng", "fra"},
		FieldBPM: {"120"}, FieldKey: {"Am"}, FieldCopyright: {copyright}, FieldComment: {aComment},
		FieldLyrics: {"Line one\nLine two"}, FieldSortTitle: {"Title, New"}, FieldSortArtist: {"Artist, Sort"},
		FieldSortAlbum: {"Album, Sort"}, FieldSortAlbumArtist: {"Album Artist, Sort"},
	}
}

// fieldsOf reads the file at path as an editor would.
func fieldsOf(t *testing.T, path string) *TrackTags {
	t.Helper()
	got, err := readTags(t.Context(), &store.EditFile{Root: filepath.Dir(path), RelPath: filepath.Base(path)})
	if err != nil {
		t.Fatal(err)
	}
	return &got
}

// wantEveryField checks a scan of the file at path sees what [everyField] set.
func wantEveryField(t *testing.T, path string, before *store.TrackWrite) {
	t.Helper()
	got := reread(t, path)
	type seen struct {
		title, sortTitle, discName                           string
		trackNo, trackTotal, discNo, discTotal, year, origin int
	}
	scanned := seen{
		title: got.Title, sortTitle: got.SortTitle, discName: got.DiscSubtitle,
		trackNo: got.TrackNo, trackTotal: got.TrackTotal, discNo: got.DiscNo, discTotal: got.DiscTotal,
		year: got.Year, origin: got.OriginalYear,
	}
	want := seen{
		title: newTitle, sortTitle: "Title, New", discName: discTwo,
		trackNo: 7, trackTotal: 12, discNo: 2, discTotal: 3, year: newYear, origin: 1977,
	}
	if scanned != want {
		t.Errorf("scan saw %+v, want %+v", scanned, want)
	}
	if *got.Digest != *before.Digest {
		t.Error("the edit changed the audio digest")
	}
}

func TestEveryFieldRoundTrips(t *testing.T) {
	t.Parallel()
	for _, sample := range []string{sampleFLAC, sampleMP3, sampleMP3v23, sampleM4A, sampleWAVInfo, sampleWAVBare, sampleWAVBoth, sampleAIFF} {
		t.Run(sample, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, sample)
			want := everyField()
			before := reread(t, path)
			edit := Edit{Fields: want}
			if err := audiometa.Write(path, edit.apply); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if sample == sampleMP3v23 || sample == sampleWAVBoth {
				want[FieldOriginalDate] = []string{"1977"} // TORY holds a year.
			}
			got := fieldsOf(t, path)
			for name, values := range want {
				if !slices.Equal(got.Fields[name], values) {
					t.Errorf("%s = %q, want %q", name, got.Fields[name], values)
				}
			}
			wantEveryField(t, path, &before)
		})
	}
}

// clearEdit removes every field [everyField] sets, but the title.
func clearEdit() Edit {
	edit := Edit{Fields: everyField()}
	delete(edit.Fields, FieldTitle)
	for name := range edit.Fields {
		edit.Fields[name] = nil
	}
	return edit
}

func TestEveryFieldClears(t *testing.T) {
	t.Parallel()
	for _, sample := range []string{sampleFLAC, sampleMP3, sampleMP3v23, sampleM4A, sampleWAVInfo, sampleWAVBoth, sampleAIFF} {
		t.Run(sample, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, sample)
			set, remove := Edit{Fields: everyField()}, clearEdit()
			if err := audiometa.Write(path, set.apply); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if err := audiometa.Write(path, remove.apply); err != nil {
				t.Fatalf("clear: %v", err)
			}
			got := fieldsOf(t, path)
			for name := range remove.Fields {
				if v, held := got.Fields[name]; held {
					t.Errorf("%s = %q, want it gone", name, v)
				}
			}
			tw := reread(t, path)
			if nums := []int{tw.Year, tw.TrackNo, tw.TrackTotal, tw.DiscNo, tw.DiscTotal}; slices.Max(nums) != 0 {
				t.Errorf("year, track, total, disc, total = %v, want none", nums)
			}
		})
	}
}

func TestClearingOneOfAPairKeepsTheOther(t *testing.T) {
	t.Parallel()
	for _, sample := range []string{sampleFLAC, sampleMP3, sampleMP3v23, sampleM4A} {
		t.Run(sample, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, sample)
			set := Edit{Fields: map[Field][]string{FieldTrack: {"7"}, FieldTrackTotal: {"12"}, FieldDisc: {"2"}, FieldDiscTotal: {"3"}}}
			if err := audiometa.Write(path, set.apply); err != nil {
				t.Fatalf("Write: %v", err)
			}
			remove := Edit{Fields: map[Field][]string{FieldTrackTotal: nil, FieldDisc: nil}}
			if err := audiometa.Write(path, remove.apply); err != nil {
				t.Fatalf("clear: %v", err)
			}
			if tw := reread(t, path); tw.TrackNo != 7 || tw.TrackTotal != 0 || tw.DiscNo != 0 || tw.DiscTotal != 3 {
				t.Errorf("track %d/%d, disc %d/%d, want 7/0 and 0/3", tw.TrackNo, tw.TrackTotal, tw.DiscNo, tw.DiscTotal)
			}
		})
	}
}

func TestClearingTakesMirrorsAndDependents(t *testing.T) {
	t.Parallel()
	path := copySample(t, sampleFLAC)
	seed(t, path, map[string][]string{
		keyDate: {oldDate}, keyYear: {"2001"}, keyTrackTotal: {"9"}, keyTotalTracks: {"9"}, keyTrackC: {"9"}, keyComposer: {oldValue}, keyComposerSort: {oldValue},
		keyVorbisLyrics: {oldValue}, keyUnsynced: {oldValue},
	})
	edit := Edit{Fields: map[Field][]string{FieldDate: nil, FieldTrackTotal: nil, FieldComposers: nil, FieldLyrics: nil}}
	if err := audiometa.Write(path, edit.apply); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := reread(t, path)
	for _, key := range []string{keyDate, keyYear, keyTrackTotal, keyTotalTracks, keyTrackC, keyComposer, keyComposerSort, keyVorbisLyrics, keyUnsynced} {
		if v := rawValues(&got, key); v != nil {
			t.Errorf("%s = %q, want it gone", key, v)
		}
	}
}

func TestEmptyValuesReadAsAbsent(t *testing.T) {
	t.Parallel()
	path := copySample(t, sampleFLAC)
	seed(t, path, map[string][]string{"CONDUCTOR": {"", " "}, "BPM": {""}})
	got := fieldsOf(t, path).Fields
	if v, held := got[FieldConductor]; held {
		t.Errorf("conductor = %q, want it absent", v)
	}
	if v, held := got[FieldBPM]; held {
		t.Errorf("bpm = %q, want it absent", v)
	}
}

func TestMirrorsStayInStepAndDependentsGo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, sample string
		seed         map[string][]string
		edit         map[Field][]string
		want         map[string][]string // By raw key. Nil means gone.
	}{
		{
			name: "totals", sample: sampleFLAC,
			seed: map[string][]string{keyTotalTracks: {"9"}, keyTrackC: {"9"}, keyTotalDiscs: {"1"}, "DISCC": {"1"}, keyTrackTotal: {"9"}},
			edit: map[Field][]string{FieldTrackTotal: {"12"}, FieldDiscTotal: {"2"}},
			want: map[string][]string{
				keyTrackTotal: {"12"}, keyTotalTracks: {"12"}, keyTrackC: {"12"}, "DISCTOTAL": {"2"}, keyTotalDiscs: {"2"}, "DISCC": {"2"},
			},
		},
		{
			name: "a date", sample: sampleFLAC,
			seed: map[string][]string{keyDate: {oldDate}, keyYear: {"2001"}},
			edit: map[Field][]string{FieldDate: {newDate}},
			want: map[string][]string{keyDate: {newDate}, keyYear: {"1987"}},
		},
		{
			name: "a year replaces a date", sample: sampleFLAC,
			seed: map[string][]string{keyDate: {oldDate}, keyYear: {"2001"}},
			edit: map[Field][]string{FieldDate: {"1987"}},
			want: map[string][]string{keyDate: {"1987"}, keyYear: {"1987"}},
		},
		{
			name: "a year replaces an ID3v2.3 date", sample: sampleMP3v23,
			seed: map[string][]string{keyTYER: {"2001"}, keyTDAT: {"1205"}, "TIME": {"1200"}},
			edit: map[Field][]string{FieldDate: {"1987"}},
			want: map[string][]string{keyTYER: {"1987"}, keyTDAT: nil, "TIME": nil},
		},
		{
			name: "a whole ID3v2.3 date", sample: sampleMP3v23,
			seed: map[string][]string{keyTYER: {"2001"}},
			edit: map[Field][]string{FieldDate: {newDate}},
			want: map[string][]string{keyTYER: {"1987"}, keyTDAT: {"0403"}},
		},
		{
			name: "an original date", sample: sampleMP3,
			seed: map[string][]string{keysOriginalDate[0]: {oldDate}},
			edit: map[Field][]string{FieldOriginalDate: {"1977"}},
			want: map[string][]string{keysOriginalDate[0]: {"1977"}},
		},
		{
			name: "grouping", sample: sampleMP3,
			seed: map[string][]string{"TIT1": {oldValue}},
			edit: map[Field][]string{FieldGrouping: {newValue}},
			want: map[string][]string{"GRP1": {newValue}, "TIT1": {newValue}},
		},
		{
			name: "sort album artist in both homes", sample: sampleMP3,
			seed: map[string][]string{keysAlbumArtistSort[1]: {oldValue}, keysAlbumArtistSort[0]: {oldValue}},
			edit: map[Field][]string{FieldSortAlbumArtist: {newValue}},
			want: map[string][]string{keysAlbumArtistSort[1]: {newValue}, keysAlbumArtistSort[0]: {newValue}},
		},
		{
			name: "label and comment", sample: sampleFLAC,
			seed: map[string][]string{"PUBLISHER": {oldValue}, "DESCRIPTION": {oldValue}},
			edit: map[Field][]string{FieldLabel: {newValue}, FieldComment: {newValue}},
			want: map[string][]string{"LABEL": {newValue}, "PUBLISHER": {newValue}, "COMMENT": {newValue}, "DESCRIPTION": {newValue}},
		},
		{
			name: "composer's sort key", sample: sampleFLAC,
			seed: map[string][]string{keyComposer: {oldValue}, keyComposerSort: {oldValue}},
			edit: map[Field][]string{FieldComposers: {newValue}},
			want: map[string][]string{keyComposer: {newValue}, keyComposerSort: nil},
		},
		{
			name: "a new title with a new sort title", sample: sampleFLAC,
			seed: map[string][]string{keysTitleSort[0]: {oldValue}, keyTitleSortEN: {oldValue}},
			edit: map[Field][]string{FieldTitle: {newTitle}, FieldSortTitle: {sortedTitle}},
			want: map[string][]string{keysTitleSort[0]: {sortedTitle}, keyTitleSortEN: nil},
		},
		{
			name: "lyrics copy that matches", sample: sampleFLAC,
			seed: map[string][]string{keyVorbisLyrics: {oldValue}, keyUnsynced: {oldValue}},
			edit: map[Field][]string{FieldLyrics: {newValue}},
			want: map[string][]string{keyVorbisLyrics: {newValue}, keyUnsynced: {newValue}},
		},
		{
			name: "lyrics copy that differs", sample: sampleFLAC,
			seed: map[string][]string{keyVorbisLyrics: {oldValue}, keyUnsynced: {otherName}},
			edit: map[Field][]string{FieldLyrics: {newValue}},
			want: map[string][]string{keyVorbisLyrics: {newValue}, keyUnsynced: {otherName}},
		},
		{
			name: "M4A's numeric genre", sample: sampleM4A,
			seed: map[string][]string{"gnre": {"16"}},
			edit: map[Field][]string{FieldGenres: {jazz}},
			want: map[string][]string{"©gen": {jazz}, "gnre": nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, tt.sample)
			seed(t, path, tt.seed)
			edit := Edit{Fields: tt.edit}
			if err := audiometa.Write(path, edit.apply); err != nil {
				t.Fatalf("Write: %v", err)
			}
			got := reread(t, path)
			for key, want := range tt.want {
				if v := rawValues(&got, key); !slices.Equal(v, want) {
					t.Errorf("%s = %q, want %q", key, v, want)
				}
			}
		})
	}
}

func TestCommentAndLyricsOfAnMP3AreItsFrames(t *testing.T) {
	t.Parallel()
	for _, sample := range []string{sampleMP3, sampleMP3v23} {
		t.Run(sample, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, sample)
			edit := Edit{Fields: map[Field][]string{FieldComment: {aComment}, FieldLyrics: {someLyrics}}}
			if err := audiometa.Write(path, edit.apply); err != nil {
				t.Fatalf("Write: %v", err)
			}
			f, err := audiometa.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close() //nolint:errcheck // Read-only use is done; nothing to report.
			if f.Tags.Comment != aComment || f.Tags.Lyrics != someLyrics || !slices.Equal(f.Tags.Get(keyLyrics), []string{someLyrics}) {
				t.Errorf("comment %q, lyrics %q", f.Tags.Comment, f.Tags.Lyrics)
			}
		})
	}
}

func TestIdentityFieldsRoundTrip(t *testing.T) {
	t.Parallel()
	for _, sample := range []string{sampleFLAC, sampleMP3, sampleMP3v23, sampleM4A} {
		t.Run(sample, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, sample)
			set := Edit{Fields: map[Field][]string{
				FieldAlbum: {albumValue}, FieldArtists: {"One", "Two"}, FieldAlbumArtists: {artistValue}, FieldCompilation: {flagOn},
			}}
			if err := audiometa.Write(path, set.apply); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if v := fieldsOf(t, path).Fields[FieldAlbum]; !slices.Equal(v, []string{albumValue}) {
				t.Errorf("an editor reads album %q", v)
			}
			wantIdentity(t, path)
			remove := Edit{Fields: map[Field][]string{FieldAlbumArtists: nil, FieldCompilation: nil}}
			if err := audiometa.Write(path, remove.apply); err != nil {
				t.Fatalf("clear: %v", err)
			}
			if got := reread(t, path); len(got.AlbumArtists) != 0 || got.Compilation {
				t.Errorf("album artists %q, compilation %t: want both gone", artistNames(got.AlbumArtists), got.Compilation)
			}
		})
	}
}

// wantIdentity checks a scan of the file at path groups it as TestIdentityFieldsRoundTrip set.
func wantIdentity(t *testing.T, path string) {
	t.Helper()
	got := reread(t, path)
	if got.Album.Name != albumValue || !slices.Equal(artistNames(got.Artists), []string{"One", "Two"}) ||
		!slices.Equal(artistNames(got.AlbumArtists), []string{artistValue}) || !got.Compilation {
		t.Errorf("album %q, artists %q, album artists %q, compilation %t",
			got.Album.Name, artistNames(got.Artists), artistNames(got.AlbumArtists), got.Compilation)
	}
}

func TestCompilationTakesOnlyOn(t *testing.T) {
	t.Parallel()
	path := copySample(t, sampleFLAC)
	edit := Edit{Fields: map[Field][]string{FieldCompilation: {"0"}}}
	if err := audiometa.Write(path, edit.apply); !errors.Is(err, ErrUnsupportedEdit) {
		t.Errorf("Write = %v, want ErrUnsupportedEdit", err)
	}
}

// artistNames returns the names in list, in order.
func artistNames(list []store.ArtistWrite) []string {
	out := make([]string, len(list))
	for i := range list {
		out[i] = list[i].Name
	}
	return out
}

func TestTimestampedLyricsAreReadOnly(t *testing.T) {
	t.Parallel()
	path := copySample(t, sampleFLAC)
	seed(t, path, map[string][]string{keyVorbisLyrics: {"[00:12.34]One\n[00:15.00]Two"}})
	if got := fieldsOf(t, path).ReadOnly; !slices.Equal(got, []Field{FieldLyrics}) {
		t.Errorf("read-only fields = %q", got)
	}
	edit := Edit{Fields: map[Field][]string{FieldLyrics: {"Plain"}}}
	if err := audiometa.Write(path, edit.apply); !errors.Is(err, ErrUnsupportedEdit) {
		t.Errorf("Write = %v, want ErrUnsupportedEdit", err)
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
		{name: "empty title", sample: sampleFLAC, edit: Edit{Fields: map[Field][]string{FieldTitle: {""}}}},
		{name: "blank title", sample: sampleMP3, edit: Edit{Fields: map[Field][]string{FieldTitle: {" \t "}}}},
		{name: "ID3v1 only", sample: sampleMP3, prepare: id3v1Only, edit: Edit{Fields: map[Field][]string{FieldTitle: {newTitle}}}},
		{name: "a year MP3 can't read back", sample: sampleMP3, edit: Edit{Fields: map[Field][]string{FieldDate: {"1850"}}}},
		{name: "a BPM M4A can't hold", sample: sampleM4A, edit: Edit{Fields: map[Field][]string{FieldBPM: {"120.5"}}}},
		{name: "a NUL", sample: sampleFLAC, edit: Edit{Fields: map[Field][]string{FieldComment: {"a\x00b"}}}},
		{name: "two comments", sample: sampleFLAC, edit: Edit{Fields: map[Field][]string{FieldComment: {"a", "b"}}}},
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
	edit := Edit{Fields: map[Field][]string{FieldTitle: {newTitle}}}
	err := edit.apply(&audiometa.File{Format: audiometa.FormatOgg})
	if !errors.Is(err, ErrUnsupportedEdit) || !strings.Contains(err.Error(), "Ogg") {
		t.Errorf("apply = %v, want ErrUnsupportedEdit naming the format", err)
	}
}
