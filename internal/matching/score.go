package matching

import (
	"math"
	"slices"
	"strings"
)

// EvidenceKind names one thing a confidence rests on. A client words it.
type EvidenceKind string

// The kinds of evidence.
const (
	// EvidenceLengths: Count of Of paired tracks have lengths within ±3 s.
	EvidenceLengths EvidenceKind = "lengths"
	// EvidenceTitleArtist: the album's title and artist are the release's.
	EvidenceTitleArtist EvidenceKind = "titleArtist"
	// EvidenceTitle: the title differs.
	EvidenceTitle EvidenceKind = "title"
	// EvidenceArtist: the artist differs.
	EvidenceArtist EvidenceKind = "artist"
	// EvidenceTagged: Count tracks' files already carry this release's ID.
	EvidenceTagged EvidenceKind = "tagged"
	// EvidenceDiscs: the release has Count media, the album Of discs; Format is
	// the release's, so a client can say sides.
	EvidenceDiscs EvidenceKind = "discs"
	// EvidenceSongs: the release has Count tracks, the album Of.
	EvidenceSongs EvidenceKind = "songs"
	// EvidenceFingerprint: Count of Of tracks' sound matched a recording of this
	// release. Matching by sound comes later; nothing gives it yet.
	EvidenceFingerprint EvidenceKind = "fingerprint"
)

// Verdict says which way a piece of evidence points.
type Verdict string

// The verdicts.
const (
	Agrees  Verdict = "agrees"
	Neutral Verdict = "neutral"
	Differs Verdict = "differs"
)

// Evidence is one thing a confidence rests on, as facts for a client to word.
type Evidence struct {
	Kind      EvidenceKind
	Verdict   Verdict
	Count, Of int
	Format    string
}

// Weights of the parts of a confidence. Only the parts known for a release count.
const (
	weightTitle   = 20
	weightArtist  = 15
	weightSongs   = 20
	weightLengths = 35
	weightDiscs   = 10
	weightTagged  = 40
)

// Thresholds for wording evidence.
const (
	// nameExact is the likeness of two names that read as the same.
	nameExact = 0.9
	// mostAgree is the share of lengths that agrees in a release worth a tick.
	mostAgree = 0.7
)

// score returns a release's confidence from 0 to 100, and the evidence for it,
// given the album and a pairing with the release's tracklist.
func score(album *Album, r *Release, paired []Pair, tracks []flat) (int, []Evidence) {
	var sum, weights float64
	part := func(weight, value float64) {
		sum += weight * value
		weights += weight
	}
	var evidence []Evidence
	title := similarity(album.Title, r.Title)
	artist := similarity(strings.Join(album.Artists, " "), credited(r.Artists))
	// Cubed: a name nearly the release's counts for much less than the same name.
	part(weightTitle, title*title*title)
	if len(album.Artists) > 0 {
		part(weightArtist, artist*artist*artist)
	}
	if len(tracks) > 0 {
		if t := lengthAgreement(album, paired, tracks); t.of > 0 {
			part(weightLengths, float64(t.agree)/float64(len(album.Tracks)))
			evidence = append(evidence, Evidence{Kind: EvidenceLengths, Verdict: verdict(float64(t.agree) >= mostAgree*float64(t.of)), Count: t.agree, Of: t.of})
		}
	}
	evidence = append(evidence, nameEvidence(title, artist, len(album.Artists) > 0)...)
	if tagged := taggedWith(album, r.ID); tagged > 0 || anyTagged(album) {
		part(weightTagged, float64(tagged)/float64(len(album.Tracks)))
		if tagged > 0 {
			evidence = append(evidence, Evidence{Kind: EvidenceTagged, Verdict: Agrees, Count: tagged})
		}
	}
	songs, have := r.TrackCount(), len(album.Tracks)
	// Squared: four songs too many is most of the way to another edition.
	part(weightSongs, ratio(songs, have)*ratio(songs, have))
	discs := album.discs()
	if len(r.Media) == discs {
		part(weightDiscs, 1)
		if discs > 1 {
			evidence = append(evidence, Evidence{Kind: EvidenceDiscs, Verdict: Neutral, Count: discs, Of: discs})
		}
	} else {
		part(weightDiscs, 0)
		evidence = append(evidence, Evidence{Kind: EvidenceDiscs, Verdict: Differs, Count: len(r.Media), Of: discs, Format: format(r)})
	}
	if songs != have {
		evidence = append(evidence, Evidence{Kind: EvidenceSongs, Verdict: Differs, Count: songs, Of: have})
	}
	return int(math.Round(100 * sum / weights)), evidence
}

// nameEvidence returns the evidence of the title and artist.
func nameEvidence(title, artist float64, haveArtist bool) []Evidence {
	switch {
	case title >= nameExact && artist >= nameExact:
		return []Evidence{{Kind: EvidenceTitleArtist, Verdict: Agrees}}
	case title >= nameExact && !haveArtist:
		return nil
	}
	var out []Evidence
	if title < nameExact {
		out = append(out, Evidence{Kind: EvidenceTitle, Verdict: Differs})
	}
	if haveArtist && artist < nameExact {
		out = append(out, Evidence{Kind: EvidenceArtist, Verdict: Differs})
	}
	return out
}

func verdict(agrees bool) Verdict {
	if agrees {
		return Agrees
	}
	return Neutral
}

// ratio returns the smaller of a and b over the larger: 1 when they're equal.
func ratio(a, b int) float64 {
	if a == b {
		return 1
	}
	return float64(min(a, b)) / float64(max(a, b))
}

// tally is how many of some things agree.
type tally struct{ agree, of int }

// lengthAgreement returns how many pairs have lengths that agree, of the pairs
// whose lengths are both known.
func lengthAgreement(album *Album, paired []Pair, tracks []flat) tally {
	var t tally
	for _, p := range paired {
		if p.Release < 0 {
			continue
		}
		at, rt := &album.Tracks[p.Track], tracks[p.Release].track
		if at.Length > 0 && rt.Length > 0 {
			t.of++
			if lengthsAgree(at.Length, rt.Length) {
				t.agree++
			}
		}
	}
	return t
}

// taggedWith counts the album's tracks whose files carry the release id.
func taggedWith(album *Album, id string) int {
	n := 0
	for i := range album.Tracks {
		if album.Tracks[i].ReleaseID == id {
			n++
		}
	}
	return n
}

func anyTagged(album *Album) bool {
	return slices.ContainsFunc(album.Tracks, func(t Track) bool { return t.ReleaseID != "" })
}

// format returns the format most of a release's media share, or "".
func format(r *Release) string {
	counts := map[string]int{}
	best := ""
	for i := range r.Media {
		f := r.Media[i].Format
		counts[f]++
		if counts[f] > counts[best] {
			best = f
		}
	}
	return best
}
