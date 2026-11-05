package server

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/simonhull/kitsune/api"
)

// maxDetail caps how much of a converted error body becomes a problem detail.
const maxDetail = 512

// problemNS is the prefix of the problem types Kitsune defines. A URN is a
// valid uri-reference and does not depend on the deployment's base URL.
const problemNS = "urn:kitsune:problem:"

// aboutBlank is the problem type for statuses with no Kitsune type.
const aboutBlank = "about:blank"

// problemType returns the stable problem type for an HTTP status. Statuses
// without a Kitsune type, such as 405, 413 and 416, get "about:blank".
func problemType(status int) string {
	if kind, ok := problemKinds[status]; ok {
		return problemNS + kind
	}
	return aboutBlank
}

// problemKinds names the statuses that have a Kitsune problem type.
//
//nolint:gochecknoglobals // A fixed lookup table, never written.
var problemKinds = map[int]string{
	http.StatusBadRequest:           "invalid-request",
	http.StatusUnauthorized:         "unauthorized",
	http.StatusForbidden:            "forbidden",
	http.StatusNotFound:             "not-found",
	http.StatusConflict:             "conflict",
	http.StatusPreconditionFailed:   "stale",
	http.StatusUnprocessableEntity:  "unprocessable",
	http.StatusPreconditionRequired: "precondition-required",
	http.StatusTooManyRequests:      "rate-limited",
	http.StatusInternalServerError:  "internal",
	http.StatusServiceUnavailable:   "unavailable",
}

// writeProblem writes an RFC 9457 problem with the given status and detail.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, detail string) {
	p := newProblem(r, status, detail)
	writeBody(w, status, api.ProblemMediaType, &p)
}

// newProblem returns the problem for status and detail.
func newProblem(r *http.Request, status int, detail string) api.Problem {
	return api.Problem{
		Type:      problemType(status),
		Title:     http.StatusText(status),
		Status:    status,
		Detail:    detail,
		RequestID: requestID(r.Context()),
	}
}

// writeBody marshals v and writes it with the given status and media type.
// v is one of the api types, which always marshal.
func writeBody(w http.ResponseWriter, status int, mediaType string, v any) {
	body, err := marshal(v)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h := w.Header()
	// Headers that described the body the handler meant to send do not
	// describe this one.
	for _, k := range []string{"Content-Length", "Content-Encoding", "Content-Disposition", "ETag", "Last-Modified"} {
		h.Del(k)
	}
	h.Set("Content-Type", mediaType)
	w.WriteHeader(status)
	_, _ = w.Write(body) //nolint:gosec // G705: a JSON body of api types with a JSON media type; browsers do not render it as HTML.
}

// marshal encodes v. Raw tags straight from audio files may hold invalid
// UTF-8, which json/v2 rejects by default; it is replaced with U+FFFD instead.
func marshal(v any) ([]byte, error) {
	b, err := json.Marshal(v, jsontext.AllowInvalidUTF8(true))
	if err != nil {
		return nil, fmt.Errorf("marshal response: %w", err)
	}
	return b, nil
}

// problemWriter turns plain-text error responses written by code outside the
// API's control (ServeMux's 404 and 405, ServeContent's 416) into problems.
// A handler that already wrote a problem passes through untouched.
type problemWriter struct {
	http.ResponseWriter

	wroteHeader bool
	// status is non-zero while an error response is being converted; its
	// body collects into detail and is written by finish.
	status int
	detail bytes.Buffer
}

// withProblems wraps next so that every error response is a problem.
func withProblems(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pw := &problemWriter{ResponseWriter: w}
		next.ServeHTTP(pw, r)
		pw.finish(r)
	})
}

// WriteHeader implements [http.ResponseWriter].
func (w *problemWriter) WriteHeader(status int) {
	switch {
	case w.status != 0:
		return // An error is being converted; finish writes its status.
	case w.wroteHeader:
		w.ResponseWriter.WriteHeader(status) // Let net/http report the superfluous call.
		return
	case status < http.StatusOK:
		w.ResponseWriter.WriteHeader(status) // Informational; the final status follows.
		return
	}
	w.wroteHeader = true
	if status >= http.StatusBadRequest && !isProblem(w.Header().Get("Content-Type")) {
		w.status = status
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

// Write implements [http.ResponseWriter].
func (w *problemWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.status != 0 {
		w.detail.Write(p[:min(len(p), maxDetail-w.detail.Len())])
		return len(p), nil
	}
	n, err := w.ResponseWriter.Write(p)
	if err != nil {
		return n, fmt.Errorf("write response: %w", err)
	}
	return n, nil
}

// ReadFrom implements [io.ReaderFrom]. [io.Copy] looks for it and nothing else,
// so without it a served file would lose sendfile and pass through a buffer.
// Like Write it sends the implicit 200, but it discards the body of an error
// being converted: the error keeps its status and loses only its detail.
func (w *problemWriter) ReadFrom(src io.Reader) (int64, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	dst := io.Writer(w.ResponseWriter)
	if w.status != 0 {
		dst = io.Discard
	}
	n, err := io.Copy(dst, src)
	if err != nil {
		return n, fmt.Errorf("write response: %w", err)
	}
	return n, nil
}

// isProblem reports whether contentType is the problem media type, with or
// without parameters.
func isProblem(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == api.ProblemMediaType
}

// Unwrap lets [http.ResponseController] reach the underlying writer.
func (w *problemWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// finish writes the converted problem, if any.
func (w *problemWriter) finish(r *http.Request) {
	if w.status == 0 {
		return
	}
	writeProblem(w.ResponseWriter, r, w.status, strings.TrimSpace(w.detail.String()))
}
