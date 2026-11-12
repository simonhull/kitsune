package store

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// historyStore is a store with user 1, three albums of one track each, plus a
// second track on album 1, and no plays.
func historyStore(t *testing.T) *Store {
	t.Helper()
	s := userStore(t)
	second := newWrite(4)
	second.Album = newWrite(1).Album
	writeAll(t, s, newWrite(1), newWrite(2), newWrite(3), second)
	return s
}

// userStore is an empty library with user 1.
func userStore(t *testing.T) *Store {
	t.Helper()
	s := openTest(t)
	if _, err := s.w.ExecContext(t.Context(), "INSERT INTO users (id, name, password_hash, role, created_at) VALUES (1, 'simon', 'h', 'admin', 1)"); err != nil {
		t.Fatal(err)
	}
	return s
}

// playAt records a play of newWrite's track n by the user, at ms.
func playAt(t *testing.T, s *Store, userID int64, n int, ms int64) {
	t.Helper()
	mustRecord(t, s, &NewPlay{
		UserID: userID, TrackID: newWrite(n).ULID, PlayedAt: time.UnixMilli(ms).UTC(),
		Source: playSource, ClientID: fmt.Sprintf("%d-%d", n, ms),
	})
}

// seenHistory is a history as album IDs, with the plays of each on repeat.
type seenHistory struct{ recent, onRepeat []string }

// history reads the user's history from since, as album IDs, with the plays
// of each on repeat.
func history(t *testing.T, s *Store, userID, since int64) seenHistory {
	t.Helper()
	h, err := s.History(t.Context(), userID, time.UnixMilli(since))
	if err != nil {
		t.Fatal(err)
	}
	recent := make([]string, len(h.Recent))
	for i := range h.Recent {
		recent[i] = h.Recent[i].ID
	}
	onRepeat := make([]string, len(h.OnRepeat))
	for i := range h.OnRepeat {
		onRepeat[i] = fmt.Sprintf("%s×%d", h.OnRepeat[i].Album.ID, h.OnRepeat[i].Plays)
	}
	return seenHistory{recent, onRepeat}
}

func albumOf(n int) string { return newWrite(n).Album.ULID }

func TestHistoryOrdersAlbumsByPlays(t *testing.T) {
	t.Parallel()
	s := historyStore(t)
	playAt(t, s, 1, 2, 100)  // Album 2, before the window.
	playAt(t, s, 1, 2, 1100) // Album 2, in it.
	playAt(t, s, 1, 1, 1200) // Album 1, twice in it, by two of its tracks.
	playAt(t, s, 1, 4, 1300)
	playAt(t, s, 1, 3, 1400) // Album 3, once in it, latest of all.
	got := history(t, s, 1, 1000)
	recent, onRepeat := got.recent, got.onRepeat
	if want := []string{albumOf(3), albumOf(1), albumOf(2)}; !slices.Equal(recent, want) {
		t.Errorf("recent = %v, want %v: newest play first", recent, want)
	}
	want := []string{albumOf(1) + "×2", albumOf(3) + "×1", albumOf(2) + "×1"}
	if !slices.Equal(onRepeat, want) {
		t.Errorf("on repeat = %v, want %v: most plays in the window, ties to the latest", onRepeat, want)
	}
}

func TestHistoryAlbumsAreSummaries(t *testing.T) {
	t.Parallel()
	s := historyStore(t)
	playAt(t, s, 1, 4, 1000)
	h, err := s.History(t.Context(), 1, time.UnixMilli(0))
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Recent) != 1 || len(h.OnRepeat) != 1 {
		t.Fatalf("history = %+v, want album 1 in each row", h)
	}
	if got := h.Recent[0]; got.Title != "Album 1" || got.Artist != "Artist 1" || got.TrackCount != 2 {
		t.Errorf("recent album = %+v, want album 1 as a list shows it", got)
	}
	if got := h.OnRepeat[0].Album; got != h.Recent[0] {
		t.Errorf("on repeat album = %+v, want the same summary as recent's", got)
	}
}

func TestHistoryLeavesOutMissingTracksAndOthersPlays(t *testing.T) {
	t.Parallel()
	s := historyStore(t)
	other := addUser(t, s)
	playAt(t, s, 1, 1, 1000)
	playAt(t, s, 1, 2, 2000)
	playAt(t, s, other, 3, 3000)
	if err := s.MarkMissing(t.Context(), []int64{trackID(t, s, newWrite(2).RelPath)}, time.UnixMilli(5000)); err != nil {
		t.Fatal(err)
	}
	got := history(t, s, 1, 0)
	recent, onRepeat := got.recent, got.onRepeat
	if !slices.Equal(recent, []string{albumOf(1)}) || !slices.Equal(onRepeat, []string{albumOf(1) + "×1"}) {
		t.Errorf("history = %v and %v, want album 1 alone: album 2's song is missing, album 3 is another's", recent, onRepeat)
	}
	if got = history(t, s, 3, 0); len(got.recent) != 0 || len(got.onRepeat) != 0 {
		t.Errorf("history of someone who never played = %v and %v, want none", got.recent, got.onRepeat)
	}
}

func TestHistoryCountsUndatedPlaysOnlyAsRecent(t *testing.T) {
	t.Parallel()
	s := historyStore(t)
	playAt(t, s, 1, 1, 1000)
	imp := Import{UserID: 1, Plays: make([]NewPlay, 3)}
	for i := range imp.Plays { // An imported song played three times, the last at 2000.
		imp.Plays[i] = NewPlay{TrackID: newWrite(2).ULID, PlayedAt: time.UnixMilli(2000), Source: SourceUndated, ClientID: fmt.Sprint("u", i)}
	}
	if _, err := s.ApplyImport(t.Context(), &imp); err != nil {
		t.Fatal(err)
	}
	got := history(t, s, 1, 0)
	if want := []string{albumOf(2), albumOf(1)}; !slices.Equal(got.recent, want) {
		t.Errorf("recent = %v, want %v: an undated play's time is the song's latest", got.recent, want)
	}
	if want := []string{albumOf(1) + "×1"}; !slices.Equal(got.onRepeat, want) {
		t.Errorf("on repeat = %v, want %v: undated plays aren't when they happened", got.onRepeat, want)
	}
}

func TestHistoryIsBounded(t *testing.T) {
	t.Parallel()
	s := userStore(t)
	tws := make([]TrackWrite, MaxHomeAlbums+5)
	for i := range tws {
		tws[i] = newWrite(i + 1)
	}
	writeAll(t, s, tws...)
	for i := range tws {
		playAt(t, s, 1, i+1, int64(1000+i))
	}
	got := history(t, s, 1, 0)
	recent, onRepeat := got.recent, got.onRepeat
	if len(recent) != MaxHomeAlbums || len(onRepeat) != MaxHomeAlbums {
		t.Errorf("history = %d recent and %d on repeat, want %d of each", len(recent), len(onRepeat), MaxHomeAlbums)
	}
	if first, want := recent[0], albumOf(len(tws)); first != want {
		t.Errorf("first recent = %s, want the last played, %s", first, want)
	}
}
