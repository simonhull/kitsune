package store_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

const simon = "Simon"

func fixedNow() time.Time { return time.UnixMilli(1_762_000_000_000).UTC() }

func openUsers(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "kitsune.db"), store.Options{
		Now: fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestCreateAndFetchUser(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	created, err := s.CreateUser(t.Context(), store.NewUser{Name: simon, PasswordHash: "h", Role: store.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.UserByName(t.Context(), "simon")
	if err != nil {
		t.Fatal(err)
	}
	if got != created {
		t.Errorf("UserByName = %+v, CreateUser returned %+v", got, created)
	}
	want := store.User{ID: got.ID, Name: simon, PasswordHash: "h", Role: store.RoleAdmin, CreatedAt: fixedNow()}
	if got != want || got.ID <= 0 {
		t.Errorf("user = %+v, want %+v with a positive ID", got, want)
	}
	n, err := s.CountUsers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("CountUsers = %d, want 1", n)
	}
}

func TestCreateUserNameTaken(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	if _, err := s.CreateUser(t.Context(), store.NewUser{Name: simon, PasswordHash: "h", Role: store.RoleUser}); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateUser(t.Context(), store.NewUser{Name: "SIMON", PasswordHash: "h", Role: store.RoleUser})
	if !errors.Is(err, store.ErrNameTaken) {
		t.Errorf("err = %v, want ErrNameTaken", err)
	}
}

func TestUserByNameNotFound(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	_, err := s.UserByName(t.Context(), "nobody")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateUserRejectsBadRole(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	_, err := s.CreateUser(t.Context(), store.NewUser{Name: simon, PasswordHash: "h", Role: store.Role("root")})
	if err == nil {
		t.Fatal("CreateUser accepted role root")
	}
	if errors.Is(err, store.ErrNameTaken) {
		t.Errorf("err = %v, must not be ErrNameTaken", err)
	}
}

func TestConcurrentCreates(t *testing.T) {
	t.Parallel()
	const n = 20
	s := openUsers(t)
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Go(func() {
			_, err := s.CreateUser(t.Context(), store.NewUser{Name: fmt.Sprintf("user%02d", i), PasswordHash: "h", Role: store.RoleUser})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	got, err := s.CountUsers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got != n {
		t.Errorf("CountUsers = %d, want %d", got, n)
	}
}
