package shuffle

import (
	"math"
	"slices"
)

// The repair's constants: set by hand, and checked on the bench
// (internal/shuffle/bench), which fails if spacing breaks or flow stops
// leaning.
const (
	// flowChoices is how many of the next units that fit flow chooses
	// among. One would be no choice at all.
	flowChoices = 4
	// flowRankDecay is what each place further down the order costs a
	// candidate, so flow bends the weighted order rather than replacing it.
	flowRankDecay = 0.8

	// scanLimit is how far down the order the repair looks for a unit that
	// fits before it gives up and places the next unit anyway. It bounds the
	// work when nothing fits, as at the end of a set that is all one artist.
	scanLimit = 512
)

// arranger places a finite order's units one by one under spacing, from a
// linked list of the units not yet placed, in order.
type arranger struct {
	p          *Pool
	c          *Config
	strength   float64 // Settings.Flow
	next, prev []int32 // by unit index; -1 at the ends
	head       int32   // the first unit not yet placed; -1 when all are
	left       int     // units not yet placed
	w          window
	flow       flow // flow after the last unit placed
	out        []Pick
}

// newArranger lists order's units as not yet placed, for an order of total
// units.
func newArranger(p *Pool, c *Config, strength float64, order []int32, total int) *arranger {
	n := len(p.units)
	a := &arranger{
		p: p, c: c, strength: strength,
		next: make([]int32, n), prev: make([]int32, n), head: -1,
		left: n, out: make([]Pick, 0, total),
	}
	for _, i := range slices.Backward(order) {
		a.next[i], a.prev[i] = a.head, -1
		if a.head >= 0 {
			a.prev[a.head] = i
		}
		a.head = i
	}
	return a
}

// place adds u to the order: unit i of the pool, or -1 for a lead from
// outside it.
func (a *arranger) place(i int32, u *Unit, r *Reason) {
	if i >= 0 {
		a.unlink(i)
		a.left--
	}
	a.out = append(a.out, Pick{Unit: u, Reason: *r})
	a.w.push(u.artists, u.albums)
	after, ok := a.w.last()
	a.flow = newFlow(a.c, a.strength, after, ok)
}

// unlink takes unit i off the list.
func (a *arranger) unlink(i int32) {
	if a.prev[i] >= 0 {
		a.next[a.prev[i]] = a.next[i]
	} else {
		a.head = a.next[i]
	}
	if a.next[i] >= 0 {
		a.prev[a.next[i]] = a.prev[i]
	}
}

// choose returns the unit to place next: the first that fits the spacing
// rules, which moves a unit that breaks one to the nearest later slot where
// it doesn't. With flow on, it is instead the best of the first few that
// fit, by flow's factor discounted by how far down each one is. If nothing
// fits, it is the first unit, for polish to fix.
func (a *arranger) choose() int32 {
	choices := 1
	if a.flow.on {
		choices = flowChoices
	}
	best, bestScore := int32(-1), math.Inf(-1)
	discount := 1.0
	found, scanned := 0, 0
	for i := a.head; i >= 0 && found < choices && scanned < scanLimit; i = a.next[i] {
		scanned++
		u := &a.p.units[i]
		if !a.w.fits(a.p, u) {
			continue
		}
		if score := a.flow.factor(u) * discount; score > bestScore {
			best, bestScore = i, score
		}
		discount *= flowRankDecay
		found++
	}
	if best < 0 {
		return a.head
	}
	return best
}
