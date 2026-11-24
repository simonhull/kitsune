package scrape

import (
	"context"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

// SourceState says how a source stands, in one word a card words.
type SourceState string

// How a source may stand.
const (
	SourceOn       SourceState = "on"       // Asked as artists fall due.
	SourceOff      SourceState = "off"      // Its switch, or the master switch, is off.
	SourceNeedsKey SourceState = "needsKey" // On, but fanart.tv has no project key.
	SourceRefused  SourceState = "refused"  // It refused the key: asked no more until a setting changes.
	SourceResting  SourceState = "resting"  // It was down or busy: asked again at Until.
)

// KeyState says whether a key is set, never what it is.
type KeyState string

// What a key may be.
const (
	KeyNone    KeyState = "none"    // The source needs no key.
	KeySet     KeyState = "set"     // Set, never shown.
	KeyMissing KeyState = "missing" // Not set.
)

// SourceReport is how one source stands.
type SourceReport struct {
	Name      string // "musicbrainz", "wikipedia" or "fanarttv".
	Enabled   bool   // Its switch, under the master switch.
	State     SourceState
	Key       KeyState // Its project key.
	ClientKey KeyState // fanart.tv's personal key; KeyNone elsewhere.
	Until     time.Time
	Failure   *Failure // The last, or nil.
}

// Report is what Health's Metadata card shows.
type Report struct {
	Remote   bool
	Language string
	Pass     Progress
	Sources  []SourceReport
	// Album artists: all, with an MBID the pass may ask about, those whose
	// every source on has answered, those with art from anywhere, those every
	// source answered with nothing, and those with no MBID, which Matching
	// gives one.
	AlbumArtists, WithMBID, LookedUp, WithArt, FoundNothing, WithoutMBID int
	// NextRefresh is when the next artist falls due, zero when none waits.
	NextRefresh time.Time
}

// Report says how the pass and its sources stand, and counts the library's
// album artists by what the pass has of them.
func (s *Scraper) Report(ctx context.Context) (Report, error) {
	cfg := s.config.Load()
	now := s.now()
	artists, err := s.o.Store.ScrapeArtists(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("artist pass report: %w", err)
	}
	r := Report{Remote: cfg.Remote, Language: cfg.Language, Pass: s.Progress(), Sources: s.sourceReports(cfg, now)}
	on := s.usable(cfg, now)
	for i := range artists {
		a := &artists[i]
		if !a.Owner {
			continue
		}
		r.AlbumArtists++
		if a.Arted {
			r.WithArt++
		}
		if !Askable(a.MBID) {
			if a.MBID == "" {
				r.WithoutMBID++
			}
			continue
		}
		r.WithMBID++
		r.count(a, on, now)
	}
	return r, nil
}

// count adds a, an album artist the pass may ask about, to the counts.
func (r *Report) count(a *store.ScrapeArtist, on []store.ScrapeSource, now time.Time) {
	answered, nothing := len(a.States) > 0, len(a.States) > 0
	for _, src := range on {
		if _, ok := a.States[src]; !ok {
			answered = false
		}
	}
	for _, st := range a.States {
		nothing = nothing && st.Status == store.ScrapeNone
		r.refreshAt(st.NextAt, now)
	}
	if answered {
		r.LookedUp++
	}
	if nothing && !a.Arted {
		r.FoundNothing++
	}
}

// refreshAt takes next as the next refresh when it is the soonest after now.
func (r *Report) refreshAt(next, now time.Time) {
	if next.After(now) && (r.NextRefresh.IsZero() || next.Before(r.NextRefresh)) {
		r.NextRefresh = next
	}
}

// sourceReports says how each source stands under cfg.
func (s *Scraper) sourceReports(cfg *Config, now time.Time) []SourceReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	mb := SourceReport{Name: "musicbrainz", Enabled: cfg.Wikipedia, Key: KeyNone, ClientKey: KeyNone}
	wiki := SourceReport{Name: "wikipedia", Enabled: cfg.Wikipedia, Key: KeyNone, ClientKey: KeyNone}
	fan := SourceReport{Name: "fanarttv", Enabled: cfg.Fanart, Key: keyState(cfg.Keys.API), ClientKey: keyState(cfg.Keys.Client)}
	wiki.State, wiki.Until, wiki.Failure = s.stateOf(store.ScrapeWikimedia, cfg.Remote && s.wikimediaOn(cfg), now)
	mb.State = SourceOff
	if cfg.Remote && s.wikimediaOn(cfg) {
		mb.State = SourceOn
	}
	fan.State, fan.Until, fan.Failure = s.stateOf(store.ScrapeFanart, cfg.Remote && s.fanartOn(cfg), now)
	if cfg.Remote && cfg.Fanart && s.o.Fanart != nil && cfg.Keys.API == "" {
		fan.State = SourceNeedsKey
	}
	return []SourceReport{mb, wiki, fan}
}

// stateOf is how src stands when on says whether it is switched on. The
// caller holds mu.
func (s *Scraper) stateOf(src store.ScrapeSource, on bool, now time.Time) (SourceState, time.Time, *Failure) {
	h := s.sources[src]
	var f *Failure
	if h != nil {
		f = h.failure
	}
	switch {
	case !on:
		return SourceOff, time.Time{}, f
	case h != nil && h.refused:
		return SourceRefused, time.Time{}, f
	case h != nil && h.restUntil.After(now):
		return SourceResting, h.restUntil, f
	}
	return SourceOn, time.Time{}, f
}

func keyState(key string) KeyState {
	if key == "" {
		return KeyMissing
	}
	return KeySet
}
