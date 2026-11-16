package store_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

// organizeLib is a store with album 1's three songs, two discs on album 2,
// and a run started over them.
func organizeLib(t *testing.T) (*store.Store, *store.OrganizeLibrary, int64) {
	t.Helper()
	s, clock := openBrowse(t)
	tracks := []store.TrackWrite{
		albumTrack(1, 1, "Album 1"), albumTrack(1, 2, "Album 1"), albumTrack(1, 3, "Album 1"),
		albumTrack(2, 1, "Album 2"), albumTrack(2, 2, "Album 2"),
	}
	for i := range 3 {
		tracks[i].TrackNo = i + 1
	}
	tracks[3].DiscNo, tracks[4].DiscNo = 1, 2
	writeAll(t, s, tracks...)
	lib, err := s.OrganizeLibrary(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.StartOrganizeRun(t.Context(), &store.OrganizeRunStart{
		ULID: strings.Repeat("R", 26), Kind: store.OrganizeKindRun, Template: "<Title>", At: clock.Now(), Total: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, lib, run
}

// songAt returns the song of lib at rel.
func songAt(t *testing.T, lib *store.OrganizeLibrary, rel string) store.OrganizeSong {
	t.Helper()
	i := slices.IndexFunc(lib.Songs, func(s store.OrganizeSong) bool { return s.RelPath == rel })
	if i < 0 {
		t.Fatalf("no song at %s", rel)
	}
	return lib.Songs[i]
}

// The organizer reads every live song with what a template reads of it: its
// path, its album's discs and its disc's songs.
func TestOrganizeLibraryReadsTheSongs(t *testing.T) {
	t.Parallel()
	_, lib, _ := organizeLib(t)
	if len(lib.Songs) != 5 {
		t.Fatalf("songs = %d, want 5", len(lib.Songs))
	}
	if s := songAt(t, lib, "album1/2.flac"); s.Track != 2 || s.Discs != 1 || s.DiscSongs != 3 || s.AlbumTitle != "Album 1" || s.Size == 0 {
		t.Errorf("album 1's second song = %+v", s)
	}
	if s := songAt(t, lib, "album2/2.flac"); s.Disc != 2 || s.Discs != 2 || s.DiscSongs != 1 {
		t.Errorf("album 2's second disc = %+v, want disc 2 of 2, alone on it", s)
	}
}

// journalFirst journals the move of lib's first song to "new/" before its path.
func journalFirst(t *testing.T, s *store.Store, lib *store.OrganizeLibrary, run int64) store.OrganizeLanded {
	t.Helper()
	song := lib.Songs[0]
	moves := []store.OrganizeMove{{Track: song.ID, Root: song.Root, From: song.RelPath, To: "new/" + song.RelPath, Kind: store.MoveSong}}
	if err := s.JournalOrganize(t.Context(), run, moves); err != nil {
		t.Fatal(err)
	}
	if planned, err := s.PlannedOrganize(t.Context()); err != nil || len(planned) != 1 || planned[0].ID != moves[0].ID {
		t.Fatalf("planned = %+v, %v; want the move", planned, err)
	}
	return store.OrganizeLanded{Move: moves[0], Album: song.Album, State: store.MoveMoved}
}

// land lands one move of run, and fails unless nothing is left planned.
func land(t *testing.T, s *store.Store, run int64, landed *store.OrganizeLanded) {
	t.Helper()
	if err := s.LandOrganize(t.Context(), run, []store.OrganizeLanded{*landed}, store.OrganizeTally{Moved: 1}); err != nil {
		t.Fatal(err)
	}
	if planned, err := s.PlannedOrganize(t.Context()); err != nil || len(planned) != 0 {
		t.Errorf("planned after landing = %+v, %v; want none", planned, err)
	}
}

// A song not where the run found it lands nothing.
func TestLandOrganizeRefusesASongElsewhere(t *testing.T) {
	t.Parallel()
	s, lib, run := organizeLib(t)
	landed := journalFirst(t, s, lib, run)
	landed.Move.From = "elsewhere.flac"
	if err := s.LandOrganize(t.Context(), run, []store.OrganizeLanded{landed}, store.OrganizeTally{Moved: 1}); err == nil {
		t.Fatal("landing a song from where it isn't = nil, want an error")
	}
	if planned, err := s.PlannedOrganize(t.Context()); err != nil || len(planned) != 1 {
		t.Errorf("planned = %+v, %v; want the move still planned", planned, err)
	}
}

// A landed move sets the song's path and its journal row in one write, and
// notes its album changed.
func TestLandOrganizeMovesThePath(t *testing.T) {
	t.Parallel()
	s, lib, run := organizeLib(t)
	ctx := t.Context()
	landed := journalFirst(t, s, lib, run)
	s.TakeChangedAlbums()
	land(t, s, run, &landed)
	id := landed.Move.Track
	if states, err := s.OrganizeTrackStates(ctx, []int64{id}); err != nil || states[id].RelPath != landed.Move.To {
		t.Errorf("state = %+v, %v; want the new path", states[id], err)
	}
	if changed := s.TakeChangedAlbums(); !slices.Equal(changed, []string{landed.Album}) {
		t.Errorf("changed = %v, want the song's album", changed)
	}
	runs, err := s.OrganizeRuns(ctx, 10)
	if err != nil || len(runs) != 1 || runs[0].Moved != 1 || !runs[0].StillThere || runs[0].State != store.OrganizeRunning {
		t.Errorf("runs = %+v, %v; want the run with one song still where it put it", runs, err)
	}
}

// A journal outlives its songs: a track deleted for good leaves its row's
// track null, and the run stays.
func TestOrganizeJournalOutlivesAPurge(t *testing.T) {
	t.Parallel()
	s, lib, run := organizeLib(t)
	ctx := t.Context()
	landed := journalFirst(t, s, lib, run)
	land(t, s, run, &landed)
	if err := s.MarkMissing(ctx, []int64{landed.Move.Track}, time.UnixMilli(1)); err != nil {
		t.Fatal(err)
	}
	if n, err := s.DeleteMissingBefore(ctx, time.UnixMilli(2)); err != nil || n != 1 {
		t.Fatalf("delete = %d, %v", n, err)
	}
	moved, err := s.MovedOrganize(ctx, run)
	if err != nil || len(moved) != 1 || moved[0].Track != 0 || moved[0].Here {
		t.Errorf("moved = %+v, %v; want the row kept, its track gone", moved, err)
	}
}

// startRuns starts n runs, the first n-1 done, started at 2, 3 and so on.
func startRuns(t *testing.T, s *store.Store, n int) {
	t.Helper()
	for i := range n {
		ulid := strings.Repeat(string(rune('A'+i)), 26)
		id, err := s.StartOrganizeRun(t.Context(), &store.OrganizeRunStart{ULID: ulid, Kind: store.OrganizeKindRun, Template: "t", At: time.UnixMilli(int64(i + 2))})
		if err != nil {
			t.Fatal(err)
		}
		if i < n-1 {
			if err = s.FinishOrganizeRun(t.Context(), id, store.OrganizeDone, 0, time.UnixMilli(10)); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// Runs past the last kept and older than the cutoff go; a running one stays,
// and goes on running until a start stops it.
func TestPruneOrganizeRuns(t *testing.T) {
	t.Parallel()
	s, _, first := organizeLib(t)
	ctx := t.Context()
	if err := s.FinishOrganizeRun(ctx, first, store.OrganizeDone, 0, time.UnixMilli(1)); err != nil {
		t.Fatal(err)
	}
	startRuns(t, s, 3)
	if n, err := s.PruneOrganizeRuns(ctx, 2, time.UnixMilli(1_000_000_000_000_000)); err != nil || n != 2 {
		t.Errorf("pruned = %d, %v; want the two oldest", n, err)
	}
	if runs, err := s.OrganizeRuns(ctx, 10); err != nil || len(runs) != 2 || runs[0].State != store.OrganizeRunning {
		t.Errorf("runs = %+v, %v; want the running one first, and one more", runs, err)
	}
	if _, err := s.OrganizeRunByULID(ctx, strings.Repeat("R", 26)); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a pruned run = %v, want ErrNotFound", err)
	}
	if stopped, err := s.StopRunningOrganizeRuns(ctx, time.UnixMilli(20)); err != nil || stopped != 1 {
		t.Errorf("stopped = %d, %v; want the running one", stopped, err)
	}
}
