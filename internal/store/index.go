package store

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// LibraryIndex is every song in the library, by artist and album, in the order
// a reader shows them.
type LibraryIndex struct {
	Artists []IndexArtist
	// Partial says the index holds only the albums [IndexOptions.Albums] named,
	// each with its After.
	Partial bool
}

// IndexArtist is one heading of the index. ID is empty for a group with no owner
// a reader shows: its albums share the artist they display.
type IndexArtist struct {
	ID       string // The artist's ULID.
	Name     string
	SortName string
	Hearted  bool // By the person the index was read for. Never on a group with no ID.
	Albums   []IndexAlbum
}

// IndexAlbum is an album with its live tracks, in disc and track order.
type IndexAlbum struct {
	ID          string // The album's ULID.
	Title       string
	SortName    string
	Artist      string
	Year        int // 0 is unknown.
	Compilation bool
	Hearted     bool   // By the person the index was read for.
	Cover       *Cover // Nil when the album has no cover.
	Tracks      []IndexTrack
	// Discs are those of Tracks, in disc order, and nil unless they say what the
	// tracks don't: when the album has more than one, or one with a name.
	Discs []Disc
	// After is, in a partial index, the album just before this one in the whole
	// index, by public ID; "" when it comes first.
	After string
}

// IndexTrack is a live track in the index.
type IndexTrack struct {
	ID              string // The track's ULID.
	Title           string
	Artists         []string
	DiscNo, TrackNo int // 0 is unknown.
	Duration        time.Duration
	Marks           Marks // The marks of the person the index was read for.
	// Extra holds what [IndexOptions] asked for, and is nil when it asked for
	// nothing. It is a pointer so an index without it costs what it did.
	Extra *IndexExtra
}

// IndexExtra is what [IndexOptions] asks for of a track. What it didn't ask for
// is zero.
type IndexExtra struct {
	Year                          int    // 0 is unknown.
	Format                        string // The container, such as "FLAC".
	SampleRate, BitDepth, Bitrate int    // Hz, bits and bits per second; 0 is unknown or lossy.
	AddedAt                       time.Time
	AlbumArtists                  []string
	Tags                          []Tag     // The values of IndexOptions.TagKeys, by key and then in order.
	Plays                         int       // By the person the index was read for.
	LastPlayed                    time.Time // Zero when never played.
	// Shuffle is the person's rules on the track: nil when it has none.
	Shuffle *ShuffleMark
}

// ShuffleMark is what a person's shuffle rules say of one track.
type ShuffleMark struct {
	Run              string // The ULID of the run it is in, or empty.
	Position, Length int    // Its place in the run, from 1, and how many songs the run has.
	Excluded         bool
}

// IndexOptions says what the index reads beyond what it always carries. The zero
// value reads nothing more.
type IndexOptions struct {
	Details      bool     // Each track's year, format, sample rate, bit depth, bitrate and when it was added.
	AlbumArtists bool     // Each track's album artists.
	Plays        bool     // How often the person played each track, and when last.
	Shuffle      bool     // The person's shuffle rules on each track.
	TagKeys      []string // Raw tags to carry the values of. The store doesn't know what they mean.
	// Albums, by public ID, makes a partial index of only those that have a
	// live track, each with where it goes in the whole one. Nil means every album.
	Albums []string
}

func (o *IndexOptions) any() bool {
	return o.Details || o.AlbumArtists || o.Plays || o.Shuffle || len(o.TagKeys) > 0
}

// indexRows is what the index's queries read, all from one snapshot.
type indexRows struct {
	albums       []sqlcdb.ListIndexAlbumsRow
	tracks       []sqlcdb.ListIndexTracksRow
	artists      []sqlcdb.ListIndexTrackArtistsRow
	marks        libraryMarks
	extra        bool // Whether IndexOptions asked for anything.
	details      []sqlcdb.ListIndexTrackDetailsRow
	albumArtists []sqlcdb.ListIndexTrackAlbumArtistsRow
	tags         []sqlcdb.ListIndexTrackTagsRow
	plays        []sqlcdb.ListIndexPlaysRow
	shuffle      []sqlcdb.ListIndexShuffleRow
}

// LibraryIndex returns every album with a live track, once, under its first
// owner, with those tracks and the user's marks on all three, and whatever opts
// asks for. An album with no owner a reader shows sits under the artist it
// displays. One read transaction covers it, so a scan or a heart can't split it.
func (s *Store) LibraryIndex(ctx context.Context, userID int64, opts *IndexOptions) (LibraryIndex, error) {
	rows, err := s.indexRows(ctx, userID, opts)
	if err != nil {
		return LibraryIndex{}, fmt.Errorf("library index: %w", err)
	}
	artists := orderIndex(indexGroups(&rows))
	if opts.Albums == nil {
		return LibraryIndex{Artists: artists}, nil
	}
	return LibraryIndex{Artists: onlyAlbums(artists, opts.Albums), Partial: true}, nil
}

// indexRows runs the index's queries, the user's marks included, on one read
// connection. What they read of the library comes from what is kept, when that
// is what the transaction sees.
func (s *Store) indexRows(ctx context.Context, userID int64, opts *IndexOptions) (indexRows, error) {
	mark := s.library.before()
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return indexRows{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Nothing was written: there is nothing to undo.
	q := s.rq.WithTx(tx)
	var rows indexRows
	// The first read takes the snapshot, which the kept rows must come after.
	if rows.marks, err = readLibraryMarks(ctx, q, userID); err != nil {
		return indexRows{}, err
	}
	l := &s.library
	if rows.albums, err = read(l, &l.albums, mark, "", func() ([]sqlcdb.ListIndexAlbumsRow, error) {
		return q.ListIndexAlbums(ctx)
	}); err != nil {
		return indexRows{}, fmt.Errorf("albums: %w", err)
	}
	if opts.Albums != nil {
		if albumErr := readAlbumRows(ctx, q, userID, opts, &rows); albumErr != nil {
			return indexRows{}, albumErr
		}
		return rows, nil
	}
	if rows.tracks, err = read(l, &l.tracks, mark, "", func() ([]sqlcdb.ListIndexTracksRow, error) {
		return q.ListIndexTracks(ctx)
	}); err != nil {
		return indexRows{}, fmt.Errorf("tracks: %w", err)
	}
	if rows.artists, err = read(l, &l.artists, mark, "", func() ([]sqlcdb.ListIndexTrackArtistsRow, error) {
		return q.ListIndexTrackArtists(ctx)
	}); err != nil {
		return indexRows{}, fmt.Errorf("track artists: %w", err)
	}
	if optErr := s.readIndexOptions(ctx, q, mark, userID, opts, &rows); optErr != nil {
		return indexRows{}, optErr
	}
	return rows, nil
}

// readIndexOptions runs the queries opts asks for, and only those.
func (s *Store) readIndexOptions(
	ctx context.Context, q *sqlcdb.Queries, mark writesMark, userID int64, opts *IndexOptions, rows *indexRows,
) error {
	var err error
	l := &s.library
	rows.extra = opts.any()
	if opts.Details {
		if rows.details, err = read(l, &l.details, mark, "", func() ([]sqlcdb.ListIndexTrackDetailsRow, error) {
			return q.ListIndexTrackDetails(ctx)
		}); err != nil {
			return fmt.Errorf("track details: %w", err)
		}
	}
	if opts.AlbumArtists {
		if rows.albumArtists, err = read(l, &l.albumArtists, mark, "", func() ([]sqlcdb.ListIndexTrackAlbumArtistsRow, error) {
			return q.ListIndexTrackAlbumArtists(ctx)
		}); err != nil {
			return fmt.Errorf("track album artists: %w", err)
		}
	}
	if len(opts.TagKeys) > 0 {
		if rows.tags, err = read(l, &l.tags, mark, tagArgs(opts.TagKeys), func() ([]sqlcdb.ListIndexTrackTagsRow, error) {
			return q.ListIndexTrackTags(ctx, opts.TagKeys)
		}); err != nil {
			return fmt.Errorf("track tags: %w", err)
		}
	}
	return readPersonal(ctx, q, userID, opts, rows)
}

// readPersonal runs the queries opts asks for that are about the person.
func readPersonal(ctx context.Context, q *sqlcdb.Queries, userID int64, opts *IndexOptions, rows *indexRows) error {
	var err error
	if opts.Plays {
		if rows.plays, err = q.ListIndexPlays(ctx, userID); err != nil {
			return fmt.Errorf("plays: %w", err)
		}
	}
	if opts.Shuffle {
		if rows.shuffle, err = q.ListIndexShuffle(ctx, userID); err != nil {
			return fmt.Errorf("shuffle rules: %w", err)
		}
	}
	return nil
}

// indexGroups puts each album, with its tracks, under who it files under, in no
// particular order.
func indexGroups(rows *indexRows) []IndexArtist {
	names := make(map[int64][]string, len(rows.tracks))
	for i := range rows.artists {
		id := rows.artists[i].TrackID
		names[id] = append(names[id], rows.artists[i].Name)
	}
	extras := indexExtras(rows)
	tracks := make(map[int64][]IndexTrack, len(rows.albums))
	albumDiscs := indexDiscs(rows.tracks)
	for i := range rows.tracks {
		t := &rows.tracks[i]
		tracks[t.AlbumID] = append(tracks[t.AlbumID], IndexTrack{
			ID: t.Ulid, Title: t.Title, Artists: names[t.ID], DiscNo: intOr0(t.DiscNo), TrackNo: intOr0(t.TrackNo),
			Duration: time.Duration(t.DurationMs) * time.Millisecond,
			Marks:    Marks{Hearted: rows.marks.tracks[t.Ulid], Stars: rows.marks.stars[t.Ulid]},
			Extra:    extras[t.ID],
		})
	}
	groups := make([]IndexArtist, 0, len(rows.albums))
	byKey := make(map[string]int, len(rows.albums))
	for i := range rows.albums {
		r := &rows.albums[i]
		head := indexHeading(r)
		head.Hearted = head.ID != "" && rows.marks.artists[head.ID]
		key := cmp.Or(head.ID, "name:"+head.Name)
		at, ok := byKey[key]
		if !ok {
			at = len(groups)
			byKey[key] = at
			groups = append(groups, head)
		}
		groups[at].Albums = append(groups[at].Albums, IndexAlbum{
			ID: r.Ulid, Title: r.Name, SortName: r.SortName, Artist: r.Artist, Year: intOr0(r.Year),
			Compilation: r.Compilation != 0, Hearted: rows.marks.albums[r.Ulid], Tracks: tracks[r.ID], Discs: albumDiscs[r.ID],
			Cover: cover(r.ArtworkHash, r.ArtworkWidth, r.ArtworkHeight, r.ArtworkAccent, r.ArtworkGround),
		})
	}
	return groups
}

// indexExtras gathers what [IndexOptions] read, by track row ID. It is nil when
// nothing was asked for.
func indexExtras(rows *indexRows) map[int64]*IndexExtra {
	if !rows.extra {
		return nil
	}
	out := make(map[int64]*IndexExtra, len(rows.tracks))
	of := func(id int64) *IndexExtra {
		e, ok := out[id]
		if !ok {
			e = &IndexExtra{}
			out[id] = e
		}
		return e
	}
	for i := range rows.tracks {
		of(rows.tracks[i].ID)
	}
	for i := range rows.details {
		r := &rows.details[i]
		e := of(r.ID)
		e.Year, e.Format, e.SampleRate = intOr0(r.Year), r.Format, intOr0(r.SampleRate)
		e.BitDepth, e.Bitrate, e.AddedAt = intOr0(r.BitDepth), intOr0(r.Bitrate), fromMS(r.AddedAt)
	}
	for i := range rows.albumArtists {
		e := of(rows.albumArtists[i].TrackID)
		e.AlbumArtists = append(e.AlbumArtists, rows.albumArtists[i].Name)
	}
	for i := range rows.tags {
		r := &rows.tags[i]
		e := of(r.TrackID)
		e.Tags = append(e.Tags, Tag{Key: r.Key, Value: r.Value})
	}
	for _, r := range rows.plays {
		e := of(r.TrackID)
		e.Plays, e.LastPlayed = int(r.Plays), fromMS(r.LastPlayed)
	}
	addShuffleMarks(rows.shuffle, of)
	return out
}

// addShuffleMarks puts each track's shuffle rules on its extra. A run's places
// count from 1 in position order, whatever gaps the positions have.
func addShuffleMarks(rows []sqlcdb.ListIndexShuffleRow, of func(int64) *IndexExtra) {
	runs := make(map[string][]sqlcdb.ListIndexShuffleRow)
	mark := func(id int64) *ShuffleMark {
		e := of(id)
		if e.Shuffle == nil {
			e.Shuffle = &ShuffleMark{}
		}
		return e.Shuffle
	}
	for _, r := range rows {
		if r.Excluded != 0 {
			mark(r.TrackID).Excluded = true
			continue
		}
		runs[r.Run] = append(runs[r.Run], r)
	}
	for run, songs := range runs {
		slices.SortFunc(songs, func(a, b sqlcdb.ListIndexShuffleRow) int { return cmp.Compare(a.Position, b.Position) })
		for i, song := range songs {
			m := mark(song.TrackID)
			m.Run, m.Position, m.Length = run, i+1, len(songs)
		}
	}
}

// indexHeading returns who an album files under: its first owner, or the artist
// it displays when nobody owns it or the owner is Various Artists, whom no reader
// shows.
func indexHeading(r *sqlcdb.ListIndexAlbumsRow) IndexArtist {
	if r.OwnerUlid == nil || r.OwnerName == nil || r.OwnerSortName == nil || r.OwnerMatchKey == nil ||
		isVarious(*r.OwnerMatchKey, r.OwnerMbid) {
		return IndexArtist{Name: r.Artist, SortName: r.Artist}
	}
	return IndexArtist{ID: *r.OwnerUlid, Name: *r.OwnerName, SortName: *r.OwnerSortName}
}

// orderIndex is the index's order, all of it, so a choice of order has one place
// to go. Artists go by sort name, ignoring case as NOCASE does, then ID, each
// under the letter a reader looks for it at: names that have none come first, and
// an accented letter files with its base letter, not after Z. An artist's albums
// go newest first, then those with no year, then compilations, as ListArtistAlbums
// has them. A client may order its copy otherwise; this is the default.
func orderIndex(artists []IndexArtist) []IndexArtist {
	letters := make(map[*IndexArtist]byte, len(artists))
	order := make([]*IndexArtist, len(artists))
	for i := range artists {
		order[i] = &artists[i]
		letters[order[i]] = indexLetter(artists[i].SortName)
		slices.SortFunc(artists[i].Albums, func(a, b IndexAlbum) int { return compareIndexAlbums(&a, &b) })
	}
	slices.SortFunc(order, func(a, b *IndexArtist) int {
		return cmp.Or(
			cmp.Compare(letters[a], letters[b]),
			compareNoCase(a.SortName, b.SortName),
			cmp.Compare(a.ID, b.ID),
			cmp.Compare(a.Name, b.Name),
		)
	})
	out := make([]IndexArtist, len(order))
	for i, a := range order {
		out[i] = *a
	}
	return out
}

func compareIndexAlbums(a, b *IndexAlbum) int {
	return cmp.Or(
		compareBool(a.Compilation, b.Compilation),
		cmp.Compare(yearRank(b.Year), yearRank(a.Year)),
		compareNoCase(a.SortName, b.SortName),
		cmp.Compare(a.ID, b.ID),
	)
}

// compareBool puts false before true.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// yearRank orders years with the unknown one below every other.
func yearRank(year int) int {
	if year == 0 {
		return -1
	}
	return year
}

// noLetter is the rank of a name with no letter from A to Z: below every letter.
const noLetter = 0

// indexLetter returns the letter from A to Z that a sort name starts with, its
// accent taken off, or [noLetter]. The client's letterOf must agree: it decides
// where the rail jumps to, and this decides where that lands.
func indexLetter(name string) byte {
	r, _ := utf8.DecodeRuneInString(name)
	if r >= utf8.RuneSelf {
		// The first rune of the decomposition is the base letter, if there is one.
		r, _ = utf8.DecodeRuneInString(norm.NFD.String(string(r)))
	}
	switch {
	case r >= 'a' && r <= 'z':
		return byte(r - 'a' + 'A')
	case r >= 'A' && r <= 'Z':
		return byte(r)
	}
	return noLetter
}

// compareNoCase compares as SQLite's NOCASE does: by byte, with A to Z lowered.
func compareNoCase(a, b string) int {
	for i := range min(len(a), len(b)) {
		if c := cmp.Compare(lowerASCII(a[i]), lowerASCII(b[i])); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a), len(b))
}

func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}
