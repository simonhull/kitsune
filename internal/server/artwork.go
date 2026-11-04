package server

import (
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/simonhull/kitsune/internal/artwork"
)

const (
	// hashLen is the length of a hash in a URL: 32 bytes in lower case hex.
	hashLen = 2 * 32
	// defaultArtworkSize is the rendition served when the request names none.
	defaultArtworkSize = artwork.Size512
	// artworkCacheControl lets a client keep a rendition for a year. It is
	// immutable: the hash names the source image, and a rendition of it never
	// changes.
	artworkCacheControl = "private, max-age=31536000, immutable"
)

// getArtwork serves a cover rendition from the cache. Rendering happened
// during the scan, so a request only reads a file.
func (s *Server) getArtwork(w http.ResponseWriter, r *http.Request) {
	if s.artwork == nil {
		writeProblem(w, r, http.StatusNotFound, "no such artwork")
		return
	}
	req, err := parseArtworkRequest(r.PathValue("hash"), r.URL.Query())
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	hash, size := req.hash, req.size
	path, err := s.artwork.Path(hash, size)
	if err != nil {
		s.artworkLookupFailed(w, r, err)
		return
	}
	f, err := os.Open(path) //nolint:gosec // G304: the cache built this path from a validated hash.
	if err != nil {
		s.artworkLookupFailed(w, r, err)
		return
	}
	defer func() { _ = f.Close() }() // Read-only.

	// The headers go on only now: a 404 must not say immutable.
	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("Cache-Control", artworkCacheControl)
	h.Set("ETag", `"`+hex.EncodeToString(hash[:])+"-"+strconv.Itoa(size)+`"`)
	// Ranges are not in the contract: a 206 would fail a client that checks.
	r.Header.Del("Range")
	http.ServeContent(noRanges{w}, r, "", time.Time{}, f)
}

// artworkLookupFailed answers a rendition that is not there, which includes
// one pruned between the lookup and the open, or an error that is the
// server's.
func (s *Server) artworkLookupFailed(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, artwork.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		writeProblem(w, r, http.StatusNotFound, "no such artwork")
		return
	}
	s.internalError(w, r, "open artwork", err)
}

// parseArtworkRequest validates the hash in the path and the size in the
// query. A missing size is the default; any other value outside the ladder,
// an empty one included, is an error.
func parseArtworkRequest(rawHash string, q url.Values) (artworkRequest, error) {
	hash, err := parseHash(rawHash)
	if err != nil {
		return artworkRequest{}, err
	}
	size, err := parseSize(q)
	if err != nil {
		return artworkRequest{}, err
	}
	return artworkRequest{hash: hash, size: size}, nil
}

// artworkRequest is a validated GET /artwork/{hash}.
type artworkRequest struct {
	hash [32]byte
	size int
}

var (
	errBadHash = errors.New("hash must be 64 lower case hex digits")
	errBadSize = errors.New("size must be 128, 512 or 1024")
)

func parseHash(s string) ([32]byte, error) {
	var hash [32]byte
	if len(s) != hashLen {
		return hash, errBadHash
	}
	for i := range len(s) {
		if c := s[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return hash, errBadHash
		}
	}
	if _, err := hex.Decode(hash[:], []byte(s)); err != nil {
		return hash, errBadHash
	}
	return hash, nil
}

func parseSize(q url.Values) (int, error) {
	if !q.Has("size") {
		return defaultArtworkSize, nil
	}
	size, err := strconv.Atoi(q.Get("size"))
	if err != nil {
		return 0, errBadSize
	}
	switch size {
	case artwork.Size128, artwork.Size512, artwork.Size1024:
		return size, nil
	}
	return 0, errBadSize
}

// noRanges keeps [http.ServeContent] from advertising byte ranges the handler
// does not serve.
type noRanges struct{ http.ResponseWriter }

// WriteHeader implements [http.ResponseWriter].
func (n noRanges) WriteHeader(status int) {
	n.Header().Del("Accept-Ranges")
	n.ResponseWriter.WriteHeader(status)
}

// Unwrap lets [http.ResponseController] reach the underlying writer.
func (n noRanges) Unwrap() http.ResponseWriter { return n.ResponseWriter }
