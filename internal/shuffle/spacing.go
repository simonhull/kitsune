package shuffle

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
// how many units each has (counts) out of n. A key with k units can keep k
// units g apart only if (k-1)g < n, so a key that holds a large share of a
// small set has its gap scaled down to the most the set allows, and never
// below 1: no rule.
func gaps(counts []int32, gap, n int) []int32 {
	out := make([]int32, len(counts))
	for i, k := range counts {
		g := gap
		if k > 1 {
			g = min(gap, max(1, (n-1)/int(k-1)))
		}
		out[i] = int32(g) //nolint:gosec // G115: g is at most gap, a small constant.
	}
	return out
}
