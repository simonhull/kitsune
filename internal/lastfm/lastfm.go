// Package lastfm is a small client for the Last.fm API: web authentication,
// now playing, scrobbles and loves. It knows nothing of the store; callers
// pass tracks and session keys in.
//
// Every call is a signed POST, so neither the shared secret nor a session key
// is ever part of a URL, and no error the package returns contains either.
package lastfm

import (
	"cmp"
	"context"
	"crypto/md5" //nolint:gosec // G501: Last.fm's request signatures are MD5; the API allows nothing else.
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Last.fm's own endpoints, the defaults for [Client.APIBase] and [Client.AuthBase].
const (
	DefaultAPIBase  = "https://ws.audioscrobbler.com/2.0/"
	DefaultAuthBase = "https://www.last.fm/api/auth/"
)

// MaxScrobbles is the most scrobbles one [Client.Scrobble] call takes.
const MaxScrobbles = 50

// maxBody caps how much of a response is read.
const maxBody = 1 << 20

// defaultTimeout bounds a call when the context does not.
const defaultTimeout = 30 * time.Second

// Last.fm error codes the client tells apart.
const (
	codeInvalidSession = 9
	codeOffline        = 11
	codeTemporary      = 16
	codeRateLimited    = 29
)

// What a failed call means for the caller. Every error a call returns wraps
// exactly one of these.
var (
	// ErrTemporary means Last.fm is unreachable or busy: try again later.
	ErrTemporary = errors.New("last.fm is unavailable")
	// ErrSession means Last.fm no longer accepts the session key: the user
	// must connect again.
	ErrSession = errors.New("last.fm rejected the session")
	// ErrRejected means Last.fm refused the request itself: sending it again
	// would fail the same way.
	ErrRejected = errors.New("last.fm rejected the request")
)

// errTooMany reports a scrobble batch over [MaxScrobbles].
var errTooMany = errors.New("too many scrobbles in one call")

// Error is an error Last.fm answered with.
type Error struct {
	// Method is the API method called, such as "track.scrobble".
	Method string
	// Code is Last.fm's error code, or 0 for an HTTP failure without one.
	Code int
	// Message is Last.fm's description, or the HTTP status.
	Message string
	kind    error
}

func (e *Error) Error() string {
	if e.Code == 0 {
		return fmt.Sprintf("last.fm %s: %s", e.Method, e.Message)
	}
	return fmt.Sprintf("last.fm %s: error %d: %s", e.Method, e.Code, e.Message)
}

// Unwrap returns [ErrTemporary], [ErrSession] or [ErrRejected].
func (e *Error) Unwrap() error { return e.kind }

// kindOf maps a Last.fm error code to what it means.
func kindOf(code int) error {
	switch code {
	case codeOffline, codeTemporary, codeRateLimited:
		return ErrTemporary
	case codeInvalidSession:
		return ErrSession
	default:
		return ErrRejected
	}
}

// Client calls the Last.fm API with one application's key and secret. Its
// zero value is not usable; make one with [New]. A Client is safe for
// concurrent use once its fields are set.
type Client struct {
	// APIBase is the API's root URL; tests point it at a fake.
	APIBase string
	// AuthBase is the root URL of the page where a user approves the
	// application.
	AuthBase string
	// HTTP sends the requests.
	HTTP *http.Client

	apiKey string
	secret string
}

// New returns a client for the application with apiKey and secret, talking
// to Last.fm itself.
func New(apiKey, secret string) *Client {
	return &Client{
		APIBase:  DefaultAPIBase,
		AuthBase: DefaultAuthBase,
		HTTP:     &http.Client{Timeout: defaultTimeout},
		apiKey:   apiKey,
		secret:   secret,
	}
}

// AuthURL returns the page to send a user to so they can approve the
// application. Last.fm sends them back to callback with a token for
// [Client.Session].
func (c *Client) AuthURL(callback string) string {
	q := url.Values{"api_key": {c.apiKey}, "cb": {callback}}
	return c.AuthBase + "?" + q.Encode()
}

// Session is a user's approval of the application.
type Session struct {
	// Name is the user's Last.fm name.
	Name string
	// Key signs calls made for the user. It does not expire, and is as
	// sensitive as a password.
	Key string
}

// Session exchanges the token Last.fm sent to the callback for a session.
//
// # Errors
//
// The error wraps [ErrTemporary], or [ErrRejected] for a token that is
// unknown, used or not approved.
func (c *Client) Session(ctx context.Context, token string) (Session, error) {
	var resp struct {
		Session struct {
			Name string `json:"name"`
			Key  string `json:"key"`
		} `json:"session"`
	}
	const method = "auth.getSession"
	if err := c.call(ctx, method, url.Values{"token": {token}}, &resp); err != nil {
		return Session{}, err
	}
	if resp.Session.Key == "" || resp.Session.Name == "" {
		return Session{}, &Error{Method: method, Message: "no session in the response", kind: ErrRejected}
	}
	return Session{Name: resp.Session.Name, Key: resp.Session.Key}, nil
}

// Track is what Last.fm is told about a song.
type Track struct {
	Artist string
	Title  string
	// Album, AlbumArtist and MBID are left out when empty.
	Album       string
	AlbumArtist string
	MBID        string
	// Duration and Number are left out when zero.
	Duration time.Duration
	Number   int
}

// params sets t's fields in v, each name passed through key.
func (t *Track) params(v url.Values, key func(string) string) {
	set := func(name, value string) {
		if value != "" {
			v.Set(key(name), value)
		}
	}
	set("artist", t.Artist)
	set("track", t.Title)
	set("album", t.Album)
	set("albumArtist", t.AlbumArtist)
	set("mbid", t.MBID)
	if s := int(t.Duration / time.Second); s > 0 {
		set("duration", strconv.Itoa(s))
	}
	if t.Number > 0 {
		set("trackNumber", strconv.Itoa(t.Number))
	}
}

// NowPlaying tells Last.fm that the user with session key sk has started t.
//
// # Errors
//
// The error wraps [ErrTemporary], [ErrSession] or [ErrRejected].
func (c *Client) NowPlaying(ctx context.Context, sk string, t *Track) error {
	v := url.Values{"sk": {sk}}
	t.params(v, func(name string) string { return name })
	return c.call(ctx, "track.updateNowPlaying", v, nil)
}

// Scrobble is one play of a track.
type Scrobble struct {
	Track

	// At is when the play started.
	At time.Time
}

// Ignored is a scrobble Last.fm accepted the call for but did not keep, such
// as one for an artist it won't take. Sending it again would be ignored too.
type Ignored struct {
	// Index is the scrobble's place in the batch.
	Index int
	// Code is Last.fm's reason code; Message its description.
	Code    int
	Message string
}

// Scrobble submits up to [MaxScrobbles] plays, in order, for the user with
// session key sk. It reports the plays Last.fm ignored; the rest are kept.
//
// # Errors
//
// The error wraps [ErrTemporary], [ErrSession] or [ErrRejected]. A batch
// over [MaxScrobbles] is rejected without a call.
func (c *Client) Scrobble(ctx context.Context, sk string, plays []Scrobble) ([]Ignored, error) {
	const method = "track.scrobble"
	if len(plays) > MaxScrobbles {
		return nil, &Error{Method: method, Message: errTooMany.Error(), kind: errors.Join(ErrRejected, errTooMany)}
	}
	if len(plays) == 0 {
		return nil, nil
	}
	v := url.Values{"sk": {sk}}
	for i := range plays {
		p := &plays[i]
		suffix := "[" + strconv.Itoa(i) + "]"
		p.params(v, func(name string) string { return name + suffix })
		v.Set("timestamp"+suffix, strconv.FormatInt(p.At.Unix(), 10))
	}
	var resp struct {
		Scrobbles struct {
			Scrobble jsonList[scrobbleResult] `json:"scrobble"`
		} `json:"scrobbles"`
	}
	if err := c.call(ctx, method, v, &resp); err != nil {
		return nil, err
	}
	var ignored []Ignored
	for i, r := range resp.Scrobbles.Scrobble {
		code, err := strconv.Atoi(cmp.Or(r.IgnoredMessage.Code, "0"))
		if err != nil || code != 0 {
			ignored = append(ignored, Ignored{Index: i, Code: code, Message: r.IgnoredMessage.Text})
		}
	}
	return ignored, nil
}

// scrobbleResult is one scrobble in track.scrobble's response.
type scrobbleResult struct {
	IgnoredMessage struct {
		Code string `json:"code"`
		Text string `json:"#text"`
	} `json:"ignoredMessage"`
}

// jsonList decodes a list that Last.fm writes as a bare object when it holds
// one item.
type jsonList[T any] []T

// UnmarshalJSON accepts an array of T or a single T.
func (l *jsonList[T]) UnmarshalJSON(b []byte) error {
	if s := strings.TrimSpace(string(b)); strings.HasPrefix(s, "[") {
		var items []T
		if err := json.Unmarshal(b, &items); err != nil {
			return fmt.Errorf("decode list: %w", err)
		}
		*l = items
		return nil
	}
	var item T
	if err := json.Unmarshal(b, &item); err != nil {
		return fmt.Errorf("decode list item: %w", err)
	}
	*l = jsonList[T]{item}
	return nil
}

// Love marks a track as loved for the user with session key sk.
//
// # Errors
//
// The error wraps [ErrTemporary], [ErrSession] or [ErrRejected].
func (c *Client) Love(ctx context.Context, sk, artist, title string) error {
	return c.call(ctx, "track.love", url.Values{"sk": {sk}, "artist": {artist}, "track": {title}}, nil)
}

// Unlove takes a love back.
//
// # Errors
//
// The error wraps [ErrTemporary], [ErrSession] or [ErrRejected].
func (c *Client) Unlove(ctx context.Context, sk, artist, title string) error {
	return c.call(ctx, "track.unlove", url.Values{"sk": {sk}, "artist": {artist}, "track": {title}}, nil)
}

// Sign returns Last.fm's signature of params: the MD5 of every name and
// value, sorted by name, then the secret. format and callback are not signed.
func Sign(params url.Values, secret string) string {
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(params)) {
		if name == "format" || name == "callback" {
			continue
		}
		b.WriteString(name)
		b.WriteString(params.Get(name))
	}
	b.WriteString(secret)
	sum := md5.Sum([]byte(b.String())) //nolint:gosec // G401: Last.fm's request signatures are MD5; the API allows nothing else.
	return hex.EncodeToString(sum[:])
}

// call posts a signed method call and decodes the response into out, which
// may be nil.
func (c *Client) call(ctx context.Context, method string, params url.Values, out any) error {
	params.Set("method", method)
	params.Set("api_key", c.apiKey)
	params.Set("api_sig", Sign(params, c.secret))
	params.Set("format", "json")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIBase, strings.NewReader(params.Encode()))
	if err != nil {
		return &Error{Method: method, Message: "bad API URL", kind: ErrRejected}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		// The error names only the base URL: parameters travel in the body.
		return &Error{Method: method, Message: err.Error(), kind: ErrTemporary}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return &Error{Method: method, Message: "read response: " + err.Error(), kind: ErrTemporary}
	}
	var apiErr struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &apiErr) == nil && apiErr.Error != 0 {
		return &Error{Method: method, Code: apiErr.Error, Message: apiErr.Message, kind: kindOf(apiErr.Error)}
	}
	switch {
	case resp.StatusCode >= http.StatusInternalServerError, resp.StatusCode == http.StatusTooManyRequests:
		return &Error{Method: method, Message: resp.Status, kind: ErrTemporary}
	case resp.StatusCode != http.StatusOK:
		return &Error{Method: method, Message: resp.Status, kind: ErrRejected}
	case out == nil:
		return nil
	}
	if json.Unmarshal(body, out) != nil {
		// Something other than Last.fm answered, such as a proxy's page.
		return &Error{Method: method, Message: "unreadable response", kind: ErrTemporary}
	}
	return nil
}
