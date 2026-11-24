package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// ArtistImageKind is what an artist's image is for.
type ArtistImageKind string

// The kinds of artist image, one of each per artist.
const (
	ArtistThumb      ArtistImageKind = "thumb"
	ArtistBackground ArtistImageKind = "background"
	ArtistLogo       ArtistImageKind = "logo"
	ArtistBanner     ArtistImageKind = "banner"
)

// AlbumImageKind is what an album's extra image is for. The album's own cover
// is not one: it is the album's.
type AlbumImageKind string

// The kinds of album extra.
const (
	AlbumDisc AlbumImageKind = "disc" // One per disc, and one for every disc.
	AlbumBack AlbumImageKind = "back"
)

// AllDiscs is the disc of a disc image for every disc of its album.
const AllDiscs = 0

// FileRef is a file under a library root as a scan read it.
type FileRef struct {
	Root, RelPath string
	Size          int64
	MTime         time.Time // Compared to the millisecond.
}

// Same reports whether f and o are one file, unchanged: the same path, size
// and mtime to the millisecond.
func (f *FileRef) Same(o *FileRef) bool {
	return f != nil && o != nil && f.Root == o.Root && f.RelPath == o.RelPath &&
		f.Size == o.Size && f.MTime.UnixMilli() == o.MTime.UnixMilli()
}

// LayoutTrack is a live track's file and its album.
type LayoutTrack struct {
	AlbumID       int64
	AlbumULID     string
	Root, RelPath string
}

// LayoutArtist is an album's owner, as a scan looking for its folder needs it.
type LayoutArtist struct {
	ID   int64
	ULID string
	MBID string // Empty when unknown.
}

// ArtistImageKey names an artist's image of one kind.
type ArtistImageKey struct {
	ArtistID int64
	Kind     ArtistImageKind
}

// AlbumImageKey names an album's extra of one kind and disc.
type AlbumImageKey struct {
	AlbumID int64
	Kind    AlbumImageKind
	Disc    int
}

// StoredExtra is what the store holds of an image or a bio: whether an
// administrator pinned it, and the file it was read from, when it came from a
// folder.
type StoredExtra struct {
	Pinned bool
	File   *FileRef // Nil for any source but a folder or an artist.nfo.
	Hash   [32]byte // An image's; zero for a bio.
}

// Layout is what a scan needs to find album and artist folders and to tell
// which of their files it has read already.
type Layout struct {
	Tracks       []LayoutTrack            // By album, then file.
	Owners       map[int64][]LayoutArtist // By album: its owners, Various Artists left out.
	ArtistImages map[ArtistImageKey]StoredExtra
	AlbumImages  map[AlbumImageKey]StoredExtra
	Bios         map[int64]StoredExtra // By artist.
}

// FolderLayout returns the layout of the whole library, read in one
// transaction. Its tracks and owners are kept between library writes, as the
// index's rows are: a scan that changed no song reads only what the store holds
// of images and bios.
func (s *Store) FolderLayout(ctx context.Context) (*Layout, error) {
	mark := s.library.before()
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("folder layout: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Nothing was written: there is nothing to undo.
	q := s.rq.WithTx(tx)
	l := &Layout{
		Owners:       map[int64][]LayoutArtist{},
		ArtistImages: map[ArtistImageKey]StoredExtra{},
		AlbumImages:  map[AlbumImageKey]StoredExtra{},
		Bios:         map[int64]StoredExtra{},
	}
	// The first read takes the snapshot, which the kept rows must come after.
	if err = l.readStored(ctx, q); err != nil {
		return nil, fmt.Errorf("folder layout: %w", err)
	}
	if err = l.readTracks(ctx, &s.library, mark, q); err != nil {
		return nil, fmt.Errorf("folder layout: %w", err)
	}
	return l, nil
}

// readTracks reads the live tracks and the albums' owners, from the rows kept
// where it can.
func (l *Layout) readTracks(ctx context.Context, kept *libraryRows, mark writesMark, q *sqlcdb.Queries) error {
	tracks, err := read(kept, &kept.layoutTracks, mark, "", func() ([]sqlcdb.LayoutTracksRow, error) { return q.LayoutTracks(ctx) })
	if err != nil {
		return fmt.Errorf("tracks: %w", err)
	}
	l.Tracks = make([]LayoutTrack, len(tracks))
	for i := range tracks {
		t := &tracks[i]
		l.Tracks[i] = LayoutTrack{AlbumID: t.AlbumID, AlbumULID: t.AlbumUlid, Root: t.Root, RelPath: t.RelPath}
	}
	owners, err := read(kept, &kept.layoutOwners, mark, "", func() ([]sqlcdb.LayoutOwnersRow, error) { return q.LayoutOwners(ctx) })
	if err != nil {
		return fmt.Errorf("owners: %w", err)
	}
	for i := range owners {
		o := &owners[i]
		if isVarious(o.MatchKey, o.Mbid) {
			continue
		}
		l.Owners[o.AlbumID] = append(l.Owners[o.AlbumID], LayoutArtist{ID: o.ArtistID, ULID: o.Ulid, MBID: strOrEmpty(o.Mbid)})
	}
	return nil
}

// readStored reads what the store holds of images and bios.
func (l *Layout) readStored(ctx context.Context, q *sqlcdb.Queries) error {
	artists, err := q.StoredArtistImages(ctx)
	if err != nil {
		return fmt.Errorf("artist images: %w", err)
	}
	for i := range artists {
		a := &artists[i]
		e := storedExtra(a.Source, a.FileRoot, a.FilePath, a.FileSize, a.FileMtime)
		e.Hash = [32]byte(a.ArtworkHash)
		l.ArtistImages[ArtistImageKey{ArtistID: a.ArtistID, Kind: ArtistImageKind(a.Kind)}] = e
	}
	albums, err := q.StoredAlbumImages(ctx)
	if err != nil {
		return fmt.Errorf("album images: %w", err)
	}
	for i := range albums {
		a := &albums[i]
		key := AlbumImageKey{AlbumID: a.AlbumID, Kind: AlbumImageKind(a.Kind), Disc: int(a.Disc)}
		e := storedExtra(a.Source, a.FileRoot, a.FilePath, a.FileSize, a.FileMtime)
		e.Hash = [32]byte(a.ArtworkHash)
		l.AlbumImages[key] = e
	}
	bios, err := q.StoredBios(ctx)
	if err != nil {
		return fmt.Errorf("bios: %w", err)
	}
	for i := range bios {
		b := &bios[i]
		l.Bios[b.ArtistID] = storedExtra(b.Source, b.FileRoot, b.FilePath, b.FileSize, b.FileMtime)
	}
	return nil
}

// sourcePinned is the source of an administrator's pin.
const sourcePinned = "pinned"

func storedExtra(source string, root, path *string, size, mtime *int64) StoredExtra {
	e := StoredExtra{Pinned: source == sourcePinned}
	if root != nil && path != nil && size != nil && mtime != nil {
		e.File = &FileRef{Root: *root, RelPath: *path, Size: *size, MTime: fromMS(*mtime)}
	}
	return e
}

// ArtistImageWrite sets an artist's image of one kind from a folder's file, or
// with a nil Image takes away the one a folder gave it. Neither touches a pin.
type ArtistImageWrite struct {
	ArtistID int64
	Kind     ArtistImageKind
	Image    *ArtworkWrite
	File     FileRef // The file read; ignored with a nil Image.
}

// AlbumImageWrite is the same for an album's extra.
type AlbumImageWrite struct {
	AlbumID   int64
	AlbumULID string // Noted as changed once the write commits.
	Kind      AlbumImageKind
	Disc      int // [AllDiscs], or a disc number.
	Image     *ArtworkWrite
	File      FileRef
}

// NfoBio is a bio read from an artist.nfo, as plain text.
type NfoBio struct {
	Summary, Body string
}

// BioWrite sets an artist's bio from its artist.nfo, or with a nil Bio takes
// away the one an artist.nfo gave it.
type BioWrite struct {
	ArtistID int64
	Bio      *NfoBio
	File     FileRef
}

// ExtrasWrite is what a scan found in the library's artist and album folders.
type ExtrasWrite struct {
	ArtistImages []ArtistImageWrite
	AlbumImages  []AlbumImageWrite
	Bios         []BioWrite
}

// Empty reports whether w writes nothing.
func (w *ExtrasWrite) Empty() bool {
	return len(w.ArtistImages) == 0 && len(w.AlbumImages) == 0 && len(w.Bios) == 0
}

// WriteExtras stores w in one transaction. It returns the artists whose images
// or bio it changed, by row ID, and notes the albums whose extras it changed
// for [Store.TakeChangedAlbums]. A write that meets a pin changes nothing.
func (s *Store) WriteExtras(ctx context.Context, w *ExtrasWrite, now time.Time) ([]int64, error) {
	var artists []int64
	var albums []string
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		var err error
		// The writer may retry a transaction, so each try starts empty.
		if artists, err = writeEach(w.ArtistImages, func(a *ArtistImageWrite) (int64, int64, error) {
			n, werr := writeArtistImage(ctx, q, a, now)
			return a.ArtistID, n, werr
		}); err != nil {
			return err
		}
		if albums, err = writeEach(w.AlbumImages, func(a *AlbumImageWrite) (string, int64, error) {
			n, werr := writeAlbumImage(ctx, q, a, now)
			return a.AlbumULID, n, werr
		}); err != nil {
			return err
		}
		bios, err := writeEach(w.Bios, func(b *BioWrite) (int64, int64, error) {
			n, werr := writeBio(ctx, q, b, now)
			return b.ArtistID, n, werr
		})
		artists = append(artists, bios...)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("write extras: %w", err)
	}
	s.changed.add(albums)
	return artists, nil
}

// writeEach writes each of ws and returns the ids of those that changed a row.
func writeEach[W, ID any](ws []W, write func(*W) (ID, int64, error)) ([]ID, error) {
	var changed []ID
	for i := range ws {
		id, n, err := write(&ws[i])
		if err != nil {
			return nil, err
		}
		if n > 0 {
			changed = append(changed, id)
		}
	}
	return changed, nil
}

// fileCols are a file's columns.
type fileCols struct {
	root, path  *string
	size, mtime *int64
}

func fileArgs(f *FileRef) fileCols {
	return fileCols{root: new(f.Root), path: new(f.RelPath), size: new(f.Size), mtime: new(ms(f.MTime))}
}

func upsertImage(ctx context.Context, q *sqlcdb.Queries, art *ArtworkWrite) error {
	err := q.UpsertArtwork(ctx, sqlcdb.UpsertArtworkParams{
		Hash: art.Hash[:], Width: int64(art.Width), Height: int64(art.Height),
		Accent: art.Accent, Ground: art.Ground, Source: string(art.Source), Alpha: flag(art.Alpha),
	})
	if err != nil {
		return fmt.Errorf("upsert artwork: %w", err)
	}
	return nil
}

func writeArtistImage(ctx context.Context, q *sqlcdb.Queries, w *ArtistImageWrite, now time.Time) (int64, error) {
	if w.Image == nil {
		n, err := q.DeleteFolderArtistImage(ctx, sqlcdb.DeleteFolderArtistImageParams{ArtistID: w.ArtistID, Kind: string(w.Kind)})
		if err == nil && n > 0 {
			err = askAgainFor(ctx, q, w.ArtistID, now)
		}
		if err != nil {
			return 0, fmt.Errorf("artist %d %s: %w", w.ArtistID, w.Kind, err)
		}
		return n, nil
	}
	if err := upsertImage(ctx, q, w.Image); err != nil {
		return 0, fmt.Errorf("artist %d %s: %w", w.ArtistID, w.Kind, err)
	}
	f := fileArgs(&w.File)
	n, err := q.UpsertFolderArtistImage(ctx, sqlcdb.UpsertFolderArtistImageParams{
		ArtistID: w.ArtistID, Kind: string(w.Kind), ArtworkHash: w.Image.Hash[:],
		FileRoot: f.root, FilePath: f.path, FileSize: f.size, FileMtime: f.mtime, FoundAt: ms(now),
	})
	if err != nil {
		return 0, fmt.Errorf("artist %d %s: %w", w.ArtistID, w.Kind, err)
	}
	return n, nil
}

func writeAlbumImage(ctx context.Context, q *sqlcdb.Queries, w *AlbumImageWrite, now time.Time) (int64, error) {
	disc := int64(w.Disc)
	if w.Image == nil {
		n, err := q.DeleteFolderAlbumImage(ctx, sqlcdb.DeleteFolderAlbumImageParams{AlbumID: w.AlbumID, Kind: string(w.Kind), Disc: disc})
		if err != nil {
			return 0, fmt.Errorf("album %d %s %d: %w", w.AlbumID, w.Kind, w.Disc, err)
		}
		return n, nil
	}
	if err := upsertImage(ctx, q, w.Image); err != nil {
		return 0, fmt.Errorf("album %d %s %d: %w", w.AlbumID, w.Kind, w.Disc, err)
	}
	f := fileArgs(&w.File)
	n, err := q.UpsertFolderAlbumImage(ctx, sqlcdb.UpsertFolderAlbumImageParams{
		AlbumID: w.AlbumID, Kind: string(w.Kind), Disc: disc, ArtworkHash: w.Image.Hash[:],
		FileRoot: f.root, FilePath: f.path, FileSize: f.size, FileMtime: f.mtime, FoundAt: ms(now),
	})
	if err != nil {
		return 0, fmt.Errorf("album %d %s %d: %w", w.AlbumID, w.Kind, w.Disc, err)
	}
	return n, nil
}

func writeBio(ctx context.Context, q *sqlcdb.Queries, w *BioWrite, now time.Time) (int64, error) {
	if w.Bio == nil {
		n, err := q.DeleteNfoBio(ctx, w.ArtistID)
		if err == nil && n > 0 {
			err = askAgainFor(ctx, q, w.ArtistID, now)
		}
		if err != nil {
			return 0, fmt.Errorf("artist %d bio: %w", w.ArtistID, err)
		}
		return n, nil
	}
	f := fileArgs(&w.File)
	n, err := q.UpsertNfoBio(ctx, sqlcdb.UpsertNfoBioParams{
		ArtistID: w.ArtistID, Summary: w.Bio.Summary, Body: w.Bio.Body,
		FileRoot: f.root, FilePath: f.path, FileSize: f.size, FileMtime: f.mtime, FetchedAt: ms(now),
	})
	if err != nil {
		return 0, fmt.Errorf("artist %d bio: %w", w.ArtistID, err)
	}
	return n, nil
}

// askAgainFor brings every source's next ask about the artist forward: its
// folder gave up an image or a bio, which a source may have.
func askAgainFor(ctx context.Context, q *sqlcdb.Queries, artistID int64, now time.Time) error {
	if err := q.AskAgainForArtist(ctx, sqlcdb.AskAgainForArtistParams{Now: ms(now), ArtistID: artistID}); err != nil {
		return fmt.Errorf("ask again: %w", err)
	}
	return nil
}

// Picture is a stored image as a page shows it: a [Cover], and whether its
// renditions keep transparency.
type Picture struct {
	Cover

	Alpha bool
}

// Credit is what a source asks to be said of an image it gave. Every field may
// be empty.
type Credit struct {
	Author     string `json:"author,omitzero"`
	License    string `json:"license,omitzero"`
	LicenseURL string `json:"licenseUrl,omitzero"`
	PageURL    string `json:"pageUrl,omitzero"`
}

// ExtraImage is an artist's image or an album's extra, with where it came from.
type ExtraImage struct {
	Picture

	Source string  // "folder", or a later source's name.
	Credit *Credit // Nil when the source asks for none, as a folder's file.
}

// ArtistImages are an artist's chosen images, each nil when it has none.
type ArtistImages struct {
	Thumb, Background, Logo, Banner *ExtraImage
}

// Bio is an artist's chosen bio, as plain text.
type Bio struct {
	Source     string // "nfo", or a later source's name.
	Lang       string // A two-letter code; empty when the source doesn't say.
	Summary    string
	Body       string // Paragraphs split by blank lines.
	URL        string // The page it came from; empty for an artist.nfo.
	License    string
	LicenseURL string
}

// DiscImage is one disc's art, or every disc's with Disc [AllDiscs].
type DiscImage struct {
	ExtraImage

	Disc int
}

// AlbumExtras are an album's extra images.
type AlbumExtras struct {
	Discs []DiscImage // [AllDiscs] first, then by disc.
	Back  *ExtraImage
}

// picture builds a Picture from an image's columns.
func picture(hash []byte, width, height int64, accent, ground string, alpha int64) Picture {
	return Picture{
		Hash: [32]byte(hash), Accent: accent, Ground: ground, Width: int(width), Height: int(height),
		Alpha: alpha != 0,
	}
}

// credit decodes a stored credit, which a bad value leaves nil: a credit is a
// courtesy, never a reason to fail a page.
func credit(raw *string) *Credit {
	if raw == nil {
		return nil
	}
	var c Credit
	if json.Unmarshal([]byte(*raw), &c) != nil {
		return nil
	}
	return &c
}

// artistArt reads an artist's images and bio.
func (s *Store) artistArt(ctx context.Context, artistID int64) (ArtistImages, *Bio, error) {
	var imgs ArtistImages
	rows, err := s.rq.ListArtistImages(ctx, artistID)
	if err != nil {
		return imgs, nil, fmt.Errorf("images: %w", err)
	}
	for i := range rows {
		r := &rows[i]
		img := &ExtraImage{
			Picture: picture(r.Hash, r.Width, r.Height, r.Accent, r.Ground, r.Alpha),
			Source:  r.Source, Credit: credit(r.Credit),
		}
		switch ArtistImageKind(r.Kind) {
		case ArtistThumb:
			imgs.Thumb = img
		case ArtistBackground:
			imgs.Background = img
		case ArtistLogo:
			imgs.Logo = img
		case ArtistBanner:
			imgs.Banner = img
		}
	}
	b, err := s.rq.GetArtistBio(ctx, artistID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return imgs, nil, nil
	case err != nil:
		return imgs, nil, fmt.Errorf("bio: %w", err)
	}
	return imgs, &Bio{
		Source: b.Source, Lang: strOrEmpty(b.Lang), Summary: b.Summary, Body: b.Body,
		URL: strOrEmpty(b.Url), License: strOrEmpty(b.License), LicenseURL: strOrEmpty(b.LicenseUrl),
	}, nil
}

// artistThumbs returns every artist's thumb, by artist row.
func artistThumbs(ctx context.Context, q *sqlcdb.Queries) (map[int64]*Picture, error) {
	rows, err := q.ListArtistThumbs(ctx)
	if err != nil {
		return nil, fmt.Errorf("thumbs: %w", err)
	}
	out := make(map[int64]*Picture, len(rows))
	for i := range rows {
		r := &rows[i]
		p := picture(r.Hash, r.Width, r.Height, r.Accent, r.Ground, r.Alpha)
		out[r.ArtistID] = &p
	}
	return out, nil
}

// albumExtras reads an album's extras.
func (s *Store) albumExtras(ctx context.Context, albumID int64) (AlbumExtras, error) {
	var ex AlbumExtras
	rows, err := s.rq.ListAlbumImages(ctx, albumID)
	if err != nil {
		return ex, fmt.Errorf("extras: %w", err)
	}
	for i := range rows {
		r := &rows[i]
		img := ExtraImage{
			Picture: picture(r.Hash, r.Width, r.Height, r.Accent, r.Ground, r.Alpha),
			Source:  r.Source, Credit: credit(r.Credit),
		}
		switch AlbumImageKind(r.Kind) {
		case AlbumDisc:
			ex.Discs = append(ex.Discs, DiscImage{Disc: int(r.Disc), ExtraImage: img})
		case AlbumBack:
			ex.Back = &img
		}
	}
	return ex, nil
}

// ExtraFiles returns every file under a root that a folder's image or an
// artist.nfo's bio was read from, with only the root and the path set.
func (s *Store) ExtraFiles(ctx context.Context) ([]FileRef, error) {
	rows, err := s.rq.ListExtraFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("extra files: %w", err)
	}
	out := make([]FileRef, 0, len(rows))
	for i := range rows {
		if r := &rows[i]; r.FileRoot != nil && r.FilePath != nil {
			out = append(out, FileRef{Root: *r.FileRoot, RelPath: *r.FilePath})
		}
	}
	return out, nil
}
