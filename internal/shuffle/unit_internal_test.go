package shuffle

import (
	"errors"
	"slices"
	"testing"
)

// song is a rated-nothing track by artist on album.
func song(id TrackID, artist ArtistID, album AlbumID) Track {
	return Track{ID: id, Artists: []ArtistID{artist}, Album: album}
}

// skipped is song by artist 1 on album 1, skipped in shuffle.
func skipped(id TrackID) Track {
	t := song(id, 1, 1)
	t.Skipped = true
	return t
}

// unitTracks lists each unit's songs, in pool order.
func unitTracks(p *Pool) [][]TrackID {
	out := make([][]TrackID, 0, p.Len())
	for i := range p.units {
		out = append(out, p.units[i].Tracks())
	}
	return out
}

func TestNewPoolRejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		tracks []Track
		runs   []Run
		want   error
	}{
		{"eleven halves", []Track{{ID: 1, Stars: MaxStars + 1}}, nil, ErrBadTrack},
		{"more skips than starts", []Track{{ID: 1, History: History{ShuffleStarts: 1, ShuffleSkips: 2}}}, nil, ErrBadTrack},
		{"more heard than listened", []Track{{ID: 1, History: History{Listens: 1, Heard: 1.5}}}, nil, ErrBadTrack},
		{"negative count", []Track{{ID: 1, History: History{EarlySkips: -1}}}, nil, ErrBadTrack},
		{"duplicate id", []Track{song(1, 1, 1), song(1, 2, 2)}, nil, ErrDuplicateTrack},
		{"song in two runs", []Track{song(1, 1, 1), song(2, 1, 1), song(3, 1, 1)}, []Run{{1, 2}, {2, 3}}, ErrRunOverlap},
		{"cycle", []Track{song(1, 1, 1), song(2, 1, 1)}, []Run{{1, 2, 1}}, ErrRunOverlap},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewPool(tt.tracks, tt.runs); !errors.Is(err, tt.want) {
				t.Fatalf("NewPool: got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNewPoolUnits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		tracks []Track
		runs   []Run
		want   [][]TrackID
	}{
		{
			name:   "singles",
			tracks: []Track{song(1, 1, 1), song(2, 1, 1)},
			want:   [][]TrackID{{1}, {2}},
		},
		{
			name:   "a run plays from its first song, in order",
			tracks: []Track{song(1, 1, 1), song(2, 1, 1), song(3, 1, 1)},
			runs:   []Run{{3, 1}},
			want:   [][]TrackID{{3, 1}, {2}},
		},
		{
			name:   "a run may cross albums",
			tracks: []Track{song(1, 1, 1), song(2, 2, 2)},
			runs:   []Run{{1, 2}},
			want:   [][]TrackID{{1, 2}},
		},
		{
			name:   "skipped singles are left out",
			tracks: []Track{song(1, 1, 1), skipped(2)},
			want:   [][]TrackID{{1}},
		},
		{
			name:   "a skipped intro still plays in its run",
			tracks: []Track{skipped(1), song(2, 1, 1)},
			runs:   []Run{{1, 2}},
			want:   [][]TrackID{{1, 2}},
		},
		{
			name:   "a run of skipped songs is skipped",
			tracks: []Track{skipped(1), skipped(2), song(3, 1, 1)},
			runs:   []Run{{1, 2}},
			want:   [][]TrackID{{3}},
		},
		{
			name:   "a missing song in a run is left out",
			tracks: []Track{song(1, 1, 1), song(3, 1, 1)},
			runs:   []Run{{1, 2, 3}},
			want:   [][]TrackID{{1, 3}},
		},
		{
			name:   "a run with every song missing is dropped",
			tracks: []Track{song(1, 1, 1)},
			runs:   []Run{{7, 8}},
			want:   [][]TrackID{{1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, err := NewPool(tt.tracks, tt.runs)
			if err != nil {
				t.Fatalf("NewPool: %v", err)
			}
			if got := unitTracks(p); !slices.EqualFunc(got, tt.want, slices.Equal) {
				t.Fatalf("units: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnitKeys(t *testing.T) {
	t.Parallel()
	tracks := []Track{
		{ID: 1, Artists: []ArtistID{5, 6}, Album: 9},
		{ID: 2, Artists: []ArtistID{6, 7}, Album: 10},
	}
	p, err := NewPool(tracks, []Run{{1, 2}})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	u := p.Units()[0]
	if got, want := u.Artists(), []ArtistID{5, 6, 7}; !slices.Equal(got, want) {
		t.Errorf("Artists: got %v, want %v", got, want)
	}
	if got, want := u.Albums(), []AlbumID{9, 10}; !slices.Equal(got, want) {
		t.Errorf("Albums: got %v, want %v", got, want)
	}
	if u.ID() != 1 || u.Len() != 2 {
		t.Errorf("ID, Len: got %d, %d, want 1, 2", u.ID(), u.Len())
	}
}

func TestGaps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		counts []int32
		n      int
		want   []int32
	}{
		{"room for the full gap", []int32{1, 10}, 1000, []int32{4, 4}},
		{"just under half of a set alternates", []int32{6, 7}, 13, []int32{2, 1}},
		{"an album shuffle has no rule", []int32{8}, 8, []int32{1}},
		{"a quarter is three apart, not a rotation", []int32{3, 9}, 12, []int32{3, 1}},
		{"a fifth has room for four", []int32{3, 12}, 16, []int32{4, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := gaps(tt.counts, ArtistGap, tt.n); !slices.Equal(got, tt.want) {
				t.Fatalf("gaps: got %v, want %v", got, tt.want)
			}
		})
	}
}
