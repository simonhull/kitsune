package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// Album page sizes: the default, and the most a caller may ask for.
const (
	DefaultAlbumLimit = 60
	MaxAlbumLimit     = 200
)

// ErrBadCursor means an album cursor is malformed or came from the other sort.
var ErrBadCursor = errors.New("invalid album cursor")

// ErrBadSort means an album sort is not one of the [AlbumSort] values.
var ErrBadSort = errors.New("invalid album sort")

// ErrBadLimit means an album page size is outside 1 to [MaxAlbumLimit].
var ErrBadLimit = errors.New("invalid album page size")

// AlbumSort orders an album list.
type AlbumSort string

// The album sorts.
const (
	// SortAdded lists the newest albums first.
	SortAdded AlbumSort = "added"
	// SortName lists albums by sort name, ignoring case.
	SortName AlbumSort = "name"
)

// Cover is an album's rendered cover. Hash names the renditions on disk.
type Cover struct {
	Hash           [32]byte
	Accent, Ground string // "#rrggbb".
	Width, Height  int    // Of the source image.
}

// AlbumSummary is an album as a list shows it.
type AlbumSummary struct {
	ID          string // The album's ULID.
	Title       string
	Artist      string
	Year        int // 0 is unknown.
	TrackCount  int // Live tracks only.
	Compilation bool
	Cover       *Cover // Nil when the album has no cover.
	AddedAt     time.Time
}

// AlbumPage is one page of an album list.
type AlbumPage struct {
	Albums []AlbumSummary
	Next   string // Cursor for the next page; empty on the last.
}

// Track is a live track as an album page shows it.
type Track struct {
	ID              string // The track's ULID.
	Title           string
	Artists         []string
	DiscNo, TrackNo int // 0 is unknown.
	Duration        time.Duration
	Format          string
	SampleRate      int // 0 is unknown.
	BitDepth        int // 0 is unknown.
	ReplayGain      ReplayGain
}

// TrackFile says where a live track's file is and what it holds.
type TrackFile struct {
	Root    string
	RelPath string // Relative to Root.
	Format  string
}

// AlbumDetail is an album with its live tracks.
type AlbumDetail struct {
	AlbumSummary

	Tracks   []Track
	Duration time.Duration
	Formats  []string // Distinct and sorted.
}

// LibraryFacts counts what the library holds. Albums and tracks are live ones.
type LibraryFacts struct {
	Albums, Tracks int64
	Duration       time.Duration
	Issues         int64
}

// Albums returns one page of albums that have a live track, in the order of
// sort. cursor is empty for the first page, otherwise the Next of the previous
// one. A cursor is opaque and belongs to one sort. Paging is by keyset, so a
// write between pages never repeats or skips an album that was already there.
//
// It returns an error wrapping [ErrBadLimit] unless limit is from 1 to
// [MaxAlbumLimit], and one wrapping [ErrBadCursor] for a cursor it did not
// issue for sort.
func (s *Store) Albums(ctx context.Context, sort AlbumSort, limit int, cursor string) (AlbumPage, error) {
	if limit < 1 || limit > MaxAlbumLimit {
		return AlbumPage{}, fmt.Errorf("albums: limit %d: %w", limit, ErrBadLimit)
	}
	var rows []sqlcdb.ListAlbumsByNameRow
	var err error
	switch sort {
	case SortName:
		rows, err = s.albumsByName(ctx, limit, cursor)
	case SortAdded:
		rows, err = s.albumsByAdded(ctx, limit, cursor)
	default:
		return AlbumPage{}, fmt.Errorf("albums: unknown sort %q: %w", sort, ErrBadSort)
	}
	if err != nil {
		return AlbumPage{}, err
	}
	page := AlbumPage{Albums: make([]AlbumSummary, 0, min(len(rows), limit))}
	for i := range min(len(rows), limit) {
		page.Albums = append(page.Albums, summarize(&rows[i]))
	}
	if len(rows) > limit {
		last := &rows[limit-1]
		page.Next = encodeCursor(sort, last.SortName, last.AddedAt, last.ID)
	}
	return page, nil
}

// albumsByName fetches limit+1 rows after cursor, in sort-name order.
func (s *Store) albumsByName(ctx context.Context, limit int, cursor string) ([]sqlcdb.ListAlbumsByNameRow, error) {
	var after nameKey
	if cursor != "" {
		var err error
		if after, err = decodeNameCursor(cursor); err != nil {
			return nil, err
		}
	}
	rows, err := s.rq.ListAlbumsByName(ctx, sqlcdb.ListAlbumsByNameParams{
		AfterName: after.name, AfterID: after.id, PageSize: int64(limit) + 1,
	})
	if err != nil {
		return nil, fmt.Errorf("albums by name: %w", err)
	}
	return rows, nil
}

// albumsByAdded fetches limit+1 rows after cursor, newest first. Its row type
// has the same fields as the by-name query's, so it converts.
func (s *Store) albumsByAdded(ctx context.Context, limit int, cursor string) ([]sqlcdb.ListAlbumsByNameRow, error) {
	before := addedKey{added: math.MaxInt64, id: math.MaxInt64}
	if cursor != "" {
		var err error
		if before, err = decodeAddedCursor(cursor); err != nil {
			return nil, err
		}
	}
	rows, err := s.rq.ListAlbumsByAdded(ctx, sqlcdb.ListAlbumsByAddedParams{
		BeforeAdded: before.added, BeforeID: before.id, PageSize: int64(limit) + 1,
	})
	if err != nil {
		return nil, fmt.Errorf("albums by added: %w", err)
	}
	out := make([]sqlcdb.ListAlbumsByNameRow, len(rows))
	for i := range rows {
		out[i] = sqlcdb.ListAlbumsByNameRow(rows[i])
	}
	return out, nil
}

// nameKey and addedKey are the decoded keys of the two sorts' cursors.
type (
	nameKey struct {
		name string
		id   int64
	}
	addedKey struct{ added, id int64 }
)

// Cursor kinds, the first byte of a cursor's payload. A cursor is the base64url
// of raw bytes, not text: sort names may not be valid UTF-8.
const (
	kindName  byte = 'n' // 'n', the ID as 8 big-endian bytes, then the sort name.
	kindAdded byte = 'a' // 'a', the added time and the ID as 8 big-endian bytes each.
)

const int64Len = 8

// encodeCursor returns the cursor that resumes after the given row.
func encodeCursor(sort AlbumSort, name string, added, id int64) string {
	var b []byte
	switch sort {
	case SortName:
		b = append(b, kindName)
		b = binary.BigEndian.AppendUint64(b, uint64(id)) //nolint:gosec // G115: the bits round-trip through decode.
		b = append(b, name...)
	case SortAdded:
		b = append(b, kindAdded)
		b = binary.BigEndian.AppendUint64(b, uint64(added)) //nolint:gosec // G115: the bits round-trip through decode.
		b = binary.BigEndian.AppendUint64(b, uint64(id))    //nolint:gosec // G115: the bits round-trip through decode.
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeCursor returns the payload of a cursor of the given kind.
func decodeCursor(cursor string, kind byte) ([]byte, error) {
	b, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil || len(b) == 0 || b[0] != kind {
		return nil, fmt.Errorf("albums: cursor %q: %w", cursor, ErrBadCursor)
	}
	return b[1:], nil
}

func decodeNameCursor(cursor string) (nameKey, error) {
	b, err := decodeCursor(cursor, kindName)
	if err != nil {
		return nameKey{}, err
	}
	if len(b) < int64Len {
		return nameKey{}, fmt.Errorf("albums: cursor %q: %w", cursor, ErrBadCursor)
	}
	return nameKey{id: readInt64(b), name: string(b[int64Len:])}, nil
}

func decodeAddedCursor(cursor string) (addedKey, error) {
	b, err := decodeCursor(cursor, kindAdded)
	if err != nil {
		return addedKey{}, err
	}
	if len(b) != 2*int64Len {
		return addedKey{}, fmt.Errorf("albums: cursor %q: %w", cursor, ErrBadCursor)
	}
	return addedKey{added: readInt64(b), id: readInt64(b[int64Len:])}, nil
}

// readInt64 reads the 8 big-endian bytes that encode wrote for an int64.
func readInt64(b []byte) int64 {
	return int64(binary.BigEndian.Uint64(b)) //nolint:gosec // G115: the bits round-trip from encode.
}

// summarize builds the list entry for a row.
func summarize(r *sqlcdb.ListAlbumsByNameRow) AlbumSummary {
	a := AlbumSummary{
		ID:          r.Ulid,
		Title:       r.Name,
		Artist:      r.Artist,
		Year:        intOr0(r.Year),
		TrackCount:  int(r.TrackCount),
		Compilation: r.Compilation != 0,
		AddedAt:     fromMS(r.AddedAt),
	}
	if r.ArtworkHash != nil && r.ArtworkWidth != nil && r.ArtworkHeight != nil && r.ArtworkAccent != nil && r.ArtworkGround != nil {
		a.Cover = &Cover{
			Hash:   [32]byte(r.ArtworkHash),
			Accent: *r.ArtworkAccent, Ground: *r.ArtworkGround,
			Width: int(*r.ArtworkWidth), Height: int(*r.ArtworkHeight),
		}
	}
	return a
}

// intOr0 returns *p, or 0 for NULL.
func intOr0(p *int64) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

// floatOr0 returns *p, or 0 for NULL.
func floatOr0(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// Album returns the album with the public ID id and its live tracks, in disc
// and track order. It returns an error wrapping [ErrNotFound] for an unknown ID
// or an album with no live track.
func (s *Store) Album(ctx context.Context, id string) (AlbumDetail, error) {
	row, err := s.rq.GetAlbum(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return AlbumDetail{}, fmt.Errorf("album %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return AlbumDetail{}, fmt.Errorf("album %s: %w", id, err)
	}
	tracks, err := s.rq.ListAlbumTracks(ctx, row.ID)
	if err != nil {
		return AlbumDetail{}, fmt.Errorf("album %s: tracks: %w", id, err)
	}
	links, err := s.rq.ListAlbumTrackArtists(ctx, row.ID)
	if err != nil {
		return AlbumDetail{}, fmt.Errorf("album %s: track artists: %w", id, err)
	}
	artists := make(map[int64][]string, len(tracks))
	for i := range links {
		artists[links[i].TrackID] = append(artists[links[i].TrackID], links[i].Name)
	}

	summary := sqlcdb.ListAlbumsByNameRow(row)
	d := AlbumDetail{AlbumSummary: summarize(&summary), Tracks: make([]Track, 0, len(tracks))}
	for i := range tracks {
		t := &tracks[i]
		tr := Track{
			ID: t.Ulid, Title: t.Title, Artists: artists[t.ID],
			DiscNo: intOr0(t.DiscNo), TrackNo: intOr0(t.TrackNo),
			Duration: time.Duration(t.DurationMs) * time.Millisecond, Format: t.Format,
			SampleRate: intOr0(t.SampleRate), BitDepth: intOr0(t.BitDepth),
			ReplayGain: ReplayGain{
				TrackGain: floatOr0(t.RgTrackGain), TrackPeak: floatOr0(t.RgTrackPeak),
				AlbumGain: floatOr0(t.RgAlbumGain), AlbumPeak: floatOr0(t.RgAlbumPeak),
			},
		}
		d.Tracks = append(d.Tracks, tr)
		d.Duration += tr.Duration
		d.Formats = append(d.Formats, tr.Format)
	}
	slices.Sort(d.Formats)
	d.Formats = slices.Compact(d.Formats)
	return d, nil
}

// TrackFile returns the file of the live track with the public ID id. It
// returns an error wrapping [ErrNotFound] for an unknown ID or a missing track.
func (s *Store) TrackFile(ctx context.Context, id string) (TrackFile, error) {
	row, err := s.rq.GetTrackFile(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return TrackFile{}, fmt.Errorf("track %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return TrackFile{}, fmt.Errorf("track %s: %w", id, err)
	}
	return TrackFile{Root: row.Root, RelPath: row.RelPath, Format: row.Format}, nil
}

// Facts returns the library's counts: albums and tracks that are live, their
// total duration, and the files the scanner could not read.
func (s *Store) Facts(ctx context.Context) (LibraryFacts, error) {
	albums, err := s.rq.CountLiveAlbums(ctx)
	if err != nil {
		return LibraryFacts{}, fmt.Errorf("facts: albums: %w", err)
	}
	tracks, err := s.rq.LiveTrackFacts(ctx)
	if err != nil {
		return LibraryFacts{}, fmt.Errorf("facts: tracks: %w", err)
	}
	issues, err := s.rq.CountScanIssues(ctx)
	if err != nil {
		return LibraryFacts{}, fmt.Errorf("facts: issues: %w", err)
	}
	return LibraryFacts{
		Albums: albums, Tracks: tracks.Tracks, Issues: issues,
		Duration: time.Duration(tracks.DurationMs) * time.Millisecond,
	}, nil
}
