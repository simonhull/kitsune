// Package matching finds the release an album is, at a source such as
// MusicBrainz, and says what its tags would become: candidates ranked by a
// confidence built from evidence, a pairing of the album's tracks with the
// release's, and the values each file would take. It proposes and never
// writes: a person accepts what they agree with, and that goes through the
// tag editor's own path. It knows nothing of the store.
package matching

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Source is somewhere releases are described: MusicBrainz now, Discogs later.
type Source interface {
	// Name names the source, as [Release.Source] does.
	Name() string
	// Search returns releases that may be what q describes, best first,
	// without their tracklists.
	Search(ctx context.Context, q *Query) ([]Release, error)
	// Release returns the release id with its tracklist, or an error wrapping
	// [ErrNotFound].
	Release(ctx context.Context, id string) (Release, error)
	// CoverURL returns where the release's front cover is, for a browser.
	CoverURL(id string) string
	// PageURL returns the release's page at the source.
	PageURL(id string) string
}

// Candidate is a release an album may be, with how sure the matcher is.
type Candidate struct {
	Release  Release // Its media without their tracks.
	Score    int     // Confidence, from 0 to 100.
	Evidence []Evidence
	Cover    string // The front cover's URL, or "" when the source has none.
	Page     string // The release's page at its source.
}

// Found is what a search found for an album.
type Found struct {
	Query      Query       // What was asked.
	Candidates []Candidate // Best first.
}

// Review is one candidate, looked at closely: what it would write, and which
// release track each album track takes its tags from.
type Review struct {
	Candidate Candidate
	Proposal  Proposal
	Pairs     []Pair // One for each album track, in album order.
}

// Matcher finds candidates for albums. It is safe for concurrent use.
type Matcher struct {
	source Source
}

// New returns a matcher that asks source.
func New(source Source) *Matcher {
	return &Matcher{source: source}
}

// Source returns the source the matcher asks.
func (m *Matcher) Source() string { return m.source.Name() }

// How many releases a search looks up whole, for their tracklists, and how
// many releases the files name it looks up besides.
const (
	lookups     = 4
	taggedLooks = 2
)

// AlbumQuery returns the query an album's own tags make.
func AlbumQuery(album *Album) Query {
	return Query{Album: album.Title, Artist: strings.Join(album.Artists, " ")}
}

// Find returns the releases the album may be, best first. q says what to ask
// for; an empty one asks for the album's own title and artist. The releases
// its files already name are always among them.
func (m *Matcher) Find(ctx context.Context, album *Album, q *Query) (Found, error) {
	asked := *q
	if asked.Empty() {
		asked = AlbumQuery(album)
	}
	tagged := taggedReleases(album)
	found, err := m.lookUp(ctx, nil, tagged, len(tagged))
	if err != nil {
		return Found{}, err
	}
	searched, err := m.search(ctx, &asked)
	if err != nil {
		return Found{}, err
	}
	rankLoosely(album, searched)
	ids := make([]string, len(searched))
	for i := range searched {
		ids[i] = searched[i].ID
	}
	if found, err = m.lookUp(ctx, found, ids, lookups); err != nil {
		return Found{}, err
	}
	out := Found{Query: asked, Candidates: make([]Candidate, len(found))}
	for i := range found {
		out.Candidates[i] = m.candidate(album, &found[i]).Candidate
	}
	slices.SortStableFunc(out.Candidates, func(a, b Candidate) int { return cmp.Compare(b.Score, a.Score) })
	return out, nil
}

// Review returns the release id at the source looked at against the album.
func (m *Matcher) Review(ctx context.Context, album *Album, id string) (Review, error) {
	r, err := m.source.Release(ctx, id)
	if err != nil {
		return Review{}, fmt.Errorf("review: %w", err)
	}
	return m.candidate(album, &r), nil
}

// lookUp looks up the releases ids, in order, until it holds most more than
// found does, skipping those found already and those the source doesn't have.
func (m *Matcher) lookUp(ctx context.Context, found []Release, ids []string, most int) ([]Release, error) {
	want := len(found) + most
	for _, id := range ids {
		if len(found) == want {
			break
		}
		if slices.ContainsFunc(found, func(r Release) bool { return r.ID == id }) {
			continue
		}
		r, err := m.source.Release(ctx, id)
		if errors.Is(err, ErrNotFound) {
			continue // Named by a file but not at the source, or merged away.
		}
		if err != nil {
			return nil, fmt.Errorf("find: %w", err)
		}
		found = append(found, r)
	}
	return found, nil
}

// search asks the source for q's phrases, then, with nothing found, for its
// words: a title typed a little differently still finds its release.
func (m *Matcher) search(ctx context.Context, q *Query) ([]Release, error) {
	found, err := m.source.Search(ctx, q)
	if loose := q.loosened(); err == nil && len(found) == 0 && loose.lucene() != q.lucene() {
		found, err = m.source.Search(ctx, &loose)
	}
	if err != nil {
		return nil, fmt.Errorf("find: %w", err)
	}
	return found, nil
}

// candidate scores the looked-up release r for the album, keeping what a
// review shows.
func (m *Matcher) candidate(album *Album, r *Release) Review {
	tracks := r.tracks()
	paired := pairs(album, tracks)
	points, evidence := score(album, r, paired, tracks)
	c := Candidate{Release: *r, Score: points, Evidence: evidence, Page: m.source.PageURL(r.ID)}
	c.Release.Media = slices.Clone(r.Media)
	for i := range c.Release.Media {
		c.Release.Media[i].Tracks = nil
	}
	if r.Cover {
		c.Cover = m.source.CoverURL(r.ID)
	}
	return Review{Candidate: c, Proposal: propose(r), Pairs: paired}
}

// taggedReleases returns the releases the album's files name, most named first.
func taggedReleases(album *Album) []string {
	counts := map[string]int{}
	var order []string
	for i := range album.Tracks {
		if id := album.Tracks[i].ReleaseID; id != "" {
			if counts[id] == 0 {
				order = append(order, id)
			}
			counts[id]++
		}
	}
	slices.SortStableFunc(order, func(a, b string) int { return cmp.Compare(counts[b], counts[a]) })
	return order[:min(len(order), taggedLooks)]
}

// Weights of a search result's rank before its tracklist is known.
const (
	looseTitle  = 0.4
	looseArtist = 0.3
	looseSongs  = 0.3
)

// rankLoosely orders search results by what they say without a tracklist:
// title, artist and song count.
func rankLoosely(album *Album, found []Release) {
	artist := strings.Join(album.Artists, " ")
	rank := func(r *Release) float64 {
		return looseTitle*similarity(album.Title, r.Title) + looseArtist*similarity(artist, credited(r.Artists)) +
			looseSongs*ratio(r.TrackCount(), len(album.Tracks))
	}
	slices.SortStableFunc(found, func(a, b Release) int { return cmp.Compare(rank(&b), rank(&a)) })
}
