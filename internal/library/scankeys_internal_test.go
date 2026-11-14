package library

import (
	"bytes"
	"encoding/binary"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/simonhull/kitsune/internal/store"
)

// scannedFields are the editor's fields a scan reads, through [scanKeys] or
// [scanValues]; a field that joins NewTrackWrite joins this list. Title and
// album are not here: a scan reads them from audiometa's typed field alone,
// which binds every key their rows have, in every format.
//
//nolint:gochecknoglobals // A fixed test table.
var scannedFields = []Field{
	FieldArtists, FieldAlbumArtists, FieldCompilation,
	FieldTrack, FieldTrackTotal, FieldDisc, FieldDiscTotal, FieldDiscSubtitle,
	FieldDate, FieldOriginalDate, FieldGenres, FieldComposers,
	FieldSortTitle, FieldSortArtist, FieldSortAlbum, FieldSortAlbumArtist,
	FieldMusicBrainzAlbumID, FieldMusicBrainzAlbumArtistID, FieldMusicBrainzArtistID,
	FieldMusicBrainzRecordingID, FieldMusicBrainzTrackID,
}

// TestScanReadsEveryEditorKey holds a scan and the editor to one list of keys
// per field: every key the editor reads or writes for a field in any format is
// one a scan reads the field from, and the other way round.
func TestScanReadsEveryEditorKey(t *testing.T) {
	t.Parallel()
	for _, name := range scannedFields {
		if fieldRows[name].scan == nil {
			t.Errorf("%s is scanned but has no scan list", name)
		}
	}
	for i := range fields {
		row := &fields[i]
		if row.scan == nil {
			continue
		}
		t.Run(string(row.name), func(t *testing.T) {
			t.Parallel()
			editor := editorKeys(row)
			for key := range editor {
				if !slices.Contains(row.scan, key) {
					t.Errorf("the editor's key %q is not read by a scan", key)
				}
			}
			for _, key := range row.scan {
				if !editor[key] {
					t.Errorf("a scan reads %q, which the editor doesn't", key)
				}
			}
		})
	}
}

// editorKeys returns every key the editor reads or writes for row in any
// family, but those a typed field reads, which audiometa decodes itself.
func editorKeys(row *field) map[string]bool {
	out := map[string]bool{}
	for _, fam := range []family{vorbis, id3v23, id3v24, mp4} {
		if row.typed && fam != vorbis {
			continue // audiometa's typed field reads TRCK, TPOS, trkn and their native text.
		}
		for _, key := range row.covers(fam) {
			if key != keyTDAT { // The day and month of a year: never a date by itself.
				out[key] = true
			}
		}
	}
	return out
}

// Vorbis keys the tests below seed.
const (
	keyDisc    = "DISC"
	keyDiscC   = "DISCC"
	keyTrack   = "TRACK"
	keyArtists = "ARTISTS"
	extFLAC    = "flac"
	aBand      = "Band"
)

// numberCase is a file's Vorbis number keys and what a scan must read from them.
type numberCase struct {
	name                           string
	tags                           map[string]string
	track, trackTotal, disc, disc2 int // disc2 is the disc total.
}

//nolint:gochecknoglobals // A fixed test table.
var numberCases = []numberCase{
	{name: "DISC only", tags: map[string]string{keyDisc: "1"}, disc: 1},
	{name: "DISCNUMBER only", tags: map[string]string{"DISCNUMBER": "2"}, disc: 2},
	{name: "DISCNUMBER wins over DISC", tags: map[string]string{"DISCNUMBER": "2", keyDisc: "1"}, disc: 2},
	{name: "DISC holds the total", tags: map[string]string{keyDisc: "1/2"}, disc: 1, disc2: 2},
	{name: "DISCC", tags: map[string]string{keyDisc: "1", keyDiscC: "3"}, disc: 1, disc2: 3},
	{name: "a total key wins over the suffix", tags: map[string]string{keyDisc: "1/2", keyDiscC: "3"}, disc: 1, disc2: 3},
	{name: "TRACK and TRACKC", tags: map[string]string{keyTrack: "4", keyTrackC: "9"}, track: 4, trackTotal: 9},
	{name: "TRACK holds the total", tags: map[string]string{keyTrack: "4/9"}, track: 4, trackTotal: 9},
	{
		// The album reported: its disc is only under DISC.
		name: "TRACKNUMBER and TRACKTOTAL win",
		tags: map[string]string{
			"TRACKNUMBER": "01", keyTrackTotal: "11", keyTrack: "1", keyTrackC: "10", keyDisc: "1", keyDiscC: "1",
		},
		track: 1, trackTotal: 11, disc: 1, disc2: 1,
	},
}

// TestScanReadsNumbersUnderEveryVorbisKey checks track and disc numbers and
// totals under every key the editor knows, in each Vorbis comment format, and
// that the editor shows what the scan read.
func TestScanReadsNumbersUnderEveryVorbisKey(t *testing.T) {
	t.Parallel()
	for _, format := range []string{extFLAC, "ogg", "opus"} {
		for _, tc := range numberCases {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				path := vorbisCommentFile(t, format, tc.tags)
				tw := reread(t, path)
				got := [4]int{tw.TrackNo, tw.TrackTotal, tw.DiscNo, tw.DiscTotal}
				if want := [4]int{tc.track, tc.trackTotal, tc.disc, tc.disc2}; got != want {
					t.Errorf("track, total, disc, total = %v, want %v", got, want)
				}
				if format != extFLAC {
					return // The editor writes only FLAC of the three.
				}
				shown := fieldsOf(t, path).Fields
				for i, name := range []Field{FieldTrack, FieldTrackTotal, FieldDisc, FieldDiscTotal} {
					var want []string
					if got[i] != 0 {
						want = []string{strconv.Itoa(got[i])}
					}
					if !slices.Equal(shown[name], want) {
						t.Errorf("the editor shows %s %q, a scan read %q", name, shown[name], want)
					}
				}
			})
		}
	}
}

// TestScanReadsTheEditorsMirrors checks the fields a scan used to miss where the
// editor showed them: a value held only under a key the editor keeps in step.
func TestScanReadsTheEditorsMirrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		tags  map[string][]string
		field Field
		got   func(*store.TrackWrite) string
		want  string
	}{
		{
			name: keyArtists, tags: map[string][]string{"ARTIST": nil, keyArtists: {"Mirror"}}, field: FieldArtists,
			got: func(tw *store.TrackWrite) string { return first(artistNames(tw.Artists)) }, want: "Mirror",
		},
		{
			name: keyYear, tags: map[string][]string{keyYear: {"1984"}}, field: FieldDate,
			got: func(tw *store.TrackWrite) string { return strconv.Itoa(tw.Year) }, want: "1984",
		},
		{
			name: "ALBUM_ARTIST", tags: map[string][]string{"ALBUM_ARTIST": {aBand}}, field: FieldAlbumArtists,
			got: func(tw *store.TrackWrite) string { return first(artistNames(tw.AlbumArtists)) }, want: aBand,
		},
		{
			name: "ALBUMARTISTS", tags: map[string][]string{"ALBUMARTISTS": {aBand}}, field: FieldAlbumArtists,
			got: func(tw *store.TrackWrite) string { return first(artistNames(tw.AlbumArtists)) }, want: aBand,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, "sample.flac")
			seed(t, path, tt.tags)
			tw := reread(t, path)
			if got := tt.got(&tw); got != tt.want {
				t.Errorf("a scan read %q, want %q", got, tt.want)
			}
			if shown := fieldsOf(t, path).Fields[tt.field]; first(shown) != tt.want {
				t.Errorf("the editor shows %q, want %q", shown, tt.want)
			}
		})
	}
}

// vorbisCommentFile writes a file of format (flac, ogg or opus) whose Vorbis
// comments hold tags, as the only number keys, and returns its path.
func vorbisCommentFile(t *testing.T, format string, tags map[string]string) string {
	t.Helper()
	if format == extFLAC {
		path := copySample(t, "sample.flac")
		seeded := make(map[string][]string, len(tags))
		for k, v := range tags {
			seeded[k] = []string{v}
		}
		seed(t, path, seeded)
		return path
	}
	comments := []string{"TITLE=Numbers"}
	for _, k := range slices.Sorted(maps.Keys(tags)) {
		comments = append(comments, k+"="+tags[k])
	}
	path := filepath.Join(t.TempDir(), "numbers."+format)
	if err := os.WriteFile(path, oggStream(format, comments), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// oggStream builds the smallest Ogg Vorbis or Opus stream audiometa reads: its
// headers with comments, then one page of silence a second long. The page
// checksums are left zero, which a reader of tags doesn't check.
func oggStream(format string, comments []string) []byte {
	var buf bytes.Buffer
	page := func(headerType byte, granule uint64, seq uint32, data []byte) {
		buf.WriteString("OggS")
		buf.WriteByte(0)
		buf.WriteByte(headerType)
		buf.Write(binary.LittleEndian.AppendUint64(nil, granule))
		buf.Write(binary.LittleEndian.AppendUint32(nil, 1)) // Serial number.
		buf.Write(binary.LittleEndian.AppendUint32(nil, seq))
		buf.Write(make([]byte, 4)) // Checksum.
		var segments []byte
		for rest := len(data); rest > 0; rest -= 255 {
			segments = append(segments, byte(min(rest, 255)))
		}
		buf.WriteByte(byte(len(segments))) //nolint:gosec // G115: a test packet is far under 255 segments.
		buf.Write(segments)
		buf.Write(data)
	}
	var tags bytes.Buffer
	vendor := "kitsune-test"
	tags.Write(le32(len(vendor)))
	tags.WriteString(vendor)
	tags.Write(le32(len(comments)))
	for _, c := range comments {
		tags.Write(le32(len(c)))
		tags.WriteString(c)
	}
	if format == "opus" {
		head := []byte("OpusHead")
		head = append(head, 1, 2)                            // Version, channels.
		head = binary.LittleEndian.AppendUint16(head, 312)   // Pre-skip.
		head = binary.LittleEndian.AppendUint32(head, 48000) // Input sample rate.
		head = append(head, 0, 0, 0)                         // Output gain, mapping family.
		page(0x02, 0, 0, head)
		page(0x00, 0, 1, append([]byte("OpusTags"), tags.Bytes()...))
		page(0x04, 48000+312, 2, make([]byte, 100))
		return buf.Bytes()
	}
	id := append([]byte{0x01}, "vorbis"...)
	id = binary.LittleEndian.AppendUint32(id, 0)      // Version.
	id = append(id, 2)                                // Channels.
	id = binary.LittleEndian.AppendUint32(id, 44100)  // Sample rate.
	id = binary.LittleEndian.AppendUint32(id, 0)      // Maximum bitrate.
	id = binary.LittleEndian.AppendUint32(id, 128000) // Nominal bitrate.
	id = binary.LittleEndian.AppendUint32(id, 0)      // Minimum bitrate.
	id = append(id, 0xB8, 0x01)                       // Block sizes, framing.
	page(0x02, 0, 0, id)
	comment := append(append([]byte{0x03}, "vorbis"...), tags.Bytes()...)
	page(0x00, 0, 1, append(comment, 0x01))
	page(0x00, 0, 2, append(append([]byte{0x05}, "vorbis"...), 0x01))
	page(0x04, 44100, 3, make([]byte, 100))
	return buf.Bytes()
}

// le32 is a length as a Vorbis comment header spells it.
func le32(n int) []byte {
	return binary.LittleEndian.AppendUint32(nil, uint32(n)) //nolint:gosec // G115: a test's lengths are tiny.
}
