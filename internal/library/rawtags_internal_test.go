package library

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/audiometa"
)

const (
	keyMood        = "MOOD"
	calm           = "Calm"
	customKey      = "MY CUSTOM TAG"
	keyGain        = "REPLAYGAIN_TRACK_GAIN"
	keyFingerprint = "ACOUSTID_FINGERPRINT"
	keyTitleSortEN = "TITLESORTEN"
)

// rawEdit sets values on the raw key.
func rawEdit(key string, values ...string) *Edit {
	return &Edit{Tags: map[string][]string{key: values}}
}

// tableKeys lists every key of the field table.
func tableKeys() []string {
	var out []string
	for i := range fields {
		f := &fields[i]
		out = slices.Concat(out, f.flac, f.mp3, f.mp3v23, f.m4a, f.dependents, f.matching)
	}
	return out
}

func TestRawKeysAFormatRefuses(t *testing.T) {
	t.Parallel()
	// Keys a field owns in any format, which a scan may read, and what isn't a person's.
	everywhere := slices.Concat(
		tableKeys(), keysAlbumArtist, keysCompilation, keysGenre, keysDiscSubtitle, structureKeys,
		[]string{"", " ", " LEADING", "TRAILING ", "lower", "Mixed", strings.Repeat("A", maxRawKey+1)},
		[]string{keyGain, "REPLAYGAIN_ALBUM_PEAK", "R128_TRACK_GAIN", keyFingerprint, "ACOUSTID_ID", "ITUNNORM", "CHAPTER001", "CHAPTER001NAME"},
	)
	tests := []struct {
		sample string
		keys   []string
	}{
		{sampleFLAC, []string{"A=B", "=", "CAFÉ", "TAB\tKEY", "NEW\nLINE", "TILDE~"}},
		{sampleMP3, []string{"APIC", "COMM", "USLT", "PRIV", "UFID", "POPM", "WOAR", "TXXX", "TMED", "tit2", "apic", "MOOD", "TT2", "NEW\x00KEY", "BELL\x07"}},
		{sampleMP3v23, []string{"APIC", "TMOO"}},
		{sampleM4A, []string{"covr", "pgap", "stik", "©nam", "©xyz", "----", "aART", "NEW\x00KEY"}},
	}
	for _, tt := range tests {
		t.Run(tt.sample, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, tt.sample)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range slices.Concat(everywhere, tt.keys) {
				if writeErr := audiometa.Write(path, rawEdit(key, "x").apply); !errors.Is(writeErr, ErrUnsupportedEdit) {
					t.Errorf("Write(%q) = %v, want ErrUnsupportedEdit", key, writeErr)
				}
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(before, after) {
				t.Error("a refused key changed the file")
			}
		})
	}
}

func TestRawKeysRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		sample, key string
		seed        map[string][]string // Keys that must exist first.
	}{
		{sample: sampleFLAC, key: "MY CUSTOM_TAG-1"},
		{sample: sampleFLAC, key: keyMood},
		{sample: sampleMP3, key: customKey},
		{sample: sampleMP3, key: keyMood, seed: map[string][]string{keyMood: {oldValue}}},
		{sample: sampleMP3, key: "TENC", seed: map[string][]string{"TENC": {oldValue}}},
		{sample: sampleMP3v23, key: customKey},
		{sample: sampleM4A, key: customKey},
		{sample: sampleM4A, key: keyMood},
		{sample: sampleM4A, key: "©too", seed: map[string][]string{"\xA9too": {oldValue}}},
	}
	for _, tt := range tests {
		t.Run(tt.sample+" "+tt.key, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, tt.sample)
			seed(t, path, tt.seed)
			if err := audiometa.Write(path, rawEdit(tt.key, calm, "Warm").apply); err != nil {
				t.Fatalf("Write: %v", err)
			}
			wantRaw(t, path, tt.key, calm, "Warm")
			if err := audiometa.Write(path, rawEdit(tt.key).apply); err != nil {
				t.Fatalf("delete: %v", err)
			}
			if i := indexRaw(fieldsOf(t, path), tt.key); i >= 0 {
				t.Errorf("%s is still there", tt.key)
			}
		})
	}
}

func indexRaw(tags *TrackTags, key string) int {
	return slices.IndexFunc(tags.Tags, func(tag RawTag) bool { return tag.Key == key })
}

// wantRaw checks an editor sees key holding values, and may edit it.
func wantRaw(t *testing.T, path, key string, values ...string) {
	t.Helper()
	got := fieldsOf(t, path)
	i := indexRaw(got, key)
	if i < 0 {
		t.Fatalf("no %s among %+v", key, got.Tags)
	}
	if tag := &got.Tags[i]; !slices.Equal(tag.Values, values) || tag.ReadOnly {
		t.Errorf("%s = %+v, want %q, editable", key, tag, values)
	}
}

func TestReadOnlyTagsShowAndStay(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("é", maxShownValue)
	path := copySample(t, sampleFLAC)
	seed(t, path, map[string][]string{
		keyGain: {"-6.5 dB"}, keyFingerprint: {"AQAD"}, keyTitleSortEN: {oldValue}, "HUGE": {long}, keyMood: {calm},
	})
	got := fieldsOf(t, path)
	for _, key := range []string{keyGain, keyFingerprint, keyTitleSortEN, "HUGE"} {
		if i := indexRaw(got, key); i < 0 || !got.Tags[i].ReadOnly {
			t.Errorf("%s: want it shown, read-only, in %+v", key, got.Tags)
		}
		if err := audiometa.Write(path, rawEdit(key, "x").apply); !errors.Is(err, ErrUnsupportedEdit) {
			t.Errorf("Write(%s) = %v, want ErrUnsupportedEdit", key, err)
		}
		if err := audiometa.Write(path, rawEdit(key).apply); !errors.Is(err, ErrUnsupportedEdit) {
			t.Errorf("delete %s = %v, want ErrUnsupportedEdit", key, err)
		}
	}
	huge := got.Tags[indexRaw(got, "HUGE")].Values[0]
	if len(huge) > maxShownValue || !strings.HasPrefix(long, huge) {
		t.Errorf("a cut value is %d bytes and must be a clean prefix", len(huge))
	}
	wantRaw(t, path, keyMood, calm)
}

func TestM4ANumberItemsAreReadOnly(t *testing.T) {
	t.Parallel()
	path := copySample(t, sampleM4A)
	seed(t, path, map[string][]string{"stik": {"2"}})
	got := fieldsOf(t, path)
	if i := indexRaw(got, "stik"); i < 0 || !got.Tags[i].ReadOnly {
		t.Errorf("stik: want it shown, read-only, in %+v", got.Tags)
	}
}

func TestATrackRawTagGoesOnTopOfAll(t *testing.T) {
	t.Parallel()
	all := Edit{Tags: map[string][]string{keyMood: {calm}, "KEEP": {"x"}}}
	own := Edit{Tags: map[string][]string{keyMood: {"Warm"}}}
	got := all.over(&own)
	if !slices.Equal(got.Tags[keyMood], []string{"Warm"}) || !slices.Equal(got.Tags["KEEP"], []string{"x"}) {
		t.Errorf("tags = %q", got.Tags)
	}
	if _, touched := all.Tags["OTHER"]; touched || len(all.Tags) != 2 || !slices.Equal(all.Tags[keyMood], []string{calm}) {
		t.Errorf("all = %q, want it unchanged", all.Tags)
	}
}
