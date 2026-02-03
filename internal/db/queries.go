package db

// ArtistRow is a single artist from the library.
type ArtistRow struct {
	ID         string
	Name       string
	AlbumCount int
}

// AlbumRow is a single album from the library.
type AlbumRow struct {
	ID         string
	Name       string
	ArtistID   string
	Year       int
	SongCount  int
	DurationMs int
	CoverArt   string
}

// TrackRow is a single track from the library.
type TrackRow struct {
	ID             string
	Title          string
	Artist         string
	Album          string
	AlbumID        string
	TrackNum       int
	DiscNum        int
	DurationMs     int
	Year           int
	Genre          string
	Format         string
	ShuffleExclude bool
	LinkedNextID   string
}

// AllArtists returns all artists, sorted alphabetically by name.
func (db *DB) AllArtists() ([]ArtistRow, error) {
	rows, err := db.Conn.Query(`
		SELECT id, name, album_count FROM artists ORDER BY name COLLATE NOCASE
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artists []ArtistRow
	for rows.Next() {
		var a ArtistRow
		if err := rows.Scan(&a.ID, &a.Name, &a.AlbumCount); err != nil {
			return nil, err
		}
		artists = append(artists, a)
	}
	return artists, rows.Err()
}

// AlbumsForArtist returns all albums for an artist, sorted by year then name.
func (db *DB) AlbumsForArtist(artistID string) ([]AlbumRow, error) {
	rows, err := db.Conn.Query(`
		SELECT id, name, artist_id, year, song_count, duration_ms, cover_art
		FROM albums WHERE artist_id = ? ORDER BY year, name COLLATE NOCASE
	`, artistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var albums []AlbumRow
	for rows.Next() {
		var a AlbumRow
		if err := rows.Scan(&a.ID, &a.Name, &a.ArtistID, &a.Year, &a.SongCount, &a.DurationMs, &a.CoverArt); err != nil {
			return nil, err
		}
		albums = append(albums, a)
	}
	return albums, rows.Err()
}

// TracksForArtist returns all tracks for an artist, ordered by album year, disc, track.
func (db *DB) TracksForArtist(artistID string) ([]TrackRow, error) {
	rows, err := db.Conn.Query(`
		SELECT t.id, t.title, t.artist, a.name, t.album_id, t.track_num, t.disc_num, t.duration_ms,
			a.year, t.genre, t.format, t.shuffle_exclude, COALESCE(t.linked_next_id, '')
		FROM tracks t
		JOIN albums a ON t.album_id = a.id
		WHERE t.artist_id = ?
		ORDER BY a.year, a.name COLLATE NOCASE, t.disc_num, t.track_num
	`, artistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []TrackRow
	for rows.Next() {
		var t TrackRow
		if err := rows.Scan(&t.ID, &t.Title, &t.Artist, &t.Album, &t.AlbumID, &t.TrackNum, &t.DiscNum,
			&t.DurationMs, &t.Year, &t.Genre, &t.Format, &t.ShuffleExclude, &t.LinkedNextID); err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}

// SearchResult holds a single search hit with its type.
type SearchResult struct {
	Kind     string // "artist", "album", "track"
	ID       string
	Title    string // name for artists, name for albums, title for tracks
	Artist   string
	Album    string
	AlbumID  string
	ArtistID string
	Year     int
}

// Search performs a fuzzy search across the library using FTS5.
// Returns up to `limit` results, grouped by type.
func (db *DB) Search(query string, limit int) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}

	// FTS5 prefix search: append * for partial matching.
	ftsQuery := query + "*"

	rows, err := db.Conn.Query(`
		SELECT
			t.id, t.title, t.artist, t.album, t.album_id, t.artist_id, a.year
		FROM tracks_fts fts
		JOIN tracks t ON t.rowid = fts.rowid
		JOIN albums a ON t.album_id = a.id
		WHERE tracks_fts MATCH ?
		ORDER BY fts.rank
		LIMIT ?
	`, ftsQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Deduplicate into artists, albums, and tracks.
	seenArtists := make(map[string]bool)
	seenAlbums := make(map[string]bool)
	var results []SearchResult

	for rows.Next() {
		var id, title, artist, album, albumID, artistID string
		var year int
		if err := rows.Scan(&id, &title, &artist, &album, &albumID, &artistID, &year); err != nil {
			return nil, err
		}

		// Emit unique artists.
		if !seenArtists[artistID] {
			seenArtists[artistID] = true
			results = append(results, SearchResult{
				Kind:     "artist",
				ID:       artistID,
				Title:    artist,
				ArtistID: artistID,
			})
		}

		// Emit unique albums.
		if !seenAlbums[albumID] {
			seenAlbums[albumID] = true
			results = append(results, SearchResult{
				Kind:     "album",
				ID:       albumID,
				Title:    album,
				Artist:   artist,
				AlbumID:  albumID,
				ArtistID: artistID,
				Year:     year,
			})
		}

		// Emit track.
		results = append(results, SearchResult{
			Kind:     "track",
			ID:       id,
			Title:    title,
			Artist:   artist,
			Album:    album,
			AlbumID:  albumID,
			ArtistID: artistID,
			Year:     year,
		})
	}

	return results, rows.Err()
}

// TracksForAlbum returns all tracks for an album, sorted by disc and track number.
func (db *DB) TracksForAlbum(albumID string) ([]TrackRow, error) {
	rows, err := db.Conn.Query(`
		SELECT t.id, t.title, t.artist, a.name, t.album_id, t.track_num, t.disc_num, t.duration_ms,
			a.year, t.genre, t.format, t.shuffle_exclude, COALESCE(t.linked_next_id, '')
		FROM tracks t
		JOIN albums a ON t.album_id = a.id
		WHERE t.album_id = ? ORDER BY t.disc_num, t.track_num
	`, albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []TrackRow
	for rows.Next() {
		var t TrackRow
		if err := rows.Scan(&t.ID, &t.Title, &t.Artist, &t.Album, &t.AlbumID, &t.TrackNum, &t.DiscNum,
			&t.DurationMs, &t.Year, &t.Genre, &t.Format, &t.ShuffleExclude, &t.LinkedNextID); err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}
