package prefs_test

import (
	"errors"
	"testing"

	"github.com/simonhull/kitsune/internal/prefs"
	"github.com/simonhull/kitsune/internal/rating"
)

// Values the heart stars refuse, named because other keys' tests use them too.
const (
	halfStar = "0.5"
	jsonNull = "null"
)

// heartStarsKey is the key of the heart stars.
func heartStarsKey(t *testing.T) prefs.Key {
	t.Helper()
	key, err := prefs.Lookup(prefs.RatingsHeartStars)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestHeartStarsCheck(t *testing.T) {
	t.Parallel()
	key := heartStarsKey(t)
	tests := []struct {
		in, kept string
		parsed   prefs.HeartStars // What ParseHeartStars gives.
	}{
		{"0", "0", 0}, {"1", "1", 2}, {"2.5", "2.5", 5}, {"4.5", "4.5", 9}, {"5", "5", 10}, {"4.0", "4", 8},
	}
	for _, tt := range tests {
		kept, err := key.Check([]byte(tt.in))
		parsed, parseErr := prefs.ParseHeartStars([]byte(tt.in))
		if err != nil || string(kept) != tt.kept || parseErr != nil || parsed != tt.parsed {
			t.Errorf("%q: Check = %s, %v; Parse = %d, %v; want %s and %d", tt.in, kept, err, parsed, parseErr, tt.kept, tt.parsed)
		}
	}
}

func TestHeartStarsRefuse(t *testing.T) {
	t.Parallel()
	key := heartStarsKey(t)
	// Under 1★ would count a heart against a song; past 5★ and between halves
	// aren't ratings.
	for _, in := range []string{halfStar, "5.5", "4.25", "-1", "-0.5", `"4"`, jsonNull, "true", "{}", ""} {
		kept, err := key.Check([]byte(in))
		parsed, parseErr := prefs.ParseHeartStars([]byte(in))
		if kept != nil || !errors.Is(err, prefs.ErrBadValue) || !errors.Is(parseErr, prefs.ErrBadValue) || parsed != 0 {
			t.Errorf("%q: Check = %s, %v; Parse = %d, %v; want ErrBadValue", in, kept, err, parsed, parseErr)
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
