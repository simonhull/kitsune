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
}

// IndexArtist is one heading of the index. ID is empty for a group with no owner
// a reader shows: its albums share the artist they display.
type IndexArtist struct {
	ID       string // The artist's ULID.
	Name     string
	SortName string
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
	Cover       *Cover // Nil when the album has no cover.
	Tracks      []IndexTrack
}

// IndexTrack is a live track in the index.
type IndexTrack struct {
	ID              string // The track's ULID.
	Title           string
	Artists         []string
	DiscNo, TrackNo int // 0 is unknown.
	Duration        time.Duration
}

// indexRows is what the index's queries read, all from one snapshot.
type indexRows struct {
	albums  []sqlcdb.ListIndexAlbumsRow
	tracks  []sqlcdb.ListIndexTracksRow
	artists []sqlcdb.ListIndexTrackArtistsRow
}

// LibraryIndex returns every album with a live track, once, under its first
// owner, with those tracks. An album with no owner a reader shows sits under the
// artist it displays. One read transaction covers it, so a scan can't split it.
func (s *Store) LibraryIndex(ctx context.Context) (LibraryIndex, error) {
	rows, err := s.indexRows(ctx)
	if err != nil {
		return LibraryIndex{}, fmt.Errorf("library index: %w", err)
	}
	return LibraryIndex{Artists: orderIndex(indexGroups(&rows))}, nil
}

// indexRows runs the index's three queries on one read connection.
func (s *Store) indexRows(ctx context.Context) (indexRows, error) {
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return indexRows{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Nothing was written: there is nothing to undo.
	q := s.rq.WithTx(tx)
	var rows indexRows
	if rows.albums, err = q.ListIndexAlbums(ctx); err != nil {
		return indexRows{}, fmt.Errorf("albums: %w", err)
	}
	if rows.tracks, err = q.ListIndexTracks(ctx); err != nil {
		return indexRows{}, fmt.Errorf("tracks: %w", err)
	}
	if rows.artists, err = q.ListIndexTrackArtists(ctx); err != nil {
		return indexRows{}, fmt.Errorf("track artists: %w", err)
	}
	return rows, nil
}

// indexGroups puts each album, with its tracks, under who it files under, in no
// particular order.
func indexGroups(rows *indexRows) []IndexArtist {
	names := make(map[int64][]string, len(rows.tracks))
	for i := range rows.artists {
		id := rows.artists[i].TrackID
		names[id] = append(names[id], rows.artists[i].Name)
	}
	tracks := make(map[int64][]IndexTrack, len(rows.albums))
	for i := range rows.tracks {
		t := &rows.tracks[i]
		tracks[t.AlbumID] = append(tracks[t.AlbumID], IndexTrack{
			ID: t.Ulid, Title: t.Title, Artists: names[t.ID], DiscNo: intOr0(t.DiscNo), TrackNo: intOr0(t.TrackNo),
			Duration: time.Duration(t.DurationMs) * time.Millisecond,
		})
	}
	groups := make([]IndexArtist, 0, len(rows.albums))
	byKey := make(map[string]int, len(rows.albums))
	for i := range rows.albums {
		r := &rows.albums[i]
		head := indexHeading(r)
		key := cmp.Or(head.ID, "name:"+head.Name)
		at, ok := byKey[key]
		if !ok {
			at = len(groups)
			byKey[key] = at
			groups = append(groups, head)
		}
		groups[at].Albums = append(groups[at].Albums, IndexAlbum{
			ID: r.Ulid, Title: r.Name, SortName: r.SortName, Artist: r.Artist, Year: intOr0(r.Year),
			Compilation: r.Compilation != 0, Tracks: tracks[r.ID],
			Cover: cover(r.ArtworkHash, r.ArtworkWidth, r.ArtworkHeight, r.ArtworkAccent, r.ArtworkGround),
		})
	}
	return groups
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
// go newest first, an unknown year last, as ListArtistAlbums has them.
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
		cmp.Compare(yearRank(b.Year), yearRank(a.Year)),
		compareNoCase(a.SortName, b.SortName),
		cmp.Compare(a.ID, b.ID),
	)
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
