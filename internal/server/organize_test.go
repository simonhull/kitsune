package server

import (
	"encoding/json/v2"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	organizePreviewPath  = "/admin/organize/preview"
	organizeExamplesPath = "/admin/organize/examples"
	organizePreviewsPath = "/admin/organize/previews/"
	organizeRunsPath     = "/admin/organize/runs"
	twinOne              = "twins/one/x.flac"
	twinTwo              = "twins/two/y.flac"
	looseMP3             = "loose.mp3"
	unknownRun           = "01J00000000000000000000000"
)

// organizeFixture is a server with tag writes on over two copies of one FLAC,
// which want one path, and an MP3 out of line.
func organizeFixture(t *testing.T, tagWrites bool) *editFixture {
	t.Helper()
	return newEditServer(t, &testServerOptions{TagWrites: tagWrites}, map[string]string{
		twinOne: sampleFLAC, twinTwo: sampleFLAC, looseMP3: sampleMP3,
	})
}

// organizeCall is a POST of body to path as the fixture's administrator.
func (fx *editFixture) organizeCall(path, body string) call {
	return call{method: http.MethodPost, path: path, body: body, header: jsonHeader(fx.token)}
}

// preview posts body to the preview route, wants a 200, and returns it.
func (fx *editFixture) preview(t *testing.T, body string) api.OrganizePreview {
	t.Helper()
	var p api.OrganizePreview
	resp := fx.send(t, fx.organizeCall(organizePreviewPath, body))
	if resp.Status != http.StatusOK {
		t.Fatalf("preview = %d: %s", resp.Status, resp.Body)
	}
	resp.decode(t, &p)
	return p
}

// startRun posts body to path, wants a 202, and returns the run.
func (fx *editFixture) startRun(t *testing.T, path, body string) api.OrganizeRun {
	t.Helper()
	var run api.OrganizeRun
	resp := fx.send(t, fx.organizeCall(path, body))
	if err := json.Unmarshal(resp.Body, &run); err != nil || resp.Status != http.StatusAccepted {
		t.Fatalf("POST %s = %d: %s", path, resp.Status, resp.Body)
	}
	return run
}

// waitRun waits for the run id to end, and returns how it did.
func (fx *editFixture) waitRun(t *testing.T, id string) api.OrganizeRunDetail {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var d api.OrganizeRunDetail
		fx.getJSON(t, fx.token, organizeRunsPath+"/"+id, &d)
		if d.Run.State != api.OrganizeRunning {
			return d
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %s still running: %+v", id, d.Run)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A preview finds the twins in conflict and looking like one recording, and
// the MP3 out of line; its pages filter and search.
func TestOrganizePreviewFindsConflictsAndSearches(t *testing.T) {
	t.Parallel()
	fx := organizeFixture(t, true)
	p := fx.preview(t, `{}`)
	if c := p.Counts; c.Songs != 3 || c.Conflicts != 2 || c.Moves != 1 || p.Scope != nil {
		t.Fatalf("counts = %+v, want 2 in conflict and 1 to move, of the whole library", c)
	}
	conflicts := fx.wantTwins(t, p.ID)
	found := fx.page(t, p.ID, "?filter=all&q="+strings.ToUpper("loose"))
	if found.Matching != 1 {
		t.Errorf("search = %+v, want the MP3's album", found)
	}
	fx.wantScoped(t, conflicts.Albums[0].ID)
}

// wantTwins fails unless the preview id's conflicts are the twins, each in its
// folder's album beside the other, the same recording, and returns the page.
func (fx *editFixture) wantTwins(t *testing.T, id string) api.OrganizePreview {
	t.Helper()
	conflicts := fx.page(t, id, "?filter=conflict")
	if len(conflicts.Albums) != 2 || len(conflicts.Albums[0].Files) != 1 {
		t.Fatalf("conflicts = %+v, want each twin's folder, an album of its own", conflicts.Albums)
	}
	if c := conflicts.Albums[0].Files[0].Conflict; c == nil || !c.SameRecording || len(c.With) != 1 || c.Self.Format == "" {
		t.Errorf("conflict = %+v, want the twin beside it, the same recording", c)
	}
	return conflicts
}

// wantScoped fails unless a preview of the album id says it is of its one song.
func (fx *editFixture) wantScoped(t *testing.T, id string) {
	t.Helper()
	scoped := fx.preview(t, `{"albums":["`+id+`"]}`)
	if scoped.Scope == nil || scoped.Scope.Songs != 1 || scoped.Scope.Album == "" {
		t.Errorf("scoped preview = %+v, want one album's song", scoped.Scope)
	}
}

// A run moves the MP3 and leaves the twins; Undo puts it back, and a second
// undo has nothing to do.
func TestOrganizeRunAndUndo(t *testing.T) {
	t.Parallel()
	fx := organizeFixture(t, true)
	p := fx.preview(t, `{}`)
	run := fx.startRun(t, organizeRunsPath, `{"previewId":"`+p.ID+`"}`)
	done := fx.waitRun(t, run.ID)
	if done.Run.State != api.OrganizeDone || done.Run.Moved != 1 || done.Run.Conflicts != 2 || !done.Run.Undoable {
		t.Errorf("run = %+v, want one moved and the twins in conflict", done.Run)
	}
	var runs api.OrganizeRuns
	fx.getJSON(t, fx.token, organizeRunsPath, &runs)
	if len(runs.Runs) != 1 || runs.Runs[0].ID != run.ID {
		t.Errorf("runs = %+v, want the run", runs.Runs)
	}
	undo := fx.startRun(t, organizeRunsPath+"/"+run.ID+"/undo", "")
	if back := fx.waitRun(t, undo.ID); back.Run.Kind != "undo" || back.Run.Moved != 1 || back.Run.UndoOf != run.ID {
		t.Errorf("undo = %+v, want the MP3 back", back.Run)
	}
	fx.send(t, fx.organizeCall(organizeRunsPath+"/"+run.ID+"/undo", "")).problem(t, http.StatusConflict)
}

// page reads a page of the preview id, with query.
func (fx *editFixture) page(t *testing.T, id, query string) api.OrganizePreview {
	t.Helper()
	var p api.OrganizePreview
	fx.getJSON(t, fx.token, organizePreviewsPath+id+query, &p)
	return p
}

// Examples show what a template reads of a few songs, never a root.
func TestOrganizeExamples(t *testing.T) {
	t.Parallel()
	fx := organizeFixture(t, true)
	var ex api.OrganizeExamples
	fx.getJSON(t, fx.token, organizeExamplesPath+"?count=2&seed=7", &ex)
	if len(ex.Songs) != 2 || ex.Songs[0].Song.Title == "" || ex.Songs[0].Root != 0 {
		t.Errorf("examples = %+v, want two songs with their tags", ex.Songs)
	}
	raw := fx.send(t, call{method: http.MethodGet, path: organizeExamplesPath, header: bearer(fx.token)})
	if strings.Contains(string(raw.Body), fx.root) {
		t.Errorf("examples hold the root: %s", raw.Body)
	}
}

// A template that doesn't parse is a 422 that says which and where.
func TestOrganizePreviewOfABadTemplate(t *testing.T) {
	t.Parallel()
	fx := organizeFixture(t, true)
	resp := fx.send(t, fx.organizeCall(organizePreviewPath, `{"compilations":"<Album>/<Tilte>"}`))
	var p api.OrganizeTemplateProblem
	if err := json.Unmarshal(resp.Body, &p); err != nil || resp.Status != http.StatusUnprocessableEntity ||
		p.Field != "compilations" || p.At != 8 {
		t.Errorf("bad template = %d %+v, want a 422 at 8 of compilations", resp.Status, p)
	}
}

// Stop on a run that isn't running is a 409; on one that is, it stops after
// its album in hand.
func TestOrganizeStop(t *testing.T) {
	t.Parallel()
	fx := organizeFixture(t, true)
	p := fx.preview(t, `{}`)
	run := fx.startRun(t, organizeRunsPath, `{"previewId":"`+p.ID+`"}`)
	fx.send(t, fx.organizeCall(organizeRunsPath+"/"+run.ID+"/stop", "")) // Running or done: either answer the contract.
	fx.waitRun(t, run.ID)
	fx.send(t, fx.organizeCall(organizeRunsPath+"/"+run.ID+"/stop", "")).problem(t, http.StatusConflict)
}

// With tag writes off a preview still works, and a run or an undo is a 409.
func TestOrganizeNeedsTagWritesToMove(t *testing.T) {
	t.Parallel()
	fx := organizeFixture(t, false)
	p := fx.preview(t, `{}`)
	fx.send(t, fx.organizeCall(organizeRunsPath, `{"previewId":"`+p.ID+`"}`)).problem(t, http.StatusConflict)
	fx.send(t, fx.organizeCall(organizeRunsPath+"/"+unknownRun+"/undo", "")).problem(t, http.StatusConflict)
	var me api.Me
	fx.getJSON(t, fx.token, "/me", &me)
	if me.CanOrganize {
		t.Error("Me.canOrganize with tag writes off, want false")
	}
}

// Every refusal: bad requests, things that aren't there, and callers who may not.
func TestOrganizeRefusals(t *testing.T) {
	t.Parallel()
	fx := organizeFixture(t, true)
	user := fx.tokenFor(t, store.RoleUser)
	p := fx.preview(t, `{}`)
	runID := fx.startRun(t, organizeRunsPath, `{"previewId":"`+p.ID+`"}`).ID
	fx.waitRun(t, runID)
	for _, tc := range []struct {
		name   string
		c      call
		status int
	}{
		{"a bad preview body", invalidCall(fx.organizeCall(organizePreviewPath, `{"songs":[]}`)), http.StatusBadRequest},
		{"a song not in the library", fx.organizeCall(organizePreviewPath, `{"tracks":["`+unknownRun+`"]}`), http.StatusNotFound},
		{"a bad filter", invalidCall(call{method: http.MethodGet, path: organizePreviewsPath + p.ID + "?filter=nope", header: bearer(fx.token)}), http.StatusBadRequest},
		{"a bad cursor", call{method: http.MethodGet, path: organizePreviewsPath + p.ID + "?cursor=x", header: bearer(fx.token)}, http.StatusBadRequest},
		{"a preview gone", call{method: http.MethodGet, path: organizePreviewsPath + "gone", header: bearer(fx.token)}, http.StatusNotFound},
		{"bad examples", invalidCall(call{method: http.MethodGet, path: organizeExamplesPath + "?count=99", header: bearer(fx.token)}), http.StatusBadRequest},
		{"a bad run body", invalidCall(fx.organizeCall(organizeRunsPath, `{}`)), http.StatusBadRequest},
		{"a run of a preview gone", fx.organizeCall(organizeRunsPath, `{"previewId":"gone"}`), http.StatusNotFound},
		{"an unknown run", call{method: http.MethodGet, path: organizeRunsPath + "/" + unknownRun, header: bearer(fx.token)}, http.StatusNotFound},
		{"stopping an unknown run", fx.organizeCall(organizeRunsPath+"/"+unknownRun+"/stop", ""), http.StatusNotFound},
		{"undoing an unknown run", fx.organizeCall(organizeRunsPath+"/"+unknownRun+"/undo", ""), http.StatusNotFound},
		{"no credentials to preview", call{method: http.MethodPost, path: organizePreviewPath, body: `{}`, header: jsonHeader("")}, http.StatusUnauthorized},
		{"no credentials for examples", invalidCall(call{method: http.MethodGet, path: organizeExamplesPath}), http.StatusUnauthorized},
		{"no credentials to page", invalidCall(call{method: http.MethodGet, path: organizePreviewsPath + p.ID}), http.StatusUnauthorized},
		{"no credentials to list", invalidCall(call{method: http.MethodGet, path: organizeRunsPath}), http.StatusUnauthorized},
		{"no credentials to run", call{method: http.MethodPost, path: organizeRunsPath, body: `{"previewId":"x"}`, header: jsonHeader("")}, http.StatusUnauthorized},
		{"no credentials for a run", invalidCall(call{method: http.MethodGet, path: organizeRunsPath + "/" + runID}), http.StatusUnauthorized},
		{"no credentials to stop", call{method: http.MethodPost, path: organizeRunsPath + "/" + runID + "/stop", header: jsonHeader("")}, http.StatusUnauthorized},
		{"no credentials to undo", call{method: http.MethodPost, path: organizeRunsPath + "/" + runID + "/undo", header: jsonHeader("")}, http.StatusUnauthorized},
		{"a listener previews", call{method: http.MethodPost, path: organizePreviewPath, body: `{}`, header: jsonHeader(user)}, http.StatusForbidden},
		{"a listener asks for examples", call{method: http.MethodGet, path: organizeExamplesPath, header: bearer(user)}, http.StatusForbidden},
		{"a listener pages", call{method: http.MethodGet, path: organizePreviewsPath + p.ID, header: bearer(user)}, http.StatusForbidden},
		{"a listener lists runs", call{method: http.MethodGet, path: organizeRunsPath, header: bearer(user)}, http.StatusForbidden},
		{"a listener runs", call{method: http.MethodPost, path: organizeRunsPath, body: `{"previewId":"` + p.ID + `"}`, header: jsonHeader(user)}, http.StatusForbidden},
		{"a listener reads a run", call{method: http.MethodGet, path: organizeRunsPath + "/" + runID, header: bearer(user)}, http.StatusForbidden},
		{"a listener stops", call{method: http.MethodPost, path: organizeRunsPath + "/" + runID + "/stop", header: jsonHeader(user)}, http.StatusForbidden},
		{"a listener undoes", call{method: http.MethodPost, path: organizeRunsPath + "/" + runID + "/undo", header: jsonHeader(user)}, http.StatusForbidden},
	} {
		if resp := fx.send(t, tc.c); resp.Status != tc.status {
			t.Errorf("%s: status = %d, want %d: %s", tc.name, resp.Status, tc.status, resp.Body)
		}
	}
}

// A run claims the library: a second run, or a scan's claim, meets a 409 with
// Retry-After while it holds it.
func TestOrganizeRunWhileBusy(t *testing.T) {
	t.Parallel()
	fx := organizeFixture(t, true)
	p := fx.preview(t, `{}`)
	release, err := fx.Scanner.Claim(0)
	if err != nil {
		t.Fatal(err)
	}
	resp := fx.send(t, fx.organizeCall(organizeRunsPath, `{"previewId":"`+p.ID+`"}`))
	if resp.Status != http.StatusConflict || resp.Header.Get("Retry-After") == "" {
		t.Errorf("a run during a scan = %d %v, want a 409 with Retry-After", resp.Status, resp.Header)
	}
	if _, err = release(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// Administrators hear a run's progress, by album and song count, never a path;
// a listener hears none of it.
func TestEventsOfAnOrganizeRun(t *testing.T) {
	t.Parallel()
	fx := organizeFixture(t, true)
	admin := fx.openStream(t, fx.token, "")
	admin.want(events.Ready)
	listener := fx.openStream(t, fx.tokenFor(t, store.RoleUser), "")
	listener.want(events.Ready)
	p := fx.preview(t, `{}`)
	run := fx.startRun(t, organizeRunsPath, `{"previewId":"`+p.ID+`"}`)
	var ev api.EventOrganize
	for ev.State != api.OrganizeDone {
		f := admin.until(events.Organize)
		decodeEvent(t, f, &ev)
		if ev.Run != run.ID || strings.Contains(f.data, fx.root) || strings.Contains(f.data, "/") {
			t.Fatalf("event = %s, want the run's progress and no path", f.data)
		}
	}
	if ev.Done != 1 || ev.Total != 1 || ev.AlbumsDone != 1 {
		t.Errorf("last event = %+v, want one song of one, one album done", ev)
	}
	fx.ping()
	listener.want(events.Library)
}
