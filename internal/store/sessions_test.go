package store_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

func sessionT0() time.Time { return time.UnixMilli(1_762_000_000_000).UTC() }

func hashN(i byte) []byte { return bytes.Repeat([]byte{i}, 32) }

func newSessionUser(t *testing.T, s *store.Store) store.User {
	t.Helper()
	u, err := s.CreateUser(t.Context(), store.NewUser{Name: simon, PasswordHash: "h", Role: store.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestSessionRoundTrip(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	u := newSessionUser(t, s)
	t0 := sessionT0()
	exp := t0.Add(24 * time.Hour)
	err := s.CreateSession(t.Context(), &store.NewSession{
		TokenHash: hashN(1), UserID: u.ID, CreatedAt: t0, ExpiresAt: exp, UserAgent: "agent/1",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.SessionByTokenHash(t.Context(), hashN(1))
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(t0) || !got.LastSeen.Equal(t0) || !got.ExpiresAt.Equal(exp) {
		t.Errorf("times = %v %v %v, want %v %v %v", got.CreatedAt, got.LastSeen, got.ExpiresAt, t0, t0, exp)
	}
	if got.UserAgent != "agent/1" {
		t.Errorf("UserAgent = %q", got.UserAgent)
	}
	want := u
	want.PasswordHash = ""
	if got.User != want {
		t.Errorf("User = %+v, want %+v", got.User, want)
	}
}

func TestSessionByTokenHashNotFound(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	_, err := s.SessionByTokenHash(t.Context(), hashN(9))
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestTouchSession(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	u := newSessionUser(t, s)
	t0 := sessionT0()
	if err := s.CreateSession(t.Context(), &store.NewSession{
		TokenHash: hashN(1), UserID: u.ID, CreatedAt: t0, ExpiresAt: t0.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	t1, t2 := t0.Add(time.Minute), t0.Add(48*time.Hour)
	if err := s.TouchSession(t.Context(), hashN(1), t1, t2); err != nil {
		t.Fatal(err)
	}
	got, err := s.SessionByTokenHash(t.Context(), hashN(1))
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastSeen.Equal(t1) || !got.ExpiresAt.Equal(t2) {
		t.Errorf("LastSeen, ExpiresAt = %v, %v, want %v, %v", got.LastSeen, got.ExpiresAt, t1, t2)
	}
}

func TestDeleteSessionIsIdempotent(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	u := newSessionUser(t, s)
	t0 := sessionT0()
	if err := s.CreateSession(t.Context(), &store.NewSession{
		TokenHash: hashN(1), UserID: u.ID, CreatedAt: t0, ExpiresAt: t0.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.DeleteSession(t.Context(), hashN(1)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SessionByTokenHash(t.Context(), hashN(1)); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	u := newSessionUser(t, s)
	t0 := sessionT0()
	for i, off := range []time.Duration{-time.Millisecond, 0, time.Millisecond} {
		if err := s.CreateSession(t.Context(), &store.NewSession{
			TokenHash: hashN(byte(i)), UserID: u.ID, CreatedAt: t0.Add(-time.Hour), ExpiresAt: t0.Add(off),
		}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.DeleteExpiredSessions(t.Context(), t0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("deleted %d, want 2", n)
	}
	for i, wantErr := range []bool{true, true, false} {
		_, err = s.SessionByTokenHash(t.Context(), hashN(byte(i)))
		if gone := errors.Is(err, store.ErrNotFound); gone != wantErr {
			t.Errorf("session %d gone = %v, want %v (err %v)", i, gone, wantErr, err)
		}
	}
}

func TestCreateSessionRejectsBadHash(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	u := newSessionUser(t, s)
	t0 := sessionT0()
	err := s.CreateSession(t.Context(), &store.NewSession{
		TokenHash: bytes.Repeat([]byte{1}, 31), UserID: u.ID, CreatedAt: t0, ExpiresAt: t0.Add(time.Hour),
	})
	if err == nil {
		t.Fatal("31-byte hash was accepted")
	}
}

func TestSetPasswordHash(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	u := newSessionUser(t, s)
	if err := s.SetPasswordHash(t.Context(), u.ID, "new"); err != nil {
		t.Fatal(err)
	}
	got, err := s.UserByName(t.Context(), simon)
	if err != nil {
		t.Fatal(err)
	}
	if got.PasswordHash != "new" {
		t.Errorf("PasswordHash = %q, want new", got.PasswordHash)
	}
	if err = s.SetPasswordHash(t.Context(), u.ID+100, "x"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
