package smartlist_test

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/smartlist"
)

// fields returns every field there is.
func fields() []smartlist.Field {
	return []smartlist.Field{
		smartlist.Title, smartlist.Album, smartlist.Artist, smartlist.AlbumArtist, smartlist.Genre,
		smartlist.Year, smartlist.Rating, smartlist.Plays, smartlist.Skips, smartlist.LastPlayed,
		smartlist.Added, smartlist.Length, smartlist.Format, smartlist.Bitrate, smartlist.Playlist,
	}
}

// ops returns every operator there is.
func ops() []smartlist.Op {
	return []smartlist.Op{
		smartlist.Is, smartlist.IsNot, smartlist.Contains, smartlist.NotContains,
		smartlist.AtLeast, smartlist.AtMost, smartlist.InLast, smartlist.NotInLast,
	}
}

// playlistID is a playlist's ID as a rule names one.
const playlistID = "01PLAYLIST0000000000000001"

// valueFor returns a value the field takes.
func valueFor(t *testing.T, f smartlist.Field) string {
	t.Helper()
	kind, ok := smartlist.KindOf(f)
	if !ok {
		t.Fatalf("no field %s", f)
	}
	switch {
	case f == smartlist.Playlist:
		return `"` + playlistID + `"`
	case kind == smartlist.KindText || kind == smartlist.KindPick:
		return `"Falling Up"`
	}
	return "5"
}

func one(field smartlist.Field, op smartlist.Op, value string) string {
	return fmt.Sprintf(`{"match":"all","rules":[{"field":%q,"op":%q,"value":%s}]}`, field, op, value)
}

// Every field takes exactly the operators the table gives it.
func TestParseTakesEachFieldsOperatorsAndNoOthers(t *testing.T) {
	t.Parallel()
	want := map[smartlist.Field][]smartlist.Op{
		smartlist.Title:       {smartlist.Is, smartlist.IsNot, smartlist.Contains, smartlist.NotContains},
		smartlist.Album:       {smartlist.Is, smartlist.IsNot, smartlist.Contains, smartlist.NotContains},
		smartlist.Artist:      {smartlist.Is, smartlist.IsNot, smartlist.Contains, smartlist.NotContains},
		smartlist.AlbumArtist: {smartlist.Is, smartlist.IsNot, smartlist.Contains, smartlist.NotContains},
		smartlist.Genre:       {smartlist.Is, smartlist.IsNot, smartlist.Contains, smartlist.NotContains},
		smartlist.Year:        {smartlist.Is, smartlist.AtLeast, smartlist.AtMost},
		smartlist.Rating:      {smartlist.Is, smartlist.AtLeast, smartlist.AtMost},
		smartlist.Plays:       {smartlist.Is, smartlist.AtLeast, smartlist.AtMost},
		smartlist.Skips:       {smartlist.Is, smartlist.AtLeast, smartlist.AtMost},
		smartlist.LastPlayed:  {smartlist.InLast, smartlist.NotInLast},
		smartlist.Added:       {smartlist.InLast, smartlist.NotInLast},
		smartlist.Length:      {smartlist.AtLeast, smartlist.AtMost},
		smartlist.Format:      {smartlist.Is, smartlist.IsNot},
		smartlist.Bitrate:     {smartlist.Is, smartlist.AtLeast, smartlist.AtMost},
		smartlist.Playlist:    {smartlist.Is, smartlist.IsNot},
	}
	for _, f := range fields() {
		for _, op := range ops() {
			t.Run(fmt.Sprintf("%s %s", f, op), func(t *testing.T) {
				t.Parallel()
				rules, err := smartlist.Parse([]byte(one(f, op, valueFor(t, f))))
				takes := slices.Contains(want[f], op)
				if takes != (err == nil) {
					t.Fatalf("Parse = %v, want it to take it: %v", err, takes)
				}
				if err != nil && !errors.Is(err, smartlist.ErrBad) {
					t.Fatalf("Parse = %v, want ErrBad", err)
				}
				if takes {
					got := rules.Conditions()[0].Rule
					if got.Field != f || got.Op != op {
						t.Fatalf("rule = %+v", got)
					}
				}
			})
		}
		if !slices.Equal(smartlist.Ops(f), want[f]) {
			t.Errorf("Ops(%s) = %v, want %v", f, smartlist.Ops(f), want[f])
		}
	}
}

// Numbers stay in each field's range, ends included.
func TestParseHoldsNumbersInRange(t *testing.T) {
	t.Parallel()
	ranges := []struct {
		field    smartlist.Field
		op       smartlist.Op
		min, max int64
	}{
		{smartlist.Year, smartlist.Is, 0, 9999},
		{smartlist.Rating, smartlist.AtLeast, 1, 10},
		{smartlist.Plays, smartlist.AtLeast, 0, 1_000_000},
		{smartlist.Skips, smartlist.AtMost, 0, 1_000_000},
		{smartlist.LastPlayed, smartlist.InLast, 1, 36_500},
		{smartlist.Added, smartlist.NotInLast, 1, 36_500},
		{smartlist.Length, smartlist.AtLeast, 0, 10_000},
		{smartlist.Bitrate, smartlist.AtMost, 0, 100_000},
	}
	for _, r := range ranges {
		for _, tc := range []struct {
			n  int64
			ok bool
		}{{r.min, true}, {r.max, true}, {r.min - 1, false}, {r.max + 1, false}} {
			t.Run(fmt.Sprintf("%s %d", r.field, tc.n), func(t *testing.T) {
				t.Parallel()
				rules, err := smartlist.Parse([]byte(one(r.field, r.op, strconv.FormatInt(tc.n, 10))))
				if (err == nil) != tc.ok {
					t.Fatalf("Parse = %v, want ok %v", err, tc.ok)
				}
				if tc.ok && rules.Conditions()[0].Rule.Number != tc.n {
					t.Fatalf("number = %d", rules.Conditions()[0].Rule.Number)
				}
			})
		}
	}
}

func TestParseRefuses(t *testing.T) {
	t.Parallel()
	rule := `{"field":"rating","op":"atLeast","value":8}`
	many := strings.TrimSuffix(strings.Repeat(rule+",", smartlist.MostRules+1), ",")
	cases := map[string]string{
		"not JSON":                    `{`,
		"no match":                    `{"rules":[]}`,
		"a bad match":                 `{"match":"most","rules":[]}`,
		"no rules list":               `{"match":"all"}`,
		"an unknown member":           `{"match":"all","rules":[],"extra":1}`,
		"an unknown rule member":      `{"match":"all","rules":[{"field":"rating","op":"atLeast","value":8,"x":1}]}`,
		"no such field":               one("mood", smartlist.Is, `"happy"`),
		"no value":                    `{"match":"all","rules":[{"field":"rating","op":"atLeast"}]}`,
		"text for a number":           one(smartlist.Year, smartlist.Is, `"2011"`),
		"a number for text":           one(smartlist.Title, smartlist.Is, `5`),
		"a fraction":                  one(smartlist.Rating, smartlist.AtLeast, `7.5`),
		"text too long":               one(smartlist.Title, smartlist.Contains, `"`+strings.Repeat("é", smartlist.MostText+1)+`"`),
		"an empty format":             one(smartlist.Format, smartlist.Is, `""`),
		"a playlist that isn't an ID": one(smartlist.Playlist, smartlist.Is, `"Falling Up"`),
		"a group in a group":          `{"match":"all","rules":[{"match":"any","rules":[{"match":"all","rules":[` + rule + `]}]}]}`,
		"an empty group":              `{"match":"all","rules":[{"match":"any","rules":[]}]}`,
		"a group with a bad match":    `{"match":"all","rules":[{"match":"either","rules":[` + rule + `]}]}`,
		"a rule and a group":          `{"match":"all","rules":[{"op":"is","match":"any","rules":[` + rule + `]}]}`,
		"too many rules":              `{"match":"all","rules":[` + many + `]}`,
		"too many in groups":          `{"match":"all","rules":[{"match":"any","rules":[` + strings.TrimSuffix(strings.Repeat(rule+",", 30), ",") + `]},{"match":"any","rules":[` + strings.TrimSuffix(strings.Repeat(rule+",", 21), ",") + `]}]}`,
		"a bad unit":                  `{"match":"all","rules":[],"limit":{"count":5,"unit":"hours","by":"random"}}`,
		"a limit of none":             `{"match":"all","rules":[],"limit":{"count":0,"unit":"songs","by":"random"}}`,
		"too many songs":              `{"match":"all","rules":[],"limit":{"count":10001,"unit":"songs","by":"random"}}`,
		"too many minutes":            `{"match":"all","rules":[],"limit":{"count":100001,"unit":"minutes","by":"random"}}`,
		"too many GB":                 `{"match":"all","rules":[],"limit":{"count":10001,"unit":"gb","by":"random"}}`,
		"no such chooser":             `{"match":"all","rules":[],"limit":{"count":5,"unit":"songs","by":"loudest"}}`,
		"no such order":               `{"match":"all","rules":[],"order":"loudness"}`,
		"chosen with no limit":        `{"match":"all","rules":[],"order":"chosen"}`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := smartlist.Parse([]byte(in)); !errors.Is(err, smartlist.ErrBad) {
				t.Fatalf("Parse = %v, want ErrBad", err)
			}
		})
	}
}

// The limits themselves are allowed: 50 rules, the largest limits, 200 characters.
func TestParseTakesTheLimits(t *testing.T) {
	t.Parallel()
	rule := `{"field":"rating","op":"atLeast","value":8}`
	cases := []string{
		`{"match":"all","rules":[` + strings.TrimSuffix(strings.Repeat(rule+",", smartlist.MostRules), ",") + `]}`,
		`{"match":"any","rules":[],"limit":{"count":10000,"unit":"songs","by":"random"}}`,
		`{"match":"any","rules":[],"limit":{"count":100000,"unit":"minutes","by":"mostPlayed"}}`,
		`{"match":"any","rules":[],"limit":{"count":10000,"unit":"gb","by":"leastRecent"},"order":"chosen"}`,
		one(smartlist.Title, smartlist.Is, `"`+strings.Repeat("é", smartlist.MostText)+`"`),
	}
	for _, in := range cases {
		if _, err := smartlist.Parse([]byte(in)); err != nil {
			t.Errorf("Parse(%.60s…) = %v", in, err)
		}
	}
}

// What Parse reads, MarshalJSON writes back, in the one form: an absent order
// is the Library's.
func TestRulesRoundTrip(t *testing.T) {
	t.Parallel()
	in := `{"match":"all","rules":[{"field":"albumArtist","op":"is","value":"Falling Up"},` +
		`{"field":"rating","op":"atLeast","value":6},{"field":"lastPlayed","op":"notInLast","value":7},` +
		`{"match":"any","rules":[{"field":"year","op":"atMost","value":2011},{"field":"plays","op":"atLeast","value":10}]}],` +
		`"limit":{"count":50,"unit":"songs","by":"leastRecent"},"order":"chosen"}`
	rules, err := smartlist.Parse([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	out, err := rules.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Fatalf("MarshalJSON =\n%s\nwant\n%s", out, in)
	}
	bare, err := smartlist.Parse([]byte(`{"match":"any","rules":[{"field":"title","op":"contains","value":"a \"b\" \\ c"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	out, _ = bare.MarshalJSON()
	if want := `{"match":"any","rules":[{"field":"title","op":"contains","value":"a \"b\" \\ c"}],"order":"library"}`; string(out) != want {
		t.Fatalf("MarshalJSON = %s, want %s", out, want)
	}
}

func TestRulesSay(t *testing.T) {
	t.Parallel()
	in := `{"match":"any","rules":[{"field":"title","op":"is","value":""},` +
		`{"field":"playlist","op":"is","value":"` + playlistID + `"},` +
		`{"match":"all","rules":[{"field":"skips","op":"atMost","value":2},{"field":"playlist","op":"isNot","value":"` + playlistID + `"}]},` +
		`{"match":"all","rules":[{"field":"album","op":"contains","value":""}]}],"order":"random"}`
	rules, err := smartlist.Parse([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	conds := rules.Conditions()
	_, limited := rules.Limit()
	for _, c := range []struct {
		what      string
		got, want bool
	}{
		{"an empty text rule counts", conds[0].Counts(), false},
		{"a playlist rule counts", conds[1].Counts(), true},
		{"a group holding a counting rule counts", conds[2].Counts(), true},
		{"a group of empty text rules counts", conds[3].Counts(), false},
		{"reads skips", rules.Reads(smartlist.Skips), true},
		{"reads an empty title", rules.Reads(smartlist.Title), false},
		{"reads a year", rules.Reads(smartlist.Year), false},
		{"shuffled once draws on the seed", rules.Random(), true},
		{"has a limit", limited, false},
	} {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.what, c.got, c.want)
		}
	}
	if got := rules.Playlists(); !slices.Equal(got, []string{playlistID}) {
		t.Errorf("Playlists = %v", got)
	}
	n := 0
	rules.Each(func(*smartlist.Rule) { n++ })
	if n != 5 {
		t.Errorf("Each saw %d rules, want 5", n)
	}
}
