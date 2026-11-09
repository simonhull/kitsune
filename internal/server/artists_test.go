package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const artistsPath = "/artists"

func TestArtists(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 1}, browseAlbum{n: 2, sortName: "b", tracks: 1})
	var list api.ArtistList
	ts.getJSON(t, token, artistsPath, &list)
	// seedAlbums tags no album artist, so Ann owns her albums as their first track artist.
	if len(list.Artists) != 1 || list.Artists[0].Name != annName || list.Artists[0].AlbumCount != 2 {
		t.Fatalf("artists = %+v, want Ann with 2 albums", list.Artists)
	}

	var got api.ArtistDetail
	ts.getJSON(t, token, artistsPath+"/"+list.Artists[0].ID, &got)
	if got.Name != annName || got.AlbumCount != 2 || len(got.Albums) != 2 || got.AppearsOn == nil || len(got.AppearsOn) != 0 {
		t.Errorf("artist = %+v", got)
	}
}

func TestArtistsSayWhenAddedAndHowOftenPlayed(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 1}, browseAlbum{n: 2, sortName: "b", tracks: 1})
	for i := range 2 {
		wantNoContent(t, ts.send(t, call{
			method: http.MethodPost, path: "/me/plays", header: jsonHeader(token),
			body: fmt.Sprintf(`{"trackId":"01J%020d%03d","playedAt":"2026-01-01T00:00:00Z","source":"test","clientId":"c%d"}`, i+1, 1, i),
		}))
	}
	var list api.ArtistList
	ts.getJSON(t, token, artistsPath, &list)
	if len(list.Artists) != 1 {
		t.Fatalf("artists = %+v, want Ann", list.Artists)
	}
	if a := list.Artists[0]; a.Plays != 2 || !a.AddedAt.Equal(ts.Clock.Now()) {
		t.Errorf("Ann = %d plays, added %v; want 2, at %v", a.Plays, a.AddedAt, ts.Clock.Now())
	}
	// Plays are the caller's own.
	ts.getJSON(t, ts.tokenFor(t, store.RoleUser), artistsPath, &list)
	if list.Artists[0].Plays != 0 {
		t.Errorf("another person's plays = %d, want 0", list.Artists[0].Plays)
	}
}

func TestArtistsEmpty(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	var list api.ArtistList
	ts.getJSON(t, ts.tokenFor(t, store.RoleUser), artistsPath, &list)
	if list.Artists == nil || len(list.Artists) != 0 {
		t.Errorf("artists = %#v, want an empty list", list.Artists)
	}
}

func TestArtistNotFound(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	ts.send(t, call{method: http.MethodGet, path: artistsPath + "/" + missingID, header: bearer(token)}).problem(t, http.StatusNotFound)
}

func TestArtistsNeedCredentials(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	for _, path := range []string{artistsPath, artistsPath + "/x"} {
		ts.send(t, call{method: http.MethodGet, path: path, invalid: true}).problem(t, http.StatusUnauthorized)
	}
}

func TestAlbumLinksToItsArtist(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 1})

	var got api.AlbumDetail
	ts.getJSON(t, token, albumsPath+"/"+albumULID(1), &got)
	if len(got.Artists) != 1 || got.Artists[0].ID != annID || got.Artists[0].Name != annName {
		t.Errorf("artists = %+v, want a link to Ann", got.Artists)
	}
}
