package server

import (
	"bufio"
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

const eventsPath = "/events"

// eventSchemas names the contract's schema for each kind of event's data.
var eventSchemas = map[string]string{ //nolint:gochecknoglobals // A read-only table of the contract.
	string(events.Library):      "EventLibrary",
	string(events.Scan):         "ScanStatus",
	string(events.Fingerprints): "FingerprintStatus",
	string(events.Marks):        "EventMarks",
	string(events.Prefs):        "EventPrefs",
	string(events.ShuffleRules): "EventShuffleRules",
	string(events.Shuffle):      "EventShuffle",
	string(events.Listening):    "EventListening",
	string(events.Lastfm):       "EventLastfm",
	string(events.Settings):     "EventSettings",
	string(events.Ready):        "EventStream",
	string(events.Resync):       "EventStream",
}

// frame is one server-sent event.
type frame struct{ id, event, data string }

// eventStream is an open GET /events, read on its own goroutine.
type eventStream struct {
	t      *testing.T
	frames chan frame
	header http.Header
}

// openStream opens GET /events as the holder of token, resuming after lastID
// when it isn't empty. It closes when the test ends.
func (ts *testServer) openStream(t *testing.T, token, lastID string) *eventStream {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+eventsPath, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header = bearer(token)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	resp, err := contract.Client(t).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /events = %d, want 200", resp.StatusCode)
	}
	s := &eventStream{t: t, frames: make(chan frame, 256), header: resp.Header}
	var reading sync.WaitGroup
	reading.Go(func() { readFrames(resp.Body, s.frames) })
	t.Cleanup(func() {
		cancel()
		reading.Wait()
		_ = resp.Body.Close() // Validates what was read, once nothing reads it.
	})
	return s
}

// readFrames sends each event r holds to out, skipping comments, and closes
// out when r ends.
func readFrames(r io.Reader, out chan<- frame) {
	defer close(out)
	var f frame
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		field, value, _ := strings.Cut(line, ": ")
		switch {
		case line == "" && f.event != "":
			out <- f
			f = frame{}
		case field == "id":
			f.id = value
		case field == "event":
			f.event = value
		case field == "data":
			f.data = value
		}
	}
}

// next returns the next event, checking its data against the contract.
func (s *eventStream) next() frame {
	s.t.Helper()
	select {
	case f, ok := <-s.frames:
		if !ok {
			s.t.Fatal("the stream ended")
		}
		schema, known := eventSchemas[f.event]
		if !known {
			s.t.Fatalf("event %q is in no schema", f.event)
		}
		contract.CheckSchema(s.t, schema, []byte(f.data))
		return f
	case <-time.After(5 * time.Second):
		s.t.Fatal("no event within 5 s")
	}
	return frame{}
}

// want returns the next event, which must be of kind.
func (s *eventStream) want(kind events.Kind) frame {
	s.t.Helper()
	f := s.next()
	if f.event != string(kind) {
		s.t.Fatalf("event = %s %s, want %s", f.event, f.data, kind)
	}
	return f
}

// until returns the first event of kind, skipping the others.
func (s *eventStream) until(kind events.Kind) frame {
	s.t.Helper()
	for {
		if f := s.next(); f.event == string(kind) {
			return f
		}
	}
}

// decodeEvent decodes f's data into v.
func decodeEvent(t *testing.T, f frame, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(f.data), v); err != nil {
		t.Fatalf("decode %s %s: %v", f.event, f.data, err)
	}
}

// ping publishes a library event everyone hears, so a stream's next event
// shows that nothing came before it.
func (ts *testServer) ping() {
	ts.Events.Publish(events.Library, events.Everyone(), []byte(`{"albums":[],"all":false}`), nil)
}

func TestEventsOpenWithReady(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	s := ts.openStream(t, ts.tokenFor(t, store.RoleUser), "")
	if got := s.header.Get("Content-Type"); got != api.EventStreamMediaType {
		t.Errorf("Content-Type = %q", got)
	}
	if s.header.Get("Cache-Control") != noStore || s.header.Get("X-Accel-Buffering") != "no" {
		t.Errorf("headers = %v, want no-store and no buffering", s.header)
	}
	if f := s.want(events.Ready); f.id == "" {
		t.Error("ready has no id")
	}
}

func TestEventsNeedCredentials(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.send(t, call{method: http.MethodGet, path: eventsPath, invalid: true}).problem(t, http.StatusUnauthorized)
}

func TestEventsResumeOrResync(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.tokenFor(t, store.RoleUser)
	first := ts.openStream(t, token, "")
	ready := first.want(events.Ready)
	ts.ping()
	ts.ping()
	missed := first.want(events.Library)
	first.want(events.Library)

	again := ts.openStream(t, token, missed.id)
	again.want(events.Ready)
	if f := again.want(events.Library); f.id == missed.id || f.id == ready.id {
		t.Errorf("resumed at %s, want the event after %s", f.id, missed.id)
	}
	ts.send(t, call{method: http.MethodGet, path: healthPath}) // Nothing else waits.
	lost := ts.openStream(t, token, "another-run-7")
	lost.want(events.Resync)
	ts.ping()
	lost.want(events.Library) // Live after the resync.
}

// wantMark checks that s hears a marks event as want.
func (s *eventStream) wantMark(want *api.EventMarks) {
	s.t.Helper()
	var got api.EventMarks
	decodeEvent(s.t, s.want(events.Marks), &got)
	if !reflect.DeepEqual(&got, want) {
		s.t.Errorf("marks event = %+v, want %+v", got, *want)
	}
}

func TestEventsOfHeartsGoToTheirPerson(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	mine := ts.openStream(t, sess.token, "")
	other := ts.openStream(t, ts.tokenFor(t, store.RoleUser), "")
	mine.want(events.Ready)
	other.want(events.Ready)
	for _, k := range everyKind() {
		ts.send(t, heartCall(http.MethodPut, sess.token, k.kind, k.id))
		mine.wantMark(&api.EventMarks{Kind: k.kind, ID: k.id, Hearted: new(true)})
	}
	ts.send(t, heartCall(http.MethodDelete, sess.token, kindTracks, firstTrack))
	mine.wantMark(&api.EventMarks{Kind: kindTracks, ID: firstTrack, Hearted: new(false)})
	ts.send(t, heartCall(http.MethodPut, sess.token, kindTracks, missingID)) // A 404 says nothing.
	ts.ping()
	mine.want(events.Library)
	other.want(events.Library) // The other person heard none of it.
}

func TestEventsOfRatings(t *testing.T) {
	t.Parallel()
	ts, sess := markFixture(t)
	s := ts.openStream(t, sess.token, "")
	s.want(events.Ready)
	ts.send(t, rateCall(sess.token, firstTrack, stars(4)))
	s.wantMark(&api.EventMarks{Kind: kindTracks, ID: firstTrack, Stars: new(4)})
	ts.send(t, call{method: http.MethodDelete, path: ratingsPath + firstTrack, header: bearer(sess.token)})
	s.wantMark(&api.EventMarks{Kind: kindTracks, ID: firstTrack, Stars: new(0)})
}

func TestEventsOfPrefsListeningAndLastfm(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	s := ts.openStream(t, sess.token, "")
	s.want(events.Ready)
	ts.send(t, putPrefCall(sess.token, themePath, `"light"`))
	var pref api.EventPrefs
	decodeEvent(t, s.want(events.Prefs), &pref)
	if pref.Key != "ui.theme" {
		t.Errorf("prefs event = %+v, want ui.theme", pref)
	}
	h := jsonHeader(sess.token)
	ts.send(t, call{method: http.MethodPost, path: nowPlayingPath, body: `{"trackId":"t1","clientId":"web-1"}`, header: h})
	var playing api.EventListening
	decodeEvent(t, s.want(events.Listening), &playing)
	if playing != (api.EventListening{TrackID: "t1", ClientID: "web-1"}) {
		t.Errorf("listening event = %+v", playing)
	}
	long := `{"trackId":"t1","clientId":"` + strings.Repeat("x", maxPlayField+1) + `"}`
	ts.send(t, call{method: http.MethodPost, path: nowPlayingPath, body: long, header: h, invalid: true}).problem(t, http.StatusBadRequest)
	ts.send(t, call{method: http.MethodDelete, path: "/me/lastfm", header: bearer(sess.token)})
	s.want(events.Lastfm)
}

func TestEventsOfSettingsGoToAdmins(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	admin := ts.openStream(t, sess.token, "")
	user := ts.openStream(t, ts.tokenFor(t, store.RoleUser), "")
	admin.want(events.Ready)
	user.want(events.Ready)
	ts.patchSettings(t, sess.token, `{"log.level":"debug","lastfm.shared_secret":"`+secretValue+`"}`, false)
	var got api.EventSettings
	f := admin.want(events.Settings)
	decodeEvent(t, f, &got)
	if !slices.Equal(got.Keys, []string{"lastfm.shared_secret", "log.level"}) || strings.Contains(f.data, secretValue) {
		t.Errorf("settings event = %s, want the keys alone", f.data)
	}
	ts.ping()
	user.want(events.Library)
}

// wantScan checks that s hears a scan start, the album change and the end.
func (s *eventStream) wantScan(album string) {
	s.t.Helper()
	var running api.ScanStatus
	decodeEvent(s.t, s.want(events.Scan), &running)
	if running.State != api.ScanRunning {
		s.t.Errorf("first scan event = %+v, want running", running)
	}
	var change api.EventLibrary
	decodeEvent(s.t, s.until(events.Library), &change)
	if change.All || !slices.Contains(change.Albums, album) {
		s.t.Errorf("library event = %+v, want album %s", change, album)
	}
	var ended api.ScanStatus
	decodeEvent(s.t, s.until(events.Scan), &ended)
	if ended.State != api.ScanIdle || ended.Last == nil {
		s.t.Errorf("last scan event = %+v, want idle with a result", ended)
	}
}

// addSample copies a testdata sample to rel under the fixture's root.
func (fx *editFixture) addSample(t *testing.T, rel, sample string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "library", "testdata", sample))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(fx.root, rel), b, 0o600); err != nil { //nolint:gosec // G703: under the test's temp root.
		t.Fatal(err)
	}
}

func TestEventsOfAScan(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{}, map[string]string{"Album/one.flac": sampleFLAC})
	admin := fx.openStream(t, fx.token, "")
	user := fx.openStream(t, fx.tokenFor(t, store.RoleUser), "")
	admin.want(events.Ready)
	user.want(events.Ready)
	fx.addSample(t, "Album/two.mp3", sampleMP3)
	if _, err := fx.Scanner.Scan(t.Context(), library.Quick); err != nil {
		t.Fatal(err)
	}
	admin.wantScan(fx.album.ID)
	user.wantScan(fx.album.ID)
	if _, err := fx.Scanner.Fingerprint(t.Context()); err != nil {
		t.Fatal(err)
	}
	var pass api.FingerprintStatus
	decodeEvent(t, admin.until(events.Fingerprints), &pass)
	fx.ping()
	user.want(events.Library) // Only administrators hear the pass.
}

// drainTap returns every event sub holds now, as "kind data".
func drainTap(sub *events.Subscription) []string {
	var heard []string
	for {
		select {
		case ev := <-sub.C():
			heard = append(heard, string(ev.Kind)+" "+string(ev.Data))
		default:
			return heard
		}
	}
}

// TestNoEventHoldsASecret runs everything that publishes, with a password, a
// token, a Last.fm secret and a library root in play, and reads every event an
// administrator hears.
func TestNoEventHoldsASecret(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{TagWrites: true}, twoFLACs())
	tap := fx.Events.Subscribe(events.Viewer{UserID: 1, Admin: true}, "")
	defer fx.Events.Unsubscribe(tap)
	token := fx.token
	fx.patchSettings(t, token, `{"lastfm.shared_secret":"`+secretValue+`","library.roots":["`+fx.root+`"]}`, false)
	fx.send(t, putPrefCall(token, themePath, `"dark"`))
	fx.send(t, heartCall(http.MethodPut, token, kindAlbums, fx.album.ID))
	fx.send(t, rateCall(token, fx.album.Tracks[0].ID, stars(3)))
	fx.send(t, call{method: http.MethodPost, path: nowPlayingPath, body: `{"trackId":"` + fx.album.Tracks[0].ID + `"}`, header: jsonHeader(token)})
	fx.send(t, fx.postCall(fx.editsPath(), fx.album.Etag, `{"all":{"set":{"title":["Renamed"]}}}`))
	fx.startShuffle(t, token, &api.ShuffleStart{Scope: api.ShuffleScope{Kind: scopeLibrary}, Mode: modeEndless})
	if _, err := fx.Scanner.Scan(t.Context(), library.Full); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(fx.root); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.Scanner.Scan(t.Context(), library.Quick); err != nil {
		t.Fatal(err)
	}
	fx.send(t, changeCall(t, token, api.PasswordChange{Current: "a password", New: newPassword}))
	fx.requestReset(t, "user-admin")
	code := fx.lastCode(t)
	fx.send(t, jsonCall(t, confirmPath, "", api.ResetConfirm{Name: "user-admin", Code: code, New: testPassword}))
	heard := drainTap(tap)
	if len(heard) < 8 {
		t.Fatalf("heard %d events, want every kind: %v", len(heard), heard)
	}
	for _, ev := range heard {
		for _, secret := range []string{secretValue, token, testPassword, newPassword, code, fx.root} {
			if strings.Contains(ev, secret) {
				t.Errorf("event %s holds %q", ev, secret)
			}
		}
	}
}

func TestEventsOfShuffleRulesGoToTheirPerson(t *testing.T) {
	t.Parallel()
	ts, sess := shuffleFixture(t)
	mine := ts.openStream(t, sess.token, "")
	other := ts.openStream(t, ts.tokenFor(t, store.RoleUser), "")
	mine.want(events.Ready)
	other.want(events.Ready)
	var run api.ShuffleRun
	ts.send(t, call{method: http.MethodPut, path: runsPath, body: tracksBody(firstTrack, secondTrack), header: jsonHeader(sess.token)}).decode(t, &run)
	mine.want(events.ShuffleRules)
	ts.send(t, call{method: http.MethodDelete, path: runsPath + "/" + run.ID, header: bearer(sess.token)})
	mine.want(events.ShuffleRules)
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		ts.send(t, call{method: method, path: excludedPath, body: tracksBody(firstTrack), header: jsonHeader(sess.token)})
		mine.want(events.ShuffleRules)
	}
	ts.send(t, call{method: http.MethodDelete, path: runsPath + "/" + run.ID, header: bearer(sess.token)}) // A 404 says nothing.
	ts.ping()
	mine.want(events.Library)
	other.want(events.Library) // The other person heard none of it.
}
