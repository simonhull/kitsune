package server

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// requestIDKey is the context key for the request ID.
type requestIDKey struct{}

// withRequestID gives each request a random ID, in the context for problems
// and logs and in the X-Request-ID response header.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := rand.Text()
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

// requestID returns the request ID from ctx, or "" outside [withRequestID].
func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string) // Absent means no ID.
	return id
}

// withBasePath serves next under the server's base path: it strips the path
// and answers every request outside it with a 404, which [withProblems]
// turns into a problem. With no base path it returns next unchanged.
//
// A known limit: ServeMux's path-cleaning redirects, such as
// /music//healthz to /healthz, drop the base path. Canonical API paths never
// trigger one, and it is not an open redirect.
func (s *Server) withBasePath(next http.Handler) http.Handler {
	if s.basePath == "" {
		return next
	}
	strip := http.StripPrefix(s.basePath, next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The escaped path: /music%2Fhealthz decodes to /music/healthz, but
		// StripPrefix would leave a RawPath with no leading slash.
		if !strings.HasPrefix(r.URL.EscapedPath(), s.basePath+"/") {
			http.NotFound(w, r)
			return
		}
		strip.ServeHTTP(w, r)
	})
}

// withAccessLog logs each request at debug level, with the path only: the
// query can hold credentials. Above debug it costs nothing.
func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if !s.log.Enabled(ctx, slog.LevelDebug) {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		s.log.DebugContext(ctx, "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"bytes", rec.bytes,
			"duration", time.Since(start),
			"requestId", requestID(ctx),
			"client", s.clientIP(r),
		)
	})
}

// statusRecorder remembers the status and size of a response.
type statusRecorder struct {
	http.ResponseWriter

	status int // The first final status; 0 until one is written.
	bytes  int64
}

// WriteHeader implements [http.ResponseWriter].
func (w *statusRecorder) WriteHeader(status int) {
	if w.status == 0 && status >= http.StatusOK {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

// Write implements [http.ResponseWriter].
func (w *statusRecorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	if err != nil {
		return n, fmt.Errorf("write response: %w", err)
	}
	return n, nil
}

// Unwrap lets [http.ResponseController] reach the underlying writer.
func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }
