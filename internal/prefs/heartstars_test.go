package prefs_test

import (
	"errors"
	"testing"

	"github.com/simonhull/kitsune/internal/prefs"
	"github.com/simonhull/kitsune/internal/rating"
)

func TestHeartStarsCheck(t *testing.T) {
	t.Parallel()
	key, err := prefs.Lookup(prefs.RatingsHeartStars)
	if err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		"0": "0", "1": "1", "2.5": "2.5", "4.5": "4.5", "5": "5", " 4.0 ": "4",
	} {
		if got, err := key.Check([]byte(in)); err != nil || string(got) != want {
			t.Errorf("Check(%s) = %s, %v, want %s", in, got, err, want)
		}
	}
	// Under 1★ would count a heart against a song; past 5★ and between halves
	// aren't ratings.
	for _, in := range []string{"0.5", "5.5", "4.25", "-1", "-0.5", `"4"`, "null", "true", "{}", ""} {
		if got, err := key.Check([]byte(in)); !errors.Is(err, prefs.ErrBadValue) {
			t.Errorf("Check(%s) = %s, %v, want ErrBadValue", in, got, err)
		}
	}
}

func TestHeartStarsOf(t *testing.T) {
	t.Parallel()
	const off, four, fourHalf, five = prefs.HeartStars(0), prefs.HeartStars(8), prefs.HeartStars(9), prefs.HeartStars(10)
	tests := []struct {
		name    string
		h       prefs.HeartStars
		own     rating.Rating
		hearted bool
		want    rating.Rating
		byHeart bool
	}{
		{"off, hearted and unrated", off, rating.None, true, rating.None, false},
		{"off, own wins", off, 7, true, 7, false},
		{"4, hearted and unrated", four, rating.None, true, 8, true},
		{"4½, hearted and unrated", fourHalf, rating.None, true, 9, true},
		{"5, hearted and unrated", five, rating.None, true, 10, true},
		{"4½, not hearted", fourHalf, rating.None, false, rating.None, false},
		{"5, own ½ wins", five, rating.Min, true, rating.Min, false},
		{"5, own 3½ wins", five, 7, true, 7, false},
		{"4, own 5 wins, not hearted", four, rating.Max, false, rating.Max, false},
	}
	for _, tt := range tests {
		if got, byHeart := tt.h.Of(tt.own, tt.hearted); got != tt.want || byHeart != tt.byHeart {
			t.Errorf("%s: Of = %v, %v, want %v, %v", tt.name, got, byHeart, tt.want, tt.byHeart)
		}
	}
}

func TestParseHeartStars(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]prefs.HeartStars{"0": 0, "1": 2, "3.5": 7, "4": 8, "4.5": 9, "5": 10} {
		if got, err := prefs.ParseHeartStars([]byte(in)); err != nil || got != want {
			t.Errorf("ParseHeartStars(%s) = %d, %v, want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"0.5", "null", `"4"`} {
		if got, err := prefs.ParseHeartStars([]byte(in)); !errors.Is(err, prefs.ErrBadValue) || got != 0 {
			t.Errorf("ParseHeartStars(%s) = %d, %v, want ErrBadValue", in, got, err)
		}
	}
}
