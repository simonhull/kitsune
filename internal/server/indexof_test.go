package server

import (
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

// someAlbumsLibrary seeds three artists, two of them with two albums, and a
// compilation with no owner a reader shows, and returns a reader's token.
func (ts *testServer) someAlbumsLibrary(t *testing.T) string {
	t.Helper()
	ann, bob, cat := person(1, annName), person(2, "Bob"), person(3, "Cat")
	by := func(a store.ArtistWrite) []indexTrack {
		return []indexTrack{{title: firstTitle, trackNo: 1, artists: []store.ArtistWrite{a}}}
	}
	ts.seedIndex(t,
		indexAlbum{n: 1, title: "Ann Old", artist: annName, year: 1990, tracks: by(ann)},
		indexAlbum{n: 2, title: "Ann New", artist: annName, year: 2020, tracks: by(ann)},
		indexAlbum{n: 3, title: "Bob One", artist: "Bob", year: 2000, tracks: by(bob)},
		indexAlbum{n: 4, title: "Cat One", artist: "Cat", year: 2001, tracks: by(cat)},
		indexAlbum{n: 5, title: "Cat Two", artist: "Cat", year: 2002, tracks: by(cat)},
		indexAlbum{n: 6, title: "Hits", artist: variousName, compilation: true, owners: []store.ArtistWrite{various()}, tracks: by(bob)},
	)
	return ts.tokenFor(t, store.RoleUser)
}

// headingKey is what tells one heading from another: its artist, or for a
// heading with no artist, the name it shows.
func headingKey(a *api.LibraryArtist) string {
	if a.ID != "" {
		return a.ID
	}
	return "name:" + a.Name
}

// patchIndex is a client's patch: it takes the albums in ids out of whole, and
// puts those part holds where their after says. It reports false when an after
// names an album whole doesn't have.
func patchIndex(whole *api.LibraryIndex, ids []string, part *api.LibraryIndex) ([]api.LibraryArtist, bool) {
	var out []api.LibraryArtist
	for _, a := range whole.Artists {
		a.Albums = slices.DeleteFunc(slices.Clone(a.Albums), func(al api.LibraryAlbum) bool { return slices.Contains(ids, al.ID) })
		if len(a.Albums) > 0 {
			out = append(out, a)
		}
	}
	for _, heading := range part.Artists {
		for _, album := range heading.Albums {
			var ok bool
			if out, ok = placeAlbum(out, &heading, &album); !ok {
				return nil, false
			}
		}
	}
	return out, true
}

// placeAlbum puts album, of heading, right after the album its after names.
func placeAlbum(artists []api.LibraryArtist, heading *api.LibraryArtist, placed *api.LibraryAlbum) ([]api.LibraryArtist, bool) {
	album := *placed
	at, pos := -1, -1 // The heading and the position the album goes in.
	if *album.After != "" {
		for i := range artists {
			if j := slices.IndexFunc(artists[i].Albums, func(a api.LibraryAlbum) bool { return a.ID == *album.After }); j >= 0 {
				at, pos = i, j+1
			}
		}
		if at < 0 {
			return nil, false
		}
	}
	album.After = nil
	switch {
	case at >= 0 && headingKey(&artists[at]) == headingKey(heading):
		artists[at].Albums = slices.Insert(artists[at].Albums, pos, album)
	case at+1 < len(artists) && headingKey(&artists[at+1]) == headingKey(heading):
		artists[at+1].Albums = slices.Insert(artists[at+1].Albums, 0, album)
	default:
		h := *heading
		h.Albums = []api.LibraryAlbum{album}
		artists = slices.Insert(artists, at+1, h)
	}
	return artists, true
}

func TestLibraryIndexOfSomeAlbumsPatchesTheWhole(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.someAlbumsLibrary(t)
	var whole api.LibraryIndex
	ts.getJSON(t, token, indexPath, &whole)
	for name, ids := range map[string][]string{
		"one":                  {albumULID(3)},
		"the first":            {albumULID(2)},
		"a whole heading":      {albumULID(4), albumULID(5)},
		"the compilation":      {albumULID(6)},
		"one gone and one not": {albumULID(1), albumULID(99)},
		"every album":          {albumULID(1), albumULID(2), albumULID(3), albumULID(4), albumULID(5), albumULID(6)},
	} {
		var part api.LibraryIndex
		ts.getJSON(t, token, indexPath+"?albums="+strings.Join(ids, ","), &part)
		for _, a := range part.Artists {
			for _, al := range a.Albums {
				if al.After == nil {
					t.Fatalf("%s: album %s has no after", name, al.ID)
				}
			}
		}
		got, ok := patchIndex(&whole, ids, &part)
		if !ok || !slices.EqualFunc(got, whole.Artists, func(a, b api.LibraryArtist) bool { return fmt.Sprint(a) == fmt.Sprint(b) }) {
			t.Errorf("%s: patched = %+v, want the whole index back", name, got)
		}
	}
}

func TestLibraryIndexOfSomeAlbumsLeavesOutWhatIsGone(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.someAlbumsLibrary(t)
	var part api.LibraryIndex
	ts.getJSON(t, token, indexPath+"?albums="+albumULID(99), &part)
	if len(part.Artists) != 0 {
		t.Errorf("index of a gone album = %+v, want no artists", part)
	}
	ts.getJSON(t, token, indexPath+"?albums=", &part)
	if len(part.Artists) != 0 {
		t.Errorf("index of no albums = %+v, want no artists", part)
	}
}

func TestLibraryIndexOfSomeAlbumsHonorsFields(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.someAlbumsLibrary(t)
	for _, track := range []string{fmt.Sprintf("01J%020d%03d", 1, 0), fmt.Sprintf("01J%020d%03d", 3, 0)} {
		ts.send(t, call{
			method: http.MethodPost, path: "/me/plays", header: jsonHeader(token),
			body: `{"trackId":"` + track + `","playedAt":"2026-01-01T00:00:00Z","source":"test","clientId":"c"}`,
		})
	}
	var whole, part api.LibraryIndex
	ts.getJSON(t, token, indexPath+allIndexFields, &whole)
	var albums []api.LibraryAlbum
	var ids []string
	for _, a := range whole.Artists {
		for _, al := range a.Albums {
			albums, ids = append(albums, al), append(ids, al.ID)
		}
	}
	// Two albums or more, so a parameter after the list of them can't be misnumbered.
	ts.getJSON(t, token, indexPath+allIndexFields+"&albums="+strings.Join(ids, ","), &part)
	var got []api.LibraryAlbum
	for _, a := range part.Artists {
		for _, al := range a.Albums {
			al.After = nil
			got = append(got, al)
		}
	}
	if len(got) < 2 || !reflect.DeepEqual(got, albums) {
		t.Errorf("albums with fields = %+v, want %+v", got, albums)
	}
}

func TestLibraryIndexOfTooManyAlbums(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ids := make([]string, maxIndexAlbums+1)
	for i := range ids {
		ids[i] = albumULID(i)
	}
	r := ts.send(t, call{
		method: http.MethodGet, path: indexPath + "?albums=" + strings.Join(ids, ","),
		header: bearer(ts.tokenFor(t, store.RoleUser)), invalid: true,
	})
	r.problem(t, http.StatusBadRequest)
}

// BenchmarkLibraryIndexChange times GET /library/index?albums= for the ten
// albums a change names, gzipped, beside the whole index, on the large library.
func BenchmarkLibraryIndexChange(b *testing.B) {
	shape := benchShapes[len(benchShapes)-1]
	ts := newTestServer(b)
	token := ts.seedBench(b, shape.tracks, shape.albums, shape.artists)
	ids := make([]string, 10)
	for i := range ids {
		ids[i] = albumULID(1 + i*shape.albums/len(ids))
	}
	albums := "albums=" + strings.Join(ids, ",")
	for name, query := range map[string]string{
		"whole":               defaultIndexFields,
		"ten albums":          defaultIndexFields + "&" + albums,
		"ten albums, nothing": "?" + albums,
	} {
		b.Run(fmt.Sprintf("tracks=%d/%s", shape.tracks, name), func(b *testing.B) { benchIndex(b, ts, token, query) })
	}
}
