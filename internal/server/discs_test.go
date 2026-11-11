package server

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/store"
)

// The names of the disc tests: three albums and two discs.
const (
	glasshouse = "Glasshouse"
	plainAlbum = "Plain"
	discOne    = "The Electric Set"
	discTwo    = "The Room Recordings"
)

// seedDiscs writes three albums: two discs with names, the first named only
// from its second song; one disc with none; and one disc with a name.
func (ts *testServer) seedDiscs(t *testing.T) {
	t.Helper()
	by := []store.ArtistWrite{person(1, annName)}
	ts.seedIndex(t,
		indexAlbum{n: 1, title: glasshouse, artist: annName, tracks: []indexTrack{
			{title: "Breakwater", disc: 1, trackNo: 1, artists: by},
			{title: "Salt", disc: 1, trackNo: 2, discName: discOne, artists: by},
			{title: "Fogbound", disc: 2, trackNo: 1, discName: discTwo, artists: by},
		}},
		indexAlbum{n: 2, title: plainAlbum, artist: annName, tracks: []indexTrack{
			{title: "One", trackNo: 1, artists: by},
			{title: "Two", trackNo: 2, artists: by},
		}},
		indexAlbum{n: 3, title: "Named", artist: annName, tracks: []indexTrack{
			{title: "Solo", disc: 1, trackNo: 1, discName: "Live", artists: by},
		}},
	)
}

// twoDiscs are the discs of the first album [testServer.seedDiscs] writes.
func twoDiscs() []api.Disc {
	return []api.Disc{
		{No: 1, Name: discOne, Songs: 2, DurationMs: 120_000},
		{No: 2, Name: discTwo, Songs: 1, DurationMs: 60_000},
	}
}

// discsByTitle returns each album's discs in an index, by its title.
func discsByTitle(index *api.LibraryIndex) map[string][]api.Disc {
	out := map[string][]api.Disc{}
	for i := range index.Artists {
		albums := index.Artists[i].Albums
		for j := range albums {
			out[albums[j].Title] = albums[j].Discs
		}
	}
	return out
}

func TestLibraryIndexSendsDiscsOnlyWhenTheySaySomething(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedDiscs(t)
	token := ts.tokenFor(t, store.RoleUser)
	var index api.LibraryIndex
	ts.getJSON(t, token, indexPath, &index)
	got := discsByTitle(&index)
	if !slices.Equal(got[glasshouse], twoDiscs()) {
		t.Errorf("two discs = %+v, want %+v", got[glasshouse], twoDiscs())
	}
	if d := got[plainAlbum]; d != nil {
		t.Errorf("one unnamed disc = %+v, want none sent", d)
	}
	if d, want := got["Named"], []api.Disc{{No: 1, Name: "Live", Songs: 1, DurationMs: 60_000}}; !slices.Equal(d, want) {
		t.Errorf("one named disc = %+v, want %+v", d, want)
	}
	// A partial index, the one an event patches in, says the same.
	var part api.LibraryIndex
	ts.getJSON(t, token, indexPath+"?albums="+albumULID(1)+","+albumULID(2), &part)
	if partial := discsByTitle(&part); !slices.Equal(partial[glasshouse], twoDiscs()) || partial[plainAlbum] != nil {
		t.Errorf("partial index discs = %+v", partial)
	}
}

func TestAlbumCarriesItsDiscsAndEachTracksDiscName(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedDiscs(t)
	token := ts.tokenFor(t, store.RoleUser)
	var two api.AlbumDetail
	ts.getJSON(t, token, albumsPath+"/"+albumULID(1), &two)
	if !slices.Equal(two.Discs, twoDiscs()) {
		t.Errorf("discs = %+v, want %+v", two.Discs, twoDiscs())
	}
	names := make([]string, len(two.Tracks))
	for i := range two.Tracks {
		names[i] = two.Tracks[i].DiscSubtitle
	}
	if want := []string{"", discOne, discTwo}; !slices.Equal(names, want) {
		t.Errorf("track disc names = %q, want %q", names, want)
	}
	// An album on one disc still has that one, so a client needn't count.
	var one api.AlbumDetail
	ts.getJSON(t, token, albumsPath+"/"+albumULID(2), &one)
	if want := []api.Disc{{No: 1, Songs: 2, DurationMs: 120_000}}; !slices.Equal(one.Discs, want) {
		t.Errorf("one disc = %+v, want %+v", one.Discs, want)
	}
	resp := ts.send(t, call{method: http.MethodGet, path: albumsPath + "/" + albumULID(2), header: bearer(token)})
	if strings.Contains(string(resp.Body), "discSubtitle") {
		t.Errorf("an untagged disc name was sent: %s", resp.Body)
	}
}

func TestEditOfADiscNameWritesItAndSaysTheAlbumChanged(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
	s := fx.openStream(t, fx.token, "")
	s.want(events.Ready)
	before := fx.album
	var got api.AlbumDetail
	fx.post(t, before.Etag, fx.trackBody(0, `{"discSubtitle":["  `+discTwo+` "]}`)).decode(t, &got)
	i := slices.IndexFunc(got.Tracks, func(tr api.Track) bool { return tr.ID == before.Tracks[0].ID })
	if name := got.Tracks[i].DiscSubtitle; name != discTwo {
		t.Errorf("track's disc name = %q, want it trimmed", name)
	}
	if want := []api.Disc{{No: 1, Name: discTwo, Songs: 2, DurationMs: got.DurationMs}}; !slices.Equal(got.Discs, want) {
		t.Errorf("discs = %+v, want %+v", got.Discs, want)
	}
	// Only the name changed, and the index shows it: the album's row must follow.
	var change api.EventLibrary
	decodeEvent(t, s.until(events.Library), &change)
	if change.All || !slices.Contains(change.Albums, before.ID) {
		t.Errorf("library event = %+v, want album %s", change, before.ID)
	}
	fx.wantStoreFollowed(t, &before, &got)
}

// mixedDiscOrder is how every form of the index lists seedMixedDiscs's album:
// the disc first, then the songs with no disc number.
func mixedDiscOrder() []string {
	return []string{"Main Title", "Arrival", "Reprise A", "Reprise B", "Coda"}
}

// seedMixedDiscs writes a soundtrack whose first songs were tagged disc 1 and
// whose others have no disc number, their numbers starting over.
func (ts *testServer) seedMixedDiscs(t *testing.T) {
	t.Helper()
	by := []store.ArtistWrite{person(1, annName)}
	ts.seedIndex(t, indexAlbum{n: 1, title: "Score", artist: annName, tracks: []indexTrack{
		{title: "Main Title", disc: 1, trackNo: 1, artists: by},
		{title: "Reprise A", trackNo: 1, artists: by},
		{title: "Arrival", disc: 1, trackNo: 2, artists: by},
		{title: "Reprise B", trackNo: 1, artists: by},
		{title: "Coda", trackNo: 2, artists: by},
	}})
}

func TestSongsWithNoDiscNumberBesideADiscAreTheirOwnGroupLast(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedMixedDiscs(t)
	token := ts.tokenFor(t, store.RoleUser)
	wantOrder := mixedDiscOrder()
	wantDiscs := []api.Disc{
		{No: 1, Songs: 2, DurationMs: 120_000},
		{Songs: 3, DurationMs: 180_000},
	}

	var album api.AlbumDetail
	ts.getJSON(t, token, albumsPath+"/"+albumULID(1), &album)
	titles := make([]string, len(album.Tracks))
	for i := range album.Tracks {
		titles[i] = album.Tracks[i].Title
	}
	if !slices.Equal(titles, wantOrder) {
		t.Errorf("album order = %q, want %q", titles, wantOrder)
	}
	if !slices.Equal(album.Discs, wantDiscs) {
		t.Errorf("album discs = %+v, want %+v", album.Discs, wantDiscs)
	}

	var index api.LibraryIndex
	ts.getJSON(t, token, indexPath, &index)
	listed := index.Artists[0].Albums[0]
	titles = make([]string, len(listed.Tracks))
	for i := range listed.Tracks {
		titles[i] = listed.Tracks[i].Title
	}
	if !slices.Equal(titles, wantOrder) {
		t.Errorf("index order = %q, want %q", titles, wantOrder)
	}
	if !slices.Equal(listed.Discs, wantDiscs) {
		t.Errorf("index discs = %+v, want %+v", listed.Discs, wantDiscs)
	}

	// The group with no disc number says so by having no number at all.
	resp := ts.send(t, call{method: http.MethodGet, path: albumsPath + "/" + albumULID(1), header: bearer(token)})
	if !strings.Contains(string(resp.Body), `{"songs":3,"durationMs":180000}`) {
		t.Errorf("the group with no disc number was sent as %s", resp.Body)
	}
}

// Every form of the index lists an album's songs in one order: the whole
// library, some albums, and one artist's.
func TestEveryFormOfTheIndexKeepsOneDiscOrder(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedMixedDiscs(t)
	token := ts.tokenFor(t, store.RoleUser)
	want := mixedDiscOrder()
	for _, path := range []string{
		indexPath,
		indexPath + "?albums=" + albumULID(1),
		indexPath + "?artist=" + person(1, annName).ULID,
	} {
		var index api.LibraryIndex
		ts.getJSON(t, token, path, &index)
		if len(index.Artists) == 0 || len(index.Artists[0].Albums) == 0 {
			t.Fatalf("%s: no album in %+v", path, index)
		}
		listed := index.Artists[0].Albums[0]
		titles := make([]string, len(listed.Tracks))
		for i := range listed.Tracks {
			titles[i] = listed.Tracks[i].Title
		}
		if !slices.Equal(titles, want) {
			t.Errorf("%s: order = %q, want %q", path, titles, want)
		}
	}
}
