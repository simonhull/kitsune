package matching_test

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/matching/musicbrainztest"
)

// memoryCache is a [matching.Cache] in memory.
type memoryCache struct {
	mu   sync.Mutex
	kept map[string]memoryEntry
}

type memoryEntry struct {
	body []byte
	at   time.Time
}

func (c *memoryCache) Get(_ context.Context, key string, notBefore time.Time) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.kept[key]; ok && !e.at.Before(notBefore) {
		return e.body, nil
	}
	return nil, nil
}

func (c *memoryCache) Put(_ context.Context, key string, body []byte, at time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.kept == nil {
		c.kept = map[string]memoryEntry{}
	}
	c.kept[key] = memoryEntry{body: body, at: at}
	return nil
}

// wantRelease checks a release read back through the stand-in against what it held.
func wantRelease(t *testing.T, got, want *matching.Release) {
	t.Helper()
	if got.Title != want.Title || len(got.Media) != len(want.Media) || got.Media[1].Title != want.Media[1].Title {
		t.Fatalf("release = %+v", got)
	}
	if !slices.EqualFunc(got.Media[1].Tracks, want.Media[1].Tracks, func(a, b matching.ReleaseTrack) bool {
		return a.ID == b.ID && a.Length == b.Length && a.Title == b.Title && slices.Equal(a.ISRCs, b.ISRCs)
	}) {
		t.Errorf("tracks = %+v, want %+v", got.Media[1].Tracks, want.Media[1].Tracks)
	}
	if !slices.Equal(got.Labels, want.Labels) || !slices.Equal(got.Genres, want.Genres) {
		t.Errorf("labels %+v and genres %q, want %+v and %q", got.Labels, got.Genres, want.Labels, want.Genres)
	}
}

func TestClientNamesItselfAndReadsAReleaseOnce(t *testing.T) {
	t.Parallel()
	cd := glasshouseCD()
	mb := musicbrainztest.New(t, cd)
	o := mb.Options()
	o.Version, o.Cache = "v1.2.3", &memoryCache{}
	c := matching.NewMusicBrainz(o)
	for range 2 {
		got, err := c.Release(t.Context(), cd.ID)
		if err != nil {
			t.Fatal(err)
		}
		wantRelease(t, &got, &cd)
	}
	calls := mb.Calls()
	if len(calls) != 1 {
		t.Fatalf("%d calls, want one: the second read comes from the cache", len(calls))
	}
	if want := "Kitsune/v1.2.3 ( https://github.com/simonhull/kitsune )"; calls[0].UserAgent != want {
		t.Errorf("User-Agent %q, want %q", calls[0].UserAgent, want)
	}
	if calls[0].Path != "release/"+cd.ID {
		t.Errorf("path %q", calls[0].Path)
	}
}

func TestClientMeetsFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		statuses []int
		want     error // Nil means it succeeds.
		calls    int
	}{
		{"busy, then fine", []int{http.StatusServiceUnavailable, http.StatusServiceUnavailable}, nil, 3},
		{"busy throughout", []int{503, 503, 503, 503}, matching.ErrUnavailable, 4},
		{"broken", []int{http.StatusInternalServerError}, matching.ErrUnavailable, 1},
		{"refused", []int{http.StatusBadRequest}, matching.ErrRejected, 1},
		{"not there", []int{http.StatusNotFound}, matching.ErrNotFound, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cd := glasshouseCD()
			mb := musicbrainztest.NewInMemory(cd)
			mb.Fail("", tt.statuses...)
			_, err := matching.NewMusicBrainz(mb.Options()).Release(t.Context(), cd.ID)
			if tt.want == nil && err != nil || tt.want != nil && !errorIs(err, tt.want) {
				t.Errorf("Release = %v, want %v", err, tt.want)
			}
			if n := len(mb.Calls()); n != tt.calls {
				t.Errorf("%d calls, want %d", n, tt.calls)
			}
		})
	}
}

func TestClientWaitsASecondBetweenRequests(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cd, vinyl := glasshouseCD(), glasshouseVinyl()
		mb := musicbrainztest.NewInMemory(cd, vinyl)
		o := mb.Options()
		o.Interval, o.Backoff = 0, 0 // MusicBrainz's own: a second, and a second's backoff.
		c := matching.NewMusicBrainz(o)
		start := time.Now()
		for _, id := range []string{cd.ID, vinyl.ID} {
			if _, err := c.Release(t.Context(), id); err != nil {
				t.Fatal(err)
			}
		}
		// A 503 asks again after its Retry-After, through the gate.
		mb.Fail("3", http.StatusServiceUnavailable)
		if _, err := c.Search(t.Context(), &matching.Query{Album: glasshouse}); err != nil {
			t.Fatal(err)
		}
		calls := mb.Calls()
		at := make([]time.Duration, len(calls))
		for i, call := range calls {
			at[i] = call.At.Sub(start)
		}
		want := []time.Duration{0, time.Second, 2 * time.Second, 5 * time.Second}
		if len(at) != len(want) {
			t.Fatalf("calls at %v, want %v", at, want)
		}
		for i := range want {
			if at[i] != want[i] {
				t.Errorf("call %d at %v, want %v", i, at[i], want[i])
			}
		}
	})
}

func TestPeopleGoBeforePrefetch(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		releases := []matching.Release{glasshouseCD(), glasshouseVinyl(), glasshouseDeluxe()}
		mb := musicbrainztest.NewInMemory(releases...)
		o := mb.Options()
		o.Interval = time.Second
		c := matching.NewMusicBrainz(o)
		if _, err := c.Release(t.Context(), releases[0].ID); err != nil { // The gate is shut for a second.
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Go(func() { _, _ = c.Release(matching.Background(t.Context()), releases[1].ID) })
		synctest.Wait() // The prefetch waits first.
		wg.Go(func() { _, _ = c.Release(t.Context(), releases[2].ID) })
		wg.Wait()
		calls := mb.Calls()
		if len(calls) != 3 || calls[1].Path != "release/"+releases[2].ID || calls[2].Path != "release/"+releases[1].ID {
			t.Errorf("calls %+v, want the person's before the prefetch", calls)
		}
	})
}

func TestTwoAskersShareARequest(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cd := glasshouseCD()
		mb := musicbrainztest.NewInMemory(cd)
		o := mb.Options()
		o.Interval = time.Second
		c := matching.NewMusicBrainz(o)
		if _, err := c.Search(t.Context(), &matching.Query{Album: "x"}); err != nil { // Shut the gate.
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				if _, err := c.Release(t.Context(), cd.ID); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		if n := len(mb.Calls()); n != 2 {
			t.Errorf("%d calls, want the release asked for once", n)
		}
	})
}

func TestACanceledAskerLeavesTheRequestToTheNext(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cd := glasshouseCD()
		mb := musicbrainztest.NewInMemory(cd)
		o := mb.Options()
		o.Interval = time.Second
		c := matching.NewMusicBrainz(o)
		if _, err := c.Search(t.Context(), &matching.Query{Album: "x"}); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		var wg sync.WaitGroup
		wg.Go(func() {
			if _, err := c.Release(ctx, cd.ID); !errorIs(err, context.Canceled) {
				t.Errorf("canceled asker got %v", err)
			}
		})
		synctest.Wait()
		wg.Go(func() {
			if _, err := c.Release(t.Context(), cd.ID); err != nil {
				t.Errorf("second asker got %v", err)
			}
		})
		synctest.Wait()
		cancel()
		wg.Wait()
	})
}

func TestSearchQueries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		q    matching.Query
		want string
	}{
		{"album and artist", matching.Query{Album: "Glasshouse", Artist: lowTide}, `release:"Glasshouse" AND artist:"Low Tide Choir"`},
		{"quotes", matching.Query{Album: `Say "Hi" \ Bye`}, `release:"Say \"Hi\" \\ Bye"`},
		{"a barcode", matching.TermsQuery("", "", " 5060000000417 "), `barcode:"5060000000417"`},
		{"a catalog number", matching.TermsQuery(lowTide, "", "SALT 041"), `artist:"Low Tide Choir" AND catno:"SALT 041"`},
		{"a short number is a catalog number", matching.TermsQuery("", "", "1234"), `catno:"1234"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mb := musicbrainztest.NewInMemory()
			if _, err := matching.NewMusicBrainz(mb.Options()).Search(t.Context(), &tt.q); err != nil {
				t.Fatal(err)
			}
			if got := mb.Calls()[0].Query; got != tt.want {
				t.Errorf("query %s, want %s", got, tt.want)
			}
		})
	}
	mb := musicbrainztest.NewInMemory()
	if _, err := matching.NewMusicBrainz(mb.Options()).Search(t.Context(), &matching.Query{}); !errorIs(err, matching.ErrRejected) {
		t.Errorf("an empty search = %v, want ErrRejected", err)
	}
}
