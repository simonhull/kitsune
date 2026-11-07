package scrobbler_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/simonhull/kitsune/internal/lastfm/lastfmtest"
	"github.com/simonhull/kitsune/internal/scrobbler"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	sessionKey = "sk-alice-0123456789"
	mbidKey    = "MUSICBRAINZ_TRACKID"
	scrobbleM  = "track.scrobble"
	annName    = "Ann"
	albumName  = "Tides"
)

// syncBuffer is a log the scrobbler and the test share.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p) //nolint:wrapcheck // strings.Builder never fails.
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// fixture is a store with one user connected to a fake Last.fm, a scrobbler
// over both, and a library of three tracks by Ann.
type fixture struct {
	st     *store.Store
	fake   *lastfmtest.Server
	s      *scrobbler.Scrobbler
	logs   *syncBuffer
	userID int64
}

func trackULID(n int) string { return fmt.Sprintf("01J%020d%03d", 1, n) }

// newFixture builds a fixture. The fake answers in memory, so the fixture works
// inside a synctest bubble.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), store.FileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	u, err := st.CreateUser(t.Context(), store.NewUser{Name: "alice", PasswordHash: "h", Role: store.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	tws := make([]store.TrackWrite, 3)
	for i := range tws {
		n := i + 1
		tws[i] = store.TrackWrite{
			ULID: trackULID(n), Root: "/music", RelPath: fmt.Sprintf("%d.flac", n),
			Digest: new(sha256.Sum256(fmt.Appendf(nil, "%d", n))), Format: "flac", Codec: "flac",
			Duration: 3 * time.Minute, Title: fmt.Sprintf("Song %d", n), TrackNo: n,
			Artists: []store.ArtistWrite{{ULID: fmt.Sprintf("03J%023d", 1), MatchKey: "ann", Name: annName, SortName: annName}},
			Album:   store.AlbumWrite{GroupKey: "g", ULID: fmt.Sprintf("02J%023d", 1), Name: albumName, SortName: albumName, Artist: annName},
		}
	}
	tws[0].Tags = []store.Tag{{Key: mbidKey, Value: "mb-1"}}
	if err = st.WriteBatch(t.Context(), &store.Batch{Tracks: tws}); err != nil {
		t.Fatal(err)
	}
	if err = st.ConnectLastfm(t.Context(), u.ID, "alice", sessionKey); err != nil {
		t.Fatal(err)
	}
	fake := lastfmtest.NewInMemory()
	fake.AddSession(sessionKey, "alice")
	client := fake.InMemoryClient()
	logs := &syncBuffer{}
	s := scrobbler.New(st, scrobbler.Options{
		Client: client, MBIDKeys: []string{mbidKey},
		Logger: slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	return &fixture{st: st, fake: fake, s: s, logs: logs, userID: u.ID}
}

// play records a play of track n that started at.
func (f *fixture) play(t *testing.T, n int, at time.Time) {
	t.Helper()
	err := f.st.RecordPlay(t.Context(), &store.NewPlay{
		UserID: f.userID, TrackID: trackULID(n), PlayedAt: at, Source: "web", ClientID: fmt.Sprintf("%d-%d", n, at.UnixMilli()),
	})
	if err != nil {
		t.Fatal(err)
	}
	f.s.Poke()
}

// run starts the sender, and stops it and waits when the test ends.
func (f *fixture) run(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Go(func() { f.s.Run(ctx) })
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})
}

func (f *fixture) account(t *testing.T) store.LastfmAccount {
	t.Helper()
	a, err := f.st.LastfmAccount(t.Context(), f.userID)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestScrobbleCarriesTheTrack(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		f.run(t)
		at := time.Now().Add(-5 * time.Minute)
		f.play(t, 1, at)
		synctest.Wait()
		calls := f.fake.CallsTo(scrobbleM)
		if len(calls) != 1 {
			t.Fatalf("scrobble calls = %d, want 1; log:\n%s", len(calls), f.logs)
		}
		p := calls[0].Params
		want := map[string]string{
			"artist[0]": annName, "track[0]": "Song 1", "album[0]": albumName, "albumArtist[0]": annName,
			"duration[0]": "180", "trackNumber[0]": "1", "mbid[0]": "mb-1",
			"timestamp[0]": strconv.FormatInt(at.Unix(), 10), "sk": sessionKey, "api_key": lastfmtest.APIKey,
		}
		for k, v := range want {
			if got := p.Get(k); got != v {
				t.Errorf("%s = %q, want %q", k, got, v)
			}
		}
		if a := f.account(t); a.Waiting != 0 || !a.LastSentAt.Equal(time.Now()) {
			t.Errorf("account = %+v, want nothing waiting and sent now", a)
		}
	})
}

func TestBatchesOfFiftyInOrder(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		start := time.Now().Add(-time.Hour)
		want := make([]string, 0, 60)
		for i := range 60 {
			n := i%3 + 1
			at := start.Add(time.Duration(i) * time.Second)
			f.play(t, n, at)
			want = append(want, fmt.Sprintf("%s - Song %d @ %d", annName, n, at.Unix()))
		}
		f.run(t)
		synctest.Wait()
		calls := f.fake.CallsTo(scrobbleM)
		if len(calls) != 2 || !calls[0].Params.Has("artist[49]") || calls[0].Params.Has("artist[50]") {
			t.Errorf("scrobble calls = %d, want a batch of 50 then one of 10", len(calls))
		}
		if got := f.fake.Scrobbles(); !slices.Equal(got, want) {
			t.Errorf("scrobbles out of order:\n got %q\nwant %q", got, want)
		}
	})
}

// wantHeld checks that the outbox has failed attempts times and waits until next.
func (f *fixture) wantHeld(t *testing.T, attempts int, next time.Time) {
	t.Helper()
	if a := f.account(t); a.Attempts != attempts || !a.NextAt.Equal(next) {
		t.Errorf("account = %+v, want %d attempts and the next at %v", a, attempts, next)
	}
}

// TestOutageBacksOffThenDeliversInOrder takes Last.fm down, queues plays,
// and brings it back: they arrive in order, after waits of 1, 2 and 4 minutes.
func TestOutageBacksOffThenDeliversInOrder(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		f.run(t)
		f.fake.SetDown(true)
		begin := time.Now()
		f.play(t, 1, begin.Add(-3*time.Minute))
		synctest.Wait()
		f.play(t, 2, begin.Add(-2*time.Minute)) // Waits behind the first, without a call of its own.
		synctest.Wait()
		f.wantHeld(t, 1, begin.Add(time.Minute))
		time.Sleep(time.Minute) // The retry at 1 minute fails too; the next waits 2.
		synctest.Wait()
		f.wantHeld(t, 2, begin.Add(3*time.Minute))
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		f.wantHeld(t, 3, begin.Add(7*time.Minute))
		f.fake.SetDown(false)
		f.play(t, 3, begin.Add(4*time.Minute)) // A poke while held changes nothing.
		synctest.Wait()
		if n := len(f.fake.CallsTo(scrobbleM)); n != 0 {
			t.Errorf("sent %d batches while held, want none", n)
		}
		time.Sleep(4 * time.Minute)
		synctest.Wait()
		want := []string{
			fmt.Sprintf("%s - Song 1 @ %d", annName, begin.Add(-3*time.Minute).Unix()),
			fmt.Sprintf("%s - Song 2 @ %d", annName, begin.Add(-2*time.Minute).Unix()),
			fmt.Sprintf("%s - Song 3 @ %d", annName, begin.Add(4*time.Minute).Unix()),
		}
		if got := f.fake.Scrobbles(); !slices.Equal(got, want) {
			t.Errorf("scrobbles = %q, want %q; log:\n%s", got, want, f.logs)
		}
		if a := f.account(t); a.Waiting != 0 || a.Attempts != 0 {
			t.Errorf("account = %+v, want an empty outbox", a)
		}
	})
}

func TestBackoffStopsAtHalfAnHour(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		f.run(t)
		f.fake.SetDown(true)
		f.play(t, 1, time.Now())
		synctest.Wait()
		for range 8 {
			time.Sleep(time.Until(f.account(t).NextAt))
			synctest.Wait()
		}
		a := f.account(t)
		if got := time.Until(a.NextAt); got != 30*time.Minute {
			t.Errorf("wait after %d failures = %v, want 30m", a.Attempts, got)
		}
	})
}

func TestRateLimitIsTemporary(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		f.fake.Fail(29)
		f.run(t)
		f.play(t, 1, time.Now())
		synctest.Wait()
		if n := len(f.fake.Scrobbles()); n != 0 {
			t.Fatalf("scrobbles = %d, want none yet", n)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if n := len(f.fake.Scrobbles()); n != 1 {
			t.Errorf("scrobbles = %d after the wait, want 1", n)
		}
	})
}

func TestInvalidSessionNeedsReconnecting(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		f.run(t)
		f.fake.Revoke(sessionKey)
		f.play(t, 1, time.Now())
		synctest.Wait()
		a := f.account(t)
		if a.State != store.LastfmReconnect || a.Waiting != 1 {
			t.Errorf("account = %+v, want reconnect with the play kept", a)
		}
		f.play(t, 2, time.Now())
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		if n := len(f.fake.CallsTo(scrobbleM)) + len(f.fake.Calls()); n != 1 {
			t.Errorf("calls = %d, want only the one that failed", n)
		}
		f.fake.AddSession("sk-new", "alice")
		if err := f.st.ConnectLastfm(t.Context(), f.userID, "alice", "sk-new"); err != nil {
			t.Fatal(err)
		}
		f.s.Poke()
		synctest.Wait()
		if got := len(f.fake.Scrobbles()); got != 2 {
			t.Errorf("scrobbles after reconnecting = %d, want 2", got)
		}
	})
}

func TestIgnoredAndRejectedAreDropped(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		f.fake.Ignore(annName)
		f.run(t)
		f.play(t, 1, time.Now())
		synctest.Wait()
		if a := f.account(t); a.Waiting != 0 {
			t.Errorf("waiting = %d, want the ignored play dropped", a.Waiting)
		}
		if !strings.Contains(f.logs.String(), "last.fm ignored a scrobble") {
			t.Errorf("log does not mention the ignored play:\n%s", f.logs)
		}
		f.fake.Fail(6)
		f.play(t, 2, time.Now())
		synctest.Wait()
		if a := f.account(t); a.Waiting != 0 || a.State != store.LastfmOK {
			t.Errorf("account = %+v, want the rejected play dropped", a)
		}
	})
}

func TestOldScrobblesAreDropped(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		f.play(t, 1, time.Now().Add(-15*24*time.Hour))
		f.play(t, 2, time.Now().Add(-time.Hour))
		f.run(t)
		synctest.Wait()
		if got := f.fake.Scrobbles(); len(got) != 1 || !strings.Contains(got[0], "Song 2") {
			t.Errorf("scrobbles = %q, want only song 2", got)
		}
		if !strings.Contains(f.logs.String(), "count=1") {
			t.Errorf("log does not count the dropped play:\n%s", f.logs)
		}
	})
}

func TestHeartsGoAsLovesInOrder(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		f.run(t)
		if err := f.st.Heart(t.Context(), f.userID, store.KindTrack, trackULID(1)); err != nil {
			t.Fatal(err)
		}
		if err := f.st.Unheart(t.Context(), f.userID, store.KindTrack, trackULID(1)); err != nil {
			t.Fatal(err)
		}
		f.s.Poke()
		synctest.Wait()
		calls := f.fake.Calls()
		got := make([]string, 0, len(calls))
		for _, c := range calls {
			got = append(got, c.Method+" "+c.Params.Get("artist")+" - "+c.Params.Get("track"))
		}
		want := []string{"track.love Ann - Song 1", "track.unlove Ann - Song 1"}
		if !slices.Equal(got, want) {
			t.Errorf("calls = %q, want %q", got, want)
		}
	})
}

func TestNowPlaying(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		f.run(t)
		f.s.NowPlaying(f.userID, trackULID(2))
		synctest.Wait()
		calls := f.fake.CallsTo("track.updateNowPlaying")
		if len(calls) != 1 || calls[0].Params.Get("track") != "Song 2" || calls[0].Params.Get("artist") != annName {
			t.Fatalf("now playing calls = %+v", calls)
		}
		if err := f.st.SetLastfmSwitches(t.Context(), f.userID, store.LastfmSwitches{Scrobble: true, Loves: true}); err != nil {
			t.Fatal(err)
		}
		f.s.NowPlaying(f.userID, trackULID(2))
		f.fake.Fail(16) // Not retried.
		synctest.Wait()
		f.s.NowPlaying(f.userID, trackULID(3))
		synctest.Wait()
		if n := len(f.fake.Calls()); n != 1 {
			t.Errorf("calls = %d, want none with the switch off", n)
		}
	})
}

func TestNotConfiguredDoesNothing(t *testing.T) {
	t.Parallel()
	var s *scrobbler.Scrobbler
	if s.Configured() {
		t.Error("nil scrobbler is configured")
	}
	s = scrobbler.New(nil, scrobbler.Options{})
	s.Poke()
	s.NowPlaying(1, "x")
	s.Run(t.Context()) // Returns at once.
	if s.Configured() || s.AuthURL("http://x/") != "" {
		t.Error("a scrobbler without a client says it is set up")
	}
}

func TestNoSecretsInTheLog(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t)
		f.run(t)
		f.fake.Fail(11, 9)
		f.play(t, 1, time.Now())
		synctest.Wait()
		time.Sleep(time.Minute)
		synctest.Wait()
		f.s.NowPlaying(f.userID, trackULID(1))
		synctest.Wait()
		log := f.logs.String()
		if !strings.Contains(log, "reconnecting") {
			t.Errorf("log lacks the failures it should show:\n%s", log)
		}
		if f.fake.HasSecret(log) {
			t.Errorf("log holds a secret:\n%s", log)
		}
	})
}
