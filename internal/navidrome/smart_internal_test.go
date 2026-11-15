package navidrome

import (
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/smartlist"
	"github.com/simonhull/kitsune/internal/store/navidromedb"
)

// kitsuneList is the Kitsune ID the fixture's playlist "pl1" takes.
const kitsuneList = "01KITSUNEPLAYLIST000000000"

// mapped maps raw for a person whose love line is love, with "pl1" coming over,
// and returns the rules as Kitsune writes them, or what stopped them.
func mapped(t *testing.T, raw string, love rating.Rating) mappedJSON {
	t.Helper()
	c := mapCriteria(raw, love, map[string]string{"pl1": kitsuneList})
	if c.stopped != nil {
		return mappedJSON{stopped: c.stopped}
	}
	b, err := c.rules.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return mappedJSON{rules: string(b)}
}

// mappedJSON is the rules criteria map to, as Kitsune writes them, or what
// stopped them.
type mappedJSON struct {
	rules   string
	stopped []string
}

// opStartsWith is an operator Kitsune has no rule for.
const opStartsWith = "startsWith"

// Every criterion that maps, one at a time.
func TestCriteriaThatMap(t *testing.T) {
	t.Parallel()
	all := func(rules string) string { return `{"match":"all","rules":[` + rules + `],"order":"library"}` }
	for _, tc := range []struct{ name, raw, want string }{
		{"title is", `{"all":[{"is":{"title":"Hydro"}}]}`, all(`{"field":"title","op":"is","value":"Hydro"}`)},
		{"album isNot", `{"all":[{"isNot":{"album":"Hours"}}]}`, all(`{"field":"album","op":"isNot","value":"Hours"}`)},
		{"artist contains", `{"all":[{"contains":{"artist":"Falling"}}]}`, all(`{"field":"artist","op":"contains","value":"Falling"}`)},
		{"album artist notContains", `{"all":[{"notContains":{"albumartist":"Various"}}]}`, all(`{"field":"albumArtist","op":"notContains","value":"Various"}`)},
		{"genre, any case", `{"all":[{"is":{"Genre":"Rock"}}]}`, all(`{"field":"genre","op":"is","value":"Rock"}`)},
		{"year is", `{"all":[{"is":{"year":2011}}]}`, all(`{"field":"year","op":"is","value":2011}`)},
		{"year gt", `{"all":[{"gt":{"year":2000}}]}`, all(`{"field":"year","op":"atLeast","value":2001}`)},
		{"year lt as text", `{"all":[{"lt":{"year":"2000"}}]}`, all(`{"field":"year","op":"atMost","value":1999}`)},
		{"plays gt", `{"all":[{"gt":{"playcount":9}}]}`, all(`{"field":"plays","op":"atLeast","value":10}`)},
		{"bitrate lt", `{"all":[{"lt":{"bitrate":320}}]}`, all(`{"field":"bitrate","op":"atMost","value":319}`)},
		{"rating is, in halves", `{"all":[{"is":{"rating":4}}]}`, all(`{"field":"rating","op":"is","value":8}`)},
		{"rating gt, half a star over", `{"all":[{"gt":{"rating":3}}]}`, all(`{"field":"rating","op":"atLeast","value":7}`)},
		{"rating lt, half a star under", `{"all":[{"lt":{"rating":3}}]}`, all(`{"field":"rating","op":"atMost","value":5}`)},
		{"loved", `{"all":[{"is":{"loved":true}}]}`, all(`{"field":"rating","op":"atLeast","value":8}`)},
		{"not loved", `{"all":[{"isNot":{"loved":true}}]}`, all(`{"field":"rating","op":"atMost","value":7}`)},
		{"last played in the last", `{"all":[{"inTheLast":{"lastplayed":30}}]}`, all(`{"field":"lastPlayed","op":"inLast","value":30}`)},
		{"added not in the last", `{"all":[{"notInTheLast":{"dateadded":7}}]}`, all(`{"field":"added","op":"notInLast","value":7}`)},
		{"duration gt, minutes up", `{"all":[{"gt":{"duration":301}}]}`, all(`{"field":"length","op":"atLeast","value":6}`)},
		{"duration lt, minutes down", `{"all":[{"lt":{"duration":359}}]}`, all(`{"field":"length","op":"atMost","value":5}`)},
		{"filetype", `{"all":[{"is":{"filetype":"flac"}}]}`, all(`{"field":"format","op":"is","value":"flac"}`)},
		{
			"in the range, in an all", `{"all":[{"inTheRange":{"year":[1980,1989]}}]}`,
			all(`{"field":"year","op":"atLeast","value":1980},{"field":"year","op":"atMost","value":1989}`),
		},
		{
			"in the range, in an any: a group", `{"any":[{"inTheRange":{"rating":[2,3]}}]}`,
			`{"match":"any","rules":[{"match":"all","rules":[{"field":"rating","op":"atLeast","value":4},{"field":"rating","op":"atMost","value":6}]}],"order":"library"}`,
		},
		{"in a playlist that comes over", `{"all":[{"inPlaylist":{"id":"pl1"}}]}`, all(`{"field":"playlist","op":"is","value":"` + kitsuneList + `"}`)},
		{"not in one", `{"all":[{"notInPlaylist":{"id":"pl1"}}]}`, all(`{"field":"playlist","op":"isNot","value":"` + kitsuneList + `"}`)},
		{
			"a nested group", `{"all":[{"is":{"loved":true}},{"any":[{"lt":{"year":2012}},{"gt":{"playcount":9}}]}]}`,
			all(`{"field":"rating","op":"atLeast","value":8},{"match":"any","rules":[{"field":"year","op":"atMost","value":2011},{"field":"plays","op":"atLeast","value":10}]}`),
		},
		{"a nested all in an all, flattened", `{"all":[{"all":[{"is":{"year":2011}}]}]}`, all(`{"field":"year","op":"is","value":2011}`)},
		{
			"a limit chosen by a sort that fits", `{"all":[{"gt":{"rating":0}}],"sort":"lastplayed","order":"asc","limit":50}`,
			`{"match":"all","rules":[{"field":"rating","op":"atLeast","value":1}],"limit":{"count":50,"unit":"songs","by":"leastRecent"},"order":"chosen"}`,
		},
		{"most played", `{"all":[],"sort":"-playcount","limit":10}`, `{"match":"all","rules":[],"limit":{"count":10,"unit":"songs","by":"mostPlayed"},"order":"chosen"}`},
		{"random", `{"any":[],"sort":"random","limit":25}`, `{"match":"any","rules":[],"limit":{"count":25,"unit":"songs","by":"random"},"order":"chosen"}`},
		{"a sort with no limit is the order", `{"all":[],"sort":"year"}`, `{"match":"all","rules":[],"order":"year"}`},
		{"newest first", `{"all":[],"sort":"dateadded","order":"desc"}`, `{"match":"all","rules":[],"order":"added"}`},
		{"no offset", `{"all":[],"offset":0}`, all(``)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := mapped(t, tc.raw, 0); got.rules != tc.want {
				t.Errorf("got %s (stopped by %v)\nwant %s", got.rules, got.stopped, tc.want)
			}
		})
	}
}

// Loved follows the person's own love line.
func TestLovedFollowsTheLoveLine(t *testing.T) {
	t.Parallel()
	got := mapped(t, `{"all":[{"is":{"loved":true}}]}`, 7).rules
	if want := `{"match":"all","rules":[{"field":"rating","op":"atLeast","value":7}],"order":"library"}`; got != want {
		t.Errorf("loved = %s, want at the 3½★ line: %s", got, want)
	}
	got = mapped(t, `{"all":[{"is":{"loved":false}}]}`, 3).rules
	if want := `{"match":"all","rules":[{"field":"rating","op":"atMost","value":2}],"order":"library"}`; got != want {
		t.Errorf("not loved = %s, want under the 1½★ line: %s", got, want)
	}
	if got := mapped(t, `{"all":[{"is":{"loved":false}}]}`, rating.Min); got.stopped == nil {
		t.Error("not loved under a ½★ line is unrated, which no rule says: it should stop")
	}
}

// Every criterion that maps to nothing stops the whole playlist, and says
// which it was.
func TestCriteriaThatDontMap(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, raw string
		want      []string
	}{
		{"starts with", `{"all":[{"startsWith":{"title":"The"}}]}`, []string{opStartsWith}},
		{"ends with", `{"all":[{"endsWith":{"title":"Mix"}}]}`, []string{"endsWith"}},
		{"before a date", `{"all":[{"before":{"lastplayed":"2020-01-01"}}]}`, []string{"before"}},
		{"after a date", `{"all":[{"after":{"dateadded":"2020-01-01"}}]}`, []string{"after"}},
		{"gt a date", `{"all":[{"gt":{"dateadded":"2020-01-01"}}]}`, []string{"gt added"}},
		{"a comment", `{"all":[{"contains":{"comment":"live"}}]}`, []string{"comment"}},
		{"lyrics", `{"all":[{"contains":{"lyrics":"love"}}]}`, []string{"lyrics"}},
		{"bpm", `{"all":[{"gt":{"bpm":120}}]}`, []string{"bpm"}},
		{"catalognumber", `{"all":[{"is":{"catalognumber":"X1"}}]}`, []string{"catalognumber"}},
		{
			"several, each once", `{"all":[{"startsWith":{"title":"A"}},{"is":{"comment":"x"}},{"startsWith":{"album":"B"}}]}`,
			[]string{opStartsWith, "comment"},
		},
		{"too deep", `{"all":[{"any":[{"all":[{"is":{"year":1}},{"is":{"year":2}}]}]}]}`, []string{whyNesting}},
		{"a range in an any group", `{"all":[{"any":[{"inTheRange":{"year":[1,2]}}]}]}`, []string{whyNesting}},
		{"a playlist not coming over", `{"all":[{"inPlaylist":{"id":"theirs"}}]}`, []string{"inPlaylist"}},
		{"a number that isn't one", `{"all":[{"is":{"year":"soon"}}]}`, []string{"is year"}},
		{"unrated", `{"all":[{"is":{"rating":0}}]}`, []string{"is rating"}},
		{"isNot a number", `{"all":[{"isNot":{"year":2000}}]}`, []string{"isNot year"}},
		{"a limit by title", `{"all":[],"sort":"title","limit":10}`, []string{"limit by title"}},
		{"a limit with no sort", `{"all":[],"limit":10}`, []string{"limit by title"}},
		{"a limit past Kitsune's", `{"all":[],"sort":"random","limit":20000}`, []string{whyLimit}},
		{"an order Kitsune runs the other way", `{"all":[],"sort":"year","order":"desc"}`, []string{"sort -year"}},
		{"several sorts", `{"all":[],"sort":"year,title"}`, []string{whySort}},
		{"an offset", `{"all":[],"offset":5}`, []string{ndOffset}},
		{"limitPercent", `{"all":[],"limitPercent":10}`, []string{"limitPercent"}},
		{"not JSON", `{"all":`, []string{whyNotRules}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := mapped(t, tc.raw, 0); got.rules != "" || !slices.Equal(got.stopped, tc.want) {
				t.Errorf("got %s, stopped by %q; want stopped by %q", got.rules, got.stopped, tc.want)
			}
		})
	}
}

// More rules than Kitsune keeps stop the playlist.
func TestTooManyRules(t *testing.T) {
	t.Parallel()
	raw := `{"all":[` + strings.Repeat(`{"is":{"year":2000}},`, smartlist.MostRules) + `{"is":{"year":2000}}]}`
	if got := mapped(t, raw, 0); !slices.Equal(got.stopped, []string{whyTooMany}) {
		t.Errorf("stopped by %q, want %q", got.stopped, whyTooMany)
	}
}

// Playlists that name each other round in a cycle stay behind, with every
// smart playlist that names one of them.
func TestSmartPlaylistsInACycle(t *testing.T) {
	t.Parallel()
	ids := map[string]string{"a": "01AAAAAAAAAAAAAAAAAAAAAAAA", "b": "01BBBBBBBBBBBBBBBBBBBBBBBB", "c": "01CCCCCCCCCCCCCCCCCCCCCCCC", "d": "01DDDDDDDDDDDDDDDDDDDDDDDD"}
	smart := []*navidromedb.Playlist{
		{ID: "a", Smart: true, Rules: `{"all":[{"inPlaylist":{"id":"b"}}]}`},
		{ID: "b", Smart: true, Rules: `{"all":[{"inPlaylist":{"id":"a"}}]}`},
		{ID: "c", Smart: true, Rules: `{"all":[{"notInPlaylist":{"id":"a"}}]}`},
		{ID: "d", Smart: true, Rules: `{"all":[{"inPlaylist":{"id":"c"}},{"startsWith":{"title":"x"}}]}`},
	}
	got := mapSmart(smart, 0, ids)
	mappedRules, left := got.rules, got.left
	if len(mappedRules) != 0 || !slices.Equal(left["a"], []string{whyCycle}) || !slices.Equal(left["b"], []string{whyCycle}) ||
		!slices.Equal(left["c"], []string{"notInPlaylist"}) || !slices.Equal(left["d"], []string{opStartsWith}) {
		t.Errorf("mapped %v, left %v; want a and b in a cycle, c naming a, d stopped by startsWith", mappedRules, left)
	}
}
