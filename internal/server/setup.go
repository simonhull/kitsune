package server

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"net/netip"
	"sync/atomic"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/auth"
)

const (
	// badSetupDetail is the detail of every malformed setup request. It never
	// echoes the body, which holds a password and maybe the code.
	badSetupDetail = "request body must be a JSON SetupRequest"
	// setupDoneDetail is the detail of a setup refused because an account exists.
	setupDoneDetail = "already set up: an account exists"
)

// sharedAddressSpace is 100.64.0.0/10 (RFC 6598): carrier-grade NAT, and the
// range Tailscale gives its devices.
var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10") //nolint:gochecknoglobals // A fixed prefix, never written.

// setupState is first-run setup: the code that callers outside the server's
// network must give, and whether an account exists. Once done is set it stays
// set, and nothing asks the store again.
type setupState struct {
	code []byte      // Normalized; nil when the server started with accounts.
	done atomic.Bool // An account exists.
}

// newSetupState returns the setup state for code, the one Config.SetupCode
// gives. With no code, setup is done from the start.
func newSetupState(code string) *setupState {
	s := &setupState{}
	if n := auth.NormalizeCode(code); n != "" {
		s.code = []byte(n)
	} else {
		s.done.Store(true)
	}
	return s
}

// matches reports whether code is the setup code, in constant time for codes
// of its length. The length is not a secret: every code has the same one.
func (s *setupState) matches(code string) bool {
	return s.code != nil && subtle.ConstantTimeCompare([]byte(auth.NormalizeCode(code)), s.code) == 1
}

// isLocal reports whether addr is in the server's network: loopback, a
// private range (RFC 1918 or fc00::/7), link-local unicast, or 100.64.0.0/10.
// An IPv4-mapped IPv6 address counts as its IPv4 address.
func isLocal(addr netip.Addr) bool {
	addr = addr.Unmap().WithZone("")
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || sharedAddressSpace.Contains(addr)
}

// callerIsLocal reports whether the request comes from the server's network,
// by the client IP: forwarded addresses count only from trusted proxies. An
// address that doesn't parse is not local.
func (s *Server) callerIsLocal(r *http.Request) bool {
	addr, err := netip.ParseAddr(s.clientIP(r))
	return err == nil && isLocal(addr)
}

// setupNeeded reports whether there are still no accounts. While setup is not
// done it asks the store, so an account made by users add closes setup too.
func (s *Server) setupNeeded(r *http.Request) (bool, error) {
	if s.setup.done.Load() {
		return false, nil
	}
	n, err := s.library.CountUsers(r.Context())
	if err != nil {
		return false, err //nolint:wrapcheck // The store's error already says what failed.
	}
	if n > 0 {
		s.setup.done.Store(true)
		return false, nil
	}
	return true, nil
}

// getSetup says whether the first account is still to be made and, while it
// is, whether the caller must give the setup code.
func (s *Server) getSetup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	needed, err := s.setupNeeded(r)
	if err != nil {
		s.internalError(w, r, "setup status", err)
		return
	}
	body := api.SetupStatus{Needed: needed, CodeRequired: needed && !s.callerIsLocal(r)}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// postSetup makes the first account, an administrator, and signs in as it
// with a cookie. From outside the server's network it needs the setup code; a
// wrong one counts against the login limiter by client IP, before anything is
// hashed.
func (s *Server) postSetup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	req, ok := decodeSetup(w, r)
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, badSetupDetail)
		return
	}
	needed, err := s.setupNeeded(r)
	if err != nil {
		s.internalError(w, r, "setup status", err)
		return
	}
	if !needed {
		writeProblem(w, r, http.StatusConflict, setupDoneDetail)
		return
	}
	if !s.callerIsLocal(r) && !s.checkSetupCode(w, r, req.Code) {
		return
	}
	if _, err = s.auth.CreateFirstAdmin(r.Context(), req.Name, req.Password); err != nil {
		s.setupFailed(w, r, err)
		return
	}
	s.setup.done.Store(true)
	res, err := s.auth.Login(r.Context(), req.Name, req.Password, r.UserAgent())
	if err != nil {
		s.internalError(w, r, "sign in as the first account", err)
		return
	}
	s.setSessionCookie(w, r, res.Token)
	session := api.Session{ExpiresAt: res.ExpiresAt, User: toAPIUser(&res.User)}
	writeBody(w, http.StatusOK, api.JSONMediaType, &session)
}

// checkSetupCode checks the code a caller outside the server's network gave.
// A missing code is refused without counting, since it guesses nothing. A
// given one counts against the caller's IP first, so a blocked caller gets 429
// and its code is never compared; the right one gets its attempt back. It
// writes the problem and reports false unless the code is right.
func (s *Server) checkSetupCode(w http.ResponseWriter, r *http.Request, code string) bool {
	if code == "" {
		writeProblem(w, r, http.StatusForbidden, "a setup code is needed from outside the server's network")
		return false
	}
	ik := ipKey(s.clientIP(r))
	if !s.checkLimits(w, r, ik) {
		return false
	}
	if !s.setup.matches(code) {
		writeProblem(w, r, http.StatusForbidden, "wrong setup code")
		return false
	}
	s.limiter.refund(ik) // The right code is not a guess.
	return true
}

// setupFailed writes the problem for an error from [auth.Service.CreateFirstAdmin].
func (s *Server) setupFailed(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrSetupDone):
		s.setup.done.Store(true)
		writeProblem(w, r, http.StatusConflict, setupDoneDetail)
	case errors.Is(err, auth.ErrInvalidName), errors.Is(err, auth.ErrPasswordTooShort), errors.Is(err, auth.ErrPasswordTooLong):
		// The auth service's messages name the rule, never the input.
		writeProblem(w, r, http.StatusBadRequest, err.Error())
	default:
		s.internalError(w, r, "create the first account", err)
	}
}

// decodeSetup reads the setup body. It reports false for anything but a small
// JSON SetupRequest with a name and a password. Requiring the JSON media type
// stops cross-site form posts, as for login.
func decodeSetup(w http.ResponseWriter, r *http.Request) (api.SetupRequest, bool) {
	var req api.SetupRequest
	ok := decodeCredentials(w, r, &req)
	return req, ok && req.Name != "" && req.Password != ""
}
