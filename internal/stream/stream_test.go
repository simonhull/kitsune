package stream_test

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/stream"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	trackName = "track.flac"
	flac      = "FLAC"
	etagKey   = "ETag"
)

// serve runs [stream.Serve] for the file and returns what it wrote.
func serve(t *testing.T, file store.TrackFile) (*httptest.ResponseRecorder, error) {
	t.Helper()
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	if err := stream.Serve(rec, r, file); err != nil {
		return rec, fmt.Errorf("serve: %w", err)
	}
	return rec, nil
}

// writeTrack creates a track with the given bytes and modification time.
func writeTrack(t *testing.T, root, body string, mtime time.Time) store.TrackFile {
	t.Helper()
	path := filepath.Join(root, trackName)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	return store.TrackFile{Root: root, RelPath: trackName, Format: flac}
}

func TestServeReportsAMissingFile(t *testing.T) {
	t.Parallel()
	rec, err := serve(t, store.TrackFile{Root: t.TempDir(), RelPath: trackName, Format: flac})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist", err)
	}
	if len(rec.Header()) != 0 || rec.Body.Len() != 0 {
		t.Errorf("wrote %v %q, want nothing", rec.Header(), rec.Body)
	}
}

func TestServeTreatsADirectoryAsMissing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, trackName), 0o700); err != nil {
		t.Fatal(err)
	}
	rec, err := serve(t, store.TrackFile{Root: root, RelPath: trackName, Format: flac})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist", err)
	}
	if len(rec.Header()) != 0 {
		t.Errorf("wrote %v, want no headers", rec.Header())
	}
}

func TestServeTreatsAFileInPlaceOfAFolderAsMissing(t *testing.T) {
	t.Parallel()
	file := writeTrack(t, t.TempDir(), "x", time.Now())
	file.RelPath = filepath.Join(trackName, trackName)
	if _, err := serve(t, file); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist", err)
	}
}

func TestServeReportsOtherOpenErrors(t *testing.T) {
	t.Parallel()
	_, err := serve(t, store.TrackFile{Root: t.TempDir(), RelPath: strings.Repeat("x", 1000), Format: flac})
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want an open error other than fs.ErrNotExist", err)
	}
}

func TestServeETag(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mtime := time.Unix(1_762_000_000, 5000).UTC()
	etagOf := func(body string, at time.Time) string {
		t.Helper()
		rec, err := serve(t, writeTrack(t, root, body, at))
		if err != nil {
			t.Fatal(err)
		}
		return rec.Header().Get(etagKey)
	}
	base := etagOf("abcd", mtime)
	if !strings.HasPrefix(base, `"`) || !strings.HasSuffix(base, `"`) {
		t.Errorf("ETag = %s, want a strong validator", base)
	}
	if got := etagOf("wxyz", mtime); got != base {
		t.Errorf("same size and time: ETag = %s, want %s, the known limit", got, base)
	}
	if got := etagOf("abcde", mtime); got == base {
		t.Error("a new size kept the ETag")
	}
	if got := etagOf("abcd", mtime.Add(time.Millisecond)); got == base {
		t.Error("a new time within the same second kept the ETag")
	}
}

func TestServeSendsNoLastModified(t *testing.T) {
	t.Parallel()
	rec, err := serve(t, writeTrack(t, t.TempDir(), "abcd", time.Unix(1_762_000_000, 0)))
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.Header().Get("Last-Modified"); got != "" {
		t.Errorf("Last-Modified = %q, want none", got)
	}
}
