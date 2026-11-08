package store

import "time"

// Disc is one disc of an album, as its live tracks say: what a heading shows.
type Disc struct {
	// No counts from 1. A track with no disc number is on disc 1.
	No int
	// Name is the disc's subtitle: that of its first track with one, in the order
	// the album lists them. Empty when none has one.
	Name     string
	Songs    int
	Duration time.Duration
}

// discs gathers an album's discs, one track at a time, in the order the tracks
// come. The album queries list tracks by disc, so a disc is nearly always the
// last one seen; the search covers a list out of that order all the same.
type discs []Disc

// add counts a track on disc no, named name, lasting d.
func (ds *discs) add(no int, name string, d time.Duration) {
	no = max(no, 1)
	at := len(*ds) - 1
	for at >= 0 && (*ds)[at].No != no {
		at--
	}
	if at < 0 {
		*ds = append(*ds, Disc{No: no})
		at = len(*ds) - 1
	}
	disc := &(*ds)[at]
	disc.Songs++
	disc.Duration += d
	if disc.Name == "" {
		disc.Name = name
	}
}

// strOrEmpty returns *p, or "" for NULL.
func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
