package store_test

import (
	"cmp"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

// browseClock is a settable clock for the store, so albums get chosen added times.
type browseClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *browseClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *browseClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

const titleD1T2 = "d1t2"

func openBrowse(t *testing.T) (*store.Store, *browseClock) {
	t.Helper()
	clock := &browseClock{now: time.UnixMilli(1_762_000_000_000).UTC()}
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "kitsune.db"), store.Options{Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, clock
}

// albumID is the public ID the helpers give album n.
func albumID(n int) string { return fmt.Sprintf("02J%023d", n) }

// albumTrack returns track number t of album n, in album n's folder.
func albumTrack(n, t int, sortName string) store.TrackWrite {
	w := newWrite(n*1000 + t)
	w.RelPath = fmt.Sprintf("album%d/%d.flac", n, t)
	w.Album = store.AlbumWrite{
		GroupKey: fmt.Sprintf("album-%d", n),
		ULID:     albumID(n),
		Name:     fmt.Sprintf("Album %d", n),
		SortName: sortName,
		Artist:   fmt.Sprintf("Artist %d", n),
	}
	w.Digest = new(sha256.Sum256(fmt.Appendf(nil, "%d/%d", n, t)))
	return w
}

// addAlbums writes one single-track album per number in ns, in order, with the given sort names.
func addAlbums(t *testing.T, s *store.Store, sortNames map[int]string, ns ...int) {
	t.Helper()
	tws := make([]store.TrackWrite, len(ns))
	for i, n := range ns {
		tws[i] = albumTrack(n, 1, sortNames[n])
	}
	writeAll(t, s, tws...)
}

// pages walks every page of the list and returns the album IDs in order.
// walked is what [pages] saw: the album IDs in order, and each page's size.
type walked struct {
	ids   []string
	sizes []int
}

func pages(t *testing.T, s *store.Store, sort store.AlbumSort, size int, cursor string) walked {
	t.Helper()
	var w walked
	for range 100 {
		p, err := s.Albums(t.Context(), sort, size, cursor)
		if err != nil {
			t.Fatalf("Albums(%s, %d, %q): %v", sort, size, cursor, err)
		}
		for i := range p.Albums {
			w.ids = append(w.ids, p.Albums[i].ID)
		}
		w.sizes = append(w.sizes, len(p.Albums))
		if p.Next == "" {
			return w
		}
		cursor = p.Next
	}
	t.Fatal("paging does not end")
	return walked{}
}

func ids(ns ...int) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = albumID(n)
	}
	return out
}

func TestAlbumsPageByNameWithTies(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	// 2, 4 and 6 tie ignoring case; ties break by insertion order (the internal ID).
	names := map[int]string{1: "bravo", 2: "alpha", 3: "Bravo", 4: "ALPHA", 5: "charlie", 6: "Alpha"}
	addAlbums(t, s, names, 1, 2, 3, 4, 5, 6)
	want := ids(2, 4, 6, 1, 3, 5)

	for _, size := range []int{1, 2, 3, 4, 6, 7} {
		w := pages(t, s, store.SortName, size, "")
		got, sizes := w.ids, w.sizes
		if !slices.Equal(got, want) {
			t.Errorf("size %d: ids = %v, want %v", size, got, want)
		}
		for i, n := range sizes {
			if n == 0 || n > size {
				t.Errorf("size %d: page %d has %d albums", size, i, n)
			}
		}
	}
	if sizes := pages(t, s, store.SortName, 3, "").sizes; len(sizes) != 2 {
		t.Errorf("6 albums at size 3: %d pages, want exactly 2 (no empty last page)", len(sizes))
	}
}

func TestAlbumsPageByAddedWithTies(t *testing.T) {
	t.Parallel()
	s, clock := openBrowse(t)
	addAlbums(t, s, nil, 1, 2, 3) // One batch: the same added time.
	clock.Advance(time.Minute)
	addAlbums(t, s, nil, 4, 5)
	clock.Advance(time.Minute)
	addAlbums(t, s, nil, 6, 7, 8)
	want := ids(8, 7, 6, 5, 4, 3, 2, 1) // Newest first; ties by descending internal ID.

	for _, size := range []int{1, 2, 3, 5, 8, 9} {
		if got := pages(t, s, store.SortAdded, size, "").ids; !slices.Equal(got, want) {
			t.Errorf("size %d: ids = %v, want %v", size, got, want)
		}
	}
}

func TestAlbumsCursorStableUnderInserts(t *testing.T) {
	t.Parallel()
	t.Run("name", func(t *testing.T) {
		t.Parallel()
		s, _ := openBrowse(t)
		names := map[int]string{1: "b", 2: "c", 3: "d", 4: "e", 5: "f", 6: "a", 7: "z"}
		addAlbums(t, s, names, 1, 2, 3, 4, 5)
		first, err := s.Albums(t.Context(), store.SortName, 2, "")
		if err != nil {
			t.Fatal(err)
		}
		addAlbums(t, s, names, 6, 7) // 6 sorts before the cursor, 7 after everything.
		rest := pages(t, s, store.SortName, 2, first.Next).ids
		if want := ids(3, 4, 5, 7); !slices.Equal(rest, want) {
			t.Errorf("remaining = %v, want %v", rest, want)
		}
	})
	t.Run("added", func(t *testing.T) {
		t.Parallel()
		s, clock := openBrowse(t)
		for n := 1; n <= 5; n++ {
			addAlbums(t, s, nil, n)
			clock.Advance(time.Minute)
		}
		first, err := s.Albums(t.Context(), store.SortAdded, 2, "")
		if err != nil {
			t.Fatal(err)
		}
		addAlbums(t, s, nil, 6) // Newer than everything: before the cursor.
		rest := pages(t, s, store.SortAdded, 2, first.Next).ids
		if want := ids(3, 2, 1); !slices.Equal(rest, want) {
			t.Errorf("remaining = %v, want %v", rest, want)
		}
	})
}

// firstPage returns the first one-album page of a sort that has a next page.
func firstPage(t *testing.T, s *store.Store, sort store.AlbumSort) store.AlbumPage {
	t.Helper()
	p, err := s.Albums(t.Context(), sort, 1, "")
	if err != nil || p.Next == "" {
		t.Fatalf("%s: %+v, %v; want a next cursor", sort, p, err)
	}
	return p
}

func TestAlbumsRejectBadInput(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	addAlbums(t, s, map[int]string{1: "a", 2: "b"}, 1, 2)
	byName, byAdded := firstPage(t, s, store.SortName), firstPage(t, s, store.SortAdded)

	cursors := []struct {
		name   string
		sort   store.AlbumSort
		cursor string
	}{
		{"not base64", store.SortName, "!!!"},
		{"padded", store.SortName, byName.Next + "="},
		{"empty payload", store.SortName, "AA"},
		{"unknown kind", store.SortName, "Zm9vYmFyYmF6cXV4"},
		{"truncated", store.SortAdded, byAdded.Next[:4]},
		{"name cursor on the added sort", store.SortAdded, byName.Next},
		{"added cursor on the name sort", store.SortName, byAdded.Next},
	}
	for _, tc := range cursors {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, got := s.Albums(t.Context(), tc.sort, 1, tc.cursor); !errors.Is(got, store.ErrBadCursor) {
				t.Errorf("err = %v, want ErrBadCursor", got)
			}
		})
	}
	for _, limit := range []int{0, -1, store.MaxAlbumLimit + 1} {
		if _, err := s.Albums(t.Context(), store.SortName, limit, ""); !errors.Is(err, store.ErrBadLimit) {
			t.Errorf("limit %d: err = %v, want ErrBadLimit", limit, err)
		}
	}
	if _, err := s.Albums(t.Context(), "bogus", 1, ""); !errors.Is(err, store.ErrBadSort) {
		t.Errorf("an unknown sort: err = %v, want ErrBadSort", err)
	}
}

func TestAlbumsPageAfterNonUTF8Name(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	names := map[int]string{1: "a", 2: "b\xff\xfe", 3: "c"}
	addAlbums(t, s, names, 1, 2, 3)
	got := pages(t, s, store.SortName, 1, "").ids
	if want := ids(1, 2, 3); !slices.Equal(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
}

func TestAlbumSummaryFields(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	w := albumTrack(1, 1, "sort")
	w.Album.Year = 1999
	w.Album.Compilation = true
	w.Album.Artwork = &store.ArtworkWrite{
		Hash: sha256.Sum256([]byte("cover")), Width: 640, Height: 480,
		Accent: "#aabbcc", Ground: "#112233", Source: store.ArtworkEmbedded,
	}
	w2 := albumTrack(1, 2, "sort")
	w2.Album.Year, w2.Album.Compilation = w.Album.Year, w.Album.Compilation // The last write wins.
	bare := albumTrack(2, 1, "tort")
	bare.Album.Name = "Unknown Album"
	writeAll(t, s, w, w2, bare)

	p, err := s.Albums(t.Context(), store.SortName, 10, "")
	if err != nil || len(p.Albums) != 2 {
		t.Fatalf("Albums = %+v, %v; want 2", p, err)
	}
	want := store.AlbumSummary{
		ID: albumID(1), Title: "Album 1", SortName: "sort", Artist: "Artist 1", SortArtist: "Artist 1",
		Year: 1999, TrackCount: 2, Compilation: true,
		Cover:   &store.Cover{Hash: w.Album.Artwork.Hash, Accent: "#aabbcc", Ground: "#112233", Width: 640, Height: 480},
		AddedAt: time.UnixMilli(1_762_000_000_000).UTC(),
	}
	got := p.Albums[0]
	if got.Cover == nil || *got.Cover != *want.Cover {
		t.Fatalf("Cover = %+v, want %+v", got.Cover, want.Cover)
	}
	got.Cover, want.Cover = nil, nil
	if got != want {
		t.Errorf("first = %+v, want %+v", got, want)
	}
	if bare := p.Albums[1]; bare.Cover != nil || bare.Year != 0 || bare.Compilation || bare.TrackCount != 1 {
		t.Errorf("bare album = %+v, want no cover or year, one track", bare)
	}
}

// markMissing marks the tracks at rels missing, as of now.
func markMissing(t *testing.T, s *store.Store, clock *browseClock, rels ...string) {
	t.Helper()
	states := fileStates(t, s)
	tids := make([]int64, len(rels))
	for i, r := range rels {
		tids[i] = states[r].TrackID
	}
	if err := s.MarkMissing(t.Context(), tids, clock.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestAlbumsLiveOnly(t *testing.T) {
	t.Parallel()
	s, clock := openBrowse(t)
	writeAll(t, s, albumTrack(1, 1, "a"), albumTrack(1, 2, "a"), albumTrack(2, 1, "b"), albumTrack(3, 1, "c"))

	markMissing(t, s, clock, "album1/2.flac")
	d, err := s.Album(t.Context(), albumID(1))
	if err != nil || d.TrackCount != 1 || len(d.Tracks) != 1 {
		t.Fatalf("Album 1 = %d tracks (%d listed), %v; want 1 live track", d.TrackCount, len(d.Tracks), err)
	}
	p, err := s.Albums(t.Context(), store.SortName, 10, "")
	if err != nil || len(p.Albums) != 3 || p.Albums[0].TrackCount != 1 {
		t.Fatalf("Albums = %+v, %v; want 3, the first with 1 track", p.Albums, err)
	}
}

func TestAlbumsWithoutLiveTracks(t *testing.T) {
	t.Parallel()
	s, clock := openBrowse(t)
	writeAll(t, s, albumTrack(1, 1, "a"), albumTrack(1, 2, "a"), albumTrack(2, 1, "b"), albumTrack(3, 1, "c"))
	markMissing(t, s, clock, "album1/1.flac", "album1/2.flac", "album3/1.flac")
	for _, sort := range []store.AlbumSort{store.SortName, store.SortAdded} {
		got := pages(t, s, sort, 10, "").ids
		if want := ids(2); !slices.Equal(got, want) {
			t.Errorf("%s: ids = %v, want %v", sort, got, want)
		}
	}
	for _, n := range []int{1, 3, 9} {
		if _, err := s.Album(t.Context(), albumID(n)); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Album(%d) err = %v, want ErrNotFound", n, err)
		}
	}
	f, err := s.Facts(t.Context())
	if err != nil || f.Albums != 1 || f.Tracks != 1 {
		t.Errorf("Facts = %+v, %v; want 1 album and 1 track", f, err)
	}
}

func TestTrackFile(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	w := albumTrack(1, 1, "a")
	w.Format = "Ogg Vorbis"
	writeAll(t, s, w)

	got, err := s.TrackFile(t.Context(), w.ULID)
	if err != nil {
		t.Fatal(err)
	}
	want := store.TrackFile{Root: w.Root, RelPath: w.RelPath, Format: w.Format}
	if got != want {
		t.Errorf("TrackFile = %+v, want %+v", got, want)
	}
}

func TestTrackFileNotFound(t *testing.T) {
	t.Parallel()
	s, clock := openBrowse(t)
	live, gone := albumTrack(1, 1, "a"), albumTrack(1, 2, "a")
	writeAll(t, s, live, gone)
	markMissing(t, s, clock, gone.RelPath)

	for _, id := range []string{gone.ULID, "01J00000000000000000009999", "not-a-ulid", ""} {
		if _, err := s.TrackFile(t.Context(), id); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("TrackFile(%q) err = %v, want ErrNotFound", id, err)
		}
	}
	if _, err := s.TrackFile(t.Context(), live.ULID); err != nil {
		t.Errorf("the live track: %v", err)
	}
}

func TestTrackFileQueryFails(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TrackFile(t.Context(), albumID(1)); err == nil || errors.Is(err, store.ErrNotFound) {
		t.Errorf("TrackFile on a closed store err = %v, want a query error", err)
	}
}

// trackSpec is one track of album 1 in TestAlbumTracks.
type trackSpec struct {
	title        string
	disc, number int
	format       string
	artists      []string
}

// albumTracksFixture returns album 1's tracks, out of order, with a missing one last.
func albumTracksFixture() []store.TrackWrite {
	specs := []trackSpec{
		{"z-no-number", 0, 0, flacFormat, nil},
		{"d2t1", 2, 1, "mp3", []string{"X"}},
		{titleD1T2, 0, 2, flacFormat, []string{"X", "Y", "Z"}}, // An unset disc counts as disc 1.
		{"a-no-number", 0, 0, flacFormat, nil},
		{"d1t1", 1, 1, flacFormat, nil},
	}
	tws := make([]store.TrackWrite, 0, len(specs)+1)
	for i, sp := range specs {
		w := albumTrack(1, i+1, "s")
		w.Title, w.DiscNo, w.TrackNo, w.Format = sp.title, sp.disc, sp.number, sp.format
		w.Duration = time.Duration(i+1) * time.Second
		w.Artists = nil
		for _, a := range sp.artists {
			w.Artists = append(w.Artists, store.ArtistWrite{ULID: artistULID(strings.ToLower(a)), MatchKey: strings.ToLower(a), Name: a, SortName: a})
		}
		tws = append(tws, w)
	}
	gone := albumTrack(1, 9, "s")
	gone.Title, gone.Duration, gone.Format = "gone", time.Hour, "ogg"
	tws = append(tws, gone)
	tws[2].SampleRate, tws[2].BitDepth = 44100, 16
	tws[2].ReplayGain = store.ReplayGain{TrackGain: -6.5, TrackPeak: 0.9, AlbumGain: -7, AlbumPeak: 1}
	return tws
}

func TestAlbumTrackOrder(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	writeAll(t, s, albumTracksFixture()...)
	d, err := s.Album(t.Context(), albumID(1))
	if err != nil {
		t.Fatal(err)
	}
	titles := make([]string, len(d.Tracks))
	for i := range d.Tracks {
		titles[i] = d.Tracks[i].Title
	}
	// The unset disc counts as disc 1; unset track numbers sort last within a disc.
	if want := []string{"d1t1", titleD1T2, "a-no-number", "gone", "z-no-number", "d2t1"}; !slices.Equal(titles, want) {
		t.Errorf("track order = %v, want %v", titles, want)
	}
}

func TestAlbumTracks(t *testing.T) {
	t.Parallel()
	s, clock := openBrowse(t)
	tws := albumTracksFixture()
	writeAll(t, s, tws...)
	markMissing(t, s, clock, "album1/9.flac")

	d, err := s.Album(t.Context(), albumID(1))
	if err != nil {
		t.Fatal(err)
	}
	if d.Duration != 15*time.Second || d.TrackCount != 5 {
		t.Errorf("Duration, TrackCount = %v, %d; want 15s, 5 (the missing track is out)", d.Duration, d.TrackCount)
	}
	if want := []string{flacFormat, "mp3"}; !slices.Equal(d.Formats, want) {
		t.Errorf("Formats = %v, want %v", d.Formats, want)
	}
	want := store.Track{
		ID: tws[2].ULID, Title: titleD1T2, Artists: []string{"X", "Y", "Z"}, DiscNo: 0, TrackNo: 2, ETag: d.Tracks[1].ETag,
		Duration: 3 * time.Second, Format: flacFormat, SampleRate: 44100, BitDepth: 16,
		ReplayGain: store.ReplayGain{TrackGain: -6.5, TrackPeak: 0.9, AlbumGain: -7, AlbumPeak: 1},
	}
	if !reflect.DeepEqual(d.Tracks[1], want) {
		t.Errorf("track = %+v, want %+v", d.Tracks[1], want)
	}
	if first := d.Tracks[0]; first.DiscNo != 1 || first.TrackNo != 1 || len(first.Artists) != 0 {
		t.Errorf("first = %+v, want disc 1 track 1 and no artists", first)
	}
}

// detail returns the album with the public ID id.
func detail(t *testing.T, s *store.Store, id string) store.AlbumDetail {
	t.Helper()
	d, err := s.Album(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestEditLookupsCarryTheETagsOfReads(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	writeAll(t, s, albumTracksFixture()...)
	d := detail(t, s, albumID(1))
	files, etag, err := s.AlbumEditFiles(t.Context(), albumID(1))
	if err != nil {
		t.Fatal(err)
	}
	if etag != d.ETag || len(files) != len(d.Tracks) {
		t.Errorf("%d files with ETag %s, want %d with %s", len(files), etag, len(d.Tracks), d.ETag)
	}
}

func TestAlbumETagCoversLiveTracksOnly(t *testing.T) {
	t.Parallel()
	s, clock := openBrowse(t)
	writeAll(t, s, albumTracksFixture()...)
	before := detail(t, s, albumID(1))
	markMissing(t, s, clock, "album1/9.flac")
	after := detail(t, s, albumID(1))
	if after.ETag == before.ETag {
		t.Error("the album's ETag covers a missing track")
	}
	if after.Tracks[1].ETag != before.Tracks[1].ETag {
		t.Error("a neighbor's ETag changed")
	}
}

func TestEditLookupsNotFound(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	if _, _, err := s.AlbumEditFiles(t.Context(), "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("AlbumEditFiles = %v", err)
	}
	if _, err := s.TrackAlbum(t.Context(), "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("TrackAlbum = %v", err)
	}
}

const (
	genreKey  = "GENRE"
	id3Genre  = "TCON"
	genreRock = "Rock"
	genrePop  = "Pop"
)

func TestAlbumTagValues(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	tws := albumTracksFixture()
	tws[0].Tags = []store.Tag{{Key: genreKey, Value: genreRock}, {Key: genreKey, Value: genrePop}, {Key: "MOOD", Value: "Calm"}}
	writeAll(t, s, tws...)
	got, err := s.AlbumTagValues(t.Context(), albumID(1), []string{genreKey, id3Genre})
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string][]string{tws[0].ULID: {genreRock, genrePop}}; !reflect.DeepEqual(got, want) {
		t.Errorf("AlbumTagValues = %v, want %v", got, want)
	}
	album, err := s.TrackAlbum(t.Context(), tws[0].ULID)
	if err != nil || album != albumID(1) {
		t.Errorf("TrackAlbum = %q, %v", album, err)
	}
}

// seedTroubled writes three issues and three tracks: a clean one, one with
// warnings and a missing one with warnings. It returns the second.
func seedTroubled(t *testing.T, s *store.Store) store.TrackWrite {
	t.Helper()
	clean, warned, gone := albumTrack(1, 1, "a"), albumTrack(1, 2, "a"), albumTrack(1, 3, "a")
	warned.Warnings = []string{"odd", "odder"}
	gone.Warnings = []string{"oddest"}
	issues := []store.ScanIssue{
		{Root: testRoot, RelPath: "z.wav", Kind: store.IssueUnsupported, Message: "z"},
		{Root: testRoot, RelPath: "m.flac", Kind: store.IssueUnreadable, Message: "a", Size: 1},
		{Root: testRoot, RelPath: "n.wav", Kind: store.IssueUnsupported, Message: "b"},
	}
	if err := s.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{clean, warned, gone}, Issues: issues}); err != nil {
		t.Fatal(err)
	}
	states, err := s.FileStates(t.Context(), testRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MarkMissing(t.Context(), []int64{states[gone.RelPath].TrackID}, time.Now()); err != nil {
		t.Fatal(err)
	}
	return warned
}

func TestFactsCountTrouble(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	seedTroubled(t, s)
	f, err := s.Facts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if f.Issues != 3 || f.Unreadable != 1 || f.Corrupted != 0 || f.Unsupported != 2 || f.WarnedTracks != 1 {
		t.Errorf("facts = %+v, want 3 issues, 1 unreadable, 2 unsupported, and 1 live track warned", f)
	}
}

func TestScanIssues(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	seedTroubled(t, s)
	listed, err := s.ScanIssues(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, len(listed))
	for i := range listed {
		paths[i] = listed[i].RelPath
	}
	if want := []string{"m.flac", "n.wav", "z.wav"}; !slices.Equal(paths, want) {
		t.Errorf("issues = %v, want %v", paths, want)
	}
}

func TestWarnedTracks(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	warned := seedTroubled(t, s)
	tracks, err := s.WarnedTracks(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []store.WarnedTrack{{ID: warned.ULID, Root: testRoot, RelPath: warned.RelPath, Warnings: warned.Warnings}}
	if !reflect.DeepEqual(tracks, want) {
		t.Errorf("WarnedTracks = %+v, want %+v", tracks, want)
	}
}

func TestAlbumTagCounts(t *testing.T) {
	t.Parallel()
	s, clock := openBrowse(t)
	one, two, other := albumTrack(1, 1, "a"), albumTrack(1, 2, "a"), albumTrack(2, 1, "b")
	gone := albumTrack(3, 1, "c")
	rock := store.Tag{Key: genreKey, Value: genreRock}
	one.Tags = []store.Tag{rock, {Key: "MOOD", Value: "Calm"}}
	two.Tags = []store.Tag{rock, {Key: genreKey, Value: genrePop}}
	other.Tags = []store.Tag{{Key: id3Genre, Value: "Jazz"}}
	gone.Tags = []store.Tag{rock}
	writeAll(t, s, one, two, other, gone)
	markMissing(t, s, clock, gone.RelPath)
	got, err := s.AlbumTagCounts(t.Context(), []string{genreKey, id3Genre})
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(got, func(a, b store.AlbumTagCount) int {
		return cmp.Or(cmp.Compare(a.AlbumID, b.AlbumID), cmp.Compare(a.Value, b.Value))
	})
	want := []store.AlbumTagCount{
		{AlbumID: albumID(1), Value: genrePop, Tracks: 1},
		{AlbumID: albumID(1), Value: genreRock, Tracks: 2},
		{AlbumID: albumID(2), Value: "Jazz", Tracks: 1},
	}
	if !slices.Equal(got, want) {
		t.Errorf("AlbumTagCounts = %+v, want %+v", got, want)
	}
}

func TestFacts(t *testing.T) {
	t.Parallel()
	s, _ := openBrowse(t)
	if f, err := s.Facts(t.Context()); err != nil || f != (store.LibraryFacts{}) {
		t.Fatalf("empty Facts = %+v, %v; want zero", f, err)
	}
	a, b, c := albumTrack(1, 1, "a"), albumTrack(1, 2, "a"), albumTrack(2, 1, "b")
	a.Duration, b.Duration, c.Duration = time.Second, 2*time.Second, 4*time.Second
	issue := store.ScanIssue{Root: testRoot, RelPath: "bad.wav", Kind: store.IssueCorrupted, Message: "x"}
	if err := s.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{a, b, c}, Issues: []store.ScanIssue{issue}}); err != nil {
		t.Fatal(err)
	}
	want := store.LibraryFacts{Albums: 2, Tracks: 3, Duration: 7 * time.Second, Issues: 1, Corrupted: 1}
	if f, err := s.Facts(t.Context()); err != nil || f != want {
		t.Errorf("Facts = %+v, %v; want %+v", f, err, want)
	}
}
