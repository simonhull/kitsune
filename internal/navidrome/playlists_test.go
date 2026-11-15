package navidrome_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/navidrome"
	"github.com/simonhull/kitsune/internal/store"
	"github.com/simonhull/kitsune/internal/store/navidromedb"
)

// playlistWorld is two songs Kitsune has and one it doesn't, a playlist made by
// hand holding them in its own order with a repeat, and a smart one.
func playlistWorld() world {
	return world{
		songs:  []navidromedb.Song{song("s1", pathOne), song("s2", pathY), song("s3", pathGone)},
		tracks: []store.ImportTrack{ktrack('1', pathOne, albumA), ktrack('2', pathY, albumB)},
		lists: []navidromedb.Playlist{
			{ID: "p1", Name: "  Falling Up ", CreatedAt: ts(3, 0), Songs: []string{"s2", "s1", "s3", "s2"}},
			{ID: "p2", Name: "Recent", Smart: true, Songs: []string{"s1"}},
		},
		opts: navidrome.Options{NewID: func(time.Time) (string, error) { return "01PLAYLISTNEW0000000000000", nil }},
	}
}

func TestPlaylistsKeepTheirOrderAndRepeats(t *testing.T) {
	t.Parallel()
	w := playlistWorld()
	p := w.person(t)
	want := []store.ImportedPlaylist{{
		ID: "01PLAYLISTNEW0000000000000", Key: "navidrome:p1", Name: "Falling Up", CreatedAt: ts(3, 0),
		Tracks: []string{track('2'), track('1'), track('2')},
	}}
	if got := p.Import.Playlists; len(got) != 1 || !playlistEqual(&got[0], &want[0]) {
		t.Errorf("playlists = %+v, want %+v: the smart one left behind", got, want)
	}
	r := p.Report.Playlists
	if r.Source != 2 || r.Smart != 1 || r.Imported() != 1 || r.Songs.Source != 4 || r.Songs.ByPath != 3 || r.Songs.Unmatched != 1 {
		t.Errorf("report = %+v, want 2 playlists, 1 smart, 4 songs, 3 by path and 1 unmatched", r)
	}
	if !slices.Equal(p.Report.Unmatched, []string{pathGone}) {
		t.Errorf("unmatched = %v, want the song Kitsune doesn't have", p.Report.Unmatched)
	}
}

func playlistEqual(a, b *store.ImportedPlaylist) bool {
	return a.ID == b.ID && a.Key == b.Key && a.Name == b.Name && a.CreatedAt.Equal(b.CreatedAt) && slices.Equal(a.Tracks, b.Tracks)
}

// A playlist with no name gets one, one too long is cut, and one with no time
// is made now.
func TestPlaylistNamesAndTimes(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("ü", store.MostPlaylistName+5)
	w := playlistWorld()
	w.lists = []navidromedb.Playlist{{ID: "p1", Name: "   "}, {ID: "p2", Name: long}}
	p := w.person(t)
	got := p.Import.Playlists
	if len(got) != 2 || got[0].Name != "Navidrome playlist" || got[1].Name != long[:2*store.MostPlaylistName] || !got[0].CreatedAt.Equal(now) {
		t.Errorf("playlists = %+v, want one called Navidrome playlist made now, and one cut to %d characters", got, store.MostPlaylistName)
	}
	for i := range got {
		if _, err := store.PlaylistName(got[i].Name); err != nil {
			t.Errorf("%q isn't a name: %v", got[i].Name, err)
		}
	}
}

// A playlist past Kitsune's limit keeps its first 10,000 songs, and the report
// counts the rest.
func TestAPlaylistTooLongIsCut(t *testing.T) {
	t.Parallel()
	w := playlistWorld()
	w.lists = []navidromedb.Playlist{{ID: "p1", Name: "Long", Songs: slices.Repeat([]string{"s1"}, store.MostPlaylistEntries+3)}}
	p := w.person(t)
	if got := p.Import.Playlists; len(got) != 1 || len(got[0].Tracks) != store.MostPlaylistEntries || p.Report.Playlists.Cut != 3 {
		t.Errorf("tracks %d, cut %d; want %d and 3", len(got[0].Tracks), p.Report.Playlists.Cut, store.MostPlaylistEntries)
	}
}

// A playlist's songs count toward the gate, as plays and marks do.
func TestPlaylistSongsCountTowardTheGate(t *testing.T) {
	t.Parallel()
	w := playlistWorld()
	w.lists = []navidromedb.Playlist{{ID: "p1", Name: "Mostly gone", Songs: []string{"s1", "s3"}}}
	w.songs = append(w.songs, song("s4", "4.flac"))
	w.lists[0].Songs = append(w.lists[0].Songs, "s4")
	plan, err := w.plan(t)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Gate != (navidrome.Gate{Songs: 3, ByPath: 1}) || !errors.Is(plan.Gate.Check(), navidrome.ErrGate) {
		t.Errorf("gate = %+v, want 1 of 3 by path, and stopped", plan.Gate)
	}
}

// Every playlist brought over takes an ID of its own; a failure to make one
// stops the plan.
func TestPlaylistIDs(t *testing.T) {
	t.Parallel()
	w := playlistWorld()
	w.opts.NewID = nil // library.NewID, as the command uses.
	w.lists = append(w.lists, navidromedb.Playlist{ID: "p3", Name: "Another"})
	p := w.person(t)
	if got := p.Import.Playlists; len(got) != 2 || len(got[0].ID) != 26 || got[0].ID == got[1].ID {
		t.Errorf("IDs = %q, %q; want two ULIDs", got[0].ID, got[1].ID)
	}
	errBroken := errors.New("no entropy")
	w.opts.NewID = func(time.Time) (string, error) { return "", errBroken }
	if _, err := w.plan(t); !errors.Is(err, errBroken) {
		t.Errorf("err = %v, want the ID's", err)
	}
}
