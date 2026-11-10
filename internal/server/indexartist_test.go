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

// artistSongsLibrary seeds Ann's own albums, one of them a mix she is only the
// album artist of, a guest spot on Bob's album and a song on a compilation, and
// returns a reader's token.
func (ts *testServer) artistSongsLibrary(t *testing.T) string {
	t.Helper()
	a, bob, cat := person(1, annName), person(2, "Bob Guest"), person(3, "Cat Guest")
	song := func(title string, no int, by ...store.ArtistWrite) indexTrack {
		return indexTrack{title: title, trackNo: no, artists: by}
	}
	ts.seedIndex(t,
		indexAlbum{n: 1, title: "Ann Old", artist: annName, year: 1990, tracks: []indexTrack{song("Old 1", 1, a), song("Old 2", 2, a)}},
		indexAlbum{n: 2, title: "Ann New", artist: annName, year: 2020, tracks: []indexTrack{song("New 1", 1, a), song("New 2", 2, a)}},
		indexAlbum{
			n: 3, title: "Ann Mix", artist: annName, year: 2010, owners: []store.ArtistWrite{a},
			tracks: []indexTrack{song("Mixed 1", 1, cat), song("Mixed 2", 2, bob)},
		},
		indexAlbum{
			n: 4, title: "Bob One", artist: bob.Name, year: 2000,
			tracks: []indexTrack{song("Bob 1", 1, bob), song("Bob and Ann", 2, bob, a), song("Bob 3", 3, bob)},
		},
		indexAlbum{
			n: 5, title: "Hits", artist: variousName, compilation: true, owners: []store.ArtistWrite{various()},
			tracks: []indexTrack{song("Ann's Hit", 1, a), song("Cat's Hit", 2, cat)},
		},
		indexAlbum{n: 6, title: "Cat One", artist: cat.Name, year: 2001, tracks: []indexTrack{song("Cat 1", 1, cat)}},
	)
	return ts.tokenFor(t, store.RoleUser)
}

// albumSongs is each album of an index, by title, with its songs' titles.
func albumSongs(index *api.LibraryIndex) []string {
	var out []string
	for _, a := range index.Artists {
		for j := range a.Albums {
			al := &a.Albums[j]
			titles := make([]string, len(al.Tracks))
			for i, tr := range al.Tracks {
				titles[i] = tr.Title
			}
			out = append(out, al.Title+": "+strings.Join(titles, ", "))
		}
	}
	return out
}

func TestLibraryIndexOfAnArtist(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token, ann := ts.artistSongsLibrary(t), person(1, annName).ULID
	var got api.LibraryIndex
	ts.getJSON(t, token, indexPath+"?artist="+ann, &got)
	if len(got.Artists) != 1 || got.Artists[0].ID != ann || got.Artists[0].Name != annName {
		t.Fatalf("headings = %+v, want Ann's one", got.Artists)
	}
	// Her own albums whole, newest first, the mix she only heads included; then
	// the albums she appears on, each with just her songs.
	want := []string{
		"Ann New: New 1, New 2", "Ann Mix: Mixed 1, Mixed 2", "Ann Old: Old 1, Old 2",
		"Bob One: Bob and Ann", "Hits: Ann's Hit",
	}
	if titles := albumSongs(&got); !slices.Equal(titles, want) {
		t.Errorf("albums = %q, want %q", titles, want)
	}
	for _, al := range got.Artists[0].Albums {
		if al.After != nil {
			t.Errorf("album %s has an after", al.Title)
		}
	}

	var detail api.ArtistDetail
	ts.getJSON(t, token, artistsPath+"/"+ann, &detail)
	if detail.SongCount != 8 {
		t.Errorf("songCount = %d, want the 8 songs of her index", detail.SongCount)
	}
}

// An artist's songs carry what the whole index gives the same songs, fields
// included.
func TestLibraryIndexOfAnArtistHonorsFields(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token, ann := ts.artistSongsLibrary(t), person(1, annName).ULID
	track := fmt.Sprintf("01J%020d%03d", 4, 1) // Bob and Ann.
	ts.send(t, call{
		method: http.MethodPost, path: playsPath, header: jsonHeader(token),
		body: `{"trackId":"` + track + `","playedAt":"2026-01-01T00:00:00Z","source":"test","clientId":"c"}`,
	})
	ts.send(t, call{method: http.MethodPut, path: runsPath, header: jsonHeader(token), body: tracksBody(track, fmt.Sprintf("01J%020d%03d", 4, 2))})
	fields := allIndexFields + ",shuffle"
	var whole, part api.LibraryIndex
	ts.getJSON(t, token, indexPath+fields, &whole)
	ts.getJSON(t, token, indexPath+fields+"&artist="+ann, &part)
	songs := map[string]api.LibraryTrack{}
	for _, a := range whole.Artists {
		for _, al := range a.Albums {
			for _, tr := range al.Tracks {
				songs[tr.ID] = tr
			}
		}
	}
	n := 0
	for _, al := range part.Artists[0].Albums {
		for _, tr := range al.Tracks {
			n++
			if want := songs[tr.ID]; !reflect.DeepEqual(tr, want) {
				t.Errorf("song %s = %+v, want %+v", tr.Title, tr, want)
			}
		}
	}
	if n != 8 {
		t.Errorf("songs = %d, want 8", n)
	}
}

func TestLibraryIndexOfAnUnknownArtist(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.artistSongsLibrary(t)
	for _, id := range []string{missingID, various().ULID} {
		ts.send(t, call{method: http.MethodGet, path: indexPath + "?artist=" + id, header: bearer(token)}).
			problem(t, http.StatusNotFound)
	}
}

func TestLibraryIndexOfAnArtistAndAlbums(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token, ann := ts.artistSongsLibrary(t), person(1, annName).ULID
	ts.send(t, call{
		method: http.MethodGet, path: indexPath + "?artist=" + ann + "&albums=" + albumULID(1), header: bearer(token),
	}).problem(t, http.StatusBadRequest)
}
