package library

import (
	"context"
	"database/sql"
	"io/fs"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/simonhull/audiometa"
)

// supportedExtensions are the audio formats we handle.
var supportedExtensions = []string{".flac", ".mp3", ".m4a", ".m4b", ".ogg", ".opus"}

// ScanResult holds summary stats from a library scan.
type ScanResult struct {
	Added   int
	Skipped int
	Errors  int
}

// Scan walks root, reads metadata with audiometa, and upserts tracks into the database.
func Scan(ctx context.Context, db *sql.DB, root string, logger *slog.Logger) (*ScanResult, error) {
	if logger == nil {
		logger = slog.Default()
	}

	result := &ScanResult{}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO tracks (path, title, artist, album, album_artist, genre, year, track_num, disc_num, duration_ms, format, modified_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			title=excluded.title, artist=excluded.artist, album=excluded.album,
			album_artist=excluded.album_artist, genre=excluded.genre, year=excluded.year,
			track_num=excluded.track_num, disc_num=excluded.disc_num,
			duration_ms=excluded.duration_ms, format=excluded.format, modified_at=excluded.modified_at
	`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			result.Errors++
			return nil
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !slices.Contains(supportedExtensions, ext) {
			result.Skipped++
			return nil
		}

		info, err := d.Info()
		if err != nil {
			result.Errors++
			return nil
		}
		modTime := info.ModTime().UnixMilli()

		file, err := audiometa.Open(path)
		if err != nil {
			logger.Debug("scan error", "path", path, "error", err)
			result.Errors++
			return nil
		}
		defer file.Close()

		genre := ""
		if len(file.Tags.Genres) > 0 {
			genre = file.Tags.Genres[0]
		}

		title := file.Tags.Title
		if title == "" {
			base := filepath.Base(path)
			title = strings.TrimSuffix(base, filepath.Ext(base))
		}

		_, err = stmt.ExecContext(ctx, path, title, file.Tags.Artist, file.Tags.Album,
			file.Tags.AlbumArtist, genre, file.Tags.Year, file.Tags.TrackNumber,
			file.Tags.DiscNumber, file.Audio.Duration.Milliseconds(), file.Audio.Codec, modTime)
		if err != nil {
			logger.Debug("insert error", "path", path, "error", err)
			result.Errors++
			return nil
		}

		result.Added++
		return nil
	})

	if err != nil {
		return result, err
	}

	return result, tx.Commit()
}
