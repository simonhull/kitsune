package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/simonhull/kitsune/internal/store"
)

func TestHashSlotRespectsContext(t *testing.T) {
	t.Parallel()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), store.FileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s, err := New(st, Options{Params: cheap(), MaxConcurrentHashes: 1})
	if err != nil {
		t.Fatal(err)
	}
	s.slots <- struct{}{} // Hold the only slot.

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = s.CreateUser(ctx, "simon", "correct horse", store.RoleUser); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
