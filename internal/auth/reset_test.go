package auth_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/store"
)

// bubbleEnv is a store and service on the bubble's fake clock. The store
// closes when the bubble's test ends, inside it.
func bubbleEnv(t *testing.T) (*auth.Service, auth.LoginResult) {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), store.FileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc, err := auth.New(st, auth.Options{Params: cheapParams()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.CreateUser(t.Context(), simon, password, store.RoleUser); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Login(t.Context(), simon, password, "")
	if err != nil {
		t.Fatal(err)
	}
	return svc, res
}

func TestResetCodeExpires(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		after time.Duration
		works bool
	}{
		{name: "just inside", after: auth.ResetCodeTTL - time.Millisecond, works: true},
		{name: "at the end", after: auth.ResetCodeTTL},
		{name: "long after", after: time.Hour},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				svc, _ := bubbleEnv(t)
				code, err := svc.RequestReset(t.Context(), simon)
				if err != nil {
					t.Fatal(err)
				}
				if want := time.Now().Add(auth.ResetCodeTTL).UTC().Truncate(time.Millisecond); !code.ExpiresAt.Equal(want) {
					t.Errorf("ExpiresAt = %v, want %v", code.ExpiresAt, want)
				}
				time.Sleep(tt.after)
				_, err = svc.ResetPassword(t.Context(), simon, code.Code, newPassword, "")
				if tt.works && err != nil {
					t.Errorf("err = %v, want success", err)
				}
				if !tt.works && !errors.Is(err, auth.ErrInvalidResetCode) {
					t.Errorf("err = %v, want ErrInvalidResetCode", err)
				}
			})
		})
	}
}

func TestResetPasswordRefusals(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		svc, _ := bubbleEnv(t)
		code, err := svc.RequestReset(t.Context(), simon)
		if err != nil {
			t.Fatal(err)
		}
		for _, tt := range []struct {
			name, code, next string
			want             error
		}{
			{simon, code.Code, "short", auth.ErrPasswordTooShort},
			{simon, "0000-0000-0000-0000", newPassword, auth.ErrInvalidResetCode},
			{"nobody", code.Code, newPassword, auth.ErrInvalidResetCode},
		} {
			if _, err = svc.ResetPassword(t.Context(), tt.name, tt.code, tt.next, ""); !errors.Is(err, tt.want) {
				t.Errorf("%s %s %s: err = %v, want %v", tt.name, tt.code, tt.next, err, tt.want)
			}
		}
		if _, err = svc.ResetPassword(t.Context(), simon, code.Code, newPassword, ""); err != nil {
			t.Errorf("the refusals used the code up: %v", err)
		}
	})
}

func TestResetPassword(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		svc, old := bubbleEnv(t)
		code, err := svc.RequestReset(t.Context(), "SIMON")
		if err != nil {
			t.Fatal(err)
		}
		if code.User.Name != simon || code.User.PasswordHash != "" {
			t.Errorf("user = %+v, want simon without a hash", code.User)
		}
		typed := strings.ToUpper(strings.ReplaceAll(code.Code, "-", " "))
		res, err := svc.ResetPassword(t.Context(), simon, typed, newPassword, "phone/1")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = svc.Authenticate(t.Context(), res.Token); err != nil {
			t.Errorf("the new session: %v", err)
		}
		if _, err = svc.Authenticate(t.Context(), old.Token); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Errorf("the old session: err = %v, want ErrUnauthenticated", err)
		}
		if _, err = svc.ResetPassword(t.Context(), simon, code.Code, newPassword, ""); !errors.Is(err, auth.ErrInvalidResetCode) {
			t.Errorf("used twice: err = %v, want ErrInvalidResetCode", err)
		}
	})
}

func TestResetRequestUnknownAndReplaced(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		svc, _ := bubbleEnv(t)
		if _, err := svc.RequestReset(t.Context(), "nobody"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("unknown: err = %v, want ErrNotFound", err)
		}
		first, err := svc.RequestReset(t.Context(), simon)
		if err != nil {
			t.Fatal(err)
		}
		second, err := svc.RequestReset(t.Context(), simon)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = svc.ResetPassword(t.Context(), simon, first.Code, newPassword, ""); !errors.Is(err, auth.ErrInvalidResetCode) {
			t.Errorf("the replaced code: err = %v", err)
		}
		if _, err = svc.ResetPassword(t.Context(), simon, second.Code, newPassword, ""); err != nil {
			t.Errorf("the live code: %v", err)
		}
	})
}
