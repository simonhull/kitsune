package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/store"
)

// matchRetryAfter is the Retry-After of a match MusicBrainz couldn't answer.
const matchRetryAfter = "10"

// postAlbumMatches answers with the releases an album may be.
func (s *Server) postAlbumMatches(w http.ResponseWriter, r *http.Request) {
	if s.matcher == nil {
		s.matchingUnavailable(w, r)
		return
	}
	var req api.MatchRequest
	if err := readJSON(w, r, &req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "request body must be a JSON match request")
		return
	}
	id := r.PathValue("id")
	album, ok := s.matchAlbum(w, r, id)
	if !ok {
		return
	}
	ctx := r.Context()
	if req.Prefetch {
		ctx = matching.Background(ctx)
	}
	q := matching.TermsQuery(req.Artist, req.Album, req.Code)
	found, err := s.matcher.Find(ctx, &album, &q)
	if err != nil {
		s.matchFailed(w, r, "find matches", err)
		return
	}
	body := api.MatchList{
		Source:     s.matcher.Source(),
		Asked:      api.MatchTerms{Artist: found.Query.Artist, Album: found.Query.Album, CatalogNumber: found.Query.Catalog, Barcode: found.Query.Barcode},
		Candidates: make([]api.MatchCandidate, len(found.Candidates)),
	}
	for i := range found.Candidates {
		body.Candidates[i] = toAPICandidate(id, &found.Candidates[i])
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// getMatch answers with one candidate, looked at against its album.
func (s *Server) getMatch(w http.ResponseWriter, r *http.Request) {
	if s.matcher == nil {
		s.matchingUnavailable(w, r)
		return
	}
	c, ok := parseCandidate(r.PathValue("id"), s.matcher.Source())
	if !ok {
		writeProblem(w, r, http.StatusNotFound, "no such candidate")
		return
	}
	album, ok := s.matchAlbum(w, r, c.album)
	if !ok {
		return
	}
	review, err := s.matcher.Review(r.Context(), &album, c.release)
	if errors.Is(err, matching.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "no such release")
		return
	}
	if err != nil {
		s.matchFailed(w, r, "review a match", err)
		return
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, toAPIReview(c.album, &album, &review))
}

// matchAlbum reads the album id as matching takes it: its names and lengths
// from the library, and the release its files name from the files. It writes
// the problem and reports false when it can't.
func (s *Server) matchAlbum(w http.ResponseWriter, r *http.Request, id string) (matching.Album, bool) {
	d, err := s.library.Album(r.Context(), id)
	if err == nil && len(d.Tracks) == 0 {
		err = store.ErrNotFound
	}
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "no such album")
		return matching.Album{}, false
	}
	if err != nil {
		s.internalError(w, r, "read an album to match", err)
		return matching.Album{}, false
	}
	tagged, err := s.taggedReleases(r.Context(), id)
	if err != nil {
		s.internalError(w, r, "read an album's tags to match", err)
		return matching.Album{}, false
	}
	out := matching.Album{Title: d.Title, Tracks: make([]matching.Track, len(d.Tracks))}
	if d.Artist != "" {
		out.Artists = []string{d.Artist}
	}
	for i := range d.Tracks {
		t := &d.Tracks[i]
		out.Tracks[i] = matching.Track{
			ID: t.ID, Disc: t.DiscNo, Number: t.TrackNo, Title: t.Title, Length: t.Duration, ReleaseID: tagged[t.ID],
		}
	}
	return out, true
}

// taggedReleases returns the release MBID each track's file carries, by track.
// A file gone leaves the album to match on what the library has.
func (s *Server) taggedReleases(ctx context.Context, id string) (map[string]string, error) {
	tagged := map[string]string{}
	tags, _, err := s.scanner.AlbumTags(ctx, id)
	if errors.Is(err, library.ErrStale) {
		return tagged, nil
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // The caller says what it was for.
	}
	for i := range tags {
		if v := tags[i].Fields[library.FieldMusicBrainzAlbumID]; len(v) > 0 {
			tagged[tags[i].ID] = v[0]
		}
	}
	return tagged, nil
}

// matchFailed answers a match the source couldn't give. A canceled request is
// a client that left, and not logged.
func (s *Server) matchFailed(w http.ResponseWriter, r *http.Request, what string, err error) {
	switch {
	case r.Context().Err() != nil:
		return
	case errors.Is(err, matching.ErrUnavailable):
		s.log.WarnContext(r.Context(), what, "requestId", requestID(r.Context()), "err", err)
		w.Header().Set("Retry-After", matchRetryAfter)
		writeProblem(w, r, http.StatusServiceUnavailable, "MusicBrainz can't be reached just now; try again shortly")
	default:
		s.internalError(w, r, what, err)
	}
}

// matchingUnavailable answers a match on a server without a matcher.
func (*Server) matchingUnavailable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Retry-After", matchRetryAfter)
	writeProblem(w, r, http.StatusServiceUnavailable, "matching isn't set up on this server")
}

// candidateSeparator joins the parts of a candidate's ID: the album, the
// source and the release. None of them holds one.
const candidateSeparator = "."

// candidateID returns the ID of a release as a candidate for the album.
func candidateID(album, source, release string) string {
	return strings.Join([]string{album, source, release}, candidateSeparator)
}

// candidate is what a candidate's ID names.
type candidate struct{ album, release string }

// parseCandidate returns what a candidate's ID names, reporting false for one
// that isn't of source.
func parseCandidate(id, source string) (candidate, bool) {
	album, rest, _ := strings.Cut(id, candidateSeparator)
	from, release, _ := strings.Cut(rest, candidateSeparator)
	if album == "" || from != source || release == "" || strings.Contains(release, candidateSeparator) {
		return candidate{}, false
	}
	return candidate{album: album, release: release}, true
}

func toAPICandidate(album string, c *matching.Candidate) api.MatchCandidate {
	r := &c.Release
	out := api.MatchCandidate{
		ID: candidateID(album, r.Source, r.ID), Score: c.Score, Cover: c.Cover, Page: c.Page,
		Evidence: make([]api.MatchEvidence, len(c.Evidence)),
		Release: api.MatchRelease{
			ID: r.ID, GroupID: r.GroupID, Title: r.Title, Artist: credit(r.Artists), Artists: toAPICredits(r.Artists),
			Date: r.Date, OriginalDate: r.Original, Country: r.Country, Barcode: r.Barcode, Status: r.Status,
			Labels: make([]api.MatchLabel, len(r.Labels)), Media: make([]api.MatchMedium, len(r.Media)), TrackCount: r.TrackCount(),
		},
	}
	for i, e := range c.Evidence {
		out.Evidence[i] = api.MatchEvidence{Kind: string(e.Kind), Verdict: string(e.Verdict), Count: e.Count, Of: e.Of, Format: e.Format}
	}
	for i, l := range r.Labels {
		out.Release.Labels[i] = api.MatchLabel{Name: l.Name, CatalogNumber: l.CatalogNumber}
	}
	for i := range r.Media {
		m := &r.Media[i]
		out.Release.Media[i] = api.MatchMedium{Position: m.Position, Format: m.Format, Title: m.Title, TrackCount: m.TrackCount}
	}
	return out
}

func toAPIReview(albumID string, album *matching.Album, review *matching.Review) *api.MatchReview {
	p := &review.Proposal
	out := &api.MatchReview{
		Candidate: toAPICandidate(albumID, &review.Candidate),
		Album:     fieldValues(p.Album), AlbumIDs: fieldValues(p.AlbumIDs),
		Tracks: make([]api.MatchTrack, len(p.Tracks)), Pairs: make([]api.MatchPair, len(review.Pairs)),
	}
	for i := range p.Tracks {
		t := &p.Tracks[i]
		out.Tracks[i] = api.MatchTrack{
			Disc: t.Medium.Position, Position: t.Track.Position, Title: t.Track.Title, Artist: credit(t.Track.Artists),
			LengthMS: t.Track.Length.Milliseconds(), Proposed: fieldValues(t.Proposed), IDs: fieldValues(t.IDs),
		}
	}
	for i, pair := range review.Pairs {
		out.Pairs[i] = api.MatchPair{Track: album.Tracks[pair.Track].ID}
		if pair.Release >= 0 {
			out.Pairs[i].Release = &pair.Release
		}
	}
	return out
}

func toAPICredits(credits []matching.Credit) []api.MatchCredit {
	out := make([]api.MatchCredit, len(credits))
	for i, c := range credits {
		out[i] = api.MatchCredit{Name: c.Name, ID: c.ID, Join: c.Join}
	}
	return out
}

// credit returns a credit as it reads.
func credit(credits []matching.Credit) string {
	var b strings.Builder
	for _, c := range credits {
		b.WriteString(c.Name)
		b.WriteString(c.Join)
	}
	return b.String()
}

func fieldValues(values matching.Values) map[string][]string {
	out := make(map[string][]string, len(values))
	for field, v := range values {
		out[string(field)] = v
	}
	return out
}

// getMatchingQueue answers with the albums to match.
func (s *Server) getMatchingQueue(w http.ResponseWriter, r *http.Request) {
	albums, err := s.library.MatchQueue(r.Context())
	if err != nil {
		s.internalError(w, r, "read the matching queue", err)
		return
	}
	body := api.MatchQueue{Albums: make([]api.QueuedAlbum, len(albums))}
	for i := range albums {
		a := &albums[i]
		body.Albums[i] = api.QueuedAlbum{ID: a.ID, Title: a.Name, Artist: a.Artist, Songs: a.Tracks, State: string(a.State)}
		switch a.State {
		case store.MatchToDo:
			body.Counts.ToDo++
		case store.MatchSkipped:
			body.Counts.Skipped++
		case store.MatchMatched:
			body.Counts.Matched++
		}
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// putMatchingAlbum says where an album stands in the queue.
func (s *Server) putMatchingAlbum(w http.ResponseWriter, r *http.Request) {
	var req api.MatchStateChange
	err := readJSON(w, r, &req)
	state := store.MatchState(req.State)
	if err != nil || (state != store.MatchToDo && state != store.MatchSkipped && state != store.MatchMatched) {
		writeProblem(w, r, http.StatusBadRequest, "request body must name a state: todo, skipped or matched")
		return
	}
	err = s.library.SetMatchState(r.Context(), r.PathValue("id"), state)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "no such album")
	case err != nil:
		s.internalError(w, r, "set an album's match state", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// LookupCache keeps a source's answers in the store, as a [matching.Cache].
type LookupCache struct {
	st     *store.Store
	source string
}

// NewLookupCache returns a cache of source's answers in st.
func NewLookupCache(st *store.Store, source string) *LookupCache {
	return &LookupCache{st: st, source: source}
}

// Get implements [matching.Cache].
func (c *LookupCache) Get(ctx context.Context, key string, notBefore time.Time) ([]byte, error) {
	body, err := c.st.Lookup(ctx, c.source, key, notBefore)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return body, err //nolint:wrapcheck // The store's error already says what failed.
}

// Put implements [matching.Cache].
func (c *LookupCache) Put(ctx context.Context, key string, body []byte, at time.Time) error {
	return c.st.PutLookup(ctx, c.source, key, body, at, at.Add(-matching.CacheFor)) //nolint:wrapcheck // As above.
}
