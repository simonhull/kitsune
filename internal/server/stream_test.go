package server

import (
	"crypto/sha256"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	acceptRanges  = "Accept-Ranges"
	contentRange  = "Content-Range"
	cacheControl  = "Cache-Control"
	contentType   = "Content-Type"
	etagHeader    = "ETag"
	ifRange       = "If-Range"
	rangeHeader   = "Range"
	streamCC      = "private, no-cache"
	streamFormat  = "FLAC"
	streamType    = "audio/flac"
	noSuchTrack   = "no such track"
	streamBody    = "0123456789abcdefghijklmnopqrstuvwxyz"
	streamTrackID = "01J00000000000000000000001"
)

// streamServer is a test server whose library root holds real files.
type streamServer struct {
	*testServer

	token string
	root  string
	seeds int
}

func newStreamServer(t *testing.T) *streamServer {
	t.Helper()
	ts := newTestServer(t)
	return &streamServer{testServer: ts, token: ts.sessionFor(t).token, root: t.TempDir()}
}

// seed records a live track of the given format and returns its relative path.
// The file itself is written by [streamServer.write].
func (ss *streamServer) seed(t *testing.T, id, format string) string {
	t.Helper()
	ss.seeds++
	rel := fmt.Sprintf("%d.audio", ss.seeds)
	ss.record(t, id, rel, format)
	return rel
}

// record writes a live track's row.
func (ss *streamServer) record(t *testing.T, id, rel, format string) {
	t.Helper()
	tw := store.TrackWrite{
		ULID: id, Root: ss.root, RelPath: rel,
		Digest: sha256.Sum256([]byte(id)),
		Format: format, Codec: format,
		Duration: time.Minute, Title: rel,
		Artists: []store.ArtistWrite{{ULID: annID, MatchKey: "ann", Name: annName, SortName: annName}},
		Album:   store.AlbumWrite{GroupKey: "g", ULID: albumULID(1), Name: "Album", SortName: "album", Artist: annName},
	}
	if err := ss.Store.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{tw}}); err != nil {
		t.Fatal(err)
	}
}

// write puts body at rel with the given modification time.
func (ss *streamServer) write(t *testing.T, rel, body string, mtime time.Time) {
	t.Helper()
	path := filepath.Join(ss.root, rel)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// track seeds the standard track, streamBody as FLAC, and returns its path.
func (ss *streamServer) track(t *testing.T) string {
	t.Helper()
	rel := ss.seed(t, streamTrackID, streamFormat)
	ss.write(t, rel, streamBody, streamTime)
	return rel
}

// streamTime is the modification time of the standard track.
var streamTime = time.Unix(1_762_000_000, 123_000_000) //nolint:gochecknoglobals // A time can't be a constant.

func streamPath(id string) string { return "/tracks/" + id + "/stream" }

// get streams id as the seeded user, adding the given headers in pairs.
func (ss *streamServer) get(t *testing.T, id string, pairs ...string) response {
	t.Helper()
	header := bearer(ss.token)
	for i := 0; i+1 < len(pairs); i += 2 {
		header.Set(pairs[i], pairs[i+1])
	}
	return ss.send(t, call{method: http.MethodGet, path: streamPath(id), header: header})
}

func TestStreamWholeFile(t *testing.T) {
	t.Parallel()
	ss := newStreamServer(t)
	ss.track(t)
	resp := ss.get(t, streamTrackID)
	if resp.Status != http.StatusOK || string(resp.Body) != streamBody {
		t.Fatalf("got %d %q, want 200 and the file", resp.Status, resp.Body)
	}
	for key, want := range map[string]string{
		contentType:      streamType,
		cacheControl:     streamCC,
		acceptRanges:     "bytes",
		"Content-Length": strconv.Itoa(len(streamBody)),
	} {
		if got := resp.Header.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if resp.Header.Get(etagHeader) == "" {
		t.Error("no ETag")
	}
	if got := resp.Header.Get("Last-Modified"); got != "" {
		t.Errorf("Last-Modified = %q, want none", got)
	}
}

func TestStreamWithCookie(t *testing.T) {
	t.Parallel()
	ss := newStreamServer(t)
	ss.track(t)
	resp := ss.send(t, call{method: http.MethodGet, path: streamPath(streamTrackID), header: cookieOnly(ss.token)})
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want 200: an audio element can only send the cookie", resp.Status)
	}
}

func TestStreamRanges(t *testing.T) {
	t.Parallel()
	ss := newStreamServer(t)
	ss.track(t)
	size := len(streamBody)
	tests := []struct {
		name, header string
		body         string
		contentRange string
	}{
		{"start", "bytes=0-3", "0123", fmt.Sprintf("bytes 0-3/%d", size)},
		{"middle", "bytes=10-12", "abc", fmt.Sprintf("bytes 10-12/%d", size)},
		{"open-ended", "bytes=33-", "xyz", fmt.Sprintf("bytes 33-35/%d", size)},
		{"suffix", "bytes=-2", "yz", fmt.Sprintf("bytes 34-35/%d", size)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := ss.get(t, streamTrackID, rangeHeader, tt.header)
			if resp.Status != http.StatusPartialContent || string(resp.Body) != tt.body {
				t.Fatalf("got %d %q, want 206 %q", resp.Status, resp.Body, tt.body)
			}
			if got := resp.Header.Get(contentRange); got != tt.contentRange {
				t.Errorf("Content-Range = %q, want %q", got, tt.contentRange)
			}
			if got := resp.Header.Get(contentType); got != streamType {
				t.Errorf("Content-Type = %q, want %q", got, streamType)
			}
			if resp.Header.Get(etagHeader) == "" || resp.Header.Get(cacheControl) != streamCC {
				t.Errorf("headers = %v, want the ETag and Cache-Control", resp.Header)
			}
		})
	}
}

func TestStreamMultipleRanges(t *testing.T) {
	t.Parallel()
	ss := newStreamServer(t)
	ss.track(t)
	resp := ss.get(t, streamTrackID, rangeHeader, "bytes=0-1,5-6")
	if resp.Status != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206: %s", resp.Status, resp.Body)
	}
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get(contentType))
	if err != nil || mediaType != "multipart/byteranges" {
		t.Fatalf("Content-Type = %q, %v; want multipart/byteranges", resp.Header.Get(contentType), err)
	}
	parts := multipart.NewReader(strings.NewReader(string(resp.Body)), params["boundary"])
	size := len(streamBody)
	for _, want := range []struct{ body, contentRange string }{
		{"01", fmt.Sprintf("bytes 0-1/%d", size)},
		{"56", fmt.Sprintf("bytes 5-6/%d", size)},
	} {
		checkPart(t, parts, want.body, want.contentRange)
	}
	if _, err = parts.NextPart(); err != io.EOF { //nolint:errorlint // NextPart returns io.EOF itself.
		t.Errorf("after the last part: %v, want io.EOF", err)
	}
}

// checkPart reads the next part and checks it.
func checkPart(t *testing.T, parts *multipart.Reader, wantBody, wantRange string) {
	t.Helper()
	part, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(part)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != wantBody || part.Header.Get(contentRange) != wantRange {
		t.Errorf("part = %q %q, want %q %q", body, part.Header.Get(contentRange), wantBody, wantRange)
	}
	if got := part.Header.Get(contentType); got != streamType {
		t.Errorf("part Content-Type = %q, want %q", got, streamType)
	}
}

func TestStreamNotModified(t *testing.T) {
	t.Parallel()
	ss := newStreamServer(t)
	ss.track(t)
	tag := ss.get(t, streamTrackID).Header.Get(etagHeader)
	tests := []struct {
		name  string
		pairs []string
	}{
		{"plain", []string{ifNoneMatch, tag}},
		{"with a range, as Chromium sends it", []string{ifNoneMatch, tag, rangeHeader, "bytes=0-"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := ss.get(t, streamTrackID, tt.pairs...)
			if resp.Status != http.StatusNotModified || len(resp.Body) != 0 {
				t.Fatalf("got %d %q, want an empty 304", resp.Status, resp.Body)
			}
			if resp.Header.Get(etagHeader) != tag || resp.Header.Get(cacheControl) != streamCC {
				t.Errorf("headers = %v, want the ETag and Cache-Control", resp.Header)
			}
		})
	}
}

func TestStreamStaleETagGetsTheFile(t *testing.T) {
	t.Parallel()
	ss := newStreamServer(t)
	ss.track(t)
	resp := ss.get(t, streamTrackID, ifNoneMatch, `"stale"`)
	if resp.Status != http.StatusOK || string(resp.Body) != streamBody {
		t.Errorf("got %d %q, want 200 and the file", resp.Status, resp.Body)
	}
}

func TestStreamUnsatisfiableRange(t *testing.T) {
	t.Parallel()
	ss := newStreamServer(t)
	ss.track(t)
	tests := []struct {
		name, header, contentRange string
	}{
		{"past the end", "bytes=1000-", fmt.Sprintf("bytes */%d", len(streamBody))},
		{"malformed", "bytes=x-y", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := ss.get(t, streamTrackID, rangeHeader, tt.header)
			resp.problem(t, http.StatusRequestedRangeNotSatisfiable)
			if got := resp.Header.Get(contentRange); got != tt.contentRange {
				t.Errorf("Content-Range = %q, want %q", got, tt.contentRange)
			}
			if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
			if resp.Header.Get(etagHeader) != "" || resp.Header.Get(cacheControl) != "" {
				t.Errorf("headers = %v, want no ETag or Cache-Control", resp.Header)
			}
		})
	}
}

func TestStreamIfRange(t *testing.T) {
	t.Parallel()
	ss := newStreamServer(t)
	ss.track(t)
	tag := ss.get(t, streamTrackID).Header.Get(etagHeader)

	resp := ss.get(t, streamTrackID, rangeHeader, "bytes=0-3", ifRange, tag)
	if resp.Status != http.StatusPartialContent || string(resp.Body) != "0123" {
		t.Errorf("current ETag: got %d %q, want 206", resp.Status, resp.Body)
	}
	date := streamTime.Add(time.Hour).UTC().Format(http.TimeFormat)
	resp = ss.get(t, streamTrackID, rangeHeader, "bytes=0-3", ifRange, date)
	if resp.Status != http.StatusOK || string(resp.Body) != streamBody {
		t.Errorf("a date: got %d %q, want 200 and the whole file", resp.Status, resp.Body)
	}
}

func TestStreamIfRangeAfterRetag(t *testing.T) {
	t.Parallel()
	retagged := strings.ToUpper(streamBody)
	tests := []struct {
		name  string
		body  string
		mtime time.Time
	}{
		{"new size", streamBody + "!", streamTime.Add(time.Minute)},
		{"same size, same second", retagged, streamTime.Add(time.Millisecond)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ss := newStreamServer(t)
			rel := ss.track(t)
			tag := ss.get(t, streamTrackID).Header.Get(etagHeader)
			date := streamTime.UTC().Format(http.TimeFormat)
			ss.write(t, rel, tt.body, tt.mtime)
			for _, validator := range []string{tag, date} {
				resp := ss.get(t, streamTrackID, rangeHeader, "bytes=0-3", ifRange, validator)
				if resp.Status != http.StatusOK || string(resp.Body) != tt.body {
					t.Errorf("If-Range %s: got %d %q, want 200 and the new file", validator, resp.Status, resp.Body)
				}
			}
			if got := ss.get(t, streamTrackID).Header.Get(etagHeader); got == tag {
				t.Errorf("ETag stayed %s", got)
			}
		})
	}
}

func TestStreamEveryFormat(t *testing.T) {
	t.Parallel()
	ss := newStreamServer(t)
	tests := []struct{ format, want string }{
		{"FLAC", "audio/flac"},
		{"MP3", "audio/mpeg"},
		{"M4A", "audio/mp4"},
		{"M4B", "audio/mp4"},
		{"Ogg Vorbis", "audio/ogg"},
		{"Opus", "audio/ogg"},
		{"WAV", "audio/wav"},
		{"AIFF", "audio/aiff"},
		{"Unknown", "application/octet-stream"},
		{"flac", "application/octet-stream"},
	}
	for i, tt := range tests {
		id := fmt.Sprintf("01J%023d", 100+i)
		ss.write(t, ss.seed(t, id, tt.format), streamBody, streamTime)
		resp := ss.get(t, id)
		if resp.Status != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", tt.format, resp.Status)
		}
		if got := resp.Header.Get(contentType); got != tt.want {
			t.Errorf("%s: Content-Type = %q, want %q", tt.format, got, tt.want)
		}
	}
}

func TestStreamNotFound(t *testing.T) {
	t.Parallel()
	ss := newStreamServer(t)
	ss.track(t)
	missing := ss.seed(t, "01J00000000000000000000002", streamFormat)
	ss.write(t, missing, streamBody, streamTime)
	states, err := ss.Store.FileStates(t.Context(), ss.root)
	if err != nil {
		t.Fatal(err)
	}
	if err = ss.Store.MarkMissing(t.Context(), []int64{states[missing].TrackID}, ss.Clock.Now()); err != nil {
		t.Fatal(err)
	}
	gone := "01J00000000000000000000003"
	ss.seed(t, gone, streamFormat) // No file was ever written.
	replaced := "01J00000000000000000000004"
	if err = os.Mkdir(filepath.Join(ss.root, ss.seed(t, replaced, streamFormat)), 0o700); err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, id string }{
		{"unknown", "01J00000000000000000009999"},
		{"malformed", missingID},
		{"missing", "01J00000000000000000000002"},
		{"file gone", gone},
		{"a directory in its place", replaced},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := ss.get(t, tt.id)
			if p := resp.problem(t, http.StatusNotFound); p.Detail != noSuchTrack {
				t.Errorf("detail = %q, want %q", p.Detail, noSuchTrack)
			}
			if resp.Header.Get(etagHeader) != "" || resp.Header.Get(cacheControl) != "" {
				t.Errorf("headers = %v, want no ETag or Cache-Control", resp.Header)
			}
		})
	}
}

func TestStreamOpenFailure(t *testing.T) {
	t.Parallel()
	ss := newStreamServer(t)
	ss.record(t, streamTrackID, strings.Repeat("x", 1000), streamFormat)
	resp := ss.send(t, call{
		method: http.MethodGet, path: streamPath(streamTrackID), header: bearer(ss.token), client: plainClient(),
	})
	resp.problem(t, http.StatusInternalServerError)
	if strings.Contains(string(resp.Body), ss.root) {
		t.Errorf("body %q names the path", resp.Body)
	}
	if !strings.Contains(ss.Log.String(), "stream track") {
		t.Errorf("log %q has no error line", ss.Log)
	}
}

func TestStreamRequiresAuthentication(t *testing.T) {
	t.Parallel()
	ss := newStreamServer(t)
	ss.track(t)
	resp := ss.send(t, call{method: http.MethodGet, path: streamPath(streamTrackID), invalid: true})
	resp.problem(t, http.StatusUnauthorized)
}
