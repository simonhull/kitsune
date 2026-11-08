package bench

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/shuffle"
)

// flowTolerance is how far neighbor similarity with flow off may sit from
// the random baseline: sampling noise over the bench's picks.
const flowTolerance = 0.02

// flow reports how far flow leans neighbors together, against a random
// pairing of the same picks.
func flow(t *testing.T, b *strings.Builder, lib *library) {
	t.Helper()
	tb := newTable(b, "Flow: neighbor similarity ("+lib.name+")", "mode", "flow", "neighbors", "random pairs", "lift")
	p := pool(t, lib)
	naive := fromTracks(lib, naivePasses(lib, len(lib.tracks), 13))
	tb.row("naive", "–", num(neighbors(naive, lib.sim)), num(baseline(naive, lib.sim, rng(13, 3))), "–")
	for _, mode := range []string{"endless", "finite"} {
		prev := -1.0
		for _, f := range []float64{0, shuffle.DefaultFlow, 1} {
			s := shuffle.Settings{Ratings: shuffle.DefaultRatings, Flow: f, Memory: true}
			var seq []item
			if mode == "endless" {
				seq = fromPicks(newSession(t, lib, s, lib.sim, neutral{}, 13).play(len(lib.tracks)))
			} else {
				c := shuffle.Config{Settings: s, Similarity: lib.sim, Now: now}
				for seed := range uint64(5) {
					seq = append(seq, fromPicks(shuffle.Finite(p, c, seed, 0))...)
				}
			}
			got, base := neighbors(seq, lib.sim), baseline(seq, lib.sim, rng(13, 4))
			tb.row(mode, num(f), num(got), num(base), num(got-base))
			if f == 0 && math.Abs(got-base) > flowTolerance {
				t.Errorf("%s flow 0: neighbors %.3f, baseline %.3f: flow off must be neutral", mode, got, base)
			}
			if got <= prev {
				t.Errorf("%s flow %v: neighbors %.3f, no more than at less flow", mode, f, got)
			}
			prev = got
		}
	}
	tb.note("The bench's similarity places artists on a circle, standing in for genre; a random pair scores" +
		" about 0.25. The naive shuffle's neighbors include the same artist back to back, which scores 1.")
}

// share is the share of seq's songs that are in set.
func share(seq []item, set map[shuffle.TrackID]bool) float64 {
	n, in := 0, 0
	for k := range seq {
		for _, id := range seq[k].tracks {
			n++
			if set[id] {
				in++
			}
		}
	}
	return float64(in) / float64(max(1, n))
}

// mean is the mean of f over rs.
func mean(rs []shuffle.Reason, f func(r *shuffle.Reason) float64) float64 {
	sum := 0.0
	for i := range rs {
		sum += f(&rs[i])
	}
	return sum / float64(max(1, len(rs)))
}

// some picks about a tenth of lib's songs, at random.
func some(lib *library, seed uint64) map[shuffle.TrackID]bool {
	r := rng(seed, 5)
	set := map[shuffle.TrackID]bool{}
	for i := range lib.tracks {
		if r.Float64() < 0.1 {
			set[lib.tracks[i].ID] = true
		}
	}
	return set
}

// signals reports how weights answer simulated listeners, and recover. It
// runs on a library small enough that a phase goes round it several times:
// the no-repeat window lets a song come up about once a pass, so a signal
// can only show from one pass to the next.
func signals(t *testing.T, b *strings.Builder) {
	t.Helper()
	lib := generate(shape{name: "signals", songs: 400, artists: 40, seed: 6})
	tb := newTable(b, "Signals: simulated listeners (400 songs, 40 artists)",
		"listener", "phase", "share of plays from their set", "skip memory", "skip rate", "listen time")
	const picks = 4000 // ten passes
	set := some(&lib, 14)
	s := shuffle.DefaultSettings()
	// Each phase is measured over its second half, once the signal has had
	// time to build.
	phase := func(ss *session) float64 {
		ss.play(picks / 2)
		return share(fromPicks(ss.play(picks/2)), set)
	}
	neutralShare := phase(newSession(t, &lib, s, nil, neutral{}, 14))
	tb.row("neutral", "always", num(neutralShare), "1.000", "1.000", "1.000")

	who := &skipper{dislikes: set, active: true}
	sk := newSession(t, &lib, s, nil, who, 14)
	skipping := phase(sk)
	during := sk.reasons(set)
	who.active = false
	recovered := phase(sk)
	after := sk.reasons(set)
	row := func(listener, phase string, sh float64, rs []shuffle.Reason) {
		tb.row(listener, phase, num(sh),
			num(mean(rs, func(r *shuffle.Reason) float64 { return r.SkipMemory })),
			num(mean(rs, func(r *shuffle.Reason) float64 { return r.SkipRate })),
			num(mean(rs, func(r *shuffle.Reason) float64 { return r.Listen })))
	}
	row("skipper", "skips its set within 12 s", skipping, during)
	row("skipper", "then stops skipping", recovered, after)

	half := newSession(t, &lib, s, nil, halfListener{set: set}, 14)
	halfShare := phase(half)
	halves := half.reasons(set)
	row("half-listener", "moves on halfway through its set", halfShare, halves)

	checkSignals(t, neutralShare, skipping, recovered, halfShare, during, after, halves)
	tb.note(fmt.Sprintf("Each phase is %d picks, 50 a day, measured over its second half; each listener's set is"+
		" the same tenth of the songs. Factors are means over the set's units that have come up, at the end of the phase.", picks))
}

// checkSignals fails the test if the weights don't answer the listeners
// within the spec's ranges, or don't recover.
func checkSignals(t *testing.T, neutralShare, skipping, recovered, halfShare float64, during, after, halves []shuffle.Reason) {
	t.Helper()
	if skipping >= neutralShare || halfShare >= neutralShare {
		t.Errorf("skipped sets kept their share: neutral %.3f, skipper %.3f, half-listener %.3f", neutralShare, skipping, halfShare)
	}
	if recovered <= skipping {
		t.Errorf("the skipper's set didn't recover: %.3f while skipping, %.3f after", skipping, recovered)
	}
	for _, rs := range [][]shuffle.Reason{during, after, halves} {
		inRange(t, rs)
	}
	recovers(t, during, after, halves)
}

// inRange fails the test for a factor outside the spec's range.
func inRange(t *testing.T, rs []shuffle.Reason) {
	t.Helper()
	for i := range rs {
		r := &rs[i]
		if r.SkipMemory < 0.25 || r.SkipMemory > 1 || r.SkipRate < 0.4 || r.SkipRate > 1 || r.Listen < 0.6 || r.Listen > 1 {
			t.Fatalf("a factor out of the spec's range: %s", r.String())
		}
	}
}

// recovers fails the test if skip memory and skip rate don't recover once
// the skips stop, or listen time doesn't answer songs left halfway.
func recovers(t *testing.T, during, after, halves []shuffle.Reason) {
	t.Helper()
	if m := mean(after, func(r *shuffle.Reason) float64 { return r.SkipMemory }); m != 1 {
		t.Errorf("skip memory still %.3f a month after the skips stopped", m)
	}
	if mean(after, func(r *shuffle.Reason) float64 { return r.SkipRate }) <= mean(during, func(r *shuffle.Reason) float64 { return r.SkipRate }) {
		t.Error("the skip rate didn't recover once the skips stopped")
	}
	if mean(halves, func(r *shuffle.Reason) float64 { return r.Listen }) >= 1 {
		t.Error("listen time didn't answer songs left halfway")
	}
}

// explain logs why the first picks of a page came up.
func explain(t *testing.T, b *strings.Builder, lib *library) {
	t.Helper()
	tb := newTable(b, "Why it came up ("+lib.name+", first page, default settings)", "#", "unit", "weight", "reason")
	s := newSession(t, lib, shuffle.DefaultSettings(), lib.sim, neutral{}, 15)
	for i, pk := range s.play(10) {
		tb.row(strconv.Itoa(i+1), fmt.Sprint(pk.Unit.Tracks()), num(pk.Reason.Weight()), pk.Reason.String())
	}
}
