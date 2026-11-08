package shuffle

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Settings are a person's shuffle settings.
type Settings struct {
	// Ratings is how much ratings and hearts matter, from 0 (not at all) to 1.
	Ratings float64
	// Flow is how much a next unit leans toward one similar to the last, from
	// 0 (pure spaced, weighted random) to 1.
	Flow float64
	// Memory is whether skips and listen time count: skip memory, skip rate
	// and listen time.
	Memory bool
}

const (
	// DefaultRatings is the default for [Settings.Ratings].
	DefaultRatings = 0.5
	// DefaultFlow is the default for [Settings.Flow].
	DefaultFlow = 0.3
)

// DefaultSettings are the settings of a person who hasn't changed any.
func DefaultSettings() Settings {
	return Settings{Ratings: DefaultRatings, Flow: DefaultFlow, Memory: true}
}

// clamped returns s with Ratings and Flow kept within 0 to 1; a value that
// isn't a number takes its default.
func (s Settings) clamped() Settings {
	s.Ratings = clamp01(s.Ratings, DefaultRatings)
	s.Flow = clamp01(s.Flow, DefaultFlow)
	return s
}

// clamp01 keeps v within 0 to 1, or returns otherwise for NaN.
func clamp01(v, otherwise float64) float64 {
	if math.IsNaN(v) {
		return otherwise
	}
	return min(1, max(0, v))
}

// The weight's constants. Each comes from the spec
// (specs/2026-11-07-shuffle-design.md, decisions 5 and 5b) unless it says
// otherwise.
const (
	// unratedStars is what an unrated unit weighs as: the middle of the scale,
	// so rating nothing changes nothing.
	unratedStars = 3
	// heartAtFull is a heart's factor at Ratings 1. It is 1.25 at the default
	// Ratings of 0.5, as the spec gives it, and scales the same way the
	// rating curve does: 1.25² = 1.5625.
	heartAtFull = 1.5625

	// recencyDays is the time after a play by which a song is fully back.
	recencyDays = 14
	// recencyPower shapes the way back: (days/14)^0.7 rises fast at first, so
	// a song from last week is already at 0.6.
	recencyPower = 0.7
	// recencyFloor keeps yesterday's song unlikely but possible.
	recencyFloor = 0.05
	// dormantDays is how long unheard a song must be to get dormantLift.
	dormantDays = 180
	// dormantLift lifts never-played and long-unheard songs, so nothing
	// starves.
	dormantLift = 1.2

	// EarlySkip is how soon after its start a skip counts toward skip memory.
	EarlySkip = 30 * time.Second
	// SkipMemoryFor is how long an early skip is remembered.
	SkipMemoryFor = 7 * 24 * time.Hour
	// skipMemoryFactor is one remembered early skip's factor.
	skipMemoryFactor = 0.5
	// skipMemoryMost is how many early skips stack: two give 0.25, as the
	// spec says, and more stack no further. The spec names no limit; without
	// one a bad week could bury a song, and the long-run skip rate is what
	// should say a song is unwanted.
	skipMemoryMost = 2

	// prior is how many completed listens a song is smoothed toward, so one
	// skip never condemns a song.
	prior = 2
	// skipRateFloor is the skip rate factor at a 100% skip rate.
	skipRateFloor = 0.4
	// listenFloor is the listen time factor of a song never heard past its
	// start.
	listenFloor = 0.6

	// hoursPerDay turns durations into the days the recency curve is in.
	hoursPerDay = 24
)

// starCurve is what 1★ to 5★ weigh at Ratings 1. At Ratings 0 each weighs 1,
// and between, a rating weighs its value here to the power of Ratings: a
// straight line in log space.
//
//nolint:gochecknoglobals // A fixed table; Go has no const arrays.
var starCurve = [MaxStars]float64{0.2, 0.6, 1.0, 1.6, 2.4}

// Reason is why a unit weighs what it does, and, for a pick, how flow leaned
// toward it. Every factor is a multiplier; the weight is their product.
type Reason struct {
	// Stars is the rating the unit weighs as: the mean of its rated songs, or
	// 0 for none.
	Stars float64
	// Hearted is whether any song in the unit is hearted.
	Hearted bool
	// Played is whether the unit has ever played.
	Played bool
	// Since is how long ago the unit last played.
	Since time.Duration
	// EarlySkips, Starts and Skips are the counts behind skip memory and skip
	// rate: the most early skips of any song in the unit, and its starts and
	// skips in shuffle. Listens counts the listens behind listen time, and
	// Heard is the share of a song they heard, on average; 0 with none.
	EarlySkips, Starts, Skips, Listens int
	Heard                              float64
	// Rating, Heart, Recency, SkipMemory, SkipRate and Listen are the factors.
	Rating, Heart, Recency, SkipMemory, SkipRate, Listen float64
	// Flow is the factor flow gave the pick, 1 when it gave none.
	Flow float64
	// After is the artist flow leaned from, the last unit's primary artist;
	// meaningful only when Flow isn't 1.
	After ArtistID
	// Similarity is the unit's similarity to After.
	Similarity float64
}

// Weight is the unit's weight: the product of its factors, flow aside.
func (r *Reason) Weight() float64 {
	return r.Rating * r.Heart * r.Recency * r.SkipMemory * r.SkipRate * r.Listen
}

// String gives the factors that moved the weight, for logs and the bench,
// such as "4★ ×1.27, heart ×1.25, played 3d ago ×0.39, flow ×1.12".
func (r *Reason) String() string {
	var b strings.Builder
	add := func(format string, args ...any) {
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, format, args...)
	}
	if r.Stars > 0 {
		add("%.3g★ ×%.2f", r.Stars, r.Rating)
	}
	if r.Hearted {
		add("heart ×%.2f", r.Heart)
	}
	switch {
	case !r.Played:
		add("never played ×%.2f", r.Recency)
	case r.Recency != 1:
		add("played %dd ago ×%.2f", int(r.Since.Hours()/hoursPerDay), r.Recency)
	}
	for _, f := range [...]struct {
		name string
		v    float64
	}{{"early skips", r.SkipMemory}, {"skip rate", r.SkipRate}, {"listen time", r.Listen}} {
		if f.v != 1 {
			add("%s ×%.2f", f.name, f.v)
		}
	}
	if r.Flow != 1 {
		add("flow from %d ×%.2f", r.After, r.Flow)
	}
	if b.Len() == 0 {
		return "no signals"
	}
	return b.String()
}

// Weight is how strongly shuffle favors u at now under s.
func Weight(u *Unit, s Settings, now time.Time) float64 {
	r := Explain(u, s, now)
	return r.Weight()
}

// Explain gives the factors behind u's weight at now under s. Its Flow is 1:
// flow belongs to a pick, and a pick's [Pick].Reason carries it.
func Explain(u *Unit, s Settings, now time.Time) Reason {
	s = s.clamped()
	f := &u.facts
	r := Reason{
		Hearted: f.hearted,
		Rating:  ratingFactor(f, s.Ratings),
		Heart:   1, SkipMemory: 1, SkipRate: 1, Listen: 1, Flow: 1,
		EarlySkips: f.earlySkips, Starts: f.starts, Skips: f.skips, Listens: f.listens,
	}
	if f.listens > 0 {
		r.Heard = f.heard / float64(f.listens)
	}
	if f.rated > 0 {
		r.Stars = float64(f.starSum) / float64(f.rated)
	}
	if f.hearted {
		r.Heart = math.Pow(heartAtFull, s.Ratings)
	}
	r.Recency = dormantLift
	if r.Played = f.lastPlayed != 0; r.Played {
		r.Since = max(0, now.Sub(time.UnixMilli(f.lastPlayed)))
		r.Recency = recency(r.Since)
	}
	if s.Memory {
		r.SkipMemory = math.Pow(skipMemoryFactor, float64(min(f.earlySkips, skipMemoryMost)))
		rate := float64(f.skips) / float64(f.starts+prior)
		r.SkipRate = 1 - (1-skipRateFloor)*rate
		heard := (f.heard + prior) / float64(f.listens+prior)
		r.Listen = listenFloor + (1-listenFloor)*heard
	}
	return r
}

// ratingFactor is the rating curve at ratings, for f's mean stars.
func ratingFactor(f *facts, ratings float64) float64 {
	stars := float64(unratedStars)
	if f.rated > 0 {
		stars = float64(f.starSum) / float64(f.rated)
	}
	lo := int(stars) // 1 to 5: a mean of ratings from 1 to 5
	w := math.Log(starCurve[lo-1])
	if lo < int(MaxStars) {
		frac := stars - float64(lo)
		w += frac * (math.Log(starCurve[lo]) - w)
	}
	return math.Exp(ratings * w)
}

// recency is the recency factor of a unit last played since ago.
func recency(since time.Duration) float64 {
	days := since.Hours() / hoursPerDay
	if days > dormantDays {
		return dormantLift
	}
	return max(recencyFloor, min(1, math.Pow(days/recencyDays, recencyPower)))
}
