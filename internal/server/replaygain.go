package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// biggestGainAlbums is how many albums GET /admin/replaygain names.
	biggestGainAlbums = 4
	// mostGainAlbums caps the albums a run names.
	mostGainAlbums = 1000
	// gainOffDetail is what a run is told while tag writes are off.
	gainOffDetail = "tag writes are off, so nothing on disk changes: library.tag_writes"
)

// getReplayGain answers what Kitsune measured and could write into the files.
func (s *Server) getReplayGain(w http.ResponseWriter, r *http.Request) {
	lib, err := s.scanner.GainLibrary(r.Context())
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "read ReplayGain", err)
		}
		return
	}
	runs, err := s.library.GainRuns(r.Context(), runsShown)
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "read ReplayGain", err)
		}
		return
	}
	body := api.ReplayGainStatus{
		Songs: lib.Songs(), Albums: len(lib.Albums), Formats: lib.Formats(), Biggest: biggestOf(lib),
		Waiting: lib.Waiting, Unmeasurable: lib.Unmeasurable, Runs: make([]api.GainRun, len(runs)),
	}
	for i := range runs {
		body.Runs[i] = s.apiGainRun(&runs[i])
		if body.Runs[i].State == store.GainRunning && s.scanner.GainProgress() != nil {
			body.Run = &body.Runs[i]
		}
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// biggestOf returns lib's albums with the most songs to write, most first.
func biggestOf(lib *store.GainLibrary) []api.GainAlbum {
	order := make([]int, len(lib.Albums))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return len(lib.Albums[b].Songs) - len(lib.Albums[a].Songs) })
	out := make([]api.GainAlbum, 0, min(biggestGainAlbums, len(order)))
	for _, i := range order[:min(biggestGainAlbums, len(order))] {
		out = append(out, apiGainAlbum(&lib.Albums[i]))
	}
	return out
}

// apiGainAlbum builds the API's view of an album a run may write.
func apiGainAlbum(a *store.GainAlbum) api.GainAlbum {
	return api.GainAlbum{
		ID: a.ID, Title: a.Title, Artist: a.Artist, Year: a.Year, Artwork: toAPIArtwork(a.Artwork),
		Formats: a.Formats(), Songs: len(a.Songs),
	}
}

// postReplayGainRun starts writing Kitsune's measure into the files.
func (s *Server) postReplayGainRun(w http.ResponseWriter, r *http.Request) {
	var req api.GainRunRequest
	if err := readJSON(w, r, &req); err != nil || len(req.Albums) > mostGainAlbums {
		writeProblem(w, r, http.StatusBadRequest, fmt.Sprintf("send JSON naming at most %d albums, or none for all", mostGainAlbums))
		return
	}
	if !s.scanner.TagWrites() {
		writeProblem(w, r, http.StatusConflict, gainOffDetail)
		return
	}
	user, _ := currentUser(r.Context())
	//nolint:contextcheck // A run outlives its request on purpose, as an organize run does; see Config.Context.
	run, fn, err := s.scanner.StartGain(s.editContext, req.Albums, user.ID)
	if err != nil {
		s.gainFailed(w, r, "start ReplayGain run", err)
		return
	}
	s.startGainRun(w, r, run.ID, fn)
}

// startGainRun hands fn to the server's background work and answers 202 with
// the run.
func (s *Server) startGainRun(w http.ResponseWriter, r *http.Request, id string, fn func(context.Context)) {
	if err := s.work(fn); err != nil {
		s.internalError(w, r, "start ReplayGain run", err)
		return
	}
	info, err := s.library.GainRunByULID(r.Context(), id)
	if err != nil {
		s.internalError(w, r, "read ReplayGain run", err)
		return
	}
	body := s.apiGainRun(&info)
	writeBody(w, http.StatusAccepted, api.JSONMediaType, &body)
}

// gainFailed answers a run or an undo that didn't start.
func (s *Server) gainFailed(w http.ResponseWriter, r *http.Request, what string, err error) {
	switch {
	case errors.Is(err, library.ErrNothingToWrite):
		writeProblem(w, r, http.StatusUnprocessableEntity, "nothing of that can be written now: every song carries ReplayGain, or still waits for its measure")
	case errors.Is(err, library.ErrTagWritesOff):
		writeProblem(w, r, http.StatusConflict, gainOffDetail)
	case errors.Is(err, library.ErrBusy):
		w.Header().Set("Retry-After", retryAfterSeconds)
		writeProblem(w, r, http.StatusConflict, "a run, a scan or an edit is holding the library")
	case errors.Is(err, library.ErrRootDown):
		w.Header().Set("Retry-After", retryAfterSeconds)
		writeProblem(w, r, http.StatusConflict, "a library folder the run would touch looks down: missing, unreadable or empty")
	case errors.Is(err, library.ErrNothingToUndo):
		writeProblem(w, r, http.StatusConflict, "nothing that run wrote is left to take out, or it is running or an undo")
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "no run has that ID")
	default:
		s.internalError(w, r, what, err)
	}
}

// getReplayGainRun reads one run, its albums and the songs it left.
func (s *Server) getReplayGainRun(w http.ResponseWriter, r *http.Request) {
	info, err := s.library.GainRunByULID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "no run has that ID")
		return
	}
	if err != nil {
		s.internalError(w, r, "read ReplayGain run", err)
		return
	}
	albums, err := s.gainRunAlbums(r.Context(), &info)
	if err != nil {
		s.internalError(w, r, "read ReplayGain run", err)
		return
	}
	left, err := s.library.LeftGain(r.Context(), info.ID, mostLeft)
	if err != nil {
		s.internalError(w, r, "read ReplayGain run", err)
		return
	}
	roots := s.scanner.Roots()
	body := api.GainRunDetail{Run: s.apiGainRun(&info), Albums: albums, Left: make([]api.GainLeft, len(left))}
	for i := range left {
		l := &left[i]
		body.Left[i] = api.GainLeft{Track: l.ULID, Title: l.Title, Path: slash(l.RelPath), Root: rootIndex(roots, l.Root), Reason: l.Reason}
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// gainRunAlbums lists a run's albums in its order: while it runs, every album
// it set out to write, each written, writing or waiting; once it ended, those
// its journal holds, each written or left.
func (s *Server) gainRunAlbums(ctx context.Context, info *store.GainRunInfo) ([]api.GainRunAlbum, error) {
	journaled, err := s.library.GainRunAlbums(ctx, info.ID)
	if err != nil {
		return nil, fmt.Errorf("albums: %w", err)
	}
	byID := make(map[string]*store.GainRunAlbum, len(journaled))
	for i := range journaled {
		byID[journaled[i].Album] = &journaled[i]
	}
	plan := s.scanner.GainPlan(info.ULID)
	order := gainRunOrder(plan, journaled)
	ids := make([]string, len(order))
	for i := range order {
		ids[i] = order[i].id
	}
	facts, err := s.library.GainAlbumFacts(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("albums: %w", err)
	}
	inHand := ""
	if p := s.scanner.GainProgress(); p != nil && p.Run == info.ULID {
		inHand = p.Album
	}
	out := make([]api.GainRunAlbum, 0, len(order))
	for _, e := range order {
		f := facts[e.id]
		a := api.GainRunAlbum{
			ID: e.id, Title: f.Title, Artist: f.Artist, Year: f.Year, Artwork: toAPIArtwork(f.Artwork),
			Formats: append([]string{}, f.Formats...), Songs: e.songs,
		}
		j := byID[e.id]
		if j != nil {
			a.Written, a.Left = j.Written, j.Left
		}
		a.State = gainAlbumState(e.id == inHand, plan != nil, j)
		out = append(out, a)
	}
	return out, nil
}

// gainRunEntry is an album of a run's list: its ID and how many songs the run
// takes of it.
type gainRunEntry struct {
	id    string
	songs int
}

// gainRunOrder is a run's albums in its order: while it runs, every album of
// its plan; once it ended, those its journal holds.
func gainRunOrder(plan []library.GainPlanAlbum, journaled []store.GainRunAlbum) []gainRunEntry {
	if plan == nil {
		out := make([]gainRunEntry, len(journaled))
		for i := range journaled {
			out[i] = gainRunEntry{journaled[i].Album, journaled[i].Songs}
		}
		return out
	}
	out := make([]gainRunEntry, len(plan))
	for i, a := range plan {
		out[i] = gainRunEntry{a.ID, a.Songs}
	}
	return out
}

// gainAlbumState says how far a run got with an album: in hand, j its journal
// (nil for none yet), running whether the run goes on.
func gainAlbumState(inHand, running bool, j *store.GainRunAlbum) string {
	switch {
	case inHand:
		return api.GainAlbumWriting
	case j == nil || (running && j.Planned > 0):
		if running {
			return api.GainAlbumWaiting
		}
		return api.GainAlbumLeft
	case j.Written == 0:
		return api.GainAlbumLeft
	}
	return api.GainAlbumWritten
}

// postReplayGainRunStop asks a run to stop after its album in hand.
func (s *Server) postReplayGainRunStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	info, err := s.library.GainRunByULID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "no run has that ID")
		return
	}
	if err != nil {
		s.internalError(w, r, "stop ReplayGain run", err)
		return
	}
	if err = s.scanner.StopGain(id); errors.Is(err, library.ErrNotRunning) {
		writeProblem(w, r, http.StatusConflict, "that run isn't running")
		return
	}
	body := s.apiGainRun(&info)
	writeBody(w, http.StatusAccepted, api.JSONMediaType, &body)
}

// postReplayGainRunUndo starts an undo of a run.
func (s *Server) postReplayGainRunUndo(w http.ResponseWriter, r *http.Request) {
	if !s.scanner.TagWrites() {
		writeProblem(w, r, http.StatusConflict, gainOffDetail)
		return
	}
	user, _ := currentUser(r.Context())
	//nolint:contextcheck // As postReplayGainRun.
	run, fn, err := s.scanner.StartGainUndo(s.editContext, r.PathValue("id"), user.ID)
	if err != nil {
		s.gainFailed(w, r, "undo ReplayGain run", err)
		return
	}
	s.startGainRun(w, r, run.ID, fn)
}

// apiGainRun builds the API's view of a run, with a running one's live
// progress.
func (s *Server) apiGainRun(info *store.GainRunInfo) api.GainRun {
	out := api.GainRun{
		ID: info.ULID, Kind: info.Kind, State: info.State, StartedAt: info.StartedAt, FinishedAt: info.FinishedAt,
		By: info.By, Total: info.Total, Written: info.Written, Left: info.Left, Failed: info.Failed,
		Albums: info.Albums, UndoOf: info.UndoOf, UndoneBy: info.UndoneBy,
		Undoable: info.Kind == store.GainKindRun && info.StillWritten &&
			(info.State == store.GainDone || info.State == store.GainStopped),
	}
	out.Done = info.Written + info.Left + info.Failed
	if p := s.scanner.GainProgress(); p != nil && p.Run == info.ULID && info.State == store.GainRunning {
		out.Done, out.AlbumsDone, out.Album, out.Stopping = p.Done, p.AlbumsDone, p.Album, p.Stopping
	} else {
		out.AlbumsDone = info.AlbumsDone
	}
	return out
}
