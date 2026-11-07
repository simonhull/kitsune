package store

import (
	"context"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// TestFingerprintPlans checks that the pass's queries read the partial indexes
// and never sort: on a library that is all fingerprinted they cost nothing.
func TestFingerprintPlans(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	tests := []struct {
		name, index string
		run         func(context.Context, *sqlcdb.Queries) error
	}{
		{"undigested", "tracks_undigested", func(ctx context.Context, q *sqlcdb.Queries) error {
			_, err := q.Undigested(ctx, sqlcdb.UndigestedParams{Root: "/m", RelPath: "a", N: 32})
			return err //nolint:wrapcheck // Test helper.
		}},
		{"count", "tracks_undigested", func(ctx context.Context, q *sqlcdb.Queries) error {
			_, err := q.CountUndigested(ctx)
			return err //nolint:wrapcheck // Test helper.
		}},
		// Either missing_since index reads only missing tracks.
		{"any missing", "tracks_missing_", func(ctx context.Context, q *sqlcdb.Queries) error {
			_, err := q.AnyMissingWithDigest(ctx)
			return err //nolint:wrapcheck // Test helper.
		}},
	}
	for _, tc := range tests {
		pl := &planner{db: s.r}
		if err := tc.run(t.Context(), sqlcdb.New(pl)); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		plan := strings.Join(pl.plans[0], "\n")
		if !strings.Contains(plan, "INDEX "+tc.index) {
			t.Errorf("%s: plan lacks INDEX %s:\n%s", tc.name, tc.index, plan)
		}
		if strings.Contains(plan, "TEMP B-TREE") {
			t.Errorf("%s: plan sorts:\n%s", tc.name, plan)
		}
	}
}
