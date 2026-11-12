package store

import (
	"cmp"
	"math"
	"strings"
)

// Place is where a track falls among its album's tracks, as the Library lists
// them: by disc, with the tracks that have no disc number after every disc; then
// by track number, none last; then by title; then by path. An album's first track
// is the first by this order, and the album's cover, year and compilation flag
// are that track's.
//
// SQL can't call it, so the queries that list an album's tracks
// (queries/browse.sql) or pick its first (queries/edits.sql) spell the same order
// out, and a test holds them to it. The lists break a last tie by row, which a
// file being read has none of yet: two tracks with the same disc, number and
// title may list in either order.
type Place struct {
	Disc, Track int // 0 is none.
	Title       string
	Path        string // Root, "/" and relative path, as the queries join them.
}

// PlaceOf returns tw's place in its album.
func PlaceOf(tw *TrackWrite) Place {
	return Place{Disc: tw.DiscNo, Track: tw.TrackNo, Title: tw.Title, Path: tw.Root + "/" + tw.RelPath}
}

// Compare returns a negative number when p comes before q in their album, a
// positive one when it comes after, and 0 for the same place.
func (p Place) Compare(q Place) int {
	return cmp.Or(
		cmp.Compare(noneLast(p.Disc), noneLast(q.Disc)),
		cmp.Compare(noneLast(p.Track), noneLast(q.Track)),
		strings.Compare(p.Title, q.Title),
		strings.Compare(p.Path, q.Path),
	)
}

// noneLast puts 0, no number, after every number.
func noneLast(n int) int {
	if n == 0 {
		return math.MaxInt
	}
	return n
}
