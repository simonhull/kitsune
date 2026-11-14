package store_test

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

// playlistLib is a library of two albums of three songs each, and a person.
type playlistLib struct {
	s      *store.Store
	clock  *browseClock
	user   int64
	other  int64
	tracks []store.TrackWrite // Album 1's three songs, then album 2's.
}

func newPlaylistLib(tb testing.TB) *playlistLib {
	tb.Helper()
	s, clock := openBrowse(tb)
	u, err := s.CreateUser(tb.Context(), store.NewUser{Name: simon, PasswordHash: "h", Role: store.RoleAdmin})
	if err != nil {
		tb.Fatal(err)
	}
	o, err := s.CreateUser(tb.Context(), store.NewUser{Name: "Other", PasswordHash: "h", Role: store.RoleUser})
	if err != nil {
		tb.Fatal(err)
	}
	var tracks []store.TrackWrite
	for album := 1; album <= 2; album++ {
		for n := 1; n <= 3; n++ {
			w := albumTrack(album, n, fmt.Sprintf("Album %d", album))
			w.TrackNo = n
			w.Title = fmt.Sprintf("Song %d.%d", album, n)
			w.Duration = time.Duration(n) * time.Minute
			tracks = append(tracks, w)
		}
	}
	writeAll(tb, s, tracks...)
	return &playlistLib{s: s, clock: clock, user: u.ID, other: o.ID, tracks: tracks}
}

// id returns the ULID of the library's song i.
func (l *playlistLib) id(i int) string { return l.tracks[i].ULID }

// make makes a playlist of the person's holding the songs at is, in order.
func (l *playlistLib) make(tb testing.TB, name string, is ...int) string {
	tb.Helper()
	id := fmt.Sprintf("01PLAYLIST%016d", len(name)+int(l.clock.Now().UnixMilli()%1_000_000))
	l.clock.Advance(time.Millisecond)
	add := &store.PlaylistAdd{Again: true}
	for _, i := range is {
		add.Tracks = append(add.Tracks, l.id(i))
	}
	if _, err := l.s.CreatePlaylist(tb.Context(), l.user, id, name, add); err != nil {
		tb.Fatal(err)
	}
	return id
}

// order returns the songs of the person's playlist, as indexes into the
// library's songs, in its order.
func (l *playlistLib) order(t *testing.T, id string) []int {
	t.Helper()
	d := must(l.s.Playlist(t.Context(), l.user, id))(t)
	out := make([]int, len(d.Entries))
	for i, e := range d.Entries {
		out[i] = slices.IndexFunc(l.tracks, func(w store.TrackWrite) bool { return w.ULID == e.Track.ID })
	}
	return out
}

// entries returns the entry IDs of the person's playlist, in its order.
func (l *playlistLib) entries(t *testing.T, id string) []int64 {
	t.Helper()
	d := must(l.s.Playlist(t.Context(), l.user, id))(t)
	out := make([]int64, len(d.Entries))
	for i, e := range d.Entries {
		out[i] = e.ID
	}
	return out
}

func TestPlaylistName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"  Falling Up ", "Falling Up", true},
		{"", "", false},
		{"   ", "", false},
		{strings.Repeat("é", store.MostPlaylistName), strings.Repeat("é", store.MostPlaylistName), true},
		{strings.Repeat("é", store.MostPlaylistName+1), "", false},
	} {
		got, err := store.PlaylistName(tc.in)
		if got != tc.want || (err == nil) != tc.ok {
			t.Errorf("PlaylistName(%q) = %q, %v; want %q, ok %v", tc.in, got, err, tc.want, tc.ok)
		}
		if err != nil && !errors.Is(err, store.ErrBadPlaylistName) {
			t.Errorf("PlaylistName(%q) err = %v, want ErrBadPlaylistName", tc.in, err)
		}
	}
}

// A playlist lists with its live songs counted and timed, its mosaic the first
// albums in its order, and how many of the songs asked about it holds.
func TestPlaylists(t *testing.T) {
	t.Parallel()
	l := newPlaylistLib(t)
	falling := l.make(t, "Falling Up", 4, 0, 1, 4)
	empty := l.make(t, "B-sides")
	if _, err := l.s.CreatePlaylist(t.Context(), l.other, "01PLAYLISTOTHER00000000000", "Theirs", &store.PlaylistAdd{}); err != nil {
		t.Fatal(err)
	}
	got := must(l.s.Playlists(t.Context(), l.user, []string{l.id(4), l.id(2), l.id(0)}))(t)
	shapes := make([]string, len(got))
	for i := range got {
		shapes[i] = shapeOf(&got[i])
	}
	want := []string{
		empty + " B-sides: 0 songs, 0 albums, 0s, holds 0, mosaic [], kitsune, never added to",
		falling + " Falling Up: 4 songs, 2 albums, 7m0s, holds 2, mosaic [" + albumID(2) + " " + albumID(1) + "], kitsune, added to",
	}
	if !slices.Equal(shapes, want) {
		t.Errorf("Playlists =\n%s\nwant\n%s", strings.Join(shapes, "\n"), strings.Join(want, "\n"))
	}
	if one := must(l.s.PlaylistSummary(t.Context(), l.user, falling))(t); shapeOf(&one) != strings.Replace(want[1], "holds 2", "holds 0", 1) {
		t.Errorf("PlaylistSummary = %s, want Falling Up as listed", shapeOf(&one))
	}
}

// shapeOf says what a list of playlists shows of p.
func shapeOf(p *store.Playlist) string {
	mosaic := make([]string, len(p.Mosaic))
	for i := range p.Mosaic {
		mosaic[i] = p.Mosaic[i].ID
	}
	added := "added to"
	if p.AddedAt.IsZero() {
		added = "never added to"
	}
	return fmt.Sprintf("%s %s: %d songs, %d albums, %v, holds %d, mosaic %v, %s, %s",
		p.ID, p.Name, p.Songs, p.Albums, p.Duration, p.Holds, mosaic, p.Source, added)
}

// A playlist's page has its entries in order, a song in twice twice, each with
// its song as the index has it, the person's stars and its album.
func TestPlaylistDetail(t *testing.T) {
	t.Parallel()
	l := newPlaylistLib(t)
	id := l.make(t, "Falling Up", 1, 3, 1)
	d := must(l.s.Playlist(t.Context(), l.user, id))(t)
	got := make([]string, len(d.Entries))
	for i, e := range d.Entries {
		got[i] = fmt.Sprintf("%s %s #%d %v by %d", e.Album, e.Track.Title, e.Track.TrackNo, e.Track.Duration, len(e.Track.Artists))
	}
	want := []string{
		albumID(1) + " Song 1.2 #2 2m0s by 1", albumID(2) + " Song 2.1 #1 1m0s by 1", albumID(1) + " Song 1.2 #2 2m0s by 1",
	}
	if !slices.Equal(got, want) || d.Entries[0].ID == d.Entries[2].ID {
		t.Errorf("entries = %q, want %q, the first and last each their own entry", got, want)
	}
	albums := make([]string, len(d.Albums))
	for i, a := range d.Albums {
		albums[i] = a.ID + " " + a.Title
	}
	if wantAlbums := []string{albumID(1) + " Album 1", albumID(2) + " Album 2"}; !slices.Equal(albums, wantAlbums) {
		t.Errorf("albums = %q, want %q", albums, wantAlbums)
	}
	if _, err := l.s.Playlist(t.Context(), l.other, id); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("another person's playlist: err = %v, want ErrNotFound", err)
	}
}

// An add skips what the playlist holds and repeats within itself, unless again,
// and puts an album's songs in, in album order.
func TestAddToPlaylist(t *testing.T) {
	t.Parallel()
	l := newPlaylistLib(t)
	id := l.make(t, "Falling Up", 0)
	got := must(l.s.AddToPlaylist(t.Context(), l.user, id, &store.PlaylistAdd{Tracks: []string{l.id(0), l.id(1), l.id(1), l.id(2)}}))(t)
	if len(got.Added) != 2 || got.Added[0].Track != l.id(1) || got.Added[1].Track != l.id(2) || !slices.Equal(got.Skipped, []string{l.id(0), l.id(1)}) {
		t.Errorf("add = %+v, want songs 1 and 2 added, 0 and 1 skipped", got)
	}
	again := must(l.s.AddToPlaylist(t.Context(), l.user, id, &store.PlaylistAdd{Tracks: []string{l.id(0)}, Again: true}))(t)
	if len(again.Added) != 1 || len(again.Skipped) != 0 {
		t.Errorf("again = %+v, want song 0 added a second time", again)
	}
	album := must(l.s.AddToPlaylist(t.Context(), l.user, id, &store.PlaylistAdd{Albums: []string{albumID(2)}}))(t)
	if len(album.Added) != 3 {
		t.Errorf("album = %+v, want its three songs", album)
	}
	if got, want := l.order(t, id), []int{0, 1, 2, 0, 3, 4, 5}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// An add naming what isn't live names it, and adds nothing.
func TestAddToPlaylistRefusesWhatIsNotLive(t *testing.T) {
	t.Parallel()
	l := newPlaylistLib(t)
	id := l.make(t, "Falling Up")
	trashBy(t, l.s, l.user, l.clock.Now(), l.id(1))
	_, err := l.s.AddToPlaylist(t.Context(), l.user, id, &store.PlaylistAdd{
		Tracks: []string{l.id(0), l.id(1), "01NOSUCHTRACK0000000000000"}, Albums: []string{"01NOSUCHALBUM0000000000000"},
	})
	notLive, ok := errors.AsType[*store.NotLiveError](err)
	if !ok {
		t.Fatalf("err = %v, want a NotLiveError", err)
	}
	if !slices.Equal(notLive.Tracks, []string{l.id(1), "01NOSUCHTRACK0000000000000"}) || !slices.Equal(notLive.Albums, []string{"01NOSUCHALBUM0000000000000"}) {
		t.Errorf("NotLiveError = %+v, want the trashed and the unknown song, and the unknown album", notLive)
	}
	if got := l.order(t, id); len(got) != 0 {
		t.Errorf("order = %v, want nothing added", got)
	}
	if _, err = l.s.AddToPlaylist(t.Context(), l.other, id, &store.PlaylistAdd{Tracks: []string{l.id(0)}}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("another person's: err = %v, want ErrNotFound", err)
	}
}

// A playlist holds at most 10,000 entries: an add past them adds nothing.
func TestAddToPlaylistStopsAtTheLimit(t *testing.T) {
	t.Parallel()
	l := newPlaylistLib(t)
	id := l.make(t, "Long")
	many := make([]string, store.MostPlaylistEntries)
	for i := range many {
		many[i] = l.id(i % len(l.tracks))
	}
	if _, err := l.s.AddToPlaylist(t.Context(), l.user, id, &store.PlaylistAdd{Tracks: many, Again: true}); err != nil {
		t.Fatal(err)
	}
	_, err := l.s.AddToPlaylist(t.Context(), l.user, id, &store.PlaylistAdd{Tracks: many[:1], Again: true})
	if !errors.Is(err, store.ErrPlaylistFull) {
		t.Errorf("one more: err = %v, want ErrPlaylistFull", err)
	}
}

// An add before an entry puts the songs there in order, and Undo's way back for
// a take-out is that add.
func TestAddToPlaylistBefore(t *testing.T) {
	t.Parallel()
	l := newPlaylistLib(t)
	id := l.make(t, "Falling Up", 0, 1)
	entries := l.entries(t, id)
	if _, err := l.s.AddToPlaylist(t.Context(), l.user, id, &store.PlaylistAdd{Tracks: []string{l.id(4), l.id(5)}, Before: entries[1]}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.s.AddToPlaylist(t.Context(), l.user, id, &store.PlaylistAdd{Tracks: []string{l.id(3)}, Before: entries[0]}); err != nil {
		t.Fatal(err)
	}
	if got, want := l.order(t, id), []int{3, 0, 4, 5, 1}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if _, err := l.s.AddToPlaylist(t.Context(), l.user, id, &store.PlaylistAdd{Tracks: []string{l.id(2)}, Before: 999_999}); !errors.Is(err, store.ErrNoEntry) {
		t.Errorf("before an unknown entry: err = %v, want ErrNoEntry", err)
	}
}

// A move goes to the start, the middle or the end, and a song in twice moves
// one entry of it.
func TestMovePlaylistEntry(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		entry, anchor int
		after         bool
		want          []int
	}{
		{"to the start", 3, 0, false, []int{3, 0, 1, 2, 0}},
		{"to the middle", 0, 2, false, []int{1, 0, 2, 3, 0}},
		{"to the end", 1, 4, true, []int{0, 2, 3, 0, 1}},
		{"after one in the middle", 4, 1, true, []int{0, 1, 0, 2, 3}},
		{"where it is", 1, 2, false, []int{0, 1, 2, 3, 0}},
		{"onto itself", 2, 2, true, []int{0, 1, 2, 3, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := newPlaylistLib(t)
			id := l.make(t, "Falling Up", 0, 1, 2, 3, 0)
			e := l.entries(t, id)
			if err := l.s.MovePlaylistEntry(t.Context(), l.user, id, e[tc.entry], e[tc.anchor], tc.after); err != nil {
				t.Fatal(err)
			}
			if got := l.order(t, id); !slices.Equal(got, tc.want) {
				t.Errorf("order = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMovePlaylistEntryRefusesWhatIsNotThere(t *testing.T) {
	t.Parallel()
	l := newPlaylistLib(t)
	id := l.make(t, "Falling Up", 0, 1)
	e := l.entries(t, id)
	if err := l.s.MovePlaylistEntry(t.Context(), l.user, id, e[0], 999_999, false); !errors.Is(err, store.ErrNoEntry) {
		t.Errorf("an unknown anchor: err = %v, want ErrNoEntry", err)
	}
	if err := l.s.MovePlaylistEntry(t.Context(), l.user, id, 999_999, e[0], false); !errors.Is(err, store.ErrNoEntry) {
		t.Errorf("an unknown entry: err = %v, want ErrNoEntry", err)
	}
	if err := l.s.MovePlaylistEntry(t.Context(), l.other, id, e[0], e[1], true); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("another person's: err = %v, want ErrNotFound", err)
	}
}

// Many moves, enough to use up the gaps over and over, keep the playlist in
// the order they made, each entry once.
func TestPlaylistOrderHoldsAfterManyMoves(t *testing.T) {
	t.Parallel()
	l := newPlaylistLib(t)
	is := make([]int, 40)
	for i := range is {
		is[i] = i % len(l.tracks)
	}
	id := l.make(t, "Shuffled by hand", is...)
	want := l.entries(t, id)
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a fixed sequence of moves, not a secret.
	for range 500 {
		from, to := r.IntN(len(want)), r.IntN(len(want))
		after := r.IntN(2) == 0
		// The worst case for a gap: half the moves go just before the first entry.
		if r.IntN(2) == 0 {
			to, after = 0, false
		}
		if err := l.s.MovePlaylistEntry(t.Context(), l.user, id, want[from], want[to], after); err != nil {
			t.Fatal(err)
		}
		want = moved(want, from, to, after)
	}
	if got := l.entries(t, id); !slices.Equal(got, want) {
		t.Errorf("after 500 moves the order is %v, want %v", got, want)
	}
}

// moved is ids with the one at from moved before the one at to, or after it.
func moved(ids []int64, from, to int, after bool) []int64 {
	if from == to {
		return ids
	}
	entry, anchor := ids[from], ids[to]
	out := slices.Delete(slices.Clone(ids), from, from+1)
	at := slices.Index(out, anchor)
	if after {
		at++
	}
	return slices.Insert(out, at, entry)
}

// A take-out removes the entries named, never the songs, and leaves alone an
// entry that isn't there.
func TestRemoveFromPlaylist(t *testing.T) {
	t.Parallel()
	l := newPlaylistLib(t)
	id := l.make(t, "Falling Up", 0, 1, 0)
	e := l.entries(t, id)
	got := must(l.s.RemoveFromPlaylist(t.Context(), l.user, id, []int64{e[2], 999_999}))(t)
	if !slices.Equal(got, []int64{e[2]}) {
		t.Errorf("removed %v, want %v", got, e[2:])
	}
	if order := l.order(t, id); !slices.Equal(order, []int{0, 1}) {
		t.Errorf("order = %v, want the first entry of song 0 kept", order)
	}
	if _, err := l.s.RemoveFromPlaylist(t.Context(), l.other, id, e[:1]); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("another person's: err = %v, want ErrNotFound", err)
	}
}

func TestRenameAndDeletePlaylist(t *testing.T) {
	t.Parallel()
	l := newPlaylistLib(t)
	id := l.make(t, "Falling Up", 0)
	if err := l.s.RenamePlaylist(t.Context(), l.other, id, "Mine now"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("rename another person's: err = %v, want ErrNotFound", err)
	}
	if err := l.s.RenamePlaylist(t.Context(), l.user, id, "Falling Down"); err != nil {
		t.Fatal(err)
	}
	if got := must(l.s.PlaylistSummary(t.Context(), l.user, id))(t); got.Name != "Falling Down" {
		t.Errorf("name = %q, want Falling Down", got.Name)
	}
	if err := l.s.DeletePlaylist(t.Context(), l.other, id); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("delete another person's: err = %v, want ErrNotFound", err)
	}
	if err := l.s.DeletePlaylist(t.Context(), l.user, id); err != nil {
		t.Fatal(err)
	}
	if _, err := l.s.Playlist(t.Context(), l.user, id); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("after delete: err = %v, want ErrNotFound", err)
	}
	if _, err := l.s.TrashableTracks(t.Context(), []string{l.id(0)}); err != nil {
		t.Errorf("the song went with the playlist: %v", err)
	}
}

// A song in Trash leaves every view of a playlist and comes back with Restore;
// emptying the Trash takes its entries with it.
func TestPlaylistKeepsTrashedEntriesUntilTheyGo(t *testing.T) {
	t.Parallel()
	l := newPlaylistLib(t)
	id := l.make(t, "Falling Up", 0, 1, 2, 1)
	trashed := trashBy(t, l.s, l.user, l.clock.Now(), l.id(1))
	if got := l.order(t, id); !slices.Equal(got, []int{0, 2}) {
		t.Errorf("with song 1 in Trash the playlist shows %v, want [0 2]", got)
	}
	if p := must(l.s.PlaylistSummary(t.Context(), l.user, id))(t); p.Songs != 2 {
		t.Errorf("counted %d songs, want 2", p.Songs)
	}
	if err := l.s.UnmarkTrashed(t.Context(), []int64{trashed[0].ID}); err != nil {
		t.Fatal(err)
	}
	if got := l.order(t, id); !slices.Equal(got, []int{0, 1, 2, 1}) {
		t.Errorf("after Restore the playlist shows %v, want [0 1 2 1]", got)
	}
	trashed = trashBy(t, l.s, l.user, l.clock.Now(), l.id(1))
	if _, err := l.s.DeleteTrashed(t.Context(), []int64{trashed[0].ID}); err != nil {
		t.Fatal(err)
	}
	rows := must(l.s.PlaylistTrackRows(t.Context(), l.user, id))(t)
	if len(rows) != 2 {
		t.Errorf("after the purge the playlist holds %d songs, live or not, want 2", len(rows))
	}
}
