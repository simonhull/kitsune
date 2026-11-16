package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// A merge keeps one copy of a recording and trashes the others. Before they go,
// everything people did with them moves onto the keeper, in the transaction
// that marks them trashed:
//
//   - plays and listens are repointed, unchanged: client IDs stay unique per
//     person, so nothing collides;
//   - each person's stars are the higher of theirs, or the one there is, with
//     its rated_at, and a rating raised over the person's love line queues a
//     Last.fm love as any rating does ([loveCrossed]);
//   - a heart kept from before moves unless the keeper has one, the earlier
//     staying;
//   - playlist entries, a file playlist's included, are repointed in place,
//     keeping their IDs and places, so a playlist holding both copies holds
//     the keeper twice;
//   - a shuffle exclusion is carried over (the union), and the keeper takes a
//     trashed copy's place in a Combine run unless the person has it in a run
//     already, when the trashed copy just leaves its run;
//   - a remembered queue naming a trashed copy names the keeper.
//
// The Last.fm outbox, the tag edit history and lookups stay as they are, and a
// trashed copy's loudness, fingerprint and artwork go with it. So a copy
// restored from the Trash comes back bare.

// MergeSong is a song in a merge: its row and public ID.
type MergeSong struct {
	ID   int64
	ULID string
}

// Merge is one keeper and the copies that go into it.
type Merge struct {
	Keep  MergeSong
	Trash []MergeSong
}

// MergePlan is what a merge moves, as its confirm words it. It holds counts,
// the viewer's own stars and the names of playlists the viewer sees by name:
// never another person's own rating, or another person's playlist's name.
type MergePlan struct {
	Plays, Listens int
	Stars          StarsPlan
	// Loves counts the Last.fm loves the stars that come over queue.
	Loves int
	// Hearts counts the hearts kept from before that move.
	Hearts int
	// Entries counts the playlist entries repointed, in any playlist.
	Entries int
	// Playlists are the viewer's own playlists and the file playlists that
	// hold an entry, each once, by name.
	Playlists []PlanPlaylist
	// OtherPlaylists counts other people's playlists that hold an entry.
	OtherPlaylists int
	// Excluded counts the people a trashed copy was excluded for and the
	// keeper wasn't, and Runs the places in Combine runs that move or go.
	Excluded, Runs int
	// Queues counts the people whose remembered queue names a trashed copy.
	Queues int
}

// StarsPlan is what a merge does with stars.
type StarsPlan struct {
	// Yours is the viewer's own, for a merge of one group in which they rated
	// a copy; nil otherwise.
	Yours *YourStars
	// Come counts the ratings that come over to a keeper: a person who rated a
	// trashed copy higher than the keeper, or rated only it. Stay counts those
	// in which the keeper's own was as high. For one group the viewer is in
	// Yours, not here.
	Come, Stay int
}

// YourStars is the viewer's own stars in a merge of one group.
type YourStars struct {
	Keeper, Other, After rating.Rating // Other is the highest the trashed copies had.
}

// PlanPlaylist is a playlist a merge repoints entries of.
type PlanPlaylist struct {
	ID, Name string
	File     bool // A file playlist, which everyone sees.
}

// Equal reports whether p and o say the same.
func (p *MergePlan) Equal(o *MergePlan) bool {
	return p.counts() == o.counts() && slices.Equal(p.Playlists, o.Playlists)
}

// planCounts is a plan without its playlists, which compares with ==.
type planCounts struct {
	plays, listens, loves, hearts, entries, others, excluded, runs, queues, come, stay int
	yours                                                                              YourStars
	rated                                                                              bool
}

// counts returns p without its playlists.
func (p *MergePlan) counts() planCounts {
	c := planCounts{
		plays: p.Plays, listens: p.Listens, loves: p.Loves, hearts: p.Hearts, entries: p.Entries,
		others: p.OtherPlaylists, excluded: p.Excluded, runs: p.Runs, queues: p.Queues,
		come: p.Stars.Come, stay: p.Stars.Stay,
	}
	if y := p.Stars.Yours; y != nil {
		c.yours, c.rated = *y, true
	}
	return c
}

// MergeChange is what a merge changed, for the events that tell people.
type MergeChange struct {
	// Marks are the keepers' new ratings, one per person and keeper.
	Marks []MarkChange
	// Playlists are the person's own playlists whose entries were repointed,
	// by owner, and Files the file playlists'.
	Playlists map[int64][]string
	Files     []string
	// Listeners are the people whose plays or listens moved, and Shufflers
	// those whose exclusions or Combine runs did.
	Listeners, Shufflers []int64
	// Loves says a Last.fm love was queued.
	Loves bool
}

// MarkChange is one person's new rating of a keeper.
type MarkChange struct {
	User  int64
	Track string // The keeper's ULID.
	Stars rating.Rating
}

// PlanChangedError means the library changed since a merge's confirm was
// shown: what it would move now is Plan. Nothing was merged.
type PlanChangedError struct {
	Plan MergePlan
}

func (e *PlanChangedError) Error() string { return "the merge would move something else now" }

// PlanMerge works out what merges would move, as the user viewer sees it.
func (s *Store) PlanMerge(ctx context.Context, viewer int64, merges []Merge) (MergePlan, error) {
	var plan MergePlan
	err := s.inReadTx(ctx, func(q *sqlcdb.Queries) error {
		f, err := readMergeFacts(ctx, q, merges)
		if err != nil {
			return err
		}
		plan = f.plan(merges, viewer)
		return nil
	})
	if err != nil {
		return MergePlan{}, fmt.Errorf("plan merge: %w", err)
	}
	return plan, nil
}

// inReadTx runs fn in a read transaction, so every read sees one snapshot.
func (s *Store) inReadTx(ctx context.Context, fn func(q *sqlcdb.Queries) error) error {
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // Read-only: nothing to undo.
	return fn(s.rq.WithTx(tx))
}

// MergeAndTrash merges, then marks every trashed copy trashed by the user by
// as of at, each with the path trash gives its file in the Trash, in one
// transaction: all or nothing. When what the merges would move, as viewer
// sees it, is not expect, it merges nothing and returns a [*PlanChangedError]
// with what it would move now. It returns an error wrapping [ErrNotFound],
// and changes nothing, when a song isn't live.
func (s *Store) MergeAndTrash(ctx context.Context, viewer int64, merges []Merge, expect *MergePlan, trash []Trashing, by int64, at time.Time) (MergeChange, error) {
	var change MergeChange
	var touched []string
	err := s.inLibraryTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		change, touched = MergeChange{}, touched[:0] // The writer may retry a transaction.
		f, err := readMergeFacts(ctx, q, merges)
		if err != nil {
			return err
		}
		if plan := f.plan(merges, viewer); !plan.Equal(expect) {
			return &PlanChangedError{Plan: plan}
		}
		if change, err = f.apply(ctx, q, merges, at); err != nil {
			return err
		}
		touched, err = markTrashed(ctx, q, trash, by, at)
		return err
	})
	if _, changed := errors.AsType[*PlanChangedError](err); changed {
		return MergeChange{}, err
	}
	if err != nil {
		return MergeChange{}, fmt.Errorf("merge: %w", err)
	}
	s.changed.add(touched)
	return change, nil
}

// markTrashed marks tracks trashed through q, and returns their albums, as
// [Store.MarkTrashed] does.
func markTrashed(ctx context.Context, q *sqlcdb.Queries, tracks []Trashing, by int64, at time.Time) ([]string, error) {
	var touched []string
	for _, t := range tracks {
		album, err := q.TrackAlbumULID(ctx, t.ID)
		if err != nil {
			return nil, fmt.Errorf("track %d: %w", t.ID, notFound(err))
		}
		n, err := q.MarkTrashed(ctx, sqlcdb.MarkTrashedParams{At: new(ms(at)), By: nullID(by), TrashPath: t.TrashPath, ID: t.ID})
		if err != nil {
			return nil, fmt.Errorf("track %d: %w", t.ID, err)
		}
		if n == 0 {
			return nil, fmt.Errorf("track %d: %w", t.ID, ErrNotFound)
		}
		touched = append(touched, album)
	}
	return touched, nil
}

// mergeFacts is what the copies of some merges hold, read in one snapshot.
type mergeFacts struct {
	plays, listens []sqlcdb.MergePlaysRow
	ratings        []sqlcdb.MergeRatingsRow
	hearts         []sqlcdb.TrackHeart
	entries        []sqlcdb.MergeEntriesRow
	excluded       []sqlcdb.MergeExcludedRow
	runs           []sqlcdb.ShuffleRunTrack
	queues         []sqlcdb.MergeQueuesRow
}

// readMergeFacts reads what the copies of merges hold.
func readMergeFacts(ctx context.Context, q *sqlcdb.Queries, merges []Merge) (*mergeFacts, error) {
	songs := songsOf(merges)
	ids := songs.ids
	list, err := json.Marshal(songs.trashed)
	if err != nil {
		return nil, fmt.Errorf("merge facts: %w", err)
	}
	f := &mergeFacts{}
	if f.plays, err = q.MergePlays(ctx, ids); err != nil {
		return nil, fmt.Errorf("merge facts: plays: %w", err)
	}
	listens, err := q.MergeListens(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("merge facts: listens: %w", err)
	}
	f.listens = make([]sqlcdb.MergePlaysRow, len(listens))
	for i, r := range listens {
		f.listens[i] = sqlcdb.MergePlaysRow(r)
	}
	if f.ratings, err = q.MergeRatings(ctx, ids); err != nil {
		return nil, fmt.Errorf("merge facts: ratings: %w", err)
	}
	if f.hearts, err = q.MergeHearts(ctx, ids); err != nil {
		return nil, fmt.Errorf("merge facts: hearts: %w", err)
	}
	if f.entries, err = q.MergeEntries(ctx, ids); err != nil {
		return nil, fmt.Errorf("merge facts: entries: %w", err)
	}
	return f, readShuffleFacts(ctx, q, f, ids, string(list))
}

// readShuffleFacts reads the exclusions, Combine runs and remembered queues
// of the songs ids into f; list is the trashed copies' ULIDs as JSON.
func readShuffleFacts(ctx context.Context, q *sqlcdb.Queries, f *mergeFacts, ids []int64, list string) error {
	var err error
	if f.excluded, err = q.MergeExcluded(ctx, ids); err != nil {
		return fmt.Errorf("merge facts: excluded: %w", err)
	}
	if f.runs, err = q.MergeRunTracks(ctx, ids); err != nil {
		return fmt.Errorf("merge facts: runs: %w", err)
	}
	if f.queues, err = q.MergeQueues(ctx, list); err != nil {
		return fmt.Errorf("merge facts: queues: %w", err)
	}
	return nil
}

// mergeSongs are the songs of some merges: every row, and the trashed
// copies' ULIDs.
type mergeSongs struct {
	ids     []int64
	trashed []string
}

// songsOf returns the songs of merges.
func songsOf(merges []Merge) mergeSongs {
	var out mergeSongs
	for i := range merges {
		out.ids = append(out.ids, merges[i].Keep.ID)
		for _, t := range merges[i].Trash {
			out.ids = append(out.ids, t.ID)
			out.trashed = append(out.trashed, t.ULID)
		}
	}
	return out
}

// trashedOf returns the rows of m's trashed copies as a set.
func trashedOf(m *Merge) map[int64]bool {
	out := make(map[int64]bool, len(m.Trash))
	for _, t := range m.Trash {
		out[t.ID] = true
	}
	return out
}

// starsMove is one person's stars in one merge: theirs on the keeper, the
// highest of the trashed copies' with its time, and what the keeper ends with.
type starsMove struct {
	user                 int64
	keeper, other, after rating.Rating
	ratedAt              int64 // The other's.
	loveFrom             *int64
	loves                bool
}

// comes reports whether the other's stars come over.
func (m *starsMove) comes() bool { return m.after != m.keeper }

// love reports whether the stars coming over queue a Last.fm love.
func (m *starsMove) love() bool {
	if !m.comes() || !m.loves || m.loveFrom == nil {
		return false
	}
	line := ratingOf(*m.loveFrom)
	return (!m.keeper.Valid() || m.keeper < line) && m.after >= line
}

// stars returns each person's stars in m who rated a trashed copy, by person.
func (f *mergeFacts) stars(m *Merge) []starsMove {
	trashed := trashedOf(m)
	by := map[int64]*starsMove{}
	var order []int64
	for i := range f.ratings {
		r := &f.ratings[i]
		if r.TrackID != m.Keep.ID && !trashed[r.TrackID] {
			continue
		}
		mv, ok := by[r.UserID]
		if !ok {
			mv = &starsMove{user: r.UserID, loveFrom: r.LoveFrom, loves: r.Loves == 1}
			by[r.UserID] = mv
			order = append(order, r.UserID)
		}
		stars := ratingOf(r.Halves)
		switch {
		case r.TrackID == m.Keep.ID:
			mv.keeper = stars
		case stars > mv.other:
			mv.other, mv.ratedAt = stars, r.RatedAt
		}
	}
	out := make([]starsMove, 0, len(order))
	for _, u := range order {
		mv := by[u]
		if !mv.other.Valid() {
			continue // Rated only the keeper: nothing moves.
		}
		mv.after = max(mv.keeper, mv.other)
		out = append(out, *mv)
	}
	return out
}

// plan works out what merges would move, as viewer sees it.
func (f *mergeFacts) plan(merges []Merge, viewer int64) MergePlan {
	var p MergePlan
	trashed := map[int64]bool{}
	for i := range merges {
		m := &merges[i]
		addKeys(trashedOf(m), trashed)
		f.planStars(&p, m, viewer, len(merges) == 1)
		p.Hearts += f.heartsMoving(m)
		p.Excluded += len(f.excludedMoving(m))
	}
	p.Plays, p.Listens = countOf(f.plays, trashed), countOf(f.listens, trashed)
	f.planEntries(&p, trashed, viewer)
	for i := range f.runs {
		if trashed[f.runs[i].TrackID] {
			p.Runs++
		}
	}
	p.Queues = len(f.queues)
	return p
}

// planStars adds to p what m does with stars. When one says it is the only
// merge, the viewer's own are told apart.
func (f *mergeFacts) planStars(p *MergePlan, m *Merge, viewer int64, one bool) {
	for _, mv := range f.stars(m) {
		if mv.love() {
			p.Loves++
		}
		switch {
		case mv.user == viewer && one:
			p.Stars.Yours = &YourStars{Keeper: mv.keeper, Other: mv.other, After: mv.after}
		case mv.comes():
			p.Stars.Come++
		default:
			p.Stars.Stay++
		}
	}
}

// countOf sums the counts of rows on trashed tracks.
func countOf(rows []sqlcdb.MergePlaysRow, trashed map[int64]bool) int {
	n := 0
	for _, r := range rows {
		if trashed[r.TrackID] {
			n += int(r.N)
		}
	}
	return n
}

// planEntries adds to p the playlist entries of trashed tracks: the viewer's
// own playlists and the file playlists by name, others' counted.
func (f *mergeFacts) planEntries(p *MergePlan, trashed map[int64]bool, viewer int64) {
	others := map[string]bool{}
	for i := range f.entries {
		e := &f.entries[i]
		if !trashed[e.TrackID] {
			continue
		}
		p.Entries++
		switch {
		case e.UserID == nil || *e.UserID == viewer:
			pl := PlanPlaylist{ID: e.Playlist, Name: e.Name, File: e.UserID == nil}
			if !slices.Contains(p.Playlists, pl) {
				p.Playlists = append(p.Playlists, pl)
			}
		case !others[e.Playlist]:
			others[e.Playlist] = true
			p.OtherPlaylists++
		}
	}
	slices.SortStableFunc(p.Playlists, func(a, b PlanPlaylist) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
}

// heartsMoving counts the people whose heart on a trashed copy of m moves to
// a keeper that has none.
func (f *mergeFacts) heartsMoving(m *Merge) int {
	trashed := trashedOf(m)
	has, moving := map[int64]bool{}, map[int64]bool{}
	for _, h := range f.hearts {
		switch {
		case h.TrackID == m.Keep.ID:
			has[h.UserID] = true
		case trashed[h.TrackID]:
			moving[h.UserID] = true
		}
	}
	n := 0
	for u := range moving {
		if !has[u] {
			n++
		}
	}
	return n
}

// excludedMoving returns the people a trashed copy of m is excluded for and
// the keeper isn't.
func (f *mergeFacts) excludedMoving(m *Merge) []int64 {
	trashed := trashedOf(m)
	has := map[int64]bool{}
	var moving []int64
	for _, e := range f.excluded {
		switch {
		case e.TrackID == m.Keep.ID:
			has[e.UserID] = true
		case trashed[e.TrackID] && !slices.Contains(moving, e.UserID):
			moving = append(moving, e.UserID)
		}
	}
	return slices.DeleteFunc(moving, func(u int64) bool { return has[u] })
}

// apply makes merges through q, as plan says, at now.
func (f *mergeFacts) apply(ctx context.Context, q *sqlcdb.Queries, merges []Merge, now time.Time) (MergeChange, error) {
	change := MergeChange{Playlists: map[int64][]string{}}
	trashed := map[int64]bool{}
	for i := range merges {
		m := &merges[i]
		if err := f.applyStars(ctx, q, m, now, &change); err != nil {
			return change, err
		}
		for _, u := range f.excludedMoving(m) {
			change.Shufflers = noted(change.Shufflers, u)
		}
		if err := f.applyRuns(ctx, q, m, &change); err != nil {
			return change, err
		}
		for _, t := range m.Trash {
			if err := repoint(ctx, q, m.Keep, t); err != nil {
				return change, fmt.Errorf("track %d: %w", t.ID, err)
			}
		}
		addKeys(trashedOf(m), trashed)
	}
	for _, rows := range [][]sqlcdb.MergePlaysRow{f.plays, f.listens} {
		for _, r := range rows {
			if trashed[r.TrackID] {
				change.Listeners = noted(change.Listeners, r.UserID)
			}
		}
	}
	f.notePlaylists(&change, trashed)
	return change, nil
}

// noted returns list with u, once.
func noted(list []int64, u int64) []int64 {
	if slices.Contains(list, u) {
		return list
	}
	return append(list, u)
}

// notePlaylists notes in change the playlists whose entries of trashed tracks
// were repointed: each person's own, and the file playlists.
func (f *mergeFacts) notePlaylists(change *MergeChange, trashed map[int64]bool) {
	for i := range f.entries {
		e := &f.entries[i]
		switch {
		case !trashed[e.TrackID]:
		case e.UserID == nil:
			if !slices.Contains(change.Files, e.Playlist) {
				change.Files = append(change.Files, e.Playlist)
			}
		case !slices.Contains(change.Playlists[*e.UserID], e.Playlist):
			change.Playlists[*e.UserID] = append(change.Playlists[*e.UserID], e.Playlist)
		}
	}
}

// addKeys copies the keys of from into into.
func addKeys(from, into map[int64]bool) {
	for k := range from {
		into[k] = true
	}
}

// applyStars gives m's keeper each person's higher stars, queueing a love
// where they cross the person's line, and notes the change.
func (f *mergeFacts) applyStars(ctx context.Context, q *sqlcdb.Queries, m *Merge, now time.Time, change *MergeChange) error {
	for _, mv := range f.stars(m) {
		if !mv.comes() {
			continue
		}
		up := sqlcdb.UpsertTrackRatingParams{UserID: mv.user, TrackID: m.Keep.ID, Halves: mv.after.Halves(), RatedAt: mv.ratedAt}
		if err := q.UpsertTrackRating(ctx, up); err != nil {
			return fmt.Errorf("stars: %w", err)
		}
		if err := loveCrossed(ctx, q, mv.user, m.Keep.ID, mv.keeper, mv.after, now); err != nil {
			return err
		}
		change.Loves = change.Loves || mv.love()
		change.Marks = append(change.Marks, MarkChange{User: mv.user, Track: m.Keep.ULID, Stars: mv.after})
	}
	return nil
}

// applyRuns gives the keeper each trashed copy's place in a Combine run, unless
// the person has the keeper in a run already: then the copy leaves its run.
func (f *mergeFacts) applyRuns(ctx context.Context, q *sqlcdb.Queries, m *Merge, change *MergeChange) error {
	trashed := trashedOf(m)
	inRun := map[int64]bool{}
	for _, r := range f.runs {
		if r.TrackID == m.Keep.ID {
			inRun[r.UserID] = true
		}
	}
	for _, r := range f.runs {
		if !trashed[r.TrackID] {
			continue
		}
		change.Shufflers = noted(change.Shufflers, r.UserID)
		var err error
		if inRun[r.UserID] {
			err = q.DropRunTrack(ctx, sqlcdb.DropRunTrackParams{RunID: r.RunID, Position: r.Position})
		} else {
			err = q.RepointRunTrack(ctx, sqlcdb.RepointRunTrackParams{Keep: m.Keep.ID, RunID: r.RunID, Position: r.Position})
			inRun[r.UserID] = true
		}
		if err != nil {
			return fmt.Errorf("combine run: %w", err)
		}
	}
	return nil
}

// repoint moves what hangs off the trashed copy t onto keep, past its stars
// and runs, and clears what is left on t.
func repoint(ctx context.Context, q *sqlcdb.Queries, keep, t MergeSong) error {
	for _, step := range []func() error{
		func() error { return q.RepointPlays(ctx, sqlcdb.RepointPlaysParams{Keep: keep.ID, Trash: t.ID}) },
		func() error { return q.RepointListens(ctx, sqlcdb.RepointListensParams{Keep: keep.ID, Trash: t.ID}) },
		func() error { return q.DeleteTrackRatings(ctx, t.ID) },
		func() error { return q.MoveTrackHearts(ctx, sqlcdb.MoveTrackHeartsParams{Keep: keep.ID, Trash: t.ID}) },
		func() error { return q.DeleteHeartsOfTrack(ctx, t.ID) },
		func() error { return q.RepointEntries(ctx, sqlcdb.RepointEntriesParams{Keep: keep.ID, Trash: t.ID}) },
		func() error { return q.MoveExcluded(ctx, sqlcdb.MoveExcludedParams{Keep: keep.ID, Trash: t.ID}) },
		func() error { return q.DeleteExcludedOfTrack(ctx, t.ID) },
		func() error {
			return q.RepointQueues(ctx, sqlcdb.RepointQueuesParams{Trash: `"` + t.ULID + `"`, Keep: `"` + keep.ULID + `"`})
		},
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}
