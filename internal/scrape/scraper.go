// Package scrape is the artist pass (plan 072): it fetches artists' images and
// bios from fanart.tv and from Wikipedia, Wikidata and Wikimedia Commons, by the
// MusicBrainz IDs the library holds, and stores what it chooses as Kitsune's
// own, beside what the library's folders give. It never searches by a name,
// never touches a file under a root, and never takes away an image or a bio a
// folder or an administrator gave. The sources' clients live beneath it
// (fanarttv, wikimedia); MusicBrainz's is the process's one, through matching.
package scrape

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv"
	"github.com/simonhull/kitsune/internal/scrape/wikimedia"
	"github.com/simonhull/kitsune/internal/store"
)

// Links is what the pass asks MusicBrainz: an artist's Wikidata item. The
// process's one [matching.MusicBrainz] client is it.
type Links interface {
	ArtistLinks(ctx context.Context, mbid string) (matching.ArtistLinks, error)
}

// Library is what the pass waits on: a scan's claim, so a scan's covers render
// first. The [library.Scanner] is it.
type Library interface {
	WaitScanFree(ctx context.Context) error
}

// Options sets up a [Scraper].
type Options struct {
	Store *store.Store // Required.
	// Artwork renders what is fetched, sharing its renders with the scan. Nil
	// fetches bios alone.
	Artwork     *artwork.Cache
	MusicBrainz Links             // Nil: Wikimedia is never asked, since only MusicBrainz names an artist's item.
	Fanart      *fanarttv.Client  // Nil: fanart.tv is never asked.
	Wikimedia   *wikimedia.Client // Nil: Wikimedia is never asked.
	Library     Library           // Nil: the pass never waits for a scan.
	// Notify hears the public IDs of artists whose images or bio changed: the
	// artists event. Nil hears nothing.
	Notify func(ids []string)
	Logger *slog.Logger     // Nil discards.
	Now    func() time.Time // Nil means time.Now.
}

// Config is what the settings say of the pass, live.
type Config struct {
	Remote    bool // The master switch: off, nothing is asked.
	Fanart    bool
	Wikipedia bool
	Language  string        // Two letters; "" means English.
	Keys      fanarttv.Keys // The project key, which fanart.tv needs, and a person's own.
}

// Scraper is the artist pass. One serves the process; it is safe for
// concurrent use, and [Scraper.Run] is its one worker.
type Scraper struct {
	o   Options
	log *slog.Logger
	now func() time.Time

	config   atomic.Pointer[Config]
	progress atomic.Pointer[Progress]
	wake     chan struct{} // Holds one signal: something to do.

	mu      sync.Mutex
	wanted  []string                       // Public IDs a person opened, oldest first.
	sources map[store.ScrapeSource]*health // Each source's state, since the last setting change.
}

// health is what the pass knows of a source as a whole.
type health struct {
	refused   bool      // It refused the key, or the client: asked no more until a setting changes.
	restUntil time.Time // It was down or busy: asked no more until then.
	rest      time.Duration
	failure   *Failure // The last, for the card.
}

// Failure is the last thing that went wrong with a source, in words.
type Failure struct {
	Words  string // Never a key, a URL or a path.
	Status int    // The HTTP status, when there was one.
	At     time.Time
}

// Progress is where the pass stands: Done of the Total artists it set out to
// ask about, and the one in hand. All zero while none runs.
type Progress struct {
	Running     bool
	Done, Total int
	Artist      string // The public ID of the artist in hand, or "".
	Name        string // Its name.
}

// New returns a scraper set up by o, with lookups off until [Scraper.Configure].
func New(o Options) *Scraper { //nolint:gocritic // hugeParam: built once, at startup.
	s := &Scraper{o: o, log: o.Logger, now: o.Now, wake: make(chan struct{}, 1), sources: map[store.ScrapeSource]*health{}}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	if s.now == nil {
		s.now = time.Now
	}
	s.config.Store(&Config{})
	s.progress.Store(&Progress{})
	return s
}

// Configure takes c from now on. Any change asks the sources again: a key
// refused may now be the right one, a source down may be back.
func (s *Scraper) Configure(c Config) {
	c.Language = cmp.Or(c.Language, "en")
	s.config.Store(&c)
	s.mu.Lock()
	clear(s.sources)
	s.mu.Unlock()
	s.Kick()
}

// Kick asks for a pass, without waiting: after a scan, an edit or a match
// that may have given an artist an MBID.
func (s *Scraper) Kick() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Want asks for the artist with public ID id at a person's priority, without
// waiting: someone opened it with nothing yet. Its art comes in on the
// artists event. A source not due for it isn't asked.
func (s *Scraper) Want(id string) {
	if !s.config.Load().Remote {
		return
	}
	s.mu.Lock()
	if !slices.Contains(s.wanted, id) {
		s.wanted = append(s.wanted, id)
	}
	s.mu.Unlock()
	s.Kick()
}

// Progress returns where the pass stands. It never waits.
func (s *Scraper) Progress() Progress { return *s.progress.Load() }

// idleWait is the longest the pass sleeps with nothing due: the day the
// latest feed is read again.
const idleWait = 24 * time.Hour

// Run runs the pass at once, again whenever it is kicked or an artist falls
// due, until ctx ends. It is the app's background work.
func (s *Scraper) Run(ctx context.Context) {
	for {
		next := s.Pass(ctx)
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(max(next.Sub(s.now()), time.Second))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.wake:
		case <-timer.C:
		}
		timer.Stop()
	}
}

// Pass asks about every artist due, people's first, and returns when the
// next is due.
func (s *Scraper) Pass(ctx context.Context) time.Time {
	cfg := s.config.Load()
	if !cfg.Remote || !s.anyOn(cfg) {
		s.takeWanted() // Nothing is asked while lookups are off, so nothing waits for it.
		return s.now().Add(idleWait)
	}
	if err := s.waitScan(ctx); err != nil {
		return s.now()
	}
	s.readFeed(ctx, cfg)
	artists, err := s.o.Store.ScrapeArtists(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.log.ErrorContext(ctx, "artist pass: reading artists failed", "err", err)
		}
		return s.now().Add(time.Hour)
	}
	if !s.work(ctx, cfg, s.queue(cfg, artists)) {
		return s.now()
	}
	return s.nextDue(cfg, artists)
}

// work asks about each artist in queue, people's first whenever they open
// one, and reports whether it got to the end.
func (s *Scraper) work(ctx context.Context, cfg *Config, queue []store.ScrapeArtist) bool {
	defer s.progress.Store(&Progress{})
	asked := map[int64]bool{} // People's, asked already this pass.
	for i := range queue {
		if s.servePeople(ctx, cfg, asked) != nil || s.waitScan(ctx) != nil {
			return false
		}
		a := &queue[i]
		if asked[a.ID] {
			continue
		}
		s.progress.Store(&Progress{Running: true, Done: i, Total: len(queue), Artist: a.ULID, Name: a.Name})
		s.artist(ctx, cfg, a, true)
	}
	return s.servePeople(ctx, cfg, asked) == nil
}

// takeWanted returns the artists people opened, and forgets them.
func (s *Scraper) takeWanted() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.wanted
	s.wanted = nil
	return w
}

// waitScan waits while a scan holds the claim.
func (s *Scraper) waitScan(ctx context.Context) error {
	if s.o.Library == nil {
		return ctx.Err() //nolint:wrapcheck // ctx's own error, as a wait's.
	}
	return s.o.Library.WaitScanFree(ctx) //nolint:wrapcheck // The scanner's wait says what it waited for.
}

// servePeople asks about the artists people opened, at their priority, and
// notes each in asked.
func (s *Scraper) servePeople(ctx context.Context, cfg *Config, asked map[int64]bool) error {
	for _, id := range s.takeWanted() {
		if err := ctx.Err(); err != nil {
			return err //nolint:wrapcheck // ctx's own.
		}
		a, err := s.o.Store.ScrapeArtistByID(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			s.log.WarnContext(ctx, "artist pass: reading an artist opened failed", "artist", id, "err", err)
			continue
		}
		if Askable(a.MBID) && !asked[a.ID] {
			s.artist(ctx, cfg, &a, false)
			asked[a.ID] = true
		}
	}
	return nil
}

// queue is every artist with something due, album artists first by everyone's
// plays, then the rest.
func (s *Scraper) queue(cfg *Config, artists []store.ScrapeArtist) []store.ScrapeArtist {
	now := s.now()
	var q []store.ScrapeArtist
	for i := range artists {
		if a := &artists[i]; Askable(a.MBID) && len(s.dueSources(cfg, a, now)) > 0 {
			q = append(q, *a)
		}
	}
	slices.SortStableFunc(q, func(a, b store.ScrapeArtist) int {
		if a.Owner != b.Owner {
			if a.Owner {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(b.Plays, a.Plays), cmp.Compare(a.ID, b.ID))
	})
	return q
}

// nextDue is when the next artist falls due, or a day from now.
func (s *Scraper) nextDue(cfg *Config, artists []store.ScrapeArtist) time.Time {
	now := s.now()
	next := now.Add(idleWait)
	for i := range artists {
		a := &artists[i]
		if !Askable(a.MBID) {
			continue
		}
		for _, src := range s.usable(cfg, now) {
			// One due before this pass has been asked since, so only what
			// waits counts; an artist never asked was asked by it.
			if st, ok := a.States[src]; ok && st.NextAt.After(now) && st.NextAt.Before(next) {
				next = st.NextAt
			}
		}
	}
	s.mu.Lock()
	for _, h := range s.sources {
		if h.restUntil.After(now) && h.restUntil.Before(next) {
			next = h.restUntil
		}
	}
	s.mu.Unlock()
	return next
}
