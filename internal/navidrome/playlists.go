package navidrome

import (
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/smartlist"
	"github.com/simonhull/kitsune/internal/store"
	"github.com/simonhull/kitsune/internal/store/navidromedb"
)

// playlistKeyPrefix starts the import key of a playlist from Navidrome; its ID
// follows.
const playlistKeyPrefix = "navidrome:"

// unnamedPlaylist is what a playlist Navidrome kept with no name is called.
const unnamedPlaylist = "Navidrome playlist"

// Playlists is what a person's import found of their playlists.
type Playlists struct {
	Source int // Every playlist they own.
	Smart  int // Of rules, those left out included.
	// LeftOut are the smart ones whose criteria don't all map, which stay
	// behind whole, in Navidrome's order.
	LeftOut []LeftOut
	Songs   Tally // The entries of the others, a song as often as it is in one.
	// Cut is how many matched entries were left off the end of a playlist
	// longer than [store.MostPlaylistEntries].
	Cut int
}

// LeftOut is a smart playlist that stays behind, and the criteria that kept
// it: an operator, a field Kitsune lacks, or what can't be said, each once.
type LeftOut struct {
	Name string
	Why  []string
}

// Imported is how many playlists come over.
func (p *Playlists) Imported() int { return p.Source - len(p.LeftOut) }

// whyCycle is what keeps smart playlists that name each other round in a
// cycle.
const whyCycle = "inPlaylist cycle"

// playlists plans the person's playlists: each made by hand keeps Navidrome's
// order and repeats, less the songs that matched nothing; each smart one comes
// over as rules when all its criteria map, loveFrom saying what loved is, and
// stays behind whole otherwise.
func (p *planner) playlists(out *Person, marks *Marks, loveFrom rating.Rating, note func(string)) {
	r := &out.Report.Playlists
	ids := make(map[string]string, len(marks.Playlists)) // Kitsune's ID of each, by Navidrome's.
	var smart []*navidromedb.Playlist
	for i := range marks.Playlists {
		nd := &marks.Playlists[i]
		r.Source++
		id, ok := p.id()
		if !ok {
			return
		}
		ids[nd.ID] = id
		if nd.Smart {
			r.Smart++
			smart = append(smart, nd)
			continue
		}
		pl := store.ImportedPlaylist{ID: id, Key: playlistKeyOf(nd), Name: playlistName(nd.Name), CreatedAt: p.or(nd.CreatedAt), Tracks: []string{}}
		for _, song := range nd.Songs {
			note(song)
			m := p.matches[song]
			r.Songs.add(m.how)
			if m.how != unmatched {
				pl.Tracks = append(pl.Tracks, m.track)
			}
		}
		if extra := len(pl.Tracks) - store.MostPlaylistEntries; extra > 0 {
			r.Cut += extra
			pl.Tracks = pl.Tracks[:store.MostPlaylistEntries]
		}
		out.Import.Playlists = append(out.Import.Playlists, pl)
	}
	mapped := mapSmart(smart, loveFrom, ids)
	for _, nd := range smart {
		if why, gone := mapped.left[nd.ID]; gone {
			r.LeftOut = append(r.LeftOut, LeftOut{Name: playlistName(nd.Name), Why: why})
			continue
		}
		rules := mapped.rules[nd.ID]
		out.Import.Playlists = append(out.Import.Playlists, store.ImportedPlaylist{
			ID: ids[nd.ID], Key: playlistKeyOf(nd), Name: playlistName(nd.Name), CreatedAt: p.or(nd.CreatedAt),
			Tracks: []string{}, Rules: &rules,
		})
	}
}

// id makes a playlist's ULID, reporting false once making one has failed.
func (p *planner) id() (string, bool) {
	if p.idErr != nil {
		return "", false
	}
	id, err := p.newID(p.now)
	if err != nil {
		p.idErr = err
		return "", false
	}
	return id, true
}

// smartMapped is what became of a person's smart playlists, by Navidrome ID:
// the rules of those that come over and, for the rest, what stopped them.
type smartMapped struct {
	rules map[string]smartlist.Rules
	left  map[string][]string
}

// mapSmart maps each smart playlist's criteria. A playlist rule maps only to a
// playlist coming over, so one left out takes with it every smart playlist
// that names it, and so do smart playlists that name each other round in a
// cycle.
func mapSmart(smart []*navidromedb.Playlist, loveFrom rating.Rating, ids map[string]string) smartMapped {
	coming := maps.Clone(ids)
	left := map[string][]string{}
	for {
		mapped := map[string]smartlist.Rules{}
		names := map[string][]string{} // The playlists each names.
		changed := false
		for _, nd := range smart {
			if _, gone := left[nd.ID]; gone {
				continue
			}
			c := mapCriteria(nd.Rules, loveFrom, coming)
			if c.stopped != nil {
				left[nd.ID] = c.stopped
				delete(coming, nd.ID)
				changed = true
				continue
			}
			mapped[nd.ID] = c.rules
			names[nd.ID] = c.named
		}
		for _, id := range inCycles(names) {
			left[id] = []string{whyCycle}
			delete(coming, id)
			changed = true
		}
		if !changed {
			return smartMapped{rules: mapped, left: left}
		}
	}
}

// inCycles returns the playlists that reach themselves through those they
// name, in ID order.
func inCycles(names map[string][]string) []string {
	var out []string
	for _, start := range slices.Sorted(maps.Keys(names)) {
		seen := map[string]bool{}
		next := slices.Clone(names[start])
		for len(next) > 0 {
			id := next[len(next)-1]
			next = next[:len(next)-1]
			if id == start {
				out = append(out, start)
				break
			}
			if !seen[id] {
				seen[id] = true
				next = append(next, names[id]...)
			}
		}
	}
	return out
}

// playlistName is a Navidrome playlist's name as Kitsune takes it: trimmed,
// cut to [store.MostPlaylistName] characters, and [unnamedPlaylist] for none.
func playlistName(name string) string {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > store.MostPlaylistName {
		name = strings.TrimSpace(string([]rune(name)[:store.MostPlaylistName]))
	}
	if name == "" {
		return unnamedPlaylist
	}
	return name
}

// playlistKeyOf returns a Navidrome playlist's import key, which a re-run finds
// it by.
func playlistKeyOf(pl *navidromedb.Playlist) string { return playlistKeyPrefix + pl.ID }
