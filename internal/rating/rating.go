// Package rating is a person's rating of a song, in half stars.
//
// It is a leaf: it imports nothing of Kitsune's, so the store, the server and
// shuffle's feeder can all speak it without bending the dependency direction.
package rating

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

// Rating is a person's rating of a song, in half stars: 1 (½★) to [Max] (5★).
// The zero value, [None], is no rating.
type Rating uint8

const (
	// None is no rating. It is never stored: clearing a rating deletes it.
	None Rating = 0
	// Min is the lowest rating, ½★.
	Min Rating = 1
	// Max is the highest rating, 5★.
	Max Rating = 10

	// halvesPerStar is how many steps a star has.
	halvesPerStar = 2
)

// ErrBad means a value is not a rating: not ½ to 5 stars in steps of a half.
var ErrBad = errors.New("a rating is ½ to 5 stars, in halves")

// FromStars returns the rating of stars, which must be ½ to 5 in halves, or an
// error wrapping [ErrBad]. Whole stars are exact: 4 is 8 halves.
//
// # Errors
//
// [ErrBad] for 0, anything outside ½ to 5, a step that isn't a half (4.25),
// NaN and the infinities.
func FromStars(stars float64) (Rating, error) {
	halves := stars * halvesPerStar
	if !(halves >= float64(Min) && halves <= float64(Max)) || halves != math.Trunc(halves) {
		return None, fmt.Errorf("%v stars: %w", stars, ErrBad)
	}
	return Rating(halves), nil
}

// FromHalves returns the rating of n half stars, 1 to 10, or an error wrapping
// [ErrBad].
//
// # Errors
//
// [ErrBad] for n outside 1 to 10.
func FromHalves(n int64) (Rating, error) {
	if n < int64(Min) || n > int64(Max) {
		return None, fmt.Errorf("%d halves: %w", n, ErrBad)
	}
	return Rating(n), nil
}

// Valid reports whether r is a rating: [Min] to [Max]. [None] is not.
func (r Rating) Valid() bool { return r >= Min && r <= Max }

// Halves is r in half stars: 9 for 4½★.
func (r Rating) Halves() int64 { return int64(r) }

// Stars is r in stars: 4.5 for 9 halves, and 0 for [None].
func (r Rating) Stars() float64 { return float64(r) / halvesPerStar }

// String is r as a person reads it: "4½★", "½★" or "5★", and "unrated" for
// [None].
func (r Rating) String() string {
	switch {
	case r == None:
		return "unrated"
	case !r.Valid():
		return "Rating(" + strconv.Itoa(int(r)) + ")"
	}
	s := ""
	if whole := r / halvesPerStar; whole > 0 {
		s = strconv.Itoa(int(whole))
	}
	if r%halvesPerStar == 1 {
		s += "½"
	}
	return s + "★"
}
