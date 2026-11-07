package server

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// lastfmStateCookie holds the one-time value that ties Last.fm's callback
	// to the connect that started it.
	lastfmStateCookie = "kitsune_lastfm_state"
	// lastfmStateLife is how long a connect may take, last.fm included.
	lastfmStateLife = 10 * time.Minute
	// lastfmCallbackPath is where last.fm sends the browser back, under the API.
	lastfmCallbackPath = "/lastfm/callback"
	// youPath is the client's You page, where the callback lands.
	youPath = "/you"
)

// How a connect ended, as the callback tells the You page.
const (
	lastfmConnected = "connected"
	lastfmRefused   = "refused"
	lastfmFailed    = "failed"
)

// lastfmStates holds each user's pending connect: the state value the
// callback must bring back, and when it stops counting. A user has at most
// one; a new connect replaces it.
type lastfmStates struct {
	mu     sync.Mutex
	byUser map[int64]pendingState
}

type pendingState struct {
	value   string
	expires time.Time
}

// begin starts a connect for userID and returns its state value.
func (p *lastfmStates) begin(userID int64, now time.Time) string {
	value := rand.Text()
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, st := range p.byUser {
		if !now.Before(st.expires) {
			delete(p.byUser, id)
		}
	}
	p.byUser[userID] = pendingState{value: value, expires: now.Add(lastfmStateLife)}
	return value
}

// finish uses up userID's pending connect and reports whether both values
// match it and it has not expired.
func (p *lastfmStates) finish(userID int64, param, cookie string, now time.Time) bool {
	p.mu.Lock()
	st, ok := p.byUser[userID]
	delete(p.byUser, userID)
	p.mu.Unlock()
	return ok && now.Before(st.expires) && param != "" &&
		subtle.ConstantTimeCompare([]byte(st.value), []byte(param)) == 1 &&
		subtle.ConstantTimeCompare([]byte(st.value), []byte(cookie)) == 1
}

// getLastfm reports the caller's Last.fm connection.
func (s *Server) getLastfm(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	status, err := s.lastfmStatus(r, user.ID)
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "read last.fm account", err)
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, &status)
}

// lastfmStatus builds the API's view of the user's connection, field by
// field, so the session key can't leak.
func (s *Server) lastfmStatus(r *http.Request, userID int64) (api.LastfmStatus, error) {
	status := api.LastfmStatus{Configured: s.lastfm.Configured(), State: api.LastfmOK}
	acct, err := s.library.LastfmAccount(r.Context(), userID)
	if errors.Is(err, store.ErrNotFound) {
		return status, nil
	}
	if err != nil {
		return api.LastfmStatus{}, err //nolint:wrapcheck // The store's error says what failed.
	}
	status.Connected = true
	status.Name = acct.Name
	status.Scrobble, status.NowPlaying, status.Loves = acct.Switches.Scrobble, acct.Switches.NowPlaying, acct.Switches.Loves
	status.Waiting = acct.Waiting
	status.LastSentAt = acct.LastSentAt
	switch {
	case acct.State == store.LastfmReconnect:
		status.State = api.LastfmReconnect
	case acct.Attempts > 0 && acct.NextAt.After(s.now()):
		status.State = api.LastfmWaiting
		status.RetryAt = acct.NextAt
	}
	return status, nil
}

// patchLastfm changes the switches the body names.
func (s *Server) patchLastfm(w http.ResponseWriter, r *http.Request) {
	var req api.LastfmSwitches
	if err := readJSON(w, r, &req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "request body must be a JSON object of switches: scrobble, nowPlaying and loves")
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	acct, err := s.library.LastfmAccount(r.Context(), user.ID)
	if err == nil {
		sw := acct.Switches
		set := func(dst, v *bool) {
			if v != nil {
				*dst = *v
			}
		}
		set(&sw.Scrobble, req.Scrobble)
		set(&sw.NowPlaying, req.NowPlaying)
		set(&sw.Loves, req.Loves)
		err = s.library.SetLastfmSwitches(r.Context(), user.ID, sw)
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "not connected to Last.fm")
		return
	case err != nil:
		if r.Context().Err() == nil {
			s.internalError(w, r, "set last.fm switches", err)
		}
		return
	}
	s.getLastfm(w, r)
}

// deleteLastfm disconnects the caller's account and drops what waited for it.
func (s *Server) deleteLastfm(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	if err := s.library.DisconnectLastfm(r.Context(), user.ID); err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "disconnect last.fm", err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// connectLastfm starts a connect: it remembers a state value for the caller,
// sets it as a cookie, and returns the last.fm page that sends the browser
// back to the callback with it.
func (s *Server) connectLastfm(w http.ResponseWriter, r *http.Request) {
	if !s.lastfm.Configured() {
		writeProblem(w, r, http.StatusConflict, "Last.fm is not set up on this server")
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	state := s.lastfmStates.begin(user.ID, s.now())
	http.SetCookie(w, s.lastfmCookie(r, state, int(lastfmStateLife.Seconds())))
	callback := s.publicBase(r) + apiPrefix + lastfmCallbackPath + "?" + url.Values{"state": {state}}.Encode()
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, &api.LastfmConnect{URL: s.lastfm.AuthURL(callback)})
}

// lastfmCallback finishes a connect and always sends the browser to the You
// page, saying how it went. The token is single use and never logged: the
// access log has the path alone.
func (s *Server) lastfmCallback(w http.ResponseWriter, r *http.Request) {
	result := s.finishConnect(r)
	http.SetCookie(w, s.lastfmCookie(r, "", -1))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", s.basePath+youPath+"?"+url.Values{"lastfm": {result}}.Encode())
	w.WriteHeader(http.StatusSeeOther)
}

// finishConnect checks the callback against the caller's pending connect,
// stores the session Last.fm gives for the token, and says how it went.
func (s *Server) finishConnect(r *http.Request) string {
	if !s.lastfm.Configured() {
		return lastfmFailed
	}
	cred, ok := credential(r)
	if !ok || !cred.viaCookie {
		return lastfmFailed
	}
	user, err := s.auth.Authenticate(r.Context(), cred.token)
	if err != nil {
		return lastfmFailed
	}
	q := r.URL.Query()
	var cookie string
	if c, cookieErr := r.Cookie(lastfmStateCookie); cookieErr == nil {
		cookie = c.Value
	}
	if !s.lastfmStates.finish(user.ID, q.Get("state"), cookie, s.now()) {
		s.log.WarnContext(r.Context(), "last.fm callback with a missing, wrong or expired state",
			"requestId", requestID(r.Context()), "userId", user.ID)
		return lastfmFailed
	}
	token := q.Get("token")
	if token == "" {
		return lastfmRefused
	}
	sess, err := s.lastfm.Session(r.Context(), token)
	if err != nil {
		s.log.WarnContext(r.Context(), "last.fm session", "requestId", requestID(r.Context()), "err", err)
		return lastfmFailed
	}
	if err = s.library.ConnectLastfm(r.Context(), user.ID, sess.Name, sess.Key); err != nil {
		s.log.ErrorContext(r.Context(), "store last.fm account", "requestId", requestID(r.Context()), "err", err)
		return lastfmFailed
	}
	s.lastfm.Poke() // A reconnect sends what waited.
	return lastfmConnected
}

// lastfmCookie returns the state cookie with the given value and Max-Age. A
// negative maxAge deletes it.
func (s *Server) lastfmCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	c := s.sessionCookie(r, value, maxAge) //nolint:gosec // G124: HttpOnly and Lax as the session cookie; Secure follows it, for the same reason.
	c.Name = lastfmStateCookie
	return c
}

// publicBase is the origin and base path the browser reaches the server at:
// Config.PublicURL, or else the request's own host.
func (s *Server) publicBase(r *http.Request) string {
	if s.publicURL != "" {
		return strings.TrimSuffix(s.publicURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || s.secureCookies {
		scheme = "https"
	}
	return scheme + "://" + r.Host + s.basePath
}

// postNowPlaying passes a start on to the sender, which tells Last.fm when
// the caller wants that. It answers the same whatever happens.
func (s *Server) postNowPlaying(w http.ResponseWriter, r *http.Request) {
	var req api.NowPlaying
	if err := readJSON(w, r, &req); err != nil || req.TrackID == "" {
		writeProblem(w, r, http.StatusBadRequest, "request body must be a JSON object with a trackId")
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	s.lastfm.NowPlaying(user.ID, req.TrackID)
	w.WriteHeader(http.StatusNoContent)
}
