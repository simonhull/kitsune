package server

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	shufflesPath = "/me/shuffles"
	scopeLibrary = "library"
	scopeAlbum   = "album"
	modeFinite   = "finite"
	modeEndless  = "endless"
	endSkipped   = "skipped"
)

// songAt is the ULID seedAlbums gives track t of album n.
func songAt(n, t int) string { return fmt.Sprintf("01J%020d%03d", n, t) }

// shufflesFixture is a server with three albums of 20 songs and a signed-in admin.
func shufflesFixture(t *testing.T) (*testServer, session) {
	t.Helper()
	ts := newTestServer(t)
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 20}, browseAlbum{n: 2, sortName: "b", tracks: 20},
		browseAlbum{n: 3, sortName: "c", tracks: 20})
	sess := ts.sessionFor(t)
	return ts, sess
}

// startShuffle sends POST /me/shuffles and decodes the 201.
func (ts *testServer) startShuffle(t *testing.T, token string, start *api.ShuffleStart) api.ShufflePage {
	t.Helper()
	body, err := json.Marshal(start)
	if err != nil {
		t.Fatal(err)
	}
	resp := ts.send(t, call{method: http.MethodPost, path: shufflesPath, body: string(body), header: jsonHeader(token)})
	if resp.Status != http.StatusCreated {
		t.Fatalf("POST %s = %d, want 201: %s", shufflesPath, resp.Status, resp.Body)
	}
	var page api.ShufflePage
	if err = json.Unmarshal(resp.Body, &page); err != nil {
		t.Fatal(err)
	}
	return page
}

// nextPath is the path of a session's page after a position.
func nextPath(id string, after int64) string {
	return fmt.Sprintf("%s/%s/next?after=%d", shufflesPath, id, after)
}

// nextShuffle sends GET /me/shuffles/{id}/next and decodes the 200.
func (ts *testServer) nextShuffle(t *testing.T, token, id string, after int64) api.ShufflePage {
	t.Helper()
	var page api.ShufflePage
	ts.getJSON(t, token, nextPath(id, after), &page)
	return page
}

// songsOf lists a page's songs, in play order.
func songsOf(p *api.ShufflePage) []string {
	var out []string
	for i := range p.Picks {
		for _, song := range p.Picks[i].Songs {
			out = append(out, song.ID)
		}
	}
	return out
}

// wantEachOnceFrom fails unless songs are album n's, each once.
func wantEachOnceFrom(t *testing.T, songs []string, n int) {
	t.Helper()
	sorted := slices.Sorted(slices.Values(songs))
	if len(slices.Compact(sorted)) != len(songs) {
		t.Errorf("songs = %v, want each once", songs)
	}
	for _, id := range songs {
		if id[3:23] != fmt.Sprintf("%020d", n) {
			t.Errorf("song %s is not on album %d", id, n)
		}
	}
}

func TestShuffleAnAlbumPlaysEverySongOnce(t *testing.T) {
	t.Parallel()
	ts, sess := shufflesFixture(t)
	first := songAt(2, 7)
	page := ts.startShuffle(t, sess.token, &api.ShuffleStart{
		Scope: api.ShuffleScope{Kind: scopeAlbum, ID: albumULID(2)}, Mode: modeFinite, First: first,
	})
	got := songsOf(&page)
	if whole := page.Done && page.Songs == 20 && page.From == 0 && page.To == 20; !whole || len(got) != 20 || got[0] != first {
		t.Fatalf("page = %d songs from %s, done %v, songs %d, %d to %d", len(got), got[0], page.Done, page.Songs, page.From, page.To)
	}
	wantEachOnceFrom(t, got, 2)
	if why := page.Picks[1].Why; why.Rating == 0 || why.Recency == 0 || why.Played {
		t.Errorf("why = %+v, want factors, and never played", why)
	}
}

func TestShuffleSaysAFiniteOneIsDone(t *testing.T) {
	t.Parallel()
	ts, sess := shufflesFixture(t)
	page := ts.startShuffle(t, sess.token, &api.ShuffleStart{Scope: api.ShuffleScope{Kind: scopeAlbum, ID: albumULID(1)}, Mode: modeFinite})
	more := ts.nextShuffle(t, sess.token, page.ID, page.To)
	if len(more.Picks) != 0 || !more.Done {
		t.Errorf("after the end: %+v, want nothing and done", more)
	}
}

func TestShuffleRespectsCombineAndExclude(t *testing.T) {
	t.Parallel()
	ts, sess := shufflesFixture(t)
	intro, song := songAt(1, 3), songAt(3, 9) // a run across albums
	excluded := songAt(1, 5)
	ts.send(t, call{method: http.MethodPut, path: runsPath, body: tracksBody(intro, song), header: jsonHeader(sess.token)})
	wantNoContent(t, ts.send(t, call{method: http.MethodPut, path: excludedPath, body: tracksBody(excluded), header: jsonHeader(sess.token)}))

	for _, mode := range []string{modeFinite, modeEndless} {
		page := ts.startShuffle(t, sess.token, &api.ShuffleStart{Scope: api.ShuffleScope{Kind: scopeAlbum, ID: albumULID(1)}, Mode: mode})
		got := songsOf(&page)
		if slices.Contains(got, excluded) {
			t.Errorf("%s: the excluded song came up", mode)
		}
		at := slices.Index(got, intro)
		if at < 0 || at+1 >= len(got) || got[at+1] != song {
			t.Errorf("%s: songs = %v, want %s then %s, the run whole, even off the album", mode, got, intro, song)
		}
		if mode == modeFinite && len(got) != 20 { // 20, less the one excluded, and the run's second song
			t.Errorf("finite: %d songs, want 20", len(got))
		}
	}
	// The song chosen leads, even one excluded.
	page := ts.startShuffle(t, sess.token, &api.ShuffleStart{Scope: api.ShuffleScope{Kind: scopeLibrary}, Mode: modeEndless, First: excluded})
	if got := songsOf(&page); got[0] != excluded || slices.Contains(got[1:], excluded) {
		t.Errorf("songs = %v, want %s first, once", got[:3], excluded)
	}
}

func TestShuffleGoesOnFromAnotherDevice(t *testing.T) {
	t.Parallel()
	ts, sess := shufflesFixture(t)
	res, err := ts.Auth.Login(t.Context(), simonName, testPassword, "phone/1")
	if err != nil {
		t.Fatal(err)
	}
	phone := res.Token
	page := ts.startShuffle(t, sess.token, &api.ShuffleStart{Scope: api.ShuffleScope{Kind: scopeLibrary}, Mode: modeEndless})
	if len(page.Picks) != 50 || page.Songs != 60 {
		t.Fatalf("first page: %d picks of %d songs", len(page.Picks), page.Songs)
	}
	second := ts.nextShuffle(t, phone, page.ID, page.To)
	if second.From != 50 || second.To != 100 || len(second.Picks) != 50 {
		t.Fatalf("second page: %d to %d, %d picks", second.From, second.To, len(second.Picks))
	}
	again := ts.nextShuffle(t, sess.token, page.ID, page.To) // the first device catches up
	if !slices.Equal(songsOf(&again), songsOf(&second)) {
		t.Errorf("the same page twice differs:\n%v\n%v", songsOf(&again), songsOf(&second))
	}
	mid := ts.nextShuffle(t, sess.token, page.ID, 60)
	if mid.From != 60 || !slices.Equal(songsOf(&mid), songsOf(&second)[10:]) {
		t.Errorf("from 60: %d, %v", mid.From, songsOf(&mid))
	}
}

func TestShuffleEndlessRepeatsNothingEarly(t *testing.T) {
	t.Parallel()
	ts, sess := shufflesFixture(t)
	page := ts.startShuffle(t, sess.token, &api.ShuffleStart{Scope: api.ShuffleScope{Kind: scopeLibrary}, Mode: modeEndless})
	// Nothing comes back before 70% of the weight has played: 42 of 60 equal songs.
	if seen := slices.Sorted(slices.Values(songsOf(&page)[:42])); len(slices.Compact(seen)) != 42 {
		t.Errorf("the first 42 picks hold %d songs, want 42", len(slices.Compact(seen)))
	}
}

func TestShuffleIsRepeatableBySeed(t *testing.T) {
	t.Parallel()
	ts, sess := shufflesFixture(t)
	seed := uint64(42)
	start := &api.ShuffleStart{Scope: api.ShuffleScope{Kind: scopeLibrary}, Mode: modeFinite, Seed: &seed}
	a, b := ts.startShuffle(t, sess.token, start), ts.startShuffle(t, sess.token, start)
	if !slices.Equal(songsOf(&a), songsOf(&b)) || a.ID == b.ID {
		t.Errorf("one seed gave two orders, or one session")
	}
}

func TestShuffleScopes(t *testing.T) {
	t.Parallel()
	ts, sess := shufflesFixture(t)
	ts.send(t, rateCall(sess.token, songAt(1, 1), stars(5)))
	ts.send(t, rateCall(sess.token, songAt(2, 2), stars(3)))
	ts.send(t, heartCall(http.MethodPut, sess.token, kindTracks, songAt(3, 3)))
	ts.send(t, heartCall(http.MethodPut, sess.token, kindAlbums, albumULID(2)))
	for name, tt := range map[string]struct {
		scope api.ShuffleScope
		songs int
	}{
		"everything":   {api.ShuffleScope{Kind: scopeLibrary}, 60},
		"4 and up":     {api.ShuffleScope{Kind: "rated"}, 1},
		"a heart":      {api.ShuffleScope{Kind: "hearted"}, 21},
		"by an artist": {api.ShuffleScope{Kind: "artist", ID: annID}, 60},
		"selection":    {api.ShuffleScope{Kind: kindTracks, Tracks: []string{songAt(1, 1), songAt(3, 4), missingID}}, 2},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			page := ts.startShuffle(t, sess.token, &api.ShuffleStart{Scope: tt.scope, Mode: modeFinite})
			if page.Songs != tt.songs || len(songsOf(&page)) != min(tt.songs, 50) {
				t.Errorf("songs = %d, %d on the page, want %d", page.Songs, len(songsOf(&page)), tt.songs)
			}
		})
	}
	page := ts.startShuffle(t, ts.tokenFor(t, store.RoleUser), &api.ShuffleStart{Scope: api.ShuffleScope{Kind: "hearted"}, Mode: modeEndless})
	if len(page.Picks) != 0 || page.Songs != 0 {
		t.Errorf("someone with no hearts got %d picks", len(page.Picks))
	}
}

// listenedTo reports five listens to song two days ago, in a shuffle: skipped
// twice early, played once, then stopped and interrupted.
func (ts *testServer) listenedTo(t *testing.T, token, song string) {
	t.Helper()
	at := ts.Clock.Now().Add(-48 * time.Hour)
	for i, ended := range []string{endSkipped, endSkipped, "completed", "stopped", "interrupted"} {
		body := listenBody(t, &api.ListenReport{TrackID: song, ClientID: fmt.Sprint("l", i), StartedAt: at, Shuffle: "s", Ended: ended, ListenedMs: 10_000})
		wantNoContent(t, ts.postListen(t, token, body))
	}
}

// whyOf starts a shuffle of song alone and returns why it came up.
func (ts *testServer) whyOf(t *testing.T, token, song string) api.ShuffleWhy {
	t.Helper()
	page := ts.startShuffle(t, token, &api.ShuffleStart{Scope: api.ShuffleScope{Kind: kindTracks, Tracks: []string{song}}, Mode: modeFinite})
	return page.Picks[0].Why
}

func TestShuffleLearnsFromListens(t *testing.T) {
	t.Parallel()
	ts, sess := shufflesFixture(t)
	song := songAt(1, 1)
	ts.listenedTo(t, sess.token, song)
	why := ts.whyOf(t, sess.token, song)
	if !why.Played || why.SinceDays != 2 || why.Starts != 3 || why.Skips != 2 || why.EarlySkips != 2 || why.Listens != 3 {
		t.Errorf("why = %+v: want 3 starts, 2 skips, both early, 3 listens: stopped and interrupted say nothing", why)
	}
	if why.SkipRate >= 1 || why.SkipMemory != 0.25 || !why.Memory {
		t.Errorf("why = %+v, want skips to count", why)
	}
}

func TestShuffleForgetsSkipsWithMemoryOff(t *testing.T) {
	t.Parallel()
	ts, sess := shufflesFixture(t)
	song := songAt(1, 1)
	ts.listenedTo(t, sess.token, song)
	wantNoContent(t, ts.send(t, putPrefCall(sess.token, "/me/prefs/shuffle.memory", `false`)))
	if why := ts.whyOf(t, sess.token, song); why.Memory || why.SkipRate != 1 || why.SkipMemory != 1 || why.Listen != 1 || !why.Played {
		t.Errorf("with memory off, why = %+v", why)
	}
}

func TestShuffleTellsTheirDevices(t *testing.T) {
	t.Parallel()
	ts, sess := shufflesFixture(t)
	mine := ts.openStream(t, sess.token, "")
	mine.want(events.Ready)
	page := ts.startShuffle(t, sess.token, &api.ShuffleStart{Scope: api.ShuffleScope{Kind: scopeLibrary}, Mode: modeEndless})
	var ev api.EventShuffle
	decodeEvent(t, mine.want(events.Shuffle), &ev)
	if ev.ID != page.ID || ev.To != 50 {
		t.Errorf("event = %+v", ev)
	}
	ts.nextShuffle(t, sess.token, page.ID, page.To)
	decodeEvent(t, mine.want(events.Shuffle), &ev)
	if ev.To != 100 {
		t.Errorf("event = %+v, want to 100", ev)
	}
	ts.nextShuffle(t, sess.token, page.ID, page.To) // a page given before says nothing
	ts.ping()
	mine.want(events.Library)
}

func TestShuffleRefuses(t *testing.T) {
	t.Parallel()
	ts, sess := shufflesFixture(t)
	page := ts.startShuffle(t, sess.token, &api.ShuffleStart{Scope: api.ShuffleScope{Kind: scopeLibrary}, Mode: modeEndless})
	for name, body := range map[string]string{
		"cut short":     "{",
		"no mode":       `{"scope":{"kind":"library"}}`,
		"bad mode":      `{"scope":{"kind":"library"},"mode":"forever"}`,
		"bad kind":      `{"scope":{"kind":"genre"},"mode":"finite"}`,
		"no tracks":     `{"scope":{"kind":"tracks","tracks":[]},"mode":"finite"}`,
		"names a user":  `{"scope":{"kind":"library"},"mode":"finite","userId":2}`,
		"negative seed": `{"scope":{"kind":"library"},"mode":"finite","seed":-1}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ts.send(t, call{method: http.MethodPost, path: shufflesPath, body: body, header: jsonHeader(sess.token), invalid: true}).problem(t, http.StatusBadRequest)
		})
	}
	for name, body := range map[string]string{
		"album, no id": `{"scope":{"kind":"album"},"mode":"finite"}`,
		"library, id":  `{"scope":{"kind":"library","id":"x"},"mode":"finite"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ts.send(t, call{method: http.MethodPost, path: shufflesPath, body: body, header: jsonHeader(sess.token)}).problem(t, http.StatusBadRequest)
		})
	}
	for name, body := range map[string]string{
		"album":  `{"scope":{"kind":"album","id":"` + missingID + `"},"mode":"finite"}`,
		"artist": `{"scope":{"kind":"artist","id":"` + missingID + `"},"mode":"finite"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ts.send(t, call{method: http.MethodPost, path: shufflesPath, body: body, header: jsonHeader(sess.token)}).problem(t, http.StatusNotFound)
		})
	}
	ts.send(t, call{method: http.MethodGet, path: nextPath(page.ID, 51), header: bearer(sess.token)}).problem(t, http.StatusBadRequest)
	ts.send(t, call{method: http.MethodGet, path: shufflesPath + "/" + page.ID + "/next?after=x", header: bearer(sess.token), invalid: true}).problem(t, http.StatusBadRequest)
	ts.send(t, call{method: http.MethodGet, path: nextPath(missingID, 0), header: bearer(sess.token)}).problem(t, http.StatusNotFound)
	other := ts.tokenFor(t, store.RoleUser)
	ts.send(t, call{method: http.MethodGet, path: nextPath(page.ID, 0), header: bearer(other)}).problem(t, http.StatusNotFound)
	ts.send(t, call{method: http.MethodGet, path: nextPath(page.ID, 0), invalid: true}).problem(t, http.StatusUnauthorized)
	h := http.Header{contentType: {api.JSONMediaType}}
	ts.send(t, call{method: http.MethodPost, path: shufflesPath, body: `{"scope":{"kind":"library"},"mode":"finite"}`, header: h, invalid: true}).problem(t, http.StatusUnauthorized)
	h = cookieOnly(sess.token)
	h.Set(contentType, api.JSONMediaType)
	ts.send(t, call{method: http.MethodPost, path: shufflesPath, body: `{"scope":{"kind":"library"},"mode":"finite"}`, header: h}).problem(t, http.StatusForbidden)
}
