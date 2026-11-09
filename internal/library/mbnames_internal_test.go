package library

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/simonhull/audiometa"
)

// Picard's names for the MusicBrainz IDs, from its tag mapping
// (picard-docs.musicbrainz.org/en/latest/appendices/tag_mapping.html), by field.
// ID3 keeps the recording's ID in a UFID frame owned by musicbrainzOwner.
//
//nolint:gochecknoglobals // Fixed expectations, never written.
var (
	picardID3 = map[Field]string{
		FieldMusicBrainzAlbumID: "MusicBrainz Album Id", FieldMusicBrainzReleaseGroupID: "MusicBrainz Release Group Id",
		FieldMusicBrainzAlbumArtistID: "MusicBrainz Album Artist Id", FieldMusicBrainzArtistID: "MusicBrainz Artist Id",
		FieldMusicBrainzTrackID: "MusicBrainz Release Track Id",
	}
	picardMP4 = map[Field]string{
		FieldMusicBrainzAlbumID: "MusicBrainz Album Id", FieldMusicBrainzReleaseGroupID: "MusicBrainz Release Group Id",
		FieldMusicBrainzAlbumArtistID: "MusicBrainz Album Artist Id", FieldMusicBrainzArtistID: "MusicBrainz Artist Id",
		FieldMusicBrainzTrackID: "MusicBrainz Release Track Id", FieldMusicBrainzRecordingID: "MusicBrainz Track Id",
	}
	picardVorbis = map[Field]string{
		FieldMusicBrainzAlbumID: "MUSICBRAINZ_ALBUMID", FieldMusicBrainzReleaseGroupID: "MUSICBRAINZ_RELEASEGROUPID",
		FieldMusicBrainzAlbumArtistID: "MUSICBRAINZ_ALBUMARTISTID", FieldMusicBrainzArtistID: "MUSICBRAINZ_ARTISTID",
		FieldMusicBrainzTrackID: "MUSICBRAINZ_RELEASETRACKID", FieldMusicBrainzRecordingID: "MUSICBRAINZ_TRACKID",
	}
)

const (
	musicbrainzOwner = "http://musicbrainz.org"
	oldMBID          = "00000000-0000-4000-8000-000000000001"
)

// onDisk is the file at path, byte for byte.
func onDisk(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// asciiUpper upper-cases the ASCII letters of b, leaving every other byte.
func asciiUpper(b []byte) []byte {
	out := bytes.Clone(b)
	for i, c := range out {
		if 'a' <= c && c <= 'z' {
			out[i] = c - 'a' + 'A'
		}
	}
	return out
}

// storedAs is how a tag's name sits in the file's bytes: an ID3 TXXX
// description ends at its terminator, an M4A freeform name follows its name
// atom's version and flags, and a Vorbis name ends at "=".
func storedAs(sample, name string) []byte {
	switch sample {
	case sampleM4A:
		return []byte("name\x00\x00\x00\x00" + name)
	case sampleFLAC:
		return []byte(name + "=")
	}
	return []byte(name + "\x00")
}

// wantNamedOnce checks the file's bytes hold name as stored, spelled exactly so,
// and, but in Vorbis, which repeats a name for each value, no other spelling of
// it: one frame or item, in the case wanted.
func wantNamedOnce(t *testing.T, raw []byte, sample, name string) {
	t.Helper()
	exact := storedAs(sample, name)
	if !bytes.Contains(raw, exact) {
		t.Errorf("no %q in the file", exact)
	}
	if n := bytes.Count(asciiUpper(raw), asciiUpper(exact)); n != 1 && sample != sampleFLAC {
		t.Errorf("%q is in the file %d times in any case, want once", exact, n)
	}
}

// wantRecordingUFID checks the file holds the recording's ID in one UFID frame
// owned by MusicBrainz, and in no TXXX.
func wantRecordingUFID(t *testing.T, raw []byte, id string) {
	t.Helper()
	body := []byte(musicbrainzOwner + "\x00" + id)
	const header = 10 // A frame's ID, size and flags.
	i := bytes.Index(raw, body)
	if i < header || string(raw[i-header:i-header+4]) != "UFID" || bytes.Count(raw, body) != 1 {
		t.Errorf("no single UFID frame holding %q", body)
	}
	if bytes.Contains(asciiUpper(raw), []byte("MUSICBRAINZ TRACK ID")) {
		t.Error("the recording's ID is also in a TXXX frame")
	}
}

// wantPicardOnDisk checks the bytes of the file at path name every MusicBrainz ID
// as Picard does.
func wantPicardOnDisk(t *testing.T, path, sample string) {
	t.Helper()
	raw := onDisk(t, path)
	names := picardID3
	switch sample {
	case sampleM4A:
		names = picardMP4
	case sampleFLAC:
		names = picardVorbis
	default:
		wantRecordingUFID(t, raw, mbidRecording)
	}
	for field, name := range names {
		t.Run(string(field), func(t *testing.T) {
			wantNamedOnce(t, raw, sample, name)
		})
	}
}

// seedMusicBrainz tags the file at path with every MusicBrainz ID but the
// recording's, under the given TXXX or freeform names, as another tagger would.
// The recording's ID goes where that tagger keeps it.
func seedMusicBrainz(t *testing.T, path string, names map[Field]string, ufid bool) {
	t.Helper()
	err := audiometa.Write(path, func(f *audiometa.File) error {
		for _, name := range names {
			f.Tags.SetNamed(name, oldMBID)
		}
		if ufid {
			f.Tags.Set(keyRecordingUFID, oldMBID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// upperCased returns names upper-cased, as some taggers write them.
func upperCased(names map[Field]string) map[Field]string {
	out := make(map[Field]string, len(names))
	for field, name := range names {
		out[field] = strings.ToUpper(name)
	}
	return out
}

// mbEdit gives every MusicBrainz ID a new value, and the title too.
func mbEdit() Edit {
	return Edit{Fields: map[Field][]string{
		FieldTitle: {newTitle}, FieldMusicBrainzAlbumID: {mbidAlbum}, FieldMusicBrainzReleaseGroupID: {mbidGroup},
		FieldMusicBrainzAlbumArtistID: {mbidArtistOne}, FieldMusicBrainzArtistID: {mbidArtistTwo},
		FieldMusicBrainzRecordingID: {mbidRecording}, FieldMusicBrainzTrackID: {mbidTrack},
	}}
}

// An edit of a file another tagger named the IDs in rewrites each frame or item in
// place, in the case the file has: Picard's mixed case stays, and so does the upper
// case some taggers write. Nothing is added beside it.
func TestEditKeepsTheCaseOfMusicBrainzNames(t *testing.T) {
	t.Parallel()
	for _, sample := range []string{sampleMP3, sampleMP3v23, sampleM4A, sampleWAVBoth, sampleAIFF} {
		picard := picardID3
		if sample == sampleM4A {
			picard = picardMP4
		}
		for style, names := range map[string]map[Field]string{"Picard": picard, "upper-case": upperCased(picard)} {
			t.Run(sample+"/"+style, func(t *testing.T) {
				t.Parallel()
				path := copySample(t, sample)
				seedMusicBrainz(t, path, names, sample != sampleM4A)
				edit := mbEdit()
				if err := audiometa.Write(path, edit.apply); err != nil {
					t.Fatalf("Write: %v", err)
				}
				raw := onDisk(t, path)
				for field, name := range names {
					wantNamedOnce(t, raw, sample, name)
					if got := fieldsOf(t, path).Fields[field]; len(got) != 1 || got[0] != edit.Fields[field][0] {
						t.Errorf("%s = %q, want %q", field, got, edit.Fields[field])
					}
				}
				if sample != sampleM4A {
					wantRecordingUFID(t, raw, mbidRecording)
				}
			})
		}
	}
}
