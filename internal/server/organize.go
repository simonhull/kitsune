package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/organize"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// previewLife is how long a preview is kept for its pages and its run.
	previewLife = 30 * time.Minute
	// mostPreviews is how many previews are kept at once: the oldest goes first.
	mostPreviews = 8
	// previewPage is how many albums a page of a preview holds.
	previewPage = 200
	// mostOrganizeIDs caps the songs a preview or a run names, and mostOrganizeAlbums its albums.
	mostOrganizeIDs    = 10000
	mostOrganizeAlbums = 1000
	// mostLeft is how many songs a run's detail names.
	mostLeft = 500
	// mostExamples and defaultExamples bound GET /admin/organize/examples.
	mostExamples, defaultExamples = 12, 3
	// sameLength is how far apart two songs' lengths may be and still look like
	// one recording.
	sameLength = 2 * time.Second
	// runsShown is how many runs the list shows.
	runsShown = 10
	// organizeOffDetail is what a run is told while tag writes are off.
	organizeOffDetail = "tag writes are off, so nothing on disk changes: library.tag_writes"
)

// previews are the previews kept for their pages and runs.
type previews struct {
	mu   sync.Mutex
	byID map[string]*keptPreview
}

// keptPreview is a preview, with what its pages read of it.
type keptPreview struct {
	id     string
	at     time.Time
	p      *library.OrganizePreview
	albums []previewAlbum // The songs by album, in the run's order.
}

// size is how many songs album i has in the preview.
func (k *keptPreview) size(i int) int { return len(k.albums[i].songs) }

// previewAlbum is one album's songs in a preview, by index into its Songs.
type previewAlbum struct {
	id    string
	songs []int
}

// keep keeps p, dropping what is past its time and the oldest past
// mostPreviews, and returns it with its ID.
func (ps *previews) keep(id string, p *library.OrganizePreview, now time.Time) *keptPreview {
	k := &keptPreview{id: id, at: now, p: p}
	for i := range p.Songs {
		album := p.Songs[i].Album
		if n := len(k.albums); n == 0 || k.albums[n-1].id != album {
			k.albums = append(k.albums, previewAlbum{id: album})
		}
		last := &k.albums[len(k.albums)-1]
		last.songs = append(last.songs, i)
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.byID == nil {
		ps.byID = map[string]*keptPreview{}
	}
	ps.prune(now)
	ps.byID[id] = k
	return k
}

// prune drops the previews past their time, and the oldest when there are
// mostPreviews. The caller holds mu.
func (ps *previews) prune(now time.Time) {
	var oldest *keptPreview
	for key, v := range ps.byID {
		switch {
		case now.Sub(v.at) > previewLife:
			delete(ps.byID, key)
		case oldest == nil || v.at.Before(oldest.at):
			oldest = v
		}
	}
	if len(ps.byID) >= mostPreviews && oldest != nil {
		delete(ps.byID, oldest.id)
	}
}

// get returns the preview id while it is kept.
func (ps *previews) get(id string, now time.Time) (*keptPreview, bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	k, ok := ps.byID[id]
	if !ok || now.Sub(k.at) > previewLife {
		return nil, false
	}
	return k, true
}

// postOrganizePreview previews where the templates would file the library, or
// some of it, keeps the preview and answers with its first page.
func (s *Server) postOrganizePreview(w http.ResponseWriter, r *http.Request) {
	var req api.OrganizeRequest
	if err := readJSON(w, r, &req); err != nil || !fewIDs(req.Tracks, req.Albums) {
		writeProblem(w, r, http.StatusBadRequest, fmt.Sprintf("send JSON with templates, and at most %d songs or %d albums, not both", mostOrganizeIDs, mostOrganizeAlbums))
		return
	}
	c, ok := s.organizeConfig(w, r, &req)
	if !ok {
		return
	}
	p, err := s.scanner.PreviewOrganize(r.Context(), &c, library.OrganizeScope{Tracks: req.Tracks, Albums: req.Albums})
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "a song or an album named isn't in the library")
		return
	case err != nil:
		if r.Context().Err() == nil {
			s.internalError(w, r, "preview organize", err)
		}
		return
	}
	id, err := library.NewID(s.now())
	if err != nil {
		s.internalError(w, r, "preview organize", err)
		return
	}
	k := s.previews.keep(id, p, s.now())
	s.writePreviewPage(w, r, k, api.OrganizeKindAll, "", 0)
}

// fewIDs reports whether a request names songs or albums, not both, within
// the limits.
func fewIDs(tracks, albums []string) bool {
	return (len(tracks) == 0 || len(albums) == 0) && len(tracks) <= mostOrganizeIDs && len(albums) <= mostOrganizeAlbums
}

// organizeConfig is how a preview files songs: the saved configuration, with
// the request's templates over its own. It writes a 422 naming the template
// that doesn't parse, and where, and reports false.
func (s *Server) organizeConfig(w http.ResponseWriter, r *http.Request, req *api.OrganizeRequest) (library.OrganizeConfig, bool) {
	c := *s.scanner.Organize()
	for _, t := range []struct {
		field, text string
		into        *organize.Template
	}{{"template", req.Template, &c.Template}, {"compilations", req.Compilations, &c.Compilations}} {
		if t.text == "" {
			continue
		}
		parsed, err := organize.Parse(t.text)
		if te, bad := errors.AsType[*organize.TemplateError](err); bad {
			body := api.OrganizeTemplateProblem{Problem: newProblem(r, http.StatusUnprocessableEntity, te.Reason), Field: t.field, At: te.At}
			writeBody(w, http.StatusUnprocessableEntity, api.ProblemMediaType, &body)
			return c, false
		}
		*t.into = parsed
		c.Roots = nil // Templates sent are for every folder.
	}
	return c, true
}

// getOrganizePreview pages a kept preview.
func (s *Server) getOrganizePreview(w http.ResponseWriter, r *http.Request) {
	k, ok := s.previews.get(r.PathValue("id"), s.now())
	if !ok {
		writeProblem(w, r, http.StatusNotFound, "that preview has gone: preview again")
		return
	}
	query := r.URL.Query()
	filter := query.Get("filter")
	if filter == "" {
		filter = api.OrganizeKindAll
	}
	at := 0
	if cursor := query.Get("cursor"); cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil || n < 0 {
			writeProblem(w, r, http.StatusBadRequest, "that cursor isn't one a page gave")
			return
		}
		at = n
	}
	if !validFilter(filter) || len(query.Get("q")) > 200 {
		writeProblem(w, r, http.StatusBadRequest, "filter is all, rename, move, conflict, skipped or inPlace, and q at most 200 characters")
		return
	}
	s.writePreviewPage(w, r, k, filter, query.Get("q"), at)
}

// validFilter reports whether f is a filter a page takes.
func validFilter(f string) bool {
	switch f {
	case api.OrganizeKindAll, api.OrganizeKindRename, api.OrganizeKindMove, api.OrganizeKindConflict, api.OrganizeKindSkipped, api.OrganizeKindInPlace:
		return true
	}
	return false
}

// kindName is a plan kind as the API names it.
func kindName(k organize.Kind) string { return k.String() }

// matches reports whether a song of kind k shows under filter.
func matches(filter string, k organize.Kind) bool {
	if filter == api.OrganizeKindAll {
		return k.Moves()
	}
	return kindName(k) == filter
}

// writePreviewPage writes the page of k's albums from at, under filter and q.
func (s *Server) writePreviewPage(w http.ResponseWriter, r *http.Request, k *keptPreview, filter, q string, at int) {
	p := k.p
	c := p.Plan.Counts
	body := api.OrganizePreview{
		ID: k.id, Template: p.Templates.Template.String(), Compilations: p.Templates.Compilations.String(), Filter: filter,
		Counts: api.OrganizeCounts{
			Songs: c.Songs, InPlace: c.InPlace, Moves: c.Renames + c.Moves, Renames: c.Renames, NewFolders: c.Moves,
			Conflicts: c.Conflicts, Skipped: c.Skipped, Folders: p.Plan.Folders,
		},
		Albums: []api.OrganizeAlbum{},
	}
	if p.Scoped {
		body.Scope = scopeOf(k)
	}
	needle := strings.ToLower(q)
	var shown []shownAlbum
	for i := range k.albums {
		if songs := k.shownSongs(i, filter, needle); len(songs) > 0 {
			shown = append(shown, shownAlbum{songs: songs, size: k.size(i)})
		}
	}
	body.Matching = len(shown)
	roots := s.scanner.Roots()
	user, _ := currentUser(r.Context())
	page := shown[min(at, len(shown)):min(at+previewPage, len(shown))]
	conflicts, err := s.claimantsOf(r.Context(), user.ID, p, page)
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "page a preview", err)
		}
		return
	}
	companions := companionsBySong(p)
	for _, a := range page {
		body.Albums = append(body.Albums, previewAlbumOf(p, a.songs, a.size, roots, conflicts, companions))
	}
	if at+previewPage < len(shown) {
		body.NextCursor = strconv.Itoa(at + previewPage)
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// shownAlbum is an album of a page: the songs of it that show, and how many
// it has in the preview.
type shownAlbum struct {
	songs []int
	size  int
}

// scopeOf says which songs a scoped preview holds.
func scopeOf(k *keptPreview) *api.OrganizeScope {
	sc := &api.OrganizeScope{Songs: len(k.p.Songs), Albums: len(k.albums)}
	if len(k.albums) == 1 && len(k.p.Songs) > 0 {
		sc.Album = k.p.Songs[0].AlbumTitle
	}
	return sc
}

// shownSongs returns album i's songs that show under filter and needle, a
// lowered search.
func (k *keptPreview) shownSongs(i int, filter, needle string) []int {
	var out []int
	for _, at := range k.albums[i].songs {
		if matches(filter, k.p.Plan.Songs[at].Kind) && (needle == "" || k.found(at, needle)) {
			out = append(out, at)
		}
	}
	return out
}

// found reports whether song at holds needle in its artist, album, title or
// either path.
func (k *keptPreview) found(at int, needle string) bool {
	song := &k.p.Songs[at]
	for _, hay := range []string{song.AlbumArtist, song.AlbumTitle, song.Title, song.RelPath, k.p.Plan.Songs[at].To} {
		if strings.Contains(strings.ToLower(hay), needle) {
			return true
		}
	}
	return slices.ContainsFunc(song.Artists, func(n string) bool { return strings.Contains(strings.ToLower(n), needle) })
}

// companionsBySong returns p's companions by the song they follow.
func companionsBySong(p *library.OrganizePreview) map[int64][]organize.Companion {
	out := map[int64][]organize.Companion{}
	for _, c := range p.Plan.Companions {
		out[c.Song] = append(out[c.Song], c)
	}
	return out
}

// rootIndex is root's index in roots, or -1.
func rootIndex(roots []string, root string) int {
	for i, r := range roots {
		if r == root {
			return i
		}
	}
	return -1
}

// previewAlbumOf builds one album of a page from the songs of p that show.
func previewAlbumOf(p *library.OrganizePreview, songs []int, size int, roots []string, conflicts map[int]*api.OrganizeConflict, companions map[int64][]organize.Companion) api.OrganizeAlbum {
	first := &p.Songs[songs[0]]
	a := api.OrganizeAlbum{
		ID: first.Album, Title: first.AlbumTitle, Artist: first.AlbumArtist, Artwork: toAPIArtwork(first.Artwork),
		Root: rootIndex(roots, first.Root), Songs: size, Files: make([]api.OrganizeFile, 0, len(songs)),
		Companions: []api.OrganizeCompanion{},
	}
	kinds := map[string]bool{}
	from, to := map[string]bool{}, map[string]bool{}
	for _, at := range songs {
		song, pl := &p.Songs[at], &p.Plan.Songs[at]
		f := api.OrganizeFile{
			Track: song.ULID, Title: song.Title, From: slash(song.RelPath), To: pl.To, Kind: kindName(pl.Kind),
			CaseOnly: pl.CaseOnly, Reason: string(pl.Reason), Unknown: pl.Unknown, Conflict: conflicts[at],
		}
		a.Files = append(a.Files, f)
		kinds[f.Kind] = true
		if f.To != "" {
			from[path.Dir(f.From)], to[path.Dir(f.To)] = true, true
		}
		for _, c := range companions[song.ID] {
			a.Companions = append(a.Companions, api.OrganizeCompanion{From: c.From, To: c.To})
		}
	}
	a.Kind = api.OrganizeKindMixed
	if len(kinds) == 1 {
		a.Kind = a.Files[0].Kind
	}
	if len(from) == 1 && len(to) == 1 && onlyKey(from) != onlyKey(to) {
		a.FromFolder, a.ToFolder = onlyKey(from)+"/", onlyKey(to)+"/"
		a.CaseOnly = strings.EqualFold(a.FromFolder, a.ToFolder)
	}
	return a
}

// onlyKey returns the one key of m.
func onlyKey(m map[string]bool) string {
	for k := range m {
		return k
	}
	return ""
}

// slash returns a store path slash-separated, as the API sends paths.
func slash(rel string) string { return strings.ReplaceAll(rel, "\\", "/") }

// claimantsOf returns the conflict of each song of page in a claimed conflict,
// by its index in p, with the songs in its way, read for the user.
func (s *Server) claimantsOf(ctx context.Context, userID int64, p *library.OrganizePreview, page []shownAlbum) (map[int]*api.OrganizeConflict, error) {
	claimed := claimedOn(p, page)
	out := make(map[int]*api.OrganizeConflict, len(claimed.songs))
	if len(claimed.songs) == 0 {
		return out, nil
	}
	facts, err := s.library.OrganizeClaimants(ctx, userID, claimed.rows)
	if err != nil {
		return nil, fmt.Errorf("conflicts: %w", err)
	}
	roots := s.scanner.Roots()
	for _, at := range claimed.songs {
		pl := &p.Plan.Songs[at]
		self := facts[p.Songs[at].ID]
		c := &api.OrganizeConflict{Self: claimant(&self, pl.To, roots), With: make([]api.OrganizeClaimant, 0, len(pl.With))}
		for _, id := range pl.With {
			other, ok := facts[id]
			if !ok {
				continue
			}
			c.With = append(c.With, claimant(&other, pl.To, roots))
			if why := sameRecording(&self, &other); why != "" && c.Why == "" {
				c.SameRecording, c.Why = true, why
			}
		}
		out[at] = c
	}
	return out, nil
}

// claimed is what the claimed conflicts of a page name.
type claimed struct {
	songs []int   // The songs of the page in a claimed conflict, by index in the preview.
	rows  []int64 // Every song those conflicts name, theirs included, by row.
}

// claimedOn returns the claimed conflicts of page.
func claimedOn(p *library.OrganizePreview, page []shownAlbum) claimed {
	var c claimed
	for _, a := range page {
		for _, at := range a.songs {
			if pl := &p.Plan.Songs[at]; pl.Reason == organize.Claimed {
				c.songs = append(c.songs, at)
				c.rows = append(c.rows, p.Songs[at].ID)
				c.rows = append(c.rows, pl.With...)
			}
		}
	}
	return c
}

// claimant is one song of a conflict over the path to.
func claimant(c *store.OrganizeClaimant, to string, roots []string) api.OrganizeClaimant {
	return api.OrganizeClaimant{
		Track: c.ULID, Title: c.Title, Album: c.Album, AlbumID: c.AlbumID, Path: slash(c.RelPath), Root: rootIndex(roots, c.Root),
		Format: c.Format, Bitrate: c.Bitrate, DurationMs: c.Duration.Milliseconds(), Plays: c.Plays,
		Stars: c.Stars.Stars(), AddedAt: c.AddedAt, Here: strings.EqualFold(slash(c.RelPath), to),
	}
}

// sameRecording says why a and b look like one recording, "digest" or
// "tags", or "" when they don't.
func sameRecording(a, b *store.OrganizeClaimant) string {
	if a.Digest != nil && b.Digest != nil && *a.Digest == *b.Digest {
		return "digest"
	}
	gap := a.Duration - b.Duration
	if strings.EqualFold(a.Title, b.Title) && strings.EqualFold(a.Artist, b.Artist) && gap <= sameLength && gap >= -sameLength {
		return "tags"
	}
	return ""
}

// getOrganizeExamples answers a few songs to show a template on.
func (s *Server) getOrganizeExamples(w http.ResponseWriter, r *http.Request) {
	ask, ok := exampleQuery(r)
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, fmt.Sprintf("count is 1 to %d, and seed a whole number", mostExamples))
		return
	}
	lib, err := s.scanner.OrganizeLibrary(r.Context())
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "organize examples", err)
		}
		return
	}
	body := api.OrganizeExamples{Songs: []api.OrganizeExample{}}
	roots := s.scanner.Roots()
	for _, at := range s.examples(lib.Songs, ask) {
		song := &lib.Songs[at]
		in := s.scanner.TemplateSong(song)
		by := song.AlbumArtist
		if len(song.Artists) > 0 {
			by = song.Artists[0]
		}
		body.Songs = append(body.Songs, api.OrganizeExample{
			Track: song.ULID, Title: song.Title, Artist: by, Album: song.AlbumTitle, From: slash(song.RelPath),
			Root: rootIndex(roots, song.Root), Song: api.OrganizeSong{
				Title: in.Title, Album: in.Album, AlbumArtist: in.AlbumArtist, Artists: append([]string{}, in.Artists...),
				Genre: in.Genre, DiscName: in.DiscName, Format: in.Format, Year: in.Year, OriginalYear: in.OriginalYear,
				Disc: in.Disc, Discs: in.Discs, Track: in.Track, DiscSongs: in.DiscSongs, Compilation: in.Compilation,
			},
		})
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// exampleAsk is what an examples request asks for.
type exampleAsk struct {
	count int
	seed  int64
}

// exampleQuery reads an examples request, and reports whether it asks for
// what the route takes.
func exampleQuery(r *http.Request) (exampleAsk, bool) {
	query := r.URL.Query()
	ask := exampleAsk{count: defaultExamples}
	var err error
	if v := query.Get("count"); v != "" {
		if ask.count, err = strconv.Atoi(v); err != nil || ask.count < 1 || ask.count > mostExamples {
			return ask, false
		}
	}
	if v := query.Get("seed"); v != "" {
		if ask.seed, err = strconv.ParseInt(v, 10, 64); err != nil {
			return ask, false
		}
	}
	return ask, true
}

// examples picks count of songs, by index, from a place seed gives: under the
// saved templates one in place first, one renamed second and one moved third
// where the library has them, then whatever comes.
func (s *Server) examples(songs []store.OrganizeSong, ask exampleAsk) []int {
	n := len(songs)
	if n == 0 {
		return nil
	}
	start := int(uint64(ask.seed) % uint64(n)) //nolint:gosec // G115: a place to start looking, under n.
	want := []organize.Kind{organize.InPlace, organize.Rename, organize.Move}
	first := map[organize.Kind]int{} // The first song of each kind wanted.
	var rest []int
	for i := 0; i < n && (len(first) < len(want) || len(rest) < ask.count); i++ {
		at := (start + i) % n
		kind := s.exampleKind(&songs[at])
		if _, seen := first[kind]; !seen && slices.Contains(want, kind) {
			first[kind] = at
		} else if len(rest) < ask.count {
			rest = append(rest, at)
		}
	}
	out := make([]int, 0, len(want)+len(rest))
	for _, k := range want {
		if at, ok := first[k]; ok {
			out = append(out, at)
		}
	}
	out = append(out, rest...)
	return out[:min(ask.count, len(out))]
}

// exampleKind says what the saved templates would do with song, ignoring
// what may be in its way.
func (s *Server) exampleKind(song *store.OrganizeSong) organize.Kind {
	to, err := s.scanner.OrganizeTarget(song)
	switch {
	case err != nil:
		return organize.Skipped
	case to == slash(song.RelPath):
		return organize.InPlace
	case path.Dir(to) == path.Dir(slash(song.RelPath)):
		return organize.Rename
	}
	return organize.Move
}

// postOrganizeRun starts running a preview's moves.
func (s *Server) postOrganizeRun(w http.ResponseWriter, r *http.Request) {
	var req api.OrganizeRunRequest
	if err := readJSON(w, r, &req); err != nil || req.PreviewID == "" || !fewIDs(req.Tracks, req.Albums) {
		writeProblem(w, r, http.StatusBadRequest, "send JSON naming a preview, and at most some of its songs or albums, not both")
		return
	}
	if !s.scanner.TagWrites() {
		writeProblem(w, r, http.StatusConflict, organizeOffDetail)
		return
	}
	k, ok := s.previews.get(req.PreviewID, s.now())
	if !ok {
		writeProblem(w, r, http.StatusNotFound, "that preview has gone: preview again")
		return
	}
	user, _ := currentUser(r.Context())
	//nolint:contextcheck // A run outlives its request on purpose, as an edit does; see Config.Context.
	run, fn, err := s.scanner.StartOrganize(s.editContext, k.p, pickOf(k.p, &req), user.ID)
	if err != nil {
		s.organizeFailed(w, r, "start organize run", err)
		return
	}
	s.startRun(w, r, run.ID, fn)
}

// startRun hands fn to the server's background work and answers 202 with the
// run id.
func (s *Server) startRun(w http.ResponseWriter, r *http.Request, id string, fn func(context.Context)) {
	if err := s.work(fn); err != nil {
		s.internalError(w, r, "start organize run", err)
		return
	}
	info, err := s.library.OrganizeRunByULID(r.Context(), id)
	if err != nil {
		s.internalError(w, r, "read organize run", err)
		return
	}
	body := s.apiRun(&info)
	writeBody(w, http.StatusAccepted, api.JSONMediaType, &body)
}

// pickOf returns which of p's songs a run of req takes: nil for all.
func pickOf(p *library.OrganizePreview, req *api.OrganizeRunRequest) func(int) bool {
	switch {
	case len(req.Tracks) > 0:
		set := setOfIDs(req.Tracks)
		return func(i int) bool { return set[p.Songs[i].ULID] }
	case len(req.Albums) > 0:
		set := setOfIDs(req.Albums)
		return func(i int) bool { return set[p.Songs[i].Album] }
	}
	return nil
}

// setOfIDs returns ids as a set.
func setOfIDs(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// organizeFailed answers a run or an undo that didn't start.
func (s *Server) organizeFailed(w http.ResponseWriter, r *http.Request, what string, err error) {
	switch {
	case errors.Is(err, library.ErrTagWritesOff):
		writeProblem(w, r, http.StatusConflict, organizeOffDetail)
	case errors.Is(err, library.ErrBusy):
		w.Header().Set("Retry-After", retryAfterSeconds)
		writeProblem(w, r, http.StatusConflict, "a run, a scan or an edit is holding the library")
	case errors.Is(err, library.ErrRootDown):
		w.Header().Set("Retry-After", retryAfterSeconds)
		writeProblem(w, r, http.StatusConflict, "a library folder the run would touch looks down: missing, unreadable or empty")
	case errors.Is(err, library.ErrNothingToUndo):
		writeProblem(w, r, http.StatusConflict, "nothing that run moved is still where it put it, or it is running or an undo")
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "no run has that ID")
	default:
		s.internalError(w, r, what, err)
	}
}

// getOrganizeRuns lists the latest runs.
func (s *Server) getOrganizeRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.library.OrganizeRuns(r.Context(), runsShown)
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "list organize runs", err)
		}
		return
	}
	body := api.OrganizeRuns{Runs: make([]api.OrganizeRun, len(runs))}
	for i := range runs {
		body.Runs[i] = s.apiRun(&runs[i])
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// getOrganizeRun reads one run, with the songs it left.
func (s *Server) getOrganizeRun(w http.ResponseWriter, r *http.Request) {
	info, err := s.library.OrganizeRunByULID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "no run has that ID")
		return
	}
	if err != nil {
		s.internalError(w, r, "read organize run", err)
		return
	}
	left, err := s.library.LeftOrganize(r.Context(), info.ID, mostLeft)
	if err != nil {
		s.internalError(w, r, "read organize run", err)
		return
	}
	roots := s.scanner.Roots()
	body := api.OrganizeRunDetail{Run: s.apiRun(&info), Left: make([]api.OrganizeLeft, len(left))}
	for i := range left {
		l := &left[i]
		body.Left[i] = api.OrganizeLeft{Track: l.ULID, Title: l.Title, From: slash(l.From), To: slash(l.To), Root: rootIndex(roots, l.Root), Reason: l.Reason}
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// postOrganizeRunStop asks a run to stop after its album in hand.
func (s *Server) postOrganizeRunStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	info, err := s.library.OrganizeRunByULID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "no run has that ID")
		return
	}
	if err != nil {
		s.internalError(w, r, "stop organize run", err)
		return
	}
	if err = s.scanner.StopOrganize(id); errors.Is(err, library.ErrNotRunning) {
		writeProblem(w, r, http.StatusConflict, "that run isn't running")
		return
	}
	body := s.apiRun(&info)
	writeBody(w, http.StatusAccepted, api.JSONMediaType, &body)
}

// postOrganizeRunUndo starts an undo of a run.
func (s *Server) postOrganizeRunUndo(w http.ResponseWriter, r *http.Request) {
	if !s.scanner.TagWrites() {
		writeProblem(w, r, http.StatusConflict, organizeOffDetail)
		return
	}
	user, _ := currentUser(r.Context())
	//nolint:contextcheck // As postOrganizeRun.
	run, fn, err := s.scanner.StartUndo(s.editContext, r.PathValue("id"), user.ID)
	if err != nil {
		s.organizeFailed(w, r, "undo organize run", err)
		return
	}
	s.startRun(w, r, run.ID, fn)
}

// apiRun builds the API's view of a run, with a running one's live progress.
func (s *Server) apiRun(info *store.OrganizeRunInfo) api.OrganizeRun {
	out := api.OrganizeRun{
		ID: info.ULID, Kind: info.Kind, State: info.State, StartedAt: info.StartedAt, FinishedAt: info.FinishedAt,
		By: info.By, Template: info.Template, Total: info.Total, Moved: info.Moved, Folders: info.Folders,
		Conflicts: info.Conflicts, Left: info.Left, Failed: info.Failed, Companions: info.Companions,
		Playlists: info.Playlists, UndoOf: info.UndoOf, UndoneBy: info.UndoneBy,
		Undoable: info.Kind != store.OrganizeKindUndo && info.StillThere &&
			(info.State == store.OrganizeDone || info.State == store.OrganizeStopped),
	}
	out.Done = info.Moved + info.Left + info.Failed
	if p := s.scanner.OrganizeProgress(); p != nil && p.Run == info.ULID && info.State == store.OrganizeRunning {
		out.Done, out.Album, out.Stopping = p.Done, p.Album, p.Stopping
	}
	return out
}
