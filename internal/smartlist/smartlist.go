// Package smartlist says what a smart playlist's rules are: which fields a rule
// may read, which operators each field takes, what values they hold and how
// many there may be. It owns that table, and nothing else does.
//
// It is pure: it knows nothing of the store or of SQL. [Parse] is the only way to
// make a [Rules], the way the store's ParseSearch is the only way to make a
// search, so a value that reaches the store has passed every limit here.
package smartlist

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"unicode/utf8"
)

// Field is what a rule reads of a song.
type Field string

// The fields, in the order an editor offers them.
const (
	Title       Field = "title"
	Album       Field = "album"
	Artist      Field = "artist"      // Any of the song's artists.
	AlbumArtist Field = "albumArtist" // Any of the song's album artists.
	Genre       Field = "genre"       // The album's display genres, never the raw tags.
	Year        Field = "year"
	Rating      Field = "rating"     // The person's own stars, in halves.
	Plays       Field = "plays"      // The person's plays, imports included.
	Skips       Field = "skips"      // The person's listens that ended skipped.
	LastPlayed  Field = "lastPlayed" // The person's latest play, in days ago.
	Added       Field = "added"      // When the song came into the library, in days ago.
	Length      Field = "length"     // In minutes.
	Format      Field = "format"
	Bitrate     Field = "bitrate" // In kbps.
	Playlist    Field = "playlist"
)

// Kind is what sort of value a field holds.
type Kind uint8

// The kinds of value.
const (
	KindText    Kind = iota + 1 // A string, compared folded; empty counts for nothing.
	KindNumber                  // A whole number.
	KindStars                   // Half stars, 1 to 10.
	KindDays                    // A number of days back from now.
	KindMinutes                 // A whole number of minutes.
	KindPick                    // One of a list: a format, or a playlist's ID.
)

// Op is how a rule compares.
type Op string

// The operators.
const (
	Is          Op = "is"
	IsNot       Op = "isNot"
	Contains    Op = "contains"
	NotContains Op = "notContains"
	AtLeast     Op = "atLeast"
	AtMost      Op = "atMost"
	InLast      Op = "inLast"
	NotInLast   Op = "notInLast"
)

// field is one row of the table: what a field holds, the operators it takes and
// the range of a number.
type field struct {
	kind     Kind
	ops      []Op
	min, max int64 // For a numeric kind.
}

// Ranges of the numeric fields.
const (
	mostYear    = 9999
	mostCount   = 1_000_000 // Plays and skips.
	mostDays    = 36_500    // A hundred years.
	mostMinutes = 10_000
	mostBitrate = 100_000 // kbps.
	leastStars  = 1
	mostStars   = 10
	leastDays   = 1
	ulidLength  = 26
)

// The operator lists the table's rows share, one per sort of field.
func textOps() []Op   { return []Op{Is, IsNot, Contains, NotContains} }
func numberOps() []Op { return []Op{Is, AtLeast, AtMost} }
func daysOps() []Op   { return []Op{InLast, NotInLast} }
func lengthOps() []Op { return []Op{AtLeast, AtMost} }
func pickOps() []Op   { return []Op{Is, IsNot} }

// table is every field there is. It is read only.
var table = map[Field]field{ //nolint:gochecknoglobals // The table is fixed; nothing writes it.
	Title:       {kind: KindText, ops: textOps()},
	Album:       {kind: KindText, ops: textOps()},
	Artist:      {kind: KindText, ops: textOps()},
	AlbumArtist: {kind: KindText, ops: textOps()},
	Genre:       {kind: KindText, ops: textOps()},
	Year:        {kind: KindNumber, ops: numberOps(), max: mostYear},
	Rating:      {kind: KindStars, ops: numberOps(), min: leastStars, max: mostStars},
	Plays:       {kind: KindNumber, ops: numberOps(), max: mostCount},
	Skips:       {kind: KindNumber, ops: numberOps(), max: mostCount},
	LastPlayed:  {kind: KindDays, ops: daysOps(), min: leastDays, max: mostDays},
	Added:       {kind: KindDays, ops: daysOps(), min: leastDays, max: mostDays},
	Length:      {kind: KindMinutes, ops: lengthOps(), max: mostMinutes},
	Format:      {kind: KindPick, ops: pickOps()},
	Bitrate:     {kind: KindNumber, ops: numberOps(), max: mostBitrate},
	Playlist:    {kind: KindPick, ops: pickOps()},
}

// KindOf returns what a field holds, and false for a field there isn't.
func KindOf(f Field) (Kind, bool) {
	row, ok := table[f]
	return row.kind, ok
}

// Ops returns the operators f takes, in the order an editor offers them. The
// caller must not change the slice.
func Ops(f Field) []Op { return table[f].ops }

// Match is how a list of conditions combines.
type Match string

// The ways to combine.
const (
	All Match = "all"
	Any Match = "any"
)

// Unit is what a limit counts.
type Unit string

// The units of a limit.
const (
	Songs   Unit = "songs"
	Minutes Unit = "minutes"
	GB      Unit = "gb" // File sizes, in 10⁹ bytes.
)

// Chooser is which songs a limit keeps first.
type Chooser string

// The choosers.
const (
	Random       Chooser = "random"
	MostPlayed   Chooser = "mostPlayed"
	LeastPlayed  Chooser = "leastPlayed"
	HighestRated Chooser = "highestRated"
	Newest       Chooser = "newest"      // Most recently added.
	LeastRecent  Chooser = "leastRecent" // Least recently played; never played first.
)

// Order is the order a smart playlist lists its songs in. Ties always fall back
// to the Library's order.
type Order string

// The orders.
const (
	Chosen       Order = "chosen"  // The limit's: only with one.
	Library      Order = "library" // The Library's own; the default.
	ByTitle      Order = "title"
	ByAlbum      Order = "album"
	ByYear       Order = "year"       // Oldest first.
	ByRating     Order = "rating"     // Highest first.
	ByPlays      Order = "plays"      // Most first.
	ByLastPlayed Order = "lastPlayed" // Longest ago first; never played first.
	ByAdded      Order = "added"      // Newest first.
	Shuffled     Order = "random"     // Shuffled once, by the playlist's seed.
)

// The limits on rules.
const (
	// MostRules is the most rules a smart playlist has, those in groups included.
	MostRules = 50
	// MostText is the longest a text value may be, in characters.
	MostText = 200
	// MostSongs, MostMinutes and MostGB are the largest limits.
	MostSongs   = 10_000
	MostMinutes = 100_000
	MostGB      = 10_000
)

// ErrBad means rules aren't rules a smart playlist can have. Every error [Parse]
// returns wraps it, and says what is wrong without quoting a value.
var ErrBad = errors.New("bad smart playlist rules")

// Rule is one condition: a field, how it compares, and a value. A text or pick
// value is Text; every other is Number.
type Rule struct {
	Field  Field
	Op     Op
	Text   string
	Number int64
}

// Kind returns what the rule's field holds.
func (r *Rule) Kind() Kind { return table[r.Field].kind }

// Counts reports whether the rule takes part: a text rule with no value counts
// for nothing, as if it weren't there.
func (r *Rule) Counts() bool { return r.Kind() != KindText || r.Text != "" }

// Group is one level of conditions inside the top level, with its own match.
type Group struct {
	Match Match
	Rules []Rule
}

// Condition is one item of the top level: a rule, or a group of them.
type Condition struct {
	Rule  Rule
	Group *Group // Nil for a rule.
}

// Counts reports whether the condition takes part: a group counts while one of
// its rules does.
func (c *Condition) Counts() bool {
	if c.Group == nil {
		return c.Rule.Counts()
	}
	for i := range c.Group.Rules {
		if c.Group.Rules[i].Counts() {
			return true
		}
	}
	return false
}

// Limit keeps at most Count of Unit, chosen By.
type Limit struct {
	Count int64
	Unit  Unit
	By    Chooser
}

// Rules are a smart playlist's rules. The zero value is not valid: only
// [Parse] makes one.
type Rules struct {
	match      Match
	conditions []Condition
	limit      *Limit
	order      Order
}

// Match returns how the top level combines.
func (r Rules) Match() Match { return r.match }

// Conditions returns the top level, in order. The caller must not change it.
func (r Rules) Conditions() []Condition { return r.conditions }

// Limit returns the limit, and false when there is none.
func (r Rules) Limit() (Limit, bool) {
	if r.limit == nil {
		return Limit{}, false
	}
	return *r.limit, true
}

// Order returns the order the songs are listed in.
func (r Rules) Order() Order { return r.order }

// Each calls fn with every rule, those in groups included, in order.
func (r Rules) Each(fn func(*Rule)) {
	for i := range r.conditions {
		c := &r.conditions[i]
		if c.Group == nil {
			fn(&c.Rule)
			continue
		}
		for j := range c.Group.Rules {
			fn(&c.Group.Rules[j])
		}
	}
}

// Playlists returns the IDs of the playlists the rules name, each once, in the
// order they are first named.
func (r Rules) Playlists() []string {
	var out []string
	r.Each(func(rule *Rule) {
		if rule.Field == Playlist && !slices.Contains(out, rule.Text) {
			out = append(out, rule.Text)
		}
	})
	return out
}

// Reads reports whether a rule that counts reads the field f.
func (r Rules) Reads(f Field) bool {
	found := false
	r.Each(func(rule *Rule) { found = found || (rule.Field == f && rule.Counts()) })
	return found
}

// Random reports whether the rules draw on a seed: a limit chosen at random, or
// an order shuffled once.
func (r Rules) Random() bool {
	return r.order == Shuffled || (r.limit != nil && r.limit.By == Random)
}

// wireRules is the JSON form of rules, which the contract spells out as
// SmartRules.
type wireRules struct {
	Match string     `json:"match"`
	Rules []wireItem `json:"rules"`
	Limit *wireLimit `json:"limit,omitzero"`
	Order string     `json:"order,omitzero"`
}

type wireItem struct {
	Field string         `json:"field,omitzero"`
	Op    string         `json:"op,omitzero"`
	Value jsontext.Value `json:"value,omitzero"`
	Match string         `json:"match,omitzero"`
	Rules []wireItem     `json:"rules,omitzero"`
}

type wireLimit struct {
	Count int64  `json:"count"`
	Unit  string `json:"unit"`
	By    string `json:"by"`
}

// Parse reads rules from their JSON form and checks every limit: the fields and
// the operators each takes, the values and their ranges, at most [MostRules]
// rules, one level of groups, a limit within [MostSongs], [MostMinutes] or
// [MostGB], and an order of [Chosen] only with a limit. An absent order is
// [Library].
//
// # Errors
//
// An error wrapping [ErrBad] for anything else.
func Parse(data []byte) (Rules, error) {
	var in wireRules
	if err := json.Unmarshal(data, &in, json.RejectUnknownMembers(true)); err != nil {
		return Rules{}, fmt.Errorf("%w: not rules", ErrBad)
	}
	out := Rules{order: Library}
	var err error
	if out.match, err = parseMatch(in.Match); err != nil {
		return Rules{}, err
	}
	if in.Rules == nil {
		return Rules{}, fmt.Errorf("%w: no rules list", ErrBad)
	}
	total := 0
	for i := range in.Rules {
		c, n, condErr := parseCondition(&in.Rules[i], i+1)
		if condErr != nil {
			return Rules{}, condErr
		}
		total += n
		out.conditions = append(out.conditions, c)
	}
	if total > MostRules {
		return Rules{}, fmt.Errorf("%w: at most %d rules", ErrBad, MostRules)
	}
	if out.limit, err = parseLimit(in.Limit); err != nil {
		return Rules{}, err
	}
	if out.order, err = parseOrder(in.Order, out.limit != nil); err != nil {
		return Rules{}, err
	}
	return out, nil
}

func parseMatch(s string) (Match, error) {
	switch m := Match(s); m {
	case All, Any:
		return m, nil
	}
	return "", fmt.Errorf("%w: match is all or any", ErrBad)
}

// parseCondition reads the nth item of the top level, and says how many rules it
// holds.
func parseCondition(in *wireItem, n int) (Condition, int, error) {
	if in.Field != "" || in.Match == "" {
		rule, err := parseRule(in, strconv.Itoa(n))
		return Condition{Rule: rule}, 1, err
	}
	g, err := parseGroup(in, n)
	if err != nil {
		return Condition{}, 0, err
	}
	return Condition{Group: g}, len(g.Rules), nil
}

// parseGroup reads the group that is the nth item of the top level.
func parseGroup(in *wireItem, n int) (*Group, error) {
	if in.Op != "" || len(in.Value) > 0 {
		return nil, fmt.Errorf("%w: item %d is a rule or a group, not both", ErrBad, n)
	}
	match, err := parseMatch(in.Match)
	if err != nil {
		return nil, fmt.Errorf("group %d: %w", n, err)
	}
	if len(in.Rules) == 0 || len(in.Rules) > MostRules {
		return nil, fmt.Errorf("%w: group %d holds 1 to %d rules", ErrBad, n, MostRules)
	}
	g := &Group{Match: match, Rules: make([]Rule, len(in.Rules))}
	for i := range in.Rules {
		if in.Rules[i].Match != "" || in.Rules[i].Rules != nil {
			return nil, fmt.Errorf("%w: groups go one level deep", ErrBad)
		}
		if g.Rules[i], err = parseRule(&in.Rules[i], fmt.Sprintf("%d.%d", n, i+1)); err != nil {
			return nil, err
		}
	}
	return g, nil
}

// parseRule reads the rule at place, which an error names.
func parseRule(in *wireItem, place string) (Rule, error) {
	if in.Match != "" || in.Rules != nil {
		return Rule{}, fmt.Errorf("%w: rule %s is a rule or a group, not both", ErrBad, place)
	}
	f, ok := table[Field(in.Field)]
	if !ok {
		return Rule{}, fmt.Errorf("%w: rule %s: no such field", ErrBad, place)
	}
	rule := Rule{Field: Field(in.Field), Op: Op(in.Op)}
	if !slices.Contains(f.ops, rule.Op) {
		return Rule{}, fmt.Errorf("%w: rule %s: %s takes %v", ErrBad, place, rule.Field, f.ops)
	}
	if len(in.Value) == 0 {
		return Rule{}, fmt.Errorf("%w: rule %s has no value", ErrBad, place)
	}
	if err := parseValue(&rule, f, in.Value, place); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

// parseValue reads a rule's value into Text or Number, as its field's row f
// says, and checks it.
func parseValue(rule *Rule, f field, value jsontext.Value, place string) error {
	switch f.kind {
	case KindText, KindPick:
		if err := json.Unmarshal(value, &rule.Text); err != nil {
			return fmt.Errorf("%w: rule %s: %s takes text", ErrBad, place, rule.Field)
		}
		return checkText(rule, f.kind, place)
	case KindNumber, KindStars, KindDays, KindMinutes:
		if err := json.Unmarshal(value, &rule.Number); err != nil {
			return fmt.Errorf("%w: rule %s: %s takes a whole number", ErrBad, place, rule.Field)
		}
		if rule.Number < f.min || rule.Number > f.max {
			return fmt.Errorf("%w: rule %s: %s is %d to %d", ErrBad, place, rule.Field, f.min, f.max)
		}
	}
	return nil
}

// checkText checks a text or pick value.
func checkText(rule *Rule, kind Kind, place string) error {
	if !utf8.ValidString(rule.Text) || utf8.RuneCountInString(rule.Text) > MostText {
		return fmt.Errorf("%w: rule %s: text is at most %d characters", ErrBad, place, MostText)
	}
	switch {
	case kind == KindPick && rule.Text == "":
		return fmt.Errorf("%w: rule %s: %s needs a value", ErrBad, place, rule.Field)
	case rule.Field == Playlist && len(rule.Text) != ulidLength:
		return fmt.Errorf("%w: rule %s: not a playlist's ID", ErrBad, place)
	}
	return nil
}

func parseLimit(in *wireLimit) (*Limit, error) {
	if in == nil {
		return nil, nil //nolint:nilnil // No limit is no error.
	}
	l := &Limit{Count: in.Count, Unit: Unit(in.Unit), By: Chooser(in.By)}
	var most int64
	switch l.Unit {
	case Songs:
		most = MostSongs
	case Minutes:
		most = MostMinutes
	case GB:
		most = MostGB
	default:
		return nil, fmt.Errorf("%w: a limit counts songs, minutes or gb", ErrBad)
	}
	if l.Count < 1 || l.Count > most {
		return nil, fmt.Errorf("%w: a limit of %s is 1 to %d", ErrBad, l.Unit, most)
	}
	switch l.By {
	case Random, MostPlayed, LeastPlayed, HighestRated, Newest, LeastRecent:
		return l, nil
	}
	return nil, fmt.Errorf("%w: no such way to choose", ErrBad)
}

func parseOrder(s string, limited bool) (Order, error) {
	switch o := Order(s); o {
	case "":
		return Library, nil
	case Chosen:
		if !limited {
			return "", fmt.Errorf("%w: the order the limit chose needs a limit", ErrBad)
		}
		return o, nil
	case Library, ByTitle, ByAlbum, ByYear, ByRating, ByPlays, ByLastPlayed, ByAdded, Shuffled:
		return o, nil
	}
	return "", fmt.Errorf("%w: no such order", ErrBad)
}

// MarshalJSON returns the rules in the JSON form [Parse] reads, the same for
// the same rules.
func (r Rules) MarshalJSON() ([]byte, error) {
	out := wireRules{Match: string(r.match), Rules: make([]wireItem, len(r.conditions)), Order: string(r.order)}
	for i := range r.conditions {
		c := &r.conditions[i]
		if c.Group == nil {
			out.Rules[i] = wireRule(&c.Rule)
			continue
		}
		g := wireItem{Match: string(c.Group.Match), Rules: make([]wireItem, len(c.Group.Rules))}
		for j := range c.Group.Rules {
			g.Rules[j] = wireRule(&c.Group.Rules[j])
		}
		out.Rules[i] = g
	}
	if r.limit != nil {
		out.Limit = &wireLimit{Count: r.limit.Count, Unit: string(r.limit.Unit), By: string(r.limit.By)}
	}
	b, err := json.Marshal(&out)
	if err != nil {
		return nil, fmt.Errorf("encode rules: %w", err)
	}
	return b, nil
}

func wireRule(r *Rule) wireItem {
	var value []byte
	if k := r.Kind(); k == KindText || k == KindPick {
		value, _ = jsontext.AppendQuote(nil, r.Text) // Parse checked it is valid UTF-8.
	} else {
		value = fmt.Appendf(nil, "%d", r.Number)
	}
	return wireItem{Field: string(r.Field), Op: string(r.Op), Value: value}
}
