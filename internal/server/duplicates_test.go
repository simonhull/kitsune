package server

import (
	"encoding/json/v2"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	duplicatesPath = "/admin/duplicates"
	copyOne        = "dupes/one/x.flac"
	copyTwo        = "dupes/two/y.flac"
	noGroup        = "000000000000000000000000"
)

// dupesFixture is a server with tag writes on over two copies of one FLAC,
// each in a folder of its own: one group of duplicates.
func dupesFixture(t *testing.T, tagWrites bool) *editFixture {
	t.Helper()
	return newEditServer(t, &testServerOptions{TagWrites: tagWrites}, map[string]string{copyOne: sampleFLAC, copyTwo: sampleFLAC})
}

// list reads the list under query as the fixture's administrator.
func (fx *editFixture) list(t *testing.T, query string) api.DuplicateList {
	t.Helper()
	var l api.DuplicateList
	fx.getJSON(t, fx.token, duplicatesPath+query, &l)
	return l
}

// group reads the one group there is.
func (fx *editFixture) group(t *testing.T) api.DuplicateGroup {
	t.Helper()
	l := fx.list(t, "")
	if len(l.Groups) != 1 {
		t.Fatalf("list = %+v, want one group", l)
	}
	var d api.DuplicateDetail
	fx.getJSON(t, fx.token, duplicatesPath+"/"+l.Groups[0].Key, &d)
	if d.Kind != api.DuplicateKindGroup || d.Group == nil || len(d.Group.Copies) != 2 || len(d.Group.Plans) != 2 {
		t.Fatalf("detail = %+v, want a group of two copies with a plan for each", d)
	}
	return *d.Group
}

// merge posts a merge of g keeping keep, with what keeping it would move, and
// returns the response.
func (fx *editFixture) merge(t *testing.T, g *api.DuplicateGroup, keep string, plan *api.MergePlan) response {
	t.Helper()
	var trash []string
	for i := range g.Copies {
		if c := g.Copies[i].Track; c != keep {
			trash = append(trash, c)
		}
	}
	body, err := json.Marshal(api.MergeRequest{Keep: keep, Trash: trash, Plan: *plan})
	if err != nil {
		t.Fatal(err)
	}
	return fx.send(t, fx.organizeCall(duplicatesPath+"/"+g.Key+"/merge", string(body)))
}

// planOf returns the plan of keeping keep.
func planOf(t *testing.T, g *api.DuplicateGroup, keep string) api.MergePlan {
	t.Helper()
	for i := range g.Plans {
		if g.Plans[i].Keep == keep {
			return g.Plans[i].Plan
		}
	}
	t.Fatalf("no plan keeps %s", keep)
	return api.MergePlan{}
}

// The two copies are one group, counted and listed under all and their
// confidence, never under another.
func TestDuplicatesList(t *testing.T) {
	t.Parallel()
	fx := dupesFixture(t, true)
	l := fx.list(t, "")
	if c := l.Counts; c.Groups != 1 || c.Songs != 2 || c.Library != 2 || c.Pairs != 0 {
		t.Errorf("counts = %+v, want one group of two in a library of two", c)
	}
	g := l.Groups[0]
	if g.Copies != 2 || len(g.Albums) != 2 {
		t.Errorf("summary = %+v, want two copies on two albums", g)
	}
	if other := fx.list(t, "?filter=pairs"); len(other.Groups) != 0 || other.Matching != 0 {
		t.Errorf("pairs = %+v, want none", other)
	}
	if mine := fx.list(t, "?filter="+g.Confidence); len(mine.Groups) != 1 {
		t.Errorf("filter %s = %+v, want the group", g.Confidence, mine)
	}
}

// A group's page compares its copies and suggests a keeper, and holds no path
// with its root.
func TestDuplicatesGroup(t *testing.T) {
	t.Parallel()
	fx := dupesFixture(t, true)
	d := fx.group(t)
	if d.Keeper == "" || d.Copies[0].Path == "" || d.Copies[0].Format != "FLAC" {
		t.Errorf("group = %+v, want a keeper and each copy's facts", d)
	}
	raw := fx.send(t, call{method: http.MethodGet, path: duplicatesPath + "/" + d.Key, header: bearer(fx.token)})
	if strings.Contains(string(raw.Body), fx.root) {
		t.Errorf("the group holds the root: %s", raw.Body)
	}
}

// No response holds another person's own rating: an administrator sees a
// count of raters and their own stars.
func TestDuplicatesNeverShowAnotherPersonsStars(t *testing.T) {
	t.Parallel()
	fx := dupesFixture(t, true)
	ana := fx.tokenFor(t, store.RoleUser)
	g := fx.group(t)
	fx.send(t, call{method: http.MethodPut, path: "/me/ratings/tracks/" + g.Copies[1].Track, body: `{"stars":4.5}`, header: jsonHeader(ana)})
	g = fx.group(t)
	if c := g.Copies[1]; c.Raters != 1 || c.Stars != 0 {
		t.Errorf("copy = %+v, want one rater and none of the admin's own stars", c)
	}
	raw := fx.send(t, call{method: http.MethodGet, path: duplicatesPath + "/" + g.Key, header: bearer(fx.token)})
	if strings.Contains(string(raw.Body), "4.5") {
		t.Errorf("the group holds Ana's 4½★: %s", raw.Body)
	}
	if p := planOf(t, &g, g.Copies[0].Track); p.Stars.Come != 1 || p.Stars.Yours != nil {
		t.Errorf("plan = %+v, want Ana's stars counted as coming over, and none of the admin's", p.Stars)
	}
}

// A merge whose plan changed since the confirm is a 409 with the new plan.
func TestDuplicatesMergeAfterAPlay(t *testing.T) {
	t.Parallel()
	fx := dupesFixture(t, true)
	ana := fx.tokenFor(t, store.RoleUser)
	g := fx.group(t)
	keep, gone := g.Copies[0].Track, g.Copies[1].Track
	stale := planOf(t, &g, keep)
	fx.send(t, call{
		method: http.MethodPost, path: playsPath, header: jsonHeader(ana),
		body: `{"trackId":"` + gone + `","playedAt":"2026-01-01T00:00:00Z","source":"web","clientId":"0123456789abcdef"}`,
	})
	resp := fx.merge(t, &g, keep, &stale)
	var changed api.PlanChangedProblem
	if err := json.Unmarshal(resp.Body, &changed); err != nil || resp.Status != http.StatusConflict || changed.Plan.Plays != stale.Plays+1 {
		t.Fatalf("a merge after a play = %d %s, want a 409 with the play in its plan", resp.Status, resp.Body)
	}
}

// A merge moves Ana's stars to the keeper and trashes the other copy; she
// hears her stars change, and the group leaves the list.
func TestDuplicatesMerge(t *testing.T) {
	t.Parallel()
	fx := dupesFixture(t, true)
	ana := fx.tokenFor(t, store.RoleUser)
	g := fx.group(t)
	keep, gone := g.Copies[0].Track, g.Copies[1].Track
	fx.send(t, call{method: http.MethodPut, path: "/me/ratings/tracks/" + gone, body: `{"stars":4.5}`, header: jsonHeader(ana)})
	g = fx.group(t)
	stream := fx.openStream(t, ana, "")
	stream.want(events.Ready)
	resp := fx.merge(t, &g, keep, new(planOf(t, &g, keep)))
	var merged api.Merged
	resp.decode(t, &merged)
	if resp.Status != http.StatusOK || !slices.Equal(merged.Trashed.Tracks, []string{gone}) || !slices.Equal(merged.Kept, []string{keep}) {
		t.Fatalf("merge = %d %+v, want the other copy trashed", resp.Status, merged)
	}
	var marks api.EventMarks
	decodeEvent(t, stream.until(events.Marks), &marks)
	if marks.ID != keep || marks.Stars != 4.5 {
		t.Errorf("Ana's marks event = %+v, want her 4½★ on the keeper", marks)
	}
	if l := fx.list(t, ""); l.Counts.Groups != 0 {
		t.Errorf("after the merge: %+v, want no group", l.Counts)
	}
	var trash api.Trash
	fx.getJSON(t, fx.token, "/admin/trash", &trash)
	if trash.Count != 1 {
		t.Errorf("trash = %d songs, want the copy", trash.Count)
	}
}

// Not duplicates hides a group for good, and lists it under dismissed; Bring
// back puts it back.
func TestDuplicatesDismissAndBringBack(t *testing.T) {
	t.Parallel()
	fx := dupesFixture(t, true)
	key := fx.list(t, "").Groups[0].Key
	path := duplicatesPath + "/" + key + "/dismiss"
	if resp := fx.send(t, fx.organizeCall(path, "")); resp.Status != http.StatusNoContent {
		t.Fatalf("dismiss = %d: %s", resp.Status, resp.Body)
	}
	l := fx.list(t, "")
	if l.Counts.Groups != 0 || l.Counts.Dismissed != 1 || len(l.Groups) != 0 {
		t.Errorf("after dismissing: %+v, want it counted as dismissed only", l.Counts)
	}
	if d := fx.list(t, "?filter=dismissed"); len(d.Groups) != 1 || !d.Groups[0].Dismissed {
		t.Errorf("dismissed = %+v, want the group", d.Groups)
	}
	back := call{method: http.MethodDelete, path: path, header: bearer(fx.token)}
	if resp := fx.send(t, back); resp.Status != http.StatusNoContent {
		t.Fatalf("bring back = %d: %s", resp.Status, resp.Body)
	}
	if again := fx.list(t, ""); again.Counts.Groups != 1 {
		t.Errorf("after bringing back: %+v, want the group", again.Counts)
	}
	fx.send(t, back).problem(t, http.StatusNotFound)
}

// Every refusal: bad requests, keys no group has, copies not in the group, tag
// writes off, and callers who may not.
func TestDuplicatesRefusals(t *testing.T) {
	t.Parallel()
	fx := dupesFixture(t, true)
	user := fx.tokenFor(t, store.RoleUser)
	g := fx.group(t)
	plan, err := json.Marshal(planOf(t, &g, g.Copies[0].Track))
	if err != nil {
		t.Fatal(err)
	}
	mergePath, dismissPath := duplicatesPath+"/"+g.Key+"/merge", duplicatesPath+"/"+g.Key+"/dismiss"
	body := func(keep, trash string) string {
		return `{"keep":"` + keep + `","trash":["` + trash + `"],"plan":` + string(plan) + `}`
	}
	good := body(g.Copies[0].Track, g.Copies[1].Track)
	for _, tc := range []struct {
		name   string
		c      call
		status int
	}{
		{"a bad filter", invalidCall(call{method: http.MethodGet, path: duplicatesPath + "?filter=nope", header: bearer(fx.token)}), http.StatusBadRequest},
		{"a bad cursor", call{method: http.MethodGet, path: duplicatesPath + "?cursor=x", header: bearer(fx.token)}, http.StatusBadRequest},
		{"no such group", call{method: http.MethodGet, path: duplicatesPath + "/" + noGroup, header: bearer(fx.token)}, http.StatusNotFound},
		{"a bad merge body", invalidCall(fx.organizeCall(mergePath, `{"keep":""}`)), http.StatusBadRequest},
		{"merging no such group", fx.organizeCall(duplicatesPath+"/"+noGroup+"/merge", good), http.StatusNotFound},
		{"keeping a song not in it", fx.organizeCall(mergePath, body(unknownRun, g.Copies[1].Track)), http.StatusUnprocessableEntity},
		{"trashing the keeper", fx.organizeCall(mergePath, body(g.Copies[0].Track, g.Copies[0].Track)), http.StatusUnprocessableEntity},
		{"dismissing no such group", fx.organizeCall(duplicatesPath+"/"+noGroup+"/dismiss", ""), http.StatusNotFound},
		{"no credentials for the list", invalidCall(call{method: http.MethodGet, path: duplicatesPath}), http.StatusUnauthorized},
		{"no credentials for a group", invalidCall(call{method: http.MethodGet, path: duplicatesPath + "/" + g.Key}), http.StatusUnauthorized},
		{"no credentials to merge", call{method: http.MethodPost, path: mergePath, body: good, header: jsonHeader("")}, http.StatusUnauthorized},
		{"no credentials to dismiss", call{method: http.MethodPost, path: dismissPath, header: jsonHeader("")}, http.StatusUnauthorized},
		{"no credentials to bring back", call{method: http.MethodDelete, path: dismissPath, header: jsonHeader("")}, http.StatusUnauthorized},
		{"a listener lists", call{method: http.MethodGet, path: duplicatesPath, header: bearer(user)}, http.StatusForbidden},
		{"a listener reads a group", call{method: http.MethodGet, path: duplicatesPath + "/" + g.Key, header: bearer(user)}, http.StatusForbidden},
		{"a listener merges", call{method: http.MethodPost, path: mergePath, body: good, header: jsonHeader(user)}, http.StatusForbidden},
		{"a listener dismisses", call{method: http.MethodPost, path: dismissPath, header: jsonHeader(user)}, http.StatusForbidden},
		{"a listener brings back", call{method: http.MethodDelete, path: dismissPath, header: jsonHeader(user)}, http.StatusForbidden},
	} {
		if resp := fx.send(t, tc.c); resp.Status != tc.status {
			t.Errorf("%s: status = %d, want %d: %s", tc.name, resp.Status, tc.status, resp.Body)
		}
	}
}

// With tag writes off the list and a group still read, and a merge is a 409.
func TestDuplicatesMergeNeedsTagWrites(t *testing.T) {
	t.Parallel()
	fx := dupesFixture(t, false)
	g := fx.group(t)
	keep := g.Copies[0].Track
	fx.merge(t, &g, keep, new(planOf(t, &g, keep))).problem(t, http.StatusConflict)
}

// pairFixture is a server with tag writes on over three songs on two albums,
// which make an album pair, and a fourth only the second album has.
func pairFixture(t *testing.T) *editFixture {
	t.Helper()
	titles := []string{"Wish Upon a Blackstar", "The Best It's Gonna Get", "End of an Empire", "The Lucky One"}
	files := map[string]string{}
	for i, title := range titles {
		if i < 3 {
			files["plain/"+title+".flac"] = sampleFLAC
		}
		files["deluxe/"+title+".flac"] = sampleFLAC
	}
	fx := newEditServer(t, &testServerOptions{TagWrites: true}, files)
	for rel := range files {
		album := "Wish Upon a Blackstar"
		if strings.HasPrefix(rel, "deluxe/") {
			album += " (Deluxe)"
		}
		title := strings.TrimSuffix(rel[strings.IndexByte(rel, '/')+1:], ".flac")
		err := audiometa.Write(filepath.Join(fx.root, rel), func(f *audiometa.File) error {
			f.Tags.Title, f.Tags.Album, f.Tags.Artists, f.Tags.AlbumArtist = title, album, []string{"Celldweller"}, "Celldweller"
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fx.Scanner.Scan(t.Context(), library.Full); err != nil {
		t.Fatal(err)
	}
	return fx
}

// Three groups on two albums are one pair, listed first, with its groups in
// its card; its page matches the track lists row by row.
func TestDuplicatesAlbumPair(t *testing.T) {
	t.Parallel()
	fx := pairFixture(t)
	l := fx.list(t, "")
	if l.Counts.Groups != 3 || l.Counts.Pairs != 1 || len(l.Pairs) != 1 || len(l.Groups) != 0 {
		t.Fatalf("list = %+v, want one pair holding its three groups", l)
	}
	if p := l.Pairs[0]; p.Both != 3 || p.Only[0]+p.Only[1] != 1 {
		t.Errorf("pair = %+v, want three in both and one only in the Deluxe", p)
	}
	d := fx.pair(t, l.Pairs[0].Key)
	if len(d.Rows) != 4 || len(d.Plans) != 2 || d.Keeper == "" {
		t.Errorf("pair = %+v, want four rows, a keeper, and a plan for each album", d)
	}
}

// Keeping one album merges the other's three songs into it and leaves the
// song only it has.
func TestDuplicatesMergeAPair(t *testing.T) {
	t.Parallel()
	fx := pairFixture(t)
	d := fx.pair(t, fx.list(t, "").Pairs[0].Key)
	keep, other := d.Albums[0].ID, d.Albums[1].ID
	body, err := json.Marshal(api.MergeRequest{Keep: keep, Trash: []string{other}, Plan: d.Plans[0].Plan})
	if err != nil {
		t.Fatal(err)
	}
	resp := fx.send(t, fx.organizeCall(duplicatesPath+"/"+d.Key+"/merge", string(body)))
	var merged api.Merged
	resp.decode(t, &merged)
	if resp.Status != http.StatusOK || len(merged.Kept) != 3 || len(merged.Trashed.Tracks) != 3 {
		t.Fatalf("merge = %d %+v, want three kept and three trashed", resp.Status, merged)
	}
	if after := fx.list(t, ""); after.Counts.Groups != 0 || after.Counts.Library != 4 {
		t.Errorf("after: %+v, want no group, and the Lucky One still there", after.Counts)
	}
}

// pair reads the album pair key.
func (fx *editFixture) pair(t *testing.T, key string) *api.DuplicatePair {
	t.Helper()
	var d api.DuplicateDetail
	fx.getJSON(t, fx.token, duplicatesPath+"/"+key, &d)
	if d.Kind != api.DuplicateKindPair || d.Pair == nil {
		t.Fatalf("detail = %+v, want a pair", d)
	}
	return d.Pair
}

// Dismissing a pair dismisses its groups with it.
func TestDuplicatesDismissAPair(t *testing.T) {
	t.Parallel()
	fx := pairFixture(t)
	key := fx.list(t, "").Pairs[0].Key
	if resp := fx.send(t, fx.organizeCall(duplicatesPath+"/"+key+"/dismiss", "")); resp.Status != http.StatusNoContent {
		t.Fatalf("dismiss = %d", resp.Status)
	}
	l := fx.list(t, "?filter=dismissed")
	if l.Counts.Groups != 0 || l.Counts.Dismissed != 1 || len(l.Pairs) != 1 || len(l.Groups) != 0 {
		t.Errorf("dismissed = %+v, want the pair alone", l)
	}
}
