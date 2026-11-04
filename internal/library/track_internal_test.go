package library

import (
	"slices"
	"testing"
	"time"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	formatFLAC     = "FLAC"
	songName       = "Song"
	soloName       = "Solo"
	featName       = "A feat. B"
	sortA          = "A-sort"
	keyArtistSort  = "ARTISTSORT"
	keyArtistMBID  = "MUSICBRAINZ_ARTISTID"
	keyID3ArtistID = "MUSICBRAINZ ARTIST ID"
	albumY         = "Album/y.flac"
)

func testInfo() *FileInfo {
	return &FileInfo{
		Root: rootMusic, RelPath: "Artist/Album/03 Song.flac",
		Size: 1234, MTime: time.UnixMilli(1_700_000_000_000), Digest: [32]byte{1, 2, 3},
	}
}

func basicsFile() *audiometa.File {
	f := &audiometa.File{}
	f.Format = audiometa.FormatFLAC
	f.Tags.Title = "  Song "
	f.Tags.TrackNumber, f.Tags.TrackTotal, f.Tags.DiscNumber, f.Tags.DiscTotal = 3, 12, 1, 2
	f.Tags.Year = 2001
	f.Tags.OriginalDate = "1997-05-21"
	f.Audio.Codec = "FLAC"
	f.Audio.Duration = 3 * time.Minute
	f.Audio.SampleRate, f.Audio.BitDepth, f.Audio.Channels, f.Audio.Bitrate = 44100, 16, 2, 900000
	f.Audio.Lossless = true
	f.Audio.ReplayGain = &audiometa.ReplayGainInfo{TrackGain: -6.5, TrackPeak: 0.9, AlbumGain: -7, AlbumPeak: 1.1}
	f.Warnings = []audiometa.Warning{
		{Stage: audiometa.StageMetadata, Message: "bad frame"},
		{Stage: audiometa.StageTechnical, Message: "odd", Offset: 42},
	}
	return f
}

func basicsTW() store.TrackWrite { return NewTrackWrite(basicsFile(), testInfo()) }

func TestNewTrackWriteBasicsIdentity(t *testing.T) {
	t.Parallel()
	tw := basicsTW()
	if tw.ID != 0 || tw.ULID != "" {
		t.Errorf("ID, ULID = %d, %q, want zero", tw.ID, tw.ULID)
	}
}

func TestNewTrackWriteBasicsFileFacts(t *testing.T) {
	t.Parallel()
	tw, fi := basicsTW(), testInfo()
	if tw.Root != fi.Root || tw.RelPath != fi.RelPath || tw.Size != fi.Size || !tw.MTime.Equal(fi.MTime) || tw.Digest != fi.Digest {
		t.Errorf("file facts not copied: %+v", tw)
	}
}

func TestNewTrackWriteBasicsTechnical(t *testing.T) {
	t.Parallel()
	tw := basicsTW()
	if tw.Format != formatFLAC || tw.Codec != formatFLAC || tw.Duration != 3*time.Minute {
		t.Errorf("format/codec/duration = %q %q %v", tw.Format, tw.Codec, tw.Duration)
	}
	if tw.SampleRate != 44100 || tw.BitDepth != 16 || tw.Channels != 2 || tw.Bitrate != 900000 || !tw.Lossless {
		t.Errorf("technical fields wrong: %+v", tw)
	}
}

func TestNewTrackWriteBasicsReplayGain(t *testing.T) {
	t.Parallel()
	tw := basicsTW()
	want := store.ReplayGain{TrackGain: -6.5, TrackPeak: 0.9, AlbumGain: -7, AlbumPeak: 1.1}
	if tw.ReplayGain != want {
		t.Errorf("ReplayGain = %+v, want %+v", tw.ReplayGain, want)
	}
}

func TestNewTrackWriteBasicsTitle(t *testing.T) {
	t.Parallel()
	tw := basicsTW()
	if tw.Title != songName || tw.SortTitle != songName {
		t.Errorf("Title, SortTitle = %q, %q", tw.Title, tw.SortTitle)
	}
}

func TestNewTrackWriteBasicsNumbers(t *testing.T) {
	t.Parallel()
	tw := basicsTW()
	if tw.TrackNo != 3 || tw.TrackTotal != 12 || tw.DiscNo != 1 || tw.DiscTotal != 2 || tw.Year != 2001 || tw.OriginalYear != 1997 {
		t.Errorf("numbers wrong: %+v", tw)
	}
}

func TestNewTrackWriteBasicsWarnings(t *testing.T) {
	t.Parallel()
	tw := basicsTW()
	want := []string{"metadata: bad frame", basicsFile().Warnings[1].String()}
	if !slices.Equal(tw.Warnings, want) || want[0] != "metadata: bad frame" {
		t.Errorf("Warnings = %q, want %q", tw.Warnings, want)
	}
}

func TestNewTrackWriteWarningsAreValidUTF8(t *testing.T) {
	t.Parallel()
	f := basicsFile()
	f.Warnings = []audiometa.Warning{{Stage: audiometa.StageMetadata, Message: "bad path caf\xe9.mp3"}}
	got := NewTrackWrite(f, testInfo()).Warnings
	if want := []string{"metadata: bad path caf\uFFFD.mp3"}; !slices.Equal(got, want) {
		t.Errorf("Warnings = %q, want %q", got, want)
	}
}

func TestNewTrackWriteOriginalYear(t *testing.T) {
	t.Parallel()
	tests := []struct {
		date string
		want int
	}{
		{"1997-05-21", 1997},
		{"1997", 1997},
		{"97", 0},
		{"abcd", 0},
		{"0999", 0},
		{"-123", 0},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.date, func(t *testing.T) {
			t.Parallel()
			f := &audiometa.File{}
			f.Tags.OriginalDate = tt.date
			if got := NewTrackWrite(f, testInfo()).OriginalYear; got != tt.want {
				t.Errorf("OriginalYear(%q) = %d, want %d", tt.date, got, tt.want)
			}
		})
	}
}

func TestNewTrackWriteTitleFallback(t *testing.T) {
	t.Parallel()
	f := &audiometa.File{}
	fi := testInfo()
	fi.RelPath = "a/01 Intro.flac"
	tw := NewTrackWrite(f, fi)
	if tw.Title != "01 Intro" || tw.SortTitle != "01 Intro" {
		t.Errorf("Title, SortTitle = %q, %q, want 01 Intro", tw.Title, tw.SortTitle)
	}
	f.Tags.Title = "Real"
	f.Tags.AddRaw("TITLESORT", " Sorted ")
	tw = NewTrackWrite(f, fi)
	if tw.Title != "Real" || tw.SortTitle != "Sorted" {
		t.Errorf("Title, SortTitle = %q, %q, want Real, Sorted", tw.Title, tw.SortTitle)
	}
}

func TestNewTrackWriteArtists(t *testing.T) {
	t.Parallel()
	type raw struct {
		key  string
		vals []string
	}
	tests := []struct {
		name    string
		artist  string
		artists []string
		raws    []raw
		want    []store.ArtistWrite
	}{
		{"two", "", []string{"A", "B"}, nil, []store.ArtistWrite{
			{MatchKey: "a", Name: "A", SortName: "A"}, {MatchKey: "b", Name: "B", SortName: "B"},
		}},
		{"solo fallback", soloName, nil, nil, []store.ArtistWrite{{MatchKey: "solo", Name: soloName, SortName: soloName}}},
		{"dedupe and drop empty", "", []string{" A ", "a", ""}, nil, []store.ArtistWrite{{MatchKey: "a", Name: "A", SortName: "A"}}},
		{"feat is one", "", []string{featName}, nil, []store.ArtistWrite{{MatchKey: "a feat. b", Name: featName, SortName: featName}}},
		{"none", "", nil, nil, nil},
		{"sorts paired", "", []string{"A", "B"}, []raw{{keyArtistSort, []string{sortA, "B-sort"}}}, []store.ArtistWrite{
			{MatchKey: "a", Name: "A", SortName: sortA}, {MatchKey: "b", Name: "B", SortName: "B-sort"},
		}},
		{"sort count mismatch", "", []string{"A", "B"}, []raw{{keyArtistSort, []string{sortA}}}, []store.ArtistWrite{
			{MatchKey: "a", Name: "A", SortName: "A"}, {MatchKey: "b", Name: "B", SortName: "B"},
		}},
		{"one value, one name left after dropping", "", []string{" A ", "a", ""}, []raw{{keyArtistSort, []string{sortA}}}, []store.ArtistWrite{
			{MatchKey: "a", Name: "A", SortName: sortA},
		}},
		{"mbid for one name", "", []string{"A"}, []raw{{keyArtistMBID, []string{"m1"}}}, []store.ArtistWrite{
			{MatchKey: "a", Name: "A", SortName: "A", MBID: "m1"},
		}},
		{"mbids paired from id3 key", "", []string{"A", "B"}, []raw{{keyID3ArtistID, []string{"m1", "m2"}}}, []store.ArtistWrite{
			{MatchKey: "a", Name: "A", SortName: "A", MBID: "m1"}, {MatchKey: "b", Name: "B", SortName: "B", MBID: "m2"},
		}},
		{"mbid one of two names stays empty", "", []string{"A", "B"}, []raw{{keyArtistMBID, []string{"m1"}}}, []store.ArtistWrite{
			{MatchKey: "a", Name: "A", SortName: "A"}, {MatchKey: "b", Name: "B", SortName: "B"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := &audiometa.File{}
			f.Tags.Artist, f.Tags.Artists = tt.artist, tt.artists
			for _, r := range tt.raws {
				for _, v := range r.vals {
					f.Tags.AddRaw(r.key, v)
				}
			}
			if got := NewTrackWrite(f, testInfo()).Artists; !slices.Equal(got, tt.want) {
				t.Errorf("Artists = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNewTrackWriteArtistMBIDNotFromTypedField(t *testing.T) {
	t.Parallel()
	f := &audiometa.File{}
	f.Tags.Artists = []string{"A"}
	f.Tags.MusicBrainzArtistID = "typed"
	if got := NewTrackWrite(f, testInfo()).Artists[0].MBID; got != "" {
		t.Errorf("MBID = %q, want empty: artist MBIDs come only from raw keys", got)
	}
}

func TestNewTrackWriteAlbumArtists(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		typed string
		raw   []string
		sorts []string
		want  []store.ArtistWrite
	}{
		{"raw two", "", []string{"X", "Y"}, nil, []store.ArtistWrite{
			{MatchKey: "x", Name: "X", SortName: "X"}, {MatchKey: "y", Name: "Y", SortName: "Y"},
		}},
		{"typed only", "Z", nil, nil, []store.ArtistWrite{{MatchKey: "z", Name: "Z", SortName: "Z"}}},
		{"none", "", nil, nil, nil},
		{"raw wins over typed", "Z", []string{"X"}, []string{"X, The"}, []store.ArtistWrite{
			{MatchKey: "x", Name: "X", SortName: "X, The"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := &audiometa.File{}
			f.Tags.AlbumArtist = tt.typed
			for _, v := range tt.raw {
				f.Tags.AddRaw("ALBUMARTIST", v)
			}
			for _, v := range tt.sorts {
				f.Tags.AddRaw("ALBUMARTISTSORT", v)
			}
			if got := NewTrackWrite(f, testInfo()).AlbumArtists; !slices.Equal(got, tt.want) {
				t.Errorf("AlbumArtists = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNewTrackWriteAlbum(t *testing.T) {
	t.Parallel()
	f := &audiometa.File{}
	f.Tags.Album = " Abbey Road "
	f.Tags.Year = 1969
	f.Tags.AddRaw("ALBUMSORT", "Abbey Road, The")
	f.Tags.AddRaw("MUSICBRAINZ ALBUM ID", " MB1 ")
	f.Tags.AddRaw("COMPILATION", "TRUE")
	tw := NewTrackWrite(f, testInfo())
	want := store.AlbumWrite{GroupKey: groupKey(&tw), Name: "Abbey Road", SortName: "Abbey Road, The", MBID: "MB1", Year: 1969, Compilation: true}
	if tw.Album != want {
		t.Errorf("Album = %+v, want %+v", tw.Album, want)
	}
	if !tw.Compilation {
		t.Error("Compilation = false, want true")
	}

	g := &audiometa.File{}
	g.Tags.Album = "X"
	g.Tags.MusicBrainzAlbumID = "typed-id"
	if got := NewTrackWrite(g, testInfo()).Album; got.MBID != "typed-id" || got.SortName != "X" || got.Compilation {
		t.Errorf("Album = %+v, want typed MBID, sort name X, no compilation", got)
	}
}

func TestNewTrackWriteCompilationValues(t *testing.T) {
	t.Parallel()
	tests := map[string]bool{"1": true, "true": true, "True": true, "0": false, "": false, "yes": false}
	for v, want := range tests {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			f := &audiometa.File{}
			if v != "" {
				f.Tags.AddRaw("TCMP", v)
			}
			if got := NewTrackWrite(f, testInfo()).Compilation; got != want {
				t.Errorf("Compilation(%q) = %t, want %t", v, got, want)
			}
		})
	}
}

func TestNewTrackWriteRawTags(t *testing.T) {
	t.Parallel()
	f := &audiometa.File{}
	f.Tags.AddRaw("B", "2")
	f.Tags.AddRaw("A", "1")
	f.Tags.AddRaw("A", "1b")
	f.Tags.AddRaw("C", " x ")
	f.Tags.AddRaw("\xa9nam", "Latin one")
	f.Tags.AddRaw("©ART", "valid utf-8")
	want := []store.Tag{
		{Key: "A", Value: "1"},
		{Key: "A", Value: "1b"},
		{Key: "B", Value: "2"},
		{Key: "C", Value: " x "},
		// Sorted by the original bytes: the lone 0xa9 sorts before UTF-8 0xc2 0xa9.
		{Key: "©nam", Value: "Latin one"},
		{Key: "©ART", Value: "valid utf-8"},
	}
	got := NewTrackWrite(f, testInfo()).Tags
	if !slices.Equal(got, want) {
		t.Errorf("Tags = %+v, want %+v", got, want)
	}
}

func TestGroupKey(t *testing.T) {
	t.Parallel()
	type track struct {
		root, rel, album, mbid string
		albumArtists, artists  []string
		compilation            bool
	}
	const mbid = "abc"
	base := track{root: "/music", rel: "Artist/Album/a.flac", album: "Album", albumArtists: []string{"Artist"}}
	with := func(mut func(*track)) track {
		tr := base
		mut(&tr)
		return tr
	}
	tests := []struct {
		name string
		a, b track
		same bool
	}{
		{"same folder", base, with(func(t *track) { t.rel = "Artist/Album/b.flac" }), true},
		{"different folders", base, with(func(t *track) { t.rel = "Other/Album/b.flac" }), false},
		{"CD1 and CD2", with(func(t *track) { t.rel = "Album/CD1/x.flac" }), with(func(t *track) { t.rel = "Album/CD2/y.flac" }), true},
		{"Disc 1 and disk_02", with(func(t *track) { t.rel = "Album/Disc 1/x.flac" }), with(func(t *track) { t.rel = "Album/disk_02/y.flac" }), true},
		{"cd.10", with(func(t *track) { t.rel = "Album/cd.10/x.flac" }), with(func(t *track) { t.rel = albumY }), true},
		{"Bonus is not a disc folder", with(func(t *track) { t.rel = "Album/Bonus/x.flac" }), with(func(t *track) { t.rel = albumY }), false},
		{"Disc 100 is not a disc folder", with(func(t *track) { t.rel = "Album/Disc 100/x.flac" }), with(func(t *track) { t.rel = albumY }), false},
		{"CD 1 of 2 is not a disc folder", with(func(t *track) { t.rel = "Album/CD 1 of 2/x.flac" }), with(func(t *track) { t.rel = albumY }), false},
		{"root named CD1 is not collapsed", with(func(t *track) { t.root, t.rel = "/mnt/CD1", "x.flac" }), with(func(t *track) { t.root, t.rel = "/mnt", "y.flac" }), false},
		{"disc folder at root collapses to root", with(func(t *track) { t.rel = "CD1/x.flac" }), with(func(t *track) { t.rel = "y.flac" }), true},
		{"same MBID in different folders", with(func(t *track) { t.mbid, t.rel = mbid, "A/x.flac" }), with(func(t *track) { t.mbid, t.rel = mbid, "B/y.flac" }), false},
		{"a track without the MBID joins its folder's album", with(func(t *track) { t.mbid = mbid }), base, true},
		{"different MBIDs in one folder", with(func(t *track) { t.mbid = mbid }), with(func(t *track) { t.mbid = "abd" }), true},
		{
			"compilation without album artist",
			with(func(t *track) { t.albumArtists, t.artists, t.compilation = nil, []string{"P"}, true }),
			with(func(t *track) { t.albumArtists, t.artists, t.compilation = nil, []string{"Q"}, true }),
			true,
		},
		{
			"no album artist, not a compilation",
			with(func(t *track) { t.albumArtists, t.artists = nil, []string{"P"} }),
			with(func(t *track) { t.albumArtists, t.artists = nil, []string{"Q"} }),
			false,
		},
		{"album name case", base, with(func(t *track) { t.album = "ALBUM" }), true},
		{"different albums", base, with(func(t *track) { t.album = "Other" }), false},
		{"different album artists", base, with(func(t *track) { t.albumArtists = []string{"Someone"} }), false},
	}
	key := func(tr track) string {
		f := &audiometa.File{}
		f.Tags.Album = tr.album
		f.Tags.Artists = tr.artists
		for _, a := range tr.albumArtists {
			f.Tags.AddRaw("ALBUMARTIST", a)
		}
		if tr.mbid != "" {
			f.Tags.AddRaw("MUSICBRAINZ ALBUM ID", tr.mbid)
		}
		if tr.compilation {
			f.Tags.AddRaw("COMPILATION", "1")
		}
		tw := NewTrackWrite(f, &FileInfo{Root: tr.root, RelPath: tr.rel})
		return groupKey(&tw)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ka, kb := key(tt.a), key(tt.b)
			if ka == "" || kb == "" {
				t.Fatalf("empty group key: %q, %q", ka, kb)
			}
			if (ka == kb) != tt.same {
				t.Errorf("keys %q and %q: same = %t, want %t", ka, kb, ka == kb, tt.same)
			}
		})
	}
}

func BenchmarkNewTrackWrite(b *testing.B) {
	f := &audiometa.File{}
	f.Format = audiometa.FormatFLAC
	f.Tags.Title, f.Tags.Album = songName, "Album"
	f.Tags.Artists = []string{"Artist One", "Artist Two"}
	f.Tags.Year = 2001
	f.Tags.AddRaw("ALBUMARTIST", "Album Artist")
	f.Tags.AddRaw(keyArtistSort, "One, Artist")
	f.Tags.AddRaw(keyArtistSort, "Two, Artist")
	for i := range 22 {
		f.Tags.AddRaw("KEY"+string(rune('A'+i)), "value")
	}
	fi := testInfo()
	b.ReportAllocs()
	for b.Loop() {
		_ = NewTrackWrite(f, fi)
	}
}
