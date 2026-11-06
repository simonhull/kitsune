package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

const (
	// webPage is the page that boots the client on any of its routes.
	webPage = "index.html"
	// webBuildDir is the build's own folder. A path under it is always a file.
	webBuildDir = "_app/"
	// webHashedDir holds files named by their content, so a name never changes meaning.
	webHashedDir = webBuildDir + "immutable/"
	// webHashedCache is the Cache-Control of a hashed file: a year.
	webHashedCache = "public, max-age=31536000, immutable"
	// webCache makes a browser ask again, so an upgrade shows at once.
	webCache = "no-cache"
)

// webHandler serves the built client in files, and its page for any path the
// client routes itself. What can't be the client is a 404, which
// [withProblems] turns into a problem: anything under /api, a write, and a
// file that isn't there. A stale chunk must fail, not parse a page as JavaScript.
func webHandler(files fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isRead := r.Method == http.MethodGet || r.Method == http.MethodHead
		if !isRead || r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		data, err := fs.ReadFile(files, name)
		if err != nil {
			if isWebFile(name) {
				http.NotFound(w, r)
				return
			}
			name = webPage
			if data, err = fs.ReadFile(files, name); err != nil {
				http.NotFound(w, r)
				return
			}
		}
		cache := webCache
		if strings.HasPrefix(name, webHashedDir) {
			cache = webHashedCache
		}
		w.Header().Set("Cache-Control", cache)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}

// isWebFile reports whether name reads as a file and not one of the client's
// routes, whose segments are words and ULIDs.
func isWebFile(name string) bool {
	return strings.HasPrefix(name, webBuildDir) || strings.Contains(name[strings.LastIndexByte(name, '/')+1:], ".")
}
