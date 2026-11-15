package store_test

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/smartlist"
	"github.com/simonhull/kitsune/internal/store"
)

// smartSong is one song of the smart playlist tests' library.
type smartSong struct {
	album, track int
	title        string
	minutes      int
	format       string
	kbps         int
	size         int64
	year         int
	artists      []string
	added        time.Duration // How long before the clock's start it came in.
}

// The fixture's artists and its lossless format.
const (
	fallingUp = "Falling Up"
	bjork     = "Björk"
	flac      = "FLAC"
)

// smartAlbums are the library's albums, by number: name, display artist, album
// artists.
var smartAlbums = map[int][3]string{ //nolint:gochecknoglobals // A fixed fixture.
	1: {"Captiva", fallingUp, fallingUp},
	2: {"Fangs!", fallingUp, fallingUp},
	3: {"Björk Live", bjork, ""},
}

// smartSongs are, in the Library's order: Björk files under B, then Falling
// Up's albums newest first (Fangs! of 2009, Captiva of 2007).
var smartSongs = []smartSong{ //nolint:gochecknoglobals // A fixed fixture.
	{3, 1, joga, 5, flac, 1000, 30_000_000, 0, []string{bjork}, 400 * 24 * time.Hour},
	{2, 1, eoptian, 4, "MP3", 320, 9_000_000, 2009, []string{fallingUp}, 30 * 24 * time.Hour},
	{2, 2, arrows, 4, "MP3", 256, 8_000_000, 2009, []string{fallingUp, "Guest; DROP"}, 30 * 24 * time.Hour},
	{1, 1, guide, 4, flac, 900, 600_000_000, 2007, []string{fallingUp}, 2 * 24 * time.Hour},
	{1, 2, hotel, 3, flac, 1049, 500_000_000, 2007, []string{fallingUp}, 2 * 24 * time.Hour},
	{1, 3, gravity, 6, flac, 1092, 700_000_000, 2007, []string{fallingUp}, 2 * 24 * time.Hour},
}

// smartLib is the library above with a person and their stars, plays and skips.
type smartLib struct {
	*playlistLib

	ids []string // By index into smartSongs.
}

func newSmartLib(tb testing.TB) *smartLib {
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
	l := &smartLib{playlistLib: &playlistLib{s: s, clock: clock, user: u.ID, other: o.ID}}
	start := clock.Now()
	for i, song := range smartSongs {
		a := smartAlbums[song.album]
		w := albumTrack(song.album, song.track, a[0])
		w.Title, w.SortTitle, w.TrackNo, w.Year = song.title, song.title, song.track, song.year
		w.Duration = time.Duration(song.minutes)*time.Minute + 30*time.Second
		w.Format, w.Bitrate, w.Size = song.format, song.kbps*1000, song.size
		w.Album.Name, w.Album.Artist, w.Album.Year = a[0], a[1], song.year
		w.Artists = nil
		for _, name := range song.artists {
			w.Artists = append(w.Artists, artistWrite(name))
		}
		if a[2] != "" {
			w.AlbumArtists = []store.ArtistWrite{artistWrite(a[2])}
		}
		clock.now = start.Add(-song.added)
		writeAll(tb, s, w)
		l.ids = append(l.ids, w.ULID)
		l.tracks = append(l.tracks, w)
		_ = i
	}
	clock.now = start
	return l
}

// smartArtists numbers the fixture's artists, for their ULIDs.
var smartArtists = []string{bjork, fallingUp, "Guest; DROP"} //nolint:gochecknoglobals // A fixed fixture.

func artistWrite(name string) store.ArtistWrite {
	ulid := fmt.Sprintf("03A%023d", slices.Index(smartArtists, name))
	return store.ArtistWrite{ULID: ulid, MatchKey: strings.ToLower(name), Name: name, SortName: name}
}

// song returns the index of the song titled title.
func (l *smartLib) song(tb testing.TB, title string) int {
	tb.Helper()
	i := slices.IndexFunc(smartSongs, func(s smartSong) bool { return s.title == title })
	if i < 0 {
		tb.Fatalf("no song %q", title)
	}
	return i
}

func (l *smartLib) rate(tb testing.TB, title string, halves rating.Rating) {
	tb.Helper()
	if err := l.s.Rate(tb.Context(), l.user, l.ids[l.song(tb, title)], halves); err != nil {
		tb.Fatal(err)
	}
}

// play records n plays of the song, the latest ago before now.
func (l *smartLib) play(tb testing.TB, title string, n int, ago time.Duration) {
	tb.Helper()
	for k := range n {
		p := &store.NewPlay{
			UserID: l.user, TrackID: l.ids[l.song(tb, title)], Source: "test",
			PlayedAt: l.clock.Now().Add(-ago - time.Duration(k)*time.Hour), ClientID: fmt.Sprintf("%s-%d-%d", title, n, k),
		}
		if err := l.s.RecordPlay(tb.Context(), p); err != nil {
			tb.Fatal(err)
		}
	}
}

func (l *smartLib) skip(tb testing.TB, title string, n int) {
	tb.Helper()
	for k := range n {
		err := l.s.RecordListen(tb.Context(), &store.NewListen{
			UserID: l.user, TrackID: l.ids[l.song(tb, title)], ClientID: fmt.Sprintf("skip-%s-%d", title, k),
			StartedAt: l.clock.Now().Add(-time.Hour), Listened: time.Second, Ended: store.Skipped,
		})
		if err != nil {
			tb.Fatal(err)
		}
	}
}

// marked gives the person their stars, plays and skips:
// Hotel Aquarium 4★, played 8 times, last 3 days ago; A Guide to Marine Life
// 3★, played twice, 20 days ago; Golden Arrows ½★, skipped twice; Jóga 5★,
// played once, 40 days ago.
func (l *smartLib) marked(tb testing.TB) *smartLib {
	tb.Helper()
	l.rate(tb, "Hotel Aquarium", 8)
	l.rate(tb, "A Guide to Marine Life", 6)
	l.rate(tb, "Golden Arrows", 1)
	l.rate(tb, "Jóga", 10)
	l.play(tb, "Hotel Aquarium", 8, 3*24*time.Hour)
	l.play(tb, "A Guide to Marine Life", 2, 20*24*time.Hour)
	l.play(tb, "Jóga", 1, 40*24*time.Hour)
	l.skip(tb, "Golden Arrows", 2)
	return l
}

func parseRules(tb testing.TB, in string) smartlist.Rules {
	tb.Helper()
	rules, err := smartlist.Parse([]byte(in))
	if err != nil {
		tb.Fatalf("Parse(%s) = %v", in, err)
	}
	return rules
}

// genresOf stands in for library.Genres: Captiva is Rock, Fangs! Rock and
// Electronic, and Björk Live Électronique.
func genresOf(keep func(string) bool) []string {
	var out []string
	for album, genres := range map[int][]string{1: {genreRock}, 2: {genreRock, "Electronic"}, 3: {"Électronique"}} {
		if slices.ContainsFunc(genres, keep) {
			out = append(out, albumID(album))
		}
	}
	return out
}

// chosen returns the titles the rules choose, in order.
func (l *smartLib) chosen(tb testing.TB, in string, seed int64) []string {
	tb.Helper()
	rules := parseRules(tb, in)
	p, err := l.s.PreviewRules(tb.Context(), l.user, &rules, "", seed, genresOf)
	if err != nil {
		tb.Fatal(err)
	}
	out := make([]string, len(p.First))
	for i := range p.First {
		out[i] = p.First[i].Title
	}
	return out
}

func rule(field, op string, value any) string {
	if s, ok := value.(string); ok {
		return fmt.Sprintf(`{"match":"all","rules":[{"field":%q,"op":%q,"value":%q}]}`, field, op, s)
	}
	return fmt.Sprintf(`{"match":"all","rules":[{"field":%q,"op":%q,"value":%v}]}`, field, op, value)
}

const (
	joga     = "Jóga"
	eoptian  = "A Colour Eoptian" //nolint:misspell // A real song title, spelled as Falling Up spell it.
	arrows   = "Golden Arrows"
	guide    = "A Guide to Marine Life"
	hotel    = "Hotel Aquarium"
	gravity  = "Goodnight Gravity"
	songsAll = "all"
)

// Each field with each of its operators, against the library and the person's
// marks, every result in the Library's order.
func TestSmartRulesMatchEachFieldAndOperator(t *testing.T) {
	t.Parallel()
	l := newSmartLib(t).marked(t)
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"title is, folded", rule("title", "is", "HOTEL aquarium"), []string{hotel}},
		{"title is not", rule("title", "isNot", "Hotel Aquarium"), []string{joga, eoptian, arrows, guide, gravity}},
		{"title contains, accents folded", rule("title", "contains", "JOG"), []string{joga}},
		{"title does not contain", rule("title", "notContains", "a"), []string{}},
		{"album is", rule("album", "is", "captiva"), []string{guide, hotel, gravity}},
		{"album contains", rule("album", "contains", "!"), []string{eoptian, arrows}},
		{"album is not", rule("album", "isNot", "Captiva"), []string{joga, eoptian, arrows}},
		{"album does not contain", rule("album", "notContains", "live"), []string{eoptian, arrows, guide, hotel, gravity}},
		{"artist is any of them", rule("artist", "is", "guest; drop"), []string{arrows}},
		{"artist is not", rule("artist", "isNot", "Falling Up"), []string{joga}},
		{"artist contains", rule("artist", "contains", "bjo"), []string{joga}},
		{"artist does not contain", rule("artist", "notContains", "guest"), []string{joga, eoptian, guide, hotel, gravity}},
		{"album artist is", rule("albumArtist", "is", "falling up"), []string{eoptian, arrows, guide, hotel, gravity}},
		{"album artist is not, none counts as not", rule("albumArtist", "isNot", "Falling Up"), []string{joga}},
		{"album artist contains", rule("albumArtist", "contains", "fall"), []string{eoptian, arrows, guide, hotel, gravity}},
		{"album artist does not contain", rule("albumArtist", "notContains", "up"), []string{joga}},
		{"genre is, as displayed", rule("genre", "is", "rock"), []string{eoptian, arrows, guide, hotel, gravity}},
		{"genre contains, accents folded", rule("genre", "contains", "electron"), []string{joga, eoptian, arrows}},
		{"genre is not", rule("genre", "isNot", "Rock"), []string{joga}},
		{"genre does not contain", rule("genre", "notContains", "electr"), []string{guide, hotel, gravity}},
		{"year is", rule("year", "is", 2009), []string{eoptian, arrows}},
		{"year is at least", rule("year", "atLeast", 2008), []string{eoptian, arrows}},
		{"year is at most, unknown is 0", rule("year", "atMost", 2007), []string{joga, guide, hotel, gravity}},
		{"rating is", rule("rating", "is", 8), []string{hotel}},
		{"rating is at least", rule("rating", "atLeast", 6), []string{joga, guide, hotel}},
		{"rating is at most, unrated under every line", rule("rating", "atMost", 1), []string{eoptian, arrows, gravity}},
		{"plays is", rule("plays", "is", 0), []string{eoptian, arrows, gravity}},
		{"plays is at least", rule("plays", "atLeast", 2), []string{guide, hotel}},
		{"plays is at most", rule("plays", "atMost", 1), []string{joga, eoptian, arrows, gravity}},
		{"skips is", rule("skips", "is", 2), []string{arrows}},
		{"skips is at least", rule("skips", "atLeast", 1), []string{arrows}},
		{"skips is at most", rule("skips", "atMost", 0), []string{joga, eoptian, guide, hotel, gravity}},
		{"last played in the last", rule("lastPlayed", "inLast", 7), []string{hotel}},
		{"last played not in the last, never played counts", rule("lastPlayed", "notInLast", 30), []string{joga, eoptian, arrows, gravity}},
		{"added in the last", rule("added", "inLast", 7), []string{guide, hotel, gravity}},
		{"added not in the last", rule("added", "notInLast", 7), []string{joga, eoptian, arrows}},
		{"length is at least", rule("length", "atLeast", 5), []string{joga, gravity}},
		{"length is at most", rule("length", "atMost", 4), []string{hotel}},
		{"format is, any case", rule("format", "is", "mp3"), []string{eoptian, arrows}},
		{"format is not", rule("format", "isNot", "MP3"), []string{joga, guide, hotel, gravity}},
		{"bitrate is", rule("bitrate", "is", 320), []string{eoptian}},
		{"bitrate is at least", rule("bitrate", "atLeast", 1000), []string{joga, hotel, gravity}},
		{"bitrate is at most", rule("bitrate", "atMost", 320), []string{eoptian, arrows}},
		{"an empty text rule counts for nothing", rule("title", "is", ""), []string{joga, eoptian, arrows, guide, hotel, gravity}},
		{"no rules choose everything", `{"match":"any","rules":[]}`, []string{joga, eoptian, arrows, guide, hotel, gravity}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := l.chosen(t, tc.in, 0); !slices.Equal(got, tc.want) {
				t.Errorf("chosen = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSmartRulesJoinByAllAnyAndOneGroup(t *testing.T) {
	t.Parallel()
	l := newSmartLib(t).marked(t)
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			"all", `{"match":"all","rules":[{"field":"albumArtist","op":"is","value":"Falling Up"},{"field":"format","op":"is","value":"FLAC"}]}`,
			[]string{guide, hotel, gravity},
		},
		{
			"any", `{"match":"any","rules":[{"field":"format","op":"is","value":"MP3"},{"field":"rating","op":"is","value":10}]}`,
			[]string{joga, eoptian, arrows},
		},
		// The board's own: Falling Up, 3★ and up, not played in a week, and
		// either from 2011 or before or played 10+ times.
		{
			"all with an any group", `{"match":"all","rules":[{"field":"albumArtist","op":"is","value":"Falling Up"},` +
				`{"field":"rating","op":"atLeast","value":6},{"field":"lastPlayed","op":"notInLast","value":7},` +
				`{"match":"any","rules":[{"field":"year","op":"atMost","value":2011},{"field":"plays","op":"atLeast","value":10}]}]}`,
			[]string{guide},
		},
		{
			"any with an all group", `{"match":"any","rules":[{"field":"title","op":"is","value":"Jóga"},` +
				`{"match":"all","rules":[{"field":"album","op":"is","value":"Captiva"},{"field":"length","op":"atLeast","value":5}]}]}`,
			[]string{joga, gravity},
		},
		{
			"a group of empty rules counts for nothing", `{"match":"all","rules":[{"field":"format","op":"is","value":"MP3"},` +
				`{"match":"any","rules":[{"field":"title","op":"is","value":""}]}]}`,
			[]string{eoptian, arrows},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := l.chosen(t, tc.in, 0); !slices.Equal(got, tc.want) {
				t.Errorf("chosen = %q, want %q", got, tc.want)
			}
		})
	}
}

func limited(count int, unit, by, order string) string {
	return fmt.Sprintf(`{"match":"all","rules":[],"limit":{"count":%d,"unit":%q,"by":%q},"order":%q}`, count, unit, by, order)
}

// Each limit unit stops before the song that would go over; each chooser
// picks in its own order, ties in the Library's.
func TestSmartLimitsChooseAndStop(t *testing.T) {
	t.Parallel()
	l := newSmartLib(t).marked(t)
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"most played", limited(2, "songs", "mostPlayed", "chosen"), []string{hotel, guide}},
		{"least played, ties in Library order", limited(3, "songs", "leastPlayed", "chosen"), []string{eoptian, arrows, gravity}},
		{"highest rated", limited(2, "songs", "highestRated", "chosen"), []string{joga, hotel}},
		{"most recently added", limited(2, "songs", "newest", "chosen"), []string{guide, hotel}},
		{"least recently played, never first", limited(4, "songs", "leastRecent", "chosen"), []string{eoptian, arrows, gravity, joga}},
		{"then in the Library's order", limited(2, "songs", "highestRated", "library"), []string{joga, hotel}},
		// Jóga runs 5:30, Eoptian 4:30, Golden Arrows 4:30: 10 minutes keeps two.
		{"minutes stop before going over", limited(10, "minutes", "leastRecent", "chosen"), []string{eoptian, arrows}},
		{"minutes keep exactly what fits", limited(9, "minutes", "leastRecent", "chosen"), []string{eoptian, arrows}},
		// Captiva's songs are 0.5 to 0.7 GB: one GB keeps Guide (0.6), not Hotel too.
		{"GB stop before going over", limited(1, "gb", "newest", "chosen"), []string{guide}},
		{"a limit past what matches keeps all", limited(50, "songs", "mostPlayed", "library"), []string{joga, eoptian, arrows, guide, hotel, gravity}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := l.chosen(t, tc.in, 0); !slices.Equal(got, tc.want) {
				t.Errorf("chosen = %q, want %q", got, tc.want)
			}
		})
	}
	rules := parseRules(t, limited(2, "songs", "mostPlayed", "chosen"))
	p, err := l.s.PreviewRules(t.Context(), l.user, &rules, "", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Songs != 2 || p.Matched != 6 || p.Albums != 1 || len(p.Mosaic) != 1 {
		t.Errorf("preview = %d songs of %d matched, %d albums, %d in the mosaic; want 2 of 6, 1, 1", p.Songs, p.Matched, p.Albums, len(p.Mosaic))
	}
	if p.Duration != 8*time.Minute { // Hotel Aquarium's 3:30 and the Guide's 4:30.
		t.Errorf("duration = %v", p.Duration)
	}
}

func TestSmartOrders(t *testing.T) {
	t.Parallel()
	l := newSmartLib(t).marked(t)
	all := func(order string) string { return `{"match":"all","rules":[],"order":"` + order + `"}` }
	cases := []struct {
		order string
		want  []string
	}{
		{"library", []string{joga, eoptian, arrows, guide, hotel, gravity}},
		{"title", []string{eoptian, guide, arrows, gravity, hotel, joga}},
		{"album", []string{joga, guide, hotel, gravity, eoptian, arrows}},
		{"year", []string{guide, hotel, gravity, eoptian, arrows, joga}},
		{"rating", []string{joga, hotel, guide, arrows, eoptian, gravity}},
		{"plays", []string{hotel, guide, joga, eoptian, arrows, gravity}},
		{"lastPlayed", []string{eoptian, arrows, gravity, joga, guide, hotel}},
		{"added", []string{guide, hotel, gravity, eoptian, arrows, joga}},
	}
	for _, tc := range cases {
		t.Run(tc.order, func(t *testing.T) {
			t.Parallel()
			if got := l.chosen(t, all(tc.order), 0); !slices.Equal(got, tc.want) {
				t.Errorf("chosen = %q, want %q", got, tc.want)
			}
		})
	}
}

// Random choices follow the stored seed: the page reads the same each time,
// until Pick again draws another.
func TestSmartSeedHoldsUntilPickAgain(t *testing.T) {
	t.Parallel()
	l := newSmartLib(t)
	rules := parseRules(t, `{"match":"all","rules":[],"order":"random"}`)
	const id = "01SMART0000000000000000001"
	if err := l.s.CreateSmartPlaylist(t.Context(), l.user, id, "Shuffled", &rules, 1); err != nil {
		t.Fatal(err)
	}
	read := func() []string {
		d, err := l.s.Playlist(t.Context(), l.user, id, nil)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(d.Entries))
		for i := range d.Entries {
			out[i] = d.Entries[i].Track.Title
		}
		return out
	}
	first := read()
	if again := read(); !slices.Equal(first, again) {
		t.Fatalf("the same seed gave %q, then %q", first, again)
	}
	if !slices.Equal(first, l.chosen(t, `{"match":"all","rules":[],"order":"random"}`, 1)) {
		t.Error("the preview with the seed differs from the playlist")
	}
	if err := l.s.SetPlaylistSeed(t.Context(), l.user, id, 2); err != nil {
		t.Fatal(err)
	}
	if picked := read(); slices.Equal(first, picked) || len(picked) != len(first) {
		t.Errorf("Pick again kept %q", picked)
	}
}

// A trashed song is not live: no rule chooses it, and it comes back with
// Restore.
func TestSmartNeverChoosesATrashedSong(t *testing.T) {
	t.Parallel()
	l := newSmartLib(t)
	rows, err := l.s.TrashableTracks(t.Context(), []string{l.ids[l.song(t, hotel)]})
	if err != nil {
		t.Fatal(err)
	}
	if err = l.s.MarkTrashed(t.Context(), []store.Trashing{{ID: rows[0].ID, TrashPath: "x"}}, l.user, l.clock.Now()); err != nil {
		t.Fatal(err)
	}
	if got := l.chosen(t, rule("album", "is", "Captiva"), 0); !slices.Equal(got, []string{guide, gravity}) {
		t.Errorf("chosen = %q, want the trashed song left out", got)
	}
	if err = l.s.UnmarkTrashed(t.Context(), []int64{rows[0].ID}); err != nil {
		t.Fatal(err)
	}
	if got := l.chosen(t, rule("album", "is", "Captiva"), 0); !slices.Equal(got, []string{guide, hotel, gravity}) {
		t.Errorf("chosen = %q after Restore", got)
	}
}

// smartID returns the nth test smart playlist's ULID.
func smartID(n int) string { return fmt.Sprintf("01SMART%019d", n) }

func inPlaylist(id string) string {
	return `{"match":"all","rules":[{"field":"playlist","op":"is","value":"` + id + `"}]}`
}

// smart makes the person's nth test smart playlist, named n, with the rules in.
func (l *smartLib) smart(t *testing.T, n int, in string) {
	t.Helper()
	rules := parseRules(t, in)
	if err := l.s.CreateSmartPlaylist(t.Context(), l.user, smartID(n), strconv.Itoa(n), &rules, 0); err != nil {
		t.Fatalf("smart %d: %v", n, err)
	}
}

// A playlist rule reads a regular playlist through its entries, and a deleted
// one matches nothing, whichever way the rule goes.
func TestSmartPlaylistRulesReadRegularPlaylists(t *testing.T) {
	t.Parallel()
	l := newSmartLib(t).marked(t)
	regular := l.make(t, "Two", l.song(t, joga), l.song(t, hotel))
	if got := l.chosen(t, inPlaylist(regular), 0); !slices.Equal(got, []string{joga, hotel}) {
		t.Errorf("in a regular playlist = %q", got)
	}
	notIn := `{"match":"all","rules":[{"field":"playlist","op":"isNot","value":"` + regular + `"}]}`
	if got := l.chosen(t, notIn, 0); !slices.Equal(got, []string{eoptian, arrows, guide, gravity}) {
		t.Errorf("not in a regular playlist = %q", got)
	}
	if err := l.s.DeletePlaylist(t.Context(), l.user, regular); err != nil {
		t.Fatal(err)
	}
	if got := l.chosen(t, inPlaylist(regular), 0); len(got) != 0 {
		t.Errorf("in a deleted playlist = %q", got)
	}
	if got := l.chosen(t, notIn, 0); len(got) != 0 {
		t.Errorf("not in a deleted playlist = %q", got)
	}
}

// nestedLib returns the marked library with a regular playlist of Jóga and
// Hotel Aquarium, and four smart playlists, each in the one before: the first
// is the regular one's songs of 4★ and up.
func nestedLib(t *testing.T) *smartLib {
	t.Helper()
	l := newSmartLib(t).marked(t)
	regular := l.make(t, "Two", l.song(t, joga), l.song(t, hotel))
	l.smart(t, 1, `{"match":"all","rules":[{"field":"rating","op":"atLeast","value":8},{"field":"playlist","op":"is","value":"`+regular+`"}]}`)
	for n := 2; n <= 4; n++ {
		l.smart(t, n, inPlaylist(smartID(n-1)))
	}
	return l
}

// Smart playlists are read by evaluating them, three deep, and a rule that
// would reach further matches nothing and isn't saved.
func TestSmartPlaylistRulesReachThreeDeep(t *testing.T) {
	t.Parallel()
	l := nestedLib(t)
	for n := 1; n <= 3; n++ {
		if got := l.chosen(t, inPlaylist(smartID(n)), 0); !slices.Equal(got, []string{joga, hotel}) {
			t.Errorf("through %d smart playlists = %q", n, got)
		}
	}
	// The fourth reaches three below it, the most there may be; a rule naming it
	// would be a fifth level, which matches nothing.
	if d, err := l.s.Playlist(t.Context(), l.user, smartID(4), nil); err != nil || len(d.Entries) != 2 {
		t.Errorf("four deep = %d entries, %v", len(d.Entries), err)
	}
	if got := l.chosen(t, inPlaylist(smartID(4)), 0); len(got) != 0 {
		t.Errorf("a fifth level = %q, want nothing", got)
	}
	// A fifth level reaches past the depth: saving it is refused.
	rules := parseRules(t, inPlaylist(smartID(4)))
	if err := l.s.CreateSmartPlaylist(t.Context(), l.user, smartID(5), "5", &rules, 0); !errors.Is(err, store.ErrSmartTooDeep) {
		t.Errorf("five deep = %v, want ErrSmartTooDeep", err)
	}
}

// Rules that name their way back are a cycle, which names the playlists in it.
func TestSmartPlaylistRulesRefuseACycle(t *testing.T) {
	t.Parallel()
	l := nestedLib(t)
	back := parseRules(t, inPlaylist(smartID(3)))
	err := l.s.SetPlaylistRules(t.Context(), l.user, smartID(2), &back)
	cycle, ok := errors.AsType[*store.CycleError](err)
	if !ok || !slices.Equal(cycle.Playlists, []string{smartID(2), smartID(3), smartID(2)}) {
		t.Errorf("a cycle = %v", err)
	}
	self := parseRules(t, inPlaylist(smartID(1)))
	if err = l.s.SetPlaylistRules(t.Context(), l.user, smartID(1), &self); !errors.As(err, &cycle) {
		t.Errorf("naming itself = %v, want a cycle", err)
	}
}

// Another person's playlist is one the rules can't see.
func TestSmartPlaylistRulesSeeOnlyTheirOwn(t *testing.T) {
	t.Parallel()
	l := newSmartLib(t).marked(t)
	theirs := fmt.Sprintf("01THEIRS%018d", 1)
	if _, err := l.s.CreatePlaylist(t.Context(), l.other, theirs, "Theirs", &store.PlaylistAdd{Tracks: []string{l.ids[0]}}); err != nil {
		t.Fatal(err)
	}
	if got := l.chosen(t, inPlaylist(theirs), 0); len(got) != 0 {
		t.Errorf("in another person's playlist = %q", got)
	}
}

// ratedRules are the rules of ratedLib's smart playlist: 3★ and up, most
// played first.
const ratedRules = `{"match":"all","rules":[{"field":"rating","op":"atLeast","value":6}],"order":"plays"}`

// ratedLib returns the marked library with one smart playlist, "Rated", whose
// ULID is smartID(1), of ratedRules: Hotel Aquarium, A Guide to Marine Life
// and Jóga.
func ratedLib(t *testing.T) *smartLib {
	t.Helper()
	l := newSmartLib(t).marked(t)
	rules := parseRules(t, ratedRules)
	if err := l.s.CreateSmartPlaylist(t.Context(), l.user, smartID(1), "Rated", &rules, 0); err != nil {
		t.Fatal(err)
	}
	return l
}

// A smart playlist lists and counts as a regular one does, and asking what
// playlists hold leaves it out: nothing is added to one.
func TestSmartPlaylistLists(t *testing.T) {
	t.Parallel()
	l := ratedLib(t)
	list, err := l.s.Playlists(t.Context(), l.user, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Rules == nil || list[0].Songs != 3 || list[0].Albums != 2 ||
		len(list[0].Mosaic) != 2 || list[0].Mosaic[0].Title != "Captiva" {
		t.Fatalf("list = %+v", list)
	}
	if holding, _ := l.s.Playlists(t.Context(), l.user, []string{}, nil); len(holding) != 0 {
		t.Errorf("holding lists %d playlists, want none", len(holding))
	}
}

// A smart playlist reads as a regular one does, its entry IDs its places.
func TestSmartPlaylistReads(t *testing.T) {
	t.Parallel()
	l := ratedLib(t)
	d, err := l.s.Playlist(t.Context(), l.user, smartID(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := d.Entries
	if len(entries) != 3 {
		t.Fatalf("entries = %+v", entries)
	}
	if first := entries[0]; first.ID != 1 || entries[2].ID != 3 || first.Track.Title != hotel {
		t.Errorf("entries = %+v, want places 1 to 3 from Hotel Aquarium", entries)
	}
	if first := entries[0]; first.Track.Extra.Plays != 8 || first.Track.Rating != 8 || first.Album != albumID(1) {
		t.Errorf("first entry = %+v", first)
	}
}

// A smart playlist has no entries to change, a regular one has no rules or
// seed, and another person's is not found.
func TestSmartPlaylistRefusesEntries(t *testing.T) {
	t.Parallel()
	l := ratedLib(t)
	ctx, id := t.Context(), smartID(1)
	if _, err := l.s.Playlist(ctx, l.other, id, nil); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("another person's = %v", err)
	}
	if _, err := l.s.AddToPlaylist(ctx, l.user, id, &store.PlaylistAdd{Tracks: []string{l.ids[0]}}); !errors.Is(err, store.ErrSmartPlaylist) {
		t.Errorf("add = %v, want ErrSmartPlaylist", err)
	}
	if _, err := l.s.RemoveFromPlaylist(ctx, l.user, id, []int64{1}); !errors.Is(err, store.ErrSmartPlaylist) {
		t.Errorf("remove = %v", err)
	}
	if err := l.s.MovePlaylistEntry(ctx, l.user, id, 1, 2, false); !errors.Is(err, store.ErrSmartPlaylist) {
		t.Errorf("move = %v", err)
	}
	regular := l.make(t, "Plain", 0)
	rules := parseRules(t, ratedRules)
	if err := l.s.SetPlaylistRules(ctx, l.user, regular, &rules); !errors.Is(err, store.ErrNotSmart) {
		t.Errorf("rules on a regular playlist = %v", err)
	}
	if err := l.s.SetPlaylistSeed(ctx, l.user, regular, 3); !errors.Is(err, store.ErrNotSmart) {
		t.Errorf("a seed on a regular playlist = %v", err)
	}
	if rows, err := l.s.PlaylistTrackRows(ctx, l.user, id, nil); err != nil || len(rows) != 3 {
		t.Errorf("shuffle's rows = %v, %v", rows, err)
	}
}

// A smart playlist copies to a regular one, which keeps what it holds when the
// stars move.
func TestSmartPlaylistCopies(t *testing.T) {
	t.Parallel()
	l := ratedLib(t)
	ctx, id := t.Context(), smartID(1)
	const copied = "01COPY00000000000000000001"
	if err := l.s.CopyPlaylist(ctx, l.user, id, copied, nil); err != nil {
		t.Fatal(err)
	}
	c, err := l.s.Playlist(ctx, l.user, copied, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Rated (copy)" || c.Rules != nil || len(c.Entries) != 3 || c.Entries[0].Track.Title != hotel {
		t.Fatalf("copy = %q, %d entries", c.Name, len(c.Entries))
	}
	l.rate(t, joga, 2)
	if c, _ = l.s.Playlist(ctx, l.user, copied, nil); len(c.Entries) != 3 {
		t.Errorf("the copy changed with the rules: %d entries", len(c.Entries))
	}
	if d, _ := l.s.Playlist(ctx, l.user, id, nil); len(d.Entries) != 2 {
		t.Errorf("the smart playlist didn't follow the stars: %d entries", len(d.Entries))
	}
}

// A regular playlist copies with its repeats, and a long name is cut to fit.
func TestCopyRegularPlaylist(t *testing.T) {
	t.Parallel()
	l := newSmartLib(t)
	long := strings.Repeat("é", store.MostPlaylistName)
	id := l.make(t, long, 0, 3, 0)
	const copied = "01COPY00000000000000000002"
	if err := l.s.CopyPlaylist(t.Context(), l.user, id, copied, nil); err != nil {
		t.Fatal(err)
	}
	c, err := l.s.Playlist(t.Context(), l.user, copied, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(c.Name)); n != store.MostPlaylistName || !strings.HasSuffix(c.Name, " (copy)") {
		t.Errorf("name is %d characters: %q", n, c.Name)
	}
	if got := len(c.Entries); got != 3 || c.Entries[2].Track.Title != joga {
		t.Errorf("copy holds %d entries", got)
	}
	if err = l.s.CopyPlaylist(t.Context(), l.other, id, "01COPY00000000000000000003", nil); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("copying another person's = %v", err)
	}
}
