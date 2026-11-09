package matching

import (
	"slices"
	"time"
)

// Album is what Kitsune holds of an album, as matching needs it.
type Album struct {
	Title   string
	Artists []string // The album artists, or the first track's artists.
	Tracks  []Track  // In album order.
}

// Track is one live track of an [Album].
type Track struct {
	ID           string // The track's ULID.
	Disc, Number int    // 0 is unknown.
	Title        string
	Length       time.Duration
	ReleaseID    string // The release MBID its file carries, or "".
}

// discs returns how many discs the album's tracks say it has: at least one.
func (a *Album) discs() int {
	n := 1
	for i := range a.Tracks {
		n = max(n, a.Tracks[i].Disc)
	}
	return n
}

// Pair is one album track and the release track it would take its tags from.
type Pair struct {
	Track   int // Index into the album's tracks.
	Release int // Index into the release's tracks, medium by medium; -1 for none.
}

// agreeWithin is how far apart two lengths may be and still agree.
const agreeWithin = 3 * time.Second

// lengthsAgree reports whether two known lengths agree.
func lengthsAgree(a, b time.Duration) bool {
	return a > 0 && b > 0 && (a-b).Abs() <= agreeWithin
}

// How a pairing weighs a title against a length.
const (
	titleWeight  = 0.6
	lengthWeight = 0.4
	// anchorAt is the least likeness two tracks have for a pairing to build on
	// them; the tracks between anchors pair in order.
	anchorAt = 0.45
	// farApart is where two lengths stop being any evidence at all.
	farApart = 30 * time.Second
)

// likeness returns how alike an album track and a release track are, from 0 to 1.
func likeness(t *Track, r *ReleaseTrack) float64 {
	length := 0.5 // Unknown.
	if t.Length > 0 && r.Length > 0 {
		gap := (t.Length - r.Length).Abs()
		switch {
		case gap <= agreeWithin:
			length = 1
		case gap >= farApart:
			length = 0
		default:
			length = 1 - float64(gap-agreeWithin)/float64(farApart-agreeWithin)
		}
	}
	return titleWeight*similarity(t.Title, r.Title) + lengthWeight*length
}

// pairs finds, for each album track, the release track it takes its tags
// from, keeping both lists in order. With as many tracks on each side, they
// pair in order: a swap shows as two pairs whose lengths differ, for a person
// to re-pair, and nothing moves unasked. Otherwise the tracks that are alike
// pair first, as the best-scoring alignment of the two lists, since a release
// may have tracks the album lacks and the other way round; between two of
// those, the tracks left on both sides pair in order.
func pairs(album *Album, release []flat) []Pair {
	out := make([]Pair, len(album.Tracks))
	for i := range out {
		out[i] = Pair{Track: i, Release: -1}
		if len(release) == len(out) {
			out[i].Release = i
		}
	}
	if len(release) == len(out) {
		return out
	}
	from := Pair{Track: -1, Release: -1}
	for _, anchor := range append(anchors(album, release), Pair{Track: len(album.Tracks), Release: len(release)}) {
		for i, j := from.Track+1, from.Release+1; i < anchor.Track && j < anchor.Release; i, j = i+1, j+1 {
			out[i].Release = j
		}
		if anchor.Track < len(out) {
			out[anchor.Track].Release = anchor.Release
		}
		from = anchor
	}
	return out
}

// anchors returns the pairs alike enough to build on, in order: the alignment
// of the two lists that has the greatest likeness in all.
func anchors(album *Album, release []flat) []Pair {
	n, m := len(album.Tracks), len(release)
	// best[i][j] is the most likeness the first i album tracks and the first j release tracks hold.
	best := make([][]float64, n+1)
	for i := range best {
		best[i] = make([]float64, m+1)
	}
	like := func(i, j int) float64 {
		if l := likeness(&album.Tracks[i], release[j].track); l >= anchorAt {
			return l
		}
		return -1 // Never better than leaving them apart.
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			best[i][j] = max(best[i-1][j], best[i][j-1], best[i-1][j-1]+like(i-1, j-1))
		}
	}
	var out []Pair
	for i, j := n, m; i > 0 && j > 0; {
		switch best[i][j] {
		case best[i-1][j]:
			i--
		case best[i][j-1]:
			j--
		default:
			out = append(out, Pair{Track: i - 1, Release: j - 1})
			i, j = i-1, j-1
		}
	}
	slices.Reverse(out)
	return out
}
