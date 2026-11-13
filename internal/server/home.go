package server

import (
	"net/http"
	"time"

	"github.com/simonhull/kitsune/api"
)

// onRepeatWindow is how far back Home's On repeat row counts plays.
const onRepeatWindow = 90 * 24 * time.Hour

// getHome answers with what Home shows of the caller's plays: the albums they
// played most recently, and the ones they played most in onRepeatWindow.
func (s *Server) getHome(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	h, err := s.library.History(r.Context(), user.ID, s.now().Add(-onRepeatWindow))
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "home history", err)
		}
		return
	}
	body := api.Home{Recent: s.toAPIAlbumEntries(h.Recent), OnRepeat: make([]api.AlbumPlays, len(h.OnRepeat))}
	for i := range h.OnRepeat {
		body.OnRepeat[i] = api.AlbumPlays{Album: s.toAPIAlbumEntry(&h.OnRepeat[i].Album), Plays: h.OnRepeat[i].Plays}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}
