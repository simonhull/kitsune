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
	testRoot = "/music"
	badPath  = "bad.flac"
)

// newWrite returns a distinct valid write: its own album and artist per index.
func newWrite(i int) store.TrackWrite {
	return store.TrackWrite{
		ULID:      fmt.Sprintf("01J%023d", i),
		Root:      testRoot,
		RelPath:   fmt.Sprintf("a/%d.flac", i),
		Size:      1000 + int64(i),
		MTime:     time.UnixMilli(1_761_000_000_000 + int64(i)).UTC().Add(456789 * time.Nanosecond),
		Digest:    sha256.Sum256([]byte(strconv.Itoa(i))),
		Format:    "flac",
		Codec:     "flac",
		Duration:  200*time.Second + 123*time.Millisecond,
		Title:     fmt.Sprintf("Title %d", i),
		SortTitle: fmt.Sprintf("Title %d", i),
		Album: store.AlbumWrite{
			GroupKey: fmt.Sprintf("album-%d", i),
			Name:     fmt.Sprintf("Album %d", i),
			SortName: fmt.Sprintf("Album %d", i),
		},
		Artists: []store.ArtistWrite{{
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
		{"empty GroupKey", func(w *store.TrackWrite) { w.Album.GroupKey = "" }},
		{"empty artist MatchKey", func(w *store.TrackWrite) { w.Artists[0].MatchKey = "" }},
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

	got, err := s.MissingByDigest(t.Context(), a.Digest)
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
	if want := (store.Pruned{Albums: 1, Artists: 1}); got != want {
		t.Errorf("PruneOrphans = %+v, want %+v", got, want)
	}
	again, err := s.PruneOrphans(t.Context())
	if err != nil || again != (store.Pruned{}) {
		t.Errorf("second PruneOrphans = %+v, %v; want zero", again, err)
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
