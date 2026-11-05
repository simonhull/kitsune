// Package stream serves a track's original file over HTTP, with byte ranges
// and validators. It never touches the audio: there is no transcoding, and
// clients decide for themselves what they can play.
package stream

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	// cacheControl lets a client keep a file but makes it ask first: a retag
	// changes the bytes under the same URL.
	cacheControl = "private, no-cache"
	// fallbackMediaType is the type of a format with no audio type of its own.
	fallbackMediaType = "application/octet-stream"
)

// Serve answers r with file, honoring Range, If-Range and If-None-Match.
//
// The size and time come from the open file, not the database, because the file
// may have changed since the scan. Serve passes [http.ServeContent] no
// modification time, so there is no Last-Modified and an If-Range holding a date
// never matches: only the ETag can resume a download, which closes a retag within
// the same second returning part of the changed bytes.
//
// It writes nothing when it returns an error. The error wraps [fs.ErrNotExist]
// if the file is gone, or if something other than a regular file took its place.
func Serve(w http.ResponseWriter, r *http.Request, file store.TrackFile) error {
	f, err := os.Open(filepath.Join(file.Root, file.RelPath))
	if err != nil {
		return fmt.Errorf("open track: %w", err)
	}
	defer func() { _ = f.Close() }() // Read-only.
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat track: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("open track: %s: %w", info.Mode().Type(), fs.ErrNotExist)
	}

	// The headers go on only now: a 404 must not carry them.
	h := w.Header()
	h.Set("Content-Type", mediaType(file.Format)) // Set, so ServeContent never sniffs.
	h.Set("Cache-Control", cacheControl)
	h.Set("ETag", etag(info))
	http.ServeContent(w, r, "", time.Time{}, f)
	return nil
}

// etag returns a strong validator from the file's size and modification time in
// nanoseconds. The audio digest can't serve: it covers audio only, so a retag
// keeps it and changes the bytes. A known limit: a retag that keeps both the size
// and the time keeps the ETag too.
func etag(info fs.FileInfo) string {
	return `"` + strconv.FormatInt(info.Size(), 16) + "-" + strconv.FormatInt(info.ModTime().UnixNano(), 16) + `"`
}

// mediaType returns the media type for a track's format, as the scanner records
// it. Case matters.
func mediaType(format string) string {
	switch format {
	case "FLAC":
		return "audio/flac"
	case "MP3":
		return "audio/mpeg"
	case "M4A", "M4B":
		return "audio/mp4"
	case "Ogg Vorbis", "Opus":
		return "audio/ogg"
	case "WAV":
		return "audio/wav"
	case "AIFF":
		return "audio/aiff"
	default:
		return fallbackMediaType
	}
}
