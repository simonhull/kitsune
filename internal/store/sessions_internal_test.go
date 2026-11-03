package store

import (
	"bytes"
	"testing"
	"time"
)

func TestSessionsCascadeWithUser(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	u, err := s.CreateUser(t.Context(), NewUser{Name: "simon", PasswordHash: "h", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	hash := bytes.Repeat([]byte{1}, 32)
	t0 := time.UnixMilli(1_762_000_000_000).UTC()
	if err = s.CreateSession(t.Context(), &NewSession{
		TokenHash: hash, UserID: u.ID, CreatedAt: t0, ExpiresAt: t0.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.w.ExecContext(t.Context(), "DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = s.r.QueryRowContext(t.Context(), "SELECT count(*) FROM sessions").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d sessions remain after deleting their user", n)
	}
}
