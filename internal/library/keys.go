package library

import (
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
// are upper-case words, ID3 keys are four-letter frame IDs, upper-cased TXXX
// descriptions or "UFID:" and the frame's owner, and M4A keys are atom names or
// upper-cased freeform names. audiometa upper-cases TXXX descriptions and
// freeform names whatever their case in the file, so "MusicBrainz Album Id" is
// found as "MUSICBRAINZ ALBUM ID". A
// WAV or AIFF file adds its native text after its ID3 frames: LIST/INFO IDs
// ("IGNR") and AIFF chunk IDs ("NAME"). Each field therefore has one candidate
// list for every format, tried in order.
// TestRawKeysPerFormat checks every name against real files written by audiometa.
// A field the tag editor has a row for is read through its row's scan list
// ([scanKeys]), which the editor reads too, so a scan and the editor see the same
// value; TestScanReadsEveryEditorKey holds every list to its row's keys.
//
//nolint:gochecknoglobals // Fixed lookup tables, never written.
var (
	keysTitleSort  = []string{"TITLESORT", "TSOT", "sonm"}
	keysArtistSort = []string{"ARTISTSORT", "TSOP", "soar"}
	keysAlbumSort  = []string{"ALBUMSORT", "TSOA", "soal"}
	// keysArtists are audiometa's keys of the typed artists, in its order (a
	// WAV or AIFF file's native text after its ID3 frame), then ARTISTS, the
	// mirror the editor keeps in step: a file holding only that has artists too.
	keysArtists = []string{"ARTIST", "TPE1", "\xA9ART", "IART", "AUTH", "ARTISTS"}
	// keysAlbumArtist ends with the mirrors the editor keeps in step, read only
	// when nothing else holds one.
	keysAlbumArtist     = []string{"ALBUMARTIST", "ALBUM ARTIST", "TPE2", "aART", "ALBUM_ARTIST", "ALBUMARTISTS", "ALBUM_ARTISTS"}
	keysAlbumArtistSort = []string{"ALBUMARTISTSORT", "TSO2", "soaa"}
	keysArtistMBID      = []string{"MUSICBRAINZ_ARTISTID", "MUSICBRAINZ ARTIST ID"}
	keysAlbumArtistMBID = []string{"MUSICBRAINZ_ALBUMARTISTID", "MUSICBRAINZ ALBUM ARTIST ID"}
	keysCompilation     = []string{"COMPILATION", "TCMP", "cpil"}
	keysAlbumMBID       = []string{"MUSICBRAINZ_ALBUMID", "MUSICBRAINZ ALBUM ID"}
	// keysTrackMBID hold the recording's ID, which an MP3 keeps in a UFID frame
	// and M4A in a freeform "MusicBrainz Track Id". The UFID comes first: a TXXX
	// of the M4A name is what some taggers leave in an MP3.
	keysTrackMBID = []string{"MUSICBRAINZ_TRACKID", keyRecordingUFID, "MUSICBRAINZ TRACK ID"}
	// keysReleaseGroupMBID and keysReleaseTrackMBID are written by a match, and
	// read only by the tag editor.
	keysReleaseGroupMBID = []string{"MUSICBRAINZ_RELEASEGROUPID", "MUSICBRAINZ RELEASE GROUP ID"}
	keysReleaseTrackMBID = []string{"MUSICBRAINZ_RELEASETRACKID", "MUSICBRAINZ RELEASE TRACK ID"}
	keysGenre            = []string{"GENRE", "TCON", "\xA9gen", "IGNR"}
	keysComposer         = []string{"COMPOSER", "TCOM", "\xA9wrt"}
	keysDate             = []string{"DATE", "YEAR", "TDRC", "TYER", "\xA9day", keyICRD}
	keysOriginalDate     = []string{"TDOR", "TORY", "ORIGINALDATE"}
	// keysDiscSubtitle hold a disc's name. ID3v2.4 has a frame for it, which
	// ID3v2.3 lacks: there, and in Vorbis and M4A, it is a DISCSUBTITLE text.
	keysDiscSubtitle = []string{"TSST", "DISCSUBTITLE"}
	// keysID3v2Names hold the two fields whose absence makes the MP3 reader fall
	// back to an ID3v1 trailer.
	keysID3v2Names = []string{"TIT2", "TPE1"}
)

// GenreKeys returns the raw tag keys that hold genres, as the store keeps them:
// the tag editor's, so a genre reads where it is written.
func GenreKeys() []string { return storedKeys(FieldGenres) }

// ComposerKeys returns the raw tag keys that hold composers, as the store keeps
// them: the tag editor's, so a composer reads where it is written.
func ComposerKeys() []string { return storedKeys(FieldComposers) }

// TrackMBIDKeys returns the raw tag keys that hold a track's MusicBrainz ID,
// as the store keeps them.
func TrackMBIDKeys() []string { return storedKeys(FieldMusicBrainzRecordingID) }

// ReleaseTrackMBIDKeys returns the raw tag keys that hold the MusicBrainz ID
// of a track on its release, as the store keeps them.
func ReleaseTrackMBIDKeys() []string { return storedKeys(FieldMusicBrainzTrackID) }

// storedKeys returns the scan list of the field, as the store keeps its keys.
func storedKeys(name Field) []string {
	list := scanKeys(name)
	out := make([]string, len(list))
	for i, k := range list {
		out[i] = tagKey(k)
	}
	return out
}

// fieldRows indexes the editor's table by field.
//
//nolint:gochecknoglobals // Built once from the fixed table, never written.
var fieldRows = func() map[Field]*field {
	out := make(map[Field]*field, len(fields))
	for i := range fields {
		out[fields[i].name] = &fields[i]
	}
	return out
}()

// scanKeys returns the keys a scan reads the field from, which the editor reads
// too. Every field a scan reads has them: TestScanReadsEveryEditorKey checks.
func scanKeys(name Field) []string { return fieldRows[name].scan }

// scanValues returns the field's values as a scan reads them, and as the editor
// shows them: cleaned, and nil for none.
func scanValues(t *audiometa.Tags, name Field) []string {
	row := fieldRows[name]
	if row.read != nil {
		return row.read(t)
	}
	return firstValues(t, row.scan)
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
