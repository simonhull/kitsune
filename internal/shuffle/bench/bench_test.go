package bench

import (
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/shuffle"
)

// pageBudget is the spec's limit on an endless page from 50,000 songs.
const pageBudget = 20 * time.Millisecond

// big is the bench's 50,000-song library, with the default config and its
// similarity: flow on, the costliest pick.
func big(tb testing.TB) (*library, *shuffle.Pool, shuffle.Config) {
	tb.Helper()
	lib := generate(shapes()[4])
	return &lib, pool(tb, &lib), shuffle.Config{Settings: shuffle.DefaultSettings(), Similarity: lib.sim, Now: now}
}

func BenchmarkEndlessPage(b *testing.B) {
	_, p, c := big(b)
	e := shuffle.NewEndless(1, 0)
	for b.Loop() {
		e.Next(p, c, shuffle.PageSize)
	}
}

// BenchmarkEndlessPageWithPool is a page as the server will serve one:
// the pool built afresh from the songs, then the page.
func BenchmarkEndlessPageWithPool(b *testing.B) {
	lib, _, c := big(b)
	e := shuffle.NewEndless(1, 0)
	for b.Loop() {
		p, err := shuffle.NewPool(lib.tracks, lib.runs)
		if err != nil {
			b.Fatal(err)
		}
		e.Next(p, c, shuffle.PageSize)
	}
}

func BenchmarkNewPool(b *testing.B) {
	lib, _, _ := big(b)
	for b.Loop() {
		if _, err := shuffle.NewPool(lib.tracks, lib.runs); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFinite(b *testing.B) {
	for _, sh := range []shape{shapes()[0], shapes()[4]} {
		lib := generate(sh)
		p := pool(b, &lib)
		c := shuffle.Config{Settings: shuffle.DefaultSettings(), Similarity: lib.sim, Now: now}
		b.Run(sh.name, func(b *testing.B) {
			for b.Loop() {
				shuffle.Finite(p, c, 1, 0)
			}
		})
	}
}

func TestPageSpeed(t *testing.T) {
	t.Parallel()
	if raceOn || testing.Short() {
		t.Skip("timing needs a plain build: the race detector slows everything several times over")
	}
	lib, _, c := big(t)
	e := shuffle.NewEndless(1, 0)
	const pages = 15
	took := make([]time.Duration, pages)
	for i := range took {
		start := time.Now()
		p, err := shuffle.NewPool(lib.tracks, lib.runs)
		if err != nil {
			t.Fatal(err)
		}
		e.Next(p, c, shuffle.PageSize)
		took[i] = time.Since(start)
	}
	slices.Sort(took)
	if median := took[pages/2]; median > pageBudget {
		t.Errorf("a 50,000-song page took %v, pool included; the budget is %v", median, pageBudget)
	}
}
