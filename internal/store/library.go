package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// ErrInvalidWrite means a [Batch] was rejected before anything was written.
var ErrInvalidWrite = errors.New("invalid write")

// TrackWrite is one file's metadata, ready to store. Pass it by pointer.
type TrackWrite struct {
	ID   int64  // 0 inserts a new track; otherwise updates that track, moving it to Root/RelPath.
	ULID string // Required when ID is 0; ignored otherwise.

	Root, RelPath string
	Size          int64
	MTime         time.Time // Stored to the millisecond.
	// Digest is the audio fingerprint, or nil when the file was not hashed. A nil
	// Digest on an update keeps the stored one while Size and MTime are unchanged,
	// and clears it otherwise: a fingerprint holds only for the file it was read from.
	Digest *[32]byte

	Format, Codec                           string
	Duration                                time.Duration // Stored to the millisecond.
	SampleRate, BitDepth, Channels, Bitrate int           // 0 is unknown.
	Lossless                                bool

	Title, SortTitle                                           string
	TrackNo, TrackTotal, DiscNo, DiscTotal, Year, OriginalYear int // 0 is unknown.
	Compilation                                                bool
	ReplayGain                                                 ReplayGain

	Album        AlbumWrite
	Artists      []ArtistWrite // In order; may be empty.
	AlbumArtists []ArtistWrite // In order; may be empty.
	Tags         []Tag         // Every raw tag; ord counts per key in slice order.
	Warnings     []string
}

// ReplayGain holds loudness values; a zero field is unknown.
type ReplayGain struct{ TrackGain, TrackPeak, AlbumGain, AlbumPeak float64 }

// AlbumWrite identifies an album by GroupKey and carries its display fields.
// ULID and the first-seen time are kept when the album already exists.
type AlbumWrite struct {
	GroupKey, Name, SortName, MBID string
	ULID                           string // Required, 26 characters; the album's public ID, kept when the album already exists.
	Artist                         string // The display artist.
	Year                           int
	Compilation                    bool
	Artwork                        *ArtworkWrite // Nil keeps the cover the album has.
}

// ArtworkSource says where a cover came from.
type ArtworkSource string

const (
	// ArtworkEmbedded is a picture inside an audio file.
	ArtworkEmbedded ArtworkSource = "embedded"
	// ArtworkFolder is an image file next to the audio.
	ArtworkFolder ArtworkSource = "folder"
)

// ArtworkWrite describes a rendered cover. Hash is the SHA-256 of the source
// image, the key of its renditions on disk; the store holds no image bytes.
type ArtworkWrite struct {
	Hash          [32]byte
	Width, Height int    // Of the source image.
	Accent        string // "#rrggbb".
	Ground        string // "#rrggbb".
	Source        ArtworkSource
}

// ArtistWrite identifies an artist by MatchKey and carries its display fields.
type ArtistWrite struct {
	MatchKey, Name, SortName, MBID string
	ULID                           string // Required, 26 characters; the artist's public ID, kept when the artist already exists.
}

// Tag is one raw tag value.
type Tag struct{ Key, Value string }

// IssueKind classifies a file that could not become a track.
type IssueKind string

const (
	// IssueUnreadable is an I/O or permission error.
	IssueUnreadable IssueKind = "unreadable" // I/O or permission errors.
	// IssueCorrupted is a structurally corrupt file.
	IssueCorrupted IssueKind = "corrupted" // audiometa.CorruptedFileError.
	// IssueUnsupported is a format audiometa does not read.
	IssueUnsupported IssueKind = "unsupported" // audiometa.UnsupportedFormatError.
)

// ScanIssue is a file that could not become a track.
type ScanIssue struct {
	Root, RelPath string
	Kind          IssueKind
	Message       string
	Size          int64
	MTime         time.Time
}

// Batch is one transaction's worth of scan results.
type Batch struct {
	Tracks []TrackWrite
	Issues []ScanIssue
}

// FileState is what the store knows about one path under a root: a track,
// missing or not, or a scan issue.
type FileState struct {
	TrackID int64 // 0 for a scan issue.
	Size    int64
	MTime   time.Time // Millisecond precision: compare with a truncated stat time.
	Missing bool      // The track's missing_since is set.
	Issue   bool      // The path is a scan issue, not a track.
}

// ulidLen is the length of a ULID's text form.
const ulidLen = 26

// FTS statements are hand-written: sqlc can't express writes to an FTS5 rowid.
const (
	ftsDeleteSQL = "DELETE FROM tracks_fts WHERE rowid = ?"
	ftsInsertSQL = "INSERT INTO tracks_fts (rowid, title, artists, album, album_artist) VALUES (?, ?, ?, ?, ?)"
)

// Artist roles in track_artists.
const (
	roleArtist      = "artist"
	roleAlbumArtist = "album_artist"
)

// ms converts a time to Unix milliseconds, the unit of every stored time.
func ms(t time.Time) int64 { return t.UnixMilli() }

// fromMS is the inverse of [ms].
func fromMS(v int64) time.Time { return time.UnixMilli(v).UTC() }

// flag converts a bool to the 0 or 1 that CHECK constraints expect.
func flag(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// nullInt returns nil for 0, which the caller uses for "unknown".
func nullInt(v int) *int64 {
	if v == 0 {
		return nil
	}
	return new(int64(v))
}

// digestBytes is a digest as a BLOB, or nil (NULL) when there is none.
func digestBytes(d *[32]byte) []byte {
	if d == nil {
		return nil
	}
	return d[:]
}

// nullDigest is digestBytes for a parameter sqlc could not type: an untyped nil
// is NULL to every driver.
func nullDigest(d *[32]byte) any {
	if d == nil {
		return nil
	}
	return d[:]
}

// nullFloat returns nil for 0, which audiometa uses for "unknown".
func nullFloat(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}

// nullStr returns nil for the empty string.
func nullStr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// inTx runs fn in a writer transaction, with every sqlc query prepared on it.
// The ncruces driver prepares, steps and finalizes every statement that has
// arguments, so preparing once per transaction saves thousands of prepares per batch.
func (s *Store) inTx(ctx context.Context, fn func(q *sqlcdb.Queries, tx *sql.Tx) error) (err error) {
	tx, err := s.w.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}
	}()
	q, err := sqlcdb.Prepare(ctx, tx)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	if err = fn(q, tx); err != nil {
		return errors.Join(err, q.Close())
	}
	if err = q.Close(); err != nil {
		return fmt.Errorf("close statements: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// validate checks b before any write, so a bad batch writes nothing.
func (b *Batch) validate() error {
	for i := range b.Tracks {
		if err := b.Tracks[i].validate(); err != nil {
			return fmt.Errorf("track %d: %w", i, err)
		}
	}
	for i := range b.Issues {
		if b.Issues[i].Root == "" || b.Issues[i].RelPath == "" {
			return fmt.Errorf("issue %d: empty Root or RelPath: %w", i, ErrInvalidWrite)
		}
	}
	return nil
}

func (tw *TrackWrite) validate() error {
	switch {
	case tw.ID == 0 && len(tw.ULID) != ulidLen:
		return fmt.Errorf("ULID is %d characters, want %d: %w", len(tw.ULID), ulidLen, ErrInvalidWrite)
	case tw.Root == "":
		return fmt.Errorf("empty Root: %w", ErrInvalidWrite)
	case tw.RelPath == "":
		return fmt.Errorf("empty RelPath: %w", ErrInvalidWrite)
	case tw.Album.GroupKey == "":
		return fmt.Errorf("empty Album.GroupKey: %w", ErrInvalidWrite)
	case len(tw.Album.ULID) != ulidLen:
		return fmt.Errorf("Album.ULID is %d characters, want %d: %w", len(tw.Album.ULID), ulidLen, ErrInvalidWrite)
	}
	if art := tw.Album.Artwork; art != nil {
		if err := art.validate(); err != nil {
			return fmt.Errorf("Album.Artwork: %w", err)
		}
	}
	return tw.validateArtists()
}

func (tw *TrackWrite) validateArtists() error {
	for _, list := range [][]ArtistWrite{tw.Artists, tw.AlbumArtists} {
		for i := range list {
			if list[i].MatchKey == "" {
				return fmt.Errorf("empty MatchKey on artist %q: %w", list[i].Name, ErrInvalidWrite)
			}
			if len(list[i].ULID) != ulidLen {
				return fmt.Errorf("ULID of artist %q is %d characters, want %d: %w", list[i].Name, len(list[i].ULID), ulidLen, ErrInvalidWrite)
			}
		}
	}
	return nil
}

func (a *ArtworkWrite) validate() error {
	switch {
	case a.Source != ArtworkEmbedded && a.Source != ArtworkFolder:
		return fmt.Errorf("unknown Source %q: %w", a.Source, ErrInvalidWrite)
	case a.Width <= 0 || a.Height <= 0:
		return fmt.Errorf("size %d x %d: %w", a.Width, a.Height, ErrInvalidWrite)
	case a.Accent == "" || a.Ground == "":
		return fmt.Errorf("empty Accent or Ground: %w", ErrInvalidWrite)
	}
	return nil
}

// ftsStmts are the prepared FTS statements, shared by a whole batch.
type ftsStmts struct{ del, ins *sql.Stmt }

func prepareFTS(ctx context.Context, tx *sql.Tx) (*ftsStmts, error) {
	var f ftsStmts
	var err error
	if f.del, err = tx.PrepareContext(ctx, ftsDeleteSQL); err != nil {
		return nil, fmt.Errorf("prepare fts delete: %w", err)
	}
	if f.ins, err = tx.PrepareContext(ctx, ftsInsertSQL); err != nil {
		return nil, errors.Join(fmt.Errorf("prepare fts insert: %w", err), f.close())
	}
	return &f, nil
}

// close closes whichever statements were prepared.
func (f *ftsStmts) close() error {
	var err error
	if f.del != nil {
		err = f.del.Close()
	}
	if f.ins != nil {
		err = errors.Join(err, f.ins.Close())
	}
	return err
}

// WriteBatch stores b in one transaction: each track with its album, artists,
// tags and search row, then the scan issues. A track with ID 0 is inserted;
// otherwise that track is updated and moved to its Root and RelPath, and any
// missing mark is cleared. A track written at a path clears that path's scan
// issue. An issue at a path that holds a track leaves the track alone.
//
// It returns an error wrapping [ErrInvalidWrite], and writes nothing, if a
// track lacks a 26-character ULID (when inserting), Root, RelPath, an album
// GroupKey, a 26-character album ULID, or an artist MatchKey and 26-character
// ULID. It returns an error wrapping [ErrNotFound] if an update names a track
// that does not exist. Any error rolls the whole batch
// back. An empty batch does nothing.
func (s *Store) WriteBatch(ctx context.Context, b *Batch) error {
	if len(b.Tracks) == 0 && len(b.Issues) == 0 {
		return nil
	}
	if err := b.validate(); err != nil {
		return fmt.Errorf("write batch: %w", err)
	}
	now := s.now()
	err := s.inTx(ctx, func(q *sqlcdb.Queries, tx *sql.Tx) (err error) {
		fts, err := prepareFTS(ctx, tx)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, fts.close()) }()
		for i := range b.Tracks {
			if err = writeTrack(ctx, q, fts, &b.Tracks[i], now); err != nil {
				return fmt.Errorf("track %d: %w", i, err)
			}
		}
		for i := range b.Issues {
			if err = writeIssue(ctx, q, &b.Issues[i], now); err != nil {
				return fmt.Errorf("issue %d: %w", i, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("write batch: %w", err)
	}
	return nil
}

// trackRefs are the rows a track points at.
type trackRefs struct {
	albumID                   int64
	artistIDs, albumArtistIDs []int64
}

// resolveRefs upserts the track's album and artists.
func resolveRefs(ctx context.Context, q *sqlcdb.Queries, tw *TrackWrite, now time.Time) (trackRefs, error) {
	var refs trackRefs
	var err error
	if refs.artistIDs, err = upsertArtists(ctx, q, tw.Artists); err != nil {
		return refs, err
	}
	if refs.albumArtistIDs, err = upsertArtists(ctx, q, tw.AlbumArtists); err != nil {
		return refs, err
	}
	var artworkHash []byte
	if art := tw.Album.Artwork; art != nil {
		if err = q.UpsertArtwork(ctx, sqlcdb.UpsertArtworkParams{
			Hash: art.Hash[:], Width: int64(art.Width), Height: int64(art.Height),
			Accent: art.Accent, Ground: art.Ground, Source: string(art.Source),
		}); err != nil {
			return refs, fmt.Errorf("upsert artwork: %w", err)
		}
		artworkHash = art.Hash[:]
	}
	refs.albumID, err = q.UpsertAlbum(ctx, sqlcdb.UpsertAlbumParams{
		Ulid:        tw.Album.ULID,
		GroupKey:    tw.Album.GroupKey,
		Name:        tw.Album.Name,
		SortName:    tw.Album.SortName,
		Artist:      tw.Album.Artist,
		Mbid:        nullStr(tw.Album.MBID),
		Year:        nullInt(tw.Album.Year),
		Compilation: flag(tw.Album.Compilation),
		ArtworkHash: artworkHash,
		AddedAt:     ms(now),
	})
	if err != nil {
		return refs, fmt.Errorf("upsert album: %w", err)
	}
	return refs, nil
}

// writeTrack stores one track and everything that hangs off it.
func writeTrack(ctx context.Context, q *sqlcdb.Queries, fts *ftsStmts, tw *TrackWrite, now time.Time) error {
	refs, err := resolveRefs(ctx, q, tw, now)
	if err != nil {
		return err
	}
	id, err := saveTrack(ctx, q, tw, refs.albumID, now)
	if err != nil {
		return err
	}
	return writeDependents(ctx, q, fts, id, tw, &refs)
}

// writeDependents writes what hangs off a saved track: tags, artist links, the
// search row, and the clearing of any scan issue at its path.
func writeDependents(ctx context.Context, q *sqlcdb.Queries, fts *ftsStmts, id int64, tw *TrackWrite, refs *trackRefs) error {
	if tw.ID != 0 {
		if err := clearLinks(ctx, q, id); err != nil {
			return err
		}
	}
	if err := writeLinks(ctx, q, id, tw, refs); err != nil {
		return err
	}
	if err := writeFTS(ctx, fts, id, tw); err != nil {
		return err
	}
	if err := q.DeleteScanIssue(ctx, sqlcdb.DeleteScanIssueParams{Root: tw.Root, RelPath: tw.RelPath}); err != nil {
		return fmt.Errorf("clear scan issue: %w", err)
	}
	return nil
}

// clearLinks deletes a track's tags and artist links before an update rewrites them.
func clearLinks(ctx context.Context, q *sqlcdb.Queries, id int64) error {
	if err := q.DeleteTrackTags(ctx, id); err != nil {
		return fmt.Errorf("delete tags: %w", err)
	}
	if err := q.DeleteTrackArtists(ctx, id); err != nil {
		return fmt.Errorf("delete artist links: %w", err)
	}
	return nil
}

// upsertArtists upserts list in order and returns the artist IDs.
func upsertArtists(ctx context.Context, q *sqlcdb.Queries, list []ArtistWrite) ([]int64, error) {
	ids := make([]int64, len(list))
	for i := range list {
		id, err := q.UpsertArtist(ctx, sqlcdb.UpsertArtistParams{
			Ulid:     list[i].ULID,
			MatchKey: list[i].MatchKey,
			Name:     list[i].Name,
			SortName: list[i].SortName,
			Mbid:     nullStr(list[i].MBID),
		})
		if err != nil {
			return nil, fmt.Errorf("upsert artist %q: %w", list[i].Name, err)
		}
		ids[i] = id
	}
	return ids, nil
}

// saveTrack inserts or updates the tracks row and returns the track's ID.
func saveTrack(ctx context.Context, q *sqlcdb.Queries, tw *TrackWrite, albumID int64, now time.Time) (int64, error) {
	warnings := tw.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	encoded, err := json.Marshal(warnings)
	if err != nil {
		return 0, fmt.Errorf("encode warnings: %w", err)
	}
	if tw.ID == 0 {
		return insertTrack(ctx, q, tw, albumID, string(encoded), now)
	}
	return tw.ID, updateTrack(ctx, q, tw, albumID, string(encoded))
}

// insertTrack inserts a new tracks row and returns its ID.
func insertTrack(ctx context.Context, q *sqlcdb.Queries, tw *TrackWrite, albumID int64, warnings string, now time.Time) (int64, error) {
	id, err := q.InsertTrack(ctx, sqlcdb.InsertTrackParams{
		Ulid:         tw.ULID,
		Root:         tw.Root,
		RelPath:      tw.RelPath,
		Size:         tw.Size,
		Mtime:        ms(tw.MTime),
		AudioDigest:  digestBytes(tw.Digest),
		AlbumID:      albumID,
		Format:       tw.Format,
		Codec:        tw.Codec,
		DurationMs:   tw.Duration.Milliseconds(),
		SampleRate:   nullInt(tw.SampleRate),
		BitDepth:     nullInt(tw.BitDepth),
		Channels:     nullInt(tw.Channels),
		Bitrate:      nullInt(tw.Bitrate),
		Lossless:     flag(tw.Lossless),
		Title:        tw.Title,
		SortTitle:    tw.SortTitle,
		TrackNo:      nullInt(tw.TrackNo),
		TrackTotal:   nullInt(tw.TrackTotal),
		DiscNo:       nullInt(tw.DiscNo),
		DiscTotal:    nullInt(tw.DiscTotal),
		Year:         nullInt(tw.Year),
		OriginalYear: nullInt(tw.OriginalYear),
		Compilation:  flag(tw.Compilation),
		RgTrackGain:  nullFloat(tw.ReplayGain.TrackGain),
		RgTrackPeak:  nullFloat(tw.ReplayGain.TrackPeak),
		RgAlbumGain:  nullFloat(tw.ReplayGain.AlbumGain),
		RgAlbumPeak:  nullFloat(tw.ReplayGain.AlbumPeak),
		ScanWarnings: warnings,
		AddedAt:      ms(now),
	})
	if err != nil {
		return 0, fmt.Errorf("insert track %s: %w", tw.RelPath, err)
	}
	return id, nil
}

// updateTrack rewrites an existing tracks row, keeping its ULID and added_at.
func updateTrack(ctx context.Context, q *sqlcdb.Queries, tw *TrackWrite, albumID int64, warnings string) error {
	n, err := q.UpdateTrack(ctx, sqlcdb.UpdateTrackParams{
		Root:         tw.Root,
		RelPath:      tw.RelPath,
		Size:         tw.Size,
		Mtime:        ms(tw.MTime),
		AudioDigest:  nullDigest(tw.Digest),
		AlbumID:      albumID,
		Format:       tw.Format,
		Codec:        tw.Codec,
		DurationMs:   tw.Duration.Milliseconds(),
		SampleRate:   nullInt(tw.SampleRate),
		BitDepth:     nullInt(tw.BitDepth),
		Channels:     nullInt(tw.Channels),
		Bitrate:      nullInt(tw.Bitrate),
		Lossless:     flag(tw.Lossless),
		Title:        tw.Title,
		SortTitle:    tw.SortTitle,
		TrackNo:      nullInt(tw.TrackNo),
		TrackTotal:   nullInt(tw.TrackTotal),
		DiscNo:       nullInt(tw.DiscNo),
		DiscTotal:    nullInt(tw.DiscTotal),
		Year:         nullInt(tw.Year),
		OriginalYear: nullInt(tw.OriginalYear),
		Compilation:  flag(tw.Compilation),
		RgTrackGain:  nullFloat(tw.ReplayGain.TrackGain),
		RgTrackPeak:  nullFloat(tw.ReplayGain.TrackPeak),
		RgAlbumGain:  nullFloat(tw.ReplayGain.AlbumGain),
		RgAlbumPeak:  nullFloat(tw.ReplayGain.AlbumPeak),
		ScanWarnings: warnings,
		ID:           tw.ID,
	})
	if err != nil {
		return fmt.Errorf("update track %d: %w", tw.ID, err)
	}
	if n == 0 {
		return fmt.Errorf("track %d: %w", tw.ID, ErrNotFound)
	}
	return nil
}

// writeLinks inserts the track's tags and artist links. Tag ord counts per
// key, artist ord per role, both in slice order.
func writeLinks(ctx context.Context, q *sqlcdb.Queries, id int64, tw *TrackWrite, refs *trackRefs) error {
	ords := make(map[string]int64, len(tw.Tags))
	for _, tag := range tw.Tags {
		ord := ords[tag.Key]
		ords[tag.Key]++
		err := q.InsertTrackTag(ctx, sqlcdb.InsertTrackTagParams{TrackID: id, Key: tag.Key, Ord: ord, Value: tag.Value})
		if err != nil {
			return fmt.Errorf("insert tag %s: %w", tag.Key, err)
		}
	}
	if err := linkArtists(ctx, q, id, roleArtist, refs.artistIDs); err != nil {
		return err
	}
	return linkArtists(ctx, q, id, roleAlbumArtist, refs.albumArtistIDs)
}

// linkArtists inserts one role's artist links, ord counting from 0.
func linkArtists(ctx context.Context, q *sqlcdb.Queries, id int64, role string, artistIDs []int64) error {
	for ord, artistID := range artistIDs {
		err := q.InsertTrackArtist(ctx, sqlcdb.InsertTrackArtistParams{
			TrackID: id, Role: role, Ord: int64(ord), ArtistID: artistID,
		})
		if err != nil {
			return fmt.Errorf("insert %s link: %w", role, err)
		}
	}
	return nil
}

// writeFTS replaces the track's search row.
func writeFTS(ctx context.Context, fts *ftsStmts, id int64, tw *TrackWrite) error {
	if tw.ID != 0 {
		if _, err := fts.del.ExecContext(ctx, id); err != nil {
			return fmt.Errorf("delete fts row: %w", err)
		}
	}
	_, err := fts.ins.ExecContext(ctx, id,
		foldSearch(tw.Title),
		foldSearch(joinNames(tw.Artists)),
		foldSearch(tw.Album.Name),
		foldSearch(joinNames(tw.AlbumArtists)),
	)
	if err != nil {
		return fmt.Errorf("insert fts row: %w", err)
	}
	return nil
}

// joinNames joins artist names with a single space.
func joinNames(list []ArtistWrite) string {
	names := make([]string, len(list))
	for i := range list {
		names[i] = list[i].Name
	}
	return strings.Join(names, " ")
}

// writeIssue upserts one scan issue, leaving any track at the path alone.
func writeIssue(ctx context.Context, q *sqlcdb.Queries, is *ScanIssue, now time.Time) error {
	err := q.UpsertScanIssue(ctx, sqlcdb.UpsertScanIssueParams{
		Root:    is.Root,
		RelPath: is.RelPath,
		Kind:    string(is.Kind),
		Message: is.Message,
		Size:    is.Size,
		Mtime:   ms(is.MTime),
		SeenAt:  ms(now),
	})
	if err != nil {
		return fmt.Errorf("upsert scan issue %s: %w", is.RelPath, err)
	}
	return nil
}

// FileStates returns what the store knows about every path under root, keyed
// by relative path. A path that is both a track and a scan issue (a once-good
// file that went bad) has TrackID set, Issue true, and the issue's size and
// mtime, so a quick scan skips the still-broken file.
func (s *Store) FileStates(ctx context.Context, root string) (map[string]FileState, error) {
	tracks, err := s.rq.TrackStates(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("file states: tracks: %w", err)
	}
	issues, err := s.rq.IssueStates(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("file states: issues: %w", err)
	}
	out := make(map[string]FileState, len(tracks)+len(issues))
	for i := range tracks {
		t := &tracks[i]
		out[t.RelPath] = FileState{TrackID: t.ID, Size: t.Size, MTime: fromMS(t.Mtime), Missing: t.Missing}
	}
	for i := range issues {
		is := &issues[i]
		st := out[is.RelPath]
		st.Size, st.MTime, st.Issue = is.Size, fromMS(is.Mtime), true
		out[is.RelPath] = st
	}
	return out, nil
}

// MissingTrack is a missing track that a new file might be.
type MissingTrack struct {
	ID            int64
	Root, RelPath string
}

// Pruned counts what PruneOrphans deleted, and lists the artwork rows it
// dropped so the caller can delete their files.
type Pruned struct {
	Albums, Artists int64
	Artwork         [][32]byte
}

// MissingByDigest returns the missing tracks whose audio digest is digest,
// ordered by ID. An unknown digest gives an empty slice.
func (s *Store) MissingByDigest(ctx context.Context, digest [32]byte) ([]MissingTrack, error) {
	rows, err := s.rq.MissingByDigest(ctx, digest[:])
	if err != nil {
		return nil, fmt.Errorf("missing by digest: %w", err)
	}
	out := make([]MissingTrack, len(rows))
	for i := range rows {
		out[i] = MissingTrack{ID: rows[i].ID, Root: rows[i].Root, RelPath: rows[i].RelPath}
	}
	return out, nil
}

// MarkMissing marks the tracks missing as of at. A track that is already
// missing keeps its original time. Unknown IDs are ignored.
func (s *Store) MarkMissing(ctx context.Context, trackIDs []int64, at time.Time) error {
	if len(trackIDs) == 0 {
		return nil
	}
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		for _, id := range trackIDs {
			if err := q.MarkMissing(ctx, sqlcdb.MarkMissingParams{MissingSince: new(ms(at)), ID: id}); err != nil {
				return fmt.Errorf("track %d: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("mark missing: %w", err)
	}
	return nil
}

// DeleteScanIssues removes the scan issues at relPaths under root. Unknown
// paths are ignored.
func (s *Store) DeleteScanIssues(ctx context.Context, root string, relPaths []string) error {
	if len(relPaths) == 0 {
		return nil
	}
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		for _, p := range relPaths {
			if err := q.DeleteScanIssue(ctx, sqlcdb.DeleteScanIssueParams{Root: root, RelPath: p}); err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete scan issues: %w", err)
	}
	return nil
}

// ftsDeleteMissingSQL is hand-written for the same reason as the other FTS
// statements: sqlc can't express an FTS5 rowid write.
const ftsDeleteMissingSQL = "DELETE FROM tracks_fts WHERE rowid IN (SELECT id FROM tracks WHERE missing_since < ?)"

// DeleteMissingBefore deletes the tracks that have been missing since before
// cutoff, with their tags, artist links and search rows, and returns how many
// it deleted.
func (s *Store) DeleteMissingBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	var n int64
	err := s.inTx(ctx, func(q *sqlcdb.Queries, tx *sql.Tx) error {
		if _, ftsErr := tx.ExecContext(ctx, ftsDeleteMissingSQL, ms(cutoff)); ftsErr != nil {
			return fmt.Errorf("delete fts rows: %w", ftsErr)
		}
		deleted, delErr := q.DeleteMissingBefore(ctx, new(ms(cutoff)))
		if delErr != nil {
			return fmt.Errorf("delete tracks: %w", delErr)
		}
		n = deleted
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("delete missing before %s: %w", cutoff.Format(time.RFC3339), err)
	}
	return n, nil
}

// PruneOrphans deletes the albums that no track uses, then the artwork rows no
// album uses, and the artists that no track links to. It returns how many
// albums and artists went, and the hashes of the artwork, so the caller can
// delete their files.
func (s *Store) PruneOrphans(ctx context.Context) (Pruned, error) {
	var p Pruned
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		albums, albumsErr := q.PruneAlbums(ctx)
		if albumsErr != nil {
			return fmt.Errorf("albums: %w", albumsErr)
		}
		hashes, artworkErr := q.PruneArtwork(ctx)
		if artworkErr != nil {
			return fmt.Errorf("artwork: %w", artworkErr)
		}
		artists, artistsErr := q.PruneArtists(ctx)
		if artistsErr != nil {
			return fmt.Errorf("artists: %w", artistsErr)
		}
		p = Pruned{Albums: albums, Artists: artists}
		for _, h := range hashes {
			p.Artwork = append(p.Artwork, [32]byte(h))
		}
		return nil
	})
	if err != nil {
		return Pruned{}, fmt.Errorf("prune orphans: %w", err)
	}
	return p, nil
}

// KnownRoots returns every root that has a track or a scan issue.
func (s *Store) KnownRoots(ctx context.Context) ([]string, error) {
	roots, err := s.rq.KnownRoots(ctx)
	if err != nil {
		return nil, fmt.Errorf("known roots: %w", err)
	}
	return roots, nil
}
