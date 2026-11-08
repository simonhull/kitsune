package library

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

// What the WAV and AIFF samples hold, and what the tests write.
const (
	formatWAV   = "WAV"
	formatAIFF  = "AIFF"
	keyTCMP     = "TCMP"
	testTitle   = "Test Title"
	testArtist  = "Test Artist"
	testAlbum   = "Test Album"
	artistOne   = "Artist One"
	artistTwo   = "Artist Two"
	copyright   = "2001 Someone"
	keyINAM     = "INAM"
	keyIPRD     = "IPRD"
	keyAIFFCopy = "(c) " // The AIFF copyright chunk's ID ends in a space.
	wavRel      = "A/a.wav"
	aiffRel     = "B/b.aiff"
)

// scannedTrack is what a scan should make of one WAV or AIFF file.
type scannedTrack struct {
	format, title, artist, album string
	year, track                  int
	genres                       []string
}

// scanned is what a scan made of tw.
func scanned(tw *store.TrackWrite) scannedTrack {
	got := scannedTrack{format: tw.Format, title: tw.Title, album: tw.Album.Name, year: tw.Year, track: tw.TrackNo, genres: firstGenres(tw)}
	if len(tw.Artists) > 0 {
		got.artist = tw.Artists[0].Name
	}
	return got
}

// id3OnlyWAV writes tags into a copy of the untagged WAV, which gives it an ID3
// chunk and no LIST/INFO.
func id3OnlyWAV(t *testing.T) string {
	t.Helper()
	path := copySample(t, sampleWAVBare)
	if err := audiometa.Write(path, func(f *audiometa.File) error {
		f.Tags.Title, f.Tags.Artists, f.Tags.Album = "ID3 Title", []string{"ID3 Artist"}, "ID3 Album"
		f.Tags.Year, f.Tags.TrackNumber = 1999, 4
		f.Tags.Set("TCON", jazz)
		return nil
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return path
}

// WAV and AIFF files keep their tags in an ID3v2 chunk, in their own text
// (LIST/INFO; NAME, AUTH, ANNO), in both, or nowhere. A scan reads them all.
func TestScanReadsWAVAndAIFFTagsWhereverTheyAre(t *testing.T) {
	t.Parallel()
	sample := func(name string) func(*testing.T) string {
		return func(t *testing.T) string { t.Helper(); return copySample(t, name) }
	}
	tests := []struct {
		name string
		path func(*testing.T) string
		want scannedTrack
	}{
		{"INFO only", sample(sampleWAVInfo), scannedTrack{formatWAV, testTitle, testArtist, testAlbum, 2021, 3, []string{rock}}},
		{"ID3 only", id3OnlyWAV, scannedTrack{formatWAV, "ID3 Title", "ID3 Artist", "ID3 Album", 1999, 4, []string{jazz}}},
		// ID3 wins over INFO's "Info Title" and "Info Artist".
		{"both", sample(sampleWAVBoth), scannedTrack{formatWAV, testTitle, testArtist, testAlbum, 2021, 3, nil}},
		{"untagged", sample(sampleWAVBare), scannedTrack{formatWAV, "sample_bare", "", unknownAlbum, 0, 0, nil}},
		{formatAIFF, sample(sampleAIFF), scannedTrack{formatAIFF, testTitle, testArtist, testAlbum, 2021, 3, nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tw := reread(t, tt.path(t))
			if got := scanned(&tw); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("scanned %+v, want %+v", got, tt.want)
			}
			if !tw.Lossless || tw.Duration <= 0 || tw.SampleRate != 8000 {
				t.Errorf("audio = lossless %t, %v, %d Hz", tw.Lossless, tw.Duration, tw.SampleRate)
			}
		})
	}
}

// firstGenres returns the raw values of the first genre key tw holds, as the
// genre index reads them.
func firstGenres(tw *store.TrackWrite) []string {
	for _, key := range GenreKeys() {
		if v := rawValues(tw, key); v != nil {
			return v
		}
	}
	return nil
}

// wantNative checks tw holds want under each key, and nothing under absent.
func wantNative(t *testing.T, tw *store.TrackWrite, want map[string][]string, absent []string) {
	t.Helper()
	for key, values := range want {
		if v := rawValues(tw, key); !slices.Equal(v, values) {
			t.Errorf("%q = %q, want %q", key, v, values)
		}
	}
	for _, key := range absent {
		if v := rawValues(tw, key); v != nil {
			t.Errorf("%q = %q, want none: a mirror is kept only where it is", key, v)
		}
	}
}

// An edit of a WAV or AIFF file writes its ID3 chunk and keeps the native text
// the file has in step, so a player that reads only that sees the edit too.
func TestEditKeepsNativeTextInStep(t *testing.T) {
	t.Parallel()
	tests := []struct {
		sample string
		want   map[string][]string // Native keys after the edit.
		absent []string            // Native keys the file lacked, which stay absent.
	}{
		{sampleWAVInfo, map[string][]string{
			keyINAM: {newTitle}, "IART": {artistOne, artistTwo}, keyIPRD: {albumValue},
			"IGNR": {jazz}, keyICRD: {newDate}, "ICMT": {aComment}, "IPRT": {"7"},
		}, []string{"ICOP", "ITRK"}},
		{sampleAIFF, map[string][]string{"NAME": {newTitle}, "ANNO": {aComment}, keyAIFFCopy: {copyright}}, []string{"AUTH"}},
	}
	edit := Edit{Fields: map[Field][]string{
		FieldTitle: {newTitle}, FieldArtists: {artistOne, artistTwo}, FieldAlbum: {albumValue},
		FieldGenres: {jazz}, FieldDate: {newDate}, FieldComment: {aComment}, FieldTrack: {"7"},
		FieldCopyright: {copyright},
	}}
	for _, tt := range tests {
		t.Run(tt.sample, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, tt.sample)
			before := reread(t, path)
			if err := audiometa.Write(path, edit.apply); err != nil {
				t.Fatalf("Write: %v", err)
			}
			got := reread(t, path)
			wantNative(t, &got, tt.want, tt.absent)
			if got.Title != newTitle || got.Album.Name != albumValue || got.TrackNo != 7 || got.Year != newYear {
				t.Errorf("scan saw %q on %q, #%d, %d", got.Title, got.Album.Name, got.TrackNo, got.Year)
			}
			if *got.Digest != *before.Digest {
				t.Error("the edit changed the audio digest")
			}
		})
	}
}

// A raw edit can't reach a native key a field owns, any more than its ID3 frame.
func TestNativeTextIsReadOnlyRaw(t *testing.T) {
	t.Parallel()
	for _, key := range []string{keyINAM, "IART", keyIPRD, "IGNR", keyICRD, "ICMT", "ICOP", "ITRK", "IPRT", "NAME", "AUTH", "ANNO", keyAIFFCopy} {
		if !readOnlyKey(key) {
			t.Errorf("%q can be edited raw", key)
		}
	}
	tags := fieldsOf(t, copySample(t, sampleWAVInfo))
	for _, raw := range tags.Tags {
		if raw.Key == keyINAM || raw.Key == keyIPRD {
			t.Errorf("%s shows as a raw tag; its field covers it", raw.Key)
		}
	}
	if !slices.Equal(tags.Fields[FieldTitle], []string{testTitle}) {
		t.Errorf("title = %q: an INFO-only file's title reads from INAM", tags.Fields[FieldTitle])
	}
}

func TestChunkID3Version(t *testing.T) {
	t.Parallel()
	tests := []struct {
		sample string
		want   family
	}{
		{sampleWAVBoth, id3v23},
		{sampleAIFF, id3v24},
		{sampleWAVInfo, id3v24}, // No ID3 chunk: audiometa adds an ID3v2.4 one.
	}
	for _, tt := range tests {
		got, err := chunkFamily(filepath.Join("testdata", tt.sample))
		if err != nil || got != tt.want {
			t.Errorf("%s: family %v, %v; want %v", tt.sample, got, err, tt.want)
		}
	}
	if _, err := chunkID3Version(filepath.Join("testdata", sampleWAVInfo)); !errors.Is(err, errNoID3Chunk) {
		t.Errorf("INFO only: err = %v, want errNoID3Chunk", err)
	}
	short := filepath.Join(t.TempDir(), "short.wav")
	if err := os.WriteFile(short, []byte("RIFF"), fileMode); err != nil {
		t.Fatal(err)
	}
	if _, err := chunkID3Version(short); err == nil || errors.Is(err, errNoID3Chunk) {
		t.Errorf("a short file: err = %v, want a read error", err)
	}
}

// A WAV an older audiometa called unsupported reads as a track on the next
// scan, though the file hasn't changed.
func TestUnsupportedWAVIsReadUnderANewReader(t *testing.T) {
	t.Parallel()
	l := issueLib(t, "a.wav", sampleWAVInfo, olderReader)
	if stats := l.scan(Quick); stats.Added != 1 || len(l.issues()) != 0 {
		t.Errorf("added %d, issues %+v; want the WAV read and its issue gone", stats.Added, l.issues())
	}
}

// wavAlbums is a scanned library with a WAV album in A and an AIFF album in B.
type wavAlbums struct {
	*testLib

	wav, aiff *store.AlbumDetail
}

func wavLib(t *testing.T) *wavAlbums {
	t.Helper()
	l := &wavAlbums{testLib: newTestLib(t, 1)}
	root := l.roots[0]
	l.copyTo(root, wavRel, sampleWAVBoth)
	l.copyTo(root, aiffRel, sampleAIFF)
	l.opts.TagWrites = true
	l.scanner = NewScanner(l.st, l.opts)
	l.scan(Quick)
	page, err := l.st.Albums(t.Context(), store.SortAdded, mostRead, "")
	if err != nil || len(page.Albums) != 2 {
		t.Fatalf("albums = %+v, %v; want 2", page.Albums, err)
	}
	for i := range page.Albums {
		d, albumErr := l.st.Album(t.Context(), page.Albums[i].ID)
		if albumErr != nil {
			t.Fatal(albumErr)
		}
		if d.Tracks[0].Format == formatWAV {
			l.wav = &d
		} else {
			l.aiff = &d
		}
	}
	return l
}

// wantEdited checks the file at rel holds the new year, its audio untouched,
// and the store shows the year on its album.
func wantEdited(t *testing.T, l *testLib, rel string, before *store.TrackWrite, album string) {
	t.Helper()
	got := reread(t, filepath.Join(l.roots[0], filepath.FromSlash(rel)))
	if got.Year != newYear || *got.Digest != *before.Digest {
		t.Errorf("%s: year %d, digest kept %t; want %d, kept", rel, got.Year, *got.Digest == *before.Digest, newYear)
	}
	if d, err := l.st.Album(t.Context(), album); err != nil || d.Year != newYear {
		t.Errorf("album %s = year %d, %v; want %d", album, d.Year, err, newYear)
	}
}

// before reads the file at rel under l's root as a scan would.
func (l *testLib) before(rel string) store.TrackWrite {
	l.t.Helper()
	return reread(l.t, filepath.Join(l.roots[0], filepath.FromSlash(rel)))
}

func TestEditAlbumOfWAVAndAIFF(t *testing.T) {
	t.Parallel()
	l := wavLib(t)
	for _, tt := range []struct {
		album *store.AlbumDetail
		rel   string
	}{{l.wav, wavRel}, {l.aiff, aiffRel}} {
		before := l.before(tt.rel)
		if err := errOf(l.scanner.EditAlbum(t.Context(), tt.album.ID, tt.album.ETag, &AlbumEdit{All: *yearEdit()})); err != nil {
			t.Fatalf("EditAlbum %s: %v", tt.rel, err)
		}
		wantEdited(t, l.testLib, tt.rel, &before, tt.album.ID)
	}
}

func TestEditTracksAcrossWAVAndAIFF(t *testing.T) {
	t.Parallel()
	l := wavLib(t)
	wav, aiff := l.wav, l.aiff
	wavBefore, aiffBefore := l.before(wavRel), l.before(aiffRel)
	edit := &TracksEdit{
		Albums: []AlbumETag{{ID: wav.ID, ETag: wav.ETag}, {ID: aiff.ID, ETag: aiff.ETag}},
		All:    *yearEdit(),
		Tracks: map[string]Edit{wav.Tracks[0].ID: {}, aiff.Tracks[0].ID: {}},
	}
	if err := l.scanner.EditTracks(t.Context(), edit); err != nil {
		t.Fatal(err)
	}
	wantEdited(t, l.testLib, wavRel, &wavBefore, wav.ID)
	wantEdited(t, l.testLib, aiffRel, &aiffBefore, aiff.ID)
}
