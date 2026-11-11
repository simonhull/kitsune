package rating_test

import (
	"errors"
	"math"
	"testing"

	"github.com/simonhull/kitsune/internal/rating"
)

func TestFromStarsTakesHalves(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		stars float64
		want  rating.Rating
	}{
		{0.5, 1}, {1, 2}, {1.5, 3}, {3.5, 7}, {4, 8}, {4.5, 9}, {5, 10},
	} {
		got, err := rating.FromStars(tt.stars)
		if err != nil || got != tt.want {
			t.Errorf("FromStars(%v) = %d, %v, want %d", tt.stars, got, err, tt.want)
		}
		if back := got.Stars(); back != tt.stars {
			t.Errorf("FromStars(%v).Stars() = %v", tt.stars, back)
		}
	}
}

func TestFromStarsRefuses(t *testing.T) {
	t.Parallel()
	for _, stars := range []float64{0, -0.5, 5.5, 4.25, 0.1, 6, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got, err := rating.FromStars(stars); !errors.Is(err, rating.ErrBad) || got != rating.None {
			t.Errorf("FromStars(%v) = %d, %v, want ErrBad", stars, got, err)
		}
	}
}

func TestFromHalves(t *testing.T) {
	t.Parallel()
	for n := int64(1); n <= 10; n++ {
		r, err := rating.FromHalves(n)
		if err != nil || r.Halves() != n || !r.Valid() {
			t.Errorf("FromHalves(%d) = %d, %v", n, r, err)
		}
	}
	for _, n := range []int64{-1, 0, 11, 256} {
		if _, err := rating.FromHalves(n); !errors.Is(err, rating.ErrBad) {
			t.Errorf("FromHalves(%d) = %v, want ErrBad", n, err)
		}
	}
}

func TestString(t *testing.T) {
	t.Parallel()
	for r, want := range map[rating.Rating]string{
		rating.None: "unrated", 1: "½★", 2: "1★", 7: "3½★", 9: "4½★", rating.Max: "5★", 11: "Rating(11)",
	} {
		if got := r.String(); got != want {
			t.Errorf("Rating(%d).String() = %q, want %q", r, got, want)
		}
	}
}

func TestStarsOfNone(t *testing.T) {
	t.Parallel()
	if rating.None.Stars() != 0 || rating.None.Valid() {
		t.Error("None is a rating")
	}
}
