package server

import (
	"errors"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// badResetDetail is the detail of every malformed reset request.
	badResetDetail = "request body must be a JSON ResetRequest"
	// badResetConfirmDetail is the detail of every malformed reset
	// confirmation. It never echoes the body, which holds a code and a password.
	badResetConfirmDetail = "request body must be a JSON ResetConfirm"
)

// postReset makes a reset code for the account named in the body and logs it
// once, at WARN: whoever can read the server's log runs the server, the trust
// the setup code already rests on. It answers 202 whether or not the account
// exists, so it can't be used to find names. Requests are limited per IP and
// per name on their own limiter, so asking for codes never blocks a login.
func (s *Server) postReset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var req api.ResetRequest
	if !decodeCredentials(w, r, &req) || req.Name == "" {
		writeProblem(w, r, http.StatusBadRequest, badResetDetail)
		return
	}
	if !s.admit(s.resetLimiter, w, r, "too many reset requests; retry later", ipKey(s.clientIP(r)), nameKey(req.Name)) {
		return
	}
	ctx := r.Context()
	code, err := s.auth.RequestReset(ctx, req.Name)
	switch {
	case errors.Is(err, store.ErrNotFound):
		// The same answer as for a real account, and nothing logged.
	case err != nil:
		s.internalError(w, r, "request a password reset", err)
		return
	default:
		s.log.WarnContext(ctx, "password reset requested: the code sets a new password once, within 15 minutes",
			"user", code.User.Name, "resetCode", code.Code, "expiresAt", code.ExpiresAt, "requestId", requestID(ctx))
	}
	w.WriteHeader(http.StatusAccepted)
}

// postResetConfirm sets a new password with a reset code, ends every session
// of that account, and signs in as it. A wrong code counts against the login
// limiter, by name and by IP, before it is compared.
func (s *Server) postResetConfirm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var req api.ResetConfirm
	if !decodeCredentials(w, r, &req) || req.Name == "" || req.Code == "" || req.New == "" {
		writeProblem(w, r, http.StatusBadRequest, badResetConfirmDetail)
		return
	}
	if err := auth.ValidatePassword(req.New); err != nil {
		writeProblem(w, r, http.StatusBadRequest, err.Error()) // It names the rule, never the input.
		return
	}
	nk, ik := nameKey(req.Name), ipKey(s.clientIP(r))
	if !s.checkLimits(w, r, ik, nk) {
		return
	}
	res, err := s.auth.ResetPassword(r.Context(), req.Name, req.Code, req.New, r.UserAgent())
	if errors.Is(err, auth.ErrInvalidResetCode) {
		writeProblem(w, r, http.StatusForbidden, auth.ErrInvalidResetCode.Error())
		return
	}
	if err != nil {
		s.limiter.refund(ik, nk) // Not a guess; don't count it.
		s.internalError(w, r, "reset password", err)
		return
	}
	s.limiter.succeeded(nk, ik)
	s.writeSession(w, r, &res, req.Cookie)
}
