package matching

import (
	"strings"
	"time"
)

// Release is a release as a source describes it. Media hold their tracks when
// the release was looked up, and only their counts when it was found by a search.
type Release struct {
	Source   string // Who described it, such as [SourceMusicBrainz].
	ID       string // The source's ID: for MusicBrainz, the release MBID.
	GroupID  string // The release group's, or "".
	Title    string
	Artists  []Credit // Who it is credited to, in order.
	Date     string   // YYYY, YYYY-MM or YYYY-MM-DD, or "".
	Original string   // The release group's first release date, in the same form, or "".
	Country  string   // ISO 3166-1, or a source's own code such as XW, or "".
	Barcode  string
	Status   string // Official, Promotion, Bootleg...
	Labels   []Label
	Media    []Medium
	Genres   []string // Most voted first.
	Cover    bool     // The source has a front cover for it.
}

// Credit is one artist of a credit: the name as credited, the artist's ID, and
// what joins it to the next.
type Credit struct {
	Name, ID, Join string
}

// Label is one label a release came out on, with its catalog number there.
type Label struct {
	Name, CatalogNumber string
}

// Medium is one disc, side set or file set of a release.
type Medium struct {
	Position   int    // From 1.
	Format     string // CD, 12" Vinyl, Digital Media...
	Title      string // Its name, or "".
	TrackCount int
	Tracks     []ReleaseTrack // Nil when the release came from a search.
}

// ReleaseTrack is one track of a medium.
type ReleaseTrack struct {
	ID          string // The release track's ID.
	RecordingID string
	Position    int // On its medium, from 1.
	Title       string
	Artists     []Credit
	Length      time.Duration // 0 is unknown.
	ISRCs       []string
}

// TrackCount returns the tracks of every medium.
func (r *Release) TrackCount() int {
	n := 0
	for i := range r.Media {
		n += r.Media[i].TrackCount
	}
	return n
}

// flat is one release track with the medium it is on.
type flat struct {
	medium *Medium
	track  *ReleaseTrack
}

// tracks returns every track of r, medium by medium.
func (r *Release) tracks() []flat {
	var out []flat
	for i := range r.Media {
		m := &r.Media[i]
		for j := range m.Tracks {
			out = append(out, flat{medium: m, track: &m.Tracks[j]})
		}
	}
	return out
}

// names returns the names of a credit, in order.
func names(credits []Credit) []string {
	out := make([]string, len(credits))
	for i, c := range credits {
		out[i] = c.Name
	}
	return out
}

// ids returns the artist IDs of a credit, in order, or nil when any is missing.
func ids(credits []Credit) []string {
	out := make([]string, len(credits))
	for i, c := range credits {
		if c.ID == "" {
			return nil
		}
		out[i] = c.ID
	}
	return out
}

// credited returns a credit as it reads: each name and its join.
func credited(credits []Credit) string {
	var b strings.Builder
	for _, c := range credits {
		b.WriteString(c.Name)
		b.WriteString(c.Join)
	}
	return b.String()
}
