package store

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Limits on what reaches MATCH. An FTS5 query's cost grows with its tokens: 2,000
// took 30 s, and 20,000 ran the connection out of memory for good.
const (
	maxQueryBytes  = 256
	maxQueryTokens = 16
)

const (
	// maxTrackHits caps the tracks in a result.
	maxTrackHits = 20
	// trackCandidates caps the best-ranked matches that get reordered, so a
	// common word costs the same on any library. An exact title ranks far above it.
	trackCandidates = 500
)

// searchTracksSQL is hand-written because sqlc rejects MATCH. CROSS JOIN keeps
// the FTS table outermost. The weights favor the title over the artists, album
// and album artists, in that order.
const searchTracksSQL = `SELECT t.id, t.title
FROM tracks_fts CROSS JOIN tracks t ON t.id = tracks_fts.rowid
WHERE tracks_fts MATCH ? AND t.missing_since IS NULL
ORDER BY bm25(tracks_fts, 20, 5, 3, 3)
LIMIT ?`

// SearchQuery is search text that is safe to run. [ParseSearch] is the only way
// to fill one, so nothing else reaches MATCH. The zero value finds nothing.
type SearchQuery struct {
	match string // The FTS5 expression; empty when nothing searchable is left.
	forms []form
}

// ParseSearch reduces raw, which may be anything a client sent, to a query: valid
// NFC text with no NUL, at most 256 bytes and 16 tokens, each token quoted. A token
// with no letter or digit is dropped, and a query with no tokens finds nothing.
func ParseSearch(raw string) SearchQuery {
	s := norm.NFC.String(strings.ToValidUTF8(raw, " "))
	s = truncate(strings.ReplaceAll(s, "\x00", ""), maxQueryBytes)
	var words, tokens []string
	for word := range strings.FieldsSeq(s) {
		words = append(words, word)
		if !strings.ContainsFunc(word, isWordRune) {
			continue
		}
		tokens = append(tokens, `"`+strings.ReplaceAll(foldSearch(word), `"`, `""`)+`"*`)
		if len(tokens) == maxQueryTokens {
			break
		}
	}
	if len(tokens) == 0 {
		return SearchQuery{}
	}
	return SearchQuery{match: strings.Join(tokens, " "), forms: forms(strings.Join(words, " "))}
}

// truncate cuts s to at most n bytes, on a rune boundary.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// TrackHit is a live track as a search result shows it.
type TrackHit struct {
	ID       string // The track's ULID.
	Title    string
	Artists  []string
	AlbumID  string // The album's ULID.
	Album    string
	Duration time.Duration
	Cover    *Cover // The album's; nil when it has none.
}

// SearchResults is what a search found, best first, each group capped.
type SearchResults struct {
	Tracks []TrackHit
}

// Search finds what q matches. A canceled ctx ends it with an error that need
// not wrap ctx.Err(): SQLite reports an interrupt.
func (s *Store) Search(ctx context.Context, q SearchQuery) (SearchResults, error) {
	if q.match == "" {
		return SearchResults{}, nil
	}
	tracks, err := s.searchTracks(ctx, q)
	if err != nil {
		return SearchResults{}, fmt.Errorf("search: tracks: %w", err)
	}
	return SearchResults{Tracks: tracks}, nil
}

// trackMatch is a track that matched, ranked by how close its title came.
type trackMatch struct {
	id        int64
	closeness closeness
}

// searchTracks returns the tracks q matches: closest titles first, then by FTS rank.
func (s *Store) searchTracks(ctx context.Context, q SearchQuery) ([]TrackHit, error) {
	matches, err := s.matchTracks(ctx, q)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(matches, func(a, b trackMatch) int { return cmp.Compare(a.closeness, b.closeness) })
	matches = matches[:min(len(matches), maxTrackHits)]
	ids := make([]int64, len(matches))
	for i := range matches {
		ids[i] = matches[i].id
	}
	return s.trackHits(ctx, ids)
}

// matchTracks runs MATCH and returns the candidates in FTS rank order.
func (s *Store) matchTracks(ctx context.Context, q SearchQuery) ([]trackMatch, error) {
	rows, err := s.r.QueryContext(ctx, searchTracksSQL, q.match, trackCandidates)
	if err != nil {
		return nil, fmt.Errorf("match: %w", err)
	}
	defer func() { _ = rows.Close() }() // rows.Err has the outcome.
	var matches []trackMatch
	for rows.Next() {
		var id int64
		var title string
		if err = rows.Scan(&id, &title); err != nil {
			return nil, fmt.Errorf("match: %w", err)
		}
		matches = append(matches, trackMatch{id: id, closeness: closenessOf(forms(title), q.forms)})
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("match: %w", err)
	}
	return matches, nil
}

// trackHits fetches what the tracks with the given IDs show, in that order.
func (s *Store) trackHits(ctx context.Context, ids []int64) ([]TrackHit, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("encode IDs: %w", err)
	}
	rows, err := s.rq.ListTrackHits(ctx, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("hits: %w", err)
	}
	links, err := s.rq.ListTrackHitArtists(ctx, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("hit artists: %w", err)
	}
	artists := make(map[int64][]string, len(ids))
	for i := range links {
		artists[links[i].TrackID] = append(artists[links[i].TrackID], links[i].Name)
	}
	byID := make(map[int64]TrackHit, len(rows))
	for i := range rows {
		r := &rows[i]
		byID[r.ID] = TrackHit{
			ID: r.Ulid, Title: r.Title, Artists: artists[r.ID], AlbumID: r.AlbumUlid, Album: r.AlbumName,
			Duration: time.Duration(r.DurationMs) * time.Millisecond,
			Cover:    cover(r.ArtworkHash, r.ArtworkWidth, r.ArtworkHeight, r.ArtworkAccent, r.ArtworkGround),
		}
	}
	hits := make([]TrackHit, 0, len(ids))
	for _, id := range ids {
		if hit, ok := byID[id]; ok {
			hits = append(hits, hit)
		}
	}
	return hits, nil
}
