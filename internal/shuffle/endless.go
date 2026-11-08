package shuffle

import (
	"cmp"
	"maps"
	"math/rand/v2"
	"slices"
	"time"
)

// The endless shuffle's constants.
const (
	// PageSize is how many units an endless page holds, from the spec.
	PageSize = 50
	// NoRepeatFor is the longest a unit is kept from coming back, from the
	// spec: "no unit repeats until about 90% of the eligible weight has
	// played, or 30 days have passed".
	NoRepeatFor = 30 * 24 * time.Hour
	// CycleShare is the share of the pool's weight that must have played
	// before played units may come back. The spec began at 0.9 ("about 90% of
	// the eligible weight"); the bench's fidelity-against-coverage curve
	// chose this lower value, so ratings and skips bite harder, now that a
	// deadline keeps every unit from starving (BASELINE.md, "Cycle share").
	CycleShare = 0.7
	// dueAfter is how many pool-sizes of picks a unit may wait before it is
	// due: then it comes next, where spacing allows, whatever its weight. It
	// is what holds the bench's hard rule that every eligible unit plays
	// within three passes.
	dueAfter = 2
	// artistLean is the most an artist's turn leans a pick: an artist that
	// has had fewer turns this cycle than its weight earns weighs up to this
	// much more, and one that has had more, up to this much less. It spreads
	// an artist's units through an endless shuffle as the finite spread does.
	artistLean = 4

	// endlessStream is the PCG stream base an endless page draws from, its
	// position added, kept clear of the finite order's streams.
	endlessStream = 1 << 62
	// endlessTries is how many draws a pick makes before it stops drawing
	// and weighs every eligible unit instead: draws keep failing only when
	// spacing or flow rules out most of the weight.
	endlessTries = 64
)

// Endless is an endless shuffle: it streams the units of a pool, page by
// page, without repeats. Each page is worked out from the pool it is given,
// so skips and plays since the last page count at once. Spacing holds across
// pages, and a unit that has played doesn't come back until [CycleShare] of
// the pool's weight has played, or [NoRepeatFor] has passed.
//
// An Endless is a value to keep between pages, not safe for concurrent use.
// [Endless.State] and [ResumeEndless] carry it between requests and devices.
type Endless struct {
	seed     uint64
	position uint64
	first    TrackID // the song to lead with on the first page; 0 once led
	w        window
	played   map[TrackID]played
	last     map[TrackID]uint64 // the position each unit was last given at
}

// played is a unit that has come up in this cycle.
type played struct {
	at     time.Time
	weight float64 // its weight when it came up
}

// NewEndless starts an endless shuffle from seed. The unit holding first
// leads it, even a song skipped in shuffle, since the person chose it; a
// zero or unknown first leads with nothing.
func NewEndless(seed uint64, first TrackID) *Endless {
	return &Endless{seed: seed, first: first, played: make(map[TrackID]played), last: make(map[TrackID]uint64)}
}

// Position is how many units the shuffle has given.
func (e *Endless) Position() uint64 { return e.position }

// Next returns the next n units from p (or fewer when p has fewer) at
// c.Now. The same state, pool and config always give the same page.
func (e *Endless) Next(p *Pool, c Config, n int) []Pick {
	if n <= 0 || (p.Len() == 0 && e.first == 0) {
		return nil
	}
	s := c.Settings.clamped()
	rng := rand.New(rand.NewPCG(e.seed, endlessStream+e.position)) //nolint:gosec // G404: a shuffle needs repeatability, not secrecy.
	pg := e.page(p, &c, s)
	out := make([]Pick, 0, n)
	if e.first != 0 {
		out = e.lead(pg, s, out)
	}
	for len(out) < n && p.Len() > 0 {
		if pg.eligible.total() <= 0 || pg.playedWeight >= c.cycle()*(pg.playedWeight+pg.eligible.total()) {
			e.newCycle(pg)
		}
		after, ok := e.w.last()
		f := newFlow(&c, s.Flow, after, ok)
		i := e.due(pg)
		if i < 0 {
			i = e.pick(pg, &f, rng)
		}
		r := pg.reasons[i]
		f.lean(&r, &p.units[i])
		out = append(out, e.record(pg, i, &p.units[i], &r))
	}
	return out
}

// EndlessState is an endless shuffle's state, to store between pages and
// hand from one device to another.
type EndlessState struct {
	// Seed is the shuffle's seed.
	Seed uint64
	// Position is how many units it has given.
	Position uint64
	// First is the song still to lead with, or 0.
	First TrackID
	// Recent are the artists and albums of the last units given, the latest
	// last, for spacing across pages.
	Recent []Recent
	// Played are the units given this cycle.
	Played []Played
	// Last are the units given, each at the position it was last given at.
	Last []Given
}

// Given is a unit given, at a position.
type Given struct {
	Unit TrackID
	At   uint64
}

// Recent is the artists and albums of one recent unit.
type Recent struct {
	Artists []ArtistID
	Albums  []AlbumID
}

// Played is a unit given this cycle: when, and its weight then.
type Played struct {
	Unit   TrackID
	At     time.Time
	Weight float64
}

// State is the shuffle's state.
func (e *Endless) State() EndlessState {
	st := EndlessState{Seed: e.seed, Position: e.position, First: e.first}
	for d := e.w.n; d >= 1; d-- {
		r := e.w.at(d)
		st.Recent = append(st.Recent, Recent{Artists: slices.Clone(r.artists), Albums: slices.Clone(r.albums)})
	}
	for id, pl := range e.played {
		st.Played = append(st.Played, Played{Unit: id, At: pl.at, Weight: pl.weight})
	}
	slices.SortFunc(st.Played, func(a, b Played) int { return cmp.Compare(a.Unit, b.Unit) })
	for id, at := range e.last {
		st.Last = append(st.Last, Given{Unit: id, At: at})
	}
	slices.SortFunc(st.Last, func(a, b Given) int { return cmp.Compare(a.Unit, b.Unit) })
	return st
}

// ResumeEndless continues the endless shuffle st was taken from.
func ResumeEndless(st *EndlessState) *Endless {
	e := NewEndless(st.Seed, st.First)
	e.position = st.Position
	for _, r := range st.Recent {
		e.w.push(r.Artists, r.Albums)
	}
	for _, pl := range st.Played {
		e.played[pl.Unit] = played{at: pl.At, weight: pl.Weight}
	}
	for _, g := range st.Last {
		e.last[g.Unit] = g.At
	}
	return e
}

// page is what one page of picks works from.
type page struct {
	p            *Pool
	reasons      []Reason // by unit index
	eligible     *fenwick // weights of the units that haven't played this cycle
	playedWeight float64  // weight of the pool's units that have
	now          time.Time
	artistWeight []float64 // by primary artist index: its units' weight
	totalWeight  float64
	turns        []int // by primary artist index: its units given this cycle
	cyclePicks   int   // units given this cycle
	waiting      []int // units that may come due during the page, longest waiting first
}

// page weighs p's units at c.Now and forgets plays older than
// [NoRepeatFor].
func (e *Endless) page(p *Pool, c *Config, s Settings) *page {
	maps.DeleteFunc(e.played, func(_ TrackID, pl played) bool { return c.Now.Sub(pl.at) >= NoRepeatFor })
	pg := &page{
		p: p, reasons: make([]Reason, p.Len()), now: c.Now,
		artistWeight: make([]float64, p.artists), turns: make([]int, p.artists),
	}
	weights := make([]float64, p.Len())
	for i := range p.units {
		u := &p.units[i]
		pg.reasons[i] = Explain(u, s, c.Now)
		w := pg.reasons[i].Weight()
		pg.totalWeight += w
		if len(u.artist) > 0 {
			pg.artistWeight[u.artist[0]] += w
		}
		if pl, ok := e.played[u.ID()]; ok {
			pg.playedWeight += pl.weight
			pg.cyclePicks++
			if len(u.artist) > 0 {
				pg.turns[u.artist[0]]++
			}
			continue
		}
		weights[i] = w
	}
	pg.eligible = newFenwick(weights)
	pg.waiting = e.waiting(p, weights)
	return pg
}

// deadline is how many picks a unit of p may wait before it is due.
func deadline(p *Pool) uint64 { return uint64(dueAfter * p.Len()) } //nolint:gosec // G115: a pool's size is never negative.

// waiting returns the eligible units of p that may come due within a page,
// the longest waiting first. A unit never given has waited since the start.
func (e *Endless) waiting(p *Pool, weights []float64) []int {
	soon := e.position + PageSize
	limit := deadline(p)
	var out []int
	for i := range p.units {
		if weights[i] > 0 && soon >= e.last[p.units[i].ID()]+limit {
			out = append(out, i)
		}
	}
	slices.SortStableFunc(out, func(a, b int) int {
		return cmp.Compare(e.last[p.units[a].ID()], e.last[p.units[b].ID()])
	})
	return out
}

// due returns a unit past its deadline that fits the spacing rules, the
// longest waiting first, or -1 for none.
func (e *Endless) due(pg *page) int {
	limit := deadline(pg.p)
	for _, i := range pg.waiting {
		u := &pg.p.units[i]
		if pg.eligible.weight[i] <= 0 {
			continue
		}
		if e.position < e.last[u.ID()]+limit {
			return -1 // the rest have waited less
		}
		if e.w.fits(pg.p, u) {
			return i
		}
	}
	return -1
}

// turn is how an artist's turn leans a pick of u: above 1 for an artist
// that has had fewer of this cycle's picks than its weight earns, below 1
// for one that has had more, and 1 for a unit without one.
func (pg *page) turn(u *Unit) float64 {
	if len(u.artist) == 0 || pg.totalWeight <= 0 {
		return 1
	}
	a := u.artist[0]
	earned := float64(pg.cyclePicks+1) * pg.artistWeight[a] / pg.totalWeight
	return min(artistLean, max(1/artistLean, (earned+1)/float64(pg.turns[a]+1)))
}

// lead appends the unit holding the song to lead with to out, if there is
// one, and forgets the song.
func (e *Endless) lead(pg *page, s Settings, out []Pick) []Pick {
	i, solo := pg.p.lead(e.first)
	e.first = 0
	switch {
	case solo != nil:
		r := Explain(solo, s, pg.now)
		return append(out, e.record(pg, -1, solo, &r))
	case i >= 0:
		return append(out, e.record(pg, i, &pg.p.units[i], &pg.reasons[i]))
	}
	return out
}

// newCycle lets every unit come up again.
func (e *Endless) newCycle(pg *page) {
	clear(e.played)
	weights := make([]float64, len(pg.reasons))
	for i := range pg.reasons {
		weights[i] = pg.reasons[i].Weight()
	}
	pg.eligible = newFenwick(weights)
	pg.playedWeight = 0
	pg.cyclePicks = 0
	clear(pg.turns)
	pg.waiting = e.waiting(pg.p, weights)
}

// record notes u, unit i of the page's pool or -1 for a lead from outside
// it, as given, and returns its pick.
func (e *Endless) record(pg *page, i int, u *Unit, r *Reason) Pick {
	if i >= 0 {
		pg.eligible.set(i, 0)
	}
	w := r.Weight()
	pg.playedWeight += w
	e.played[u.ID()] = played{at: pg.now, weight: w}
	e.last[u.ID()] = e.position
	pg.cyclePicks++
	if len(u.artist) > 0 && i >= 0 {
		pg.turns[u.artist[0]]++
	}
	e.w.push(u.artists, u.albums)
	e.position++
	return Pick{Unit: u, Reason: *r}
}

// pick draws the next unit: in proportion to its weight times flow's
// factor, among the eligible units that fit the spacing rules. It draws by
// weight and keeps a draw that fits with probability factor/most, which is
// exactly that distribution, and falls back to weighing every eligible unit
// when draws keep failing. If no eligible unit fits, spacing gives way.
func (e *Endless) pick(pg *page, f *flow, rng *rand.Rand) int {
	most := (1 + f.strength*neutral) * artistLean
	for range endlessTries {
		i := pg.eligible.find(rng.Float64() * pg.eligible.total())
		u := &pg.p.units[i]
		if pg.eligible.weight[i] <= 0 || !e.w.fits(pg.p, u) {
			continue
		}
		if rng.Float64()*most < f.factor(u)*pg.turn(u) {
			return i
		}
	}
	return e.weigh(pg, f, rng)
}

// weigh draws the next unit by weighing every eligible one: the same
// distribution as pick, for when drawing keeps failing.
func (e *Endless) weigh(pg *page, f *flow, rng *rand.Rand) int {
	w := pg.eligible.weight
	odds := make([]float64, len(w))
	total := 0.0
	for i := range w {
		if u := &pg.p.units[i]; w[i] > 0 && e.w.fits(pg.p, u) {
			odds[i] = w[i] * f.factor(u) * pg.turn(u)
			total += odds[i]
		}
	}
	if total <= 0 {
		odds, total = w, pg.eligible.total()
	}
	target := rng.Float64() * total
	last := 0
	for i, o := range odds {
		if o <= 0 {
			continue
		}
		if target -= o; target < 0 {
			return i
		}
		last = i
	}
	return last // rounding ran past the end
}
