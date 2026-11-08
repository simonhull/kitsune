package bench

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/simonhull/kitsune/internal/shuffle"
)

// now is the bench's present.
var now = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) //nolint:gochecknoglobals // A fixed instant for the bench.

const day = 24 * time.Hour

// shape is how a generated library looks.
type shape struct {
	name         string
	songs        int
	artists      int
	skew         float64 // Zipf exponent of songs per artist: 0 for even
	compilations float64 // share of songs on compilations of many artists
	runs         float64 // share of songs in runs
	skipped      float64 // share of the other songs skipped in shuffle
	seed         uint64
}

// shapes are the bench's libraries, from the spec: skewed, compilation-heavy,
// a tiny album, run-heavy, and 50,000 songs.
func shapes() []shape {
	return []shape{
		{name: "skewed", songs: 3000, artists: 150, skew: 1.1, runs: 0.03, skipped: 0.02, seed: 1},
		{name: "compilations", songs: 2000, artists: 400, skew: 0.6, compilations: 0.5, runs: 0.02, skipped: 0.02, seed: 2},
		{name: "tiny album", songs: 9, artists: 1, seed: 3},
		{name: "many runs", songs: 1500, artists: 80, skew: 0.8, runs: 0.35, skipped: 0.06, seed: 4},
		{name: "50k", songs: 50_000, artists: 3000, skew: 0.9, compilations: 0.1, runs: 0.02, skipped: 0.02, seed: 5},
	}
}

// library is a generated library.
type library struct {
	name   string
	tracks []shuffle.Track
	runs   []shuffle.Run
	sim    ring
	taste  []float64 // by artist: the rating its songs tend to get
}

// rng returns a repeatable generator for seed and a stream.
func rng(seed, stream uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, stream)) //nolint:gosec // G404: the bench must repeat.
}

// generate builds a library of shape sh.
func generate(sh shape) library {
	r := rng(sh.seed, 0)
	lib := library{name: sh.name, sim: newRing(sh.artists, r), taste: make([]float64, sh.artists+1)}
	for a := range lib.taste {
		lib.taste[a] = float64(stars(r))
	}
	zipf := artistPicker(sh.artists, sh.skew, r)
	compilations := int(float64(sh.songs) * sh.compilations)
	nextAlbum := shuffle.AlbumID(1)
	// Albums by one artist each: the artist picked by skew, 8 to 14 songs.
	for len(lib.tracks) < sh.songs-compilations {
		artist := zipf()
		size := min(8+r.IntN(7), sh.songs-compilations-len(lib.tracks))
		for range size {
			lib.add(r, []shuffle.ArtistID{artist}, nextAlbum)
		}
		nextAlbum++
	}
	// Compilations: 15 songs each, every song its own artist.
	for len(lib.tracks) < sh.songs {
		for range min(15, sh.songs-len(lib.tracks)) {
			lib.add(r, []shuffle.ArtistID{zipf()}, nextAlbum)
		}
		nextAlbum++
	}
	lib.link(r, sh.runs)
	lib.skip(r, sh.skipped)
	return lib
}

// add appends a song by artists on album, with a rating, heart and history
// drawn the way a real library's fall: most unrated or middling, rated near
// what the person thinks of the artist, a few hearted, most played at some
// point in the last year.
func (l *library) add(r *rand.Rand, artists []shuffle.ArtistID, album shuffle.AlbumID) {
	t := shuffle.Track{
		ID:      shuffle.TrackID(len(l.tracks) + 1),
		Artists: artists,
		Album:   album,
		Hearted: r.Float64() < 0.06,
	}
	if r.Float64() < 0.6 {
		t.Stars = shuffle.Stars(min(5, max(1, math.Round(l.taste[artists[0]]+0.8*r.NormFloat64()))))
	}
	if r.Float64() < 0.6 {
		t.History.LastPlayed = now.Add(-time.Duration(r.Float64() * 400 * float64(day)))
	}
	l.tracks = append(l.tracks, t)
}

// stars draws a rating, as people rate artists: mostly 3 and 4.
func stars(r *rand.Rand) shuffle.Stars {
	x := r.Float64()
	for i, edge := range [...]float64{0.05, 0.2, 0.55, 0.85} {
		if x < edge {
			return shuffle.Stars(i + 1)
		}
	}
	return shuffle.MaxStars
}

// link makes runs of 2 to 5 neighboring songs on an album, about share of
// all songs; one in ten carries on onto the next album.
func (l *library) link(r *rand.Rand, share float64) {
	want := int(share * float64(len(l.tracks)))
	linked := 0
	for i := 0; i+1 < len(l.tracks) && linked < want; {
		if r.Float64() > 2*share {
			i++
			continue
		}
		size := min(2+r.IntN(4), len(l.tracks)-i)
		run := make(shuffle.Run, 0, size)
		for j := i; j < i+size; j++ {
			if l.tracks[j].Album != l.tracks[i].Album && r.Float64() > 0.1 {
				break
			}
			run = append(run, l.tracks[j].ID)
		}
		if len(run) > 1 {
			l.runs = append(l.runs, run)
			linked += len(run)
		}
		i += len(run)
	}
}

// skip marks about share of the songs skipped in shuffle: singles, intros
// of runs, and now and then a whole run.
func (l *library) skip(r *rand.Rand, share float64) {
	if share == 0 {
		return
	}
	inRun := map[shuffle.TrackID]bool{}
	for _, run := range l.runs {
		for _, id := range run {
			inRun[id] = true
		}
		switch x := r.Float64(); {
		case x < 0.03:
			for _, id := range run {
				l.tracks[id-1].Skipped = true
			}
		case x < 0.2:
			l.tracks[run[0]-1].Skipped = true
		}
	}
	for i := range l.tracks {
		if !inRun[l.tracks[i].ID] && r.Float64() < share {
			l.tracks[i].Skipped = true
		}
	}
}

// artistPicker draws artist ids 1 to n, the k-th most prolific in
// proportion to 1/k^skew.
func artistPicker(n int, skew float64, r *rand.Rand) func() shuffle.ArtistID {
	cum := make([]float64, n)
	total := 0.0
	for k := range n {
		total += 1 / math.Pow(float64(k+1), skew)
		cum[k] = total
	}
	return func() shuffle.ArtistID {
		x := r.Float64() * total
		lo, hi := 0, n-1
		for lo < hi {
			mid := (lo + hi) / 2
			if cum[mid] < x {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		return shuffle.ArtistID(lo + 1)
	}
}

// ring is a similarity: artists sit at random places on a circle, standing
// in for genre, and two artists are alike the closer they sit, nothing alike
// a quarter of the way round. A random pair scores about 0.25.
type ring struct{ place []float64 }

// newRing places n artists.
func newRing(n int, r *rand.Rand) ring {
	place := make([]float64, n+1)
	for i := range place {
		place[i] = r.Float64()
	}
	return ring{place: place}
}

// Artist is the similarity of a and b.
func (g ring) Artist(a, b shuffle.ArtistID) float64 {
	d := math.Abs(g.place[a] - g.place[b])
	d = min(d, 1-d)
	return max(0, 1-4*d)
}
