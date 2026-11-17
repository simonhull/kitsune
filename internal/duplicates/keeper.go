package duplicates

import (
	"slices"
	"time"
)

// bitrateMargin is how far apart two bitrates must be, as a share of the
// higher, before the higher counts: 5%. Two rips of one CD differ by less.
const bitrateMargin = 0.05

// Copy is one copy of a recording as the keeper is chosen from it.
type Copy struct {
	Lossless bool
	// Original says the copy is on an album its own album artist released,
	// that isn't a compilation and isn't tagged a compilation or a
	// soundtrack: not on a best-of or a soundtrack.
	Original bool
	Bitrate  int // In kb/s; 0 when unknown.
	// InPlace says the copy sits where the organizer's template puts it.
	InPlace bool
	Plays   int // Everyone's.
	AddedAt time.Time
}

// Reason is why a copy is the one to keep: a way it beats every other copy.
type Reason string

// The reasons, in the order they decide.
const (
	Lossless Reason = "lossless" // Lossless over lossy.
	Original Reason = "original" // On an original album, over a compilation, best-of or soundtrack.
	Bitrate  Reason = "bitrate"  // A higher bitrate, by over 5%.
	InPlace  Reason = "inPlace"  // Already where the template puts it.
	Plays    Reason = "plays"    // More plays.
	Earlier  Reason = "earlier"  // Added first.
)

// order is the reasons in the order they decide.
//
//nolint:gochecknoglobals // A fixed order, never written: the one place it is said.
var order = []Reason{Lossless, Original, Bitrate, InPlace, Plays, Earlier}

// Keeper suggests which of copies to keep, by index, and why. The order of
// reasons is: lossless over lossy; then a copy on an original album over one
// on a compilation, best-of or soundtrack; then the higher bitrate, counted
// only when it is over 5% higher; then the copy already in place; then more
// plays; then the earlier date added. Copies alike in all of them keep the
// first.
//
// The reasons are each way the keeper beats every other copy, in that order.
// When it beats none of them on every count, the reason is the first that sets
// it apart from the copy that came closest. Copies alike in everything have
// none. It returns -1 for no copies.
func Keeper(copies []Copy) (int, []Reason) {
	if len(copies) == 0 {
		return -1, nil
	}
	best := 0
	for i := 1; i < len(copies); i++ {
		if decides(&copies[i], &copies[best]) > 0 {
			best = i
		}
	}
	return best, reasons(copies, best)
}

// reasons returns why copies[best] is kept.
func reasons(copies []Copy, best int) []Reason {
	var out []Reason
	for _, r := range order {
		if slices.ContainsFunc(indexes(len(copies), best), func(i int) bool { return compare(r, &copies[best], &copies[i]) <= 0 }) {
			continue
		}
		out = append(out, r)
	}
	if len(out) > 0 || len(copies) < 2 {
		return out
	}
	// It beats no copy on every count: say what set it apart from the copy that
	// came closest, the one alike with it the longest.
	last := -1
	for i := range copies {
		if i == best {
			continue
		}
		for n, r := range order {
			if compare(r, &copies[best], &copies[i]) != 0 {
				last = max(last, n)
				break
			}
		}
	}
	if last < 0 {
		return nil
	}
	return []Reason{order[last]}
}

// indexes returns 0 to n-1 without skip.
func indexes(n, skip int) []int {
	out := make([]int, 0, n)
	for i := range n {
		if i != skip {
			out = append(out, i)
		}
	}
	return out
}

// decides compares a and b by every reason in order: positive when a is the
// better one to keep.
func decides(a, b *Copy) int {
	for _, r := range order {
		if c := compare(r, a, b); c != 0 {
			return c
		}
	}
	return 0
}

// compare compares a and b by one reason: positive when a is better by it.
func compare(r Reason, a, b *Copy) int {
	switch r {
	case Lossless:
		return boolCompare(a.Lossless, b.Lossless)
	case Original:
		return boolCompare(a.Original, b.Original)
	case Bitrate:
		return bitrateCompare(a.Bitrate, b.Bitrate)
	case InPlace:
		return boolCompare(a.InPlace, b.InPlace)
	case Plays:
		return a.Plays - b.Plays
	case Earlier:
		return b.AddedAt.Compare(a.AddedAt)
	}
	return 0
}

// boolCompare is positive when a holds and b doesn't.
func boolCompare(a, b bool) int {
	switch {
	case a && !b:
		return 1
	case b && !a:
		return -1
	}
	return 0
}

// bitrateCompare is positive when a is higher than b by over bitrateMargin of
// the higher. An unknown bitrate is never higher.
func bitrateCompare(a, b int) int {
	if a <= 0 || b <= 0 {
		return 0
	}
	hi, lo := max(a, b), min(a, b)
	if float64(hi-lo) <= bitrateMargin*float64(hi) {
		return 0
	}
	if a > b {
		return 1
	}
	return -1
}
