package server

import (
	"encoding/json/v2"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// maxLoginBody caps a login request body.
	maxLoginBody = 4 << 10
	// badLoginDetail is the detail of every malformed login request. It never
	// echoes the body, which holds a password.
	badLoginDetail = "request body must be a JSON LoginRequest"
)

// toAPIUser builds the API's view of u field by field, so a new store column
// can't leak into a response.
func toAPIUser(u *store.User) api.User {
	return api.User{ID: u.ID, Name: u.Name, Role: string(u.Role), CreatedAt: u.CreatedAt}
}

// getMe returns the authenticated user.
func (*Server) getMe(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r.Context()) // authenticate ran, so ok.
	w.Header().Set("Cache-Control", "no-store")
	body := toAPIUser(&u)
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// login exchanges a name and password for a session. It counts the attempt
// against the rate limits before it hashes anything, so a blocked client
// costs the server no hashing, even with the right password.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeLogin(w, r)
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, badLoginDetail)
		return
	}
	nk, ik := nameKey(req.Name), ipKey(s.clientIP(r))
	if !s.checkLimits(w, r, ik, nk) {
		return
	}
	res, err := s.auth.Login(r.Context(), req.Name, req.Password, r.UserAgent())
	if errors.Is(err, auth.ErrInvalidCredentials) {
		writeProblem(w, r, http.StatusUnauthorized, "invalid name or password")
		return
	}
	if err != nil {
		s.limiter.refund(ik, nk) // Not a guess; don't count it.
		s.internalError(w, r, "login", err)
		return
	}
	s.limiter.succeeded(nk, ik)

	session := api.Session{ExpiresAt: res.ExpiresAt, User: toAPIUser(&res.User)}
	if req.Cookie {
		s.setSessionCookie(w, r, res.Token)
	} else {
		session.Token = res.Token
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, &session)
}

// decodeLogin reads the login body. It reports false for anything but a
// small JSON LoginRequest with a name and a password. Requiring the JSON
// media type stops login CSRF: a cross-site text/plain form can produce a
// body that decodes, and its Lax cookie would log the victim into the
// attacker's account.
func decodeLogin(w http.ResponseWriter, r *http.Request) (api.LoginRequest, bool) {
	var req api.LoginRequest
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != api.JSONMediaType {
		return req, false
	}
	body := http.MaxBytesReader(w, r.Body, maxLoginBody)
	if err = json.UnmarshalRead(body, &req, json.RejectUnknownMembers(true)); err != nil {
		return req, false
	}
	return req, req.Name != "" && req.Password != ""
}

// checkLimits counts a login attempt on the keys and, if one is blocked,
// writes the 429 problem and reports false.
func (s *Server) checkLimits(w http.ResponseWriter, r *http.Request, keys ...string) bool {
	wait, ok := s.limiter.attempt(keys, s.now())
	if ok {
		return true
	}
	seconds := int((wait + time.Second - 1) / time.Second)
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeProblem(w, r, http.StatusTooManyRequests, "too many failed logins; retry later")
	return false
}

// logout ends the session the request authenticated with and clears the
// session cookie, which is harmless for a bearer logout.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.Logout(r.Context(), currentToken(r.Context())); err != nil {
		s.internalError(w, r, "logout", err)
		return
	}
	http.SetCookie(w, s.sessionCookie(r, "", -1))
	w.WriteHeader(http.StatusNoContent)
}

// setSessionCookie sends the session token as an HttpOnly cookie. The server
// enforces the real, sliding expiry; the cookie only has to outlive it.
func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, s.sessionCookie(r, token, int(auth.SessionMaxAge.Seconds())))
}

// sessionCookie returns the session cookie with the given value and Max-Age.
// A negative maxAge deletes the cookie.
func (s *Server) sessionCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // G124: Secure follows TLS or Config.SecureCookies, because an HTTPS proxy may terminate TLS in front of the server.
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || s.secureCookies,
	}
}
