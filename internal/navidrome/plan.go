package navidrome

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store"
	"github.com/simonhull/kitsune/internal/store/navidromedb"
)

// The sources imported plays carry. An undated play's time is the latest it
// can have happened, not when it did: a history view should leave them out.
const (
	SourceDated   = "navidrome"
	SourceUndated = store.SourceUndated
)

// Sentinels for a -user that names nobody, and for the path gate.
var (
	// ErrNoUser means a -user ND names no Navidrome user.
	ErrNoUser = errors.New("no Navidrome user has that name")
	// ErrNoAccount means a -user KITSUNE names no Kitsune account.
	ErrNoAccount = errors.New("no Kitsune account has that name")
	// ErrGate means too few songs matched by path to trust the rest.
	ErrGate = errors.New("fewer than half the songs with plays or marks match a Kitsune track by path; run with -dry-run -v and look at the pattern before importing")
)

// Options steers a plan.
type Options struct {
	Accounts []Account         // Kitsune's accounts.
	Users    map[string]string // -user: Navidrome name to Kitsune name, for names that differ.
	Roots    map[string]string // -root: Navidrome library path to Kitsune root, for one mounted elsewhere.
	Paths    map[string]string // -map: Navidrome relative path to Kitsune relative path, for a moved file.
	Now      time.Time         // Stands in for a heart's or a rating's missing time.
	// HeartStars is what a starred song with no rating is rated, -heart-stars:
	// [rating.None] leaves it unrated.
	HeartStars rating.Rating
}

// Account is a Kitsune account.
type Account struct {
	ID   int64
	Name string
}

// Plan is a whole import, resolved: one person per Navidrome user with a
// Kitsune account.
type Plan struct {
	People   []Person
	Unmapped []string // Navidrome users with no Kitsune account, by name.
	Gate     Gate
}

// Person is one Navidrome user's import.
type Person struct {
	From   string // The Navidrome name.
	To     string // The Kitsune name.
	Import store.Import
	// HeartStars is what their starred songs with no rating are rated.
	HeartStars rating.Rating
	Report     Report
}

// Report says what a person's import found, before anything is written.
// Plays are counted one by one; marks one per item. Kitsune likes songs only,
// in stars: album and artist marks are counted, and not imported.
type Report struct {
	Plays, Dated, Undated Tally
	Ratings               Tally
	UnratedHearts         Tally // Starred songs with no rating, rated [Options.HeartStars].
	RatedHearts           int   // Starred songs with a rating, which keep just the rating.
	AlbumHearts           Tally // Matched through their songs. Not imported.
	ArtistHearts          Tally // Starred or rated artists. Not imported.
	SplitAlbums           int   // Starred Navidrome albums whose songs lie in more than one Kitsune album.
	ArtistRatings         int   // Rated artists without a star. Not imported.
	AlbumRatings          int   // Dropped: Kitsune rates songs only.
	NoDatePlays           int   // Dropped: a count with no time at all to stamp.
	Left                  navidromedb.Left
	Unmatched             []string // Paths of songs with plays or marks that matched nothing, relative.
}

// Tally counts one kind of item and how each found its Kitsune match.
type Tally struct {
	Source                         int
	ByPath, ByName, ByMBID, BySong int // Albums match by their songs.
	Unmatched                      int
}

// Matched counts the items that found a match.
func (t *Tally) Matched() int { return t.ByPath + t.ByName + t.ByMBID + t.BySong }

// add counts an item that matched as h.
func (t *Tally) add(h how) {
	t.Source++
	switch h {
	case byPath:
		t.ByPath++
	case byName:
		t.ByName++
	case byMBID:
		t.ByMBID++
	case unmatched:
		t.Unmatched++
	}
}

// Gate is the check before writing: of the songs anybody played or marked,
// at least half must match by path, or the rest is not to be trusted.
type Gate struct{ Songs, ByPath int }

// Check returns an error wrapping [ErrGate] if fewer than half matched by path.
func (g Gate) Check() error {
	if 2*g.ByPath < g.Songs {
		return fmt.Errorf("%d of %d by path: %w", g.ByPath, g.Songs, ErrGate)
	}
	return nil
}

// New plans the import of src into a library that targets describes.
func New(src *Source, targets *store.ImportTargets, opts *Options) (Plan, error) {
	r := newResolver(targets)
	matches, err := r.resolveSongs(src, opts)
	if err != nil {
		return Plan{}, err
	}
	people, unmapped, err := pairUsers(src.Users, opts)
	if err != nil {
		return Plan{}, err
	}
	p := planner{src: src, matches: matches, r: r, now: opts.Now, heartStars: opts.HeartStars, albums: albumsOf(src.Songs, matches)}
	plan := Plan{Unmapped: unmapped}
	gate := map[string]bool{} // Annotated songs, by ID: matched by path or not.
	for _, pair := range people {
		person := p.person(pair.nd, pair.account, gate)
		plan.People = append(plan.People, person)
	}
	for _, path := range gate {
		plan.Gate.Songs++
		if path {
			plan.Gate.ByPath++
		}
	}
	return plan, nil
}

// pair is a Navidrome user and their Kitsune account.
type pair struct {
	nd      navidromedb.User
	account Account
}

// pairUsers finds each Navidrome user's Kitsune account, by name ignoring
// ASCII case as Kitsune's names do, after -user.
func pairUsers(users []navidromedb.User, opts *Options) ([]pair, []string, error) {
	var errs []error
	for _, from := range slices.Sorted(maps.Keys(opts.Users)) {
		if !slices.ContainsFunc(users, func(u navidromedb.User) bool { return u.Name == from }) {
			errs = append(errs, fmt.Errorf("-user %s: %w", from, ErrNoUser))
		}
	}
	var (
		pairs    []pair
		unmapped []string
	)
	for _, u := range users {
		name, explicit := opts.Users[u.Name]
		if !explicit {
			name = u.Name
		}
		i := slices.IndexFunc(opts.Accounts, func(a Account) bool { return equalASCIIFold(a.Name, name) })
		switch {
		case i >= 0:
			pairs = append(pairs, pair{nd: u, account: opts.Accounts[i]})
		case explicit:
			errs = append(errs, fmt.Errorf("-user %s=%s: %w", u.Name, name, ErrNoAccount))
		default:
			unmapped = append(unmapped, u.Name)
		}
	}
	return pairs, unmapped, errors.Join(errs...)
}

// equalASCIIFold compares as SQLite's NOCASE does: ASCII letters only.
func equalASCIIFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		if lowerASCII(a[i]) != lowerASCII(b[i]) {
			return false
		}
	}
	return true
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// albumsOf gives each Navidrome album the Kitsune albums its matched songs
// lie in, sorted.
func albumsOf(songs []navidromedb.Song, matches map[string]match) map[string][]string {
	out := map[string][]string{}
	for i := range songs {
		m, ok := matches[songs[i].ID]
		if !ok {
			continue
		}
		if albums := out[songs[i].AlbumID]; !slices.Contains(albums, m.album) {
			out[songs[i].AlbumID] = append(albums, m.album)
		}
	}
	for _, albums := range out {
		slices.Sort(albums)
	}
	return out
}

// planner plans one person at a time over what every person shares.
type planner struct {
	src     *Source
	matches map[string]match
	albums  map[string][]string
	r       *resolver
	now     time.Time
	// heartStars is what an unrated heart becomes.
	heartStars rating.Rating
}

// person plans one Navidrome user's import into account, noting in gate each
// song they played or marked and whether it matched by path.
func (p *planner) person(nd navidromedb.User, account Account, gate map[string]bool) Person {
	marks := p.src.Marks[nd.ID]
	out := Person{From: nd.Name, To: account.Name, Import: store.Import{UserID: account.ID}, HeartStars: p.heartStars}
	out.Report.Left = marks.Left
	paths := make(map[string]string, len(p.src.Songs))
	for i := range p.src.Songs {
		paths[p.src.Songs[i].ID] = p.src.Songs[i].Path
	}
	lost := map[string]bool{}
	note := func(songID string) {
		m := p.matches[songID]
		gate[songID] = m.how == byPath
		if m.how == unmatched {
			lost[cmp.Or(paths[songID], songID)] = true
		}
	}

	p.plays(&out, &marks, note)
	songs := map[string]*navidromedb.Annotation{}
	for i := range marks.Annotations {
		a := &marks.Annotations[i]
		switch a.ItemType {
		case navidromedb.ItemSong:
			songs[a.ItemID] = a
		case navidromedb.ItemAlbum:
			p.album(&out, a)
		case navidromedb.ItemArtist:
			p.artist(&out, a)
		}
	}
	p.songMarks(&out, songs, note)
	out.Report.Unmatched = slices.Sorted(maps.Keys(lost))
	return out
}

// history is a person's plays as Navidrome kept them, by song ID.
type history struct {
	scrobbles map[string][]navidromedb.Scrobble
	counts    map[string]int64
	dates     map[string]time.Time
	first     time.Time // The person's first scrobble; zero with none.
}

// historyOf gathers a person's plays.
func historyOf(marks *Marks) history {
	h := history{scrobbles: map[string][]navidromedb.Scrobble{}, counts: map[string]int64{}, dates: map[string]time.Time{}}
	for _, s := range marks.Scrobbles {
		h.scrobbles[s.SongID] = append(h.scrobbles[s.SongID], s)
		if h.first.IsZero() || s.PlayedAt.Before(h.first) {
			h.first = s.PlayedAt
		}
	}
	for i := range marks.Annotations {
		if a := &marks.Annotations[i]; a.ItemType == navidromedb.ItemSong && a.PlayCount > 0 {
			h.counts[a.ItemID] = a.PlayCount
			h.dates[a.ItemID] = a.PlayDate
		}
	}
	return h
}

// songs returns the IDs of the songs played, sorted.
func (h *history) songs() []string {
	ids := slices.Collect(maps.Keys(h.counts))
	for id := range h.scrobbles {
		if _, ok := h.counts[id]; !ok {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// plays stamps the person's plays. A scrobble is a play at its own time. A
// song without scrobbles has its last play at play_date, and the earlier ones
// there too, since they can't have been later. Plays past a song's scrobbles
// came before the first scrobble, and are stamped just before it. So every
// song's latest play is Navidrome's play_date, as recency reads it.
func (p *planner) plays(out *Person, marks *Marks, note func(string)) {
	h := historyOf(marks)
	for _, id := range h.songs() {
		note(id)
		p.songPlays(out, &h, id)
	}
}

// songPlays stamps one song's plays.
func (p *planner) songPlays(out *Person, h *history, id string) {
	scrobbles := h.scrobbles[id]
	count := max(h.counts[id], int64(len(scrobbles)))
	var (
		undatedAt time.Time
		undated   int64
	)
	switch {
	case len(scrobbles) > 0:
		for _, s := range scrobbles {
			p.play(out, id, s.PlayedAt, SourceDated, "navidrome:s:"+strconv.FormatInt(s.ID, 10))
		}
		undatedAt, undated = h.first.Add(-time.Millisecond), count-int64(len(scrobbles))
	case !h.dates[id].IsZero():
		p.play(out, id, h.dates[id], SourceDated, "navidrome:d:"+id)
		undatedAt, undated = h.dates[id], count-1
	default:
		out.Report.NoDatePlays += int(count)
		return
	}
	for k := range undated {
		p.play(out, id, undatedAt, SourceUndated, "navidrome:u:"+id+":"+strconv.FormatInt(k+1, 10))
	}
}

// play adds one play of the Navidrome song songID, if it matched, and counts it.
func (p *planner) play(out *Person, songID string, at time.Time, source, clientID string) {
	m := p.matches[songID]
	out.Report.Plays.add(m.how)
	if source == SourceDated {
		out.Report.Dated.add(m.how)
	} else {
		out.Report.Undated.add(m.how)
	}
	if m.how != unmatched {
		out.Import.Plays = append(out.Import.Plays, store.NewPlay{
			UserID: out.Import.UserID, TrackID: m.track, PlayedAt: at, Source: source, ClientID: clientID,
		})
	}
}

// halvesPerStar is how many of Kitsune's half stars one of Navidrome's stars is.
const halvesPerStar = 2

// rated is a song's rating, with Navidrome's time, zero when it had none.
type rated struct {
	r      rating.Rating
	at, by time.Time
}

// songMarks adds the person's song ratings. A starred song keeps its rating
// alone; one with none is rated the plan's heart stars, when it has them.
func (p *planner) songMarks(out *Person, songs map[string]*navidromedb.Annotation, note func(string)) {
	tm := trackMarks{hearts: map[string]time.Time{}, ratings: map[string]rated{}}
	for _, id := range slices.Sorted(maps.Keys(songs)) {
		a := songs[id]
		if !marked(a) {
			continue
		}
		note(id)
		m := p.matches[id]
		r, err := rating.FromHalves(halvesPerStar * a.Rating)
		if err == nil {
			out.Report.Ratings.add(m.how)
			if m.how != unmatched {
				tm.rate(m.track, rated{r: r, at: p.or(a.RatedAt), by: a.RatedAt})
			}
		}
		switch {
		case !a.Starred:
		case err == nil:
			out.Report.RatedHearts++
		default:
			out.Report.UnratedHearts.add(m.how)
			if m.how != unmatched {
				tm.heart(m.track, p.or(a.StarredAt))
			}
		}
	}
	tm.into(&out.Import, p.heartStars)
}

// trackMarks are song marks by Kitsune track, where two Navidrome songs may
// meet: the earliest unrated heart, and the latest rating.
type trackMarks struct {
	hearts  map[string]time.Time
	ratings map[string]rated
}

func (t *trackMarks) heart(track string, at time.Time) {
	if was, ok := t.hearts[track]; !ok || at.Before(was) {
		t.hearts[track] = at
	}
}

func (t *trackMarks) rate(track string, r rated) {
	if was, ok := t.ratings[track]; !ok || r.by.After(was.by) {
		t.ratings[track] = r
	}
}

// into adds the marks to imp, by track ULID: the ratings, then heartStars for
// each heart on a track no Navidrome song rated, at the heart's time. With no
// heart stars, a heart adds nothing.
func (t *trackMarks) into(imp *store.Import, heartStars rating.Rating) {
	for _, id := range slices.Sorted(maps.Keys(t.ratings)) {
		imp.Ratings = append(imp.Ratings, store.ImportedRating{TrackID: id, Rating: t.ratings[id].r, At: t.ratings[id].at})
	}
	if !heartStars.Valid() {
		return
	}
	for _, id := range slices.Sorted(maps.Keys(t.hearts)) {
		if _, ok := t.ratings[id]; !ok {
			imp.HeartRatings = append(imp.HeartRatings, store.ImportedRating{TrackID: id, Rating: heartStars, At: t.hearts[id]})
		}
	}
}

// album counts a starred or rated Navidrome album. Neither is imported:
// Kitsune likes songs only.
func (p *planner) album(out *Person, a *navidromedb.Annotation) {
	if a.Rating > 0 {
		out.Report.AlbumRatings++
	}
	if !a.Starred {
		return
	}
	albums := p.albums[a.ItemID]
	t := &out.Report.AlbumHearts
	t.Source++
	if len(albums) == 0 {
		t.Unmatched++
		return
	}
	t.BySong++
	if len(albums) > 1 {
		out.Report.SplitAlbums++
	}
}

// artist counts a starred or rated Navidrome artist, matched as an artist
// would be. Neither is imported: how much a person likes an artist comes from
// how they rate the artist's songs.
func (p *planner) artist(out *Person, a *navidromedb.Annotation) {
	if !marked(a) {
		return
	}
	if !a.Starred {
		out.Report.ArtistRatings++
	}
	h := unmatched
	if i := slices.IndexFunc(p.src.Artists, func(x navidromedb.Artist) bool { return x.ID == a.ItemID }); i >= 0 {
		h = p.r.artist(&p.src.Artists[i])
	}
	out.Report.ArtistHearts.add(h)
}

// or returns t, or the plan's now when t is zero.
func (p *planner) or(t time.Time) time.Time {
	if t.IsZero() {
		return p.now
	}
	return t
}
