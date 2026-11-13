package store

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
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

func mustRate(t *testing.T, s *Store, userID int64, id string, r rating.Rating) {
	t.Helper()
	if err := s.Rate(t.Context(), userID, id, r); err != nil {
		t.Fatal(err)
	}
}

// ratingsOf returns the user's ratings on seedLibrary's album, read the album
// page's way.
func ratingsOf(t *testing.T, s *Store, userID int64) map[string]rating.Rating {
	t.Helper()
	m, err := s.AlbumRatings(t.Context(), userID, seedAlbum)
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

func TestRateReplacesTheRating(t *testing.T) {
	t.Parallel()
	s, now := markedStore(t)
	mustRate(t, s, 1, firstTrack, rating.Min)
	*now = time.UnixMilli(5000)
	mustRate(t, s, 1, firstTrack, rating.Max)
	if got := rawRows(t, s.w, "SELECT halves, rated_at FROM track_ratings"); len(got) != 1 || got[0] != "10 5000" {
		t.Errorf("ratings = %v, want one of 10 halves at 5000", got)
	}
	if err := s.Unrate(t.Context(), 1, firstTrack); err != nil {
		t.Fatal(err)
	}
	if err := s.Unrate(t.Context(), 1, firstTrack); err != nil {
		t.Fatal(err)
	}
	if got := ratingsOf(t, s, 1); len(got) != 0 {
		t.Errorf("ratings = %+v, want none", got)
	}
}

func TestRateRefusesWhatIsNotARating(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	for _, r := range []rating.Rating{rating.None, rating.Max + 1, 255} {
		if err := s.Rate(t.Context(), 1, firstTrack, r); !errors.Is(err, rating.ErrBad) {
			t.Errorf("Rate(%d) = %v, want rating.ErrBad", r, err)
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

func TestRatingsKeepUsersApart(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	other := addUser(t, s)
	mustRate(t, s, other, firstTrack, 8)

	if got := ratingsOf(t, s, 1); len(got) != 0 {
		t.Errorf("the first user's ratings = %+v, want none", got)
	}
	index, err := s.LibraryIndex(t.Context(), 1, &IndexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := index.Artists[0].Albums[0].Tracks[0].Rating; got != rating.None {
		t.Errorf("the first user's index rates the first track %v, want none", got)
	}
	if got := ratingsOf(t, s, other); got[firstTrack] != 8 {
		t.Errorf("the second user's ratings = %+v", got)
	}
}

func TestIndexCarriesTheUsersRatings(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	mustHeart(t, s, 1, KindTrack, firstTrack) // A heart shows nowhere.
	second := fmt.Sprintf("T%025d", 2)
	mustRate(t, s, 1, second, 7)

	index, err := s.LibraryIndex(t.Context(), 1, &IndexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Artists) != 1 || len(index.Artists[0].Albums) != 1 {
		t.Fatalf("index = %+v, want one artist with one album", index)
	}
	got := map[string]rating.Rating{}
	for _, tr := range index.Artists[0].Albums[0].Tracks {
		if tr.Rating != rating.None {
			got[tr.ID] = tr.Rating
		}
	}
	if want := map[string]rating.Rating{second: 7}; !maps.Equal(got, want) {
		t.Errorf("track ratings = %v, want %v", got, want)
	}
}

func TestDeletingATrackDeletesItsMarks(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	mustHeart(t, s, 1, KindTrack, firstTrack)
	mustRate(t, s, 1, firstTrack, rating.Max)
	if _, err := s.w.ExecContext(t.Context(), "UPDATE tracks SET missing_since = 1 WHERE ulid = ?", firstTrack); err != nil {
		t.Fatal(err)
	}
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

// hearted is a store and the second user in it.
type hearted struct {
	s     *Store
	other int64
}

// heartedStore is markedStore with user 1's hearts on seedLibrary's first
// three tracks, the album and the artist, the second track rated 4½★ and the
// third 2★, and a second user, other, with hearts and ratings of their own.
func heartedStore(t *testing.T) hearted {
	t.Helper()
	s, now := markedStore(t)
	other := addUser(t, s)
	for i, id := range []string{trackULID(1), trackULID(2), trackULID(3)} {
		*now = time.UnixMilli(int64(1000 * (i + 1)))
		mustHeart(t, s, 1, KindTrack, id)
	}
	mustHeart(t, s, 1, KindAlbum, seedAlbum)
	mustHeart(t, s, 1, KindArtist, seedArtist)
	mustRate(t, s, 1, trackULID(2), 9)
	mustRate(t, s, 1, trackULID(3), 4)
	mustHeart(t, s, other, KindTrack, trackULID(1))
	mustHeart(t, s, other, KindAlbum, seedAlbum)
	mustRate(t, s, other, trackULID(4), 6)
	return hearted{s, other}
}

func mustSummary(t *testing.T, s *Store, userID int64) HeartsSummary {
	t.Helper()
	h, err := s.HeartsSummary(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// mustConvert turns user 1's hearts into stars.
func mustConvert(t *testing.T, s *Store, stars rating.Rating) HeartsConverted {
	t.Helper()
	c, err := s.ConvertHearts(t.Context(), 1, stars)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// sameSummary reports whether two summaries count the same.
func sameSummary(a, b *HeartsSummary) bool {
	return a.ByStars == b.ByStars && a.Unrated == b.Unrated && a.Albums == b.Albums &&
		a.Artists == b.Artists && slices.Equal(a.Preview, b.Preview)
}

// sameConverted reports whether two conversions wrote the same.
func sameConverted(a, b *HeartsConverted) bool {
	return slices.Equal(a.Rated, b.Rated) && a.Tracks == b.Tracks && a.Albums == b.Albums && a.Artists == b.Artists
}

func TestHeartsSummaryCountsByKindAndRating(t *testing.T) {
	t.Parallel()
	h := heartedStore(t)
	s, other := h.s, h.other
	got := mustSummary(t, s, 1)
	want := HeartsSummary{
		ByStars: [5]int{0, 1, 0, 0, 1}, Unrated: 1, Albums: 1, Artists: 1,
		Preview: []HeartedTrack{{ID: firstTrack, Title: "Hørizøn alpha", Artist: seedBand}},
	}
	if !sameSummary(&got, &want) || got.Rated() != 2 {
		t.Errorf("summary = %+v, want %+v", got, want)
	}
	theirs := mustSummary(t, s, other)
	if theirs.Unrated != 1 || theirs.Rated() != 0 || theirs.Albums != 1 {
		t.Errorf("the other user's summary = %+v", theirs)
	}
}

func TestConvertHeartsRatesTheUnratedAndTakesEveryHeart(t *testing.T) {
	t.Parallel()
	s := heartedStore(t).s
	queued := outbox(t, s)
	got := mustConvert(t, s, 8)
	if want := (HeartsConverted{Rated: []string{firstTrack}, Tracks: 3, Albums: 1, Artists: 1}); !sameConverted(&got, &want) {
		t.Errorf("converted = %+v, want %+v", got, want)
	}
	if got, want := ratingsOf(t, s, 1), map[string]rating.Rating{trackULID(1): 8, trackULID(2): 9, trackULID(3): 4}; !maps.Equal(got, want) {
		t.Errorf("ratings = %v, want %v: only the unrated heart gets stars", got, want)
	}
	if got := mustSummary(t, s, 1); !sameSummary(&got, &HeartsSummary{}) {
		t.Errorf("summary after = %+v, want all zeros", got)
	}
	if got := outbox(t, s); !slices.Equal(got, queued) {
		t.Errorf("outbox = %q, want %q: nothing reaches Last.fm", got, queued)
	}
}

func TestConvertHeartsTwiceDoesNothingMore(t *testing.T) {
	t.Parallel()
	s := heartedStore(t).s
	mustConvert(t, s, 8)
	if again := mustConvert(t, s, 4); !sameConverted(&again, &HeartsConverted{}) {
		t.Errorf("a second conversion = %+v, want nothing done", again)
	}
	if got, want := ratingsOf(t, s, 1), map[string]rating.Rating{trackULID(1): 8, trackULID(2): 9, trackULID(3): 4}; !maps.Equal(got, want) {
		t.Errorf("ratings = %v, want %v", got, want)
	}
}

func TestConvertHeartsLeavesOthersAlone(t *testing.T) {
	t.Parallel()
	h := heartedStore(t)
	s, other := h.s, h.other
	before := mustSummary(t, s, other)
	mustConvert(t, s, 8)
	if got := mustSummary(t, s, other); !sameSummary(&got, &before) {
		t.Errorf("the other user's hearts = %+v, want %+v", got, before)
	}
	if got := rawRows(t, s.w, "SELECT track_id, halves FROM track_ratings WHERE user_id = ?", other); !slices.Equal(got, []string{"4 6"}) {
		t.Errorf("the other user's ratings = %q, want their one", got)
	}
}

func TestConvertHeartsCanLeaveThemUnrated(t *testing.T) {
	t.Parallel()
	s := heartedStore(t).s
	got, err := s.ConvertHearts(t.Context(), 1, rating.None)
	if err != nil || len(got.Rated) != 0 || got.Tracks != 3 {
		t.Errorf("converted = %+v, %v, want 3 hearts gone and nothing rated", got, err)
	}
	if got, want := ratingsOf(t, s, 1), map[string]rating.Rating{trackULID(2): 9, trackULID(3): 4}; !maps.Equal(got, want) {
		t.Errorf("ratings = %v, want %v", got, want)
	}
}

func TestConvertHeartsQueuesNothingForLastfm(t *testing.T) {
	t.Parallel()
	s := heartedStore(t).s
	if _, err := s.w.ExecContext(t.Context(), "DELETE FROM lastfm_outbox"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConnectLastfm(t.Context(), 1, "simonhull", "sk-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLastfmSwitches(t.Context(), 1, LastfmSwitches{Scrobble: true, NowPlaying: true, Loves: true, LoveFrom: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConvertHearts(t.Context(), 1, rating.Max); err != nil {
		t.Fatal(err)
	}
	if got := outbox(t, s); len(got) != 0 {
		t.Errorf("outbox = %q, want empty: those songs were loved through their hearts", got)
	}
	mustRate(t, s, 1, trackULID(5), rating.Max) // What would queue does, so the check means something.
	if got := outbox(t, s); len(got) != 1 {
		t.Errorf("outbox after a rating = %q, want one love", got)
	}
}

// BenchmarkRate times the smallest write: one rating that changes nothing after the first.
func BenchmarkRate(b *testing.B) {
	ctx := b.Context()
	s := openBench(b)
	b.Cleanup(func() { _ = s.Close() })
	if err := s.WriteBatch(ctx, benchBatch()); err != nil {
		b.Fatal(err)
	}
	const user = 1
	if _, err := s.w.ExecContext(ctx, "INSERT INTO users (id, name, password_hash, role, created_at) VALUES (?, 'simon', 'h', 'admin', 1)", user); err != nil {
		b.Fatal(err)
	}
	var ulid string
	if err := s.r.QueryRowContext(ctx, "SELECT ulid FROM tracks ORDER BY id LIMIT 1").Scan(&ulid); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if err := s.Rate(ctx, user, ulid, rating.Max); err != nil {
			b.Fatal(err)
		}
	}
}
