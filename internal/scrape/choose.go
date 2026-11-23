package scrape

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/simonhull/kitsune/internal/store"
)

// Where an image or a bio came from that no source's answer may replace: an
// administrator's pin, the library's own files.
const (
	fromFolder = "folder"
	fromPinned = "pinned"
	fromNfo    = "nfo"
)

// Candidate is one image a source offers for one kind.
type Candidate struct {
	Source string // As artist_images names it: "fanart" or "wikimedia".
	ID     string // The source's own ID: fanart.tv's number, a Commons file name.
	URL    string
	Lang   string // A two-letter code, or "" for none.
	Likes  int
	// Width and Height are the image's, or what its kind's rules make it.
	Width, Height int
	// Tier ranks a source's kinds of image for one of Kitsune's: 0 first, as
	// fanart.tv's 4K backgrounds go before its 1080p ones.
	Tier   int
	Credit *store.Credit
}

// Pick returns the one candidate a source's images give a kind: the lowest
// tier, then lang, then English, then no language, then any other; then the
// most likes, the largest, and the oldest ID, so the choice is stable. It
// returns nil for none.
func Pick(cands []Candidate, lang string) *Candidate {
	if len(cands) == 0 {
		return nil
	}
	best := slices.MinFunc(cands, func(a, b Candidate) int {
		return cmp.Or(
			cmp.Compare(a.Tier, b.Tier),
			cmp.Compare(langRank(a.Lang, lang), langRank(b.Lang, lang)),
			cmp.Compare(b.Likes, a.Likes),
			cmp.Compare(b.Width*b.Height, a.Width*a.Height),
			compareIDs(a.ID, b.ID),
		)
	})
	return &best
}

// How languages rank: the one wanted, English, none, the rest.
const (
	rankWanted = iota
	rankEnglish
	rankNone
	rankOther
)

// langRank is l's rank when want is wanted.
func langRank(l, want string) int {
	switch l {
	case want:
		return rankWanted
	case "en":
		return rankEnglish
	case "":
		return rankNone
	}
	return rankOther
}

// compareIDs orders IDs as numbers where both are, else as text.
func compareIDs(a, b string) int {
	x, errA := strconv.ParseUint(a, 10, 64)
	y, errB := strconv.ParseUint(b, 10, 64)
	if errA == nil && errB == nil {
		return cmp.Compare(x, y)
	}
	return cmp.Compare(a, b)
}

// Answer is what one source said this time about one kind of image.
type Answer struct {
	Source string // As artist_images names it.
	// Answered says the source was asked and answered, with images or with
	// none. A source that wasn't asked, or failed, never takes anything away.
	Answered   bool
	Candidates []Candidate
}

// Decision is what to do with one kind of an artist's image: set a candidate,
// take away the image a source gave (Clear names it), or neither, which keeps
// what the artist has.
type Decision struct {
	Set   *Candidate
	Clear string
}

// Decide chooses an artist's image of one kind from the sources' answers, in
// precedence order, given what the artist holds (nil for nothing). A pin and
// a folder's image are never replaced or taken away. The first source that
// answered with an image gives it; one that wasn't asked or failed keeps its
// own image, ahead of every source after it. An image whose source answered
// with nothing goes when no source after it gives one.
func Decide(held *store.HeldImage, answers []Answer, lang string) Decision {
	if held != nil && (held.Source == fromFolder || held.Source == fromPinned) {
		return Decision{}
	}
	for _, a := range answers {
		if d, done := decideOne(&a, held, lang); done {
			return d
		}
	}
	if held != nil && slices.ContainsFunc(answers, func(a Answer) bool { return a.Answered && a.Source == held.Source }) {
		return Decision{Clear: held.Source}
	}
	return Decision{}
}

// decideOne is what a's answer decides, and whether it decides anything: a
// source that wasn't asked keeps its own image, and one that answered with an
// image gives it.
func decideOne(a *Answer, held *store.HeldImage, lang string) (Decision, bool) {
	heldHere := held != nil && held.Source == a.Source
	if !a.Answered {
		return Decision{}, heldHere
	}
	c := Pick(a.Candidates, lang)
	switch {
	case c == nil:
		return Decision{}, false
	case heldHere && held.RemoteID == c.ID:
		return Decision{}, true
	}
	return Decision{Set: c}, true
}

// BioAnswer is what one source said this time about an artist's bio.
type BioAnswer struct {
	Source   string // As artist_bios names it.
	Answered bool
	Bio      *store.Bio // Nil when it has none.
}

// BioDecision is what to do with an artist's bio, as [Decision] is for an image.
type BioDecision struct {
	Set   *store.Bio
	From  string // Set's source.
	Clear string
}

// DecideBio chooses an artist's bio as [Decide] chooses an image: an
// artist.nfo's is never replaced.
func DecideBio(held *store.HeldBio, answers []BioAnswer) BioDecision {
	heldBy := ""
	if held != nil {
		heldBy = held.Source
	}
	if heldBy == fromNfo {
		return BioDecision{}
	}
	for _, a := range answers {
		if a.Answered && a.Bio != nil {
			return BioDecision{Set: a.Bio, From: a.Source}
		}
		if !a.Answered && heldBy == a.Source {
			return BioDecision{}
		}
	}
	if answeredBio(answers, heldBy) {
		return BioDecision{Clear: heldBy}
	}
	return BioDecision{}
}

// answeredBio reports whether source answered, and isn't "".
func answeredBio(answers []BioAnswer, source string) bool {
	return source != "" && slices.ContainsFunc(answers, func(a BioAnswer) bool { return a.Answered && a.Source == source })
}
