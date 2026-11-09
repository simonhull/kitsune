package library

import (
	"slices"
	"strings"
	"unicode"

	"github.com/simonhull/audiometa"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// MatchKey returns the identity key for a name: it trims the name, collapses
// every run of white space to one space, normalizes to NFC and folds case,
// then normalizes to NFC again because folding can leave text that is not NFC.
// It strips no punctuation and splits nothing, so "A feat. B" is one key.
// Every identity key in the library goes through it.
func MatchKey(s string) string {
	s = strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
	s = norm.NFC.String(s)
	// A cases.Caser is not safe for concurrent use, so each call makes its own.
	s = cases.Fold().String(s)
	return norm.NFC.String(s)
}

// Raw tag names differ by format but never collide across formats: Vorbis keys
// are upper-case words, ID3 keys are four-letter frame IDs or upper-cased TXXX
// descriptions, and M4A keys are atom names or upper-cased freeform names. A
// WAV or AIFF file adds its native text after its ID3 frames: LIST/INFO IDs
// ("IGNR") and AIFF chunk IDs ("NAME"). Each field therefore has one candidate
// list for every format, tried in order.
// TestRawKeysPerFormat checks every name against real files written by audiometa.
//
//nolint:gochecknoglobals // Fixed lookup tables, never written.
var (
	keysTitleSort       = []string{"TITLESORT", "TSOT", "sonm"}
	keysArtistSort      = []string{"ARTISTSORT", "TSOP", "soar"}
	keysAlbumSort       = []string{"ALBUMSORT", "TSOA", "soal"}
	keysAlbumArtist     = []string{"ALBUMARTIST", "ALBUM ARTIST", "TPE2", "aART"}
	keysAlbumArtistSort = []string{"ALBUMARTISTSORT", "TSO2", "soaa"}
	keysArtistMBID      = []string{"MUSICBRAINZ_ARTISTID", "MUSICBRAINZ ARTIST ID"}
	keysAlbumArtistMBID = []string{"MUSICBRAINZ_ALBUMARTISTID", "MUSICBRAINZ ALBUM ARTIST ID"}
	keysCompilation     = []string{"COMPILATION", "TCMP", "cpil"}
	keysAlbumMBID       = []string{"MUSICBRAINZ_ALBUMID", "MUSICBRAINZ ALBUM ID"}
	keysTrackMBID       = []string{"MUSICBRAINZ_TRACKID", "MUSICBRAINZ TRACK ID"}
	keysGenre           = []string{"GENRE", "TCON", "\xA9gen", "IGNR"}
	keysDate            = []string{"DATE", "YEAR", "TDRC", "TYER", "\xA9day", keyICRD}
	keysOriginalDate    = []string{"TDOR", "TORY", "ORIGINALDATE"}
	// keysDiscSubtitle hold a disc's name. ID3v2.4 has a frame for it, which
	// ID3v2.3 lacks: there, and in Vorbis and M4A, it is a DISCSUBTITLE text.
	keysDiscSubtitle = []string{"TSST", "DISCSUBTITLE"}
	// keysID3v2Names hold the two fields whose absence makes the MP3 reader fall
	// back to an ID3v1 trailer.
	keysID3v2Names = []string{"TIT2", "TPE1"}
)

// GenreKeys returns the raw tag keys that hold genres, as the store keeps them.
func GenreKeys() []string {
	out := make([]string, len(keysGenre))
	for i, k := range keysGenre {
		out[i] = tagKey(k)
	}
	return out
}

// ComposerKeys returns the raw tag keys that hold composers, as the store keeps
// them: the tag editor's, so a composer reads where it is written.
func ComposerKeys() []string {
	i := slices.IndexFunc(fields, func(f field) bool { return f.name == FieldComposers })
	row := &fields[i] // The table always has composers: TestComposerKeys checks.
	var out []string
	for _, k := range slices.Concat(row.flac, row.mp3, row.m4a) {
		if key := tagKey(k); !slices.Contains(out, key) {
			out = append(out, key)
		}
	}
	return out
}

// TrackMBIDKeys returns the raw tag keys that hold a track's MusicBrainz ID,
// as the store keeps them.
func TrackMBIDKeys() []string {
	out := make([]string, len(keysTrackMBID))
	for i, k := range keysTrackMBID {
		out[i] = tagKey(k)
	}
	return out
}

// multi returns the values of the first key in keys that has any.
func multi(t *audiometa.Tags, keys []string) []string {
	for _, k := range keys {
		if v := t.Get(k); len(v) > 0 {
			return v
		}
	}
	return nil
}

// single returns the trimmed first non-empty value among keys.
func single(t *audiometa.Tags, keys []string) string {
	return strings.TrimSpace(t.GetBest(keys...))
}
