package store

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"
)

// mbidKey is the raw tag the tests keep a track's MusicBrainz ID under.
const (
	mbidKey  = "MUSICBRAINZ_TRACKID"
	seedBand = "Band" // seedLibrary's one artist.
)

// lastfmStore is a store with seedLibrary's rows, no plays, marks or Last.fm
// rows, and user 1 connected to Last.fm, on a clock that says what time it is.
func lastfmStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	s, now := markedStore(t)
	if _, err := s.w.ExecContext(t.Context(), "DELETE FROM plays; DELETE FROM lastfm_accounts"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConnectLastfm(t.Context(), 1, "simonhull", "sk-1"); err != nil {
		t.Fatal(err)
	}
	return s, now
}

// outbox lists the outbox as "user kind track at attempts next_at", in id order.
func outbox(t *testing.T, s *Store) []string {
	t.Helper()
	return rawRows(t, s.w, `SELECT o.user_id, o.kind, t.ulid, o.at, o.attempts, o.next_at
		FROM lastfm_outbox o JOIN tracks t ON t.id = o.track_id ORDER BY o.id`)
}

func trackULID(n int) string { return fmt.Sprintf("T%025d", n) }

func mustSwitch(t *testing.T, s *Store, sw LastfmSwitches) {
	t.Helper()
	if err := s.SetLastfmSwitches(t.Context(), 1, sw); err != nil {
		t.Fatal(err)
	}
}

func mustAccount(t *testing.T, s *Store) LastfmAccount {
	t.Helper()
	a, err := s.LastfmAccount(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestConnectLastfmStartsWithEverythingOn(t *testing.T) {
	t.Parallel()
	s, _ := lastfmStore(t)
	got := mustAccount(t, s)
	want := LastfmAccount{
		Name: "simonhull", SessionKey: "sk-1", Switches: LastfmSwitches{Scrobble: true, NowPlaying: true, Loves: true},
		State: LastfmOK, ConnectedAt: time.UnixMilli(1000).UTC(),
	}
	if got != want {
		t.Errorf("account = %+v, want %+v", got, want)
	}
	if _, err := s.LastfmAccount(t.Context(), 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("LastfmAccount(2) err = %v, want ErrNotFound", err)
	}
}

func TestRecordPlayQueuesAScrobbleOnce(t *testing.T) {
	t.Parallel()
	s, _ := lastfmStore(t)
	mustRecord(t, s, play(1, "a"))
	mustRecord(t, s, play(1, "a")) // A retry of the same report.
	want := []string{"1 scrobble " + firstTrack + " 1000 0 1000"}
	if got := outbox(t, s); !slices.Equal(got, want) {
		t.Errorf("outbox = %q, want %q", got, want)
	}
	if got := mustAccount(t, s).Waiting; got != 1 {
		t.Errorf("Waiting = %d, want 1", got)
	}
}

func TestNothingQueuedWithoutAnAccountOrSwitch(t *testing.T) {
	t.Parallel()
	s, _ := lastfmStore(t)
	other := addUser(t, s)
	mustRecord(t, s, play(other, "a"))
	mustHeart(t, s, other, KindTrack, firstTrack)
	mustSwitch(t, s, LastfmSwitches{NowPlaying: true})
	mustRecord(t, s, play(1, "b"))
	mustHeart(t, s, 1, KindTrack, firstTrack)
	if got := outbox(t, s); len(got) != 0 {
		t.Errorf("outbox = %q, want empty", got)
	}
}

func TestTrackHeartsQueueLoves(t *testing.T) {
	t.Parallel()
	s, now := lastfmStore(t)
	mustHeart(t, s, 1, KindTrack, firstTrack)
	mustHeart(t, s, 1, KindTrack, firstTrack) // Already hearted: nothing changes.
	mustHeart(t, s, 1, KindAlbum, seedAlbum)  // Last.fm has nothing to match.
	mustHeart(t, s, 1, KindArtist, seedArtist)
	*now = time.UnixMilli(2000)
	for range 2 {
		if err := s.Unheart(t.Context(), 1, KindTrack, firstTrack); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"1 love " + firstTrack + " 1000 0 1000", "1 unlove " + firstTrack + " 2000 0 2000"}
	if got := outbox(t, s); !slices.Equal(got, want) {
		t.Errorf("outbox = %q, want %q", got, want)
	}
	if got := mustAccount(t, s).Waiting; got != 0 {
		t.Errorf("Waiting = %d, want 0: only plays count", got)
	}
}

func TestSetLastfmSwitches(t *testing.T) {
	t.Parallel()
	s, _ := lastfmStore(t)
	sw := LastfmSwitches{Scrobble: false, NowPlaying: true, Loves: false}
	mustSwitch(t, s, sw)
	if got := mustAccount(t, s).Switches; got != sw {
		t.Errorf("switches = %+v, want %+v", got, sw)
	}
	if err := s.SetLastfmSwitches(t.Context(), 2, sw); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetLastfmSwitches(2) err = %v, want ErrNotFound", err)
	}
}

// addGuest gives seedLibrary's second track a number, an MBID and a second artist.
func addGuest(t *testing.T, s *Store) {
	t.Helper()
	for _, stmt := range []struct {
		query string
		args  []any
	}{
		{query: "UPDATE tracks SET track_no = 4 WHERE id = 2"},
		{query: "INSERT INTO track_tags (track_id, key, ord, value) VALUES (2, ?, 0, 'mb-2')", args: []any{mbidKey}},
		{query: "INSERT INTO artists (id, ulid, match_key, name, sort_name) VALUES (2, ?, 'guest', 'Guest', 'Guest')", args: []any{fmt.Sprintf("A%025d", 2)}},
		{query: "INSERT INTO track_artists (track_id, role, ord, artist_id) VALUES (2, 'artist', 1, 2)"},
	} {
		if _, err := s.w.ExecContext(t.Context(), stmt.query, stmt.args...); err != nil {
			t.Fatal(err)
		}
	}
}

// due is LastfmDue for user 1 with the IDs left out, which the tests don't predict.
func due(t *testing.T, s *Store, now time.Time, limit int, kinds ...OutboxKind) []LastfmItem {
	t.Helper()
	items, err := s.LastfmDue(t.Context(), 1, now, limit, []string{mbidKey}, kinds...)
	if err != nil {
		t.Fatal(err)
	}
	for i := range items {
		items[i].ID = 0
	}
	return items
}

func TestLastfmDueInOrderWithTrackFacts(t *testing.T) {
	t.Parallel()
	s, now := lastfmStore(t)
	addGuest(t, s)
	for i, n := range []int{2, 1, 3} {
		mustRecord(t, s, &NewPlay{UserID: 1, TrackID: trackULID(n), PlayedAt: time.UnixMilli(int64(100 * (i + 1))), Source: playSource, ClientID: strconv.Itoa(i)})
	}
	mustHeart(t, s, 1, KindTrack, trackULID(3))
	track := func(word string) LastfmTrack {
		return LastfmTrack{Artists: []string{seedBand}, Title: "Hørizøn " + word, Album: "Album", AlbumArtist: seedBand, Duration: time.Second}
	}
	bravo := track("bravo")
	bravo.Artists, bravo.MBID, bravo.Number = []string{seedBand, "Guest"}, "mb-2", 4
	want := []LastfmItem{
		{Kind: OutboxScrobble, At: time.UnixMilli(100).UTC(), Track: bravo},
		{Kind: OutboxScrobble, At: time.UnixMilli(200).UTC(), Track: track("alpha")},
	}
	if got := due(t, s, *now, 2, OutboxScrobble); !reflect.DeepEqual(got, want) {
		t.Errorf("scrobbles = %+v,\nwant %+v", got, want)
	}
	wantLoves := []LastfmItem{{Kind: OutboxLove, At: now.UTC(), Track: track("charlie")}}
	if got := due(t, s, *now, 10, OutboxLove, OutboxUnlove); !reflect.DeepEqual(got, wantLoves) {
		t.Errorf("loves = %+v,\nwant %+v", got, wantLoves)
	}
}

func TestLastfmTrackInfo(t *testing.T) {
	t.Parallel()
	s, _ := lastfmStore(t)
	got, err := s.LastfmTrackInfo(t.Context(), firstTrack, []string{mbidKey})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Hørizøn alpha" || !slices.Equal(got.Artists, []string{seedBand}) || got.Album != "Album" {
		t.Errorf("track = %+v", got)
	}
	if _, err = s.LastfmTrackInfo(t.Context(), trackULID(99), nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown track err = %v, want ErrNotFound", err)
	}
}

// TestDeferKeepsOrder holds the outbox, then queues more: nothing new can
// overtake what waits.
func TestDeferKeepsOrder(t *testing.T) {
	t.Parallel()
	s, now := lastfmStore(t)
	mustRecord(t, s, play(1, "a"))
	if err := s.DeferLastfm(t.Context(), 1, time.UnixMilli(61_000)); err != nil {
		t.Fatal(err)
	}
	*now = time.UnixMilli(5000)
	mustRecord(t, s, &NewPlay{UserID: 1, TrackID: trackULID(2), PlayedAt: time.UnixMilli(4000), Source: playSource, ClientID: "b"})
	want := []string{"1 scrobble " + firstTrack + " 1000 1 61000", "1 scrobble " + trackULID(2) + " 4000 0 61000"}
	if got := outbox(t, s); !slices.Equal(got, want) {
		t.Errorf("outbox = %q, want %q", got, want)
	}
	if due, err := s.LastfmDueUsers(t.Context(), *now); err != nil || len(due) != 0 {
		t.Errorf("due users at 5000 = %v, %v, want none", due, err)
	}
	if due, err := s.LastfmDueUsers(t.Context(), time.UnixMilli(61_000)); err != nil || !slices.Equal(due, []int64{1}) {
		t.Errorf("due users at 61000 = %v, %v, want [1]", due, err)
	}
	a := mustAccount(t, s)
	if a.Attempts != 1 || !a.NextAt.Equal(time.UnixMilli(61_000)) || a.Waiting != 2 {
		t.Errorf("account = %+v, want 1 attempt, next at 61000, 2 waiting", a)
	}
}

func TestReconnectHoldsTheOutbox(t *testing.T) {
	t.Parallel()
	s, _ := lastfmStore(t)
	mustRecord(t, s, play(1, "a"))
	if err := s.SetLastfmReconnect(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if got := mustAccount(t, s).State; got != LastfmReconnect {
		t.Errorf("state = %v, want reconnect", got)
	}
	if due, err := s.LastfmDueUsers(t.Context(), time.UnixMilli(2_000_000)); err != nil || len(due) != 0 {
		t.Errorf("due users = %v, %v, want none while reconnecting", due, err)
	}
	mustRecord(t, s, play(1, "b")) // Plays pile up meanwhile.
	if got := mustAccount(t, s).Waiting; got != 2 {
		t.Errorf("Waiting = %d, want 2", got)
	}
}

func TestConnectAgainResumesAtOnce(t *testing.T) {
	t.Parallel()
	s, now := lastfmStore(t)
	mustSwitch(t, s, LastfmSwitches{Scrobble: true})
	mustRecord(t, s, play(1, "a"))
	if err := s.DeferLastfm(t.Context(), 1, time.UnixMilli(1_000_000)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLastfmReconnect(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	*now = time.UnixMilli(3000)
	if err := s.ConnectLastfm(t.Context(), 1, "simonhull", "sk-2"); err != nil {
		t.Fatal(err)
	}
	a := mustAccount(t, s)
	if a.State != LastfmOK || a.SessionKey != "sk-2" || a.Switches != (LastfmSwitches{Scrobble: true}) || a.Attempts != 0 {
		t.Errorf("account = %+v, want ok, the new key and its switches kept", a)
	}
	if due, err := s.LastfmDueUsers(t.Context(), *now); err != nil || !slices.Equal(due, []int64{1}) {
		t.Errorf("due users = %v, %v, want [1] at once", due, err)
	}
}

func TestSentAndDropped(t *testing.T) {
	t.Parallel()
	s, now := lastfmStore(t)
	mustRecord(t, s, play(1, "a"))
	mustHeart(t, s, 1, KindTrack, firstTrack)
	items, err := s.LastfmDue(t.Context(), 1, *now, 50, nil, OutboxScrobble)
	if err != nil || len(items) != 1 {
		t.Fatalf("due = %v, %v", items, err)
	}
	if err = s.LastfmSent(t.Context(), 1, []int64{items[0].ID}, time.UnixMilli(7000)); err != nil {
		t.Fatal(err)
	}
	loves, err := s.LastfmDue(t.Context(), 1, *now, 50, nil, OutboxLove, OutboxUnlove)
	if err != nil || len(loves) != 1 {
		t.Fatalf("loves = %v, %v", loves, err)
	}
	if err = s.DropLastfm(t.Context(), 1, []int64{loves[0].ID}); err != nil {
		t.Fatal(err)
	}
	a := mustAccount(t, s)
	if len(outbox(t, s)) != 0 || !a.LastSentAt.Equal(time.UnixMilli(7000)) {
		t.Errorf("outbox = %q, last sent %v; want empty, sent at 7000", outbox(t, s), a.LastSentAt)
	}
}

func TestDropOldLastfmScrobbles(t *testing.T) {
	t.Parallel()
	s, _ := lastfmStore(t)
	mustRecord(t, s, play(1, "a"))
	mustRecord(t, s, &NewPlay{UserID: 1, TrackID: firstTrack, PlayedAt: time.UnixMilli(5000), Source: playSource, ClientID: "b"})
	mustHeart(t, s, 1, KindTrack, firstTrack) // Loves don't age out.
	n, err := s.DropOldLastfmScrobbles(t.Context(), 1, time.UnixMilli(2000))
	if err != nil || n != 1 {
		t.Fatalf("DropOldLastfmScrobbles = %d, %v, want 1", n, err)
	}
	if got := len(outbox(t, s)); got != 2 {
		t.Errorf("outbox = %d rows, want 2", got)
	}
}

func TestDisconnectClearsTheOutbox(t *testing.T) {
	t.Parallel()
	s, _ := lastfmStore(t)
	mustRecord(t, s, play(1, "a"))
	for range 2 { // There is nothing left to disconnect the second time, and that is no error.
		if err := s.DisconnectLastfm(t.Context(), 1); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(outbox(t, s)); got != 0 {
		t.Errorf("outbox after disconnect = %d rows, want 0", got)
	}
}

func TestDeletingATrackDeletesItsOutboxRows(t *testing.T) {
	t.Parallel()
	s, _ := lastfmStore(t)
	mustRecord(t, s, play(1, "a"))
	if _, err := s.w.ExecContext(t.Context(), "UPDATE tracks SET missing_since = 1 WHERE ulid = ?", firstTrack); err != nil {
		t.Fatal(err)
	}
	if n, err := s.DeleteMissingBefore(t.Context(), time.UnixMilli(2)); err != nil || n != 1 {
		t.Fatalf("DeleteMissingBefore = %d, %v, want 1", n, err)
	}
	if got := len(outbox(t, s)); got != 0 {
		t.Errorf("outbox after the track went = %d rows, want 0", got)
	}
}

func TestDeletingAUserDeletesTheirAccount(t *testing.T) {
	t.Parallel()
	s, _ := lastfmStore(t)
	mustRecord(t, s, play(1, "a"))
	if _, err := s.w.ExecContext(t.Context(), "DELETE FROM users WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	if got := rawInt(t, s.w, "SELECT count(*) FROM lastfm_accounts") + rawInt(t, s.w, "SELECT count(*) FROM lastfm_outbox"); got != 0 {
		t.Errorf("Last.fm rows after the user went = %d, want 0", got)
	}
}
