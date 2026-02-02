package subsonic

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// SyncResult holds stats from a library sync.
type SyncResult struct {
	Artists int
	Albums  int
	Tracks  int
	Elapsed time.Duration
}

// Sync pulls the full library from a Subsonic server into the local SQLite cache.
// It upserts all data, preserving kitsune-specific metadata (shuffle_exclude, linked_next_id).
func Sync(ctx context.Context, client *Client, db *sql.DB, logger *slog.Logger) (*SyncResult, error) {
	if logger == nil {
		logger = slog.Default()
	}
	start := time.Now()
	result := &SyncResult{}

	// Fetch all artists.
	artists, err := client.GetArtists()
	if err != nil {
		return nil, fmt.Errorf("fetching artists: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	artistStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO artists (id, name, album_count)
		VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, album_count=excluded.album_count
	`)
	if err != nil {
		return nil, fmt.Errorf("preparing artist stmt: %w", err)
	}
	defer artistStmt.Close()

	albumStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO albums (id, name, artist_id, artist_name, year, song_count, duration_ms, cover_art)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, artist_id=excluded.artist_id, artist_name=excluded.artist_name,
			year=excluded.year, song_count=excluded.song_count, duration_ms=excluded.duration_ms,
			cover_art=excluded.cover_art
	`)
	if err != nil {
		return nil, fmt.Errorf("preparing album stmt: %w", err)
	}
	defer albumStmt.Close()

	trackStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO tracks (id, title, artist, album, album_id, artist_id, track_num, disc_num,
			duration_ms, genre, year, bitrate, format, cover_art)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title, artist=excluded.artist, album=excluded.album,
			album_id=excluded.album_id, artist_id=excluded.artist_id,
			track_num=excluded.track_num, disc_num=excluded.disc_num,
			duration_ms=excluded.duration_ms, genre=excluded.genre, year=excluded.year,
			bitrate=excluded.bitrate, format=excluded.format, cover_art=excluded.cover_art
	`)
	if err != nil {
		return nil, fmt.Errorf("preparing track stmt: %w", err)
	}
	defer trackStmt.Close()

	// Insert artists and fetch their albums + tracks.
	for _, a := range artists {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}

		if _, err := artistStmt.ExecContext(ctx, a.ID, a.Name, a.AlbumCount); err != nil {
			logger.Warn("artist insert failed", "artist", a.Name, "error", err)
			continue
		}
		result.Artists++

		// Fetch albums for this artist.
		detail, err := client.GetArtist(a.ID)
		if err != nil {
			logger.Warn("fetching artist albums failed", "artist", a.Name, "error", err)
			continue
		}

		for _, alb := range detail.Album {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}

			if _, err := albumStmt.ExecContext(ctx, alb.ID, alb.Name, alb.ArtistID, alb.Artist,
				alb.Year, alb.SongCount, alb.Duration*1000, alb.CoverArt); err != nil {
				logger.Warn("album insert failed", "album", alb.Name, "error", err)
				continue
			}
			result.Albums++

			// Fetch tracks for this album.
			albumDetail, err := client.GetAlbum(alb.ID)
			if err != nil {
				logger.Warn("fetching album tracks failed", "album", alb.Name, "error", err)
				continue
			}

			for _, s := range albumDetail.Song {
				if _, err := trackStmt.ExecContext(ctx, s.ID, s.Title, s.Artist, s.Album,
					s.AlbumID, s.ArtistID, s.TrackNum, s.DiscNum,
					s.Duration*1000, s.Genre, s.Year, s.BitRate, s.Suffix, s.CoverArt); err != nil {
					logger.Warn("track insert failed", "track", s.Title, "error", err)
					continue
				}
				result.Tracks++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("committing sync: %w", err)
	}

	result.Elapsed = time.Since(start)
	logger.Info("sync complete",
		"artists", result.Artists,
		"albums", result.Albums,
		"tracks", result.Tracks,
		"elapsed", result.Elapsed.Round(time.Millisecond),
	)

	return result, nil
}
