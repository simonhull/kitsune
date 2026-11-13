package server

import (
	"net/http"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const listeningPath = "/me/listening"

func TestListeningCountsTheCallersOwn(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	other := ts.tokenFor(t, store.RoleUser)
	ts.heart(t, sess.user.ID, store.KindTrack, firstTrack) // A heart counts for nothing.
	for _, c := range []call{
		rateCall(sess.token, secondTrack, stars(4)),
		rateCall(other, firstTrack, stars(1)),
		{method: http.MethodPost, path: playsPath, body: playsBody(1, ts.Clock.Now()), header: jsonHeader(sess.token)},
	} {
		wantNoContent(t, ts.send(t, c))
	}
	var got api.Listening
	ts.getJSON(t, sess.token, listeningPath, &got)
	if want := (api.Listening{Plays: 1, Rated: 1}); got != want {
		t.Errorf("listening = %+v, want %+v", got, want)
	}
	ts.getJSON(t, other, listeningPath, &got)
	if want := (api.Listening{Rated: 1}); got != want {
		t.Errorf("the other's = %+v, want %+v", got, want)
	}
	ts.send(t, call{method: http.MethodGet, path: listeningPath, invalid: true}).problem(t, http.StatusUnauthorized)
}
