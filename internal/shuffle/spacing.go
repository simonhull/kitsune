package shuffle

import "slices"

const (
	// ArtistGap is how far apart two units of one artist must be: a unit
	// never shares an artist with any of the ArtistGap-1 before it. From the
	// spec: "the same artist doesn't come within 4 units".
	ArtistGap = 4
	// AlbumGap is the same for albums, from the spec's "the same album
	// within 8".
	AlbumGap = 8
)

// gaps returns, by key, how far apart the units of each key must be, given
// how many units each has (counts) out of n. A key that holds a large share
// of a small set has its gap scaled down to the most that leaves the order
// some freedom, the largest g with k·g < n, and never below 1: no rule.
//
// Tighter gaps exist. On its own a key could keep units (n-1)/(k-1) apart,
// packed from the first slot to the last, and at k·g = n its units could
// still come every g-th slot. But either leaves rigid rotations as the only
// orders, which sound nothing like a shuffle, and two keys held that tight
// leave each other no room: an artist's albums share its turns. In tests,
// the repair met such sets with breaks it could not undo.
func gaps(counts []int32, gap, n int) []int32 {
	out := make([]int32, len(counts))
	for i, k := range counts {
		g := gap
		if k > 1 {
			g = min(gap, max(1, (n-1)/int(k)))
		}
		out[i] = int32(g) //nolint:gosec // G115: g is at most gap, a small constant.
	}
	return out
}

// windowSize is how many placed units spacing looks back over: the most any
// gap can need.
const windowSize = max(ArtistGap, AlbumGap) - 1

// placed is what spacing remembers of a unit after it is placed. It keeps
// ids, not pool indexes, so it holds across pools: an endless shuffle builds
// a new pool for every page.
type placed struct {
	artists []ArtistID
	albums  []AlbumID
}

// window is the last few units placed, for spacing.
type window struct {
	recent [windowSize]placed
	n      int // how many of recent are filled
	head   int // where the next one goes
}

// push records a unit with artists and albums as the latest placed.
func (w *window) push(artists []ArtistID, albums []AlbumID) {
	w.recent[w.head] = placed{artists: artists, albums: albums}
	w.head = (w.head + 1) % windowSize
	w.n = min(w.n+1, windowSize)
}

// at returns the unit placed d units ago, d from 1 to w.n.
func (w *window) at(d int) *placed {
	return &w.recent[(w.head-d+windowSize)%windowSize]
}

// last returns the primary artist of the latest unit placed, or false.
func (w *window) last() (ArtistID, bool) {
	if w.n == 0 || len(w.at(1).artists) == 0 {
		return 0, false
	}
	return w.at(1).artists[0], true
}

// fits reports whether u may be placed next under p's spacing rules.
func (w *window) fits(p *Pool, u *Unit) bool {
	for d := 1; d <= w.n; d++ {
		prev := w.at(d)
		if clashes(p, u, prev.artists, prev.albums, d) {
			return false
		}
	}
	return true
}

// clashes reports whether u, placed d units after a unit with artists and
// albums, comes too close to it under p's spacing rules. u must be one of
// p's units.
func clashes(p *Pool, u *Unit, artists []ArtistID, albums []AlbumID, d int) bool {
	for k, a := range u.artists {
		if d < int(p.artistGap[u.artist[k]]) && slices.Contains(artists, a) {
			return true
		}
	}
	for k, a := range u.albums {
		if d < int(p.albumGap[u.album[k]]) && slices.Contains(albums, a) {
			return true
		}
	}
	return false
}
