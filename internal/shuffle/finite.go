package shuffle

import (
	"cmp"
	"math"
	"math/rand/v2"
	"slices"
)

// The finite order's constants. The spec fixes the method; these tune it,
// and each was chosen on the bench (internal/shuffle/bench).
const (
	// finiteStream is the PCG stream a finite order draws from, so it never
	// shares draws with an endless shuffle of the same seed.
	finiteStream = 0x66696e697465 // "finite"

	// spreadShare is the share of a set's units an artist must hold to be
	// spread evenly. Below it, an artist's units are few enough that spacing
	// alone keeps them apart, and the weights keep deciding their places.
	spreadShare = 1.0 / 16
	// spreadJitter is how far, in slots of the artist's own spacing, an
	// evenly spread unit may stray, so an artist's units don't land on a
	// visible beat. Spotify's dithering did the same.
	spreadJitter = 0.2

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
// scaled down when the set is too small to allow them); an artist holding a
// large share of the set is spread evenly through it; and with a similarity,
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
	spread(p, order, rng)

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

// spread evens out the artists that hold a large share of order, the
// dithering Spotify described in 2014. Each such artist's units, in their
// order, take evenly spaced places from 0 to 1, with a random phase and a
// little jitter; every other unit takes an evenly spaced place by its rank
// among the rest; and order is sorted by place. The weights still decide
// which of an artist's units come first, and the order of the rest.
func spread(p *Pool, order []int32, rng *rand.Rand) {
	n := len(order)
	place := make([]float64, n)
	spreading := make([]bool, n)
	for _, units := range byPrimary(p, order) {
		k := float64(len(units))
		if len(units) < 2 || k < spreadShare*float64(n) {
			continue
		}
		phase := rng.Float64()
		for j, i := range units {
			jitter := spreadJitter * (2*rng.Float64() - 1)
			place[i] = (float64(j) + phase + jitter) / k
			spreading[i] = true
		}
	}
	rest := slices.DeleteFunc(slices.Clone(order), func(i int32) bool { return spreading[i] })
	if len(rest) == n {
		return
	}
	for rank, i := range rest {
		place[i] = (float64(rank) + neutral) / float64(len(rest))
	}
	slices.SortStableFunc(order, func(a, b int32) int { return cmp.Compare(place[a], place[b]) })
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
