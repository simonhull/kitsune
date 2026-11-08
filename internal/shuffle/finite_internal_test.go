package shuffle

import (
	"slices"
	"testing"
)

// grid is a library of artists × songs each, every artist's songs split
// over albums albums. Track ids count from 1, artist by artist.
func grid(artists, songs, albums int) []Track {
	tracks := make([]Track, 0, artists*songs)
	for a := range artists {
		for s := range songs {
			id := TrackID(len(tracks) + 1)
			album := AlbumID(a*albums + s%albums + 1)
			tracks = append(tracks, song(id, ArtistID(a+1), album))
		}
	}
	return tracks
}

// pool builds a pool, failing the test on error.
func pool(t *testing.T, tracks []Track, runs []Run) *Pool {
	t.Helper()
	p, err := NewPool(tracks, runs)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	return p
}

// ids flattens picks to their songs, in play order.
func ids(picks []Pick) []TrackID {
	var out []TrackID
	for i := range picks {
		out = append(out, picks[i].Unit.tracks...)
	}
	return out
}

// config is the default settings at now, without similarity.
func config() Config { return Config{Settings: DefaultSettings(), Now: now} }

func TestFinitePlaysEveryUnitOnce(t *testing.T) {
	t.Parallel()
	tracks := grid(5, 6, 2)
	tracks[3].Skipped = true  // a skipped single: never comes up
	tracks[10].Skipped = true // a skipped intro: plays in its run
	runs := []Run{{11, 12, 13}, {20, 2}}
	p := pool(t, tracks, runs)
	for seed := range uint64(50) {
		got := ids(Finite(p, config(), seed, 0))
		if len(got) != len(tracks)-1 || slices.Contains(got, 4) {
			t.Fatalf("seed %d: got %d songs %v, want all but 4", seed, len(got), got)
		}
		for _, run := range runs {
			at := slices.Index(got, run[0])
			if at < 0 || at+len(run) > len(got) || !slices.Equal(got[at:at+len(run)], run) {
				t.Fatalf("seed %d: run %v broken in %v", seed, run, got)
			}
		}
		sorted := slices.Sorted(slices.Values(got))
		if len(slices.Compact(sorted)) != len(got) {
			t.Fatalf("seed %d: a song repeats in %v", seed, got)
		}
	}
}

func TestFiniteIsRepeatable(t *testing.T) {
	t.Parallel()
	p := pool(t, grid(6, 5, 1), []Run{{1, 2}})
	sim := ring{n: 6}
	c := Config{Settings: Settings{Ratings: 1, Flow: 1, Memory: true}, Similarity: sim, Now: now}
	a, b := ids(Finite(p, c, 42, 0)), ids(Finite(p, c, 42, 0))
	if !slices.Equal(a, b) {
		t.Fatalf("one seed, two orders:\n%v\n%v", a, b)
	}
	if other := ids(Finite(p, c, 43, 0)); slices.Equal(a, other) {
		t.Fatalf("seeds 42 and 43 gave one order: %v", a)
	}
	// Locks the order a seed gives, so a change to it is a decision: update
	// this with the bench report that justifies it.
	want := []TrackID{
		17, 14, 10, 28, 5, 24, 18, 15, 9, 29, 23, 19, 13, 8, 1, 2, 27, 25, 16, 11, 7, 30, 4, 22, 20, 12, 6, 26, 21, 3,
	}
	if !slices.Equal(a, want) {
		t.Fatalf("seed 42 gave\n%#v\nwant\n%#v", a, want)
	}
}

func TestFiniteLead(t *testing.T) {
	t.Parallel()
	tracks := grid(4, 4, 1)
	tracks[5].Skipped = true
	p := pool(t, tracks, []Run{{9, 10, 11}})
	tests := []struct {
		name  string
		first TrackID
		want  []TrackID
	}{
		{"a single leads", 3, []TrackID{3}},
		{"a run leads from its first song", 10, []TrackID{9, 10, 11}},
		{"a skipped song chosen directly leads", 6, []TrackID{6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for seed := range uint64(20) {
				got := ids(Finite(p, config(), seed, tt.first))
				if !slices.Equal(got[:len(tt.want)], tt.want) {
					t.Fatalf("seed %d: starts %v, want %v", seed, got[:len(tt.want)], tt.want)
				}
			}
		})
	}
}

func TestFiniteSpacing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		tracks []Track
	}{
		{"many artists", grid(20, 10, 2)},
		{"few artists", grid(4, 12, 3)},
		{"one big artist", bigArtist(30, 30)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := pool(t, tt.tracks, nil)
			for seed := range uint64(30) {
				picks := Finite(p, config(), seed, 0)
				if b := scaledBreaks(p, picks); b != 0 {
					t.Fatalf("seed %d: %d breaks of the scaled rules in %v", seed, b, ids(picks))
				}
			}
		})
	}
}

// scaledBreaks counts the picks that break p's scaled spacing rules.
func scaledBreaks(p *Pool, picks []Pick) int {
	var w window
	breaks := 0
	for i := range picks {
		pk := &picks[i]
		if !w.fits(p, pk.Unit) {
			breaks++
		}
		w.push(pk.Unit.artists, pk.Unit.albums)
	}
	return breaks
}

func TestFiniteSmallSets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		tracks []Track
	}{
		{"empty", nil},
		{"one song", grid(1, 1, 1)},
		{"an album", grid(1, 12, 1)},
		{"two albums", grid(2, 6, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := pool(t, tt.tracks, nil)
			picks := Finite(p, config(), 7, 0)
			if len(picks) != len(tt.tracks) || scaledBreaks(p, picks) != 0 {
				t.Fatalf("got %v (%d scaled breaks)", ids(picks), scaledBreaks(p, picks))
			}
		})
	}
}

// bigArtist is songs by artist 1 over three albums, then others songs each
// by an artist of its own.
func bigArtist(songs, others int) []Track {
	tracks := grid(1, songs, 3)
	for i := range others {
		tracks = append(tracks, song(TrackID(1000+i), ArtistID(1000+i), AlbumID(1000+i)))
	}
	return tracks
}

func TestFiniteSpreadsABigArtist(t *testing.T) {
	t.Parallel()
	// Artist 1 holds 20 of 41: it should take every other place.
	p := pool(t, bigArtist(20, 21), nil)
	for seed := range uint64(30) {
		picks := Finite(p, config(), seed, 0)
		run := 0
		for _, pk := range picks {
			if pk.Unit.artists[0] != 1 {
				run = 0
				continue
			}
			run++
			if run > 1 {
				t.Fatalf("seed %d: artist 1 twice in a row in %v", seed, ids(picks))
			}
		}
	}
}

func TestFiniteWeightsShapeTheOrder(t *testing.T) {
	t.Parallel()
	tracks := grid(40, 5, 1)
	for i := range tracks {
		tracks[i].Stars = Stars(i%5 + 1)
	}
	p := pool(t, tracks, nil)
	c := Config{Settings: Settings{Ratings: 1}, Now: now}
	var sum [MaxStars + 1]float64
	const seeds = 200
	for seed := range uint64(seeds) {
		for at, pk := range Finite(p, c, seed, 0) {
			sum[int(pk.Reason.Stars)] += float64(at)
		}
	}
	for stars := 2; stars <= int(MaxStars); stars++ {
		if sum[stars] >= sum[stars-1] {
			t.Errorf("%d★ come later on average than %d★: %v", stars, stars-1, sum)
		}
	}
}

// ring is a similarity of n artists on a circle: neighbors are alike,
// opposites are not.
type ring struct{ n int }

func (r ring) Artist(a, b ArtistID) float64 {
	d := int(a-b) % r.n
	d = min((d+r.n)%r.n, (r.n-d)%r.n)
	return 1 - 2*float64(d)/float64(r.n)
}

func TestFiniteFlow(t *testing.T) {
	t.Parallel()
	p := pool(t, grid(30, 10, 2), nil)
	sim := ring{n: 30}
	mean := func(strength float64) float64 {
		c := Config{Settings: Settings{Ratings: DefaultRatings, Flow: strength}, Similarity: sim, Now: now}
		total, pairs := 0.0, 0
		for seed := range uint64(40) {
			picks := Finite(p, c, seed, 0)
			for i := 1; i < len(picks); i++ {
				total += sim.Artist(picks[i-1].Unit.artists[0], picks[i].Unit.artists[0])
				pairs++
			}
		}
		return total / float64(pairs)
	}
	off, on := mean(0), mean(1)
	if on < off+0.05 {
		t.Fatalf("flow 1 gave neighbor similarity %.3f, flow 0 %.3f: want a clear lean", on, off)
	}
}

func TestFiniteFlowIsExplained(t *testing.T) {
	t.Parallel()
	p := pool(t, grid(8, 3, 1), nil)
	c := Config{Settings: Settings{Flow: 1}, Similarity: ring{n: 8}, Now: now}
	picks := Finite(p, c, 1, 0)
	if picks[0].Reason.Flow != 1 {
		t.Errorf("the first pick has flow %v, want 1", picks[0].Reason.Flow)
	}
	for i := 1; i < len(picks); i++ {
		r := picks[i].Reason
		if want := picks[i-1].Unit.artists[0]; r.After != want {
			t.Fatalf("pick %d: flow after %d, want %d", i, r.After, want)
		}
		if want := 1 + (r.Similarity - 0.5); !near(r.Flow, want) {
			t.Fatalf("pick %d: flow %v, want %v", i, r.Flow, want)
		}
	}
}
