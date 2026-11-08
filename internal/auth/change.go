package auth

import (
	"context"
	"fmt"
)

// SignOut says which of a person's sessions a new password ends. The zero
// value ends them all.
type SignOut struct {
	keep string // The token of the one session to keep.
	none bool   // Keep every session.
}

// SignOutEverywhere ends every session, as after a reset.
func SignOutEverywhere() SignOut { return SignOut{} }

// SignOutElsewhere ends every session but the one token belongs to, so the
// person changing their password stays signed in where they did it.
func SignOutElsewhere(token string) SignOut { return SignOut{keep: token} }

// StaySignedIn ends no session.
func StaySignedIn() SignOut { return SignOut{none: true} }

// SetPassword validates password, hashes it and makes it the password of the
// user with id, ending the sessions out says to in the same write. It returns
// an error wrapping [ErrPasswordTooShort] or [ErrPasswordTooLong], one
// wrapping [store.ErrNotFound] if no user has id, or the context's error if it
// ends while waiting to hash.
func (s *Service) SetPassword(ctx context.Context, id int64, password string, out SignOut) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	var hash string
	if err := s.withSlot(ctx, func() { hash = hashPassword(s.params, password) }); err != nil {
		return err
	}
	var err error
	switch {
	case out.none:
		err = s.store.SetPasswordHash(ctx, id, hash)
	case out.keep != "":
		err = s.store.SetPassword(ctx, id, hash, hashToken(out.keep))
	default:
		err = s.store.SetPassword(ctx, id, hash, nil)
	}
	if err != nil {
		return fmt.Errorf("auth: set password: %w", err)
	}
	return nil
}

// ChangePassword makes next the password of the user called name, once
// current is shown to be theirs, and ends every session of theirs but the one
// token belongs to. It returns [ErrInvalidCredentials] if current is wrong, an
// error wrapping [ErrPasswordTooShort] or [ErrPasswordTooLong] if next breaks
// the rules (checked first, so a weak next costs no hashing), or the context's
// error if it ends while waiting to hash.
func (s *Service) ChangePassword(ctx context.Context, name, current, next, token string) error {
	if err := ValidatePassword(next); err != nil {
		return err
	}
	u, _, err := s.checkCredentials(ctx, name, current)
	if err != nil {
		return err
	}
	return s.SetPassword(ctx, u.ID, next, SignOutElsewhere(token))
}
