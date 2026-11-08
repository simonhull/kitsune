package store

import (
	"context"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// Undigested is a live track that has no fingerprint yet, with the size and
// mtime its row holds.
type Undigested struct {
	ID            int64
	Root, RelPath string
	Size          int64
	MTime         time.Time
}

// FileKey is a place in path order: a root and a path under it. The zero value
// comes before every file.
type FileKey struct{ Root, RelPath string }

// Undigested returns up to limit live tracks without a fingerprint that come
// after the file at after, in path order.
func (s *Store) Undigested(ctx context.Context, after FileKey, limit int) ([]Undigested, error) {
	rows, err := s.rq.Undigested(ctx, sqlcdb.UndigestedParams{Root: after.Root, RelPath: after.RelPath, N: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("undigested: %w", err)
	}
	out := make([]Undigested, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = Undigested{ID: r.ID, Root: r.Root, RelPath: r.RelPath, Size: r.Size, MTime: fromMS(r.Mtime)}
	}
	return out, nil
}

// CountUndigested counts the live tracks without a fingerprint.
func (s *Store) CountUndigested(ctx context.Context) (int, error) {
	n, err := s.rq.CountUndigested(ctx)
	if err != nil {
		return 0, fmt.Errorf("count undigested: %w", err)
	}
	return int(n), nil
}

// SetDigest stores track id's fingerprint, read from a file of size and mtime.
// It writes only while the row still has that size and mtime and is live, and
// reports whether it did: a file that changed since it was read keeps waiting.
func (s *Store) SetDigest(ctx context.Context, id, size int64, mtime time.Time, digest [32]byte) (bool, error) {
	n, err := s.wq.SetDigest(ctx, sqlcdb.SetDigestParams{AudioDigest: digest[:], ID: id, Size: size, Mtime: ms(mtime)})
	if err != nil {
		return false, named(fmt.Errorf("set digest of track %d: %w", id, err))
	}
	return n == 1, nil
}

// AnyMissingWithDigest reports whether any missing track has a fingerprint,
// that is, whether a new file could be one of them moved.
func (s *Store) AnyMissingWithDigest(ctx context.Context) (bool, error) {
	found, err := s.rq.AnyMissingWithDigest(ctx)
	if err != nil {
		return false, fmt.Errorf("any missing with digest: %w", err)
	}
	return found, nil
}
