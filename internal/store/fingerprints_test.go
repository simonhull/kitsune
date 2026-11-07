package store_test

import (
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

// undigested lists the paths Undigested returns after after, up to limit.
func undigested(t *testing.T, s *store.Store, after store.FileKey, limit int) []string {
	t.Helper()
	rows, err := s.Undigested(t.Context(), after, limit)
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, len(rows))
	for i := range rows {
		paths[i] = rows[i].RelPath
	}
	return paths
}

func countUndigested(t *testing.T, s *store.Store) int {
	t.Helper()
	n, err := s.CountUndigested(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// unhashed is newWrite(i) without a fingerprint.
func unhashed(i int) store.TrackWrite {
	w := newWrite(i)
	w.Digest = nil
	return w
}

func TestUndigestedInPathOrder(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	gone := unhashed(4)
	writeAll(t, s, unhashed(3), newWrite(2), unhashed(1), unhashed(0), gone)
	if err := s.MarkMissing(t.Context(), []int64{idOf(t, s, &gone)}, fixedNow()); err != nil {
		t.Fatal(err)
	}

	first, second, fourth := newWrite(0).RelPath, newWrite(1).RelPath, newWrite(3).RelPath
	if got, want := undigested(t, s, store.FileKey{}, 10), []string{first, second, fourth}; !slices.Equal(got, want) {
		t.Errorf("Undigested = %q, want %q: live tracks without one, in path order", got, want)
	}
	if got, want := undigested(t, s, store.FileKey{}, 2), []string{first, second}; !slices.Equal(got, want) {
		t.Errorf("Undigested limit 2 = %q, want %q", got, want)
	}
	after := store.FileKey{Root: testRoot, RelPath: second}
	if got, want := undigested(t, s, after, 10), []string{fourth}; !slices.Equal(got, want) {
		t.Errorf("Undigested after a/1 = %q, want %q", got, want)
	}
	if got := countUndigested(t, s); got != 3 {
		t.Errorf("CountUndigested = %d, want 3", got)
	}
}

// waiting returns the row of w, the one track waiting for a fingerprint, and
// checks that it holds w's size and mtime.
func waiting(t *testing.T, s *store.Store, w *store.TrackWrite) store.Undigested {
	t.Helper()
	rows, err := s.Undigested(t.Context(), store.FileKey{}, 1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("Undigested = %v, %v; want one row", rows, err)
	}
	if rows[0].Size != w.Size || !rows[0].MTime.Equal(w.MTime.Truncate(time.Millisecond)) {
		t.Errorf("row = %+v, want the write's size and mtime", rows[0])
	}
	return rows[0]
}

func TestSetDigestOnlyForTheFileItRead(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	w := unhashed(0)
	writeAll(t, s, w)
	row := waiting(t, s, &w)
	digest := [32]byte{7}
	tests := []struct {
		name  string
		size  int64
		mtime time.Time
		set   bool
	}{
		{"another size", row.Size + 1, row.MTime, false},
		{"another mtime", row.Size, row.MTime.Add(time.Second), false},
		{"the file it read", row.Size, row.MTime, true},
	}
	for _, tt := range tests {
		ok, err := s.SetDigest(t.Context(), row.ID, tt.size, tt.mtime, digest)
		if err != nil || ok != tt.set {
			t.Errorf("%s: SetDigest = %t, %v; want %t", tt.name, ok, err, tt.set)
		}
	}
	if got := countUndigested(t, s); got != 0 {
		t.Errorf("CountUndigested = %d, want 0", got)
	}
	if err := s.MarkMissing(t.Context(), []int64{row.ID}, fixedNow()); err != nil {
		t.Fatal(err)
	}
	got, err := s.MissingByDigest(t.Context(), digest)
	if err != nil || len(got) != 1 || got[0].ID != row.ID {
		t.Errorf("MissingByDigest = %v, %v; want the track the digest was set on", got, err)
	}
}

func TestUpdateWithoutDigest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*store.TrackWrite)
		kept   bool
	}{
		{"same file keeps it", func(*store.TrackWrite) {}, true},
		{"new size clears it", func(w *store.TrackWrite) { w.Size++ }, false},
		{"new mtime clears it", func(w *store.TrackWrite) { w.MTime = w.MTime.Add(time.Second) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := openUsers(t)
			w := newWrite(0)
			writeAll(t, s, w)
			update := unhashed(0)
			update.ID = idOf(t, s, &w)
			tt.change(&update)

			writeAll(t, s, update)

			if got := countUndigested(t, s) == 0; got != tt.kept {
				t.Errorf("digest kept = %t, want %t", got, tt.kept)
			}
		})
	}
}

func TestAnyMissingWithDigest(t *testing.T) {
	t.Parallel()
	s := openUsers(t)
	hashed, bare := newWrite(0), unhashed(1)
	writeAll(t, s, hashed, bare)
	anyMissing := func() bool {
		t.Helper()
		found, err := s.AnyMissingWithDigest(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	if anyMissing() {
		t.Error("true with nothing missing")
	}
	if err := s.MarkMissing(t.Context(), []int64{idOf(t, s, &bare)}, fixedNow()); err != nil {
		t.Fatal(err)
	}
	if anyMissing() {
		t.Error("true with only a track without a fingerprint missing")
	}
	if err := s.MarkMissing(t.Context(), []int64{idOf(t, s, &hashed)}, fixedNow()); err != nil {
		t.Fatal(err)
	}
	if !anyMissing() {
		t.Error("false with a fingerprinted track missing")
	}
}
