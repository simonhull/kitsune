package server

import (
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

// toAPIUser builds the API's view of u field by field, so a new store column
// can't leak into a response.
func toAPIUser(u *store.User) api.User {
	return api.User{ID: u.ID, Name: u.Name, Role: string(u.Role), CreatedAt: u.CreatedAt}
}

// getMe returns the authenticated user.
func (*Server) getMe(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r.Context()) // authenticate ran, so ok.
	w.Header().Set("Cache-Control", "no-store")
	body := toAPIUser(&u)
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}
