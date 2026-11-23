package remote

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// ProjectURL is the contact every request's User-Agent gives, as MusicBrainz
// and Wikimedia ask of every client.
const ProjectURL = "https://github.com/simonhull/kitsune"

// UserAgent is what Kitsune at version calls itself.
func UserAgent(version string) string {
	return fmt.Sprintf("Kitsune/%s ( %s )", cmp.Or(version, "dev"), ProjectURL)
}

// What a request that failed means for the caller. Every error a [Client]
// returns wraps one of these, or the context's.
var (
	// ErrUnavailable means the service could not be reached, answered with a
	// server error, or stayed busy through every retry: ask again later.
	ErrUnavailable = errors.New("the service is unavailable")
	// ErrNotFound means the service has nothing for the request: a 404.
	ErrNotFound = errors.New("the service has none")
	// ErrRefused means the service refused the request's key, or the client: a
	// 401 or 403. Asking again with the same key fails the same way.
	ErrRefused = errors.New("the service refused the key")
	// ErrRejected means the service refused the request itself: any other 4xx.
	ErrRejected = errors.New("the service refused the request")
	// ErrTooLarge means the answer was larger than the client takes.
	ErrTooLarge = errors.New("the answer is too large")
)

// StatusError is an answer with a status the caller can't use. It wraps the
// sentinel the status means, and never holds the URL asked.
type StatusError struct {
	Status     int           // The HTTP status.
	RetryAfter time.Duration // What the service asked to wait, if it said.
	Err        error         // One of the sentinels.
}

// Error says what the status means and the status, never the URL.
func (e *StatusError) Error() string {
	return fmt.Sprintf("%v: %d %s", e.Err, e.Status, http.StatusText(e.Status))
}

// Unwrap returns the sentinel.
func (e *StatusError) Unwrap() error { return e.Err }

// Defaults of [Options].
const (
	defaultRetries = 3
	defaultBackoff = time.Second
	defaultTimeout = 30 * time.Second
	defaultMaxBody = 1 << 20
	// maxRetryAfter caps a wait a service asks for between retries. A longer
	// one ends the request as unavailable, with the wait in its StatusError.
	maxRetryAfter = time.Minute
)

// Options sets up a [Client].
type Options struct {
	// Agent is the User-Agent. "" means [UserAgent] of "dev".
	Agent string
	// HTTP sends the requests. Nil means a client with a 30 s timeout.
	HTTP *http.Client
	// Gate spaces the requests. Nil means none.
	Gate *Gate
	// Retries is how many times a 429 or a 503 is asked again, and Backoff the
	// wait before the first, doubling after, unless the service says how long
	// in Retry-After. 0 means 3 and a second; a negative Retries means none.
	Retries int
	Backoff time.Duration
	// MaxBody caps an answer, in bytes. 0 means 1 MiB.
	MaxBody int64
	// Logger notes retries. Nil discards.
	Logger *slog.Logger
	// Name names the service in the log.
	Name string
}

// Client asks one service for answers through its gate. It is safe for
// concurrent use.
type Client struct {
	agent   string
	http    *http.Client
	gate    *Gate
	retries int
	backoff time.Duration
	maxBody int64
	log     *slog.Logger
	name    string

	mu     sync.Mutex
	flying map[string]*flight // Requests under way, by key: a second asker waits for the first.
}

// flight is one request under way.
type flight struct {
	done chan struct{}
	body []byte
	err  error
}

// New returns a client set up by o.
func New(o Options) *Client { //nolint:gocritic // hugeParam: built once per service, at startup.
	c := &Client{
		agent: cmp.Or(o.Agent, UserAgent("")), http: o.HTTP, gate: o.Gate,
		retries: cmp.Or(o.Retries, defaultRetries), backoff: cmp.Or(o.Backoff, defaultBackoff),
		maxBody: cmp.Or(o.MaxBody, defaultMaxBody), log: o.Logger, name: o.Name,
		flying: map[string]*flight{},
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

// Get returns the body of a GET of u, sending header besides the User-Agent. A
// second Get with the same key while the first is under way waits for its
// answer, so key must say everything that decides the answer, and never a key
// that isn't the caller's. A 200 is the body; anything else is an error
// wrapping one of the sentinels, which never quotes u.
func (c *Client) Get(ctx context.Context, key, u string, header http.Header) ([]byte, error) {
	for {
		body, err := c.share(ctx, key, u, header)
		// A request another asker gave up on is asked again by the next.
		if (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) && ctx.Err() == nil {
			continue
		}
		return body, err
	}
}

// share asks for key, or waits for the same request already under way.
func (c *Client) share(ctx context.Context, key, u string, header http.Header) ([]byte, error) {
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
	f.body, f.err = c.fetch(ctx, u, header)
	c.mu.Lock()
	delete(c.flying, key)
	c.mu.Unlock()
	close(f.done)
	return f.body, f.err
}

// fetch asks for u through the gate, asking again after a 429 or a 503.
func (c *Client) fetch(ctx context.Context, u string, header http.Header) ([]byte, error) {
	wait := c.backoff
	for attempt := 0; ; attempt++ {
		body, err := c.once(ctx, u, header)
		se, busy := errors.AsType[*StatusError](err)
		if !busy || (se.Status != http.StatusTooManyRequests && se.Status != http.StatusServiceUnavailable) {
			return body, err
		}
		if attempt == c.retries || se.RetryAfter > maxRetryAfter {
			return nil, err
		}
		pause := cmp.Or(se.RetryAfter, wait)
		c.log.InfoContext(ctx, "a service is busy: asking again", "service", c.name, "status", se.Status, "after", pause)
		if slept := Sleep(ctx, pause); slept != nil {
			return nil, slept
		}
		wait *= 2
	}
}

// once sends one request.
func (c *Client) once(ctx context.Context, u string, header http.Header) ([]byte, error) {
	if err := c.gate.wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: not a URL", ErrRejected) // url.Parse's error quotes it.
	}
	maps.Copy(req.Header, header)
	req.Header.Set("User-Agent", c.agent)
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ask: %w", context.Cause(ctx))
		}
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, unquoted(err))
	}
	defer func() { _ = resp.Body.Close() }() // Read below, or abandoned.
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, c.maxBody)) // So the connection can be reused.
		return nil, statusError(resp)
	}
	return c.read(ctx, resp.Body)
}

// read reads an answer's body, up to the cap.
func (c *Client) read(ctx context.Context, r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, c.maxBody+1))
	switch {
	case err != nil && ctx.Err() != nil:
		return nil, fmt.Errorf("read: %w", context.Cause(ctx))
	case err != nil:
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, unquoted(err))
	case int64(len(body)) > c.maxBody:
		return nil, fmt.Errorf("%w: over %d bytes", ErrTooLarge, c.maxBody)
	}
	return body, nil
}

// wait waits at a nil gate for nothing.
func (g *Gate) wait(ctx context.Context) error {
	if g == nil {
		return nil
	}
	return g.Wait(ctx)
}

// statusError is the error an answer's status means.
func statusError(resp *http.Response) *StatusError {
	e := &StatusError{Status: resp.StatusCode, RetryAfter: retryAfter(resp.Header.Get("Retry-After"))}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		e.Err = ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		e.Err = ErrRefused
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError:
		e.Err = ErrUnavailable
	default:
		e.Err = ErrRejected
	}
	return e
}

// retryAfter reads a Retry-After of whole seconds, or returns 0.
func retryAfter(v string) time.Duration {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

// unquoted is err without the URL a [url.Error] quotes, which may hold a key.
func unquoted(err error) error {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		return ue.Err
	}
	return err
}
