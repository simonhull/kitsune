package server

import (
	"crypto/sha256"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	indexPath   = "/library/index"
	variousName = "Various Artists"
	firstTitle  = "One"
)

// indexAlbum builds the tracks of one album. Each edit may change a track, and
// the default track has the album's own artist as its only artist.
type indexAlbum struct {
	n           int
	title       string
	artist      string // The display artist.
	year        int
	compilation bool
	owners      []store.ArtistWrite // Album artists; none leaves the first track artist.
	tracks      []indexTrack
}

// indexTrack is a track of an [indexAlbum].
type indexTrack struct {
	title         string
	disc, trackNo int
	artists       []store.ArtistWrite
}

// person returns the artist write for a name, with a ULID of its own.
func person(n int, name string) store.ArtistWrite {
	return store.ArtistWrite{ULID: fmt.Sprintf("04J%023d", n), MatchKey: name, Name: name, SortName: name}
}

// various is the row no reader shows.
func various() store.ArtistWrite {
	return store.ArtistWrite{ULID: fmt.Sprintf("04J%023d", 0), MatchKey: "various artists", Name: variousName, SortName: variousName}
}

func (ts *testServer) seedIndex(tb testing.TB, albums ...indexAlbum) {
	tb.Helper()
	var tws []store.TrackWrite
	for _, a := range albums {
		for i, t := range a.tracks {
			tws = append(tws, store.TrackWrite{
				ULID:    fmt.Sprintf("01J%020d%03d", a.n, i),
				Root:    "/music",
				RelPath: fmt.Sprintf("a%d/%d.flac", a.n, i),
				Digest:  sha256.Sum256(fmt.Appendf(nil, "%d/%d", a.n, i)),
				Format:  flacFormat, Codec: flacFormat,
				Duration: time.Minute,
				Title:    t.title, DiscNo: t.disc, TrackNo: t.trackNo,
				Artists: t.artists, AlbumArtists: a.owners,
				Album: store.AlbumWrite{
					GroupKey: fmt.Sprintf("g%d", a.n), ULID: albumULID(a.n), Name: a.title, SortName: a.title,
					Artist: a.artist, Year: a.year, Compilation: a.compilation,
				},
			})
		}
	}
	if err := ts.Store.WriteBatch(tb.Context(), &store.Batch{Tracks: tws}); err != nil {
		tb.Fatal(err)
	}
}

// index reads the library index as a signed-in user.
func (ts *testServer) index(t *testing.T) api.LibraryIndex {
	t.Helper()
	var out api.LibraryIndex
	ts.getJSON(t, ts.tokenFor(t, store.RoleUser), indexPath, &out)
	return out
}

// artistNames returns the headings of the index, in order.
func artistNames(index *api.LibraryIndex) []string {
	names := make([]string, len(index.Artists))
	for i := range index.Artists {
		names[i] = index.Artists[i].Name
	}
	return names
}

func albumTitles(a *api.LibraryArtist) []string {
	titles := make([]string, len(a.Albums))
	for i := range a.Albums {
		titles[i] = a.Albums[i].Title
	}
	return titles
}

func TestLibraryIndexPutsAnAlbumUnderItsFirstOwnerOnce(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ann, bob := person(1, annName), person(2, "Bob")
	ts.seedIndex(t, indexAlbum{
		n: 1, title: "Shared", artist: "Ann, Bob", owners: []store.ArtistWrite{ann, bob},
		tracks: []indexTrack{{title: firstTitle, trackNo: 1, artists: []store.ArtistWrite{ann}}},
	})
	index := ts.index(t)
	if got := artistNames(&index); !slices.Equal(got, []string{annName}) {
		t.Fatalf("artists = %v, want only the first owner", got)
	}
	if a := index.Artists[0]; a.ID != ann.ULID || len(a.Albums) != 1 || a.Albums[0].Artist != "Ann, Bob" {
		t.Errorf("artist = %+v", a)
	}
}

func TestLibraryIndexGroupsACompilationUnderItsDisplayedArtist(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ann, bob := person(1, annName), person(2, "Bob")
	tracks := []indexTrack{
		{title: firstTitle, trackNo: 1, artists: []store.ArtistWrite{ann}},
		{title: "Two", trackNo: 2, artists: []store.ArtistWrite{bob}},
	}
	ts.seedIndex(t,
		indexAlbum{n: 1, title: "Sampler", artist: variousName, compilation: true, tracks: tracks},
		// Tagged with the row no reader shows: the same heading, and no link either.
		indexAlbum{n: 2, title: "Tagged", artist: variousName, owners: []store.ArtistWrite{various()}, tracks: tracks},
	)
	index := ts.index(t)
	if got := artistNames(&index); !slices.Equal(got, []string{variousName}) {
		t.Fatalf("artists = %v", got)
	}
	group := index.Artists[0]
	if group.ID != "" || group.SortName != variousName {
		t.Errorf("group = %+v, want no ID", group)
	}
	if len(group.Albums) != 2 {
		t.Fatalf("albums = %v", albumTitles(&group))
	}
	sampler := group.Albums[0]
	if !sampler.Compilation {
		t.Errorf("album = %+v, want a compilation", sampler)
	}
	if got := sampler.Tracks[1].Artists; !slices.Equal(got, []string{"Bob"}) {
		t.Errorf("track artists = %v, want its own", got)
	}
}

func TestLibraryIndexLeavesOutWhatIsMissing(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ann := person(1, annName)
	own := []store.ArtistWrite{ann}
	ts.seedIndex(t,
		indexAlbum{n: 1, title: "Half", artist: annName, tracks: []indexTrack{
			{title: "Stays", trackNo: 1, artists: own}, {title: "Goes", trackNo: 2, artists: own},
		}},
		indexAlbum{n: 2, title: "Gone", artist: annName, tracks: []indexTrack{{title: "Goes too", trackNo: 1, artists: own}}},
	)
	states, err := ts.Store.FileStates(t.Context(), "/music")
	if err != nil {
		t.Fatal(err)
	}
	gone := []int64{states["a1/1.flac"].TrackID, states["a2/0.flac"].TrackID}
	if err = ts.Store.MarkMissing(t.Context(), gone, ts.Clock.Now()); err != nil {
		t.Fatal(err)
	}
	index := ts.index(t)
	if len(index.Artists) != 1 || len(index.Artists[0].Albums) != 1 {
		t.Fatalf("index = %+v, want one album", index)
	}
	album := index.Artists[0].Albums[0]
	if album.Title != "Half" || len(album.Tracks) != 1 || album.Tracks[0].Title != "Stays" {
		t.Errorf("album = %+v", album)
	}
}

func TestLibraryIndexOrder(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ann := person(1, annName)
	own := []store.ArtistWrite{ann}
	ts.seedIndex(t,
		indexAlbum{n: 1, title: "Unknown year", artist: annName, tracks: []indexTrack{{title: "A", trackNo: 1, artists: own}}},
		indexAlbum{n: 2, title: "Old", artist: annName, year: 1999, tracks: []indexTrack{{title: "B", trackNo: 1, artists: own}}},
		indexAlbum{n: 3, title: "New", artist: annName, year: 2021, tracks: []indexTrack{
			{title: "Disc two", disc: 2, trackNo: 1, artists: own},
			{title: "No number", disc: 1, artists: own},
			{title: "Second", disc: 1, trackNo: 2, artists: own},
			{title: "First", trackNo: 1, artists: own}, // No disc counts as disc 1.
		}},
	)
	index := ts.index(t)
	if got := albumTitles(&index.Artists[0]); !slices.Equal(got, []string{"New", "Old", "Unknown year"}) {
		t.Errorf("albums = %v, want newest first and no year last", got)
	}
	tracks := index.Artists[0].Albums[0].Tracks
	titles := make([]string, len(tracks))
	for i := range tracks {
		titles[i] = tracks[i].Title
	}
	if want := []string{"First", "Second", "No number", "Disc two"}; !slices.Equal(titles, want) {
		t.Errorf("tracks = %v, want %v", titles, want)
	}
	// The same order an album's page has.
	var detail api.AlbumDetail
	ts.getJSON(t, ts.tokenFor(t, store.RoleAdmin), albumsPath+"/"+albumULID(3), &detail)
	for i, tr := range detail.Tracks {
		if tr.ID != tracks[i].ID {
			t.Errorf("track %d differs from the album's order", i)
		}
	}
}

func TestLibraryIndexFilesArtistsByLetter(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	names := []store.ArtistWrite{
		person(1, "Zed"), person(2, "élan"), person(3, "Elbow"), person(4, "3 Doors"), person(5, "東京"),
		{ULID: fmt.Sprintf("04J%023d", 6), MatchKey: "the band", Name: "The Band", SortName: "Band, The"},
	}
	albums := make([]indexAlbum, len(names))
	for i, a := range names {
		albums[i] = indexAlbum{
			n: i + 1, title: a.Name, artist: a.Name,
			tracks: []indexTrack{{title: firstTitle, trackNo: 1, artists: []store.ArtistWrite{a}}},
		}
	}
	ts.seedIndex(t, albums...)
	index := ts.index(t)
	want := []string{"3 Doors", "東京", "The Band", "Elbow", "élan", "Zed"}
	if got := artistNames(&index); !slices.Equal(got, want) {
		t.Errorf("artists = %v, want %v", got, want)
	}
}

func TestLibraryIndexRequiresAuth(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.send(t, call{method: http.MethodGet, path: indexPath, invalid: true}).problem(t, http.StatusUnauthorized)
}

// Full size, as the spec measured it.
const (
	benchArtists = 912
	benchAlbums  = 927
	benchTracks  = 7586
)

func BenchmarkLibraryIndex(b *testing.B) {
	ts := newTestServer(b)
	albums := make([]indexAlbum, benchAlbums)
	for i := range albums {
		artist := person(i%benchArtists+1, fmt.Sprintf("Artist %d", i%benchArtists))
		count := benchTracks / benchAlbums
		if i < benchTracks%benchAlbums {
			count++
		}
		albums[i] = indexAlbum{n: i + 1, title: fmt.Sprintf("Album %d", i), artist: artist.Name, year: 1960 + i%60}
		for t := range count {
			albums[i].tracks = append(albums[i].tracks, indexTrack{
				title: fmt.Sprintf("Track %d", t), trackNo: t + 1, artists: []store.ArtistWrite{artist},
			})
		}
	}
	ts.seedIndex(b, albums...)
	token := ts.tokenFor(b, store.RoleUser)
	var size int64
	for b.Loop() {
		req, err := http.NewRequestWithContext(b.Context(), http.MethodGet, ts.URL+indexPath, http.NoBody)
		if err != nil {
			b.Fatal(err)
		}
		req.Header.Set(authorizationHeader, "Bearer "+token)
		resp, err := ts.Server.Client().Do(req)
		if err != nil {
			b.Fatal(err)
		}
		size, err = io.Copy(io.Discard, resp.Body)
		if closeErr := resp.Body.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(size), "bytes/response")
}

func TestToAPILibraryIndexKeepsEmptyListsNonNull(t *testing.T) {
	t.Parallel()
	out := toAPILibraryIndex(&store.LibraryIndex{Artists: []store.IndexArtist{{
		Name: annName, Albums: []store.IndexAlbum{{Tracks: []store.IndexTrack{{}}}},
	}}})
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"artists":[]`; !strings.Contains(string(body), want) {
		t.Errorf("body = %s, want %s", body, want)
	}
}
