package navidrome

import (
	"cmp"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/smartlist"
)

// A Navidrome smart playlist keeps its criteria as JSON: an expression of
// "all" or "any" over criteria, nested, and a sort, an order and a limit, as
// {"all":[{"is":{"loved":true}},{"any":[...]}],"sort":"-rating","limit":50}.
// Each criterion is one operator over one field, {"gt":{"year":2000}}. Field
// names compare without case, as Navidrome's do.
//
// mapCriteria turns them into Kitsune's rules when every criterion maps, and
// otherwise says which stopped them: a playlist is never half imported.

// defaultLoveFrom is the love line of a person with no Last.fm account: 4★, as
// lastfm_accounts.love_from starts.
const defaultLoveFrom rating.Rating = 8

// secondsPerMinute turns Navidrome's seconds into Kitsune's minutes.
const secondsPerMinute = 60

// Navidrome's top-level keys that aren't an expression, and its expressions.
const (
	ndSort   = "sort"
	ndOrder  = "order"
	ndLimit  = "limit"
	ndOffset = "offset"
	ndAll    = "all"
	ndAny    = "any"
)

// Navidrome's field names that a sort and the table share.
const (
	ndPlayCount  = "playcount"
	ndRatingName = "rating"
	ndLastPlayed = "lastplayed"
	ndDateAdded  = "dateadded"
	ndRandom     = "random"
)

// Why a criterion maps to nothing, as the report names it when the field or
// the operator alone doesn't say.
const (
	whyNesting  = "nesting"
	whyNotRules = "not criteria"
	whyTooMany  = "more than 50 rules"
	whySort     = "sort"
	whyLimit    = "limit"
)

// ndField is a Navidrome field Kitsune has, and how its values map.
type ndField struct {
	field smartlist.Field
	kind  ndKind
}

type ndKind uint8

const (
	ndText    ndKind = iota + 1
	ndNumber         // year, playcount, bitrate: as they are.
	ndRating         // 0 to 5 stars: halves.
	ndLoved          // true or false: the love line.
	ndDate           // lastplayed, dateadded: only in the last days.
	ndSeconds        // duration: minutes.
	ndFormat         // filetype.
)

// ndFields is every Navidrome field that maps, by its name in lower case.
var ndFields = map[string]ndField{ //nolint:gochecknoglobals // Fixed; nothing writes it.
	"title":       {smartlist.Title, ndText},
	"album":       {smartlist.Album, ndText},
	"artist":      {smartlist.Artist, ndText},
	"albumartist": {smartlist.AlbumArtist, ndText},
	"genre":       {smartlist.Genre, ndText},
	"year":        {smartlist.Year, ndNumber},
	ndPlayCount:   {smartlist.Plays, ndNumber},
	"bitrate":     {smartlist.Bitrate, ndNumber},
	ndRatingName:  {smartlist.Rating, ndRating},
	"loved":       {smartlist.Rating, ndLoved},
	ndLastPlayed:  {smartlist.LastPlayed, ndDate},
	ndDateAdded:   {smartlist.Added, ndDate},
	"duration":    {smartlist.Length, ndSeconds},
	"filetype":    {smartlist.Format, ndFormat},
}

// sortBy is a Navidrome sort: one field, and whether it runs high to low.
type sortBy struct {
	key  string
	desc bool
}

// choosers are the sorts a Kitsune limit chooses by.
var choosers = map[sortBy]smartlist.Chooser{ //nolint:gochecknoglobals // Fixed; nothing writes it.
	{ndRandom, false}:     smartlist.Random,
	{ndRandom, true}:      smartlist.Random,
	{ndPlayCount, true}:   smartlist.MostPlayed,
	{ndPlayCount, false}:  smartlist.LeastPlayed,
	{ndRatingName, true}:  smartlist.HighestRated,
	{ndDateAdded, true}:   smartlist.Newest,
	{ndLastPlayed, false}: smartlist.LeastRecent,
}

// orders are the sorts Kitsune lists a playlist in, each its one way.
var orders = map[sortBy]smartlist.Order{ //nolint:gochecknoglobals // Fixed; nothing writes it.
	{"", false}:           smartlist.Library,
	{ndRandom, false}:     smartlist.Shuffled,
	{ndRandom, true}:      smartlist.Shuffled,
	{"title", false}:      smartlist.ByTitle,
	{"album", false}:      smartlist.ByAlbum,
	{"year", false}:       smartlist.ByYear,
	{ndRatingName, true}:  smartlist.ByRating,
	{ndPlayCount, true}:   smartlist.ByPlays,
	{ndLastPlayed, false}: smartlist.ByLastPlayed,
	{ndDateAdded, true}:   smartlist.ByAdded,
}

// smartRule is a Kitsune rule, or a group of them, in the JSON form
// [smartlist.Parse] reads.
type smartRule struct {
	Field string `json:"field,omitzero"`
	Op    string `json:"op,omitzero"`
	Value any    `json:"value,omitzero"`
	// A group has Match and Rules instead.
	Match string      `json:"match,omitzero"`
	Rules []smartRule `json:"rules,omitzero"`
}

type smartLimit struct {
	Count int64  `json:"count"`
	Unit  string `json:"unit"`
	By    string `json:"by"`
}

type smartRules struct {
	Match string      `json:"match"`
	Rules []smartRule `json:"rules"`
	Limit *smartLimit `json:"limit,omitzero"`
	Order string      `json:"order,omitzero"`
}

// criteriaMapper maps one playlist's criteria, noting what stops them.
type criteriaMapper struct {
	loveFrom  rating.Rating
	playlists map[string]string // Kitsune's IDs of the Navidrome playlists coming over.
	named     []string          // The Navidrome playlists the criteria name, in order.
	stopped   []string
}

func (m *criteriaMapper) stop(why string) {
	if !slices.Contains(m.stopped, why) {
		m.stopped = append(m.stopped, why)
	}
}

// criteria is what one playlist's criteria map to: the rules and the
// Navidrome playlists they name, or what stopped them, each once, in the order
// met.
type criteria struct {
	rules   smartlist.Rules
	named   []string
	stopped []string // Nil when they all map.
}

// stoppedBy is criteria that don't map, for why.
func stoppedBy(why string) criteria { return criteria{stopped: []string{why}} }

// mapCriteria maps raw, a Navidrome playlist's criteria, for a person whose
// love line is loveFrom, with playlists giving the Kitsune ID of each
// Navidrome playlist coming over.
func mapCriteria(raw string, loveFrom rating.Rating, playlists map[string]string) criteria {
	m := criteriaMapper{loveFrom: cmp.Or(loveFrom, defaultLoveFrom), playlists: playlists}
	var top map[string]jsontext.Value
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		return stoppedBy(whyNotRules)
	}
	out := m.expression(top)
	m.sortAndLimit(top, &out)
	if len(m.stopped) > 0 {
		return criteria{stopped: m.stopped}
	}
	encoded, err := json.Marshal(&out)
	if err != nil {
		return stoppedBy(whyNotRules)
	}
	rules, err := smartlist.Parse(encoded)
	if err != nil {
		if count(out.Rules) > smartlist.MostRules {
			return stoppedBy(whyTooMany)
		}
		return stoppedBy(whyNotRules)
	}
	return criteria{rules: rules, named: m.named}
}

// expression maps the top level's all or any, and notes any key Kitsune
// can't take.
func (m *criteriaMapper) expression(top map[string]jsontext.Value) smartRules {
	out := smartRules{Match: string(smartlist.All), Rules: []smartRule{}}
	expressions := 0
	for _, key := range slices.Sorted(maps.Keys(top)) {
		value := top[key]
		switch strings.ToLower(key) {
		case ndAll, ndAny:
			expressions++
			out.Match = matchOf(key)
			out.Rules = m.list(value, out.Match, 0)
		case ndSort, ndOrder, ndLimit:
		case ndOffset:
			if n, ok := number(value); !ok || n != 0 {
				m.stop(ndOffset)
			}
		default:
			m.stop(key)
		}
	}
	if expressions > 1 {
		m.stop(whyNotRules)
	}
	return out
}

func matchOf(key string) string {
	if strings.EqualFold(key, ndAny) {
		return string(smartlist.Any)
	}
	return string(smartlist.All)
}

// count is how many rules out holds, those in groups included.
func count(out []smartRule) int {
	n := 0
	for i := range out {
		n += max(len(out[i].Rules), 1)
	}
	return n
}

// list maps the criteria of an all or any, combined by match, at depth: 0 is
// the top level, 1 a group, and Kitsune has no deeper.
func (m *criteriaMapper) list(raw jsontext.Value, match string, depth int) []smartRule {
	var items []map[string]jsontext.Value
	if err := json.Unmarshal(raw, &items); err != nil {
		m.stop(whyNotRules)
		return nil
	}
	out := []smartRule{}
	for _, item := range items {
		if len(item) != 1 {
			m.stop(whyNotRules)
			continue
		}
		for op, value := range item {
			out = append(out, m.item(op, value, match, depth)...)
		}
	}
	return out
}

// item maps one criterion, or a nested all or any, found in a list combined by
// match at depth.
func (m *criteriaMapper) item(op string, value jsontext.Value, match string, depth int) []smartRule {
	if lower := strings.ToLower(op); lower == ndAll || lower == ndAny {
		return m.nested(matchOf(op), value, match, depth)
	}
	var fields map[string]jsontext.Value
	if err := json.Unmarshal(value, &fields); err != nil || len(fields) != 1 {
		m.stop(whyNotRules)
		return nil
	}
	for name, v := range fields {
		rules := m.criterion(op, strings.ToLower(name), v)
		if len(rules) > 1 && match != string(smartlist.All) {
			// A range is two rules an all holds: a group of its own.
			return m.nested(string(smartlist.All), nil, match, depth, rules...)
		}
		return rules
	}
	return nil
}

// nested maps a list combined by inner inside one combined by outer at depth:
// flattened into it when they combine alike, a group at the top level, and
// too deep anywhere else. Rules already mapped stand in for raw.
func (m *criteriaMapper) nested(inner string, raw jsontext.Value, outer string, depth int, rules ...smartRule) []smartRule {
	switch {
	case inner == outer && rules == nil:
		return m.list(raw, outer, depth)
	case inner == outer:
		return rules
	case depth > 0:
		m.stop(whyNesting)
		return nil
	}
	if rules == nil {
		rules = m.list(raw, inner, depth+1)
	}
	if len(rules) == 0 {
		return nil // An empty group matches as nothing there would.
	}
	return []smartRule{{Match: inner, Rules: rules}}
}

// criterion maps op over field, with its value: one rule, or two for a range
// that an all holds.
func (m *criteriaMapper) criterion(op, name string, value jsontext.Value) []smartRule {
	switch op {
	case "inPlaylist", "notInPlaylist":
		return m.inPlaylist(op, name, value)
	case "inTheLast", "notInTheLast":
		return m.inTheLast(op, name, value)
	case "inTheRange":
		return m.inTheRange(name, value)
	}
	f, ok := ndFields[name]
	if !ok {
		m.stop(name)
		return nil
	}
	var rule *smartRule
	switch op {
	case "is", "isNot":
		rule = m.is(op, &f, value)
	case "contains", "notContains":
		rule = m.contains(op, &f, value)
	case "gt", "lt":
		rule = m.bound(op, &f, value)
	default: // startsWith, endsWith, before, after and anything newer.
		m.stop(op)
	}
	if rule == nil {
		return nil
	}
	return []smartRule{*rule}
}

// unmapped notes op over a field it can't take in Kitsune.
func (m *criteriaMapper) unmapped(op string, f *ndField) *smartRule {
	m.stop(op + " " + string(f.field))
	return nil
}

func (m *criteriaMapper) is(op string, f *ndField, value jsontext.Value) *smartRule {
	negated := op == "isNot"
	kop := string(smartlist.Is)
	if negated {
		kop = string(smartlist.IsNot)
	}
	switch f.kind {
	case ndText, ndFormat:
		if text, ok := textOf(value); ok {
			return &smartRule{Field: string(f.field), Op: kop, Value: text}
		}
	case ndLoved:
		if loved := boolOf(value); loved != nil {
			return m.loved(*loved != negated)
		}
	case ndNumber, ndRating:
		return m.isNumber(op, f, value, negated)
	case ndDate, ndSeconds:
	}
	return m.unmapped(op, f)
}

// isNumber maps is over a number, or a rating in whole stars. Kitsune says no
// number is not, and unrated is no rating it can be.
func (m *criteriaMapper) isNumber(op string, f *ndField, value jsontext.Value, negated bool) *smartRule {
	n, ok := number(value)
	if f.kind == ndRating {
		ok = ok && n >= 1 && n <= int64(rating.Max)/halvesPerStar
		n *= halvesPerStar
	}
	if !ok || negated {
		return m.unmapped(op, f)
	}
	return &smartRule{Field: string(f.field), Op: string(smartlist.Is), Value: n}
}

// loved is a rating at the love line or over it, or under it when not loved.
func (m *criteriaMapper) loved(loved bool) *smartRule {
	if loved {
		return &smartRule{Field: string(smartlist.Rating), Op: string(smartlist.AtLeast), Value: int64(m.loveFrom)}
	}
	if m.loveFrom <= rating.Min {
		// Under ½★ is unrated, which no rating rule says.
		m.stop("loved false")
		return nil
	}
	return &smartRule{Field: string(smartlist.Rating), Op: string(smartlist.AtMost), Value: int64(m.loveFrom) - 1}
}

func (m *criteriaMapper) contains(op string, f *ndField, value jsontext.Value) *smartRule {
	text, ok := textOf(value)
	if f.kind != ndText || !ok {
		return m.unmapped(op, f)
	}
	kop := string(smartlist.Contains)
	if op == "notContains" {
		kop = string(smartlist.NotContains)
	}
	return &smartRule{Field: string(f.field), Op: kop, Value: text}
}

// bound maps gt and lt, which Kitsune says as at least or at most one step
// over or under: a year, a play or a kbps; half a star; for a length, the
// minutes that keep every song the seconds did not.
func (m *criteriaMapper) bound(op string, f *ndField, value jsontext.Value) *smartRule {
	n, ok := number(value)
	if !ok {
		return m.unmapped(op, f)
	}
	over := op == "gt"
	kop := string(smartlist.AtMost)
	if over {
		kop = string(smartlist.AtLeast)
	}
	switch f.kind {
	case ndNumber:
		n = stepped(n, 1, over)
	case ndRating:
		n = stepped(halvesPerStar*n, 1, over)
	case ndSeconds:
		if over {
			n = ceilMinutes(n)
		} else {
			n /= secondsPerMinute
		}
	case ndText, ndLoved, ndDate, ndFormat: // A fixed date is before or after.
		return m.unmapped(op, f)
	}
	return &smartRule{Field: string(f.field), Op: kop, Value: n}
}

// stepped is n a step over, or under.
func stepped(n, step int64, over bool) int64 {
	if over {
		return n + step
	}
	return n - step
}

// ceilMinutes is the fewest whole minutes at or over seconds.
func ceilMinutes(seconds int64) int64 {
	return (seconds + secondsPerMinute - 1) / secondsPerMinute
}

// inTheRange is two rules, at least the first value and at most the second.
func (m *criteriaMapper) inTheRange(name string, value jsontext.Value) []smartRule {
	f, ok := ndFields[name]
	if !ok {
		m.stop(name)
		return nil
	}
	var bounds []jsontext.Value
	if err := json.Unmarshal(value, &bounds); err != nil || len(bounds) != 2 {
		m.unmapped("inTheRange", &f)
		return nil
	}
	lo, loOK := number(bounds[0])
	hi, hiOK := number(bounds[1])
	if !loOK || !hiOK {
		m.unmapped("inTheRange", &f)
		return nil
	}
	switch f.kind {
	case ndNumber:
	case ndRating:
		lo, hi = halvesPerStar*lo, halvesPerStar*hi
	case ndSeconds:
		lo, hi = ceilMinutes(lo), hi/secondsPerMinute
	case ndText, ndLoved, ndDate, ndFormat:
		m.unmapped("inTheRange", &f)
		return nil
	}
	return []smartRule{
		{Field: string(f.field), Op: string(smartlist.AtLeast), Value: lo},
		{Field: string(f.field), Op: string(smartlist.AtMost), Value: hi},
	}
}

func (m *criteriaMapper) inTheLast(op, name string, value jsontext.Value) []smartRule {
	f, ok := ndFields[name]
	if !ok {
		m.stop(name)
		return nil
	}
	days, numOK := number(value)
	if f.kind != ndDate || !numOK {
		m.unmapped(op, &f)
		return nil
	}
	kop := string(smartlist.InLast)
	if op == "notInTheLast" {
		kop = string(smartlist.NotInLast)
	}
	return []smartRule{{Field: string(f.field), Op: kop, Value: days}}
}

// inPlaylist maps to a playlist rule when the playlist named comes over too.
func (m *criteriaMapper) inPlaylist(op, name string, value jsontext.Value) []smartRule {
	id, ok := textOf(value)
	kitsune, coming := m.playlists[id]
	if name != "id" || !ok || !coming {
		m.stop(op)
		return nil
	}
	if !slices.Contains(m.named, id) {
		m.named = append(m.named, id)
	}
	kop := string(smartlist.Is)
	if op == "notInPlaylist" {
		kop = string(smartlist.IsNot)
	}
	return []smartRule{{Field: string(smartlist.Playlist), Op: kop, Value: kitsune}}
}

// sortAndLimit maps the sort, the order and the limit into out. A limit keeps
// the songs its sort ranks first, so it maps only where Kitsune chooses that
// way; a sort with no limit is the order the songs are listed in.
func (m *criteriaMapper) sortAndLimit(top map[string]jsontext.Value, out *smartRules) {
	by := sortOf(top)
	count := m.limitOf(top)
	switch chooser, ok := choosers[by]; {
	case count == 0:
		out.Order = m.order(by)
	case count > smartlist.MostSongs:
		m.stop(whyLimit)
	case !ok:
		m.stop(whyLimit + " by " + cmp.Or(by.key, "title"))
	default:
		out.Limit = &smartLimit{Count: count, Unit: string(smartlist.Songs), By: string(chooser)}
		out.Order = string(smartlist.Chosen)
	}
}

// limitOf reads the limit, 0 for none.
func (m *criteriaMapper) limitOf(top map[string]jsontext.Value) int64 {
	for key, value := range top {
		if strings.EqualFold(key, ndLimit) {
			n, ok := number(value)
			if !ok || n < 0 {
				m.stop(whyLimit)
			}
			return n
		}
	}
	return 0
}

// sortOf reads Navidrome's sort: one field with its direction, "-rating", or
// "rating" with order "desc".
func sortOf(top map[string]jsontext.Value) sortBy {
	var sort, order string
	for key, value := range top {
		switch strings.ToLower(key) {
		case ndSort:
			sort, _ = textOf(value)
		case ndOrder:
			order, _ = textOf(value)
		}
	}
	sort = strings.ToLower(strings.TrimSpace(sort))
	desc := strings.EqualFold(strings.TrimSpace(order), "desc")
	if rest, found := strings.CutPrefix(sort, "-"); found {
		return sortBy{rest, !desc}
	}
	return sortBy{strings.TrimPrefix(sort, "+"), desc}
}

// order maps a sort with no limit to the order Kitsune lists songs in.
func (m *criteriaMapper) order(by sortBy) string {
	if o, ok := orders[by]; ok {
		return string(o)
	}
	switch {
	case strings.Contains(by.key, ","):
		m.stop(whySort) // Several fields.
	case by.desc:
		m.stop(whySort + " -" + by.key)
	default:
		m.stop(whySort + " " + by.key)
	}
	return ""
}

// textOf reads a criterion's value as text: a string, or a number Navidrome
// was given as one.
func textOf(value jsontext.Value) (string, bool) {
	var s string
	if err := json.Unmarshal(value, &s); err == nil {
		return s, true
	}
	if n, ok := number(value); ok {
		return strconv.FormatInt(n, 10), true
	}
	return "", false
}

// number reads a whole number, given as a number or a string of one.
func number(value jsontext.Value) (int64, bool) {
	var f float64
	if err := json.Unmarshal(value, &f); err != nil {
		var s string
		if json.Unmarshal(value, &s) != nil {
			return 0, false
		}
		parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if parseErr != nil {
			return 0, false
		}
		f = parsed
	}
	if f != math.Trunc(f) || math.Abs(f) > 1<<53 {
		return 0, false
	}
	return int64(f), true
}

// boolOf reads true or false, given as such or as a string of one; nil for
// neither.
func boolOf(value jsontext.Value) *bool {
	var b bool
	if err := json.Unmarshal(value, &b); err == nil {
		return &b
	}
	if s, isText := textOf(value); isText {
		if parsed, err := strconv.ParseBool(s); err == nil {
			return &parsed
		}
	}
	return nil
}
