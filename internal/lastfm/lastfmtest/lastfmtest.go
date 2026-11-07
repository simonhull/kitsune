// Package lastfmtest is a fake Last.fm for tests: the API, which checks every
// signature, and the page where a user approves an application. Nothing in a
// test ever talks to the real Last.fm.
package lastfmtest

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/simonhull/kitsune/internal/lastfm"
)

// The application credentials the fake accepts.
const (
	APIKey = "test-api-key"
	Secret = "test-shared-secret"
)

// Last.fm error codes the fake answers with.
const (
	codeInvalidMethod    = 3
	codeInvalidToken     = 4
	codeInvalidParams    = 6
	codeInvalidSession   = 9
	codeInvalidAPIKey    = 10
	codeInvalidSignature = 13
)

// minStatus is the smallest value [Server.Fail] takes as an HTTP status.
const minStatus = 100

// Call is one API call the fake answered.
type Call struct {
	Method string
	Params url.Values
	// OK reports whether the call succeeded.
	OK bool
}

// Server is a running fake. Its methods are safe for concurrent use.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	calls    []Call
	tokens   map[string]lastfm.Session // Approved, unused tokens.
	sessions map[string]string         // Valid session keys, to user names.
	issued   map[string]bool           // Every session key ever handed out.
	failures []int                     // Error codes or HTTP statuses for the next calls, in order.
	ignore   map[string]bool           // Artists whose scrobbles are ignored.
	next     int                       // Tokens handed out by the approval page.
}

// New starts a fake and stops it when the test ends.
func New(tb testing.TB) *Server {
	tb.Helper()
	s := &Server{
		tokens: map[string]lastfm.Session{}, sessions: map[string]string{},
		issued: map[string]bool{}, ignore: map[string]bool{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /2.0/", s.api)
	mux.HandleFunc("GET /api/auth/", s.auth)
	s.Server = httptest.NewServer(mux)
	tb.Cleanup(s.Close)
	return s
}

// Client returns a client for the fake.
func (s *Server) Client() *lastfm.Client {
	c := lastfm.New(APIKey, Secret)
	s.Point(c)
	return c
}

// Point aims c at the fake.
func (s *Server) Point(c *lastfm.Client) {
	c.APIBase = s.URL + "/2.0/"
	c.AuthBase = s.URL + "/api/auth/"
	c.HTTP = s.Server.Client()
}

// Approve makes token exchangeable, once, for a session of user name with key.
func (s *Server) Approve(token, name, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = lastfm.Session{Name: name, Key: key}
	s.issued[key] = true
}

// AddSession makes key a valid session key for user name.
func (s *Server) AddSession(key, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[key] = name
	s.issued[key] = true
}

// Revoke makes key invalid, as removing access on last.fm does.
func (s *Server) Revoke(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, key)
}

// Fail makes the next calls fail, one per value: a value of 100 or more is
// an HTTP status with no body, anything else a Last.fm error code.
func (s *Server) Fail(codes ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, codes...)
}

// Ignore makes the fake ignore scrobbles of artist.
func (s *Server) Ignore(artist string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ignore[artist] = true
}

// Calls returns the calls answered so far, failed ones included.
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Call(nil), s.calls...)
}

// CallsTo returns the calls to method answered without an error.
func (s *Server) CallsTo(method string) []Call {
	var out []Call
	for _, c := range s.Calls() {
		if c.Method == method && c.OK {
			out = append(out, c)
		}
	}
	return out
}

// Scrobbles returns the plays in the successful track.scrobble calls, in the
// order they arrived, as "artist - title @ timestamp".
func (s *Server) Scrobbles() []string {
	var out []string
	for _, c := range s.CallsTo("track.scrobble") {
		for i := 0; c.Params.Has(indexed("artist", i)); i++ {
			out = append(out, c.Params.Get(indexed("artist", i))+" - "+c.Params.Get(indexed("track", i))+
				" @ "+c.Params.Get(indexed("timestamp", i)))
		}
	}
	return out
}

// HasSecret reports whether text holds the shared secret or any session key
// the fake has handed out.
func (s *Server) HasSecret(text string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.Contains(text, Secret) {
		return true
	}
	for key := range s.issued {
		if strings.Contains(text, key) {
			return true
		}
	}
	return false
}

// auth is the approval page: it approves at once and sends the browser back
// to the callback with a new token, as saying yes on last.fm does.
func (s *Server) auth(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cb, err := url.Parse(q.Get("cb"))
	if q.Get("api_key") != APIKey || err != nil || cb.Scheme == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.next++
	token := "token-" + strconv.Itoa(s.next)
	s.tokens[token] = lastfm.Session{Name: "fakeuser", Key: "sk-" + token}
	s.issued["sk-"+token] = true
	s.mu.Unlock()
	cbq := cb.Query()
	cbq.Set("token", token)
	cb.RawQuery = cbq.Encode()
	http.Redirect(w, r, cb.String(), http.StatusFound) //nolint:gosec // G710: a fake for tests; the callback is the client's own.
}

// api answers one method call.
func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	params := r.PostForm
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.answer(params)
	s.calls = append(s.calls, Call{Method: params.Get("method"), Params: params, OK: a.status == http.StatusOK && a.code == 0})
	if a.body == nil {
		w.WriteHeader(a.status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(a.status)
	_ = json.MarshalWrite(w, a.body)
}

// answer is a response from the fake.
type answer struct {
	status int
	code   int // A Last.fm error code, or 0.
	body   any
}

// fail is an error response with a Last.fm code.
func fail(status, code int, message string) answer {
	return answer{status: status, code: code, body: map[string]any{"error": code, "message": message}}
}

// ok is a successful response.
func ok(body any) answer { return answer{status: http.StatusOK, body: body} }

// answer decides the response to params. It runs with s.mu held.
func (s *Server) answer(params url.Values) answer {
	if len(s.failures) > 0 {
		code := s.failures[0]
		s.failures = s.failures[1:]
		if code >= minStatus {
			return answer{status: code}
		}
		return fail(http.StatusOK, code, "injected failure")
	}
	if a, bad := s.check(params); bad {
		return a
	}
	switch params.Get("method") {
	case "auth.getSession":
		return s.session(params.Get("token"))
	case "track.updateNowPlaying", "track.love", "track.unlove":
		if params.Get("artist") == "" || params.Get("track") == "" {
			return fail(http.StatusBadRequest, codeInvalidParams, "artist and track are required")
		}
		return ok(map[string]any{})
	case "track.scrobble":
		return s.scrobble(params)
	default:
		return fail(http.StatusBadRequest, codeInvalidMethod, "Invalid Method")
	}
}

// check rejects a call that is not JSON, has the wrong key or signature, or
// needs a session it doesn't have. It runs with s.mu held.
func (s *Server) check(params url.Values) (answer, bool) {
	signed := url.Values{}
	for k, vs := range params {
		if k != "api_sig" {
			signed[k] = vs
		}
	}
	switch {
	case params.Get("format") != "json":
		return fail(http.StatusBadRequest, codeInvalidParams, "want format=json"), true
	case params.Get("api_key") != APIKey:
		return fail(http.StatusForbidden, codeInvalidAPIKey, "Invalid API key"), true
	case params.Get("api_sig") != lastfm.Sign(signed, Secret):
		return fail(http.StatusForbidden, codeInvalidSignature, "Invalid method signature supplied"), true
	}
	if params.Get("method") == "auth.getSession" {
		return answer{}, false
	}
	if _, valid := s.sessions[params.Get("sk")]; !valid {
		return fail(http.StatusForbidden, codeInvalidSession, "Invalid session key - Please re-authenticate"), true
	}
	return answer{}, false
}

// session answers auth.getSession, using up token. It runs with s.mu held.
func (s *Server) session(token string) answer {
	sess, found := s.tokens[token]
	if !found {
		return fail(http.StatusForbidden, codeInvalidToken, "Invalid authentication token supplied")
	}
	delete(s.tokens, token)
	s.sessions[sess.Key] = sess.Name
	return ok(map[string]any{"session": map[string]any{"name": sess.Name, "key": sess.Key, "subscriber": 0}})
}

// scrobble answers track.scrobble, writing a single result as a bare object
// the way Last.fm does. It runs with s.mu held.
func (s *Server) scrobble(params url.Values) answer {
	var results []any
	accepted, ignored := 0, 0
	for i := 0; params.Has(indexed("artist", i)); i++ {
		artist := params.Get(indexed("artist", i))
		if params.Get(indexed("track", i)) == "" || params.Get(indexed("timestamp", i)) == "" {
			return fail(http.StatusBadRequest, codeInvalidParams, "a scrobble is incomplete")
		}
		code, text := "0", ""
		if s.ignore[artist] {
			code, text = "1", "Artist was ignored"
			ignored++
		} else {
			accepted++
		}
		results = append(results, map[string]any{
			"artist":         map[string]any{"#text": artist},
			"ignoredMessage": map[string]any{"code": code, "#text": text},
		})
	}
	if len(results) == 0 {
		return fail(http.StatusBadRequest, codeInvalidParams, "no scrobbles")
	}
	var list any = results
	if len(results) == 1 {
		list = results[0]
	}
	return ok(map[string]any{"scrobbles": map[string]any{
		"scrobble": list,
		"@attr":    map[string]any{"accepted": accepted, "ignored": ignored},
	}})
}

// indexed is a scrobble parameter's name, such as "artist[2]".
func indexed(name string, i int) string { return name + "[" + strconv.Itoa(i) + "]" }
