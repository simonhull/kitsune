package shuffler

import (
	"time"

	"github.com/simonhull/kitsune/internal/shuffle"
)

// Why is why a unit came up: the engine's reason, as plain values. Each factor
// multiplies the unit's weight; a client words them, and the server never does.
type Why struct {
	// Stars is the rating the unit weighs as, the mean of its rated songs; 0
	// for unrated. Rating is its factor.
	Stars  float64 `json:"stars"`
	Rating float64 `json:"rating"`
	// Hearted is whether any song in it is hearted, and Heart its factor.
	Hearted bool    `json:"hearted"`
	Heart   float64 `json:"heart"`
	// Played is whether it has come up before, SinceDays how many days ago it
	// last did, and Recency their factor.
	Played    bool    `json:"played"`
	SinceDays int     `json:"sinceDays"`
	Recency   float64 `json:"recency"`
	// Memory is whether skips and listen time count, as the person set it.
	Memory bool `json:"memory"`
	// EarlySkips are its early skips this week, and SkipMemory their factor.
	EarlySkips int     `json:"earlySkips"`
	SkipMemory float64 `json:"skipMemory"`
	// Starts and Skips are its starts in shuffle and how many were skipped, and
	// SkipRate their factor.
	Starts   int     `json:"starts"`
	Skips    int     `json:"skips"`
	SkipRate float64 `json:"skipRate"`
	// Listens are its listens that played or were skipped, Heard the share of
	// it they heard on average, and Listen their factor.
	Listens int     `json:"listens"`
	Heard   float64 `json:"heard"`
	Listen  float64 `json:"listen"`
	// Flow is the factor flow gave it, 1 for none; After is the artist it
	// leaned from (a row ID, 0 for none) and Similarity how alike they are.
	Flow       float64 `json:"flow"`
	After      int64   `json:"after,omitzero"`
	Similarity float64 `json:"similarity,omitzero"`
}

// hoursPerDay turns a duration into days.
const hoursPerDay = 24

// whyOf is r in plain values.
func whyOf(r *shuffle.Reason, memory bool) Why {
	w := Why{
		Stars: r.Stars, Rating: r.Rating, Hearted: r.Hearted, Heart: r.Heart,
		Played: r.Played, Recency: r.Recency, Memory: memory,
		EarlySkips: r.EarlySkips, SkipMemory: r.SkipMemory,
		Starts: r.Starts, Skips: r.Skips, SkipRate: r.SkipRate,
		Listens: r.Listens, Heard: r.Heard, Listen: r.Listen, Flow: r.Flow,
	}
	if r.Played {
		w.SinceDays = int(r.Since / (hoursPerDay * time.Hour))
	}
	if r.Flow != 1 {
		w.After, w.Similarity = int64(r.After), r.Similarity
	}
	return w
}
