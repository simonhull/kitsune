package library

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/duplicates"
	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store"
)

// Rich Mullins's Creed, on the album it is from and on a best-of, and a song
// that is no copy of anything.
const (
	mullins   = "Rich Mullins"
	creed     = "Creed"
	liturgy   = "A Liturgy, a Legacy & a Ragamuffin Band"
	bestOf    = "Songs"
	creedLit  = "Rich Mullins/1993 - A Liturgy/05 - Creed.flac"
	creedBest = "Rich Mullins/1996 - Songs/04 - Creed.flac"
	holdMe    = "Rich Mullins/1996 - Songs/01 - Hold Me Jesus.flac"
	listName  = "Rich.m3u8"
)

// dupesLib is a library with tag writes on and Creed twice, the best-of
// tagged a compilation by MusicBrainz, scanned.
func dupesLib(t *testing.T) *testLib {
	t.Helper()
	l := newTestLib(t, 1)
	l.opts.TagWrites = true
	l.scanner = NewScanner(l.st, l.opts)
	l.songs(
		tagged{creedLit, mullins, liturgy, creed, 1993, 5},
		tagged{creedBest, mullins, bestOf, creed, 1996, 4},
		tagged{holdMe, mullins, bestOf, "Hold Me Jesus", 1996, 1},
	)
	l.retag(creedBest, func(f *audiometa.File) { f.Tags.Set(keysReleaseType[0], "album", "compilation") })
	l.retag(holdMe, func(f *audiometa.File) { f.Tags.Set(keysReleaseType[0], "album; compilation") })
	l.scan(Quick)
	return l
}

// retag changes the tags of the file at rel.
func (l *testLib) retag(rel string, fn func(f *audiometa.File)) {
	l.t.Helper()
	err := audiometa.Write(filepath.Join(l.roots[0], rel), func(f *audiometa.File) error {
		fn(f)
		return nil
	})
	if err != nil {
		l.t.Fatalf("Write %s: %v", rel, err)
	}
}

// songAt returns the live song at rel, as a merge names it.
func (l *testLib) songAt(rel string) store.MergeSong {
	l.t.Helper()
	lib, err := l.st.OrganizeLibrary(l.t.Context())
	if err != nil {
		l.t.Fatal(err)
	}
	for i := range lib.Songs {
		if lib.Songs[i].RelPath == filepath.FromSlash(rel) {
			return store.MergeSong{ID: lib.Songs[i].ID, ULID: lib.Songs[i].ULID}
		}
	}
	l.t.Fatalf("no live song at %s", rel)
	return store.MergeSong{}
}

// find finds the library's duplicates.
func (l *testLib) find() *Duplicates {
	l.t.Helper()
	d, err := l.scanner.FindDuplicates(l.t.Context())
	if err != nil {
		l.t.Fatal(err)
	}
	return d
}

// Creed on the album and on the best-of are one song, probably, found from
// either copy.
func TestFindDuplicates(t *testing.T) {
	t.Parallel()
	l := dupesLib(t)
	d := l.find()
	if len(d.Groups) != 1 || d.Groups[0].Confidence != duplicates.SameSong {
		t.Fatalf("find = %+v, want Creed's group, probably", d.Result)
	}
	if d.Result.Songs != 2 || len(d.Songs) != 3 || d.Checked != 0 {
		t.Errorf("find = %+v, checked %d, want 2 songs of 3 grouped and none fingerprinted", d.Result, d.Checked)
	}
	if found, ok := d.GroupOf(l.songAt(creedBest).ULID); !ok || found.Key != d.Groups[0].Key {
		t.Error("GroupOf the best-of's copy found no group")
	}
}

// The copy on the album it is from is the original, and the keeper, though the
// best-of's has more plays.
func TestTheKeeperIsOnTheOriginalAlbum(t *testing.T) {
	t.Parallel()
	l := dupesLib(t)
	lit := l.songAt(creedLit).ULID
	if keep, why := keeperOf(l.find(), lit); keep != lit || !slices.Equal(why, []duplicates.Reason{duplicates.Original}) {
		t.Errorf("keeper = %s %q, want A Liturgy's for its original album", keep, why)
	}
}

// A find is kept until a fingerprint lands, and then finds the same audio.
func TestAFindIsKeptUntilAFingerprint(t *testing.T) {
	t.Parallel()
	l := dupesLib(t)
	d := l.find()
	if l.find() != d {
		t.Error("a second find with nothing changed found again")
	}
	l.fingerprint()
	again := l.find()
	if again == d || again.Checked != 3 || again.Groups[0].Confidence != duplicates.SameAudio {
		t.Errorf("after the fingerprint pass: checked %d, confidence %v, want 3 checked and the same audio", again.Checked, again.Groups[0].Confidence)
	}
}

// keeperOf suggests the keeper of d's first group, all copies lossless and
// the one with the ULID played once while the others were played twice.
func keeperOf(d *Duplicates, played string) (string, []duplicates.Reason) {
	g := &d.Groups[0]
	copies := make([]duplicates.Copy, 0, len(g.Songs))
	for _, i := range g.Songs {
		plays := 2
		if d.Songs[i].ULID == played {
			plays = 1
		}
		copies = append(copies, duplicates.Copy{Lossless: true, Original: d.Original[i], Plays: plays})
	}
	keep, why := duplicates.Keeper(copies)
	return d.Songs[g.Songs[keep]].ULID, why
}

// people are the two people of a merge test: an administrator and Ana.
type people struct{ admin, ana int64 }

// twoPeople makes them, and connects Ana to Last.fm with loves from 4★.
func (l *testLib) twoPeople() people {
	l.t.Helper()
	ctx := l.t.Context()
	var p people
	for name, id := range map[string]*int64{"admin": &p.admin, "ana": &p.ana} {
		role := store.RoleUser
		if name == "admin" {
			role = store.RoleAdmin
		}
		u, err := l.st.CreateUser(ctx, store.NewUser{Name: name, PasswordHash: "x", Role: role})
		if err != nil {
			l.t.Fatal(err)
		}
		*id = u.ID
	}
	must(l.t, l.st.ConnectLastfm(ctx, p.ana, "ana", "key"))
	four, _ := rating.FromStars(4)
	must(l.t, l.st.SetLastfmSwitches(ctx, p.ana, store.LastfmSwitches{Scrobble: true, Loves: true, LoveFrom: four}))
	return p
}

// must fails t on err.
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// stars returns n stars.
func stars(t *testing.T, n float64) rating.Rating {
	t.Helper()
	r, err := rating.FromStars(n)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// mergeScene is a library where the admin and Ana did everything a merge
// moves with the best-of's Creed (gone), some of it with the album's (keep).
type mergeScene struct {
	*testLib

	who               people
	keep, gone, other store.MergeSong
	queue             string
	lovesBefore       int
}

// newMergeScene sets the scene: plays and a listen, stars over and under
// Ana's line, a kept heart, a playlist and a file playlist holding both, an
// exclusion, a Combine run of Ana's and one of the admin's holding both, and
// Ana's remembered queue.
func newMergeScene(t *testing.T) *mergeScene {
	t.Helper()
	l := dupesLib(t)
	ctx := t.Context()
	l.write(t, l.roots[0], listName, []byte(creedLit+"\n"+creedBest+"\n"))
	l.scan(Quick)
	sc := &mergeScene{testLib: l, who: l.twoPeople(), keep: l.songAt(creedLit), gone: l.songAt(creedBest), other: l.songAt(holdMe)}
	who, keep, gone := sc.who, sc.keep.ULID, sc.gone.ULID
	for i, p := range []struct {
		user  int64
		track string
	}{{who.ana, gone}, {who.ana, gone}, {who.admin, gone}, {who.admin, keep}} {
		must(t, l.st.RecordPlay(ctx, &store.NewPlay{UserID: p.user, TrackID: p.track, PlayedAt: l.clock.Now(), Source: "web", ClientID: string(rune('a' + i))}))
	}
	must(t, l.st.RecordListen(ctx, &store.NewListen{UserID: who.ana, TrackID: gone, ClientID: "listen-ana-0001", StartedAt: l.clock.Now(), Listened: time.Minute, Ended: store.Completed}))
	must(t, l.st.Rate(ctx, who.admin, keep, stars(t, 4)))
	must(t, l.st.Rate(ctx, who.admin, gone, stars(t, 3.5)))
	must(t, l.st.Rate(ctx, who.ana, gone, stars(t, 4.5))) // Over her line: one love already.
	must(t, l.st.Heart(ctx, who.ana, store.KindTrack, gone))
	added, err := l.st.CreatePlaylist(ctx, who.ana, newTestID(t), "Ragamuffin", &store.PlaylistAdd{Tracks: []string{keep, gone}})
	if err != nil || len(added.Added) != 2 {
		t.Fatalf("playlist = %+v, %v, want both copies in it", added, err)
	}
	must(t, l.st.SetExcluded(ctx, who.ana, []string{gone}, true))
	sc.run(who.ana, sc.other.ULID, gone)
	sc.run(who.admin, keep, gone)
	sc.queue = `{"kind":"songs","tracks":["` + gone + `","` + sc.other.ULID + `"]}`
	must(t, l.st.SetPlayback(ctx, who.ana, &store.Playback{Kind: "songs", Queue: []byte(sc.queue), ClientID: "0123456789abcdef", Device: "computer"}))
	sc.lovesBefore = l.loves(who.ana)
	return sc
}

// run combines tracks into a run of the user's.
func (sc *mergeScene) run(user int64, tracks ...string) {
	sc.t.Helper()
	if _, err := sc.st.SetRun(sc.t.Context(), user, newTestID(sc.t), tracks); err != nil {
		sc.t.Fatal(err)
	}
}

// merges keeps the album's Creed and trashes the best-of's.
func (sc *mergeScene) merges() []store.Merge {
	return []store.Merge{{Keep: sc.keep, Trash: []store.MergeSong{sc.gone}}}
}

// A merge moves every kind of row onto the keeper: plays and listens, the
// higher stars (with a love when they cross Ana's line), playlist entries
// (both kept where a playlist holds both), a file playlist's, an exclusion, a
// Combine run holding both, and a remembered queue; then the copy goes to the
// Trash, and Restore brings back a bare song.
func TestMergeMovesEverythingToTheKeeper(t *testing.T) {
	t.Parallel()
	sc := newMergeScene(t)
	plan, err := sc.st.PlanMerge(t.Context(), sc.who.admin, sc.merges())
	must(t, err)
	want := store.MergePlan{
		Plays: 3, Listens: 1, Stars: store.StarsPlan{Yours: &store.YourStars{Keeper: stars(t, 4), Other: stars(t, 3.5), After: stars(t, 4)}, Come: 1},
		Loves: 1, Hearts: 1, Entries: 2, Playlists: []store.PlanPlaylist{{ID: sc.mustFilePlaylist(listName).ID, Name: "Rich", File: true}},
		OtherPlaylists: 1, Excluded: 1, Runs: 2, Queues: 1,
	}
	if !plan.Equal(&want) {
		t.Fatalf("plan = %+v (yours %+v), want %+v", plan, plan.Stars.Yours, want)
	}
	res, err := sc.scanner.MergeDuplicates(t.Context(), sc.who.admin, sc.merges(), &plan)
	must(t, err)
	if !slices.Equal(res.Trashed.Tracks, []string{sc.gone.ULID}) || !slices.Equal(res.Marks, []store.MarkChange{{User: sc.who.ana, Track: sc.keep.ULID, Stars: stars(t, 4.5)}}) || !res.Loves {
		t.Errorf("merge = %+v, want the best-of's copy trashed and Ana's stars on the keeper, loved", res)
	}
	sc.wantOnTheKeeper()
	sc.wantPlaylistOf(sc.who.ana, sc.keep.ULID, 2)
	sc.wantShuffleMoved()
	sc.wantBareRestore()
}

// wantBareRestore fails unless the copy's file is in the Trash, and Restore
// brings back a song with nothing of its own.
func (sc *mergeScene) wantBareRestore() {
	t := sc.t
	t.Helper()
	if !exists(t, sc.roots[0], trashPath(sc.gone.ULID, creedBest)) || exists(t, sc.roots[0], creedBest) {
		t.Error("the copy's file is not in the Trash")
	}
	if _, err := sc.scanner.RestoreTracks(t.Context(), []string{sc.gone.ULID}); err != nil {
		t.Fatal(err)
	}
	back, err := sc.st.DuplicateCopies(t.Context(), sc.who.ana, []int64{sc.gone.ID})
	must(t, err)
	if c := back[sc.gone.ID]; c.Plays != 0 || c.Raters != 0 || c.Playlists != 0 {
		t.Errorf("restored = %+v, want a bare song", c)
	}
}

// wantOnTheKeeper fails unless the keeper has every play, Ana's stars and
// their love, and the copy isn't live.
func (sc *mergeScene) wantOnTheKeeper() {
	t := sc.t
	t.Helper()
	copies, err := sc.st.DuplicateCopies(t.Context(), sc.who.ana, []int64{sc.keep.ID, sc.gone.ID})
	must(t, err)
	if c := copies[sc.keep.ID]; c.Plays != 4 || c.Stars != stars(t, 4.5) || c.Raters != 2 || c.Playlists != 2 {
		t.Errorf("keeper = %+v, want 4 plays, Ana's 4½★, 2 raters, in 2 playlists", c)
	}
	if _, live := copies[sc.gone.ID]; live {
		t.Error("the trashed copy is still live")
	}
	if got := sc.loves(sc.who.ana); got != sc.lovesBefore+1 {
		t.Errorf("Ana's loves = %d, want one more than %d", got, sc.lovesBefore)
	}
}

// wantShuffleMoved fails unless Ana's exclusion, run and queue name the
// keeper, and the admin's run of both is gone.
func (sc *mergeScene) wantShuffleMoved() {
	t := sc.t
	t.Helper()
	keep, other := sc.keep.ULID, sc.other.ULID
	rules, err := sc.st.ShuffleRulesOf(t.Context(), sc.who.ana, []string{keep, other})
	must(t, err)
	if !slices.Equal(rules.Excluded, []string{keep}) || len(rules.Runs) != 1 || !slices.Equal(rules.Runs[0].Tracks, []string{other, keep}) {
		t.Errorf("Ana's rules = %+v, want the keeper excluded and in her run in the copy's place", rules)
	}
	admin, err := sc.st.ShuffleRulesOf(t.Context(), sc.who.admin, []string{keep})
	must(t, err)
	if len(admin.Runs) != 0 {
		t.Errorf("the admin's run of both = %+v, want it gone with one song left", admin.Runs)
	}
	pb, err := sc.st.Playback(t.Context(), sc.who.ana)
	must(t, err)
	if string(pb.Queue) != strings.Replace(sc.queue, sc.gone.ULID, keep, 1) {
		t.Errorf("queue = %s, want the keeper in the copy's place", pb.Queue)
	}
}

// newTestID is a fresh ULID.
func newTestID(t *testing.T) string {
	t.Helper()
	id, err := NewID(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// loves counts the loves waiting for the user's Last.fm.
func (l *testLib) loves(user int64) int {
	l.t.Helper()
	due, err := l.st.LastfmDue(l.t.Context(), user, time.Now().AddDate(10, 0, 0), 100, TrackMBIDKeys(), store.OutboxLove)
	must(l.t, err)
	return len(due)
}

// wantPlaylistOf fails unless the user's only own playlist holds the track n
// times.
func (l *testLib) wantPlaylistOf(user int64, track string, n int) {
	l.t.Helper()
	lists, err := l.st.Playlists(l.t.Context(), user, nil, nil)
	must(l.t, err)
	for i := range lists {
		p := &lists[i]
		if p.Source == "file" {
			continue
		}
		d, readErr := l.st.Playlist(l.t.Context(), user, p.ID, nil)
		must(l.t, readErr)
		got := 0
		for _, e := range d.Entries {
			if e.Track.ID == track {
				got++
			}
		}
		if got != n || len(d.Entries) != n {
			l.t.Errorf("playlist %s holds %d of %d entries as the keeper, want %d", p.Name, got, len(d.Entries), n)
		}
	}
	file := l.mustFilePlaylist(listName)
	for _, e := range file.Entries {
		if e.Track.ID != track {
			l.t.Errorf("file playlist entry %d is %s, want the keeper", e.ID, e.Track.ID)
		}
	}
}

// A merge whose plan changed, or which fails midway, changes nothing.
func TestMergeIsAllOrNothing(t *testing.T) {
	t.Parallel()
	l := dupesLib(t)
	ctx := t.Context()
	who := l.twoPeople()
	keep, gone := l.songAt(creedLit), l.songAt(creedBest)
	merges := []store.Merge{{Keep: keep, Trash: []store.MergeSong{gone}}}
	plan, err := l.st.PlanMerge(ctx, who.admin, merges)
	must(t, err)
	must(t, l.st.RecordPlay(ctx, &store.NewPlay{UserID: who.ana, TrackID: gone.ULID, PlayedAt: l.clock.Now(), Source: "web", ClientID: "late"}))
	_, err = l.scanner.MergeDuplicates(ctx, who.admin, merges, &plan)
	changed, ok := errors.AsType[*store.PlanChangedError](err)
	if !ok || changed.Plan.Plays != 1 {
		t.Fatalf("merge after a play = %v, want the plan changed, with the play", err)
	}
	// A copy that isn't live fails the transaction after the rows moved: none did.
	gone2 := l.songAt(holdMe)
	l.trash(gone2.ULID)
	plan = changed.Plan
	_, err = l.st.MergeAndTrash(ctx, who.admin, merges, &plan, []store.Trashing{{ID: gone.ID, TrashPath: "x"}, {ID: gone2.ID, TrashPath: "y"}}, who.admin, l.clock.Now())
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("merge with a trashed copy = %v, want not found", err)
	}
	copies, err := l.st.DuplicateCopies(ctx, who.ana, []int64{keep.ID, gone.ID})
	must(t, err)
	if copies[keep.ID].Plays != 0 || copies[gone.ID].Plays != 1 {
		t.Errorf("after a failed merge: %+v, want the play where it was", copies)
	}
	if l.st.DuplicatesMark().Writes == 0 {
		t.Error("no library write was counted")
	}
}

// Tag writes off, nothing merges.
func TestMergeNeedsTagWrites(t *testing.T) {
	t.Parallel()
	l := dupesLib(t)
	l.opts.TagWrites = false
	l.scanner = NewScanner(l.st, l.opts)
	keep, gone := l.songAt(creedLit), l.songAt(creedBest)
	_, err := l.scanner.MergeDuplicates(t.Context(), 0, []store.Merge{{Keep: keep, Trash: []store.MergeSong{gone}}}, &store.MergePlan{})
	if !errors.Is(err, ErrTagWritesOff) {
		t.Errorf("merge with tag writes off = %v, want ErrTagWritesOff", err)
	}
}

// Three songs on two albums make an album pair; the song only one has is
// never part of it.
func TestAlbumPairs(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	l.opts.TagWrites = true
	l.scanner = NewScanner(l.st, l.opts)
	const by, plain, deluxe = "Klayton", "Blackstar", "Blackstar (Deluxe)"
	titles := []string{"Unshakeable", "Against the Tide", "Eon"}
	list := make([]tagged, 0, 2*len(titles)+1)
	for i, title := range titles {
		list = append(list,
			tagged{by + "/plain/" + title + ".flac", by, plain, title, 2012, i + 1},
			tagged{by + "/deluxe/" + title + ".flac", by, deluxe, title, 2012, i + 1})
	}
	list = append(list, tagged{by + "/deluxe/Lucky.flac", by, deluxe, "Lucky", 2012, 9})
	l.songs(list...)
	l.scan(Quick)
	d := l.find()
	if len(d.Groups) != 3 || len(d.Pairs) != 1 {
		t.Fatalf("find = %+v, want three groups in one pair", d.Result)
	}
	p := d.Pairs[0]
	if len(p.Only[0])+len(p.Only[1]) != 1 {
		t.Errorf("only = %v, want the Lucky One alone", p.Only)
	}
	if a, ok := d.Album(p.Albums[0]); !ok || a.Title == "" {
		t.Errorf("album %s not found", p.Albums[0])
	}
}

// release types split by "; " or given one a value both count, and an album
// by another artist is never the original.
func TestOriginal(t *testing.T) {
	t.Parallel()
	const rich, picard, itunes = "rich mullins", "RELEASETYPE", "MUSICBRAINZ ALBUM TYPE"
	keys := ReleaseTypeKeys()
	album := &store.DuplicateAlbum{Owner: rich}
	for _, tt := range []struct {
		name   string
		album  *store.DuplicateAlbum
		artist string
		tags   []store.Tag
		want   bool
	}{
		{"an album of the artist's", album, rich, []store.Tag{{Key: picard, Value: "album"}}, true},
		{"a best-of", album, rich, []store.Tag{{Key: picard, Value: "album; compilation"}}, false},
		{"a soundtrack", album, rich, []store.Tag{{Key: itunes, Value: "Soundtrack"}}, false},
		{"another artist's", album, "amy grant", nil, false},
		{"a compilation", &store.DuplicateAlbum{Owner: rich, Compilation: true}, rich, nil, false},
		{"no owner", &store.DuplicateAlbum{}, rich, nil, false},
	} {
		if got := original(tt.album, tt.artist, tt.tags, keys); got != tt.want {
			t.Errorf("%s: original = %t, want %t", tt.name, got, tt.want)
		}
	}
}

// A release type written under each format's key, as Picard writes it, reads
// back under one of ReleaseTypeKeys.
func TestReleaseTypeKeysPerFormat(t *testing.T) {
	t.Parallel()
	picard, itunes := keysReleaseType[0], keysReleaseType[1]
	for _, tt := range []struct{ sample, key string }{
		{sampleFLAC, picard},
		{sampleMP3, itunes},    // A TXXX.
		{sampleMP3v23, itunes}, // A TXXX in ID3v2.3.
		{sampleM4A, itunes},    // The freeform ----:com.apple.iTunes:MusicBrainz Album Type.
	} {
		t.Run(tt.sample, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, tt.sample)
			err := audiometa.Write(path, func(f *audiometa.File) error {
				f.Tags.Set(tt.key, "album", "compilation")
				return nil
			})
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			got := reread(t, path)
			found := make([]string, 0, len(keysReleaseType))
			for _, k := range ReleaseTypeKeys() {
				found = append(found, rawValues(&got, k)...)
			}
			if !slices.Contains(found, "compilation") {
				t.Errorf("release types = %q, want compilation among them; tags %+v", found, got.Tags)
			}
		})
	}
}
