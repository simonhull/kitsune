package store_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	flacFormat = "flac"
	testRoot   = "/music"
	badPath    = "bad.flac"
)

// artistULID returns a 26-character ID for an artist's match key.
func artistULID(key string) string { return fmt.Sprintf("%026s", key) }

// newWrite returns a distinct valid write: its own album and artist per index.
func newWrite(i int) store.TrackWrite {
	return store.TrackWrite{
		ULID:      fmt.Sprintf("01J%023d", i),
		Root:      testRoot,
		RelPath:   fmt.Sprintf("a/%d.flac", i),
		Size:      1000 + int64(i),
		MTime:     time.UnixMilli(1_761_000_000_000 + int64(i)).UTC().Add(456789 * time.Nanosecond),
		Digest:    new(sha256.Sum256([]byte(strconv.Itoa(i)))),
		Format:    flacFormat,
		Codec:     flacFormat,
		Duration:  200*time.Second + 123*time.Millisecond,
		Title:     fmt.Sprintf("Title %d", i),
		SortTitle: fmt.Sprintf("Title %d", i),
		Album: store.AlbumWrite{
			GroupKey: fmt.Sprintf("album-%d", i),
			ULID:     fmt.Sprintf("02J%023d", i),
			Artist:   fmt.Sprintf("Artist %d", i),
			Name:     fmt.Sprintf("Album %d", i),
			SortName: fmt.Sprintf("Album %d", i),
		},
		Artists: []store.ArtistWrite{{
			ULID:     artistULID(fmt.Sprintf("artist-%d", i)),
			MatchKey: fmt.Sprintf("artist-%d", i),
			Name:     fmt.Sprintf("Artist %d", i),
			SortName: fmt.Sprintf("Artist %d", i),
		}},
	}
}

func writeAll(t *testing.T, s *store.Store, tws ...store.TrackWrite) {
	t.Helper()
	if err := s.WriteBatch(t.Context(), &store.Batch{Tracks: tws}); err != nil {
		t.Fatal(err)
	}
}

// A scan writes an album's tracks in any order; its year is still its first
// track's, as an edit leaves it.
func TestWriteBatchGivesAnAlbumItsFirstTracksYear(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	first, last := newWrite(1), newWrite(2)
	last.Album = first.Album // the same album, as each track's tags describe it
	first.TrackNo, first.Year, first.Album.Year = 1, 1999, 1999
	last.TrackNo, last.Year, last.Album.Year = 2, 2011, 2011
	writeAll(t, s, last, first) // the first track written last
	last.ID = 1                 // a fresh database numbers tracks in insert order
	writeAll(t, s, last)        // and a later batch that rereads only track 2
	album, err := s.Album(t.Context(), first.Album.ULID)
	if err != nil {
		t.Fatal(err)
	}
	if album.Year != 1999 {
		t.Errorf("year = %d, want 1999, the first track's", album.Year)
	}
}

// An album is a compilation when its first track says so, whichever track a
// scan wrote last.
func TestWriteBatchGivesAnAlbumItsFirstTracksCompilation(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	first, last := newWrite(1), newWrite(2)
	last.Album = first.Album // the same album, as each track's tags describe it
	first.TrackNo, first.Compilation, first.Album.Compilation = 1, false, false
	last.TrackNo, last.Compilation, last.Album.Compilation = 2, true, true
	writeAll(t, s, last, first)
	last.ID = 1 // a fresh database numbers tracks in insert order
	writeAll(t, s, last)
	album, err := s.Album(t.Context(), first.Album.ULID)
	if err != nil {
		t.Fatal(err)
	}
	if album.Compilation {
		t.Error("compilation = true, want false, the first track's")
	}
}

// mixedDiscs returns one album's tracks, in the order the Library lists them: a
// disc 1, a disc 2, and a song with no disc number, which comes after both. It
// is numbered 1 and its path sorts first, so a rule that counts no disc as disc 1
// would put it first.
func mixedDiscs() []store.TrackWrite {
	tws := []store.TrackWrite{newWrite(1), newWrite(2), newWrite(3)}
	places := []struct{ disc, track, year int }{{1, 2, 1999}, {2, 1, 2005}, {0, 1, 2011}}
	paths := []string{"a/2.flac", "a/3.flac", "a/1.flac"}
	for i := range tws {
		tws[i].Album = tws[0].Album
		tws[i].DiscNo, tws[i].TrackNo = places[i].disc, places[i].track
		tws[i].Year, tws[i].Album.Year = places[i].year, places[i].year
		tws[i].Compilation, tws[i].Album.Compilation = i > 0, i > 0
		tws[i].RelPath = paths[i]
	}
	return tws
}

// writeMixedDiscs writes mixedDiscs, the first written last, and returns them
// with the album as the store lists it.
func writeMixedDiscs(t *testing.T, s *store.Store) ([]store.TrackWrite, store.AlbumDetail) {
	t.Helper()
	tws := mixedDiscs()
	writeAll(t, s, tws[2], tws[1], tws[0])
	album, err := s.Album(t.Context(), tws[0].Album.ULID)
	if err != nil {
		t.Fatal(err)
	}
	return tws, album
}

// writeTitles returns the titles of tws, in order.
func writeTitles(tws []store.TrackWrite) []string {
	out := make([]string, len(tws))
	for i := range tws {
		out[i] = tws[i].Title
	}
	return out
}

// The album lists its songs as [store.Place] orders them, untagged after every disc.
func TestPlaceIsTheOrderAnAlbumLists(t *testing.T) {
	t.Parallel()
	tws, album := writeMixedDiscs(t, openUsers(t))
	listed := make([]string, len(album.Tracks))
	for i := range album.Tracks {
		listed[i] = album.Tracks[i].Title
	}
	if want := writeTitles(tws); !slices.Equal(listed, want) {
		t.Fatalf("the album lists %q, want %q", listed, want)
	}
	byPlace := slices.Clone(tws)
	slices.SortFunc(byPlace, func(a, b store.TrackWrite) int { return store.PlaceOf(&a).Compare(store.PlaceOf(&b)) })
	if placed := writeTitles(byPlace); !slices.Equal(placed, listed) {
		t.Errorf("Place orders %q, the album lists %q", placed, listed)
	}
}

// An album's year and compilation flag are its first song's as the Library
// shows it: songs with no disc number come after every disc there, so here too.
func TestAnAlbumsYearIsTheFirstListedSongs(t *testing.T) {
	t.Parallel()
	_, album := writeMixedDiscs(t, openUsers(t))
	if album.Year != 1999 {
		t.Errorf("year = %d, want 1999, disc 1's", album.Year)
	}
	if album.Compilation {
		t.Error("compilation = true, want false, disc 1's")
	}
}

// An edit looks for an album's cover in its files in the order the album lists them.
func TestAlbumCoverFilesGoInTheAlbumsOrder(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	tws, _ := writeMixedDiscs(t, s)
	row, err := s.AlbumRow(t.Context(), tws[0].Album.ULID)
	if err != nil {
		t.Fatal(err)
	}
	files, err := s.AlbumCoverFiles(t.Context(), row)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(files))
	for i := range files {
		got[i] = files[i].RelPath
	}
	want := make([]string, len(tws))
	for i := range tws {
		want[i] = tws[i].RelPath
	}
	if !slices.Equal(got, want) {
		t.Errorf("cover files %q, want %q, the album's order", got, want)
	}
}

func fileStates(t *testing.T, s *store.Store) map[string]store.FileState {
	t.Helper()
	got, err := s.FileStates(t.Context(), testRoot)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestWriteBatchMove(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	w := newWrite(0)
	writeAll(t, s, w)
	id := fileStates(t, s)[w.RelPath].TrackID

	moved := newWrite(0)
	moved.ID = id
	moved.RelPath = "b.flac"
	writeAll(t, s, moved)

	got := fileStates(t, s)
	if _, ok := got["a/0.flac"]; ok {
		t.Error("old path still known after the move")
	}
	if got["b.flac"].TrackID != id {
		t.Errorf("b.flac TrackID = %d, want %d", got["b.flac"].TrackID, id)
	}
}

func TestWriteBatchMoveOntoTakenPathFails(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	a, b := newWrite(0), newWrite(1)
	writeAll(t, s, a, b)
	before := fileStates(t, s)

	moved := newWrite(0)
	moved.ID = before[a.RelPath].TrackID
	moved.RelPath = b.RelPath
	moved.Title = "changed"
	if err := s.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{moved}}); err == nil {
		t.Fatal("moving onto a taken path succeeded")
	}
	after := fileStates(t, s)
	if len(after) != 2 || after[a.RelPath] != before[a.RelPath] || after[b.RelPath] != before[b.RelPath] {
		t.Errorf("states changed after a failed move: %+v, was %+v", after, before)
	}
}

func TestWriteBatchUpdateUnknownID(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	w := newWrite(0)
	w.ID = 999
	err := s.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{w}})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if got := fileStates(t, s); len(got) != 0 {
		t.Errorf("%d paths known after a failed write", len(got))
	}
}

func writeIssue(t *testing.T, s *store.Store, is *store.ScanIssue) {
	t.Helper()
	if err := s.WriteBatch(t.Context(), &store.Batch{Issues: []store.ScanIssue{*is}}); err != nil {
		t.Fatal(err)
	}
}

func TestWriteBatchIssues(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	mtime := time.UnixMilli(1_760_000_000_000).UTC()
	issue := store.ScanIssue{
		Root: testRoot, RelPath: badPath, Kind: store.IssueCorrupted,
		Message: "bad header", Size: 5, MTime: mtime,
	}
	writeIssue(t, s, &issue)
	st := fileStates(t, s)[badPath]
	if !st.Issue || st.TrackID != 0 || st.Size != 5 || !st.MTime.Equal(mtime) {
		t.Errorf("issue state = %+v", st)
	}

	// A track written at the same path replaces the issue.
	w := newWrite(0)
	w.RelPath = badPath
	writeAll(t, s, w)
	st = fileStates(t, s)[badPath]
	if st.Issue || st.TrackID == 0 {
		t.Errorf("state after a good read = %+v, want a plain track", st)
	}
}

// A once-good file that goes bad leaves the track and records the issue.
func TestWriteBatchIssueOverTrack(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	mtime := time.UnixMilli(1_760_000_000_000).UTC()
	issue := store.ScanIssue{
		Root: testRoot, RelPath: badPath, Kind: store.IssueCorrupted,
		Message: "bad header", Size: 5, MTime: mtime,
	}
	w := newWrite(0)
	w.RelPath = badPath
	writeAll(t, s, w)
	st := fileStates(t, s)[badPath]
	issue.Size, issue.MTime = 77, mtime.Add(time.Hour)
	writeIssue(t, s, &issue)
	both := fileStates(t, s)[badPath]
	if both.TrackID != st.TrackID || !both.Issue || both.Size != 77 || !both.MTime.Equal(issue.MTime) {
		t.Errorf("state = %+v, want track %d, Issue, size 77, mtime %v", both, st.TrackID, issue.MTime)
	}
}

func TestWriteBatchValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(w *store.TrackWrite)
	}{
		{"empty ULID", func(w *store.TrackWrite) { w.ULID = "" }},
		{"25-character ULID", func(w *store.TrackWrite) { w.ULID = strings.Repeat("0", 25) }},
		{"empty RelPath", func(w *store.TrackWrite) { w.RelPath = "" }},
		{"empty Album.ULID", func(w *store.TrackWrite) { w.Album.ULID = "" }},
		{"25-character Album.ULID", func(w *store.TrackWrite) { w.Album.ULID = strings.Repeat("0", 25) }},
		{"empty GroupKey", func(w *store.TrackWrite) { w.Album.GroupKey = "" }},
		{"empty artist MatchKey", func(w *store.TrackWrite) { w.Artists[0].MatchKey = "" }},
		{"empty artist ULID", func(w *store.TrackWrite) { w.Artists[0].ULID = "" }},
		{"25-character artist ULID", func(w *store.TrackWrite) { w.Artists[0].ULID = strings.Repeat("0", 25) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := openUsers(t)
			bad := newWrite(1)
			tc.mutate(&bad)
			err := s.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{newWrite(0), bad}})
			if !errors.Is(err, store.ErrInvalidWrite) {
				t.Fatalf("err = %v, want ErrInvalidWrite", err)
			}
			if !strings.Contains(err.Error(), "track 1") {
				t.Errorf("message %q does not name track 1", err)
			}
			if got := fileStates(t, s); len(got) != 0 {
				t.Errorf("%d paths known after a rejected batch", len(got))
			}
		})
	}
}

func TestWriteBatchEmpty(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	if err := s.WriteBatch(t.Context(), &store.Batch{}); err != nil {
		t.Fatal(err)
	}
}

func idOf(t *testing.T, s *store.Store, w *store.TrackWrite) int64 {
	t.Helper()
	st, ok := fileStates(t, s)[w.RelPath]
	if !ok {
		t.Fatalf("%s is not known", w.RelPath)
	}
	return st.TrackID
}

func TestMarkMissingFlagsTracks(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	a, b := newWrite(0), newWrite(1)
	writeAll(t, s, a, b)
	if err := s.MarkMissing(t.Context(), nil, fixedNow()); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkMissing(t.Context(), []int64{idOf(t, s, &a)}, fixedNow()); err != nil {
		t.Fatal(err)
	}
	got := fileStates(t, s)
	if !got[a.RelPath].Missing || got[b.RelPath].Missing {
		t.Errorf("Missing = %v, %v; want true, false", got[a.RelPath].Missing, got[b.RelPath].Missing)
	}
}

func TestWriteBatchMoveClearsMissing(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	w := newWrite(0)
	writeAll(t, s, w)
	id := idOf(t, s, &w)
	if err := s.MarkMissing(t.Context(), []int64{id}, fixedNow()); err != nil {
		t.Fatal(err)
	}

	moved := newWrite(0)
	moved.ID = id
	moved.RelPath = "b.flac"
	writeAll(t, s, moved)
	got := fileStates(t, s)
	if _, ok := got[w.RelPath]; ok || got["b.flac"].TrackID != id || got["b.flac"].Missing {
		t.Errorf("states after the move = %+v", got)
	}
}

func TestMissingByDigest(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	a, b, c := newWrite(0), newWrite(1), newWrite(2)
	b.Digest, c.Digest = a.Digest, a.Digest
	writeAll(t, s, a, b, c)
	ids := []int64{idOf(t, s, &a), idOf(t, s, &b)}
	if err := s.MarkMissing(t.Context(), ids, fixedNow()); err != nil {
		t.Fatal(err)
	}

	got, err := s.MissingByDigest(t.Context(), *a.Digest)
	if err != nil {
		t.Fatal(err)
	}
	want := []store.MissingTrack{
		{ID: ids[0], Root: testRoot, RelPath: a.RelPath},
		{ID: ids[1], Root: testRoot, RelPath: b.RelPath},
	}
	if !slices.Equal(got, want) {
		t.Errorf("MissingByDigest = %+v, want %+v", got, want)
	}
	none, err := s.MissingByDigest(t.Context(), [32]byte{0xff})
	if err != nil || len(none) != 0 {
		t.Errorf("unknown digest: %v, %v; want empty and nil", none, err)
	}
}

func TestPruneOrphans(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	a, b := newWrite(0), newWrite(1)
	writeAll(t, s, a, b)
	aID, bID := idOf(t, s, &a), idOf(t, s, &b)

	// Track 0 moves to track 1's album and artist.
	moved := newWrite(1)
	moved.ID = aID
	moved.RelPath = a.RelPath
	writeAll(t, s, moved)
	if err := s.MarkMissing(t.Context(), []int64{bID}, fixedNow().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n, err := s.DeleteMissingBefore(t.Context(), fixedNow()); err != nil || n != 1 {
		t.Fatalf("DeleteMissingBefore = %d, %v; want 1, nil", n, err)
	}

	got, err := s.PruneOrphans(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	wantPruned(t, got, 1, 1)
	again, err := s.PruneOrphans(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	wantPruned(t, again, 0, 0)
}

// wantPruned checks the counts of a prune that dropped no artwork.
func wantPruned(t *testing.T, got store.Pruned, albums, artists int64) {
	t.Helper()
	if got.Albums != albums || got.Artists != artists || len(got.Artwork) != 0 {
		t.Errorf("PruneOrphans = %+v, want %d albums, %d artists and no artwork", got, albums, artists)
	}
}

func TestKnownRoots(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	w := newWrite(0)
	w.Root = "/a"
	writeAll(t, s, w)
	issue := store.ScanIssue{Root: "/b", RelPath: "x", Kind: store.IssueUnreadable, Message: "m"}
	if err := s.WriteBatch(t.Context(), &store.Batch{Issues: []store.ScanIssue{issue}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.KnownRoots(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if want := []string{"/a", "/b"}; !slices.Equal(got, want) {
		t.Errorf("KnownRoots = %v, want %v", got, want)
	}
}

func TestDeleteScanIssues(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	issues := []store.ScanIssue{
		{Root: testRoot, RelPath: "one", Kind: store.IssueCorrupted, Message: "m"},
		{Root: testRoot, RelPath: "two", Kind: store.IssueUnsupported, Message: "m"},
	}
	if err := s.WriteBatch(t.Context(), &store.Batch{Issues: issues}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteScanIssues(t.Context(), testRoot, []string{"one", "unknown"}); err != nil {
		t.Fatal(err)
	}
	got := fileStates(t, s)
	if _, ok := got["one"]; ok || !got["two"].Issue {
		t.Errorf("states = %+v, want only two", got)
	}
}
