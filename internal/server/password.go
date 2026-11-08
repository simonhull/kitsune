package server

import (
	"encoding/json/v2"
	"errors"
	"mime"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/auth"
)

// badPasswordChangeDetail is the detail of every malformed password change. It
// never echoes the body, which holds two passwords.
const badPasswordChangeDetail = "request body must be a JSON PasswordChange"

// postMyPassword changes the caller's password. The current one must be
// right; a wrong one counts against the login limiter, by name and by IP,
// before anything is hashed. The session the request used stays and the
// caller's others end.
func (s *Server) postMyPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var req api.PasswordChange
	if !decodeCredentials(w, r, &req) || req.Current == "" || req.New == "" {
		writeProblem(w, r, http.StatusBadRequest, badPasswordChangeDetail)
		return
	}
	if err := auth.ValidatePassword(req.New); err != nil {
		writeProblem(w, r, http.StatusBadRequest, err.Error()) // It names the rule, never the input.
		return
	}
	ctx := r.Context()
	u, _ := currentUser(ctx) // authenticate ran, so ok.
	nk, ik := nameKey(u.Name), ipKey(s.clientIP(r))
	if !s.checkLimits(w, r, ik, nk) {
		return
	}
	err := s.auth.ChangePassword(ctx, u.Name, req.Current, req.New, currentToken(ctx))
	if errors.Is(err, auth.ErrInvalidCredentials) {
		writeProblem(w, r, http.StatusForbidden, "the current password is wrong")
		return
	}
	if err != nil {
		s.limiter.refund(ik, nk) // Not a guess; don't count it.
		s.internalError(w, r, "change password", err)
		return
	}
	s.limiter.succeeded(nk, ik)
	w.WriteHeader(http.StatusNoContent)
}

// decodeCredentials reads a small JSON body that holds a password or a code
// into req, a pointer. It reports false for anything else, a body with unknown
// members included. Requiring the JSON media type stops cross-site form
// posts: a text/plain form can produce a body that decodes.
func decodeCredentials(w http.ResponseWriter, r *http.Request, req any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != api.JSONMediaType {
		return false
	}
	body := http.MaxBytesReader(w, r.Body, maxLoginBody)
	return json.UnmarshalRead(body, req, json.RejectUnknownMembers(true)) == nil
}
