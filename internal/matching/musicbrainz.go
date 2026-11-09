package matching

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SourceMusicBrainz names MusicBrainz as a source.
const SourceMusicBrainz = "musicbrainz"

// MusicBrainz's own roots, the defaults for [MusicBrainzOptions].
const (
	DefaultMusicBrainzURL = "https://musicbrainz.org/ws/2/"
	DefaultCoverURL       = "https://coverartarchive.org/"
	// ProjectURL is the contact MusicBrainz asks every client's User-Agent to give.
	ProjectURL = "https://github.com/simonhull/kitsune"
)

// What a lookup or search that failed means for the caller. Every error the
// client returns wraps one of these, or the context's.
var (
	// ErrUnavailable means the source could not be reached, or stayed busy
	// through every retry: try again later.
	ErrUnavailable = errors.New("the source is unavailable")
	// ErrNotFound means the source has no such release.
	ErrNotFound = errors.New("no such release")
	// ErrRejected means the source refused the request itself: sending it again
	// would fail the same way.
	ErrRejected = errors.New("the source refused the request")
)

// Defaults of [MusicBrainzOptions].
const (
	// defaultInterval is MusicBrainz's limit: one request a second per client.
	defaultInterval = time.Second
	defaultRetries  = 3
	defaultBackoff  = time.Second
	defaultTimeout  = 30 * time.Second
	// cacheFor is how long an answer is kept.
	cacheFor = 24 * time.Hour
	// maxBody caps how much of an answer is read: a box set's tracklist is large.
	maxBody = 8 << 20
	// searchLimit is how many releases a search asks for.
	searchLimit = 10
)

// lookupInc is what a release lookup asks MusicBrainz to include.
const lookupInc = "artist-credits+labels+recordings+release-groups+isrcs+genres"

// Cache keeps a source's answers between requests. Get returns nil and no error
// when it holds nothing for key fetched at or after notBefore.
type Cache interface {
	Get(ctx context.Context, key string, notBefore time.Time) ([]byte, error)
	Put(ctx context.Context, key string, body []byte, at time.Time) error
}

// MusicBrainzOptions sets up a [MusicBrainz] client. The zero value talks to
// MusicBrainz itself, a request a second, keeping nothing.
type MusicBrainzOptions struct {
	// BaseURL is the web service's root, ending in /ws/2/. "" means MusicBrainz.
	BaseURL string
	// CoverURL is the Cover Art Archive's root. "" means the real one.
	CoverURL string
	// Version is Kitsune's, for the User-Agent MusicBrainz asks for.
	Version string
	// HTTP sends the requests. Nil means a client with a 30 s timeout.
	HTTP *http.Client
	// Cache keeps answers for a day. Nil keeps none.
	Cache Cache
	// Interval is the least time between two requests. 0 means a second.
	Interval time.Duration
	// Retries is how many times a 503 is asked again, and Backoff the wait
	// before the first, doubling after: MusicBrainz answers 503 to a client
	// going too fast. 0 means 3 and a second; a negative Retries means none.
	Retries int
	Backoff time.Duration
	// Logger notes retries. Nil discards.
	Logger *slog.Logger
}

// MusicBrainz is a client for MusicBrainz's web service. One serves the whole
// process: its gate is what keeps every request Kitsune makes to one a second.
// It is safe for concurrent use.
type MusicBrainz struct {
	base, covers string
	agent        string
	http         *http.Client
	cache        Cache
	gate         *gate
	retries      int
	backoff      time.Duration
	log          *slog.Logger

	mu     sync.Mutex
	flying map[string]*flight // Requests under way, by key: a second asker waits for the first.
}

// flight is one request under way.
type flight struct {
	done chan struct{}
	body []byte
	err  error
}

// NewMusicBrainz returns a client set up by o.
func NewMusicBrainz(o MusicBrainzOptions) *MusicBrainz { //nolint:gocritic // hugeParam: built once, at startup.
	c := &MusicBrainz{
		base:    withSlash(cmp.Or(o.BaseURL, DefaultMusicBrainzURL)),
		covers:  withSlash(cmp.Or(o.CoverURL, DefaultCoverURL)),
		agent:   fmt.Sprintf("Kitsune/%s ( %s )", cmp.Or(o.Version, "dev"), ProjectURL),
		http:    o.HTTP,
		cache:   o.Cache,
		gate:    newGate(cmp.Or(o.Interval, defaultInterval)),
		retries: cmp.Or(o.Retries, defaultRetries),
		backoff: cmp.Or(o.Backoff, defaultBackoff),
		log:     o.Logger,
		flying:  map[string]*flight{},
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: defaultTimeout}
	}
	if c.retries < 0 {
		c.retries = 0
	}
	if c.log == nil {
		c.log = slog.New(slog.DiscardHandler)
	}
	return c
}

func withSlash(s string) string {
	if strings.HasSuffix(s, "/") {
		return s
	}
	return s + "/"
}

// Name implements [Source].
func (*MusicBrainz) Name() string { return SourceMusicBrainz }

// CoverURL implements [Source]: the release's front cover at the Cover Art
// Archive, about 250 pixels across.
func (c *MusicBrainz) CoverURL(id string) string {
	return c.covers + "release/" + url.PathEscape(id) + "/front-250"
}

// PageURL implements [Source]: the release's page on MusicBrainz.
func (*MusicBrainz) PageURL(id string) string {
	return "https://musicbrainz.org/release/" + url.PathEscape(id)
}

// Release implements [Source]: it looks up the release id with its tracklist,
// labels, release group, ISRCs and genres.
func (c *MusicBrainz) Release(ctx context.Context, id string) (Release, error) {
	if !mbidPattern.MatchString(id) {
		return Release{}, fmt.Errorf("release %q: %w", id, ErrNotFound)
	}
	key := "release/" + id + "?" + url.Values{"inc": {lookupInc}, "fmt": {"json"}}.Encode()
	body, err := c.get(ctx, key)
	if err != nil {
		return Release{}, fmt.Errorf("release %s: %w", id, err)
	}
	var r mbRelease
	if err = json.Unmarshal(body, &r); err != nil {
		return Release{}, fmt.Errorf("release %s: %w: %w", id, ErrUnavailable, err)
	}
	return r.release(true), nil
}

// Search implements [Source]: releases that may be the one q describes, as
// MusicBrainz ranks them, without their tracklists.
func (c *MusicBrainz) Search(ctx context.Context, q *Query) ([]Release, error) {
	lucene := q.lucene()
	if lucene == "" {
		return nil, fmt.Errorf("search: %w: nothing to search for", ErrRejected)
	}
	key := "release?" + url.Values{"query": {lucene}, "limit": {strconv.Itoa(searchLimit)}, "fmt": {"json"}}.Encode()
	body, err := c.get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	var found struct {
		Releases []mbRelease `json:"releases"`
	}
	if err = json.Unmarshal(body, &found); err != nil {
		return nil, fmt.Errorf("search: %w: %w", ErrUnavailable, err)
	}
	out := make([]Release, len(found.Releases))
	for i := range found.Releases {
		out[i] = found.Releases[i].release(false)
	}
	return out, nil
}

// get returns the answer to key, a path and query under the root: from the
// cache while it is fresh, from a request already under way, or asked through
// the gate.
func (c *MusicBrainz) get(ctx context.Context, key string) ([]byte, error) {
	if c.cache != nil {
		body, err := c.cache.Get(ctx, key, time.Now().Add(-cacheFor))
		if err != nil {
			c.log.WarnContext(ctx, "reading a kept MusicBrainz answer failed", "err", err)
		}
		if body != nil {
			return body, nil
		}
	}
	for {
		body, err := c.share(ctx, key)
		// A request another asker gave up on is asked again by the next.
		if (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) && ctx.Err() == nil {
			continue
		}
		return body, err
	}
}

// share asks for key, or waits for the same request already under way.
func (c *MusicBrainz) share(ctx context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	if f, ok := c.flying[key]; ok {
		c.mu.Unlock()
		select {
		case <-f.done:
			return f.body, f.err
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for the same request: %w", context.Cause(ctx))
		}
	}
	f := &flight{done: make(chan struct{})}
	c.flying[key] = f
	c.mu.Unlock()
	f.body, f.err = c.fetch(ctx, key)
	c.mu.Lock()
	delete(c.flying, key)
	c.mu.Unlock()
	close(f.done)
	if f.err == nil && c.cache != nil {
		if err := c.cache.Put(context.WithoutCancel(ctx), key, f.body, time.Now()); err != nil {
			c.log.WarnContext(ctx, "keeping a MusicBrainz answer failed", "err", err)
		}
	}
	return f.body, f.err
}

// fetch asks for key through the gate, asking again after a 503.
func (c *MusicBrainz) fetch(ctx context.Context, key string) ([]byte, error) {
	wait := c.backoff
	for attempt := 0; ; attempt++ {
		body, retryAfter, err := c.once(ctx, key)
		if !errors.Is(err, errBusy) {
			return body, err
		}
		if attempt == c.retries {
			return nil, fmt.Errorf("%w: still busy after %d tries", ErrUnavailable, attempt+1)
		}
		pause := cmp.Or(retryAfter, wait)
		c.log.InfoContext(ctx, "MusicBrainz is busy: asking again", "after", pause)
		if slept := sleep(ctx, pause, nil); slept != nil {
			return nil, slept
		}
		wait *= 2
	}
}

// errBusy is a 503: MusicBrainz asks for slower requests.
var errBusy = errors.New("busy")

// once sends one request. A 503 is [errBusy] with the wait the source asked for, if any.
func (c *MusicBrainz) once(ctx context.Context, key string) ([]byte, time.Duration, error) {
	if err := c.gate.wait(ctx); err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+key, http.NoBody)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrRejected, err)
	}
	req.Header.Set("User-Agent", c.agent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, fmt.Errorf("ask: %w", context.Cause(ctx))
		}
		return nil, 0, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }() // Read to the end below, or abandoned.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	switch {
	case resp.StatusCode == http.StatusServiceUnavailable:
		return nil, retryAfter(resp.Header.Get("Retry-After")), errBusy
	case err != nil:
		return nil, 0, fmt.Errorf("%w: %w", ErrUnavailable, err)
	case resp.StatusCode == http.StatusOK:
		return body, 0, nil
	case resp.StatusCode == http.StatusNotFound:
		return nil, 0, ErrNotFound
	case resp.StatusCode >= http.StatusInternalServerError:
		return nil, 0, fmt.Errorf("%w: %s", ErrUnavailable, resp.Status)
	}
	return nil, 0, fmt.Errorf("%w: %s", ErrRejected, resp.Status)
}

// maxRetryAfter caps a wait the source asks for.
const maxRetryAfter = 30 * time.Second

// retryAfter reads a Retry-After of whole seconds, or returns 0.
func retryAfter(v string) time.Duration {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0
	}
	return min(time.Duration(n)*time.Second, maxRetryAfter)
}
