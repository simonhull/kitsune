package server

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// fieldsTrack is the ULID of fieldsLibrary's first track.
	fieldsTrack = "01J00000000000000000001000"
	mp3Name     = "MP3"
)

// fieldsLibrary seeds an album whose tracks have everything a song column shows:
// two FLACs with tags, guests and an album artist, and an MP3 with none of it.
func (ts *testServer) fieldsLibrary(t *testing.T) {
	t.Helper()
	ann, bob := person(1, annName), person(2, "Bob")
	album := store.AlbumWrite{GroupKey: "g1", ULID: albumULID(1), Name: glasshouse, SortName: glasshouse, Artist: annName, Year: 2021}
	write := func(i int, title string) store.TrackWrite {
		return store.TrackWrite{
			ULID: fmt.Sprintf("01J%020d%03d", 1, i), Root: musicBase, RelPath: fmt.Sprintf("a1/%d.flac", i),
			Format: streamFormat, Codec: flacFormat, Duration: time.Minute, SampleRate: 44100, BitDepth: 16,
			Bitrate: 912_400, Lossless: true, Title: title, TrackNo: i + 1, Year: 2021, Album: album,
			Artists: []store.ArtistWrite{ann}, AlbumArtists: []store.ArtistWrite{ann},
		}
	}
	first := write(0, "Breakwater")
	first.Artists = []store.ArtistWrite{ann, bob}
	first.Tags = []store.Tag{
		{Key: library.GenreKeys()[0], Value: "Dream pop, Shoegaze"},
		{Key: library.GenreKeys()[0], Value: "dream pop"},
		{Key: library.ComposerKeys()[0], Value: "I. Marrow"},
		{Key: library.ComposerKeys()[0], Value: " M. Vey "},
		{Key: "COMMENT", Value: "not asked for"},
	}
	second := write(1, "Salt in the Lungs")
	third := write(2, "Kelp Line")
	third.Format, third.Codec, third.SampleRate, third.BitDepth, third.Bitrate = mp3Name, "mp3", 44100, 0, 320_000
	third.Lossless, third.Year, third.AlbumArtists = false, 0, nil
	if err := ts.Store.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{first, second, third}}); err != nil {
		t.Fatal(err)
	}
}

// fieldsOf returns what fields added to a track, which is nothing when absent.
func fieldsOf(t *api.LibraryTrack) api.TrackFields {
	if t.TrackFields == nil {
		return api.TrackFields{}
	}
	return *t.TrackFields
}

// indexWith reads the index's raw body with a query, as the holder of token.
func (ts *testServer) indexWith(t *testing.T, token, query string) response {
	t.Helper()
	return ts.send(t, call{method: http.MethodGet, path: indexPath + query, header: bearer(token)})
}

// indexBody is the index of fieldsLibrary without fields, as it was before
// fields existed: byte for byte, so a client that never asks sees no change.
const indexBody = `{"artists":[{"id":"04J00000000000000000000001","name":"Ann","sortName":"Ann","hearted":false,` +
	`"albums":[{"id":"02J00000000000000000000001","title":"Glasshouse","artist":"Ann","compilation":false,"hearted":false,"year":2021,` +
	`"tracks":[{"id":"01J00000000000000000001000","title":"Breakwater","durationMs":60000,"artists":["Ann","Bob"],"trackNo":1,"hearted":true,"stars":4},` +
	`{"id":"01J00000000000000000001001","title":"Salt in the Lungs","durationMs":60000,"artists":["Ann"],"trackNo":2,"hearted":false},` +
	`{"id":"01J00000000000000000001002","title":"Kelp Line","durationMs":60000,"artists":["Ann"],"trackNo":3,"hearted":false}]}]}]}`

func TestLibraryIndexWithoutFieldsIsUnchanged(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.fieldsLibrary(t)
	sess := ts.sessionFor(t)
	wantNoContent(t, ts.send(t, heartCall(http.MethodPut, sess.token, kindTracks, fieldsTrack)))
	wantNoContent(t, ts.send(t, rateCall(sess.token, fieldsTrack, stars(4))))
	if err := ts.Store.RecordPlay(t.Context(), &store.NewPlay{
		UserID: sess.user.ID, TrackID: fieldsTrack, PlayedAt: ts.Clock.Now(), Source: "test", ClientID: "p1",
	}); err != nil {
		t.Fatal(err)
	}
	if got := ts.indexWith(t, sess.token, ""); got.Status != http.StatusOK || string(got.Body) != indexBody {
		t.Errorf("GET %s = %d\n%s\nwant\n%s", indexPath, got.Status, got.Body, indexBody)
	}
}

func TestLibraryIndexSendsTheFieldsAskedFor(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.fieldsLibrary(t)
	sess := ts.sessionFor(t)
	other := ts.tokenFor(t, store.RoleUser)
	for i, at := range []time.Duration{-3 * time.Hour, -time.Hour} {
		if err := ts.Store.RecordPlay(t.Context(), &store.NewPlay{
			UserID: sess.user.ID, TrackID: fieldsTrack, PlayedAt: ts.Clock.Now().Add(at), Source: "test", ClientID: strconv.Itoa(i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	var index api.LibraryIndex
	ts.getJSON(t, sess.token, indexPath+"?fields=albumArtist,genre,year,composer,format,added,plays,lastPlayed", &index)
	tracks := index.Artists[0].Albums[0].Tracks
	added := ts.Clock.Now().UTC().Truncate(time.Millisecond)
	want := []api.TrackFields{
		{
			AlbumArtists: []string{annName}, DisplayGenres: []string{"Dream pop", "Shoegaze"},
			Composers: []string{"I. Marrow", "M. Vey"}, Year: 2021, Format: streamFormat, SampleRate: 44100,
			BitDepth: 16, Bitrate: 912, AddedAt: added, Plays: 2, LastPlayedAt: added.Add(-time.Hour),
		},
		{AlbumArtists: []string{annName}, Year: 2021, Format: streamFormat, SampleRate: 44100, BitDepth: 16, Bitrate: 912, AddedAt: added},
		// An MP3 with nothing to say but its format.
		{Format: mp3Name, SampleRate: 44100, Bitrate: 320, AddedAt: added},
	}
	for i := range want {
		if got := fieldsOf(&tracks[i]); !reflect.DeepEqual(got, want[i]) {
			t.Errorf("track %d = %+v, want %+v", i, got, want[i])
		}
	}

	// Plays are the caller's own.
	var theirs api.LibraryIndex
	ts.getJSON(t, other, indexPath+"?fields=plays&fields=lastPlayed", &theirs)
	if got := fieldsOf(&theirs.Artists[0].Albums[0].Tracks[0]); got.Plays != 0 || !got.LastPlayedAt.IsZero() {
		t.Errorf("another user sees plays %d, last %v", got.Plays, got.LastPlayedAt)
	}
}

func TestLibraryIndexSendsOnlyTheFieldsAskedFor(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.fieldsLibrary(t)
	token := ts.tokenFor(t, store.RoleUser)
	var index api.LibraryIndex
	ts.getJSON(t, token, indexPath+"?fields=bitrate", &index)
	got := fieldsOf(&index.Artists[0].Albums[0].Tracks[0])
	if got.Bitrate != 912 || got.Format != "" || got.SampleRate != 0 || got.Year != 0 || got.DisplayGenres != nil || !got.AddedAt.IsZero() {
		t.Errorf("track = %+v, want only the bitrate", got)
	}
}

func TestLibraryIndexRefusesUnknownFields(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.tokenFor(t, store.RoleUser)
	for _, query := range []string{"?fields=artist", "?fields=genre,skips", "?fields=Genre"} {
		c := call{method: http.MethodGet, path: indexPath + query, header: bearer(token), invalid: true}
		ts.send(t, c).problem(t, http.StatusBadRequest)
	}
}

func TestParseIndexFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   []string
		want indexFields
		ok   bool
	}{
		{nil, 0, true},
		{[]string{""}, 0, true},
		{[]string{"plays,genre"}, fieldPlays | fieldGenre, true},
		{[]string{"plays", "plays,,year"}, fieldPlays | fieldYear, true},
		{[]string{"plays, genre"}, 0, false},
		{[]string{"title"}, 0, false},
	}
	for _, tt := range tests {
		got, err := parseIndexFields(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("parseIndexFields(%q) = %b, %v", tt.in, got, err)
		}
	}
}
