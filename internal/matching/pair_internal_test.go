package matching

import (
	"slices"
	"testing"
	"time"
)

func TestFold(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"Björk & Friends!", "bjork and friends"},
		{"  Low   Tide\tChoir ", "low tide choir"},
		{"Harbor Lights (Intro)", "harbor lights intro"},
		{"ＡＢＣ", "abc"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := fold(tt.in); got != tt.want {
			t.Errorf("fold(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSimilarity(t *testing.T) {
	t.Parallel()
	const glasshouse = "Glasshouse"
	tests := []struct {
		a, b     string
		min, max float64
	}{
		{glasshouse, "glasshouse", 1, 1},
		{glasshouse, "Glass House", 0.9, 0.95},
		{glasshouse, "Glasshouse (Deluxe Edition)", containment, containment},
		{"Low Tide Choir", "Low Tide Chorus", 0.7, 0.85},
		{glasshouse, "Tidewrack", 0, 0.3},
		{"", glasshouse, 0, 0},
	}
	for _, tt := range tests {
		if got := similarity(tt.a, tt.b); got < tt.min || got > tt.max {
			t.Errorf("similarity(%q, %q) = %.2f, want %.2f to %.2f", tt.a, tt.b, got, tt.min, tt.max)
		}
	}
}

// listed returns a release of one medium with tracks of the titles and seconds.
func listed(tracks ...ReleaseTrack) []flat {
	m := &Medium{Position: 1, Tracks: tracks, TrackCount: len(tracks)}
	out := make([]flat, len(tracks))
	for i := range tracks {
		out[i] = flat{medium: m, track: &m.Tracks[i]}
	}
	return out
}

func song(title string, seconds int) ReleaseTrack {
	return ReleaseTrack{Title: title, Length: time.Duration(seconds) * time.Second}
}

func albumOf(tracks ...ReleaseTrack) *Album {
	a := &Album{}
	for _, t := range tracks {
		a.Tracks = append(a.Tracks, Track{Title: t.Title, Length: t.Length})
	}
	return a
}

func releasesOf(p []Pair) []int {
	out := make([]int, len(p))
	for i := range p {
		out[i] = p[i].Release
	}
	return out
}

func TestPairs(t *testing.T) {
	t.Parallel()
	a, b, c, d := song("Alpha", 200), song("Bravo", 180), song("Charlie", 240), song("Delta", 300)
	tests := []struct {
		name    string
		album   *Album
		release []flat
		want    []int
	}{
		{"same", albumOf(a, b, c), listed(a, b, c), []int{0, 1, 2}},
		{"a swap pairs in order", albumOf(a, c, b), listed(a, b, c), []int{0, 1, 2}},
		{"bonus tracks between", albumOf(a, b, d), listed(a, song("Bonus", 100), b, c, d), []int{0, 2, 4}},
		{"bonus tracks first", albumOf(c, d), listed(a, b, c, d), []int{2, 3}},
		{"songs the release lacks", albumOf(a, b, c, d), listed(a, d), []int{0, -1, -1, 1}},
		{"retitled between anchors", albumOf(a, song("Untitled", 180), song("Track 3", 240), d), listed(a, b, c, song("Bonus", 60), d), []int{0, 1, 2, 4}},
		{"nothing", albumOf(a), listed(), []int{-1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := releasesOf(pairs(tt.album, tt.release)); !slices.Equal(got, tt.want) {
				t.Errorf("pairs = %v, want %v", got, tt.want)
			}
		})
	}
}
