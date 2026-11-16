package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json/v2"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/simonhull/kitsune/internal/smartlist"
	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// A smart playlist is rules, not entries: it is worked out each time it is
// read, so a song joins or leaves as it is rated, played, added or trashed, with
// nothing to refresh. Its rules compile to one query, built here over the
// allow-listed expressions in [smartColumns] with every value a bound
// parameter: no request text ever reaches the statement. What SQL can't say is
// worked out in Go first and bound as a JSON array of rows: text rules, which
// fold as a search does, against what the index keeps of the library; genre
// rules, through the display genres the caller resolves ([GenreAlbums]); and
// playlist rules, a regular playlist through its entries and a smart one by
// evaluating it, at most [MostSmartDepth] deep.
//
// The limit and the order are applied in Go, after the query: their ties fall
// back to the Library's own order, which lives in orderIndex and nowhere else.

// MostSmartDepth is how deep playlist rules may reach through smart playlists.
// One further down matches nothing.
const MostSmartDepth = 3

// copySuffix is what a copy's name ends with.
const copySuffix = " (copy)"

// day is a day in milliseconds, which a days rule counts back in.
const day = int64(24 * time.Hour / time.Millisecond)

// minute is a minute in milliseconds, which a length rule and a limit of
// minutes count in.
const minute = int64(time.Minute / time.Millisecond)

// The columns a rule's rows are tested against.
const (
	trackColumn = "t.id"
	albumColumn = "t.album_id"
)

// bytesPerGB is what a GB limit counts in.
const bytesPerGB = 1_000_000_000

// Sentinels callers match with [errors.Is].
var (
	// ErrSmartPlaylist means a write names a smart playlist's entries, which it
	// doesn't have: its rules choose its songs.
	ErrSmartPlaylist = errors.New("a smart playlist's rules choose its songs")
	// ErrNotSmart means a change to rules or a seed names a regular playlist.
	ErrNotSmart = errors.New("not a smart playlist")
	// ErrSmartTooDeep means rules reach more than [MostSmartDepth] smart
	// playlists deep through playlist rules.
	ErrSmartTooDeep = errors.New("playlist rules reach at most 3 smart playlists deep")
)

// CycleError is rules that name a playlist that names them back, directly or
// through others. Nothing was saved.
type CycleError struct {
	Playlists []string // The playlists in the cycle, by ULID, from the one saved.
}

func (e *CycleError) Error() string {
	return "playlist rules name each other: " + strings.Join(e.Playlists, " → ")
}

// GenreAlbums returns the public IDs of the albums with a display genre that
// keep accepts. Genres are derived in memory (library.Genres), so the store
// asks for them. A nil GenreAlbums knows no genre: a genre rule then matches as
// if no album had one.
type GenreAlbums func(keep func(genre string) bool) []string

// FoldText folds text the way a smart rule compares it, as a search folds:
// case, accents and the letters searchFolder replaces go.
func FoldText(s string) string {
	ascii := true
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return strings.ToLower(s)
	}
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return dropRune
		}
		return r
	}, strings.ToLower(foldSearch(norm.NFKD.String(s))))
}

// SmartPreview is what rules choose now, as a rules editor's live card shows it.
type SmartPreview struct {
	Songs, Matched, Albums int // Matched is before the limit.
	Duration               time.Duration
	Mosaic                 []PlaylistAlbum
	First                  []PreviewSong // At most [PreviewSongs].
}

// PreviewSongs is how many songs a preview lists.
const PreviewSongs = 40

// PreviewSong is a song a preview lists.
type PreviewSong struct {
	ID, Album, Title string // ULIDs and the title.
	Track            IndexTrack
}

// smartSong is one song the query found, with what the limit, the order and a
// page read of it.
type smartSong struct {
	row, album       int64
	durationMs, size int64
	addedAt          int64
	year             int64 // 0 is unknown.
	bitrate          int64 // bps; 0 is unknown.
	format           string
	halves, plays    int64
	lastPlayed       int64 // 0 is never.
	rank             int   // Its place in the Library's order.
}

// smartChoice is what rules chose: the songs, in order, and how many matched
// before the limit.
type smartChoice struct {
	songs   []smartSong
	matched int
}

// smartBasis is what evaluating reads of the library, kept between library
// writes as the index's rows are: the Library's order, and each song's and
// album's names, folded as a rule compares them only when a rule asks.
type smartBasis struct {
	gen     uint64
	tracks  []sqlcdb.ListIndexTracksRow
	albums  []sqlcdb.ListIndexAlbumsRow
	byRow   map[int64]int    // Index into tracks, by track row.
	album   map[int64]int    // Index into albums, by album row.
	albumOf map[string]int64 // Album row, by ULID.
	rank    map[int64]int    // Place in the Library's order, by track row.

	artistRows []sqlcdb.ListIndexTrackArtistsRow

	titlesOnce, albumsOnce, artistsOnce, albumArtistsOnce, namesOnce, detailsOnce sync.Once

	details map[int64]*sqlcdb.ListIndexTrackDetailsRow // By track row, read on the first ask.

	foldedTitles       []string           // By index into tracks.
	foldedAlbums       map[int64]string   // By album row.
	foldedArtists      map[int64][]string // By track row.
	foldedAlbumArtists map[int64][]string // By track row.
	artistNames        map[int64][]string // As shown, by track row.
}

// libraryOrder returns each live track's place in the Library's order, by row:
// albums under their headings as orderIndex has them, and each album's songs
// in the order ListIndexTracks reads them.
func libraryOrder(albums []sqlcdb.ListIndexAlbumsRow, tracks []sqlcdb.ListIndexTracksRow) map[int64]int {
	groups := indexGroups(&indexRows{albums: albums})
	rowOf := make(map[string]int64, len(albums))
	for i := range albums {
		rowOf[albums[i].Ulid] = albums[i].ID
	}
	place := make(map[int64]int, len(albums)) // Album row → its place.
	for _, artist := range orderIndex(groups) {
		for i := range artist.Albums {
			place[rowOf[artist.Albums[i].ID]] = len(place)
		}
	}
	order := make([]int, len(tracks))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Compare(place[tracks[a].AlbumID], place[tracks[b].AlbumID])
	})
	rank := make(map[int64]int, len(tracks))
	for at, i := range order {
		rank[tracks[i].ID] = at
	}
	return rank
}

// smartBasis returns what evaluating reads of the library, as the read
// transaction q is in sees it. mark is where library writes stood before its
// snapshot, which q's first read must already have taken.
func (s *Store) smartBasis(ctx context.Context, q *sqlcdb.Queries, mark writesMark) (*smartBasis, error) {
	l := &s.library
	settled := mark.settled && l.writing.Load() == 0 && l.gen.Load() == mark.gen
	if b := l.smart.Load(); settled && b != nil && b.gen == mark.gen {
		return b, nil
	}
	albums, err := read(l, &l.albums, mark, "", func() ([]sqlcdb.ListIndexAlbumsRow, error) { return q.ListIndexAlbums(ctx) })
	if err != nil {
		return nil, fmt.Errorf("albums: %w", err)
	}
	tracks, err := read(l, &l.tracks, mark, "", func() ([]sqlcdb.ListIndexTracksRow, error) { return q.ListIndexTracks(ctx) })
	if err != nil {
		return nil, fmt.Errorf("tracks: %w", err)
	}
	artists, err := read(l, &l.artists, mark, "", func() ([]sqlcdb.ListIndexTrackArtistsRow, error) {
		return q.ListIndexTrackArtists(ctx)
	})
	if err != nil {
		return nil, fmt.Errorf("track artists: %w", err)
	}
	b := newSmartBasis(mark.gen, albums, tracks, artists)
	if settled {
		l.smart.Store(b)
	}
	return b, nil
}

// newSmartBasis returns the basis of the rows read at library generation gen,
// with each track and album found by row and each album by ULID.
func newSmartBasis(gen uint64, albums []sqlcdb.ListIndexAlbumsRow, tracks []sqlcdb.ListIndexTracksRow, artists []sqlcdb.ListIndexTrackArtistsRow) *smartBasis {
	b := &smartBasis{
		gen: gen, tracks: tracks, albums: albums, artistRows: artists,
		byRow: make(map[int64]int, len(tracks)), album: make(map[int64]int, len(albums)),
		albumOf: make(map[string]int64, len(albums)), rank: libraryOrder(albums, tracks),
	}
	for i := range tracks {
		b.byRow[tracks[i].ID] = i
	}
	for i := range albums {
		b.album[albums[i].ID] = i
		b.albumOf[albums[i].Ulid] = albums[i].ID
	}
	return b
}

// titles returns each track's title folded, by index into tracks.
func (b *smartBasis) titles() []string {
	b.titlesOnce.Do(func() {
		b.foldedTitles = make([]string, len(b.tracks))
		for i := range b.tracks {
			b.foldedTitles[i] = FoldText(b.tracks[i].Title)
		}
	})
	return b.foldedTitles
}

// albumNames returns each album's title folded, by row.
func (b *smartBasis) albumNames() map[int64]string {
	b.albumsOnce.Do(func() {
		b.foldedAlbums = make(map[int64]string, len(b.albums))
		for i := range b.albums {
			b.foldedAlbums[b.albums[i].ID] = FoldText(b.albums[i].Name)
		}
	})
	return b.foldedAlbums
}

// artists returns each track's artists folded, by row.
func (b *smartBasis) artists() map[int64][]string {
	b.artistsOnce.Do(func() {
		b.foldedArtists = foldNames(b.artistRows, func(r *sqlcdb.ListIndexTrackArtistsRow) (int64, string) { return r.TrackID, r.Name })
	})
	return b.foldedArtists
}

// names returns each track's artists as the index shows them, by row.
func (b *smartBasis) names() map[int64][]string {
	b.namesOnce.Do(func() {
		b.artistNames = make(map[int64][]string, len(b.tracks))
		for i := range b.artistRows {
			r := &b.artistRows[i]
			b.artistNames[r.TrackID] = append(b.artistNames[r.TrackID], r.Name)
		}
	})
	return b.artistNames
}

// albumArtists returns each track's album artists folded, by row, reading them
// on the first ask.
func (s *Store) albumArtists(ctx context.Context, q *sqlcdb.Queries, mark writesMark, b *smartBasis) (map[int64][]string, error) {
	var err error
	b.albumArtistsOnce.Do(func() {
		l := &s.library
		var rows []sqlcdb.ListIndexTrackAlbumArtistsRow
		rows, err = read(l, &l.albumArtists, mark, "", func() ([]sqlcdb.ListIndexTrackAlbumArtistsRow, error) {
			return q.ListIndexTrackAlbumArtists(ctx)
		})
		if err != nil {
			err = fmt.Errorf("album artists: %w", err)
			return
		}
		b.foldedAlbumArtists = foldNames(rows, func(r *sqlcdb.ListIndexTrackAlbumArtistsRow) (int64, string) { return r.TrackID, r.Name })
	})
	if err != nil {
		// A failed read must not leave the basis thinking it has them.
		s.library.smart.CompareAndSwap(b, nil)
		return nil, err
	}
	return b.foldedAlbumArtists, nil
}

// trackDetails returns each track's details by row, reading them on the first
// ask.
func (s *Store) trackDetails(ctx context.Context, q *sqlcdb.Queries, mark writesMark, b *smartBasis) (map[int64]*sqlcdb.ListIndexTrackDetailsRow, error) {
	var err error
	b.detailsOnce.Do(func() {
		l := &s.library
		var rows []sqlcdb.ListIndexTrackDetailsRow
		rows, err = read(l, &l.details, mark, "", func() ([]sqlcdb.ListIndexTrackDetailsRow, error) {
			return q.ListIndexTrackDetails(ctx)
		})
		if err != nil {
			err = fmt.Errorf("track details: %w", err)
			return
		}
		b.details = make(map[int64]*sqlcdb.ListIndexTrackDetailsRow, len(rows))
		for i := range rows {
			b.details[rows[i].ID] = &rows[i]
		}
	})
	if err != nil {
		s.library.smart.CompareAndSwap(b, nil) // A failed read must not leave the basis thinking it has them.
		return nil, err
	}
	return b.details, nil
}

func foldNames[T any](rows []T, of func(*T) (int64, string)) map[int64][]string {
	out := make(map[int64][]string, len(rows))
	for i := range rows {
		track, name := of(&rows[i])
		out[track] = append(out[track], FoldText(name))
	}
	return out
}

// evaluator works out smart rules in one read transaction.
type evaluator struct {
	s      *Store
	tx     *sql.Tx
	q      *sqlcdb.Queries
	mark   writesMark
	userID int64
	genres GenreAlbums
	now    int64
	basis  *smartBasis
	// held memoizes what each named playlist holds, by ULID, in one evaluation.
	held map[string][]int64
	// facts is what the person did with their songs, read once on the first ask.
	facts *personFacts
	// detailed says each song chosen carries its details: year, format, bitrate
	// and when it was added, which a page's columns show.
	detailed bool
}

// personFacts is what one person did with their songs, by track row: their
// stars in halves, how often they played each and when last (Unix ms), and how
// often they skipped it. A song they never touched is in none of them.
type personFacts struct {
	halves, plays, lastPlayed, skips map[int64]int64
}

// person returns the person's facts, reading them on the first ask.
func (e *evaluator) person(ctx context.Context) (*personFacts, error) {
	if e.facts != nil {
		return e.facts, nil
	}
	ratings, err := e.q.ListSmartRatings(ctx, e.userID)
	if err != nil {
		return nil, fmt.Errorf("ratings: %w", err)
	}
	plays, err := e.q.ListIndexPlays(ctx, e.userID)
	if err != nil {
		return nil, fmt.Errorf("plays: %w", err)
	}
	skips, err := e.q.ListSmartSkips(ctx, e.userID)
	if err != nil {
		return nil, fmt.Errorf("skips: %w", err)
	}
	f := &personFacts{
		halves: make(map[int64]int64, len(ratings)), plays: make(map[int64]int64, len(plays)),
		lastPlayed: make(map[int64]int64, len(plays)), skips: make(map[int64]int64, len(skips)),
	}
	for _, r := range ratings {
		f.halves[r.TrackID] = r.Halves
	}
	for _, p := range plays {
		f.plays[p.TrackID], f.lastPlayed[p.TrackID] = p.Plays, p.LastPlayed
	}
	for _, s := range skips {
		f.skips[s.TrackID] = s.Skips
	}
	e.facts = f
	return f, nil
}

// newEvaluator starts evaluating for the user in a read transaction, which it
// returns for the caller to roll back. The transaction's snapshot is taken here.
func (s *Store) newEvaluator(ctx context.Context, userID int64, genres GenreAlbums) (*evaluator, error) {
	mark := s.library.before()
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	e := &evaluator{s: s, tx: tx, q: s.rq.WithTx(tx), mark: mark, userID: userID, genres: genres, now: ms(s.now()), held: map[string][]int64{}}
	// The first read takes the snapshot, which the kept rows must come after. Any
	// read does; this one is cheap and reads nothing that matters.
	if _, err = e.q.CountPlaylistEntries(ctx, 0); err != nil {
		_ = tx.Rollback() // Nothing was written: there is nothing to undo.
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	if e.basis, err = s.smartBasis(ctx, e.q, mark); err != nil {
		_ = tx.Rollback() // Nothing was written: there is nothing to undo.
		return nil, err
	}
	return e, nil
}

func (e *evaluator) close() { _ = e.tx.Rollback() } // Nothing was written: there is nothing to undo.

// smartColumns are the expressions a rule on a song's own columns compares, by
// field: the whole allow-list. Their text is fixed here; only values are bound.
// What a person did with a song is no column: those rules are worked out from
// [personFacts] and bound as rows.
var smartColumns = map[smartlist.Field]string{ //nolint:gochecknoglobals // Fixed; nothing writes it.
	smartlist.Year:    "coalesce(t.year, 0)",
	smartlist.Added:   "t.added_at",
	smartlist.Length:  "t.duration_ms",
	smartlist.Bitrate: "((coalesce(t.bitrate, 0) + 500) / 1000)",
	smartlist.Format:  "t.format",
}

// smartComparisons are the operators a numeric rule compares with.
var smartComparisons = map[smartlist.Op]string{ //nolint:gochecknoglobals // Fixed; nothing writes it.
	smartlist.Is:      " = ?",
	smartlist.AtLeast: " >= ?",
	smartlist.AtMost:  " <= ?",
}

// The fixed pieces of the statement.
const (
	// The two ways a statement starts: with each song's size, for a GB limit, or
	// without. The rest of what a song needs is kept of the library.
	smartSelect      = "SELECT t.id FROM tracks t WHERE t.missing_since IS NULL AND "
	smartSelectSized = "SELECT t.id, t.size FROM tracks t WHERE t.missing_since IS NULL AND "
	// inRows tests a column against a bound JSON array of rows.
	inRows    = " IN (SELECT value FROM json_each(?))"
	notInRows = " NOT IN (SELECT value FROM json_each(?))"
	nothing   = "0"
	anything  = "1"
)

// statement is a query being built: its text, from fixed pieces only, and the
// values bound to it.
type statement struct {
	text strings.Builder
	args []any
}

func (st *statement) bind(text string, args ...any) {
	st.text.WriteString(text)
	st.args = append(st.args, args...)
}

// compile builds the query for rules, which reads each song's size when sized.
// depth is how many smart playlists deep it is, and path the ULIDs of those
// above it, which a playlist rule may not name.
func (e *evaluator) compile(ctx context.Context, rules *smartlist.Rules, sized bool, depth int, path []string) (*statement, error) {
	st := &statement{}
	if sized {
		st.bind(smartSelectSized)
	} else {
		st.bind(smartSelect)
	}
	if err := e.conditions(ctx, st, rules.Match(), rules.Conditions(), depth, path); err != nil {
		return nil, err
	}
	return st, nil
}

// conditions writes the conditions that count, joined by match, or
// [anything] when none does.
func (e *evaluator) conditions(ctx context.Context, st *statement, match smartlist.Match, conds []smartlist.Condition, depth int, path []string) error {
	join := " AND "
	if match == smartlist.Any {
		join = " OR "
	}
	wrote := false
	st.bind("(")
	for i := range conds {
		c := &conds[i]
		if !c.Counts() {
			continue
		}
		if wrote {
			st.bind(join)
		}
		wrote = true
		if c.Group == nil {
			if err := e.rule(ctx, st, &c.Rule, depth, path); err != nil {
				return err
			}
			continue
		}
		group := make([]smartlist.Condition, len(c.Group.Rules))
		for j := range c.Group.Rules {
			group[j] = smartlist.Condition{Rule: c.Group.Rules[j]}
		}
		if err := e.conditions(ctx, st, c.Group.Match, group, depth, path); err != nil {
			return err
		}
	}
	if !wrote {
		st.bind(anything)
	}
	st.bind(")")
	return nil
}

// rule writes one rule that counts.
func (e *evaluator) rule(ctx context.Context, st *statement, r *smartlist.Rule, depth int, path []string) error {
	switch r.Field {
	case smartlist.Title, smartlist.Album, smartlist.Artist, smartlist.AlbumArtist, smartlist.Genre:
		return e.textRule(ctx, st, r)
	case smartlist.Playlist:
		return e.playlistRule(ctx, st, r, depth, path)
	case smartlist.Rating, smartlist.Plays, smartlist.Skips, smartlist.LastPlayed:
		return e.personRule(ctx, st, r)
	case smartlist.Format, smartlist.Added, smartlist.Length, smartlist.Year, smartlist.Bitrate:
		return e.columnRule(st, r)
	}
	return fmt.Errorf("%s: %w", r.Field, smartlist.ErrBad) // Parse never lets one through.
}

// columnRule writes a rule on one of the song's own columns, [smartColumns]:
// a format compared without case, a days rule counted back from now, and a
// number compared, a length in minutes.
func (e *evaluator) columnRule(st *statement, r *smartlist.Rule) error {
	col, okCol := smartColumns[r.Field]
	if !okCol {
		return fmt.Errorf("%s: %w", r.Field, smartlist.ErrBad) // Parse never lets one through.
	}
	if r.Field == smartlist.Format {
		op := " = ? COLLATE NOCASE"
		if r.Op == smartlist.IsNot {
			op = " <> ? COLLATE NOCASE"
		}
		st.bind(col+op, r.Text)
		return nil
	}
	if r.Field == smartlist.Added {
		cutoff := e.now - r.Number*day
		if r.Op == smartlist.InLast {
			st.bind(col+" >= ?", cutoff)
		} else {
			st.bind("("+col+" IS NULL OR "+col+" < ?)", cutoff)
		}
		return nil
	}
	op, okOp := smartComparisons[r.Op]
	if !okOp {
		return fmt.Errorf("%s %s: %w", r.Field, r.Op, smartlist.ErrBad) // Parse never lets one through.
	}
	n := r.Number
	if r.Field == smartlist.Length {
		n *= minute
	}
	st.bind(col+op, n)
	return nil
}

// personRule writes a rule on what the person did with a song as a test of
// rows: a song they never touched holds 0 (unrated, never played, never
// skipped), so when 0 passes the rule keeps every song but those that fail it,
// and otherwise only those that pass. Either way the rows bound are among the
// songs they touched.
func (e *evaluator) personRule(ctx context.Context, st *statement, r *smartlist.Rule) error {
	f, err := e.person(ctx)
	if err != nil {
		return err
	}
	pass := e.personTest(r)
	keepZero := pass(0)
	var rows []int64
	for row, v := range f.of(r.Field) {
		if pass(v) != keepZero {
			rows = append(rows, row)
		}
	}
	if keepZero {
		return bindRows(st, trackColumn+notInRows, rows)
	}
	return bindRows(st, trackColumn+inRows, rows)
}

// of returns the facts a rule on field reads: none for a field that isn't the
// person's.
func (f *personFacts) of(field smartlist.Field) map[int64]int64 {
	switch field {
	case smartlist.Rating:
		return f.halves
	case smartlist.Plays:
		return f.plays
	case smartlist.Skips:
		return f.skips
	case smartlist.LastPlayed:
		return f.lastPlayed
	case smartlist.Title, smartlist.Album, smartlist.Artist, smartlist.AlbumArtist, smartlist.Genre,
		smartlist.Year, smartlist.Added, smartlist.Length, smartlist.Format, smartlist.Bitrate, smartlist.Playlist:
	}
	return nil
}

// personTest returns the test a rule on what the person did makes of one
// song's fact: a last play counted back from now, any other compared.
func (e *evaluator) personTest(r *smartlist.Rule) func(int64) bool {
	if r.Field != smartlist.LastPlayed {
		return compareNumber(r.Op, r.Number)
	}
	cutoff := e.now - r.Number*day
	if r.Op == smartlist.NotInLast {
		return func(last int64) bool { return last == 0 || last < cutoff }
	}
	return func(last int64) bool { return last != 0 && last >= cutoff }
}

// compareNumber returns the test of a number against n that op makes.
func compareNumber(op smartlist.Op, n int64) func(int64) bool {
	switch op {
	case smartlist.AtLeast:
		return func(v int64) bool { return v >= n }
	case smartlist.AtMost:
		return func(v int64) bool { return v <= n }
	case smartlist.Is, smartlist.IsNot, smartlist.Contains, smartlist.NotContains, smartlist.InLast, smartlist.NotInLast:
		// Only Is compares a number here: Parse lets no other through.
	}
	return func(v int64) bool { return v == n }
}

// textRule writes a text rule as a test of rows against the set its positive
// form matches, worked out in Go: is and contains keep that set, isNot and
// notContains leave it out.
func (e *evaluator) textRule(ctx context.Context, st *statement, r *smartlist.Rule) error {
	want := FoldText(r.Text)
	match := func(s string) bool { return s == want }
	if r.Op == smartlist.Contains || r.Op == smartlist.NotContains {
		match = func(s string) bool { return strings.Contains(s, want) }
	}
	in := inRows
	if r.Op == smartlist.IsNot || r.Op == smartlist.NotContains {
		in = notInRows
	}
	return e.bindText(ctx, st, r.Field, in, match)
}

// bindText writes in, a test of rows, against the rows whose text in field
// match accepts: tracks for a title or an artist, albums for an album or a
// genre.
func (e *evaluator) bindText(ctx context.Context, st *statement, field smartlist.Field, in string, match func(string) bool) error {
	switch field {
	case smartlist.Title:
		return bindRows(st, trackColumn+in, e.titleRows(match))
	case smartlist.Album:
		return bindRows(st, albumColumn+in, e.albumRows(match))
	case smartlist.Artist:
		return bindRows(st, trackColumn+in, anyName(e.basis.artists(), match))
	case smartlist.AlbumArtist:
		names, err := e.s.albumArtists(ctx, e.q, e.mark, e.basis)
		if err != nil {
			return err
		}
		return bindRows(st, trackColumn+in, anyName(names, match))
	case smartlist.Genre:
		return bindRows(st, albumColumn+in, e.genreRows(match))
	case smartlist.Year, smartlist.Rating, smartlist.Plays, smartlist.Skips, smartlist.LastPlayed,
		smartlist.Added, smartlist.Length, smartlist.Format, smartlist.Bitrate, smartlist.Playlist:
	}
	return fmt.Errorf("%s: %w", field, smartlist.ErrBad) // Parse never lets one through.
}

// titleRows returns the tracks whose folded title match accepts.
func (e *evaluator) titleRows(match func(string) bool) []int64 {
	var rows []int64
	for i, title := range e.basis.titles() {
		if match(title) {
			rows = append(rows, e.basis.tracks[i].ID)
		}
	}
	return rows
}

// albumRows returns the albums whose folded title match accepts.
func (e *evaluator) albumRows(match func(string) bool) []int64 {
	var rows []int64
	for row, name := range e.basis.albumNames() {
		if match(name) {
			rows = append(rows, row)
		}
	}
	return rows
}

// genreRows returns the albums with a display genre that match accepts,
// folded: none when the evaluator knows no genres.
func (e *evaluator) genreRows(match func(string) bool) []int64 {
	if e.genres == nil {
		return nil
	}
	var rows []int64
	for _, id := range e.genres(func(genre string) bool { return match(FoldText(genre)) }) {
		if row, ok := e.basis.albumOf[id]; ok {
			rows = append(rows, row)
		}
	}
	return rows
}

// anyName returns the tracks with a name that match accepts.
func anyName(names map[int64][]string, match func(string) bool) []int64 {
	var out []int64
	for track, list := range names {
		if slices.ContainsFunc(list, match) {
			out = append(out, track)
		}
	}
	return out
}

// bindRows writes test, which holds one parameter, with rows as a JSON array.
func bindRows(st *statement, test string, rows []int64) error {
	if rows == nil {
		rows = []int64{}
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("encode rows: %w", err)
	}
	st.bind(test, string(encoded))
	return nil
}

// playlistRule writes a playlist rule. A playlist the user doesn't have, one
// reached past [MostSmartDepth], and one that would name its own way back,
// match nothing, whether the rule says is or isNot.
func (e *evaluator) playlistRule(ctx context.Context, st *statement, r *smartlist.Rule, depth int, path []string) error {
	rows, err := e.holds(ctx, r.Text, depth, path)
	if errors.Is(err, errCantSay) {
		st.bind(nothing)
		return nil
	}
	if err != nil {
		return err
	}
	in := inRows
	if r.Op == smartlist.IsNot {
		in = notInRows
	}
	return bindRows(st, trackColumn+in, rows)
}

// errCantSay means a playlist rule can't say what a playlist holds, so the rule
// matches nothing. [evaluator.playlistRule] takes it; it goes no further.
var errCantSay = errors.New("can't say what the playlist holds")

// holds returns the rows of the songs the user's playlist id holds, and
// [errCantSay] when it can't say: no such playlist of theirs, or a smart one
// too deep or on path.
func (e *evaluator) holds(ctx context.Context, id string, depth int, path []string) ([]int64, error) {
	if rows, ok := e.held[id]; ok {
		return rows, nil
	}
	if slices.Contains(path, id) {
		return nil, errCantSay
	}
	row, err := e.q.PlaylistRow(ctx, sqlcdb.PlaylistRowParams{UserID: e.userID, Ulid: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errCantSay
	}
	if err != nil {
		return nil, fmt.Errorf("playlist %s: %w", id, err)
	}
	rows, err := e.playlistRows(ctx, id, &row, depth, path)
	if err != nil {
		return nil, err
	}
	e.held[id] = rows
	return rows, nil
}

// playlistRows returns the rows of the songs the playlist row, whose ULID is
// id, holds: a regular one's entries, and a smart one's choice, which is
// [errCantSay] when it is too deep to evaluate.
func (e *evaluator) playlistRows(ctx context.Context, id string, row *sqlcdb.PlaylistRowRow, depth int, path []string) ([]int64, error) {
	if row.Rules == nil {
		rows, err := e.q.ListPlaylistTrackRows(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("playlist %s: tracks: %w", id, err)
		}
		return rows, nil
	}
	if depth >= MostSmartDepth {
		return nil, errCantSay
	}
	rules, err := smartlist.Parse([]byte(*row.Rules))
	if err != nil {
		return nil, fmt.Errorf("playlist %s: %w", id, err)
	}
	choice, err := e.evaluate(ctx, &rules, seedOf(row.Seed), depth+1, append(slices.Clip(path), id))
	if err != nil {
		return nil, err
	}
	rows := make([]int64, len(choice.songs))
	for i := range choice.songs {
		rows[i] = choice.songs[i].row
	}
	return rows, nil
}

// evaluate works out what rules choose, in order.
func (e *evaluator) evaluate(ctx context.Context, rules *smartlist.Rules, seed int64, depth int, path []string) (smartChoice, error) {
	limit, limited := rules.Limit()
	sized := limited && limit.Unit == smartlist.GB
	st, err := e.compile(ctx, rules, sized, depth, path)
	if err != nil {
		return smartChoice{}, err
	}
	order := rules.Order()
	detailed := e.detailed || (limited && limit.By == smartlist.Newest) || order == smartlist.ByAdded || order == smartlist.ByYear
	songs, err := e.run(ctx, st, sized, detailed)
	if err != nil {
		return smartChoice{}, err
	}
	out := smartChoice{matched: len(songs)}
	if limited {
		slices.SortFunc(songs, chooser(limit.By, seed, e.basis))
		songs = applyLimit(songs, limit)
	}
	if order != smartlist.Chosen {
		slices.SortFunc(songs, orderBy(order, seed, e.basis))
	}
	out.songs = songs
	return out, nil
}

// run runs a compiled statement, which reads each song's row and, when sized,
// its size, and gives each song the rest from what is kept of the library, the
// details only when detailed, and the person's facts.
func (e *evaluator) run(ctx context.Context, st *statement, sized, detailed bool) ([]smartSong, error) {
	f, err := e.person(ctx)
	if err != nil {
		return nil, err
	}
	var details map[int64]*sqlcdb.ListIndexTrackDetailsRow
	if detailed {
		if details, err = e.s.trackDetails(ctx, e.q, e.mark, e.basis); err != nil {
			return nil, err
		}
	}
	rows, err := e.tx.QueryContext(ctx, st.text.String(), st.args...)
	if err != nil {
		return nil, fmt.Errorf("evaluate: %w", err)
	}
	defer func() { _ = rows.Close() }() // Read only; Err below reports a failure.
	var out []smartSong
	for rows.Next() {
		var s smartSong
		if sized {
			err = rows.Scan(&s.row, &s.size)
		} else {
			err = rows.Scan(&s.row)
		}
		if err != nil {
			return nil, fmt.Errorf("evaluate: %w", err)
		}
		tr := &e.basis.tracks[e.basis.byRow[s.row]]
		s.album, s.durationMs, s.rank = tr.AlbumID, tr.DurationMs, e.basis.rank[s.row]
		s.halves, s.plays, s.lastPlayed = f.halves[s.row], f.plays[s.row], f.lastPlayed[s.row]
		if d := details[s.row]; d != nil {
			s.addedAt, s.format = d.AddedAt, d.Format
			s.year, s.bitrate = int64(intOr0(d.Year)), int64(intOr0(d.Bitrate))
		}
		out = append(out, s)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("evaluate: %w", err)
	}
	return out, nil
}

// byRank is the Library's order, which every other order falls back to.
func byRank(a, b *smartSong) int { return cmp.Compare(a.rank, b.rank) }

// chooser returns the order a limit keeps songs in.
func chooser(by smartlist.Chooser, seed int64, b *smartBasis) func(a, b smartSong) int {
	switch by {
	case smartlist.MostPlayed:
		return func(x, y smartSong) int { return cmp.Or(cmp.Compare(y.plays, x.plays), byRank(&x, &y)) }
	case smartlist.LeastPlayed:
		return func(x, y smartSong) int { return cmp.Or(cmp.Compare(x.plays, y.plays), byRank(&x, &y)) }
	case smartlist.HighestRated:
		return func(x, y smartSong) int { return cmp.Or(cmp.Compare(y.halves, x.halves), byRank(&x, &y)) }
	case smartlist.Newest:
		return func(x, y smartSong) int { return cmp.Or(cmp.Compare(y.addedAt, x.addedAt), byRank(&x, &y)) }
	case smartlist.LeastRecent:
		return func(x, y smartSong) int { return cmp.Or(cmp.Compare(x.lastPlayed, y.lastPlayed), byRank(&x, &y)) }
	case smartlist.Random:
	}
	return shuffled(seed, b)
}

// orderBy returns the order songs are listed in.
func orderBy(order smartlist.Order, seed int64, b *smartBasis) func(a, b smartSong) int {
	switch order {
	case smartlist.ByTitle:
		return func(x, y smartSong) int {
			return cmp.Or(compareNoCase(b.tracks[b.byRow[x.row]].Title, b.tracks[b.byRow[y.row]].Title), byRank(&x, &y))
		}
	case smartlist.ByAlbum:
		return func(x, y smartSong) int {
			return cmp.Or(compareNoCase(b.albums[b.album[x.album]].Name, b.albums[b.album[y.album]].Name), byRank(&x, &y))
		}
	case smartlist.ByYear:
		return func(x, y smartSong) int {
			return cmp.Or(cmp.Compare(yearLast(x.year), yearLast(y.year)), byRank(&x, &y))
		}
	case smartlist.ByRating:
		return chooser(smartlist.HighestRated, seed, b)
	case smartlist.ByPlays:
		return chooser(smartlist.MostPlayed, seed, b)
	case smartlist.ByLastPlayed:
		return chooser(smartlist.LeastRecent, seed, b)
	case smartlist.ByAdded:
		return chooser(smartlist.Newest, seed, b)
	case smartlist.Shuffled:
		return shuffled(seed, b)
	case smartlist.Library, smartlist.Chosen:
	}
	return func(x, y smartSong) int { return byRank(&x, &y) }
}

// unknownYear is where an unknown year ranks: after every known one.
const unknownYear = 1 << 62

// yearLast ranks an unknown year after every known one.
func yearLast(year int64) int64 {
	if year == 0 {
		return unknownYear
	}
	return year
}

// shuffled returns an order the seed fixes: each song's place comes from a hash
// of the seed and its ULID, which outlives its row.
func shuffled(seed int64, b *smartBasis) func(a, b smartSong) int {
	key := func(s *smartSong) uint64 {
		h := fnv.New64a()
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], uint64(seed)) //nolint:gosec // A seed's bits, not its sign, are what mix.
		_, _ = h.Write(buf[:])                              // A hash never fails to write.
		_, _ = h.Write([]byte(b.tracks[b.byRow[s.row]].Ulid))
		return mix(h.Sum64())
	}
	return func(x, y smartSong) int { return cmp.Or(cmp.Compare(key(&x), key(&y)), byRank(&x, &y)) }
}

// splitmix64's finalizer: its three shifts and two multipliers.
const (
	mixShift1 = 30
	mixShift2 = 27
	mixShift3 = 31
	mixMul1   = 0xbf58476d1ce4e5b9
	mixMul2   = 0x94d049bb133111eb
)

// mix is splitmix64's finalizer, which spreads a hash's bits.
func mix(x uint64) uint64 {
	x ^= x >> mixShift1
	x *= mixMul1
	x ^= x >> mixShift2
	x *= mixMul2
	return x ^ (x >> mixShift3)
}

// applyLimit keeps songs, in order, up to the one that would go over limit.
func applyLimit(songs []smartSong, limit smartlist.Limit) []smartSong {
	if limit.Unit == smartlist.Songs {
		return songs[:min(len(songs), int(limit.Count))]
	}
	most, of := limit.Count*minute, func(s *smartSong) int64 { return s.durationMs }
	if limit.Unit == smartlist.GB {
		most, of = limit.Count*bytesPerGB, func(s *smartSong) int64 { return s.size }
	}
	var sum int64
	for i := range songs {
		if sum += of(&songs[i]); sum > most {
			return songs[:i]
		}
	}
	return songs
}

func seedOf(seed *int64) int64 {
	if seed == nil {
		return 0
	}
	return *seed
}

// tally counts what rules chose, as a list of playlists shows it.
func (e *evaluator) tally(choice *smartChoice) *tally {
	t := newTally()
	for i := range choice.songs {
		s := &choice.songs[i]
		t.add("", s.album, s.durationMs, nil)
	}
	return t
}

// indexTrack returns a chosen song as the index has it, with what a smart
// playlist's columns read.
func (e *evaluator) indexTrack(s *smartSong) IndexTrack {
	tr := &e.basis.tracks[e.basis.byRow[s.row]]
	names := e.basis.names()[s.row]
	if names == nil {
		names = []string{}
	}
	extra := &IndexExtra{
		Year: int(s.year), Format: s.format, Bitrate: int(s.bitrate), AddedAt: fromMS(s.addedAt), Plays: int(s.plays),
	}
	if s.lastPlayed != 0 {
		extra.LastPlayed = fromMS(s.lastPlayed)
	}
	return IndexTrack{
		ID: tr.Ulid, Title: tr.Title, Artists: names, DiscNo: intOr0(tr.DiscNo), TrackNo: intOr0(tr.TrackNo),
		Duration: time.Duration(tr.DurationMs) * time.Millisecond, Rating: ratingOf(s.halves), Extra: extra,
	}
}

// albumULID returns the public ID of the album with the row id.
func (e *evaluator) albumULID(row int64) string { return e.basis.albums[e.basis.album[row]].Ulid }

// parseStored reads rules the store kept.
func parseStored(id string, rules *string) (*smartlist.Rules, error) {
	if rules == nil {
		return nil, nil //nolint:nilnil // A regular playlist has no rules, and that is no error.
	}
	r, err := smartlist.Parse([]byte(*rules))
	if err != nil {
		return nil, fmt.Errorf("playlist %s: stored rules: %w", id, err)
	}
	return &r, nil
}

// encodeRules returns rules as the store keeps them.
func encodeRules(rules *smartlist.Rules) (string, error) {
	b, err := rules.MarshalJSON()
	if err != nil {
		return "", err //nolint:wrapcheck // MarshalJSON names it.
	}
	return string(b), nil
}

// PreviewRules works out what rules would choose for the user now, without
// saving them. editing names the smart playlist being edited, whose seed a
// random choice then follows; otherwise seed does. A playlist rule may not name
// editing: that rule matches nothing, as a save would refuse it. It returns an
// error wrapping [ErrNotFound] when editing is a playlist the user doesn't have.
func (s *Store) PreviewRules(ctx context.Context, userID int64, rules *smartlist.Rules, editing string, seed int64, genres GenreAlbums) (SmartPreview, error) {
	p, err := s.previewRules(ctx, userID, rules, editing, seed, genres)
	if err != nil {
		return SmartPreview{}, fmt.Errorf("preview rules: %w", err)
	}
	return p, nil
}

func (s *Store) previewRules(ctx context.Context, userID int64, rules *smartlist.Rules, editing string, seed int64, genres GenreAlbums) (SmartPreview, error) {
	e, err := s.newEvaluator(ctx, userID, genres)
	if err != nil {
		return SmartPreview{}, err
	}
	defer e.close()
	var path []string
	if editing != "" {
		row, rowErr := e.q.PlaylistRow(ctx, sqlcdb.PlaylistRowParams{UserID: userID, Ulid: editing})
		if rowErr != nil {
			return SmartPreview{}, notFound(rowErr)
		}
		seed, path = seedOf(row.Seed), []string{editing}
	}
	choice, err := e.evaluate(ctx, rules, seed, 0, path)
	if err != nil {
		return SmartPreview{}, err
	}
	t := e.tally(&choice)
	albums, err := playlistAlbums(ctx, e.q, t.mosaic)
	if err != nil {
		return SmartPreview{}, err
	}
	var p Playlist
	t.fill(&p, albums)
	out := SmartPreview{
		Songs: p.Songs, Matched: choice.matched, Albums: p.Albums, Duration: p.Duration, Mosaic: p.Mosaic,
		First: make([]PreviewSong, 0, min(len(choice.songs), PreviewSongs)),
	}
	for i := range choice.songs[:min(len(choice.songs), PreviewSongs)] {
		s := &choice.songs[i]
		tr := e.indexTrack(s)
		out.First = append(out.First, PreviewSong{ID: tr.ID, Album: e.albumULID(s.album), Title: tr.Title, Track: tr})
	}
	return out, nil
}

// CreateSmartPlaylist makes a smart playlist of the user's named name, with the
// ULID newID, the rules and the seed. The name must have passed
// [PlaylistName]. It returns [ErrSmartTooDeep] for rules that reach too deep
// through the user's smart playlists; then nothing is made.
func (s *Store) CreateSmartPlaylist(ctx context.Context, userID int64, newID, name string, rules *smartlist.Rules, seed int64) error {
	encoded, err := encodeRules(rules)
	if err != nil {
		return fmt.Errorf("create smart playlist: %w", err)
	}
	err = s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		if checkErr := checkPlaylistRules(ctx, q, userID, newID, rules); checkErr != nil {
			return checkErr
		}
		at := ms(s.now())
		if _, insertErr := q.InsertPlaylist(ctx, sqlcdb.InsertPlaylistParams{
			Ulid: newID, UserID: userID, Name: name, Source: PlaylistMade, CreatedAt: at, UpdatedAt: at,
			Rules: &encoded, Seed: &seed,
		}); insertErr != nil {
			return fmt.Errorf("insert: %w", insertErr)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("create smart playlist: %w", err)
	}
	return nil
}

// SetPlaylistRules changes the user's smart playlist's rules, keeping its seed.
//
// It returns an error wrapping [ErrNotFound] for a playlist the user doesn't
// have, [ErrNotSmart] for a regular one, a [*CycleError] for rules that name
// their way back to it, and [ErrSmartTooDeep] for rules that reach too deep.
// Then nothing is changed.
func (s *Store) SetPlaylistRules(ctx context.Context, userID int64, id string, rules *smartlist.Rules) error {
	encoded, err := encodeRules(rules)
	if err != nil {
		return fmt.Errorf("set rules of playlist %s: %w", id, err)
	}
	err = s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		row, rowErr := ownRow(ctx, q, userID, id)
		if rowErr != nil {
			return rowErr
		}
		if row.Rules == nil {
			return ErrNotSmart
		}
		if checkErr := checkPlaylistRules(ctx, q, userID, id, rules); checkErr != nil {
			return checkErr
		}
		return q.SetPlaylistRules(ctx, sqlcdb.SetPlaylistRulesParams{Rules: &encoded, Seed: row.Seed, UpdatedAt: ms(s.now()), ID: row.ID})
	})
	if err != nil {
		return fmt.Errorf("set rules of playlist %s: %w", id, err)
	}
	return nil
}

// SetPlaylistSeed gives the user's smart playlist a new seed: Pick again. It
// returns an error wrapping [ErrNotFound] for a playlist the user doesn't have,
// and [ErrNotSmart] for a regular one.
func (s *Store) SetPlaylistSeed(ctx context.Context, userID int64, id string, seed int64) error {
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		row, err := ownRow(ctx, q, userID, id)
		if err != nil {
			return err
		}
		if row.Rules == nil {
			return ErrNotSmart
		}
		return q.SetPlaylistSeed(ctx, sqlcdb.SetPlaylistSeedParams{Seed: &seed, UpdatedAt: ms(s.now()), ID: row.ID})
	})
	if err != nil {
		return fmt.Errorf("new seed for playlist %s: %w", id, err)
	}
	return nil
}

// checkPlaylistRules refuses rules for the user's playlist id that name their
// way back to it, or reach more than [MostSmartDepth] smart playlists deep, by
// the rules the user's smart playlists have now.
func checkPlaylistRules(ctx context.Context, q *sqlcdb.Queries, userID int64, id string, rules *smartlist.Rules) error {
	if len(rules.Playlists()) == 0 {
		return nil
	}
	rows, err := q.ListSmartPlaylistRules(ctx, userID)
	if err != nil {
		return fmt.Errorf("smart playlists: %w", err)
	}
	names, err := namedPlaylists(rows, id)
	if err != nil {
		return err
	}
	names[id] = rules.Playlists()
	return walkNamed(names, id, id, []string{""})
}

// namedPlaylists returns the playlists each of the smart playlists rows names
// by its stored rules, by ULID, leaving out id, whose rules are being saved.
func namedPlaylists(rows []sqlcdb.ListSmartPlaylistRulesRow, id string) (map[string][]string, error) {
	names := make(map[string][]string, len(rows))
	for _, r := range rows {
		if r.Rules == nil || r.Ulid == id {
			continue
		}
		stored, err := smartlist.Parse([]byte(*r.Rules))
		if err != nil {
			return nil, fmt.Errorf("playlist %s: stored rules: %w", r.Ulid, err)
		}
		names[r.Ulid] = stored.Playlists()
	}
	return names, nil
}

// walkNamed follows the playlists that at names, and those they name in turn,
// with path the way down to at, and refuses a way back to id or one more than
// [MostSmartDepth] smart playlists deep.
func walkNamed(names map[string][]string, id, at string, path []string) error {
	for _, next := range names[at] {
		if next == id {
			return &CycleError{Playlists: append(slices.Clone(path), at, id)[1:]}
		}
		if _, smart := names[next]; !smart {
			continue // A regular playlist, or none: nothing below it.
		}
		if len(path) > MostSmartDepth {
			return ErrSmartTooDeep
		}
		if err := walkNamed(names, id, next, append(path, at)); err != nil {
			return err
		}
	}
	return nil
}

// smartSongs evaluates the user's smart playlist row with rules, in a fresh
// evaluator, and returns the songs' rows in order.
func (s *Store) smartRows(ctx context.Context, userID int64, id string, rules *smartlist.Rules, seed *int64, genres GenreAlbums) ([]int64, error) {
	e, err := s.newEvaluator(ctx, userID, genres)
	if err != nil {
		return nil, err
	}
	defer e.close()
	choice, err := e.evaluate(ctx, rules, seedOf(seed), 0, []string{id})
	if err != nil {
		return nil, err
	}
	out := make([]int64, len(choice.songs))
	for i := range choice.songs {
		out[i] = choice.songs[i].row
	}
	return out, nil
}

// CopyPlaylist makes a regular playlist of the user's with the ULID newID,
// named after the user's playlist id with [copySuffix], holding its songs in
// the order they are now: a smart one's as its rules choose them. It returns an
// error wrapping [ErrNotFound] for a playlist the user doesn't have.
func (s *Store) CopyPlaylist(ctx context.Context, userID int64, id, newID string, genres GenreAlbums) error {
	err := s.copyPlaylist(ctx, userID, id, newID, genres)
	if err != nil {
		return fmt.Errorf("copy playlist %s: %w", id, err)
	}
	return nil
}

func (s *Store) copyPlaylist(ctx context.Context, userID int64, id, newID string, genres GenreAlbums) error {
	row, err := s.rq.PlaylistRow(ctx, sqlcdb.PlaylistRowParams{UserID: userID, Ulid: id})
	if err != nil {
		return notFound(err)
	}
	rules, err := parseStored(id, row.Rules)
	if err != nil {
		return err
	}
	var tracks []int64
	if rules != nil {
		if tracks, err = s.smartRows(ctx, userID, id, rules, row.Seed, genres); err != nil {
			return err
		}
	}
	name := []rune(row.Name)
	name = name[:min(len(name), MostPlaylistName-len([]rune(copySuffix)))]
	return s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		if rules == nil {
			entries, listErr := q.ListPlaylistEntries(ctx, row.ID)
			if listErr != nil {
				return fmt.Errorf("entries: %w", listErr)
			}
			for i := range entries {
				tracks = append(tracks, entries[i].TrackRow)
			}
		}
		return s.insertCopy(ctx, q, userID, newID, string(name)+copySuffix, tracks)
	})
}

// insertCopy makes the user's regular playlist newID, named name, holding the
// tracks by row, in order.
func (s *Store) insertCopy(ctx context.Context, q *sqlcdb.Queries, userID int64, newID, name string, tracks []int64) error {
	at := ms(s.now())
	made, err := q.InsertPlaylist(ctx, sqlcdb.InsertPlaylistParams{
		Ulid: newID, UserID: userID, Name: name, Source: PlaylistMade, CreatedAt: at, UpdatedAt: at,
	})
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	if len(tracks) == 0 {
		return nil
	}
	songs := make([]song, len(tracks))
	for i, t := range tracks {
		songs[i] = song{row: t}
	}
	if _, err = insertEntries(ctx, q, made, 0, songs, at); err != nil {
		return err
	}
	if err = q.TouchPlaylistAdded(ctx, sqlcdb.TouchPlaylistAddedParams{At: at, ID: made}); err != nil {
		return fmt.Errorf("touch: %w", err)
	}
	return nil
}
