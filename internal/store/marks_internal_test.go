package store

import (
	"errors"
	"fmt"
	"maps"
	"testing"
	"time"
)

// Public IDs seedLibrary writes, beside firstTrack.
var (
	seedAlbum  = fmt.Sprintf("B%025d", 1) //nolint:gochecknoglobals // Read-only fixture shared by the tests below.
	seedArtist = fmt.Sprintf("A%025d", 1) //nolint:gochecknoglobals // Read-only fixture shared by the tests below.
)

// markedStore is a store with seedLibrary's rows in it but no marks, on a clock
// that says what time it is.
func markedStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	s := seededStore(t)
	const unmark = "DELETE FROM track_hearts; DELETE FROM album_hearts; DELETE FROM artist_hearts; DELETE FROM track_ratings;"
	if _, err := s.w.ExecContext(t.Context(), unmark); err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1000).UTC()
	s.now = func() time.Time { return now }
	return s, &now
}

func mustHeart(t *testing.T, s *Store, userID int64, kind Kind, id string) {
	t.Helper()
	if err := s.Heart(t.Context(), userID, kind, id); err != nil {
		t.Fatal(err)
	}
}

func mustRate(t *testing.T, s *Store, userID int64, id string, stars int) {
	t.Helper()
	if err := s.Rate(t.Context(), userID, id, stars); err != nil {
		t.Fatal(err)
	}
}

// marksOf returns the user's marks on seedLibrary's album, read the album
// page's way.
func marksOf(t *testing.T, s *Store, userID int64) AlbumMarks {
	t.Helper()
	m, err := s.AlbumMarks(t.Context(), userID, seedAlbum)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestHeartingAgainKeepsTheFirstTime(t *testing.T) {
	t.Parallel()
	s, now := markedStore(t)
	for _, kind := range []Kind{KindTrack, KindAlbum, KindArtist} {
		id := map[Kind]string{KindTrack: firstTrack, KindAlbum: seedAlbum, KindArtist: seedArtist}[kind]
		*now = time.UnixMilli(1000)
		mustHeart(t, s, 1, kind, id)
		*now = time.UnixMilli(2000)
		mustHeart(t, s, 1, kind, id)
	}
	for _, m := range markFacts[:3] {
		if got := rawRows(t, s.w, "SELECT hearted_at FROM "+m.table); len(got) != 1 || got[0] != "1000" {
			t.Errorf("%s hearted_at = %v, want one heart at 1000", m.table, got)
		}
	}
}

func TestUnheartTakesOnlyThatHeart(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	second := fmt.Sprintf("T%025d", 2)
	mustHeart(t, s, 1, KindTrack, firstTrack)
	mustHeart(t, s, 1, KindTrack, second)
	mustHeart(t, s, 1, KindAlbum, seedAlbum)
	for range 2 { // A second unheart has nothing to take, and is no error.
		if err := s.Unheart(t.Context(), 1, KindTrack, firstTrack); err != nil {
			t.Fatal(err)
		}
	}
	want := AlbumMarks{Hearted: true, Tracks: map[string]Marks{second: {Hearted: true}}}
	if got := marksOf(t, s, 1); got.Hearted != want.Hearted || !maps.Equal(got.Tracks, want.Tracks) {
		t.Errorf("marks = %+v, want %+v", got, want)
	}
}

func TestRateReplacesTheStars(t *testing.T) {
	t.Parallel()
	s, now := markedStore(t)
	mustRate(t, s, 1, firstTrack, 2)
	*now = time.UnixMilli(5000)
	mustRate(t, s, 1, firstTrack, 5)
	if got := rawRows(t, s.w, "SELECT stars, rated_at FROM track_ratings"); len(got) != 1 || got[0] != "5 5000" {
		t.Errorf("ratings = %v, want one of 5 at 5000", got)
	}
	if err := s.Unrate(t.Context(), 1, firstTrack); err != nil {
		t.Fatal(err)
	}
	if err := s.Unrate(t.Context(), 1, firstTrack); err != nil {
		t.Fatal(err)
	}
	if got := marksOf(t, s, 1).Tracks; len(got) != 0 {
		t.Errorf("marks = %+v, want none", got)
	}
}

func TestRateRefusesStarsOutOfRange(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	for _, stars := range []int{-1, 0, MaxStars + 1} {
		if err := s.Rate(t.Context(), 1, firstTrack, stars); !errors.Is(err, ErrBadStars) {
			t.Errorf("Rate(%d) = %v, want ErrBadStars", stars, err)
		}
	}
}

func TestMarksOnUnknownItemsAreNotFound(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	for _, kind := range []Kind{KindTrack, KindAlbum, KindArtist} {
		if err := s.Heart(t.Context(), 1, kind, "nope"); !errors.Is(err, ErrNotFound) {
			t.Errorf("Heart(%s) = %v, want ErrNotFound", kind, err)
		}
		if err := s.Unheart(t.Context(), 1, kind, "nope"); !errors.Is(err, ErrNotFound) {
			t.Errorf("Unheart(%s) = %v, want ErrNotFound", kind, err)
		}
	}
	if err := s.Rate(t.Context(), 1, "nope", 3); !errors.Is(err, ErrNotFound) {
		t.Errorf("Rate = %v, want ErrNotFound", err)
	}
	if err := s.Unrate(t.Context(), 1, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Unrate = %v, want ErrNotFound", err)
	}
	if err := s.Heart(t.Context(), 1, Kind(9), firstTrack); !errors.Is(err, errBadKind) {
		t.Errorf("Heart(Kind(9)) = %v, want errBadKind", err)
	}
}

func TestMarksKeepUsersApart(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	other := addUser(t, s)
	mustHeart(t, s, other, KindTrack, firstTrack)
	mustHeart(t, s, other, KindAlbum, seedAlbum)
	mustHeart(t, s, other, KindArtist, seedArtist)
	mustRate(t, s, other, firstTrack, 4)

	if got := marksOf(t, s, 1); got.Hearted || len(got.Tracks) != 0 {
		t.Errorf("the first user's marks = %+v, want none", got)
	}
	index, err := s.LibraryIndex(t.Context(), 1, &IndexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if a := index.Artists[0]; a.Hearted || a.Albums[0].Hearted || a.Albums[0].Tracks[0].Marks != (Marks{}) {
		t.Errorf("the first user's index = %+v, want no marks", a)
	}
	if got := marksOf(t, s, other); !got.Hearted || got.Tracks[firstTrack] != (Marks{Hearted: true, Stars: 4}) {
		t.Errorf("the second user's marks = %+v", got)
	}
}

func TestIndexCarriesTheUsersMarks(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	mustHeart(t, s, 1, KindArtist, seedArtist)
	mustHeart(t, s, 1, KindAlbum, seedAlbum)
	mustHeart(t, s, 1, KindTrack, firstTrack)
	second := fmt.Sprintf("T%025d", 2)
	mustRate(t, s, 1, second, 3)

	index, err := s.LibraryIndex(t.Context(), 1, &IndexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Artists) != 1 || len(index.Artists[0].Albums) != 1 {
		t.Fatalf("index = %+v, want one artist with one album", index)
	}
	artist := index.Artists[0]
	album := artist.Albums[0]
	if !artist.Hearted || !album.Hearted {
		t.Errorf("artist hearted = %t, album hearted = %t, want both", artist.Hearted, album.Hearted)
	}
	got := map[string]Marks{}
	for _, tr := range album.Tracks {
		if tr.Marks != (Marks{}) {
			got[tr.ID] = tr.Marks
		}
	}
	if want := map[string]Marks{firstTrack: {Hearted: true}, second: {Stars: 3}}; !maps.Equal(got, want) {
		t.Errorf("track marks = %v, want %v", got, want)
	}
}

func TestDeletingATrackDeletesItsMarks(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	mustHeart(t, s, 1, KindTrack, firstTrack)
	mustRate(t, s, 1, firstTrack, 5)
	if _, err := s.w.ExecContext(t.Context(), "UPDATE tracks SET missing_since = 1 WHERE ulid = ?", firstTrack); err != nil {
		t.Fatal(err)
	}
	mustHeart(t, s, 1, KindTrack, firstTrack) // Missing still takes a heart: the file may be back.
	if n, err := s.DeleteMissingBefore(t.Context(), time.UnixMilli(2)); err != nil || n != 1 {
		t.Fatalf("DeleteMissingBefore = %d, %v, want 1", n, err)
	}
	for _, table := range []string{"track_hearts", "track_ratings"} {
		if got := rawRows(t, s.w, "SELECT count(*) FROM "+table); got[0] != "0" {
			t.Errorf("%s rows = %s, want 0", table, got[0])
		}
	}
}

func TestPruningDeletesAlbumAndArtistHearts(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	mustHeart(t, s, 1, KindAlbum, seedAlbum)
	mustHeart(t, s, 1, KindArtist, seedArtist)
	if _, err := s.w.ExecContext(t.Context(), "UPDATE tracks SET missing_since = 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteMissingBefore(t.Context(), time.UnixMilli(2)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PruneOrphans(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"album_hearts", "artist_hearts"} {
		if got := rawRows(t, s.w, "SELECT count(*) FROM "+table); got[0] != "0" {
			t.Errorf("%s rows = %s, want 0", table, got[0])
		}
	}
}

func TestDeletingAUserDeletesTheirMarks(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	other := addUser(t, s)
	mustHeart(t, s, other, KindTrack, firstTrack)
	mustHeart(t, s, other, KindAlbum, seedAlbum)
	mustHeart(t, s, other, KindArtist, seedArtist)
	mustRate(t, s, other, firstTrack, 1)
	if _, err := s.w.ExecContext(t.Context(), "DELETE FROM users WHERE id = ?", other); err != nil {
		t.Fatal(err)
	}
	for _, m := range markFacts {
		if got := rawRows(t, s.w, "SELECT count(*) FROM "+m.table); got[0] != "0" {
			t.Errorf("%s rows = %s, want 0", m.table, got[0])
		}
	}
}
