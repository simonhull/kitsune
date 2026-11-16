package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/duplicates"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// duplicatesPage is how many groups a page of the list holds.
	duplicatesPage = 100
	// mostCopies caps the copies a merge names.
	mostCopies = 64
	// noGroupDetail is what a key no group has now is told.
	noGroupDetail = "no group of duplicates has that key now: the library may have changed"
)

// found is a find of duplicates with what the list reads beside it: which
// groups and pairs were dismissed, and everyone's plays of every grouped song.
type found struct {
	d         *library.Duplicates
	dismissed map[string][]string
	plays     map[int64]int
}

// find finds the library's duplicates, with what was dismissed and the plays.
func (s *Server) find(ctx context.Context) (*found, error) {
	d, err := s.scanner.FindDuplicates(ctx)
	if err != nil {
		return nil, err //nolint:wrapcheck // The scanner's error says what failed.
	}
	dismissed, err := s.library.DuplicateDismissals(ctx)
	if err != nil {
		return nil, err //nolint:wrapcheck // As above.
	}
	ids := make([]int64, 0, d.Result.Songs)
	for k := range d.Groups {
		for _, i := range d.Groups[k].Songs {
			ids = append(ids, d.Songs[i].ID)
		}
	}
	plays, err := s.library.PlayTotals(ctx, ids)
	if err != nil {
		return nil, err //nolint:wrapcheck // As above.
	}
	return &found{d: d, dismissed: dismissed, plays: plays}, nil
}

// findOrFail finds the library's duplicates, or answers why it couldn't and
// returns nil.
func (s *Server) findOrFail(w http.ResponseWriter, r *http.Request) *found {
	f, err := s.find(r.Context())
	if err != nil && r.Context().Err() == nil {
		s.internalError(w, r, "find duplicates", err)
	}
	return f
}

// pairDismissed reports whether pair k was said not to be duplicates.
func (f *found) pairDismissed(k int) bool {
	_, ok := f.dismissed[f.d.Pairs[k].Key]
	return ok
}

// groupDismissed reports whether group k was, or its pair was.
func (f *found) groupDismissed(k int) bool {
	g := &f.d.Groups[k]
	if _, ok := f.dismissed[g.Key]; ok {
		return true
	}
	return g.Pair >= 0 && f.pairDismissed(g.Pair)
}

// groupPlays returns everyone's plays of group k's songs.
func (f *found) groupPlays(k int) int {
	n := 0
	for _, i := range f.d.Groups[k].Songs {
		n += f.plays[f.d.Songs[i].ID]
	}
	return n
}

// listAsk is what a request for the list asks for.
type listAsk struct {
	filter string
	at     int // The first group of the page.
}

// listQuery reads a request for the list, and reports whether it asks for
// what the route takes.
func listQuery(r *http.Request) (listAsk, bool) {
	query := r.URL.Query()
	ask := listAsk{filter: cmp.Or(query.Get("filter"), api.DuplicatesAll)}
	if c := query.Get("cursor"); c != "" {
		n, err := strconv.Atoi(c)
		if err != nil || n < 0 {
			return ask, false
		}
		ask.at = n
	}
	return ask, slices.Contains([]string{
		api.DuplicatesAll, api.DuplicatesAudio, api.DuplicatesRecording, api.DuplicatesProbably,
		api.DuplicatesPairs, api.DuplicatesDismissed,
	}, ask.filter)
}

// getDuplicates answers the list: its counts and one page of it.
func (s *Server) getDuplicates(w http.ResponseWriter, r *http.Request) {
	ask, ok := listQuery(r)
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, "filter is all, audio, recording, probably, pairs or dismissed, and cursor one a page gave")
		return
	}
	f := s.findOrFail(w, r)
	if f == nil {
		return
	}
	list := f.shown(ask.filter)
	body := api.DuplicateList{
		Counts: f.counts(), Filter: ask.filter, Matching: len(list.pairs) + len(list.groups),
		Pairs: []api.DuplicatePairSummary{}, Groups: []api.DuplicateGroupSummary{},
	}
	if ask.at == 0 {
		for _, k := range list.pairs {
			body.Pairs = append(body.Pairs, f.pairSummary(k))
		}
	}
	end := min(ask.at+duplicatesPage, len(list.groups))
	for _, k := range list.groups[min(ask.at, end):end] {
		body.Groups = append(body.Groups, f.groupSummary(k))
	}
	if end < len(list.groups) {
		body.NextCursor = strconv.Itoa(end)
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// counts are the find's totals.
func (f *found) counts() api.DuplicateCounts {
	c := api.DuplicateCounts{Library: len(f.d.Songs), Checked: f.d.Checked}
	for k := range f.d.Pairs {
		if f.pairDismissed(k) {
			c.Dismissed++
		} else {
			c.Pairs++
		}
	}
	for k := range f.d.Groups {
		g := &f.d.Groups[k]
		if f.groupDismissed(k) {
			if g.Pair < 0 || !f.pairDismissed(g.Pair) {
				c.Dismissed++
			}
			continue
		}
		c.Groups++
		c.Songs += len(g.Songs)
		switch g.Confidence {
		case duplicates.SameAudio:
			c.Audio++
		case duplicates.SameRecording:
			c.Recording++
		case duplicates.SameSong:
			c.Probably++
		}
	}
	return c
}

// shownList is what a filter shows: pairs and groups, by index.
type shownList struct {
	pairs, groups []int
}

// shown returns the pairs and groups filter shows, in the list's order:
// confidence first, then most copies, then most plays.
func (f *found) shown(filter string) shownList {
	var out shownList
	for k := range f.d.Pairs {
		if f.pairShows(k, filter) {
			out.pairs = append(out.pairs, k)
		}
	}
	for k := range f.d.Groups {
		if f.groupShows(k, filter) {
			out.groups = append(out.groups, k)
		}
	}
	slices.SortStableFunc(out.groups, func(a, b int) int {
		ga, gb := &f.d.Groups[a], &f.d.Groups[b]
		return cmp.Or(
			cmp.Compare(gb.Confidence, ga.Confidence),
			cmp.Compare(len(gb.Songs), len(ga.Songs)),
			cmp.Compare(f.groupPlays(b), f.groupPlays(a)),
		)
	})
	return out
}

// pairShows reports whether pair k shows under filter.
func (f *found) pairShows(k int, filter string) bool {
	switch filter {
	case api.DuplicatesAll, api.DuplicatesPairs:
		return !f.pairDismissed(k)
	case api.DuplicatesDismissed:
		return f.pairDismissed(k)
	}
	return false
}

// groupShows reports whether group k shows under filter. Under all, a pair's
// groups are in its card; a dismissed pair lists alone.
func (f *found) groupShows(k int, filter string) bool {
	g := &f.d.Groups[k]
	dismissed := f.groupDismissed(k)
	switch filter {
	case api.DuplicatesAll:
		return !dismissed && g.Pair < 0
	case api.DuplicatesDismissed:
		return dismissed && (g.Pair < 0 || !f.pairDismissed(g.Pair))
	case api.DuplicatesAudio, api.DuplicatesRecording, api.DuplicatesProbably:
		return !dismissed && g.Confidence.String() == filter
	}
	return false
}

// albumRef is the album of row as the API names it.
func (f *found) albumRef(row int64) api.DuplicateAlbum {
	a := f.d.Albums[row]
	if a == nil {
		return api.DuplicateAlbum{}
	}
	return api.DuplicateAlbum{ID: a.ULID, Title: a.Title, Artist: a.Artist, Year: a.Year, Artwork: toAPIArtwork(a.Cover)}
}

// albumRow returns the row of the album with the public ID id, or 0.
func (f *found) albumRow(id string) int64 {
	if a, ok := f.d.Album(id); ok {
		return a.ID
	}
	return 0
}

// groupSummary is group k as the list shows it.
func (f *found) groupSummary(k int) api.DuplicateGroupSummary {
	g := &f.d.Groups[k]
	first := &f.d.Songs[g.Songs[0]]
	out := api.DuplicateGroupSummary{
		Key: g.Key, Title: first.Title, Artist: first.Artist, Confidence: g.Confidence.String(), Copies: len(g.Songs),
		Formats: []string{}, Albums: []api.DuplicateAlbum{}, DurationMs: []int64{}, Plays: f.groupPlays(k),
		Dismissed: f.groupDismissed(k),
	}
	if g.Pair >= 0 {
		out.Pair = f.d.Pairs[g.Pair].Key
	}
	var seen []int64
	for _, i := range g.Songs {
		s := &f.d.Songs[i]
		out.Formats = append(out.Formats, s.Format)
		out.DurationMs = append(out.DurationMs, s.Duration.Milliseconds())
		if !slices.Contains(seen, s.AlbumID) {
			seen = append(seen, s.AlbumID)
			out.Albums = append(out.Albums, f.albumRef(s.AlbumID))
		}
	}
	return out
}

// pairSummary is pair k as the list shows it.
func (f *found) pairSummary(k int) api.DuplicatePairSummary {
	p := &f.d.Pairs[k]
	out := api.DuplicatePairSummary{
		Key: p.Key, Both: len(p.Groups), Only: [2]int{len(p.Only[0]), len(p.Only[1])}, Dismissed: f.pairDismissed(k),
	}
	for side, id := range p.Albums {
		out.Albums[side] = f.albumRef(f.albumRow(id))
	}
	return out
}

// getDuplicate answers one group or album pair, its copies side by side.
func (s *Server) getDuplicate(w http.ResponseWriter, r *http.Request) {
	f := s.findOrFail(w, r)
	if f == nil {
		return
	}
	key := r.PathValue("group")
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	var body api.DuplicateDetail
	var err error
	if g, isGroup := f.d.Group(key); isGroup {
		body.Kind = api.DuplicateKindGroup
		body.Group, err = s.groupDetail(r.Context(), f, g, user.ID)
	} else if k := slices.IndexFunc(f.d.Pairs, func(p duplicates.AlbumPair) bool { return p.Key == key }); k >= 0 {
		body.Kind = api.DuplicateKindPair
		body.Pair, err = s.pairDetail(r.Context(), f, k, user.ID)
	} else {
		writeProblem(w, r, http.StatusNotFound, noGroupDetail)
		return
	}
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "read duplicates", err)
		}
		return
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// copies are the live copies of some songs, as a page compares them.
type copies struct {
	of      map[int64]store.DuplicateCopy
	inPlace map[int64]bool
}

// readCopies reads the songs at idx of f's find for the user.
func (s *Server) readCopies(ctx context.Context, f *found, idx []int, userID int64) (*copies, error) {
	ids := make([]int64, len(idx))
	for n, i := range idx {
		ids[n] = f.d.Songs[i].ID
	}
	of, err := s.library.DuplicateCopies(ctx, userID, ids)
	if err != nil {
		return nil, err //nolint:wrapcheck // The store's error says what failed.
	}
	inPlace, err := s.scanner.InPlace(ctx, ids)
	if err != nil {
		return nil, err //nolint:wrapcheck // As above.
	}
	return &copies{of: of, inPlace: inPlace}, nil
}

// apiCopy is song i of f's find as the API compares it, or nil when it isn't
// live.
func (s *Server) apiCopy(f *found, c *copies, i int) *api.DuplicateCopy {
	song := &f.d.Songs[i]
	cp, ok := c.of[song.ID]
	if !ok {
		return nil
	}
	return &api.DuplicateCopy{
		Track: cp.ULID, Title: cp.Title, Album: f.albumRef(song.AlbumID), Disc: cp.Disc, TrackNo: cp.Track,
		Path: slash(cp.RelPath), Root: rootIndex(s.scanner.Roots(), cp.Root), Format: cp.Format, Bitrate: cp.Bitrate,
		SampleRate: cp.SampleRate, BitDepth: cp.BitDepth, Lossless: cp.Lossless, DurationMs: cp.Duration.Milliseconds(),
		Plays: cp.Plays, Stars: cp.Stars.Stars(), Raters: cp.Raters, Playlists: cp.Playlists, AddedAt: cp.AddedAt,
		InPlace: c.inPlace[song.ID], Original: f.d.Original[i],
	}
}

// keeperCopy is copy c as the keeper is chosen.
func keeperCopy(c *api.DuplicateCopy) duplicates.Copy {
	return duplicates.Copy{
		Lossless: c.Lossless, Original: c.Original, Bitrate: c.Bitrate, InPlace: c.InPlace, Plays: c.Plays,
		AddedAt: c.AddedAt,
	}
}

// mergeSong is song i of f's find as a merge names it.
func mergeSong(f *found, i int) store.MergeSong {
	return store.MergeSong{ID: f.d.Songs[i].ID, ULID: f.d.Songs[i].ULID}
}

// groupDetail is group g, with each copy's facts, the suggested keeper and what
// keeping each copy would move, for the user.
func (s *Server) groupDetail(ctx context.Context, f *found, g *duplicates.Group, userID int64) (*api.DuplicateGroup, error) {
	first := &f.d.Songs[g.Songs[0]]
	out := &api.DuplicateGroup{
		Key: g.Key, Title: first.Title, Artist: first.Artist, Confidence: g.Confidence.String(),
		Reasons: []string{}, Copies: []api.DuplicateCopy{}, Plans: []api.KeepPlan{},
	}
	_, out.Dismissed = f.dismissed[g.Key]
	if g.Pair >= 0 {
		out.Pair = f.d.Pairs[g.Pair].Key
		out.Dismissed = out.Dismissed || f.pairDismissed(g.Pair)
	}
	live, err := s.groupCopies(ctx, f, g, userID, out)
	if err != nil {
		return nil, err
	}
	for n := range live {
		m := store.Merge{Keep: mergeSong(f, live[n])}
		for o := range live {
			if o != n {
				m.Trash = append(m.Trash, mergeSong(f, live[o]))
			}
		}
		plan, planErr := s.library.PlanMerge(ctx, userID, []store.Merge{m})
		if planErr != nil {
			return nil, planErr //nolint:wrapcheck // The store's error says what failed.
		}
		out.Plans = append(out.Plans, api.KeepPlan{Keep: m.Keep.ULID, Plan: toAPIPlan(&plan)})
	}
	return out, nil
}

// groupCopies fills out's copies of g, its keeper and why, and returns the
// live copies by index into f's songs.
func (s *Server) groupCopies(ctx context.Context, f *found, g *duplicates.Group, userID int64, out *api.DuplicateGroup) ([]int, error) {
	c, err := s.readCopies(ctx, f, g.Songs, userID)
	if err != nil {
		return nil, err
	}
	var live []int
	choose := make([]duplicates.Copy, 0, len(g.Songs))
	for _, i := range g.Songs {
		if cp := s.apiCopy(f, c, i); cp != nil {
			out.Copies = append(out.Copies, *cp)
			live = append(live, i)
			choose = append(choose, keeperCopy(cp))
		}
	}
	keep, why := duplicates.Keeper(choose)
	if keep >= 0 {
		out.Keeper = out.Copies[keep].Track
	}
	for _, r := range why {
		out.Reasons = append(out.Reasons, string(r))
	}
	return live, nil
}

// pairDetail is album pair k, its track lists matched row by row, with the
// suggested album to keep and what keeping each would move, for the user.
func (s *Server) pairDetail(ctx context.Context, f *found, k int, userID int64) (*api.DuplicatePair, error) {
	p := &f.d.Pairs[k]
	var idx []int
	for _, g := range p.Groups {
		idx = append(idx, f.d.Groups[g].Songs...)
	}
	idx = slices.Concat(idx, p.Only[0], p.Only[1])
	c, err := s.readCopies(ctx, f, idx, userID)
	if err != nil {
		return nil, err
	}
	out := &api.DuplicatePair{Key: p.Key, Dismissed: f.pairDismissed(k), Rows: []api.DuplicatePairRow{}, Plans: []api.KeepPlan{}}
	for side, id := range p.Albums {
		out.Albums[side] = f.albumRef(f.albumRow(id))
	}
	rows := s.pairRows(f, c, p)
	out.Keeper = p.Albums[pairKeeper(rows, p)]
	out.Rows = append(out.Rows, rows...)
	for side := range p.Albums {
		plan, planErr := s.library.PlanMerge(ctx, userID, pairMerges(f, p, side))
		if planErr != nil {
			return nil, planErr //nolint:wrapcheck // The store's error says what failed.
		}
		out.Plans = append(out.Plans, api.KeepPlan{Keep: p.Albums[side], Plan: toAPIPlan(&plan)})
	}
	return out, nil
}

// pairKeeper returns the side of p to keep: the album whose copies the
// keeper's order picks more often, and on a tie the one with more songs of its
// own, so a deluxe edition keeps over the plain one.
func pairKeeper(rows []api.DuplicatePairRow, p *duplicates.AlbumPair) int {
	wins := [2]int{}
	for i := range rows {
		a, b := rows[i].Copies[0], rows[i].Copies[1]
		if a == nil || b == nil {
			continue
		}
		keep, _ := duplicates.Keeper([]duplicates.Copy{keeperCopy(a), keeperCopy(b)})
		wins[keep]++
	}
	if wins[1] > wins[0] || (wins[1] == wins[0] && len(p.Only[1]) > len(p.Only[0])) {
		return 1
	}
	return 0
}

// pairRows matches p's track lists row by row: the first album's songs in
// its order, each beside its copy on the other, then the songs only the other
// has.
func (s *Server) pairRows(f *found, c *copies, p *duplicates.AlbumPair) []api.DuplicatePairRow {
	match := pairMatches(f, p)
	first := f.albumRow(p.Albums[0])
	var out []api.DuplicatePairRow
	for i := range f.d.Songs {
		if f.d.Songs[i].AlbumID != first {
			continue
		}
		var row api.DuplicatePairRow
		row.Copies[0] = s.apiCopy(f, c, i)
		if m, ok := match[i]; ok {
			row.Group, row.Copies[1] = m.group, s.apiCopy(f, c, m.other)
		}
		if row.Copies[0] != nil || row.Copies[1] != nil {
			out = append(out, row)
		}
	}
	for _, i := range p.Only[1] {
		var row api.DuplicatePairRow
		if row.Copies[1] = s.apiCopy(f, c, i); row.Copies[1] != nil {
			out = append(out, row)
		}
	}
	return out
}

// pairMatch is a song of a pair's first album matched to its copy on the
// second, by index into f's songs, in the group it names.
type pairMatch struct {
	other int
	group string
}

// pairMatches returns each of p's first album's songs that a group of p pairs
// with a song of its second, by index into f's songs.
func pairMatches(f *found, p *duplicates.AlbumPair) map[int]pairMatch {
	rows := [2]int64{f.albumRow(p.Albums[0]), f.albumRow(p.Albums[1])}
	out := map[int]pairMatch{}
	for _, k := range p.Groups {
		g := &f.d.Groups[k]
		at := [2]int{-1, -1}
		for _, i := range g.Songs {
			if side := slices.Index(rows[:], f.d.Songs[i].AlbumID); side >= 0 && at[side] < 0 {
				at[side] = i
			}
		}
		if at[0] >= 0 && at[1] >= 0 {
			out[at[0]] = pairMatch{other: at[1], group: g.Key}
		}
	}
	return out
}

// pairMerges are the merges that keep p's album at side: for each of its
// groups, the copy on that album keeps, and the copies on the other go.
func pairMerges(f *found, p *duplicates.AlbumPair, side int) []store.Merge {
	keepRow, goneRow := f.albumRow(p.Albums[side]), f.albumRow(p.Albums[1-side])
	var out []store.Merge
	for _, k := range p.Groups {
		m := store.Merge{Keep: store.MergeSong{ID: -1}}
		for _, i := range f.d.Groups[k].Songs {
			switch row := f.d.Songs[i].AlbumID; {
			case row == keepRow && m.Keep.ID < 0:
				m.Keep = mergeSong(f, i)
			case row == goneRow:
				m.Trash = append(m.Trash, mergeSong(f, i))
			}
		}
		if m.Keep.ID >= 0 && len(m.Trash) > 0 {
			out = append(out, m)
		}
	}
	return out
}

// toAPIPlan is plan as the API words it.
func toAPIPlan(p *store.MergePlan) api.MergePlan {
	out := api.MergePlan{
		Plays: p.Plays, Listens: p.Listens, Loves: p.Loves, Hearts: p.Hearts, Entries: p.Entries,
		Playlists: []api.MergePlanPlaylist{}, OtherPlaylists: p.OtherPlaylists, Excluded: p.Excluded, Runs: p.Runs,
		Queues: p.Queues, Stars: api.MergeStars{Come: p.Stars.Come, Stay: p.Stars.Stay},
	}
	if y := p.Stars.Yours; y != nil {
		out.Stars.Yours = &api.YourStars{Keeper: y.Keeper.Stars(), Other: y.Other.Stars(), After: y.After.Stars()}
	}
	for _, pl := range p.Playlists {
		out.Playlists = append(out.Playlists, api.MergePlanPlaylist{ID: pl.ID, Name: pl.Name, File: pl.File})
	}
	return out
}

// fromAPIPlan is the plan a client sent back, or false when it can't be one.
func fromAPIPlan(p *api.MergePlan) (store.MergePlan, bool) {
	out := store.MergePlan{
		Plays: p.Plays, Listens: p.Listens, Loves: p.Loves, Hearts: p.Hearts, Entries: p.Entries,
		OtherPlaylists: p.OtherPlaylists, Excluded: p.Excluded, Runs: p.Runs, Queues: p.Queues,
		Stars: store.StarsPlan{Come: p.Stars.Come, Stay: p.Stars.Stay},
	}
	if y := p.Stars.Yours; y != nil {
		var stars [3]rating.Rating
		for n, v := range []float64{y.Keeper, y.Other, y.After} {
			r, err := rating.FromStars(v)
			if v != 0 && err != nil {
				return out, false
			}
			stars[n] = r
		}
		out.Stars.Yours = &store.YourStars{Keeper: stars[0], Other: stars[1], After: stars[2]}
	}
	for _, pl := range p.Playlists {
		out.Playlists = append(out.Playlists, store.PlanPlaylist{ID: pl.ID, Name: pl.Name, File: pl.File})
	}
	return out, true
}

// readMerge reads a merge's body and the plan it sends back. It writes the
// problem and reports false when it can't go on.
func readMerge(w http.ResponseWriter, r *http.Request) (api.MergeRequest, store.MergePlan, bool) {
	var req api.MergeRequest
	if err := readJSON(w, r, &req); err != nil || req.Keep == "" || len(req.Trash) == 0 || len(req.Trash) > mostCopies {
		writeProblem(w, r, http.StatusBadRequest, fmt.Sprintf("send JSON naming what to keep, from 1 to %d to trash, and the plan the confirm showed", mostCopies))
		return req, store.MergePlan{}, false
	}
	expect, ok := fromAPIPlan(&req.Plan)
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, "the plan's stars are ½ to 5 in halves, or 0 for none")
	}
	return req, expect, ok
}

// postDuplicateMerge keeps one copy, or one album of a pair, merges the others
// into it and trashes them.
func (s *Server) postDuplicateMerge(w http.ResponseWriter, r *http.Request) {
	req, expect, ok := readMerge(w, r)
	if !ok {
		return
	}
	if !s.scanner.TagWrites() {
		writeProblem(w, r, http.StatusConflict, trashOffDetail)
		return
	}
	f := s.findOrFail(w, r)
	if f == nil {
		return
	}
	merges, refused := mergesOf(f, r.PathValue("group"), &req)
	if refused.status != 0 {
		writeProblem(w, r, refused.status, refused.detail)
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	//nolint:contextcheck // A merge outlives its request on purpose, as a trash does; see Config.Context.
	res, err := s.scanner.MergeDuplicates(s.editContext, user.ID, merges, &expect)
	if err != nil {
		s.mergeFailed(w, r, err)
		return
	}
	s.publishMerge(&res.MergeChange)
	body := api.Merged{Kept: []string{}, Trashed: api.TrashChange{Tracks: res.Trashed.Tracks, Albums: res.Trashed.Albums}}
	for i := range merges {
		body.Kept = append(body.Kept, merges[i].Keep.ULID)
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// mergeFailed answers a merge that didn't happen: with the plan as it is now
// when that changed, else as a trash that didn't.
func (s *Server) mergeFailed(w http.ResponseWriter, r *http.Request, err error) {
	changed, ok := errors.AsType[*store.PlanChangedError](err)
	if !ok {
		s.trashFailed(w, r, "merge duplicates", err)
		return
	}
	body := api.PlanChangedProblem{
		Problem: newProblem(r, http.StatusConflict, "the library changed since the confirm: here is what the merge would move now"),
		Plan:    toAPIPlan(&changed.Plan),
	}
	writeBody(w, http.StatusConflict, api.ProblemMediaType, &body)
}

// refusal is why a request can't be done: its status and detail, or a zero
// status when it can.
type refusal struct {
	status int
	detail string
}

// mergesOf returns the merges req asks of the group or pair key, or why it
// can't be.
func mergesOf(f *found, key string, req *api.MergeRequest) ([]store.Merge, refusal) {
	if g, ok := f.d.Group(key); ok {
		return groupMerge(f, g, req)
	}
	if p, ok := f.d.Pair(key); ok {
		side := slices.Index(p.Albums[:], req.Keep)
		if side < 0 || len(req.Trash) != 1 || req.Trash[0] != p.Albums[1-side] {
			return nil, refusal{http.StatusUnprocessableEntity, "keep one album of the pair and trash the other"}
		}
		return pairMerges(f, p, side), refusal{}
	}
	return nil, refusal{http.StatusNotFound, noGroupDetail}
}

// groupMerge returns the merge req asks of group g, or why it can't be.
func groupMerge(f *found, g *duplicates.Group, req *api.MergeRequest) ([]store.Merge, refusal) {
	in := map[string]int{}
	for _, i := range g.Songs {
		in[f.d.Songs[i].ULID] = i
	}
	keep, ok := in[req.Keep]
	if !ok {
		return nil, refusal{http.StatusUnprocessableEntity, "keep one of the group's copies"}
	}
	m := store.Merge{Keep: mergeSong(f, keep)}
	for _, id := range req.Trash {
		i, isCopy := in[id]
		if !isCopy || id == req.Keep || slices.ContainsFunc(m.Trash, func(t store.MergeSong) bool { return t.ULID == id }) {
			return nil, refusal{http.StatusUnprocessableEntity, "trash other copies of the group, each once, never the one kept"}
		}
		m.Trash = append(m.Trash, mergeSong(f, i))
	}
	return []store.Merge{m}, refusal{}
}

// publishMerge tells people what a merge changed: their stars, their
// playlists, their shuffle rules and their listening. The scanner tells
// everyone which albums.
func (s *Server) publishMerge(c *store.MergeChange) {
	for _, m := range c.Marks {
		s.publishMarks(m.User, &api.EventMarks{Kind: trackKind, ID: m.Track, Stars: m.Stars.Stars()})
	}
	if c.Loves {
		s.lastfm.Poke()
	}
	for owner, ids := range c.Playlists {
		for _, id := range ids {
			s.publishPlaylist(owner, id)
		}
	}
	if len(c.Files) > 0 {
		s.publish(events.Playlists, events.Everyone(), &api.EventPlaylists{IDs: c.Files})
	}
	for _, u := range c.Shufflers {
		s.publishShuffleRules(u)
	}
	for _, u := range c.Listeners {
		s.shuffles.Forget(u)
	}
}

// postDuplicateDismiss says a group or album pair is not duplicates.
func (s *Server) postDuplicateDismiss(w http.ResponseWriter, r *http.Request) {
	f := s.findOrFail(w, r)
	if f == nil {
		return
	}
	key := r.PathValue("group")
	var ids []string
	if g, isGroup := f.d.Group(key); isGroup {
		for _, i := range g.Songs {
			ids = append(ids, f.d.Songs[i].ULID)
		}
	} else if p, isPair := f.d.Pair(key); isPair {
		ids = p.Albums[:]
	} else {
		writeProblem(w, r, http.StatusNotFound, noGroupDetail)
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	if err := s.library.DismissDuplicates(r.Context(), key, ids, user.ID); err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "dismiss duplicates", err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteDuplicateDismiss brings a dismissed group or pair back to the list.
func (s *Server) deleteDuplicateDismiss(w http.ResponseWriter, r *http.Request) {
	err := s.library.BringBackDuplicates(r.Context(), r.PathValue("group"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "that group wasn't dismissed")
	case err != nil:
		if r.Context().Err() == nil {
			s.internalError(w, r, "bring back duplicates", err)
		}
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
