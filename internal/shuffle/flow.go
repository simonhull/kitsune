package shuffle

import (
	"math"
	"time"
)

// Similarity says how alike two artists are, from 0 (nothing alike) to 1.
// A score outside that range is clamped, and one that isn't a number counts
// as 0.5: no lean either way.
type Similarity interface {
	Artist(a, b ArtistID) float64
}

// Config is what an order is worked out from, besides the units and seed.
type Config struct {
	// Settings are the person's settings.
	Settings Settings
	// Similarity scores artists for flow; nil turns flow off.
	Similarity Similarity
	// Now is the present, for recency.
	Now time.Time
}

// Pick is one unit in an order, with why it came up.
type Pick struct {
	// Unit is the unit picked. It belongs to the pool it was picked from.
	Unit *Unit
	// Reason is why: the unit's weight factors, and flow's lean.
	Reason Reason
}

// neutral is the similarity that leaves a weight as it is.
const neutral = 0.5

// flow works out flow's factor for candidates after the last unit placed.
type flow struct {
	strength float64    // Settings.Flow; 0 for off
	sim      Similarity // nil for off
	after    ArtistID   // the last unit's primary artist
	on       bool       // whether flow leans at all for this pick
}

// newFlow is flow after the unit whose primary artist is after, if any.
func newFlow(c *Config, strength float64, after ArtistID, hasAfter bool) flow {
	return flow{
		strength: strength, sim: c.Similarity, after: after,
		on: strength > 0 && c.Similarity != nil && hasAfter,
	}
}

// similarity is u's similarity to the last unit, clamped to 0 to 1, or
// neutral when flow is off or u has no artist.
func (f *flow) similarity(u *Unit) float64 {
	a, ok := u.primary()
	if !f.on || !ok {
		return neutral
	}
	s := f.sim.Artist(f.after, a)
	if math.IsNaN(s) {
		return neutral
	}
	return min(1, max(0, s))
}

// factor is flow's factor for u: the spec's 1 + flow × (similarity − 0.5),
// from 1 − flow/2 for nothing alike to 1 + flow/2 for the most alike.
func (f *flow) factor(u *Unit) float64 {
	return 1 + f.strength*(f.similarity(u)-neutral)
}

// lean records flow's factor for u in r.
func (f *flow) lean(r *Reason, u *Unit) {
	if !f.on {
		return
	}
	r.Similarity = f.similarity(u)
	r.Flow = 1 + f.strength*(r.Similarity-neutral)
	r.After = f.after
}
