package library

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
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

// The fields of the table. Album, artists, album artists and compilation decide
// which album a track groups into, so editing them can move a track to another.
const (
	FieldTitle           Field = "title"
	FieldAlbum           Field = "album"
	FieldArtists         Field = "artists"
	FieldAlbumArtists    Field = "albumArtists"
	FieldCompilation     Field = "compilation"
	FieldTrack           Field = "track"
	FieldTrackTotal      Field = "trackTotal"
	FieldDisc            Field = "disc"
	FieldDiscTotal       Field = "discTotal"
	FieldDiscSubtitle    Field = "discSubtitle"
	FieldDate            Field = "date"
	FieldGenres          Field = "genres"
	FieldComposers       Field = "composers"
	FieldConductor       Field = "conductor"
	FieldLyricist        Field = "lyricist"
	FieldPerformers      Field = "performers"
	FieldRemixer         Field = "remixer"
	FieldGrouping        Field = "grouping"
	FieldLabel           Field = "label"
	FieldCatalogNumber   Field = "catalogNumber"
	FieldBarcode         Field = "barcode"
	FieldISRC            Field = "isrc"
	FieldOriginalDate    Field = "originalDate"
	FieldLanguage        Field = "language"
	FieldBPM             Field = "bpm"
	FieldKey             Field = "key"
	FieldCopyright       Field = "copyright"
	FieldComment         Field = "comment"
	FieldLyrics          Field = "lyrics"
	FieldSortTitle       Field = "sortTitle"
	FieldSortArtist      Field = "sortArtist"
	FieldSortAlbum       Field = "sortAlbum"
	FieldSortAlbumArtist Field = "sortAlbumArtist"

	// FieldMusicBrainzAlbumID and the fields after it are MusicBrainz's IDs, which
	// a match writes. The recording is what Picard calls the track in Vorbis, and
	// the track is the release's track.
	FieldMusicBrainzAlbumID        Field = "musicbrainzAlbumId"
	FieldMusicBrainzReleaseGroupID Field = "musicbrainzReleaseGroupId"
	FieldMusicBrainzAlbumArtistID  Field = "musicbrainzAlbumArtistId"
	FieldMusicBrainzArtistID       Field = "musicbrainzArtistId"
	FieldMusicBrainzRecordingID    Field = "musicbrainzRecordingId"
	FieldMusicBrainzTrackID        Field = "musicbrainzTrackId"
)

// Edit is a change to a file's tags.
type Edit struct {
	// Fields holds new values. A field it doesn't name stays as it is, and one
	// named with no values is removed.
	Fields map[Field][]string
	// Tags does the same for raw tags, by the keys [TrackTags] shows.
	Tags map[string][]string
}

// Empty reports whether the edit names nothing.
func (e *Edit) Empty() bool { return len(e.Fields) == 0 && len(e.Tags) == 0 }

// over returns e with own on top: what both name takes own's value.
func (e *Edit) over(own *Edit) Edit {
	return Edit{Fields: merged(e.Fields, own.Fields), Tags: merged(e.Tags, own.Tags)}
}

func merged[K comparable](base, top map[K][]string) map[K][]string {
	out := make(map[K][]string, len(base)+len(top))
	maps.Copy(out, base)
	maps.Copy(out, top)
	return out
}

// lyricTimestamp matches the time code that starts a line of synced lyrics.
var lyricTimestamp = regexp.MustCompile(`(?m)^\s*\[\d+:\d{2}([.:]\d+)?\]`)

// family is a way of naming tags: the formats that share one have the same keys.
type family int

const (
	vorbis family = iota
	// id3v23 also covers ID3v2.2, whose frames audiometa reports by their ID3v2.3
	// names. It rewrites such a tag as ID3v2.4, so a month, a day or an original
	// date is refused there.
	id3v23
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
	// native are the text tags a WAV or AIFF file keeps beside its ID3 chunk
	// (LIST/INFO, NAME, AUTH...): mirrors of mp3, read after it and kept in
	// step only where they exist.
	native []string
	// typed reports a field that ID3 writes through audiometa's typed field,
	// which keeps the native text in step itself, so its native keys aren't
	// written here: audiometa refuses an edit of both a field and a key it binds
	// to the field.
	typed bool
	// mp3v23 replaces mp3 on an ID3v2.3 tag, which lacks some frames.
	mp3v23 []string
	// dependents are deleted on a change, in every format: they were made for the old value.
	dependents []string
	// matching are copies that a change follows only while they equal the old value.
	matching []string
	multi    bool // It may hold several values.
	required bool // It can't be removed.
	// frozen reports values a client may see but not replace.
	frozen func([]string) bool
	// valid says what a value must look like. Nil takes any.
	valid func(family, string) error
	// scan holds the keys a scan reads the field from, in every format, in the
	// order it tries them; the editor reads them too, so the two never disagree.
	// It covers every key of flac, mp3, mp3v23, m4a and native, which
	// TestScanReadsEveryEditorKey checks. Nil, for a field a scan doesn't read,
	// means the editor reads the first of the family's keys with any.
	scan []string
	// read returns the file's values, for a field whose values need more than
	// the first of scan with any. A scan reads the field through it too.
	read func(t *audiometa.Tags) []string
	// write saves values, or with none removes the field. Nil means [writeKeys].
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

// Keys the table's functions treat apart.
const (
	keyYear    = "YEAR"
	keyTYER    = "TYER"
	keyTDAT    = "TDAT"
	keyTORY    = "TORY"
	keyComment = "COMM" // MP3: audiometa keeps it out of the raw tags.
	keyICRD    = "ICRD" // WAV: the date in LIST/INFO.
	keyLyrics  = "USLT" // MP3: audiometa takes it through the typed field.
	keyTSST    = "TSST"
	// keyDiscSubtitle is the disc's name in Vorbis and M4A, and a TXXX in ID3.
	keyDiscSubtitle = "DISCSUBTITLE"
	// keyRecordingUFID is the MP3 frame MusicBrainz Picard keeps the recording's ID in.
	keyRecordingUFID = "UFID:http://musicbrainz.org"
)

// picardNames spells the TXXX descriptions and M4A freeform names of the
// MusicBrainz IDs as MusicBrainz Picard writes them, by the raw key each is read
// under. A new frame or item gets this name, since some readers match it
// case-sensitively; one the file has keeps its own, so nothing is written twice.
// Vorbis names are case-insensitive and stay as their keys.
//
//nolint:gochecknoglobals // A fixed table, never written.
var picardNames = spelled(
	"MusicBrainz Album Id", "MusicBrainz Release Group Id", "MusicBrainz Album Artist Id",
	"MusicBrainz Artist Id", "MusicBrainz Track Id", "MusicBrainz Release Track Id",
)

// spelled maps each name's raw key, the name upper-cased, to the name.
func spelled(names ...string) map[string]string {
	out := make(map[string]string, len(names))
	for _, n := range names {
		out[strings.ToUpper(n)] = n
	}
	return out
}

// fields is the table, in the order an edit applies: a field comes before those
// among its dependents, so that one request can change both. Raw names are checked
// by TestEveryFieldRoundTrips and TestEditLandsInEveryOwnedKey, on files carrying
// the duplicate keys of a real library. Only formats audiometa can write have a
// family.
//
//nolint:gochecknoglobals // A fixed table, never written.
var fields = []field{
	{
		name: FieldTitle, flac: keys("TITLE"), mp3: keys("TIT2"), m4a: keys("\xA9nam"), required: true, native: keys("INAM", "NAME"),
		dependents: []string{"TITLESORT", "TITLESORTEN", "TSOT", "sonm"},
	},
	{
		name: FieldAlbum, flac: keys("ALBUM"), mp3: keys("TALB"), m4a: keys("\xA9alb"), native: keys("IPRD"),
		dependents: []string{"ALBUMSORT", "ALBUMSORTEN", "TSOA", "soal"},
	},
	{
		name: FieldArtists, flac: keys("ARTIST", "ARTISTS"), mp3: keys("TPE1", "ARTISTS"), m4a: keys("\xA9ART", "ARTISTS"),
		multi: true, native: keys("IART", "AUTH"), scan: keysArtists,
		dependents: slices.Concat(keysArtistSort, keysArtistMBID, keys("ARTISTS_SORT", "ARTIST_CREDIT", "ARTISTS_CREDIT", "ARTIST CREDIT")),
	},
	{
		name:  FieldAlbumArtists,
		flac:  keys("ALBUMARTIST", "ALBUM ARTIST", "ALBUM_ARTIST", "ALBUMARTISTS", "ALBUM_ARTISTS"),
		mp3:   keys("TPE2", "ALBUMARTIST", "ALBUM ARTIST", "ALBUM_ARTIST", "ALBUMARTISTS", "ALBUM_ARTISTS"),
		m4a:   keys("aART"),
		multi: true, scan: keysAlbumArtist,
		dependents: slices.Concat(keysAlbumArtistSort, keysAlbumArtistMBID, keys("ALBUMARTISTS_SORT", "ALBUMARTIST_CREDIT", "ALBUMARTISTS_CREDIT", "ALBUM ARTIST CREDIT")),
	},
	{name: FieldCompilation, flac: keys("COMPILATION"), mp3: keys("TCMP"), m4a: keys("cpil"), valid: validFlag, scan: keysCompilation},
	number(FieldTrack, trackNumber, vorbisTrack, keys("TRCK"), keys("trkn"), "ITRK", "IPRT"),
	numberTotal(FieldTrackTotal, trackTotal, vorbisTrack, keys("TRCK"), keys("trkn")),
	number(FieldDisc, discNumber, vorbisDisc, keys("TPOS"), keys("disk")),
	numberTotal(FieldDiscTotal, discTotal, vorbisDisc, keys("TPOS"), keys("disk")),
	{
		// ID3v2.3 has no TSST, which audiometa won't write there: its name is a
		// TXXX. A stray TSST on such a file would still win a scan, so a change
		// takes it off; on ID3v2.4 the change writes it straight back.
		name: FieldDiscSubtitle, flac: keys(keyDiscSubtitle), mp3: keys(keyTSST, keyDiscSubtitle),
		mp3v23: keys(keyDiscSubtitle), m4a: keys(keyDiscSubtitle),
		dependents: []string{keyTSST}, scan: keysDiscSubtitle,
	},
	{
		name: FieldDate, flac: keys("DATE", keyYear), mp3: keys("TDRC"), mp3v23: keys(keyTYER, keyTDAT), m4a: keys("\xA9day"),
		dependents: []string{"TDRC", keyTYER, keyTDAT, "TIME"}, native: keys(keyICRD),
		valid: validReleaseDate, scan: keysDate, read: readDate, write: writeDate,
	},
	{
		name: FieldGenres, flac: keys("GENRE"), mp3: keys("TCON"), m4a: keys("\xA9gen"), multi: true, native: keys("IGNR"),
		dependents: []string{"gnre"}, scan: keysGenre,
	},
	{
		name: FieldComposers, flac: keys("COMPOSER"), mp3: keys("TCOM"), m4a: keys("\xA9wrt"), multi: true,
		dependents: []string{"COMPOSERSORT"}, scan: keysComposer,
	},
	{name: FieldConductor, flac: keys("CONDUCTOR"), mp3: keys("TPE3"), m4a: keys("CONDUCTOR"), multi: true},
	{name: FieldLyricist, flac: keys("LYRICIST"), mp3: keys("TEXT"), m4a: keys("LYRICIST"), multi: true},
	// MP3 and M4A have no standard home for performers: these are custom keys.
	{name: FieldPerformers, flac: keys("PERFORMER"), mp3: keys("PERFORMER"), m4a: keys("PERFORMER"), multi: true},
	{name: FieldRemixer, flac: keys("REMIXER"), mp3: keys("TPE4"), m4a: keys("REMIXER"), multi: true},
	{name: FieldGrouping, flac: keys("GROUPING"), mp3: keys("GRP1", "TIT1"), m4a: keys("\xA9grp")},
	{name: FieldLabel, flac: keys("LABEL", "PUBLISHER"), mp3: keys("TPUB"), m4a: keys("LABEL"), multi: true},
	{name: FieldCatalogNumber, flac: keys("CATALOGNUMBER"), mp3: keys("CATALOGNUMBER"), m4a: keys("CATALOGNUMBER"), multi: true},
	{name: FieldBarcode, flac: keys("BARCODE"), mp3: keys("BARCODE"), m4a: keys("BARCODE"), multi: true},
	{name: FieldISRC, flac: keys("ISRC"), mp3: keys("TSRC"), m4a: keys("ISRC"), multi: true},
	{
		name: FieldOriginalDate, flac: keys("ORIGINALDATE"), mp3: keys("TDOR"), mp3v23: keys(keyTORY), m4a: keys("ORIGINALDATE"),
		dependents: []string{"TDOR", keyTORY},
		valid:      validDate, scan: keysOriginalDate, write: writeOriginalDate,
	},
	{name: FieldLanguage, flac: keys("LANGUAGE"), mp3: keys("TLAN"), m4a: keys("LANGUAGE"), multi: true},
	{name: FieldBPM, flac: keys("BPM"), mp3: keys("TBPM"), m4a: keys("tmpo"), valid: validBPM},
	{name: FieldKey, flac: keys("INITIALKEY"), mp3: keys("TKEY"), m4a: keys("INITIALKEY")},
	{name: FieldCopyright, flac: keys("COPYRIGHT"), mp3: keys("TCOP"), m4a: keys("cprt"), native: keys("ICOP", "(c) ")},
	{
		name: FieldComment, flac: keys("COMMENT", "DESCRIPTION"), mp3: keys(keyComment), m4a: keys("\xA9cmt"),
		native: keys("ICMT", "ANNO"), typed: true,
	},
	{
		name: FieldLyrics, flac: keys("LYRICS"), mp3: keys(keyLyrics), m4a: keys("\xA9lyr"),
		matching: []string{"UNSYNCEDLYRICS"}, frozen: timestamped,
	},
	{name: FieldSortTitle, flac: keys("TITLESORT"), mp3: keys("TSOT"), m4a: keys("sonm"), scan: keysTitleSort},
	{name: FieldSortArtist, flac: keys("ARTISTSORT"), mp3: keys("TSOP"), m4a: keys("soar"), scan: keysArtistSort},
	{name: FieldSortAlbum, flac: keys("ALBUMSORT"), mp3: keys("TSOA"), m4a: keys("soal"), scan: keysAlbumSort},
	{
		name: FieldSortAlbumArtist, flac: keys("ALBUMARTISTSORT"), mp3: keys("TSO2", "ALBUMARTISTSORT"), m4a: keys("soaa"),
		scan: keysAlbumArtistSort,
	},
	// The MBIDs come after artists and album artists, whose change deletes the
	// artist IDs as dependents: one request can rename and give the new IDs.
	mbid(FieldMusicBrainzAlbumID, false, keysAlbumMBID),
	mbid(FieldMusicBrainzReleaseGroupID, false, keysReleaseGroupMBID),
	mbid(FieldMusicBrainzAlbumArtistID, true, keysAlbumArtistMBID),
	mbid(FieldMusicBrainzArtistID, true, keysArtistMBID),
	{
		// The recording's ID is a UFID in ID3, which a TXXX left by another tagger
		// follows, and a freeform "MusicBrainz Track Id" in M4A.
		name: FieldMusicBrainzRecordingID, flac: keys(keysTrackMBID[0]), mp3: keys(keyRecordingUFID, keysTrackMBID[2]),
		m4a: keys(keysTrackMBID[2]), valid: validMBID, scan: keysTrackMBID,
	},
	mbid(FieldMusicBrainzTrackID, false, keysReleaseTrackMBID),
}

func keys(names ...string) []string { return names }

// mbid is the row of a MusicBrainz ID. candidates holds its Vorbis key, then
// its ID3 TXXX description, which M4A's freeform atom shares; [picardNames]
// spells the latter.
func mbid(name Field, multi bool, candidates []string) field {
	vorbisKey, otherKey := candidates[0], candidates[1]
	return field{name: name, flac: keys(vorbisKey), mp3: keys(otherKey), m4a: keys(otherKey), multi: multi, valid: validMBID, scan: candidates}
}

// mbidPattern is the shape of a MusicBrainz ID: a UUID, as MusicBrainz writes it.
var mbidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// validMBID takes only a MusicBrainz ID.
func validMBID(_ family, v string) error {
	if !mbidPattern.MatchString(v) {
		return fmt.Errorf("%q is not a MusicBrainz ID: %w", v, ErrUnsupportedEdit)
	}
	return nil
}

func trackNumber(t *audiometa.Tags) *int { return &t.TrackNumber }
func trackTotal(t *audiometa.Tags) *int  { return &t.TrackTotal }
func discNumber(t *audiometa.Tags) *int  { return &t.DiscNumber }
func discTotal(t *audiometa.Tags) *int   { return &t.DiscTotal }

// timestamped reports lyrics with time codes, which a plain text box would ruin.
func timestamped(values []string) bool {
	return slices.ContainsFunc(values, lyricTimestamp.MatchString)
}

// vorbisNumbers are the Vorbis keys of a number and of its total. A number key
// may hold the total too, as "3/12", which audiometa reads as the total while no
// total key holds one; a change to either keeps that suffix in step.
type vorbisNumbers struct{ number, total []string }

//nolint:gochecknoglobals // Fixed tables, never written.
var (
	vorbisTrack = vorbisNumbers{number: keys("TRACKNUMBER", "TRACK"), total: keys("TRACKTOTAL", "TOTALTRACKS", "TRACKC")}
	vorbisDisc  = vorbisNumbers{number: keys("DISCNUMBER", "DISC"), total: keys("DISCTOTAL", "TOTALDISCS", "DISCC")}
)

// writeNumber saves the number, or with none removes it. A number key that
// holds the total keeps it; once the number is gone, the total key holds it.
func (n vorbisNumbers) writeNumber(t *audiometa.Tags, values []string) {
	if len(values) == 0 {
		total := n.suffix(t)
		writeKeys(t, n.number, nil)
		if total != "" && firstValues(t, n.total) == nil {
			t.Set(n.total[0], total)
		}
		return
	}
	for i, key := range n.number {
		if i > 0 && t.Get(key) == nil {
			continue // A mirror, kept in step only where it exists.
		}
		v := values[0]
		if _, total, held := strings.Cut(t.GetFirst(key), "/"); held {
			v += "/" + total
		}
		t.Set(key, v)
	}
}

// writeTotal saves the total, or with none removes it, wherever the file keeps
// it: in the total keys and after a number. A file that keeps it only after a
// number keeps it there.
func (n vorbisNumbers) writeTotal(t *audiometa.Tags, values []string) {
	after := false
	for _, key := range n.number {
		num, _, held := strings.Cut(t.GetFirst(key), "/")
		if !held {
			continue
		}
		after = true
		if len(values) > 0 {
			num += "/" + values[0]
		}
		set(t, key, clean([]string{num}))
	}
	if !after || len(values) == 0 || slices.ContainsFunc(n.total, func(key string) bool { return t.Get(key) != nil }) {
		writeKeys(t, n.total, values)
	}
}

// readNumber returns the number a scan and the editor read: typed, audiometa's
// field, which reads only the first number key; else the number part of the
// first number key that holds one; else 0.
func (n vorbisNumbers) readNumber(t *audiometa.Tags, typed int) int {
	if typed != 0 {
		return typed
	}
	for _, key := range n.number {
		num, _, _ := strings.Cut(t.GetFirst(key), "/")
		if v, ok := positive(num); ok {
			return v
		}
	}
	return 0
}

// readTotal returns the total a scan and the editor read: typed, audiometa's
// field, which reads only some of the keys; else the first total key that holds
// one; else what the first number key holding one keeps after its number, an
// explicit total winning over that as it does for audiometa; else 0.
func (n vorbisNumbers) readTotal(t *audiometa.Tags, typed int) int {
	if typed != 0 {
		return typed
	}
	for _, key := range n.total {
		if v, ok := positive(t.GetFirst(key)); ok {
			return v
		}
	}
	for _, key := range n.number {
		if _, total, held := strings.Cut(t.GetFirst(key), "/"); held {
			if v, ok := positive(total); ok {
				return v
			}
		}
	}
	return 0
}

// positive parses s, trimmed, as a number above zero.
func positive(s string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	return v, err == nil && v > 0
}

// suffix returns the total the first number key holding one keeps after its
// number, or "".
func (n vorbisNumbers) suffix(t *audiometa.Tags) string {
	for _, key := range n.number {
		if _, total, held := strings.Cut(t.GetFirst(key), "/"); held {
			return total
		}
	}
	return ""
}

// number is the row of a track or disc number, and [numberTotal] that of its
// total. Their typed fields read every format, since a Vorbis value can be
// "3/12", and write the two that keep a number and a total together. Vorbis
// keeps them under the raw keys, and audiometa reads only some: a value held
// only by a mirror is still the field's, for a scan as for the editor.
func number(name Field, pick func(*audiometa.Tags) *int, v vorbisNumbers, mp3, m4a []string, native ...string) field {
	return numeric(name, pick, v.number, v.readNumber, v.writeNumber, mp3, m4a, native)
}

// numberTotal is the row of a track or disc total; see [number].
func numberTotal(name Field, pick func(*audiometa.Tags) *int, v vorbisNumbers, mp3, m4a []string) field {
	return numeric(name, pick, v.total, v.readTotal, v.writeTotal, mp3, m4a, nil)
}

func numeric(
	name Field, pick func(*audiometa.Tags) *int, flac []string,
	readVorbis func(*audiometa.Tags, int) int, writeVorbis func(*audiometa.Tags, []string), mp3, m4a, native []string,
) field {
	return field{
		name: name, flac: flac, mp3: mp3, m4a: m4a, native: native, typed: true, valid: validNumber, scan: flac,
		read: func(t *audiometa.Tags) []string {
			if n := readVorbis(t, *pick(t)); n != 0 {
				return []string{strconv.Itoa(n)}
			}
			return nil
		},
		write: func(t *audiometa.Tags, fam family, _, values []string) {
			if fam == vorbis {
				writeVorbis(t, values)
				return
			}
			*pick(t) = 0
			if len(values) > 0 {
				*pick(t), _ = strconv.Atoi(values[0]) // validNumber read it.
			}
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

// flagOn is the one value a flag takes: clearing it is how it goes off.
const flagOn = "1"

// validFlag takes only [flagOn].
func validFlag(_ family, v string) error {
	if v != flagOn {
		return fmt.Errorf("%q is not %s: %w", v, flagOn, ErrUnsupportedEdit)
	}
	return nil
}

// validBPM refuses what M4A's integer can't hold, or would read back otherwise
// ("07" as "7"), which audiometa refuses too.
func validBPM(fam family, v string) error {
	if n, err := strconv.ParseInt(v, 10, 16); (err != nil || strconv.FormatInt(n, 10) != v) && fam == mp4 {
		return fmt.Errorf("%q is not a whole number: %w", v, ErrUnsupportedEdit)
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
	if len(values) == 0 {
		writeKeys(t, keys, nil)
		return
	}
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

// writeOriginalDate saves the date, or its year where the frame holds no more.
func writeOriginalDate(t *audiometa.Tags, _ family, keys, values []string) {
	if len(values) == 0 {
		writeKeys(t, keys, nil)
		return
	}
	date := values[0]
	if keys[0] == keyTORY {
		date = date[:yearDigits]
	}
	t.Set(keys[0], date)
}

// writeKeys saves values to the first of keys, and to each of the others that
// exists. With no values, that removes them all.
func writeKeys(t *audiometa.Tags, keys, values []string) {
	set(t, keys[0], values)
	for _, key := range keys[1:] {
		if t.Get(key) != nil {
			t.Set(key, values...)
		}
	}
}

// get returns the values of key, which may be one of the keys audiometa keeps
// behind a typed field.
func get(t *audiometa.Tags, key string) []string {
	switch key {
	case keyComment:
		return []string{t.Comment}
	case keyLyrics:
		return []string{t.Lyrics}
	}
	return t.Get(key)
}

// set is the writing side of [get].
func set(t *audiometa.Tags, key string, values []string) {
	switch key {
	case keyComment:
		t.Comment = strings.Join(values, "")
	case keyLyrics:
		t.Lyrics = strings.Join(values, "")
	default:
		if name, ok := picardNames[key]; ok {
			t.SetNamed(name, values...)
			return
		}
		t.Set(key, values...)
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

// values returns what the file holds. Empty values read as absent. A WAV or
// AIFF file's native text counts after its ID3 frames, as audiometa reads it.
func (f *field) values(t *audiometa.Tags, fam family) []string {
	switch {
	case f.read != nil:
		return f.read(t)
	case f.scan != nil:
		return firstValues(t, f.scan)
	}
	return firstValues(t, f.covers(fam))
}

// covers returns the keys the field reads in fam, its native text included.
func (f *field) covers(fam family) []string {
	if isID3(fam) {
		return slices.Concat(f.keys(fam), f.native)
	}
	return f.keys(fam)
}

// isID3 reports whether fam is an ID3v2 tag, an MP3's or a WAV or AIFF file's.
func isID3(fam family) bool { return fam == id3v23 || fam == id3v24 }

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
	old := f.values(t, fam)
	if slices.Equal(values, old) {
		return nil
	}
	if f.frozen != nil && f.frozen(old) {
		return fmt.Errorf("%s is read-only: %w", f.name, ErrUnsupportedEdit)
	}
	f.save(t, fam, old, values)
	return nil
}

// save replaces old with values.
func (f *field) save(t *audiometa.Tags, fam family, old, values []string) {
	t.Delete(f.dependents...)
	for _, key := range f.matching {
		if v := t.Get(key); v != nil && slices.Equal(clean(v), old) {
			t.Set(key, values...)
		}
	}
	if isID3(fam) && !f.typed {
		for _, key := range f.native {
			if t.Get(key) != nil {
				t.Set(key, values...)
			}
		}
	}
	if f.write != nil {
		f.write(t, fam, f.keys(fam), values)
		return
	}
	writeKeys(t, f.keys(fam), values)
}

// clean returns values trimmed, without the empty ones, as valid UTF-8.
func clean(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(strings.ToValidUTF8(v, "\uFFFD")); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// firstValues returns the cleaned values of the first of keys that has any.
func firstValues(t *audiometa.Tags, keys []string) []string {
	for _, key := range keys {
		if v := clean(get(t, key)); len(v) > 0 {
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
	case audiometa.FormatWAV, audiometa.FormatAIFF:
		return chunkFamily(f.Path)
	case audiometa.FormatM4A, audiometa.FormatM4B:
		return mp4, nil
	case audiometa.FormatUnknown, audiometa.FormatOgg, audiometa.FormatOpus:
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

// id3v24Major is the major version of an ID3v2.4 tag.
const id3v24Major = 4

// chunkFamily reads the version of a WAV or AIFF file's ID3v2 chunk. A file
// without one gets ID3v2.4, which audiometa writes when it adds the chunk.
func chunkFamily(path string) (family, error) {
	version, err := chunkID3Version(path)
	switch {
	case errors.Is(err, errNoID3Chunk):
		return id3v24, nil
	case err != nil:
		return 0, err
	case version < id3v24Major:
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
	return e.applyTags(&f.Tags, fam)
}

// readsFromID3v1 reports whether f's names came from an ID3v1 trailer. The
// table reads ID3v2 frames, which such a file lacks, so an editor would show
// its title, artist, album and year as empty though a scan shows them.
func readsFromID3v1(f *audiometa.File) bool {
	t := &f.Tags
	return f.Format == audiometa.FormatMP3 && (t.Title != "" || t.Artist != "") && multi(t, keysID3v2Names) == nil
}
