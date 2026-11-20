package server

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	gainPath     = "/admin/replaygain"
	gainRunsPath = "/admin/replaygain/runs"
)

// gainFixture is two sines on one album, measured, on a server with tag writes
// as asked.
func gainFixture(t *testing.T, tagWrites bool) *editFixture {
	t.Helper()
	return gainFixtureWith(t, &testServerOptions{TagWrites: tagWrites})
}

// gainFixtureWith is gainFixture's library on a server with opts.
func gainFixtureWith(t *testing.T, opts *testServerOptions) *editFixture {
	t.Helper()
	fx := newEditFixture(t, opts, map[string]string{fileOne: sineFLAC, fileTwo: sineFLAC})
	if _, err := fx.Scanner.MeasureLoudness(t.Context()); err != nil {
		t.Fatal(err)
	}
	fx.reload(t)
	return fx
}

// gainStatus reads GET /admin/replaygain.
func (fx *editFixture) gainStatus(t *testing.T) api.ReplayGainStatus {
	t.Helper()
	var st api.ReplayGainStatus
	fx.getJSON(t, fx.token, gainPath, &st)
	return st
}

// startGain posts body to path, wants a 202, and returns the run.
func (fx *editFixture) startGain(t *testing.T, path, body string) api.GainRun {
	t.Helper()
	var run api.GainRun
	resp := fx.send(t, call{method: http.MethodPost, path: path, body: body, header: jsonHeader(fx.token)})
	if err := json.Unmarshal(resp.Body, &run); err != nil || resp.Status != http.StatusAccepted {
		t.Fatalf("POST %s = %d: %s", path, resp.Status, resp.Body)
	}
	return run
}

// waitGain waits for the run id to end, and returns how it did.
func (fx *editFixture) waitGain(t *testing.T, id string) api.GainRunDetail {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var d api.GainRunDetail
		fx.getJSON(t, fx.token, gainRunsPath+"/"+id, &d)
		if d.Run.State != store.GainRunning {
			return d
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %s still running: %+v", id, d.Run)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A run writes both sines with what Kitsune played, which then plays from the
// tags at the same gain; Undo takes it out, and they play measured again.
func TestReplayGainRunAndUndo(t *testing.T) {
	t.Parallel()
	fx := gainFixture(t, true)
	if st := fx.gainStatus(t); !twoToWrite(&st) || st.Run != nil || len(st.Runs) != 0 {
		t.Fatalf("status = %+v, want two FLACs on one album to write", st)
	}
	before := fx.album.Tracks[0].ReplayGain

	run := fx.startGain(t, gainRunsPath, `{}`)
	written := fx.waitGain(t, run.ID)
	fx.wantGainWritten(t, &written, before)
	if st := fx.gainStatus(t); st.Songs != 0 || len(st.Runs) != 1 {
		t.Errorf("status after = %+v", st)
	}
	fx.send(t, call{method: http.MethodPost, path: gainRunsPath, body: `{"albums":["` + fx.album.ID + `"]}`, header: jsonHeader(fx.token)}).
		problem(t, http.StatusUnprocessableEntity)

	undo := fx.startGain(t, gainRunsPath+"/"+run.ID+"/undo", "")
	undone := fx.waitGain(t, undo.ID)
	fx.wantGainUndone(t, &undone, run.ID)
}

// twoToWrite reports whether st says the two FLACs of one album can be written.
func twoToWrite(st *api.ReplayGainStatus) bool {
	return st.Songs == 2 && st.Albums == 1 && slices.Equal(st.Formats, []string{streamFormat}) &&
		len(st.Biggest) == 1 && st.Biggest[0].Songs == 2 && st.Waiting == 0
}

// wantGainWritten fails unless the run d wrote both sines, which now play from
// their tags at before's gain.
func (fx *editFixture) wantGainWritten(t *testing.T, d *api.GainRunDetail, before *api.ReplayGain) {
	t.Helper()
	if got, want := runCounts(&d.Run), (runCount{state: store.GainDone, written: 2, done: 2, albumsDone: 1, undoable: true}); got != want {
		t.Errorf("run = %+v, want %+v", got, want)
	}
	if len(d.Albums) != 1 || d.Albums[0].State != api.GainAlbumWritten || len(d.Left) != 0 {
		t.Errorf("albums = %+v, left = %+v", d.Albums, d.Left)
	}
	fx.reload(t)
	after := fx.album.Tracks[0].ReplayGain
	if after == nil || after.Source != api.GainFromTags || !nearGain(after, before) {
		t.Errorf("replayGain = %+v after, %+v before: the same gain, from the tags", after, before)
	}
}

// runCount is what a run did, counted.
type runCount struct {
	state, undoOf             string
	written, done, albumsDone int
	undoable                  bool
}

// runCounts counts what r did.
func runCounts(r *api.GainRun) runCount {
	return runCount{state: r.State, undoOf: r.UndoOf, written: r.Written, done: r.Done, albumsDone: r.AlbumsDone, undoable: r.Undoable}
}

// nearGain reports whether a and b are the same gains, to a tag's precision.
func nearGain(a, b *api.ReplayGain) bool {
	return math.Abs(a.TrackGain-b.TrackGain) <= 0.005 && math.Abs(a.AlbumGain-b.AlbumGain) <= 0.005 &&
		math.Abs(a.TrackPeak-b.TrackPeak) <= 1e-6
}

// wantGainUndone fails unless the undo d took out what the run id wrote, the
// sines play measured again, a second undo is a 409, and the run reads undone.
func (fx *editFixture) wantGainUndone(t *testing.T, d *api.GainRunDetail, id string) {
	t.Helper()
	if got, want := runCounts(&d.Run), (runCount{state: store.GainDone, undoOf: id, written: 2, done: 2, albumsDone: 1}); got != want {
		t.Errorf("undo = %+v, want %+v", got, want)
	}
	fx.reload(t)
	if rg := fx.album.Tracks[0].ReplayGain; rg == nil || rg.Source != api.GainMeasured {
		t.Errorf("replayGain after the undo = %+v, want measured again", rg)
	}
	fx.send(t, call{method: http.MethodPost, path: gainRunsPath + "/" + id + "/undo", header: jsonHeader(fx.token)}).
		problem(t, http.StatusConflict)
	if st := fx.gainStatus(t); st.Songs != 2 || len(st.Runs) != 2 || st.Runs[1].State != store.GainUndone || st.Runs[1].UndoneBy != d.Run.ID {
		t.Errorf("status after the undo = %+v", st)
	}
}

// A run of the album named, and Stop: a 202 while it runs, a 409 after. The
// run is held at its first step until Stop has answered, so it is running when
// Stop arrives.
func TestReplayGainRunOfAnAlbumAndStop(t *testing.T) {
	t.Parallel()
	held, stopped := make(chan struct{}), make(chan struct{})
	var once sync.Once
	fx := gainFixtureWith(t, &testServerOptions{TagWrites: true, GainStep: func(string, string) {
		once.Do(func() { close(held); <-stopped })
	}})
	run := fx.startGain(t, gainRunsPath, `{"albums":["`+fx.album.ID+`"]}`)
	if run.Albums != 1 || run.Total != 2 {
		t.Errorf("run = %+v, want the album's two songs", run)
	}
	<-held
	if resp := fx.send(t, call{method: http.MethodPost, path: gainRunsPath + "/" + run.ID + "/stop", header: jsonHeader(fx.token)}); resp.Status != http.StatusAccepted {
		t.Errorf("stop of a running run = %d: %s, want 202", resp.Status, resp.Body)
	}
	close(stopped)
	fx.waitGain(t, run.ID)
	fx.send(t, call{method: http.MethodPost, path: gainRunsPath + "/" + run.ID + "/stop", header: jsonHeader(fx.token)}).
		problem(t, http.StatusConflict)
}

// While a run goes, GET /admin/replaygain names it and its detail lists its
// album in hand as writing.
func TestReplayGainStatusNamesTheRunGoing(t *testing.T) {
	t.Parallel()
	entered, release := make(chan struct{}, 2), make(chan struct{})
	fx := newEditFixture(t, &testServerOptions{TagWrites: true, Write: holdingWrite(entered, release)},
		map[string]string{fileOne: sineFLAC, fileTwo: sineFLAC})
	if _, err := fx.Scanner.MeasureLoudness(t.Context()); err != nil {
		t.Fatal(err)
	}
	run := fx.startGain(t, gainRunsPath, `{}`)
	<-entered
	st := fx.gainStatus(t)
	var d api.GainRunDetail
	fx.getJSON(t, fx.token, gainRunsPath+"/"+run.ID, &d)
	close(release)
	if st.Run == nil || st.Run.ID != run.ID || st.Run.Album != fx.album.ID {
		t.Errorf("status during the run = %+v, want the run, writing the album", st.Run)
	}
	if len(d.Albums) != 1 || d.Albums[0].State != api.GainAlbumWriting || d.Albums[0].Title == "" {
		t.Errorf("albums during the run = %+v, want the album, writing", d.Albums)
	}
	fx.waitGain(t, run.ID)
}

// With tag writes off, the status reads and a run or an undo is a 409.
func TestReplayGainNeedsTagWrites(t *testing.T) {
	t.Parallel()
	fx := gainFixture(t, false)
	if st := fx.gainStatus(t); st.Songs != 2 {
		t.Errorf("status = %+v, want two songs to write", st)
	}
	fx.send(t, call{method: http.MethodPost, path: gainRunsPath, body: `{}`, header: jsonHeader(fx.token)}).problem(t, http.StatusConflict)
	fx.send(t, call{method: http.MethodPost, path: gainRunsPath + "/" + unknownRun + "/undo", header: jsonHeader(fx.token)}).problem(t, http.StatusConflict)
}

// A run while a scan holds the library is a 409 with Retry-After.
func TestReplayGainRunWhileBusy(t *testing.T) {
	t.Parallel()
	fx := gainFixture(t, true)
	release, err := fx.Scanner.Claim(0)
	if err != nil {
		t.Fatal(err)
	}
	resp := fx.send(t, call{method: http.MethodPost, path: gainRunsPath, body: `{}`, header: jsonHeader(fx.token)})
	if resp.Status != http.StatusConflict || resp.Header.Get("Retry-After") == "" {
		t.Errorf("a run during a scan = %d %v, want a 409 with Retry-After", resp.Status, resp.Header)
	}
	if _, err = release(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// Every refusal: bad requests, runs that aren't there, and callers who may not.
func TestReplayGainRefusals(t *testing.T) {
	t.Parallel()
	fx := gainFixture(t, true)
	user := fx.tokenFor(t, store.RoleUser)
	runID := fx.startGain(t, gainRunsPath, `{}`).ID
	fx.waitGain(t, runID)
	post := func(path, body, token string) call {
		return call{method: http.MethodPost, path: path, body: body, header: jsonHeader(token)}
	}
	get := func(path, token string) call {
		c := call{method: http.MethodGet, path: path}
		if token != "" {
			c.header = bearer(token)
		}
		return c
	}
	many := `{"albums":[` + strings.Repeat(`"x",`, mostGainAlbums) + `"x"]}`
	for _, tc := range []struct {
		name   string
		c      call
		status int
	}{
		{"too many albums", invalidCall(post(gainRunsPath, many, fx.token)), http.StatusBadRequest},
		{"a body that isn't JSON", invalidCall(post(gainRunsPath, `[`, fx.token)), http.StatusBadRequest},
		{"an album not in the library", post(gainRunsPath, `{"albums":["`+unknownRun+`"]}`, fx.token), http.StatusUnprocessableEntity},
		{"an unknown run", get(gainRunsPath+"/"+unknownRun, fx.token), http.StatusNotFound},
		{"stopping an unknown run", post(gainRunsPath+"/"+unknownRun+"/stop", "", fx.token), http.StatusNotFound},
		{"undoing an unknown run", post(gainRunsPath+"/"+unknownRun+"/undo", "", fx.token), http.StatusNotFound},
		{"no credentials for the status", invalidCall(get(gainPath, "")), http.StatusUnauthorized},
		{"no credentials to run", post(gainRunsPath, `{}`, ""), http.StatusUnauthorized},
		{"no credentials for a run", invalidCall(get(gainRunsPath+"/"+runID, "")), http.StatusUnauthorized},
		{"no credentials to stop", post(gainRunsPath+"/"+runID+"/stop", "", ""), http.StatusUnauthorized},
		{"no credentials to undo", post(gainRunsPath+"/"+runID+"/undo", "", ""), http.StatusUnauthorized},
		{"a listener reads the status", get(gainPath, user), http.StatusForbidden},
		{"a listener runs", post(gainRunsPath, `{}`, user), http.StatusForbidden},
		{"a listener reads a run", get(gainRunsPath+"/"+runID, user), http.StatusForbidden},
		{"a listener stops", post(gainRunsPath+"/"+runID+"/stop", "", user), http.StatusForbidden},
		{"a listener undoes", post(gainRunsPath+"/"+runID+"/undo", "", user), http.StatusForbidden},
	} {
		if resp := fx.send(t, tc.c); resp.Status != tc.status {
			t.Errorf("%s: status = %d, want %d: %s", tc.name, resp.Status, tc.status, resp.Body)
		}
	}
}

// Administrators hear a run's progress, by album and song count, never a
// path; a listener hears none of it.
func TestEventsOfAReplayGainRun(t *testing.T) {
	t.Parallel()
	fx := gainFixture(t, true)
	admin := fx.openStream(t, fx.token, "")
	admin.want(events.Ready)
	listener := fx.openStream(t, fx.tokenFor(t, store.RoleUser), "")
	listener.want(events.Ready)
	run := fx.startGain(t, gainRunsPath, `{}`)
	var ev api.EventReplayGain
	for ev.State != store.GainDone {
		f := admin.until(events.ReplayGain)
		decodeEvent(t, f, &ev)
		if ev.Run != run.ID || strings.Contains(f.data, fx.root) || strings.Contains(f.data, "/") {
			t.Fatalf("event = %s, want the run's progress and no path", f.data)
		}
	}
	if ev.Done != 2 || ev.Total != 2 || ev.AlbumsDone != 1 {
		t.Errorf("last event = %+v, want two songs of two, one album done", ev)
	}
	fx.ping()
	listener.want(events.Library)
}

// No response names a library root.
func TestReplayGainNamesNoRoot(t *testing.T) {
	t.Parallel()
	fx := gainFixture(t, true)
	if err := os.Chmod(filepath.Join(fx.root, fileTwo), 0o400); err != nil {
		t.Fatal(err)
	}
	run := fx.startGain(t, gainRunsPath, `{}`)
	d := fx.waitGain(t, run.ID)
	if len(d.Left) != 1 || d.Left[0].Path != fileTwo || d.Left[0].Root != 0 || d.Left[0].Reason != "unwritable" {
		t.Errorf("left = %+v, want the read-only file, by its path under the root", d.Left)
	}
	for _, path := range []string{gainPath, gainRunsPath + "/" + run.ID} {
		if raw := fx.send(t, call{method: http.MethodGet, path: path, header: bearer(fx.token)}); strings.Contains(string(raw.Body), fx.root) {
			t.Errorf("%s holds the root: %s", path, raw.Body)
		}
	}
}

// bigSine makes the sine at path tens of megabytes, with pictures, so a stream
// of it outlasts the socket's buffers.
func bigSine(t *testing.T, path string) {
	t.Helper()
	img := make([]byte, 8<<20)
	copy(img, []byte{0xFF, 0xD8, 0xFF, 0xE0}) // A JPEG's start; nothing decodes the rest.
	err := audiometa.Write(path, func(f *audiometa.File) error {
		for _, kind := range []audiometa.ArtworkType{audiometa.ArtworkBackCover, audiometa.ArtworkMedia, audiometa.ArtworkOther} {
			if err := f.AddArtwork(audiometa.Artwork{MIMEType: mediaJPEG, Type: kind, Data: img}); err != nil {
				return fmt.Errorf("add a picture: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if info, statErr := os.Stat(path); statErr != nil || info.Size() < 24<<20 {
		t.Fatalf("the big sine = %v, %v", info, statErr)
	}
}

// A song streaming while its file is written plays on: the stream holds the
// file open, so the run rewrites it to a new file, and the stream reads the
// file it opened to its end.
func TestReplayGainKeepsAStreamPlaying(t *testing.T) {
	t.Parallel()
	fx := gainFixture(t, true)
	path := filepath.Join(fx.root, fileOne)
	bigSine(t, path)
	if _, err := fx.Scanner.Scan(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.Scanner.MeasureLoudness(t.Context()); err != nil {
		t.Fatal(err)
	}
	fx.reload(t)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := fx.openTrackStream(t, "one")
	got := make([]byte, 4096, len(original))
	if _, err = io.ReadFull(body, got); err != nil {
		t.Fatal(err)
	}
	if !fx.Handle.streams.Open(path) {
		t.Fatal("the stream doesn't hold the file open")
	}

	fx.waitGain(t, fx.startGain(t, gainRunsPath, `{}`).ID)

	rest, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if got = append(got, rest...); !slices.Equal(got, original) {
		t.Errorf("the stream read %d bytes that aren't the file it opened (%d bytes)", len(got), len(original))
	}
	if now, readErr := os.ReadFile(path); readErr != nil || slices.Equal(now, original) {
		t.Errorf("the streamed file wasn't written: %v", readErr)
	}
}

// openTrackStream starts streaming the album's song titled title, and returns
// its body, unread.
func (fx *editFixture) openTrackStream(t *testing.T, title string) io.Reader {
	t.Helper()
	i := slices.IndexFunc(fx.album.Tracks, func(tr api.Track) bool { return tr.Title == title })
	if i < 0 {
		t.Fatalf("no song %q in %+v", title, fx.album.Tracks)
	}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fx.URL+"/tracks/"+fx.album.Tracks[i].ID+"/stream", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header = bearer(fx.token)
	// Not the contract's client, which reads a body whole to validate it.
	resp, err := fx.Server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp.Body
}
