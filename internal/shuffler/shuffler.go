// Package shuffler runs each person's shuffles. It builds a pool from the store
// (the songs a scope holds, with the person's marks, Combine and Exclude rules,
// and what their listening says), hands it to [shuffle], and keeps each
// session's state in the store, so a shuffle goes on where it was, on any of
// the person's devices.
package shuffler

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/simonhull/kitsune/internal/prefs"
	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/shuffle"
	"github.com/simonhull/kitsune/internal/store"
)

var (
	// ErrBadAfter means a page was asked for after a position the session
	// hasn't reached.
	ErrBadAfter = errors.New("the shuffle hasn't got that far")
	// errBadState means a session's stored state can't be read.
	errBadState = errors.New("bad shuffle state")
)

// seedBits keeps seeds within what a JSON number carries exactly.
const seedBits = 1<<53 - 1

// Genres says an album's genres, by its ULID.
type Genres interface {
	Of(albumID string) []string
}

// Service runs shuffles. It is safe for concurrent use.
type Service struct {
	st     *store.Store
	genres Genres
	now    func() time.Time

	mu      sync.Mutex
	history map[int64]*history    // by person
	people  map[int64]*sync.Mutex // by person: one shuffle request at a time
}

// New returns a service over st. A nil genres knows none, and a nil now is
// [time.Now].
func New(st *store.Store, genres Genres, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{st: st, genres: genres, now: now, history: map[int64]*history{}, people: map[int64]*sync.Mutex{}}
}

// Forget drops what is kept of the user's listening, after a listen or a play
// of theirs: their next page reads it again.
func (s *Service) Forget(userID int64) {
	s.mu.Lock()
	delete(s.history, userID)
	s.mu.Unlock()
}

// Start is a request for a new shuffle.
type Start struct {
	// ID is the new session's ULID.
	ID    string
	Scope Scope
	Mode  Mode
	// First is the ULID of the song to lead with, which the client may already
	// be playing; "" for none. An unknown one leads with nothing.
	First string
	// Seed is the shuffle's seed; nil for a random one.
	Seed *uint64
}

// Page is one page of a shuffle.
type Page struct {
	Session string
	Mode    Mode
	// From is the position the page starts at, and To the one after it, which
	// the next page is asked for after.
	From, To int64
	// Songs is how many songs the scope holds that shuffle may pick.
	Songs int
	// Done says a finite shuffle has nothing after this page.
	Done bool
	// Moved says the session moved on to give the page; a page given before
	// leaves it where it was.
	Moved bool
	Picks []Pick
}

// Pick is one unit of a page: its songs, in play order, and why it came up.
type Pick struct {
	Songs []store.TrackHit
	Why   Why
	// After is the artist flow leaned from, if it did.
	After *store.ArtistLink
}

// Start begins a shuffle for the user, and returns its first page.
//
// # Errors
//
// [ErrBadScope] for a scope or mode that isn't one, and an error wrapping
// [store.ErrNotFound] for an album or artist that isn't there.
func (s *Service) Start(ctx context.Context, userID int64, req *Start) (Page, error) {
	if err := req.Scope.check(); err != nil {
		return Page{}, err
	}
	if !req.Mode.valid() {
		return Page{}, fmt.Errorf("mode %q: %w", req.Mode, ErrBadScope)
	}
	defer s.lock(userID)()
	now := s.now()
	b, err := s.build(ctx, userID, &req.Scope, now)
	if err != nil {
		return Page{}, err
	}
	first, err := s.firstOf(ctx, req.First)
	if err != nil {
		return Page{}, err
	}
	seed := randomSeed()
	if req.Seed != nil {
		seed = *req.Seed & seedBits
	}
	st, g := b.begin(req.Mode, seed, first)
	scope, err := json.Marshal(&req.Scope)
	if err != nil {
		return Page{}, fmt.Errorf("encode scope: %w", err)
	}
	sess := store.ShuffleSession{
		ID: req.ID, Scope: string(scope), Mode: string(req.Mode), Seed: seed, Position: g.To,
		CreatedAt: now, UpdatedAt: now,
	}
	if sess.State, err = st.encode(); err != nil {
		return Page{}, err
	}
	if createErr := s.st.CreateShuffleSession(ctx, userID, &sess); createErr != nil {
		return Page{}, createErr //nolint:wrapcheck // The store names it.
	}
	page, err := s.page(ctx, &sess, &g, st.Songs, done(st, req.Mode, g.To))
	page.Moved = true
	return page, err
}

// Next returns the user's session id from position after on. after is
// usually the last page's To; a device behind gets what was given from there,
// while the session keeps it.
//
// # Errors
//
// An error wrapping [store.ErrNotFound] for a session the user doesn't have,
// and [ErrBadAfter] for a position it hasn't reached.
func (s *Service) Next(ctx context.Context, userID int64, id string, after int64) (Page, error) {
	defer s.lock(userID)()
	sess, st, scope, err := s.load(ctx, userID, id)
	if err != nil {
		return Page{}, err
	}
	mode := Mode(sess.Mode)
	switch {
	case after < 0 || after > sess.Position:
		return Page{}, fmt.Errorf("after %d of %d: %w", after, sess.Position, ErrBadAfter)
	case after < sess.Position:
		g, ok := st.givenFrom(after)
		if !ok { // Older than any page kept: the oldest is what there is.
			g = st.Given[0]
		}
		return s.page(ctx, &sess, &g, st.Songs, done(st, mode, g.To))
	}
	now := s.now()
	b, err := s.build(ctx, userID, &scope, now)
	if err != nil {
		return Page{}, err
	}
	g, err := b.next(st, &sess)
	if err != nil {
		return Page{}, err
	}
	if sess.State, err = st.encode(); err != nil {
		return Page{}, err
	}
	was := sess.Position
	sess.Position, sess.UpdatedAt = g.To, now
	if advanceErr := s.st.AdvanceShuffleSession(ctx, userID, &sess, was); advanceErr != nil {
		return Page{}, advanceErr //nolint:wrapcheck // The store names it.
	}
	page, err := s.page(ctx, &sess, &g, st.Songs, done(st, mode, g.To))
	page.Moved = g.To > was
	return page, err
}

// load reads the user's session id, with its state and scope.
func (s *Service) load(ctx context.Context, userID int64, id string) (store.ShuffleSession, *state, Scope, error) {
	sess, err := s.st.ShuffleSessionOf(ctx, userID, id)
	if err != nil {
		return store.ShuffleSession{}, nil, Scope{}, err //nolint:wrapcheck // The store names it.
	}
	st, err := decodeState(sess.State)
	if err != nil {
		return store.ShuffleSession{}, nil, Scope{}, err
	}
	var scope Scope
	if err = json.Unmarshal([]byte(sess.Scope), &scope); err != nil {
		return store.ShuffleSession{}, nil, Scope{}, fmt.Errorf("decode scope: %w: %w", errBadState, err)
	}
	if len(st.Given) == 0 {
		return store.ShuffleSession{}, nil, Scope{}, fmt.Errorf("no page given: %w", errBadState)
	}
	return sess, st, scope, nil
}

// lock takes the user's turn, and returns what gives it back.
func (s *Service) lock(userID int64) func() {
	s.mu.Lock()
	m, ok := s.people[userID]
	if !ok {
		m = &sync.Mutex{}
		s.people[userID] = m
	}
	s.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// done reports whether a session of mode has nothing after to.
func done(st *state, mode Mode, to int64) bool {
	return mode == Finite && to >= int64(len(st.Order))
}

// firstOf returns the row ID of the song with the ULID id, or 0 for none or
// an unknown one.
func (s *Service) firstOf(ctx context.Context, id string) (shuffle.TrackID, error) {
	if id == "" {
		return 0, nil
	}
	rows, err := s.st.TrackRows(ctx, []string{id})
	if err != nil {
		return 0, err //nolint:wrapcheck // The store names it.
	}
	return shuffle.TrackID(rows[id]), nil
}

// page is g as the client gets it: every song named, and every artist flow
// leaned from.
func (s *Service) page(ctx context.Context, sess *store.ShuffleSession, g *given, songs int, done bool) (Page, error) {
	var tracks, artists []int64
	for i := range g.Picks {
		p := &g.Picks[i]
		tracks = append(tracks, p.Tracks...)
		if p.Why.After != 0 {
			artists = append(artists, p.Why.After)
		}
	}
	byRow, err := s.st.TrackHits(ctx, tracks)
	if err != nil {
		return Page{}, err //nolint:wrapcheck // The store names it.
	}
	links, err := s.st.ArtistLinks(ctx, artists)
	if err != nil {
		return Page{}, err //nolint:wrapcheck // The store names it.
	}
	out := Page{Session: sess.ID, Mode: Mode(sess.Mode), From: g.From, To: g.To, Songs: songs, Done: done, Picks: make([]Pick, 0, len(g.Picks))}
	for i := range g.Picks {
		p := &g.Picks[i]
		pick := Pick{Why: p.Why}
		for _, row := range p.Tracks {
			if hit, ok := byRow[row]; ok {
				pick.Songs = append(pick.Songs, hit)
			}
		}
		if link, ok := links[p.Why.After]; ok {
			pick.After = &link
		}
		if len(pick.Songs) > 0 {
			out.Picks = append(out.Picks, pick)
		}
	}
	return out, nil
}

// rowsOf is ids as row IDs.
func rowsOf(ids []shuffle.TrackID) []int64 {
	out := make([]int64, len(ids))
	for i, id := range ids {
		out[i] = int64(id)
	}
	return out
}

// randomSeed is a seed no one can guess or repeat.
func randomSeed() uint64 {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails.
	return binary.LittleEndian.Uint64(b[:]) & seedBits
}

// settingsOf reads the user's shuffle settings. A setting they never made, or
// one that no longer passes its rule, takes its default.
func (s *Service) settingsOf(ctx context.Context, userID int64) (shuffle.Settings, error) {
	out := shuffle.DefaultSettings()
	for _, k := range [...]struct {
		key  string
		into any
	}{{prefs.ShuffleRatings, &out.Ratings}, {prefs.ShuffleFlow, &out.Flow}, {prefs.ShuffleMemory, &out.Memory}} {
		stored, err := s.st.Pref(ctx, userID, k.key)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return shuffle.Settings{}, fmt.Errorf("shuffle settings: %w", err)
		}
		key, err := prefs.Lookup(k.key)
		if err != nil {
			return shuffle.Settings{}, fmt.Errorf("shuffle settings: %w", err)
		}
		if value, checkErr := key.Check(stored); checkErr == nil {
			_ = json.Unmarshal(value, k.into) // Check proved it decodes.
		}
	}
	return out, nil
}

// historyOf returns the user's history as at now, read again when it is old or
// they have listened since.
func (s *Service) historyOf(ctx context.Context, userID int64, now time.Time) (*history, error) {
	s.mu.Lock()
	h, ok := s.history[userID]
	s.mu.Unlock()
	if ok && now.Sub(h.read) < historyFor && !now.Before(h.read) {
		return h, nil
	}
	h, err := readHistory(ctx, s.st, userID, now)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.history[userID] = h
	s.mu.Unlock()
	return h, nil
}

// built is a pool for one page, with what working it out needs.
type built struct {
	pool     *shuffle.Pool
	config   shuffle.Config
	songs    int
	memory   bool
	units    map[shuffle.TrackID]int // unit IDs to their index, made when a finite page needs it
	unitList []shuffle.Unit
}

// build makes the pool of the user's scope as at now.
func (s *Service) build(ctx context.Context, userID int64, sc *Scope, now time.Time) (*built, error) {
	settings, err := s.settingsOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	catalog, err := s.st.ShuffleCatalog(ctx, userID)
	if err != nil {
		return nil, err //nolint:wrapcheck // The store names it.
	}
	in, err := keeper(ctx, s.st, userID, sc)
	if err != nil {
		return nil, err
	}
	hist, err := s.historyOf(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	songs, runs := inScope(&catalog, in)
	tracks := make([]shuffle.Track, len(songs))
	for i, song := range songs {
		artists := make([]shuffle.ArtistID, len(song.Artists))
		for j, a := range song.Artists {
			artists[j] = shuffle.ArtistID(a)
		}
		tracks[i] = shuffle.Track{
			ID: shuffle.TrackID(song.ID), Artists: artists, Album: shuffle.AlbumID(song.AlbumID),
			Stars: starsOf(song.Rating), Hearted: song.Hearted, Skipped: song.Excluded,
			History: hist.of(song.ID),
		}
	}
	pool, err := shuffle.NewPool(tracks, runs)
	if err != nil {
		return nil, fmt.Errorf("shuffle pool: %w", err)
	}
	b := &built{pool: pool, memory: settings.Memory, config: shuffle.Config{Settings: settings, Now: now}}
	if sim := s.similarityOf(songs, &hist.co); sim != nil {
		b.config.Similarity = sim
	}
	b.unitList = pool.Units()
	for i := range b.unitList {
		b.songs += b.unitList[i].Len()
	}
	return b, nil
}

// starsOf is a rating as shuffle takes it: both count half stars.
func starsOf(r rating.Rating) shuffle.Stars {
	if !r.Valid() {
		return shuffle.Unrated
	}
	return shuffle.Stars(r)
}

// inScope returns the catalog's songs that in keeps, and every live song of a
// run that holds one of them, with those runs: a run plays whole, from its
// first song, even into another album.
func inScope(catalog *store.ShuffleCatalog, in keep) ([]*store.ShuffleSong, []shuffle.Run) {
	byID := make(map[int64]*store.ShuffleSong, len(catalog.Songs))
	chosen := make(map[int64]bool)
	for i := range catalog.Songs {
		song := &catalog.Songs[i]
		byID[song.ID] = song
		if in(song) {
			chosen[song.ID] = true
		}
	}
	var runs []shuffle.Run
	for _, run := range catalog.Runs {
		if !slices.ContainsFunc(run, func(id int64) bool { return chosen[id] }) {
			continue
		}
		var members shuffle.Run
		for _, id := range run {
			if _, live := byID[id]; live {
				chosen[id] = true
				members = append(members, shuffle.TrackID(id))
			}
		}
		runs = append(runs, members)
	}
	songs := make([]*store.ShuffleSong, 0, len(chosen))
	for i := range catalog.Songs {
		if chosen[catalog.Songs[i].ID] {
			songs = append(songs, &catalog.Songs[i])
		}
	}
	return songs, runs
}

// similarityOf is flow's similarity among songs' artists, or nil when nothing
// is known of any of them, which turns flow off.
func (s *Service) similarityOf(songs []*store.ShuffleSong, co *coListening) *similarity {
	sim := &similarity{genres: map[shuffle.ArtistID]map[string]bool{}, co: co}
	if s.genres != nil {
		albums := map[string][]string{}
		for _, song := range songs {
			genres, ok := albums[song.AlbumULID]
			if !ok {
				genres = s.genres.Of(song.AlbumULID)
				albums[song.AlbumULID] = genres
			}
			if len(genres) == 0 || len(song.Artists) == 0 {
				continue
			}
			a := shuffle.ArtistID(song.Artists[0])
			set := sim.genres[a]
			if set == nil {
				set = map[string]bool{}
				sim.genres[a] = set
			}
			for _, g := range genres {
				set[g] = true
			}
		}
	}
	if len(sim.genres) < 2 && !co.known() {
		return nil
	}
	return sim
}

// begin works out a new shuffle's state and first page.
func (b *built) begin(mode Mode, seed uint64, first shuffle.TrackID) (*state, given) {
	st := &state{Songs: b.songs}
	var g given
	if mode == Finite {
		order := shuffle.Finite(b.pool, b.config, seed, first)
		st.Order = make([]placed, len(order))
		for i := range order {
			r := &order[i].Reason
			st.Order[i] = placed{Tracks: rowsOf(order[i].Unit.Tracks()), Flow: r.Flow, After: int64(r.After), Sim: r.Similarity}
		}
		g = b.givenOf(order[:min(len(order), shuffle.PageSize)], 0)
	} else {
		e := shuffle.NewEndless(seed, first)
		g = b.givenOf(e.Next(b.pool, b.config, shuffle.PageSize), 0)
		st.Endless = endlessOf(new(e.State()))
	}
	st.remember(g)
	return st, g
}

// givenOf is picks as a page given from position from.
func (b *built) givenOf(picks []shuffle.Pick, from int64) given {
	g := given{From: from, To: from + int64(len(picks)), Picks: make([]givenPick, len(picks))}
	for i := range picks {
		g.Picks[i] = givenPick{At: from + int64(i), Tracks: rowsOf(picks[i].Unit.Tracks()), Why: whyOf(&picks[i].Reason, b.memory)}
	}
	return g
}

// next works out the page after the session's position, and keeps it in st.
func (b *built) next(st *state, sess *store.ShuffleSession) (given, error) {
	g, err := b.pageAfter(st, sess)
	if err != nil {
		return given{}, err
	}
	st.remember(g)
	st.Songs = b.songs
	return g, nil
}

// pageAfter works out the page after the session's position.
func (b *built) pageAfter(st *state, sess *store.ShuffleSession) (given, error) {
	from := sess.Position
	if st.Endless != nil {
		e, err := st.Endless.resume(sess.Seed, uint64(from)) //nolint:gosec // G115: a position is never negative.
		if err != nil {
			return given{}, err
		}
		g := b.givenOf(e.Next(b.pool, b.config, shuffle.PageSize), from)
		st.Endless = endlessOf(new(e.State()))
		return g, nil
	}
	if b.units == nil {
		b.units = make(map[shuffle.TrackID]int, len(b.unitList))
		for i := range b.unitList {
			b.units[b.unitList[i].ID()] = i
		}
	}
	to := min(from+shuffle.PageSize, int64(len(st.Order)))
	g := given{From: from, To: to}
	for at := from; at < to; at++ {
		p := &st.Order[at]
		i, ok := b.units[shuffle.TrackID(p.Tracks[0])]
		if !ok { // Gone, or left out, since the order was made.
			continue
		}
		r := shuffle.Explain(&b.unitList[i], b.config.Settings, b.config.Now)
		if p.Flow != 0 {
			r.Flow, r.After, r.Similarity = p.Flow, shuffle.ArtistID(p.After), p.Sim
		}
		g.Picks = append(g.Picks, givenPick{At: at, Tracks: rowsOf(b.unitList[i].Tracks()), Why: whyOf(&r, b.memory)})
	}
	return g, nil
}
