package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// A playlist is one person's songs in their own order. Nobody else reads or
// changes it: every method takes the owner, and another person's playlist is
// [ErrNotFound], as one that was never there is. A song may be in it more than
// once, each time its own entry, with an entry ID of its own that is never
// reused. Its entries are spaced [positionStep] apart, so a move writes one row;
// a playlist is renumbered only when two neighbors have no room between them.
//
// What a playlist holds is read through live tracks only, so a song in Trash
// leaves every view of it and comes back with Restore; its entries go only when
// the track is deleted for good, by ON DELETE CASCADE (see [deleteTracks]).

// Where a playlist came from.
const (
	PlaylistMade     = "kitsune"   // Made here.
	PlaylistImported = "navidrome" // Brought over by an import.
)

// MostPlaylistEntries is the most entries a playlist holds.
const MostPlaylistEntries = 10_000

// MostPlaylistName is the longest a playlist's name may be, in characters.
const MostPlaylistName = 200

// mosaicSize is how many albums a playlist's mosaic shows.
const mosaicSize = 4

// positionStep is the room between entries. A move into a gap halves it, so 16
// moves can land between the same two songs before the playlist is renumbered;
// 10,000 entries a step apart are still far inside an int64.
const positionStep = 1 << 16

// Sentinels callers match with [errors.Is].
var (
	// ErrPlaylistFull means an add would take a playlist past
	// [MostPlaylistEntries].
	ErrPlaylistFull = errors.New("a playlist holds at most 10,000 songs")
	// ErrNoEntry means an entry named isn't in the playlist.
	ErrNoEntry = errors.New("no such entry")
	// ErrBadPlaylistName means a name is empty after trimming, or longer than
	// [MostPlaylistName].
	ErrBadPlaylistName = errors.New("a playlist's name is 1 to 200 characters")
)

// NotLiveError is an add that names songs or albums with nothing live in the
// library: unknown, missing or in Trash. Nothing was added.
type NotLiveError struct {
	Tracks []string // Track ULIDs, in the order named.
	Albums []string // Album ULIDs, in the order named.
}

func (e *NotLiveError) Error() string {
	return fmt.Sprintf("not in the library: tracks %s, albums %s",
		strings.Join(e.Tracks, ","), strings.Join(e.Albums, ","))
}

// Playlist is a playlist as a list of them shows it. Its counts are of live
// songs.
type Playlist struct {
	ID                   string // The playlist's ULID.
	Name                 string
	Source               string // [PlaylistMade] or [PlaylistImported].
	CreatedAt, UpdatedAt time.Time
	AddedAt              time.Time // When songs were last added; zero before.
	Songs                int       // Live entries, a song in twice counted twice.
	Albums               int       // Distinct albums among them.
	Duration             time.Duration
	// Mosaic is the first albums of the playlist, in its order, each once, at
	// most [mosaicSize].
	Mosaic []PlaylistAlbum
	// Holds is how many of the songs asked about it holds, each counted once.
	Holds int
}

// PlaylistAlbum is an album as a playlist shows it.
type PlaylistAlbum struct {
	ID          string // The album's ULID.
	Title       string
	Artist      string
	Year        int // 0 is unknown.
	Compilation bool
	Cover       *Cover // Nil when the album has none.
}

// PlaylistDetail is a playlist with its live entries in order.
type PlaylistDetail struct {
	Playlist

	Entries []PlaylistEntry
	// Albums are every album of Entries, in the order they first appear.
	Albums []PlaylistAlbum
}

// PlaylistEntry is one place in a playlist.
type PlaylistEntry struct {
	ID    int64  // The entry's own ID, never reused.
	Album string // The song's album's ULID.
	Track IndexTrack
}

// PlaylistAdd is what to put in a playlist.
type PlaylistAdd struct {
	// Albums are added first, each in the order the Library lists its songs,
	// then Tracks, in the order named.
	Albums []string
	Tracks []string
	// Again adds a song the playlist holds already, or one named twice. Without
	// it, those are skipped.
	Again bool
	// Before puts the songs just before this entry. Zero means at the end.
	Before int64
}

// PlaylistAdded is what an add did.
type PlaylistAdded struct {
	Added   []AddedEntry
	Skipped []string // Track ULIDs left out as already there, in the order named.
}

// AddedEntry is one entry an add made.
type AddedEntry struct {
	Entry int64
	Track string // The track's ULID.
}

// PlaylistName returns name trimmed, or an error wrapping [ErrBadPlaylistName].
//
// # Errors
//
// [ErrBadPlaylistName] for a name that is empty after trimming or too long.
func PlaylistName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || len([]rune(trimmed)) > MostPlaylistName {
		return "", ErrBadPlaylistName
	}
	return trimmed, nil
}

// Playlists returns the user's playlists by name, each saying how many of the
// songs with the ULIDs holding it holds.
func (s *Store) Playlists(ctx context.Context, userID int64, holding []string) ([]Playlist, error) {
	out, err := s.playlists(ctx, userID, holding)
	if err != nil {
		return nil, fmt.Errorf("playlists: %w", err)
	}
	return out, nil
}

func (s *Store) playlists(ctx context.Context, userID int64, holding []string) ([]Playlist, error) {
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Nothing was written: there is nothing to undo.
	q := s.rq.WithTx(tx)
	rows, err := q.ListPlaylists(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	songs, err := q.ListPlaylistSongs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("songs: %w", err)
	}
	asked := make(map[string]bool, len(holding))
	for _, id := range holding {
		asked[id] = true
	}
	tallies := make(map[int64]*tally, len(rows))
	for i := range songs {
		r := &songs[i]
		t := tallies[r.PlaylistID]
		if t == nil {
			t = newTally()
			tallies[r.PlaylistID] = t
		}
		t.add(r.Ulid, r.AlbumID, r.DurationMs, asked)
	}
	albums, err := mosaicAlbums(ctx, q, tallies)
	if err != nil {
		return nil, err
	}
	out := make([]Playlist, len(rows))
	for i := range rows {
		r := &rows[i]
		out[i] = playlistOf(r.Ulid, r.Name, r.Source, r.CreatedAt, r.UpdatedAt, r.AddedAt)
		cmp.Or(tallies[r.ID], newTally()).fill(&out[i], albums)
	}
	return out, nil
}

// tally is what a list of playlists counts of one as its songs go by.
type tally struct {
	songs    int
	duration int64
	albums   map[int64]bool
	mosaic   []int64
	holds    map[string]bool
}

func newTally() *tally { return &tally{albums: map[int64]bool{}, holds: map[string]bool{}} }

func (t *tally) add(track string, album, durationMs int64, asked map[string]bool) {
	t.songs++
	t.duration += durationMs
	if !t.albums[album] {
		t.albums[album] = true
		if len(t.mosaic) < mosaicSize {
			t.mosaic = append(t.mosaic, album)
		}
	}
	if asked[track] {
		t.holds[track] = true
	}
}

// fill puts the tally in p, its mosaic from albums by row.
func (t *tally) fill(p *Playlist, albums map[int64]PlaylistAlbum) {
	p.Songs, p.Albums, p.Holds = t.songs, len(t.albums), len(t.holds)
	p.Duration = time.Duration(t.duration) * time.Millisecond
	p.Mosaic = make([]PlaylistAlbum, 0, len(t.mosaic))
	for _, id := range t.mosaic {
		if a, ok := albums[id]; ok {
			p.Mosaic = append(p.Mosaic, a)
		}
	}
}

// mosaicAlbums reads every album a tally's mosaic shows, by row.
func mosaicAlbums(ctx context.Context, q *sqlcdb.Queries, tallies map[int64]*tally) (map[int64]PlaylistAlbum, error) {
	var ids []int64
	for _, t := range tallies {
		ids = append(ids, t.mosaic...)
	}
	return playlistAlbums(ctx, q, ids)
}

// playlistAlbums reads the albums with the row IDs ids, by row.
func playlistAlbums(ctx context.Context, q *sqlcdb.Queries, ids []int64) (map[int64]PlaylistAlbum, error) {
	out := make(map[int64]PlaylistAlbum, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("encode albums: %w", err)
	}
	rows, err := q.ListPlaylistAlbums(ctx, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("albums: %w", err)
	}
	for i := range rows {
		r := &rows[i]
		out[r.ID] = PlaylistAlbum{
			ID: r.Ulid, Title: r.Name, Artist: r.Artist, Year: intOr0(r.Year), Compilation: r.Compilation != 0,
			Cover: cover(r.ArtworkHash, r.ArtworkWidth, r.ArtworkHeight, r.ArtworkAccent, r.ArtworkGround),
		}
	}
	return out, nil
}

func playlistOf(id, name, source string, created, updated int64, added *int64) Playlist {
	p := Playlist{
		ID: id, Name: name, Source: source,
		CreatedAt: time.UnixMilli(created).UTC(), UpdatedAt: time.UnixMilli(updated).UTC(),
	}
	if added != nil {
		p.AddedAt = time.UnixMilli(*added).UTC()
	}
	return p
}

// Playlist returns the user's playlist with its live entries in order, each
// with its song as the library index has it and the user's stars. It returns an
// error wrapping [ErrNotFound] for a playlist the user doesn't have.
func (s *Store) Playlist(ctx context.Context, userID int64, id string) (PlaylistDetail, error) {
	d, err := s.playlistDetail(ctx, userID, id)
	if err != nil {
		return PlaylistDetail{}, fmt.Errorf("playlist %s: %w", id, err)
	}
	return d, nil
}

func (s *Store) playlistDetail(ctx context.Context, userID int64, id string) (PlaylistDetail, error) {
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PlaylistDetail{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Nothing was written: there is nothing to undo.
	q := s.rq.WithTx(tx)
	row, err := q.PlaylistRow(ctx, sqlcdb.PlaylistRowParams{UserID: userID, Ulid: id})
	if err != nil {
		return PlaylistDetail{}, notFound(err)
	}
	entries, err := q.ListPlaylistEntries(ctx, row.ID)
	if err != nil {
		return PlaylistDetail{}, fmt.Errorf("entries: %w", err)
	}
	artists, err := q.ListPlaylistTrackArtists(ctx, row.ID)
	if err != nil {
		return PlaylistDetail{}, fmt.Errorf("artists: %w", err)
	}
	ratings, err := q.ListPlaylistRatings(ctx, sqlcdb.ListPlaylistRatingsParams{UserID: userID, PlaylistID: row.ID})
	if err != nil {
		return PlaylistDetail{}, fmt.Errorf("ratings: %w", err)
	}
	order := make([]int64, 0, len(entries))
	t := newTally()
	for i := range entries {
		e := &entries[i]
		if !t.albums[e.AlbumID] {
			order = append(order, e.AlbumID)
		}
		t.add(e.Ulid, e.AlbumID, e.DurationMs, nil)
	}
	albums, err := playlistAlbums(ctx, q, order)
	if err != nil {
		return PlaylistDetail{}, err
	}
	d := PlaylistDetail{Playlist: playlistOf(row.Ulid, row.Name, row.Source, row.CreatedAt, row.UpdatedAt, row.AddedAt)}
	t.fill(&d.Playlist, albums)
	d.Albums = make([]PlaylistAlbum, 0, len(order))
	for _, a := range order {
		d.Albums = append(d.Albums, albums[a])
	}
	d.Entries = playlistEntries(entries, artists, ratings, albums)
	return d, nil
}

// playlistEntries makes the entries of a playlist from what its queries read.
func playlistEntries(
	entries []sqlcdb.ListPlaylistEntriesRow,
	artists []sqlcdb.ListPlaylistTrackArtistsRow,
	ratings []sqlcdb.ListPlaylistRatingsRow,
	albums map[int64]PlaylistAlbum,
) []PlaylistEntry {
	names := make(map[int64][]string, len(entries))
	for i := range artists {
		id := artists[i].TrackID
		names[id] = append(names[id], artists[i].Name)
	}
	stars := make(map[int64]rating.Rating, len(ratings))
	for _, r := range ratings {
		stars[r.TrackID] = ratingOf(r.Halves)
	}
	out := make([]PlaylistEntry, len(entries))
	for i := range entries {
		e := &entries[i]
		named := names[e.TrackRow]
		if named == nil {
			named = []string{}
		}
		out[i] = PlaylistEntry{
			ID: e.ID, Album: albums[e.AlbumID].ID,
			Track: IndexTrack{
				ID: e.Ulid, Title: e.Title, Artists: named,
				DiscNo: intOr0(e.DiscNo), TrackNo: intOr0(e.TrackNo),
				Duration: time.Duration(e.DurationMs) * time.Millisecond,
				Rating:   stars[e.TrackRow], // rating.None when unrated.
			},
		}
	}
	return out
}

// PlaylistSummary returns the user's playlist as [Store.Playlists] lists it. It
// returns an error wrapping [ErrNotFound] for a playlist the user doesn't have.
func (s *Store) PlaylistSummary(ctx context.Context, userID int64, id string) (Playlist, error) {
	p, err := s.playlistSummary(ctx, userID, id)
	if err != nil {
		return Playlist{}, fmt.Errorf("playlist %s: %w", id, err)
	}
	return p, nil
}

func (s *Store) playlistSummary(ctx context.Context, userID int64, id string) (Playlist, error) {
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Playlist{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Nothing was written: there is nothing to undo.
	q := s.rq.WithTx(tx)
	row, err := q.PlaylistRow(ctx, sqlcdb.PlaylistRowParams{UserID: userID, Ulid: id})
	if err != nil {
		return Playlist{}, notFound(err)
	}
	songs, err := q.ListPlaylistSongsOf(ctx, row.ID)
	if err != nil {
		return Playlist{}, fmt.Errorf("songs: %w", err)
	}
	t := newTally()
	for i := range songs {
		t.add(songs[i].Ulid, songs[i].AlbumID, songs[i].DurationMs, nil)
	}
	albums, err := playlistAlbums(ctx, q, t.mosaic)
	if err != nil {
		return Playlist{}, err
	}
	p := playlistOf(row.Ulid, row.Name, row.Source, row.CreatedAt, row.UpdatedAt, row.AddedAt)
	t.fill(&p, albums)
	return p, nil
}

// CreatePlaylist makes a playlist of the user's named name, with the ULID
// newID, and puts add's songs in it. It returns what the add did. The name must
// have passed [PlaylistName].
//
// It returns a [*NotLiveError] for songs or albums not in the library, and an
// error wrapping [ErrPlaylistFull] for more than [MostPlaylistEntries] songs;
// then nothing is made.
func (s *Store) CreatePlaylist(ctx context.Context, userID int64, newID, name string, add *PlaylistAdd) (PlaylistAdded, error) {
	var added PlaylistAdded
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		at := ms(s.now())
		row, err := q.InsertPlaylist(ctx, sqlcdb.InsertPlaylistParams{
			Ulid: newID, UserID: userID, Name: name, Source: PlaylistMade, CreatedAt: at, UpdatedAt: at,
		})
		if err != nil {
			return fmt.Errorf("insert: %w", err)
		}
		added, err = addEntries(ctx, q, row, add, at)
		return err
	})
	if err != nil {
		return PlaylistAdded{}, fmt.Errorf("create playlist: %w", err)
	}
	return added, nil
}

// RenamePlaylist renames the user's playlist. The name must have passed
// [PlaylistName]. It returns an error wrapping [ErrNotFound] for a playlist the
// user doesn't have.
func (s *Store) RenamePlaylist(ctx context.Context, userID int64, id, name string) error {
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		row, err := ownPlaylist(ctx, q, userID, id)
		if err != nil {
			return err
		}
		return q.RenamePlaylist(ctx, sqlcdb.RenamePlaylistParams{Name: name, UpdatedAt: ms(s.now()), ID: row})
	})
	if err != nil {
		return fmt.Errorf("rename playlist %s: %w", id, err)
	}
	return nil
}

// DeletePlaylist deletes the user's playlist with its entries. It never touches
// a song. It returns an error wrapping [ErrNotFound] for a playlist the user
// doesn't have.
func (s *Store) DeletePlaylist(ctx context.Context, userID int64, id string) error {
	n, err := s.wq.DeletePlaylist(ctx, sqlcdb.DeletePlaylistParams{UserID: userID, Ulid: id})
	if err != nil {
		return fmt.Errorf("delete playlist %s: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("delete playlist %s: %w", id, ErrNotFound)
	}
	return nil
}

// AddToPlaylist puts add's songs in the user's playlist and says which went in
// and which were skipped as already there.
//
// It returns an error wrapping [ErrNotFound] for a playlist the user doesn't
// have, [ErrNoEntry] for an add.Before that isn't in it, [ErrPlaylistFull] for
// an add that would take it past [MostPlaylistEntries], and a [*NotLiveError]
// for songs or albums not in the library. Then nothing is added.
func (s *Store) AddToPlaylist(ctx context.Context, userID int64, id string, add *PlaylistAdd) (PlaylistAdded, error) {
	var added PlaylistAdded
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		row, err := ownPlaylist(ctx, q, userID, id)
		if err != nil {
			return err
		}
		added, err = addEntries(ctx, q, row, add, ms(s.now()))
		return err
	})
	if err != nil {
		return PlaylistAdded{}, fmt.Errorf("add to playlist %s: %w", id, err)
	}
	return added, nil
}

// ownPlaylist returns the row of the user's playlist id, or [ErrNotFound].
func ownPlaylist(ctx context.Context, q *sqlcdb.Queries, userID int64, id string) (int64, error) {
	row, err := q.PlaylistRow(ctx, sqlcdb.PlaylistRowParams{UserID: userID, Ulid: id})
	if err != nil {
		return 0, notFound(err)
	}
	return row.ID, nil
}

// addEntries puts add's songs in the playlist with the row playlist, at at.
func addEntries(ctx context.Context, q *sqlcdb.Queries, playlist int64, add *PlaylistAdd, at int64) (PlaylistAdded, error) {
	songs, err := songsToAdd(ctx, q, add)
	if err != nil {
		return PlaylistAdded{}, err
	}
	out := PlaylistAdded{Added: []AddedEntry{}, Skipped: []string{}}
	songs, out.Skipped, err = skipHeld(ctx, q, playlist, songs, add.Again)
	if err != nil || len(songs) == 0 {
		return out, err
	}
	if out.Added, err = insertEntries(ctx, q, playlist, add.Before, songs, at); err != nil {
		return PlaylistAdded{}, err
	}
	if err = q.TouchPlaylistAdded(ctx, sqlcdb.TouchPlaylistAddedParams{At: at, ID: playlist}); err != nil {
		return PlaylistAdded{}, fmt.Errorf("touch: %w", err)
	}
	return out, nil
}

// insertEntries puts songs in the playlist, in order, before the entry before or
// at the end for 0, and returns the entries made.
func insertEntries(ctx context.Context, q *sqlcdb.Queries, playlist, before int64, songs []song, at int64) ([]AddedEntry, error) {
	held, err := q.CountPlaylistEntries(ctx, playlist)
	if err != nil {
		return nil, fmt.Errorf("count: %w", err)
	}
	if held+int64(len(songs)) > MostPlaylistEntries {
		return nil, ErrPlaylistFull
	}
	positions, err := placesFor(ctx, q, playlist, before, len(songs))
	if err != nil {
		return nil, err
	}
	pairs := make([][2]int64, len(songs))
	ulids := make(map[int64]string, len(songs))
	for i, song := range songs {
		pairs[i] = [2]int64{positions[i], song.row}
		ulids[song.row] = song.id
	}
	encoded, err := json.Marshal(pairs)
	if err != nil {
		return nil, fmt.Errorf("encode entries: %w", err)
	}
	rows, err := q.InsertPlaylistEntries(ctx, sqlcdb.InsertPlaylistEntriesParams{PlaylistID: playlist, AddedAt: at, Entries: string(encoded)})
	if err != nil {
		return nil, fmt.Errorf("insert entries: %w", err)
	}
	slices.SortFunc(rows, func(a, b sqlcdb.InsertPlaylistEntriesRow) int { return cmp.Compare(a.Position, b.Position) })
	out := make([]AddedEntry, len(rows))
	for i, r := range rows {
		out[i] = AddedEntry{Entry: r.ID, Track: ulids[r.TrackID]}
	}
	return out, nil
}

// song is a live track on its way into a playlist.
type song struct {
	row int64
	id  string // Its ULID.
}

// songsToAdd returns add's songs in order: its albums', then its tracks. It
// returns a [*NotLiveError] naming those not in the library.
func songsToAdd(ctx context.Context, q *sqlcdb.Queries, add *PlaylistAdd) ([]song, error) {
	gone := &NotLiveError{}
	out, err := albumSongs(ctx, q, add.Albums, gone)
	if err != nil {
		return nil, err
	}
	tracks, err := liveTracks(ctx, q, add.Tracks)
	if err != nil {
		return nil, err
	}
	for _, id := range add.Tracks {
		row, ok := tracks[id]
		switch {
		case ok:
			out = append(out, song{row: row, id: id})
		case !slices.Contains(gone.Tracks, id):
			gone.Tracks = append(gone.Tracks, id)
		}
	}
	if len(gone.Tracks) > 0 || len(gone.Albums) > 0 {
		return nil, gone
	}
	return out, nil
}

// albumSongs returns the live songs of albums, each album's in the order the
// Library lists them, and notes in gone each album with none.
func albumSongs(ctx context.Context, q *sqlcdb.Queries, albums []string, gone *NotLiveError) ([]song, error) {
	var out []song
	for _, album := range albums {
		rows, err := q.ListAlbumTracksToAdd(ctx, album)
		if err != nil {
			return nil, fmt.Errorf("album %s: %w", album, err)
		}
		if len(rows) == 0 && !slices.Contains(gone.Albums, album) {
			gone.Albums = append(gone.Albums, album)
		}
		for _, r := range rows {
			out = append(out, song{row: r.ID, id: r.Ulid})
		}
	}
	return out, nil
}

// liveTracks returns the rows of the live tracks among ids, by ULID.
func liveTracks(ctx context.Context, q *sqlcdb.Queries, ids []string) (map[string]int64, error) {
	out := make(map[string]int64, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("encode tracks: %w", err)
	}
	rows, err := q.ListTracksToAdd(ctx, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("tracks: %w", err)
	}
	for _, r := range rows {
		if r.Live != 0 {
			out[r.Ulid] = r.ID
		}
	}
	return out, nil
}

// skipHeld returns songs less those the playlist holds already and repeats of
// one named before, unless again, with the ULIDs of those it left out, each once.
func skipHeld(ctx context.Context, q *sqlcdb.Queries, playlist int64, songs []song, again bool) ([]song, []string, error) {
	skipped := []string{}
	if again {
		return songs, skipped, nil
	}
	rows, err := q.ListPlaylistTrackRows(ctx, playlist)
	if err != nil {
		return nil, nil, fmt.Errorf("held: %w", err)
	}
	held := make(map[int64]bool, len(rows)+len(songs))
	for _, r := range rows {
		held[r] = true
	}
	kept := make([]song, 0, len(songs))
	for _, s := range songs {
		if !held[s.row] {
			held[s.row] = true
			kept = append(kept, s)
		} else if !slices.Contains(skipped, s.id) {
			skipped = append(skipped, s.id)
		}
	}
	return kept, skipped, nil
}

// placesFor returns n positions in order for entries going just before the
// entry before, or at the end for 0, renumbering the playlist first when there
// isn't room.
func placesFor(ctx context.Context, q *sqlcdb.Queries, playlist, before int64, n int) ([]int64, error) {
	out := make([]int64, n)
	if before == 0 {
		last, err := q.LastPlaylistPosition(ctx, playlist)
		if err != nil {
			return nil, fmt.Errorf("last position: %w", err)
		}
		for i := range out {
			out[i] = last + int64(i+1)*positionStep
		}
		return out, nil
	}
	g, err := roomFor(ctx, q, playlist, n, func() (gap, error) { return gapBefore(ctx, q, playlist, before) })
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i] = g.lo + (g.hi-g.lo)*int64(i+1)/int64(n+1)
	}
	return out, nil
}

// gap is the room between two neighboring positions, lo and hi, neither of
// which is in it.
type gap struct{ lo, hi int64 }

// fits reports whether n positions fit strictly inside g.
func (g gap) fits(n int) bool { return g.hi-g.lo > int64(n) }

// roomFor returns the gap find finds, renumbering the playlist first when n
// positions don't fit in it.
func roomFor(ctx context.Context, q *sqlcdb.Queries, playlist int64, n int, find func() (gap, error)) (gap, error) {
	g, err := find()
	if err != nil || g.fits(n) {
		return g, err
	}
	if renumberErr := renumber(ctx, q, playlist); renumberErr != nil {
		return gap{}, renumberErr
	}
	return find()
}

// gapBefore returns the gap just before entry: from the position before it, 0
// at the start, to its own.
func gapBefore(ctx context.Context, q *sqlcdb.Queries, playlist, entry int64) (gap, error) {
	hi, err := entryPosition(ctx, q, playlist, entry)
	if err != nil {
		return gap{}, err
	}
	lo, err := q.PositionBefore(ctx, sqlcdb.PositionBeforeParams{PlaylistID: playlist, Position: hi})
	if err != nil {
		return gap{}, fmt.Errorf("position before: %w", err)
	}
	return gap{lo: lo, hi: hi}, nil
}

// gapAfter returns the gap just after entry: from its own position to the next,
// which past the last is a step and more beyond.
func gapAfter(ctx context.Context, q *sqlcdb.Queries, playlist, entry int64) (gap, error) {
	lo, err := entryPosition(ctx, q, playlist, entry)
	if err != nil {
		return gap{}, err
	}
	hi, err := q.PositionAfter(ctx, sqlcdb.PositionAfterParams{PlaylistID: playlist, Position: lo})
	if err != nil {
		return gap{}, fmt.Errorf("position after: %w", err)
	}
	if hi < 0 {
		hi = lo + positionStep + positionStep
	}
	return gap{lo: lo, hi: hi}, nil
}

// renumber spaces a playlist's entries [positionStep] apart again, in the same
// order: every position goes below zero first, so none collides on the way.
func renumber(ctx context.Context, q *sqlcdb.Queries, playlist int64) error {
	rows, err := q.ListEntryRows(ctx, playlist)
	if err != nil {
		return fmt.Errorf("renumber: %w", err)
	}
	if err = q.ParkPositions(ctx, playlist); err != nil {
		return fmt.Errorf("renumber: park: %w", err)
	}
	for i, id := range rows {
		_, err = q.SetEntryPosition(ctx, sqlcdb.SetEntryPositionParams{Position: int64(i+1) * positionStep, PlaylistID: playlist, ID: id})
		if err != nil {
			return fmt.Errorf("renumber: %w", err)
		}
	}
	return nil
}

// RemoveFromPlaylist takes entries out of the user's playlist and returns those
// it took; one not in it is left alone. The songs stay in the library. It
// returns an error wrapping [ErrNotFound] for a playlist the user doesn't have.
func (s *Store) RemoveFromPlaylist(ctx context.Context, userID int64, id string, entries []int64) ([]int64, error) {
	var removed []int64
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		row, err := ownPlaylist(ctx, q, userID, id)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(entries)
		if err != nil {
			return fmt.Errorf("encode entries: %w", err)
		}
		if removed, err = q.DeletePlaylistEntries(ctx, sqlcdb.DeletePlaylistEntriesParams{PlaylistID: row, Ids: string(encoded)}); err != nil {
			return fmt.Errorf("delete entries: %w", err)
		}
		if len(removed) == 0 {
			return nil
		}
		return q.TouchPlaylist(ctx, sqlcdb.TouchPlaylistParams{UpdatedAt: ms(s.now()), ID: row})
	})
	if err != nil {
		return nil, fmt.Errorf("remove from playlist %s: %w", id, err)
	}
	slices.Sort(removed)
	return removed, nil
}

// MovePlaylistEntry moves an entry of the user's playlist to just before the
// entry anchor, or just after it when after is set. It writes the one entry,
// unless the gap it lands in has no room, when the playlist is renumbered.
//
// It returns an error wrapping [ErrNotFound] for a playlist the user doesn't
// have, and [ErrNoEntry] when entry or anchor isn't in it.
func (s *Store) MovePlaylistEntry(ctx context.Context, userID int64, id string, entry, anchor int64, after bool) error {
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		row, err := ownPlaylist(ctx, q, userID, id)
		if err != nil {
			return err
		}
		moved, err := moveEntry(ctx, q, row, entry, anchor, after)
		if err != nil || !moved {
			return err
		}
		return q.TouchPlaylist(ctx, sqlcdb.TouchPlaylistParams{UpdatedAt: ms(s.now()), ID: row})
	})
	if err != nil {
		return fmt.Errorf("move in playlist %s: %w", id, err)
	}
	return nil
}

// moveEntry moves entry next to anchor, and reports whether it had to move.
func moveEntry(ctx context.Context, q *sqlcdb.Queries, playlist, entry, anchor int64, after bool) (bool, error) {
	at, err := entryPosition(ctx, q, playlist, entry)
	if err != nil {
		return false, err
	}
	find := func() (gap, error) { return gapBefore(ctx, q, playlist, anchor) }
	if after {
		find = func() (gap, error) { return gapAfter(ctx, q, playlist, anchor) }
	}
	g, err := find()
	if err != nil {
		return false, err
	}
	if entry == anchor || at == g.lo || at == g.hi {
		return false, nil // Already there.
	}
	if g, err = roomFor(ctx, q, playlist, 1, find); err != nil {
		return false, err
	}
	_, err = q.SetEntryPosition(ctx, sqlcdb.SetEntryPositionParams{Position: g.lo + (g.hi-g.lo)/2, PlaylistID: playlist, ID: entry})
	if err != nil {
		return false, fmt.Errorf("set position: %w", err)
	}
	return true, nil
}

// entryPosition returns where entry is in the playlist, or [ErrNoEntry].
func entryPosition(ctx context.Context, q *sqlcdb.Queries, playlist, entry int64) (int64, error) {
	at, err := q.EntryPosition(ctx, sqlcdb.EntryPositionParams{PlaylistID: playlist, ID: entry})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNoEntry
	}
	if err != nil {
		return 0, fmt.Errorf("entry position: %w", err)
	}
	return at, nil
}

// PlaylistTrackRows returns the row of every track the user's playlist holds,
// once each, live or not: a shuffle's scope. It returns an error wrapping
// [ErrNotFound] for a playlist the user doesn't have.
func (s *Store) PlaylistTrackRows(ctx context.Context, userID int64, id string) ([]int64, error) {
	row, err := ownPlaylist(ctx, s.rq, userID, id)
	if err != nil {
		return nil, fmt.Errorf("playlist %s: %w", id, err)
	}
	rows, err := s.rq.ListPlaylistTrackRows(ctx, row)
	if err != nil {
		return nil, fmt.Errorf("playlist %s: tracks: %w", id, err)
	}
	return rows, nil
}
