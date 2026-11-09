package auth

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

// ResetCodeTTL is how long a reset code works.
const ResetCodeTTL = 15 * time.Minute

// ErrInvalidResetCode means a reset was refused: the code was wrong, expired
// or already used, or no account has the name. It is the same for all of them.
var ErrInvalidResetCode = errors.New("wrong or expired reset code")

// ResetCode is a code that sets a new password for User once, until ExpiresAt.
type ResetCode struct {
	Code      string // For the server's log only: never a response.
	User      store.User
	ExpiresAt time.Time
}

// resets holds the live reset codes, at most one per person. They live in
// memory only, so a restart voids them.
type resets struct {
	mu     sync.Mutex
	byUser map[int64]pendingReset
}

// pendingReset is one person's live code.
type pendingReset struct {
	code    []byte // Normalized.
	expires time.Time
}

// RequestReset makes a reset code for the user called name, valid for
// [ResetCodeTTL], and voids any code they had before. It returns an error
// wrapping [store.ErrNotFound] if no user has the name. The returned user
// carries no password hash.
func (s *Service) RequestReset(ctx context.Context, name string) (ResetCode, error) {
	u, err := s.store.UserByName(ctx, name)
	if err != nil {
		return ResetCode{}, fmt.Errorf("auth: request reset: %w", err)
	}
	u.PasswordHash = ""
	code, now := NewCode(), s.clock()
	expires := now.Add(ResetCodeTTL)
	s.resets.mu.Lock()
	defer s.resets.mu.Unlock()
	maps.DeleteFunc(s.resets.byUser, func(_ int64, p pendingReset) bool { return !now.Before(p.expires) })
	s.resets.byUser[u.ID] = pendingReset{code: []byte(NormalizeCode(code)), expires: expires}
	return ResetCode{Code: code, User: u, ExpiresAt: expires}, nil
}

// ResetPassword makes next the password of the user called name, if code is
// their live reset code, which it then voids. It ends every session of theirs
// and starts a new one, as [Service.Login] does. It returns an error wrapping
// [ErrPasswordTooShort] or [ErrPasswordTooLong] if next breaks the rules,
// checked first so the code stays live, or [ErrInvalidResetCode].
func (s *Service) ResetPassword(ctx context.Context, name, code, next, userAgent string) (LoginResult, error) {
	if err := ValidatePassword(next); err != nil {
		return LoginResult{}, err
	}
	u, err := s.store.UserByName(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return LoginResult{}, ErrInvalidResetCode
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("auth: reset password: %w", err)
	}
	if !s.resets.take(u.ID, code, s.clock()) {
		return LoginResult{}, ErrInvalidResetCode
	}
	if setErr := s.SetPassword(ctx, u.ID, next, SignOutEverywhere()); setErr != nil {
		return LoginResult{}, setErr
	}
	return s.startSession(ctx, &u, userAgent)
}

// take reports whether code is id's live reset code at now and, if so, voids
// it, so of two racing resets with the right code only one succeeds.
func (r *resets) take(id int64, code string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byUser[id]
	if !ok {
		return false
	}
	if !now.Before(p.expires) {
		delete(r.byUser, id)
		return false
	}
	if !codeMatches(code, p.code) {
		return false
	}
	delete(r.byUser, id)
	return true
}
