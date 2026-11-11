package bench

import (
	"math"
	"math/rand/v2"
	"slices"

	"github.com/simonhull/kitsune/internal/shuffle"
)

// item is one unit of a played sequence, the shuffle's or the naive one's.
type item struct {
	tracks  []shuffle.TrackID
	artists []shuffle.ArtistID
	albums  []shuffle.AlbumID
	stars   int // in whole stars, a half rounded up; 0 for unrated
}

// primary is the item's primary artist.
func (it *item) primary() shuffle.ArtistID {
	if len(it.artists) == 0 {
		return 0
	}
	return it.artists[0]
}

// fromPicks turns the shuffle's picks into items.
func fromPicks(picks []shuffle.Pick) []item {
	out := make([]item, len(picks))
	for i := range picks {
		u := picks[i].Unit
		out[i] = item{
			tracks: u.Tracks(), artists: u.Artists(), albums: u.Albums(),
			stars: int(math.Round(picks[i].Reason.Stars)),
		}
	}
	return out
}

// fromTracks turns songs into one item each: the naive shuffle's units.
func fromTracks(lib *library, ids []shuffle.TrackID) []item {
	out := make([]item, len(ids))
	for i, id := range ids {
		t := &lib.tracks[id-1]
		out[i] = item{
			tracks: []shuffle.TrackID{id}, artists: t.Artists,
			albums: []shuffle.AlbumID{t.Album}, stars: (int(t.Stars) + 1) / 2,
		}
	}
	return out
}

// adjacency is the spec's adjacency: the items that share an artist with
// one of the shuffle.ArtistGap-1 before them, and those that share an album
// with one of the shuffle.AlbumGap-1 before them.
type adjacency struct{ artists, albums int }

// near counts seq's adjacency.
func near(seq []item) adjacency {
	var a adjacency
	for k := range seq {
		if sharesBack(seq, k, shuffle.ArtistGap, func(x, y *item) bool { return overlaps(x.artists, y.artists) }) {
			a.artists++
		}
		if sharesBack(seq, k, shuffle.AlbumGap, func(x, y *item) bool { return overlaps(x.albums, y.albums) }) {
			a.albums++
		}
	}
	return a
}

// sharesBack reports whether seq[k] shares with one of the gap-1 before it.
func sharesBack(seq []item, k, gap int, shares func(a, b *item) bool) bool {
	for d := 1; d < gap && d <= k; d++ {
		if shares(&seq[k], &seq[k-d]) {
			return true
		}
	}
	return false
}

// overlaps reports whether a and b hold a value in common.
func overlaps[T comparable](a, b []T) bool {
	return slices.ContainsFunc(a, func(x T) bool { return slices.Contains(b, x) })
}

// streak is the longest run of consecutive items by one primary artist.
func streak(seq []item) int {
	longest, run := 0, 0
	for k := range seq {
		if k > 0 && seq[k].primary() == seq[k-1].primary() {
			run++
		} else {
			run = 1
		}
		longest = max(longest, run)
	}
	return longest
}

// gapSpread is how unevenly a finite order spreads each artist: the
// coefficient of variation of the gaps between an artist's items, averaged
// over artists with at least three, weighted by their items. Perfectly even
// is 0; a uniformly random order is about 1.
func gapSpread(seq []item) float64 {
	at := map[shuffle.ArtistID][]int{}
	for k := range seq {
		at[seq[k].primary()] = append(at[seq[k].primary()], k)
	}
	sum, weight := 0.0, 0
	for _, pos := range at {
		if len(pos) < 3 {
			continue
		}
		gaps := make([]float64, len(pos)-1)
		for i := range gaps {
			gaps[i] = float64(pos[i+1] - pos[i])
		}
		sum += cv(gaps) * float64(len(pos))
		weight += len(pos)
	}
	if weight == 0 {
		return 0
	}
	return sum / float64(weight)
}

// cv is the coefficient of variation of xs.
func cv(xs []float64) float64 {
	mean := 0.0
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	v := 0.0
	for _, x := range xs {
		v += (x - mean) * (x - mean)
	}
	return math.Sqrt(v/float64(len(xs))) / mean
}

// positions adds, by stars, the place (0 to 1) of each item in a finite
// order to sum, and counts them in n.
func positions(seq []item, sum *[wholeStars + 1]float64, n *[wholeStars + 1]int) {
	for k := range seq {
		sum[seq[k].stars] += float64(k) / float64(max(1, len(seq)-1))
		n[seq[k].stars]++
	}
}

// expected is what the rules say a library's units are: each song's run,
// and the songs that must never come up.
type expected struct {
	run    map[shuffle.TrackID]shuffle.Run // a run member's whole run
	banned map[shuffle.TrackID]bool
}

// expect works out lib's rules from the library alone, not from the
// shuffle, so the bench checks the shuffle's units rather than repeating
// them.
func expect(lib *library) expected {
	e := expected{run: map[shuffle.TrackID]shuffle.Run{}, banned: map[shuffle.TrackID]bool{}}
	for _, run := range lib.runs {
		allSkipped := true
		for _, id := range run {
			e.run[id] = run
			allSkipped = allSkipped && lib.tracks[id-1].Skipped
		}
		if allSkipped {
			for _, id := range run {
				e.banned[id] = true
			}
		}
	}
	for i := range lib.tracks {
		if t := &lib.tracks[i]; t.Skipped && e.run[t.ID] == nil {
			e.banned[t.ID] = true
		}
	}
	return e
}

// ruled is what rules counts.
type ruled struct{ intact, broken, banned int }

// rules counts, over seq's songs in play order, the runs played whole and
// in order from their first song, the run songs played any other way, and
// the songs played that must never come up.
func (e *expected) rules(seq []item) ruled {
	var r ruled
	var songs []shuffle.TrackID
	for k := range seq {
		songs = append(songs, seq[k].tracks...)
	}
	for k := 0; k < len(songs); k++ {
		id := songs[k]
		if e.banned[id] {
			r.banned++
		}
		run := e.run[id]
		if run == nil {
			continue
		}
		if id == run[0] && k+len(run) <= len(songs) && slices.Equal(songs[k:k+len(run)], run) {
			r.intact++
			k += len(run) - 1
			continue
		}
		r.broken++
	}
	return r
}

// neighbors is the mean similarity of each item's primary artist to the
// one before.
func neighbors(seq []item, sim shuffle.Similarity) float64 {
	sum := 0.0
	for k := 1; k < len(seq); k++ {
		sum += sim.Artist(seq[k-1].primary(), seq[k].primary())
	}
	return sum / float64(max(1, len(seq)-1))
}

// baseline is the mean similarity of random pairs of seq's items by
// different artists: what neighbors would be with no lean at all.
func baseline(seq []item, sim shuffle.Similarity, r *rand.Rand) float64 {
	const pairs = 20_000
	sum, n := 0.0, 0
	for range pairs {
		a, b := &seq[r.IntN(len(seq))], &seq[r.IntN(len(seq))]
		if a.primary() == b.primary() {
			continue
		}
		sum += sim.Artist(a.primary(), b.primary())
		n++
	}
	return sum / float64(max(1, n))
}

// covered is what coverage measures.
type covered struct {
	share       float64 // of units played in the first pool-size picks
	wait, never int     // the longest wait, in picks; units never played
}

// coverage is the share of units played in the first size picks, the
// longest any unit waited between plays (or from the start), in picks, and
// how many units never played. Units are told apart by their first song.
func coverage(seq []item, size int) covered {
	share, wait := 0.0, 0
	last := map[shuffle.TrackID]int{}
	for k := range seq {
		id := seq[k].tracks[0]
		prev, ok := last[id]
		if !ok {
			prev = -1
			if k < size {
				share++
			}
		}
		wait = max(wait, k-prev)
		last[id] = k
	}
	for _, k := range last {
		wait = max(wait, len(seq)-k)
	}
	return covered{share: share / float64(size), wait: wait, never: size - len(last)}
}

// fit is what fidelity measures.
type fit struct {
	tv             float64 // total variation distance, 0 when plays follow weight exactly
	plays, weights float64 // a 5★ unit's play rate over a 1★ unit's, and their weights' ratio
}

// fidelity compares plays by stars with the share of weight each rating
// holds: the total variation distance between the two, and the play rate
// of a 5★ unit over a 1★ unit's next to the same ratio of their weights.
func fidelity(seq []item, weight *[wholeStars + 1]float64, units *[wholeStars + 1]int) fit {
	tv := 0.0
	var played [wholeStars + 1]float64
	for k := range seq {
		played[seq[k].stars]++
	}
	total := 0.0
	for _, w := range weight {
		total += w
	}
	for c := range played {
		tv += math.Abs(played[c]/float64(len(seq))-weight[c]/total) / 2
	}
	rate := func(v *[wholeStars + 1]float64, c int) float64 { return v[c] / float64(max(1, units[c])) }
	return fit{tv: tv, plays: rate(&played, 5) / rate(&played, 1), weights: rate(weight, 5) / rate(weight, 1)}
}
