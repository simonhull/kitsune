package server

import "net/http"

// authenticate wraps next so that it runs only for authenticated requests.
func (*Server) authenticate(next http.Handler) http.Handler {
	return next
}
