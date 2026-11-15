package navidrome

import (
	"strings"
	"time"
	"unicode/utf8"

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
	Source int   // Every playlist they own.
	Smart  int   // Of rules: left for smart playlists to come (plan 066).
	Songs  Tally // The entries of the others, a song as often as it is in one.
	// Cut is how many matched entries were left off the end of a playlist
	// longer than [store.MostPlaylistEntries].
	Cut int
}

// Imported is how many playlists come over.
func (p *Playlists) Imported() int { return p.Source - p.Smart }

// playlists plans the person's playlists made by hand: each keeps
// Navidrome's order and repeats, less the songs that matched nothing. Their
// IDs are left for [New] to make.
func (p *planner) playlists(out *Person, marks *Marks, note func(string)) {
	r := &out.Report.Playlists
	for i := range marks.Playlists {
		nd := &marks.Playlists[i]
		r.Source++
		if nd.Smart {
			r.Smart++
			continue
		}
		pl := store.ImportedPlaylist{Key: playlistKeyOf(nd), Name: playlistName(nd.Name), CreatedAt: p.or(nd.CreatedAt), Tracks: []string{}}
		for _, id := range nd.Songs {
			note(id)
			m := p.matches[id]
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

// nameIDs gives every planned playlist a new ULID, from newID at now.
func nameIDs(plan *Plan, newID func(time.Time) (string, error), now time.Time) error {
	for i := range plan.People {
		imp := &plan.People[i].Import
		for j := range imp.Playlists {
			id, err := newID(now)
			if err != nil {
				return err
			}
			imp.Playlists[j].ID = id
		}
	}
	return nil
}

// playlistKeyOf returns a Navidrome playlist's import key, which a re-run finds
// it by.
func playlistKeyOf(pl *navidromedb.Playlist) string { return playlistKeyPrefix + pl.ID }
