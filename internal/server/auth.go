package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// sessionCookie is the name of the session cookie.
	sessionCookie = "kitsune_session"
	// requestHeader must be "1" on cookie-authenticated unsafe requests. A
	// cross-site form cannot set a custom header, so this stops CSRF.
	requestHeader = "X-Kitsune-Request"
	// bearerScheme is the Authorization scheme for session tokens.
	bearerScheme = "Bearer"
)

// authInfo is what [Server.authenticate] puts in the request context.
type authInfo struct {
	user  store.User
	token string // The credential the request used.
}

// authInfoKey is the context key for [authInfo].
type authInfoKey struct{}

// currentUser returns the user the request was authenticated as. It reports
// false outside [Server.authenticate].
func currentUser(ctx context.Context) (store.User, bool) {
	info, ok := ctx.Value(authInfoKey{}).(authInfo)
	return info.user, ok
}

// currentToken returns the session token the request authenticated with. It
// is empty outside [Server.authenticate].
func currentToken(ctx context.Context) string {
	info, _ := ctx.Value(authInfoKey{}).(authInfo) // Absent means no token.
	return info.token
}

// authenticate wraps next so that it runs only for requests with a valid
// session token, from the Authorization header or the session cookie. The
// header wins over the cookie. A cookie-authenticated request that is not
// safe must also send X-Kitsune-Request: 1, which is checked before the
// lookup so a forged request costs nothing.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cred, ok := credential(r)
		if !ok {
			unauthorized(w, r)
			return
		}
		if cred.viaCookie && !isSafeMethod(r.Method) && r.Header.Get(requestHeader) != "1" {
			writeProblem(w, r, http.StatusForbidden, "cookie-authenticated requests must send "+requestHeader+": 1")
			return
		}
		user, err := s.auth.Authenticate(r.Context(), cred.token)
		if errors.Is(err, auth.ErrUnauthenticated) {
			unauthorized(w, r)
			return
		}
		if err != nil {
			s.internalError(w, r, "authenticate", err)
			return
		}
		ctx := context.WithValue(r.Context(), authInfoKey{}, authInfo{user: user, token: cred.token})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireAdmin wraps next so that it runs only for administrators. It belongs
// inside [Server.authenticate], which knows the caller: outside it, everyone is
// refused.
func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(r.Context()) {
			writeProblem(w, r, http.StatusForbidden, "administrators only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isAdmin reports whether the caller is an administrator.
func isAdmin(ctx context.Context) bool {
	u, ok := currentUser(ctx)
	return ok && u.Role == store.RoleAdmin
}

// credentials is the session token a request carries and where it came from.
type credentials struct {
	token     string
	viaCookie bool
}

// credential returns the credentials a request carries. ok is false when
// there are none that are usable: a malformed Authorization header does not
// fall back to the cookie.
func credential(r *http.Request) (credentials, bool) {
	if values := r.Header.Values("Authorization"); len(values) > 0 {
		scheme, token, found := strings.Cut(values[0], " ")
		if len(values) != 1 || !found || !strings.EqualFold(scheme, bearerScheme) || token == "" {
			return credentials{}, false
		}
		return credentials{token: token}, true
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return credentials{}, false
	}
	return credentials{token: c.Value, viaCookie: true}, true
}

// isSafeMethod reports whether method cannot change state.
func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

// unauthorized writes the 401 problem. The detail never says why, so it
// cannot echo a credential.
func unauthorized(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", bearerScheme)
	writeProblem(w, r, http.StatusUnauthorized, "missing or invalid credentials")
}

// internalError logs err with the request ID and writes a 500 problem without
// it.
func (s *Server) internalError(w http.ResponseWriter, r *http.Request, what string, err error) {
	s.log.ErrorContext(r.Context(), what, "requestId", requestID(r.Context()), "err", err)
	writeProblem(w, r, http.StatusInternalServerError, "internal error")
}
