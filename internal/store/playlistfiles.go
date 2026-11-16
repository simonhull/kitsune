package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/simonhull/kitsune/internal/m3u"
	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// A file playlist is read from a .m3u or .m3u8 file in the library. Everyone
// sees it and nobody owns it: it has no user, its source is [PlaylistFromFile],
// and every write a person can make to a playlist is [ErrFilePlaylist], since it
// belongs to its file. The scanner alone writes one ([Store.WriteFilePlaylist])
// and deletes one ([Store.DeleteFilePlaylists]).
//
// Its entries are playlist_tracks rows, as a regular playlist's are, in the
// file's order with its repeats; each entry's position is the number of the line
// that named it, and keeps the line as written. Every read is through live
// tracks, so a song in the Trash drops out and comes back with Restore.

// PlaylistFromFile is the source of a playlist read from a file in the library.
const PlaylistFromFile = "file"

// ErrFilePlaylist means a change was asked of a file playlist, which belongs to
// its file.
var ErrFilePlaylist = errors.New("a file playlist belongs to its file")

// Why a [MissingLine] found no song.
const (
	MissingNoSong  = "noSong"  // No song at the path.
	MissingOutside = "outside" // An absolute path in no root, or one climbing out of its own.
	MissingNotFile = "notFile" // A web address.
	MissingTrashed = "trashed" // The song it names is in the Trash.
)

// MissingLine is a line of a playlist file that found no song. Text is what may
// be shown of it: never an absolute path past its last two segments.
type MissingLine struct {
	Line           int    `json:"line"`
	Text           string `json:"text"`
	Title          string `json:"title,omitzero"`
	Seconds        int    `json:"seconds,omitzero"`
	Reason         string `json:"reason"`
	OtherExtension string `json:"otherExtension,omitzero"`
}

// PlaylistFile is where a file playlist comes from.
type PlaylistFile struct {
	RelPath    string // Relative to its root, which no caller is told.
	ModifiedAt time.Time
	ReadAt     time.Time
	PastLimit  int
	// NotFound counts the lines that found no song when the file was read,
	// the entries whose song has left the library since, and PastLimit.
	NotFound int
}

// FilePlaylistState is what the store knows of a playlist file.
type FilePlaylistState struct {
	Row      int64  // The playlist's row.
	ID       string // The playlist's ULID.
	Size     int64
	MTime    time.Time
	Reader   int
	NotFound int // Lines that found no song when it was read, those past the limit included.
}

// FilePlaylistStates returns what the store knows of every playlist file under
// root, by relative path.
func (s *Store) FilePlaylistStates(ctx context.Context, root string) (map[string]FilePlaylistState, error) {
	rows, err := s.rq.FilePlaylistStates(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("file playlist states: %w", err)
	}
	out := make(map[string]FilePlaylistState, len(rows))
	for i := range rows {
		r := &rows[i]
		out[r.RelPath] = FilePlaylistState{
			Row: r.PlaylistID, ID: r.Ulid, Size: r.Size, MTime: fromMS(r.MtimeMs), Reader: int(r.Reader), NotFound: int(r.NotFound),
		}
	}
	return out, nil
}

// FilePlaylistRoots returns every root a file playlist is read from.
func (s *Store) FilePlaylistRoots(ctx context.Context) ([]string, error) {
	roots, err := s.rq.FilePlaylistRoots(ctx)
	if err != nil {
		return nil, fmt.Errorf("file playlist roots: %w", err)
	}
	return roots, nil
}

// TrackPath is a track as a playlist file's line can name it.
type TrackPath struct {
	Row     int64
	Root    string
	RelPath string // Where its file goes back to while it is in the Trash.
	Live    bool
}

// TrackPaths returns every track's path, live or not, with where a trashed
// one's file goes back to.
func (s *Store) TrackPaths(ctx context.Context) ([]TrackPath, error) {
	rows, err := s.rq.TrackPaths(ctx)
	if err != nil {
		return nil, fmt.Errorf("track paths: %w", err)
	}
	out := make([]TrackPath, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = TrackPath{Row: r.ID, Root: r.Root, RelPath: r.RelPath, Live: r.Live != 0}
	}
	return out, nil
}

// FileEntryLines returns the lines of the file playlist row's last read that
// found a song still live, each with that song's row: what keeps a song that
// moved when its file is read again.
func (s *Store) FileEntryLines(ctx context.Context, row int64) (map[string]int64, error) {
	rows, err := s.rq.ListFileEntryLines(ctx, row)
	if err != nil {
		return nil, fmt.Errorf("file entry lines: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Line] = r.TrackID
	}
	return out, nil
}

// FileEntry is one song a playlist file names.
type FileEntry struct {
	Line  int    // The line's number in the file, from 1.
	Text  string // The line as written.
	Track int64  // The track's row.
}

// FilePlaylistWrite is a read of a playlist file.
type FilePlaylistWrite struct {
	Root, RelPath string
	Size          int64
	MTime         time.Time
	Reader        int    // The version of the reader that read it.
	NewID         string // The ULID a playlist new to the store takes.
	Name          string // Already a name [PlaylistName] passes.
	Lines         int    // The paths it named, those past the limit left out.
	PastLimit     int
	Entries       []FileEntry // In the file's order.
	Missing       []MissingLine
}

// WriteFilePlaylist stores a read of a playlist file, in one transaction: a
// file new to the store becomes a playlist with w.NewID, and a known one keeps
// its ULID, takes w's name and has its entries replaced. It returns the
// playlist's ULID.
func (s *Store) WriteFilePlaylist(ctx context.Context, w *FilePlaylistWrite) (string, error) {
	encoded, err := encodeFileRead(w)
	if err != nil {
		return "", fmt.Errorf("write file playlist: %w", err)
	}
	var id string
	err = s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		at := ms(s.now())
		p, rowErr := fileRow(ctx, q, w, at)
		if rowErr != nil {
			return rowErr
		}
		id = p.id
		if entriesErr := q.InsertFileEntries(ctx, sqlcdb.InsertFileEntriesParams{PlaylistID: p.row, AddedAt: at, Entries: encoded.entries}); entriesErr != nil {
			return fmt.Errorf("entries: %w", entriesErr)
		}
		return q.UpsertPlaylistFile(ctx, sqlcdb.UpsertPlaylistFileParams{
			PlaylistID: p.row, Root: w.Root, RelPath: w.RelPath, Size: w.Size, MtimeMs: ms(w.MTime), ReadAt: at,
			Reader: int64(w.Reader), Lines: int64(w.Lines), PastLimit: int64(w.PastLimit), Missing: encoded.missing,
		})
	})
	if err != nil {
		return "", fmt.Errorf("write file playlist: %w", err)
	}
	return id, nil
}

// fileReadJSON is a file read's missing lines and entries, as the JSON the
// store keeps and inserts.
type fileReadJSON struct{ missing, entries string }

func encodeFileRead(w *FilePlaylistWrite) (fileReadJSON, error) {
	missing := []byte("[]")
	if len(w.Missing) > 0 {
		var err error
		if missing, err = json.Marshal(w.Missing); err != nil {
			return fileReadJSON{}, fmt.Errorf("encode missing: %w", err)
		}
	}
	triples := make([][3]any, len(w.Entries))
	for i, e := range w.Entries {
		triples[i] = [3]any{e.Line, e.Track, e.Text}
	}
	entries, err := json.Marshal(triples)
	if err != nil {
		return fileReadJSON{}, fmt.Errorf("encode entries: %w", err)
	}
	return fileReadJSON{missing: string(missing), entries: string(entries)}, nil
}

// playlistRef is a playlist's row and ULID.
type playlistRef struct {
	row int64
	id  string
}

// fileRow returns the playlist read from w's file, made with w.NewID when there
// is none, and else named afresh and emptied of its entries, for the read's to
// replace.
func fileRow(ctx context.Context, q *sqlcdb.Queries, w *FilePlaylistWrite, at int64) (playlistRef, error) {
	row, err := q.FilePlaylistAt(ctx, sqlcdb.FilePlaylistAtParams{Root: w.Root, RelPath: w.RelPath})
	if errors.Is(err, sql.ErrNoRows) {
		made, insertErr := q.InsertFilePlaylist(ctx, sqlcdb.InsertFilePlaylistParams{Ulid: w.NewID, Name: w.Name, At: at})
		if insertErr != nil {
			return playlistRef{}, fmt.Errorf("insert: %w", insertErr)
		}
		return playlistRef{row: made, id: w.NewID}, nil
	}
	if err != nil {
		return playlistRef{}, fmt.Errorf("find: %w", err)
	}
	if err = q.SetFilePlaylistName(ctx, sqlcdb.SetFilePlaylistNameParams{Name: w.Name, UpdatedAt: at, ID: row.PlaylistID}); err != nil {
		return playlistRef{}, fmt.Errorf("name: %w", err)
	}
	if err = q.ClearPlaylistEntries(ctx, row.PlaylistID); err != nil {
		return playlistRef{}, fmt.Errorf("clear: %w", err)
	}
	return playlistRef{row: row.PlaylistID, id: row.Ulid}, nil
}

// DeleteFilePlaylists deletes the playlists read from the files at rels under
// root, with their entries, and returns their ULIDs. A copy someone made of one
// stays: it is a playlist of its own.
func (s *Store) DeleteFilePlaylists(ctx context.Context, root string, rels []string) ([]string, error) {
	var ids []string
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		for _, rel := range rels {
			id, err := q.DeleteFilePlaylist(ctx, sqlcdb.DeleteFilePlaylistParams{Root: root, RelPath: rel})
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("delete file playlists: %w", err)
	}
	return ids, nil
}

// FilePlaylistAt is the file playlist read from a file.
type FilePlaylistAt struct {
	ID, Name   string
	Songs      int // Its live songs.
	ModifiedAt time.Time
}

// FilePlaylistOf returns the file playlist read from the file at rel under
// root. It returns an error wrapping [ErrNotFound] when none is.
func (s *Store) FilePlaylistOf(ctx context.Context, root, rel string) (FilePlaylistAt, error) {
	row, err := s.rq.FilePlaylistAt(ctx, sqlcdb.FilePlaylistAtParams{Root: root, RelPath: rel})
	if err != nil {
		return FilePlaylistAt{}, fmt.Errorf("file playlist %s: %w", rel, notFound(err))
	}
	songs, err := s.rq.ListPlaylistSongsOf(ctx, row.PlaylistID)
	if err != nil {
		return FilePlaylistAt{}, fmt.Errorf("file playlist %s: songs: %w", rel, err)
	}
	return FilePlaylistAt{ID: row.Ulid, Name: row.Name, Songs: len(songs), ModifiedAt: fromMS(row.MtimeMs)}, nil
}

// playlistFiles returns every file playlist's file, by playlist row.
func playlistFiles(ctx context.Context, q *sqlcdb.Queries) (map[int64]*PlaylistFile, error) {
	rows, err := q.ListPlaylistFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("files: %w", err)
	}
	out := make(map[int64]*PlaylistFile, len(rows))
	for i := range rows {
		r := &rows[i]
		out[r.PlaylistID] = &PlaylistFile{
			RelPath: r.RelPath, ModifiedAt: fromMS(r.MtimeMs), ReadAt: fromMS(r.ReadAt), PastLimit: int(r.PastLimit),
			NotFound: int(r.Missing + r.Gone + r.PastLimit),
		}
	}
	return out, nil
}

// fileDetail returns the file of the file playlist row and its lines that
// found no song: those its read found none for, and those whose song has left
// the library since, in the file's order.
func fileDetail(ctx context.Context, q *sqlcdb.Queries, row int64) (*PlaylistFile, []MissingLine, error) {
	f, err := q.PlaylistFileOf(ctx, row)
	if err != nil {
		return nil, nil, fmt.Errorf("file: %w", err)
	}
	var missing []MissingLine
	if err = json.Unmarshal([]byte(f.Missing), &missing); err != nil {
		return nil, nil, fmt.Errorf("file: missing: %w", err)
	}
	gone, err := q.ListGoneFileEntries(ctx, row)
	if err != nil {
		return nil, nil, fmt.Errorf("file: gone: %w", err)
	}
	for _, g := range gone {
		reason := MissingNoSong
		if g.Trashed != 0 {
			reason = MissingTrashed
		}
		missing = append(missing, MissingLine{Line: int(g.Position), Text: m3u.Shown(g.Line), Reason: reason})
	}
	slices.SortStableFunc(missing, func(a, b MissingLine) int { return cmp.Compare(a.Line, b.Line) })
	out := &PlaylistFile{
		RelPath: f.RelPath, ModifiedAt: fromMS(f.MtimeMs), ReadAt: fromMS(f.ReadAt), PastLimit: int(f.PastLimit),
	}
	out.NotFound = len(missing) + out.PastLimit
	return out, missing, nil
}

// ExportSong is a song as an exported playlist file names it.
type ExportSong struct {
	Root, RelPath string
	Duration      time.Duration
	Artists       []string
	Title         string
}

// PlaylistExport returns the name of a playlist the user sees and its live
// songs in order: a smart one's as its rules choose them now, its genre rules
// through genres. It returns an error wrapping [ErrNotFound] for a playlist the
// user doesn't see.
func (s *Store) PlaylistExport(ctx context.Context, userID int64, id string, genres GenreAlbums) (string, []ExportSong, error) {
	name, songs, err := s.playlistExport(ctx, userID, id, genres)
	if err != nil {
		return "", nil, fmt.Errorf("export playlist %s: %w", id, err)
	}
	return name, songs, nil
}

func (s *Store) playlistExport(ctx context.Context, userID int64, id string, genres GenreAlbums) (string, []ExportSong, error) {
	row, err := s.rq.PlaylistRow(ctx, sqlcdb.PlaylistRowParams{UserID: userID, Ulid: id})
	if err != nil {
		return "", nil, notFound(err)
	}
	rules, err := parseStored(id, row.Rules)
	if err != nil {
		return "", nil, err
	}
	var order []int64
	if rules != nil {
		if order, err = s.smartRows(ctx, userID, id, rules, row.Seed, genres); err != nil {
			return "", nil, err
		}
	} else {
		entries, listErr := s.rq.ListPlaylistEntries(ctx, row.ID)
		if listErr != nil {
			return "", nil, fmt.Errorf("entries: %w", listErr)
		}
		order = make([]int64, len(entries))
		for i := range entries {
			order[i] = entries[i].TrackRow
		}
	}
	songs, err := s.exportSongs(ctx, order)
	return row.Name, songs, err
}

// exportSongs returns the live tracks with the rows order, in that order, a
// track named twice twice.
func (s *Store) exportSongs(ctx context.Context, order []int64) ([]ExportSong, error) {
	ids := slices.Compact(slices.Sorted(slices.Values(order)))
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("encode tracks: %w", err)
	}
	tracks, err := s.rq.ListExportTracks(ctx, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("tracks: %w", err)
	}
	artists, err := s.rq.ListExportArtists(ctx, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("artists: %w", err)
	}
	byRow := make(map[int64]*ExportSong, len(tracks))
	for i := range tracks {
		t := &tracks[i]
		byRow[t.ID] = &ExportSong{
			Root: t.Root, RelPath: t.RelPath, Duration: time.Duration(t.DurationMs) * time.Millisecond, Title: t.Title,
		}
	}
	for _, a := range artists {
		if song := byRow[a.TrackID]; song != nil {
			song.Artists = append(song.Artists, a.Name)
		}
	}
	out := make([]ExportSong, 0, len(order))
	for _, row := range order {
		if song := byRow[row]; song != nil {
			out = append(out, *song)
		}
	}
	return out, nil
}
