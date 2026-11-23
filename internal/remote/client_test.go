package remote_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/simonhull/kitsune/internal/remote"
)

// secret is a key that must never appear in an error.
const secret = "k3y-th4t-must-n0t-leak"

// memory answers requests in memory with h, for a synctest bubble.
type memory struct{ h http.Handler }

func (m memory) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	m.h.ServeHTTP(rec, req)
	return rec.Result(), nil
}

func client(h http.HandlerFunc, o *remote.Options) *remote.Client {
	o.HTTP = &http.Client{Transport: memory{h}}
	return remote.New(*o)
}

func TestStatusesAreSentinels(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, remote.ErrNotFound},
		{http.StatusUnauthorized, remote.ErrRefused},
		{http.StatusForbidden, remote.ErrRefused},
		{http.StatusBadRequest, remote.ErrRejected},
		{http.StatusInternalServerError, remote.ErrUnavailable},
		{http.StatusServiceUnavailable, remote.ErrUnavailable},
		{http.StatusTooManyRequests, remote.ErrUnavailable},
	} {
		cl := client(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(c.status) }, &remote.Options{Retries: -1})
		_, err := cl.Get(t.Context(), "k", "http://service.test/a?api_key="+secret, nil)
		if !errors.Is(err, c.want) {
			t.Errorf("%d: %v, want %v", c.status, err, c.want)
		}
		if se, ok := errors.AsType[*remote.StatusError](err); !ok || se.Status != c.status {
			t.Errorf("%d: %v is no StatusError with its status", c.status, err)
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("%d: %q quotes the key", c.status, err)
		}
	}
}

func TestNoErrorQuotesTheURL(t *testing.T) {
	t.Parallel()
	o := remote.Options{Retries: -1}
	o.HTTP = &http.Client{Transport: failing{}}
	_, err := remote.New(o).Get(t.Context(), "k", "http://service.test/x/"+secret+"?k="+secret, nil)
	if !errors.Is(err, remote.ErrUnavailable) || strings.Contains(err.Error(), secret) {
		t.Errorf("err = %v, want unavailable without the URL", err)
	}
}

// failing fails every request as a network would, quoting nothing itself.
type failing struct{}

func (failing) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection refused")
}

func TestBodiesAreCapped(t *testing.T) {
	t.Parallel()
	c := client(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(make([]byte, 101)) }, &remote.Options{MaxBody: 100})
	if _, err := c.Get(t.Context(), "k", "http://service.test/", nil); !errors.Is(err, remote.ErrTooLarge) {
		t.Errorf("err = %v, want too large", err)
	}
	c = client(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(make([]byte, 100)) }, &remote.Options{MaxBody: 100})
	if body, err := c.Get(t.Context(), "k", "http://service.test/", nil); err != nil || len(body) != 100 {
		t.Errorf("at the cap: %d bytes, %v", len(body), err)
	}
}

func TestBusyIsAskedAgainAfterRetryAfter(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		var at []time.Duration
		start := time.Now()
		c := client(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			at = append(at, time.Since(start))
			if got := r.Header.Get("User-Agent"); got != remote.UserAgent("test") {
				t.Errorf("User-Agent %q", got)
			}
			switch len(at) {
			case 1:
				w.Header().Set("Retry-After", "5")
				w.WriteHeader(http.StatusTooManyRequests)
			case 2:
				w.WriteHeader(http.StatusServiceUnavailable)
			default:
				_, _ = w.Write([]byte("ok"))
			}
		}, &remote.Options{Agent: remote.UserAgent("test"), Backoff: 2 * time.Second})
		body, err := c.Get(t.Context(), "k", "http://service.test/", nil)
		if err != nil || string(body) != "ok" {
			t.Fatalf("got %q, %v", body, err)
		}
		want := []time.Duration{0, 5 * time.Second, 9 * time.Second} // Retry-After, then the doubled backoff.
		if len(at) != len(want) || at[1] != want[1] || at[2] != want[2] {
			t.Errorf("asked at %v, want %v", at, want)
		}
	})
}

func TestALongRetryAfterEndsTheRequest(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	c := client(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}, &remote.Options{})
	_, err := c.Get(t.Context(), "k", "http://service.test/", nil)
	se, ok := errors.AsType[*remote.StatusError](err)
	if !ok || se.RetryAfter != time.Hour || calls.Load() != 1 {
		t.Errorf("err = %v after %d calls, want an hour to wait and no retry", err, calls.Load())
	}
}

func TestPeopleGoBeforeBackgroundWork(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		var order []string
		c := client(func(_ http.ResponseWriter, r *http.Request) {
			mu.Lock()
			order = append(order, r.URL.Path)
			mu.Unlock()
		}, &remote.Options{Gate: remote.NewGate(time.Second)})
		if _, err := c.Get(t.Context(), "first", "http://service.test/first", nil); err != nil { // Shuts the gate for a second.
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Go(func() { _, _ = c.Get(remote.Background(t.Context()), "ahead", "http://service.test/ahead", nil) })
		synctest.Wait()
		wg.Go(func() { _, _ = c.Get(t.Context(), "person", "http://service.test/person", nil) })
		wg.Wait()
		if strings.Join(order, " ") != "/first /person /ahead" {
			t.Errorf("order %v, want the person before the work ahead", order)
		}
	})
}

func TestOneKeyIsAskedOnce(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		c := client(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			_, _ = w.Write([]byte("x"))
		}, &remote.Options{Gate: remote.NewGate(time.Second)})
		if _, err := c.Get(t.Context(), "shut", "http://service.test/shut", nil); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				if _, err := c.Get(t.Context(), "same", "http://service.test/same", nil); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		if n := calls.Load(); n != 2 {
			t.Errorf("%d calls, want the same key asked once", n)
		}
	})
}

// A timeout is the service being slow, not another asker giving up: it ends
// the request instead of asking again forever.
func TestATimeoutIsUnavailable(t *testing.T) {
	t.Parallel()
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(slow.Close)
	hc := slow.Client()
	hc.Timeout = 50 * time.Millisecond
	_, err := remote.New(remote.Options{HTTP: hc}).Get(t.Context(), "k", slow.URL, nil)
	if !errors.Is(err, remote.ErrUnavailable) {
		t.Errorf("err = %v, want unavailable", err)
	}
}
