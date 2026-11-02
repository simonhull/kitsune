package server

import (
	"context"
	"crypto/rand"
	"net/http"
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
