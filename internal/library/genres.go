package library

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/simonhull/kitsune/internal/store"
)

// genreSeparators holds every character that divides a tag value into several
// genres, so "Pop, Rock" is two. Emptying it makes that one genre, and that is
// all it takes: nothing else knows about the split, and nothing is stored.
const genreSeparators = ","

// splitGenres returns the genres a tag value names, as they are spelled there.
func splitGenres(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return strings.ContainsRune(genreSeparators, r) })
	genres := make([]string, 0, len(parts))
	for _, part := range parts {
		if genre := strings.TrimSpace(trimGenreNumber(strings.TrimSpace(part))); genre != "" {
			genres = append(genres, genre)
		}
	}
	return genres
}

// trimGenreNumber removes the ID3v1 number some MP3s put before a genre, as in
// "(12)Spiritual".
func trimGenreNumber(genre string) string {
	rest, found := strings.CutPrefix(genre, "(")
	if !found {
		return genre
	}
	number, after, found := strings.Cut(rest, ")")
	if !found || number == "" || strings.Trim(number, "0123456789") != "" {
		return genre
	}
	return after
}

// genreName is one genre of a tag value.
type genreName struct{ key, spelling string }

// genreNames splits a tag value and keys each genre by [MatchKey].
func genreNames(value string) []genreName {
	genres := splitGenres(value)
	names := make([]genreName, len(genres))
	for i, genre := range genres {
		names[i] = genreName{key: MatchKey(genre), spelling: genre}
	}
	return names
}

// mostTracks returns the keys of tracks, most tracks first and then by key.
func mostTracks(tracks map[string]int) []string {
	return slices.SortedFunc(maps.Keys(tracks), func(a, b string) int {
		return cmp.Or(cmp.Compare(tracks[b], tracks[a]), cmp.Compare(a, b))
	})
}

// genreState is what a refresh works out: each album's genres, and the one
// spelling of each genre, by its key.
type genreState struct {
	albums    map[string][]string
	spellings map[string]string
}

// displayGenres works out each album's genres from counts of raw tag values, and
// each genre's spelling. One genre has one spelling across the library, its most
// common. Spellings are only grouped, never reinterpreted: synonyms stay apart,
// and the tag editor fixes them.
func displayGenres(counts []store.AlbumTagCount) genreState {
	names := make(map[string][]genreName)
	spellings := make(map[string]map[string]int) // By genre key.
	albums := make(map[string]map[string]int)    // By album ID, then genre key.
	for i := range counts {
		c := &counts[i]
		parsed, seen := names[c.Value]
		if !seen { // A library repeats a few values thousands of times.
			parsed = genreNames(c.Value)
			names[c.Value] = parsed
		}
		for _, name := range parsed {
			add(spellings, name.key, name.spelling, c.Tracks)
			add(albums, c.AlbumID, name.key, c.Tracks)
		}
	}
	out := genreState{albums: make(map[string][]string, len(albums)), spellings: make(map[string]string, len(spellings))}
	for key, spelled := range spellings {
		out.spellings[key] = mostTracks(spelled)[0]
	}
	for id, genres := range albums {
		for _, key := range mostTracks(genres) {
			out.albums[id] = append(out.albums[id], out.spellings[key])
		}
	}
	return out
}

// add counts n more under outer and inner.
func add(counts map[string]map[string]int, outer, inner string, n int) {
	if counts[outer] == nil {
		counts[outer] = make(map[string]int)
	}
	counts[outer][inner] += n
}

// Genres knows every album's genres. They come from the raw tags and live only in
// memory, so a change to the rule takes a restart and nothing more. It is safe for
// concurrent use, and readers never wait for a refresh.
type Genres struct {
	st         *store.Store
	state      atomic.Pointer[genreState] // Nil until the first refresh.
	refreshing sync.Mutex                 // Held by a refresh, so an older one can't land last.
}

// NewGenres returns a Genres over st that knows nothing until [Genres.Refresh].
func NewGenres(st *store.Store) *Genres { return &Genres{st: st} }

// Refresh rereads the tags and replaces what is known in one step.
func (g *Genres) Refresh(ctx context.Context) error {
	g.refreshing.Lock()
	defer g.refreshing.Unlock()
	counts, err := g.st.AlbumTagCounts(ctx, GenreKeys())
	if err != nil {
		return fmt.Errorf("refresh genres: %w", err)
	}
	state := displayGenres(counts)
	g.state.Store(&state)
	return nil
}

// Of returns the genres of the album with the public ID id, the ones on most of
// its tracks first. Nil means none. The caller must not change the slice.
func (g *Genres) Of(id string) []string {
	if state := g.state.Load(); state != nil {
		return state.albums[id]
	}
	return nil
}

// Display returns the genres a track's raw genre values name, split as an
// album's are and spelled the library's way, each once, in the order they are
// tagged. A genre the last refresh didn't see keeps its own spelling. Nil means
// none.
func (g *Genres) Display(values []string) []string {
	var spellings map[string]string
	if state := g.state.Load(); state != nil {
		spellings = state.spellings
	}
	var out, seen []string
	for _, value := range values {
		for _, name := range genreNames(value) {
			if !slices.Contains(seen, name.key) {
				seen = append(seen, name.key)
				out = append(out, cmp.Or(spellings[name.key], name.spelling))
			}
		}
	}
	return out
}
