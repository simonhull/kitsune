package library

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/simonhull/audiometa"
)

const (
	// maxShownValue is the longest raw value a client sees. Past it a value is
	// cut, and a cut value can't be saved back.
	maxShownValue = 4 << 10
	// maxRawKey is the longest key a client may add.
	maxRawKey = 64
)

//nolint:gochecknoglobals // Fixed lookup tables, never written.
var (
	// frameIDPattern matches an ID3v2 frame ID. A new key of that shape would be a
	// TXXX frame posing as the frame.
	frameIDPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{2,3}$`)
	// chapterKey matches the Vorbis comments that hold chapters.
	chapterKey = regexp.MustCompile(`^CHAPTER\d+(NAME|URL)?$`)
	// machinePrefixes start keys that hold what a tool measured, not what a person wrote.
	machinePrefixes = []string{"REPLAYGAIN_", "REPLAYGAIN ", "R128_", "MP3GAIN_", "ACOUSTID", "FINGERPRINT", "ITUN"}
	// structureKeys hold pictures and chapters in a text tag.
	structureKeys = []string{"METADATA_BLOCK_PICTURE", "COVERART", "COVERARTMIME", "OVERDRIVE MEDIAMARKERS"}
	// m4aBinaryItems are the items that hold a number or a picture, not text.
	m4aBinaryItems = []string{"covr", "----", "pgap", "stik", "rtng", "hdvd", "tves", "tvsn", "\xA9mvi", "\xA9mvc", "gnre"}
)

// readOnlyKey reports a key no edit may touch, in any format: one a field owns,
// so the table stays the only way to it, or one that holds what a tool made.
func readOnlyKey(key string) bool {
	upper := strings.ToUpper(key)
	return ownedKey(key) || slices.Contains(structureKeys, upper) || chapterKey.MatchString(upper) ||
		slices.ContainsFunc(machinePrefixes, func(prefix string) bool { return strings.HasPrefix(upper, prefix) })
}

// ownedKey reports a key of any row, in any format. A scan reads its candidates
// in every format, so an M4A freeform ALBUMARTIST would move a track as surely
// as aART.
func ownedKey(key string) bool {
	return slices.ContainsFunc(fields, func(f field) bool {
		return slices.ContainsFunc([][]string{f.flac, f.mp3, f.mp3v23, f.m4a, f.dependents, f.matching}, func(keys []string) bool {
			return slices.Contains(keys, key)
		})
	})
}

// validNewKey reports whether key can be added to a file of fam. Vorbis keys are
// ASCII; ID3 and M4A names are free but for their case, and frames can't be made
// from a name.
func validNewKey(fam family, key string) bool {
	if key == "" || len(key) > maxRawKey || key != strings.TrimSpace(key) || key != strings.ToUpper(key) {
		return false
	}
	if fam == vorbis {
		return !strings.ContainsFunc(key, func(r rune) bool { return r < ' ' || r > '}' || r == '=' })
	}
	if strings.ContainsFunc(key, func(r rune) bool { return !unicode.IsPrint(r) }) {
		return false
	}
	return fam == mp4 || !frameIDPattern.MatchString(key)
}

// rawEditable reports whether the raw key, holding values now, may change.
func rawEditable(fam family, key string, values []string, exists bool) bool {
	switch {
	case readOnlyKey(key):
		return false
	case slices.ContainsFunc(values, func(v string) bool { return len(v) > maxShownValue }):
		return false
	case fam == mp4 && (slices.Contains(m4aBinaryItems, key) || exists && len(clean(values)) == 0):
		return false // A value audiometa can't show isn't text.
	case fam == vorbis || !exists:
		return validNewKey(fam, key)
	}
	return true
}

// applyTags makes the edit's raw changes. A key is named as [TrackTags] shows it.
func (e *Edit) applyTags(t *audiometa.Tags, fam family) error {
	for name, values := range e.Tags {
		key, exists := rawKey(t, name)
		if !rawEditable(fam, key, t.Get(key), exists) {
			return fmt.Errorf("tag %q can't be edited here: %w", name, ErrUnsupportedEdit)
		}
		values = clean(values)
		if slices.ContainsFunc(values, func(v string) bool { return strings.ContainsRune(v, 0) }) {
			return fmt.Errorf("tag %q holds a NUL: %w", name, ErrUnsupportedEdit)
		}
		t.Set(key, values...)
	}
	return nil
}

// rawKey finds the key that [tagKey] shows as name, reporting whether the file
// has it. Without one, name itself is the key.
func rawKey(t *audiometa.Tags, name string) (string, bool) {
	for key := range t.All() {
		if tagKey(key) == name {
			return key, true
		}
	}
	return name, false
}

// shown cuts values a client won't get whole. It reports whether it cut any.
func shown(values []string) ([]string, bool) {
	cut := false
	out := make([]string, len(values))
	for i, v := range values {
		if len(v) > maxShownValue {
			v, cut = strings.ToValidUTF8(v[:maxShownValue], ""), true
		}
		out[i] = v
	}
	return out, cut
}
