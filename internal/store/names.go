package store

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
)

// variousArtistsMBID is MusicBrainz's special artist for compilations.
const variousArtistsMBID = "89ad4ac3-39f7-470e-963a-56509c546377"

// variousArtistsKey is that artist's match key. It is a real row, but nobody's name.
const variousArtistsKey = "various artists"

// isVarious reports whether an artist row is the Various Artists placeholder.
func isVarious(matchKey string, mbid *string) bool {
	return matchKey == variousArtistsKey || (mbid != nil && strings.EqualFold(*mbid, variousArtistsMBID))
}

// Typo allowance: nothing for a very short word, and one more edit per
// runesPerTypo runes, up to maxTypos.
const (
	minTypoRunes = 3
	runesPerTypo = 6
	maxTypos     = 3
)

// allowedTypos returns how many edits a query of n runes may differ by.
func allowedTypos(n int) int {
	if n < minTypoRunes {
		return 0
	}
	return min(maxTypos, 1+n/runesPerTypo)
}

// spelling is a form with its runes laid out for edit distance.
type spelling struct {
	form

	runes     []rune
	wordRunes [][]rune
}

// spellings lays out fs for edit distance.
func spellings(fs []form) []spelling {
	out := make([]spelling, len(fs))
	for i := range fs {
		wordRunes := make([][]rune, len(fs[i].words))
		for j, w := range fs[i].words {
			wordRunes[j] = []rune(w)
		}
		out[i] = spelling{form: fs[i], runes: []rune(fs[i].text), wordRunes: wordRunes}
	}
	return out
}

// nameEntry is one artist or album in a [nameIndex].
type nameEntry struct {
	id        string // The ULID.
	forms     []form
	spellings []spelling
}

// nameIndex is a list of names that is searched in memory. FTS can't do this
// job: it finds 3.4% of mistyped artists, and can't say which artist matched.
// It is never changed once built.
type nameIndex []nameEntry

// with returns ix with one more name. A name with no letter or digit stays out:
// no query could reach it.
func (ix nameIndex) with(id, name string) nameIndex {
	if fs := forms(name); len(fs) > 0 {
		return append(ix, nameEntry{id: id, forms: fs, spellings: spellings(fs)})
	}
	return ix
}

// editRows is scratch space for edit distance.
type editRows struct{ older, prev, cur []int }

// nameHit is an entry that matched.
type nameHit struct {
	entry     *nameEntry
	closeness closeness
	typos     int
}

// match returns the IDs of the best entries for query, at most maxNameHits,
// closest first. Typos are only looked for when plain matches leave room: they
// rank last.
func (ix nameIndex) match(query []form) []string {
	var hits []nameHit
	var rest []*nameEntry
	for i := range ix {
		if c := closenessOf(ix[i].forms, query); c != closeNone {
			hits = append(hits, nameHit{entry: &ix[i], closeness: c})
		} else {
			rest = append(rest, &ix[i])
		}
	}
	if len(hits) < maxNameHits {
		var rows editRows
		q := spellings(query)
		for _, e := range rest {
			if hit := e.typoHit(q, &rows); hit.closeness != closeNone {
				hits = append(hits, hit)
			}
		}
	}
	slices.SortFunc(hits, compareHits)
	hits = hits[:min(len(hits), maxNameHits)]
	ids := make([]string, len(hits))
	for i := range hits {
		ids[i] = hits[i].entry.id
	}
	return ids
}

// compareHits orders hits best first; among equals, the shorter name is the
// nearer one, and the rest is only there to keep results stable.
func compareHits(a, b nameHit) int {
	x, y := &a.entry.spellings[0], &b.entry.spellings[0]
	return cmp.Or(
		cmp.Compare(a.closeness, b.closeness),
		cmp.Compare(a.typos, b.typos),
		cmp.Compare(len(x.runes), len(y.runes)),
		cmp.Compare(x.text, y.text),
		cmp.Compare(a.entry.id, b.entry.id),
	)
}

// typoHit returns how well e matches query allowing typos, the closest of all
// its spellings. Its closeness is closeNone when it is too far.
func (e *nameEntry) typoHit(query []spelling, rows *editRows) nameHit {
	best := nameHit{entry: e, closeness: closeNone}
	for i := range e.spellings {
		for j := range query {
			n, q := &e.spellings[i], &query[j]
			if typos, ok := rows.within(n.runes, q.runes, allowedTypos(len(q.runes))); ok {
				best = closer(best, nameHit{entry: e, closeness: closeTypo, typos: typos})
			}
			if typos, ok := rows.wordTypos(n, q); ok {
				best = closer(best, nameHit{entry: e, closeness: closeWordTypo, typos: typos})
			}
		}
	}
	return best
}

// closer returns the better of two hits on the same entry.
func closer(a, b nameHit) nameHit {
	if cmp.Or(cmp.Compare(a.closeness, b.closeness), cmp.Compare(a.typos, b.typos)) <= 0 {
		return a
	}
	return b
}

// wordTypos returns the total edits that put every word of q in name, and
// whether that can be done.
func (r *editRows) wordTypos(name, q *spelling) (int, bool) {
	total := 0
	for i := range q.words {
		typos, ok := r.nearestWord(name, q.words[i], q.wordRunes[i])
		if !ok {
			return 0, false
		}
		total += typos
	}
	return total, true
}

// nearestWord returns the edits from word to the nearest word of name, and
// whether any is near enough. Starting a word of name costs nothing.
func (r *editRows) nearestWord(name *spelling, word string, runes []rune) (int, bool) {
	if wordIn(name.words, word) != closeNone {
		return 0, true
	}
	limit := allowedTypos(len(runes))
	best := limit + 1
	for _, w := range name.wordRunes {
		if typos, ok := r.within(w, runes, limit); ok {
			best = min(best, typos)
		}
	}
	return best, best <= limit
}

// within returns the edits between a and b, counting an insert, a delete, a
// change or a swap of neighbors as one, and whether that is at most limit. A
// limit of 0 is always false: it means no typo is forgiven, so don't look.
func (r *editRows) within(a, b []rune, limit int) (int, bool) {
	if limit == 0 || max(len(a)-len(b), len(b)-len(a)) > limit {
		return 0, false
	}
	r.older, r.prev, r.cur = sized(r.older, len(b)+1), sized(r.prev, len(b)+1), sized(r.cur, len(b)+1)
	for j := range r.prev {
		r.prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		if r.fill(a, b, i) > limit {
			return 0, false
		}
		r.older, r.prev, r.cur = r.prev, r.cur, r.older
	}
	return r.prev[len(b)], r.prev[len(b)] <= limit
}

// fill computes row i of the distance table into cur and returns its lowest cell.
func (r *editRows) fill(a, b []rune, i int) int {
	r.cur[0] = i
	lowest := i
	for j := 1; j <= len(b); j++ {
		edit := r.prev[j-1]
		if a[i-1] != b[j-1] {
			edit++
		}
		edit = min(edit, r.prev[j]+1, r.cur[j-1]+1)
		if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
			edit = min(edit, r.older[j-2]+1)
		}
		r.cur[j] = edit
		lowest = min(lowest, edit)
	}
	return lowest
}

// sized returns row with length n, reusing its memory when it can.
func sized(row []int, n int) []int { return slices.Grow(row[:0], n)[:n] }

// names is the search index of artists and albums.
type names struct{ artists, albums nameIndex }

// RefreshNames rebuilds the index that finds artists and albums by name, from
// the live library. Call it after anything that adds, removes or renames either.
// A search never waits for it: searches use the old index until the new one
// replaces it in one step.
func (s *Store) RefreshNames(ctx context.Context) error {
	s.refreshing.Lock()
	defer s.refreshing.Unlock()
	artists, err := s.rq.ListArtistNames(ctx)
	if err != nil {
		return fmt.Errorf("refresh names: artists: %w", err)
	}
	albums, err := s.rq.ListAlbumNames(ctx)
	if err != nil {
		return fmt.Errorf("refresh names: albums: %w", err)
	}
	index := names{artists: make(nameIndex, 0, len(artists)), albums: make(nameIndex, 0, len(albums))}
	for i := range artists {
		if a := &artists[i]; !isVarious(a.MatchKey, a.Mbid) {
			index.artists = index.artists.with(a.Ulid, a.Name)
		}
	}
	for i := range albums {
		index.albums = index.albums.with(albums[i].Ulid, albums[i].Name)
	}
	s.names.Store(&index)
	return nil
}
