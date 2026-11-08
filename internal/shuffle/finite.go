package shuffle

import (
	"cmp"
	"math"
	"math/rand/v2"
	"slices"
)

// The finite order's constants. The spec fixes the method; these tune it.
// They were set by hand and checked on the bench (internal/shuffle/bench).
const (
	// finiteStream is the PCG stream a finite order draws from, so it never
	// shares draws with an endless shuffle of the same seed.
	finiteStream = 0x66696e697465 // "finite"

	// spreadJitter is how far, as a share of its expected wait, a spread
	// unit may stray, so an artist's units don't land on a visible beat.
	// Spotify's dithering did the same.
	spreadJitter = 0.2
	// firstWait is how many expected waits an artist's first spread unit
	// may land within, uniformly: two keeps the race's mean wait.
	firstWait = 2

	// finiteAttempts is how many orders Finite makes of a set that the
	// repair and polish leave with spacing breaks, keeping the one with the
	// fewest. Only small, tight sets ever need a second: an artist's
	// discography, or a playlist half one artist's.
	finiteAttempts = 8
	// retryUpTo is the largest set Finite makes more than one order of. A
	// larger set has room enough that its breaks, if any, are few.
	retryUpTo = 4096
)

// Finite orders every unit in p once. Heavier units tend to come sooner;
// the same artist and album are kept apart (by [ArtistGap] and [AlbumGap],
// scaled down when the set is too small to allow them); each artist's units
// are spread evenly through it; and with a similarity,
// a next unit leans toward one like the last. The unit holding first leads,
// even a song skipped in shuffle, since the person chose it. A zero or
// unknown first leads with nothing.
//
// The order is fully decided by p, c and seed.
func Finite(p *Pool, c Config, seed uint64, first TrackID) []Pick {
	s := c.Settings.clamped()
	reasons := make([]Reason, len(p.units))
	for i := range p.units {
		reasons[i] = Explain(&p.units[i], s, c.Now)
	}
	attempts := 1
	if len(p.units) <= retryUpTo {
		attempts = finiteAttempts
	}
	best := attempt{breaks: math.MaxInt}
	for try := range uint64(attempts) {
		rng := rand.New(rand.NewPCG(seed, finiteStream+try)) //nolint:gosec // G404: a shuffle needs repeatability, not secrecy.
		if got := arrange(p, &c, reasons, rng, first); got.breaks < best.breaks {
			best = got
		}
		if best.breaks == 0 {
			break
		}
	}
	relean(&c, s.Flow, best.picks)
	return best.picks
}

// attempt is one finite order, with how many spacing breaks it was left
// with.
type attempt struct {
	picks  []Pick
	breaks int
}

// arrange makes one finite order of p's units, drawing on rng.
func arrange(p *Pool, c *Config, reasons []Reason, rng *rand.Rand, first TrackID) attempt {
	// Efraimidis–Spirakis: sorting by Exp(1)/w is a weighted random order,
	// the same as sorting by u^(1/w) without its underflow.
	n := len(p.units)
	keys := make([]float64, n)
	order := make([]int32, n)
	for i := range p.units {
		keys[i] = rng.ExpFloat64() / reasons[i].Weight()
		order[i] = int32(i)
	}
	slices.SortStableFunc(order, func(a, b int32) int { return cmp.Compare(keys[a], keys[b]) })
	spread(p, keys, reasons, order, rng)

	s := c.Settings.clamped()
	lead, solo := p.lead(first)
	total := n
	if solo != nil {
		total++
	}
	a := newArranger(p, c, s.Flow, order, total)
	fixed := 1
	switch {
	case solo != nil:
		r := Explain(solo, s, c.Now)
		a.place(-1, solo, &r)
	case lead >= 0:
		i := int32(lead) //nolint:gosec // G115: a pool index.
		a.place(i, &p.units[i], &reasons[i])
	default:
		fixed = 0
	}
	for a.left > 0 {
		i := a.choose()
		r := reasons[i]
		a.flow.lean(&r, &p.units[i])
		a.place(i, &p.units[i], &r)
	}
	return attempt{picks: a.out, breaks: polish(p, a.out, fixed)}
}

// spread evens out every artist's units without undoing the weights: the
// dithering Spotify described in 2014, in the weighted order's own time.
//
// A weighted random order is a race: each unit arrives at its key, an
// exponential time at the rate of its weight, and the order is the order of
// arrival. An artist's units arrive as a random process, so they bunch and
// leave gaps. spread keeps each artist's units in their order of arrival,
// and so which of them come first, but moves them to the times the race
// expects: the artist's first unit at a random phase within twice its
// expected wait, each next one after the expected wait given the weight it
// has left, with a little jitter. An artist with more weight still arrives
// sooner and more densely; it just stops arriving in clumps. A unit alone
// in its artist keeps its key.
//
// On the bench, spreading only artists with a large share of the set (the
// spec's wording) left the gaps between an artist's units nearly as uneven
// as a plain shuffle's; spreading every artist evenly by rank evened them
// but cost the weights a third of their pull. This keeps both.
func spread(p *Pool, keys []float64, reasons []Reason, order []int32, rng *rand.Rand) {
	for _, units := range byPrimary(p, order) {
		if len(units) <= 1 {
			continue
		}
		left := 0.0
		for _, i := range units {
			left += reasons[i].Weight()
		}
		t := firstWait * rng.Float64() / left
		for _, i := range units {
			keys[i] = t
			if left -= reasons[i].Weight(); left > 0 {
				t += (1 + spreadJitter*(rng.Float64()-neutral)/neutral) / left
			}
		}
	}
	slices.SortStableFunc(order, func(a, b int32) int { return cmp.Compare(keys[a], keys[b]) })
}

// byPrimary lists order's units by their primary artist's index, each in
// order.
func byPrimary(p *Pool, order []int32) [][]int32 {
	by := make([][]int32, len(p.artistGap))
	for _, i := range order {
		if u := &p.units[i]; len(u.artist) > 0 {
			by[u.artist[0]] = append(by[u.artist[0]], i)
		}
	}
	return by
}
