package scrape

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/remote"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv"
	"github.com/simonhull/kitsune/internal/scrape/wikimedia"
	"github.com/simonhull/kitsune/internal/store"
)

// How long a source that was down or busy rests: a minute, doubling to an
// hour, unless it said how long.
const (
	firstRest = time.Minute
	longRest  = time.Hour
)

// portraitWidth is the width a Commons portrait is fetched at: the thumb
// ladder's largest rung.
const portraitWidth = 1024

// What Wikipedia's text is under, and where its license is.
const (
	wikipediaLicense    = "CC BY-SA 4.0"
	wikipediaLicenseURL = "https://creativecommons.org/licenses/by-sa/4.0/"
)

// sourceName is a source as a person reads it.
func sourceName(src store.ScrapeSource) string {
	switch src {
	case store.ScrapeFanart:
		return "fanart.tv"
	case store.ScrapeLastfm:
		return "Last.fm"
	case store.ScrapeArchive:
		return "The Cover Art Archive"
	case store.ScrapeMusicBrainz:
		return "MusicBrainz"
	case store.ScrapeWikimedia:
	}
	return "Wikipedia"
}

// artistSources are the sources asked about an artist, in the order they're
// asked: Last.fm last, since it is asked only for a bio no one else gave.
func artistSources() []store.ScrapeSource {
	return []store.ScrapeSource{store.ScrapeFanart, store.ScrapeWikimedia, store.ScrapeLastfm}
}

// albumSources are the sources asked about an album, in the order they're
// asked: MusicBrainz first, since its release group is what fanart.tv is asked by.
func albumSources() []store.ScrapeSource {
	return []store.ScrapeSource{store.ScrapeMusicBrainz, store.ScrapeArchive, store.ScrapeFanart}
}

// anyOn reports whether cfg turns on any source the pass has a client for.
func (s *Scraper) anyOn(cfg *Config) bool {
	return s.fanartOn(cfg) || s.wikimediaOn(cfg) || s.lastfmOn(cfg) || s.archiveOn(cfg)
}

// on reports whether cfg turns src on, with a client for it.
func (s *Scraper) on(cfg *Config, src store.ScrapeSource) bool {
	switch src {
	case store.ScrapeFanart:
		return s.fanartOn(cfg)
	case store.ScrapeWikimedia:
		return s.wikimediaOn(cfg)
	case store.ScrapeLastfm:
		return s.lastfmOn(cfg)
	case store.ScrapeArchive:
		return s.archiveOn(cfg)
	case store.ScrapeMusicBrainz:
		return s.groupsOn(cfg)
	}
	return false
}

func (s *Scraper) lastfmOn(cfg *Config) bool { return cfg.LastfmBios && cfg.Lastfm != nil }

func (s *Scraper) archiveOn(cfg *Config) bool { return cfg.Archive && s.o.MusicBrainz != nil }

// groupsOn reports whether albums' release groups are looked up: only
// fanart.tv's album art is asked for by one.
func (s *Scraper) groupsOn(cfg *Config) bool { return s.fanartOn(cfg) && s.o.MusicBrainz != nil }

func (s *Scraper) fanartOn(cfg *Config) bool {
	return cfg.Fanart && s.o.Fanart != nil && cfg.Keys.API != ""
}

func (s *Scraper) wikimediaOn(cfg *Config) bool {
	return cfg.Wikipedia && s.o.Wikimedia != nil && s.o.MusicBrainz != nil
}

// usable are those of sources on in cfg that neither refused nor rest at now.
func (s *Scraper) usable(cfg *Config, now time.Time, sources []store.ScrapeSource) []store.ScrapeSource {
	var out []store.ScrapeSource
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, src := range sources {
		if h := s.sources[src]; s.on(cfg, src) && (h == nil || (!h.refused && !h.restUntil.After(now))) {
			out = append(out, src)
		}
	}
	return out
}

// healthOf returns src's health, making it. The caller holds mu.
func (s *Scraper) healthOf(src store.ScrapeSource) *health {
	h := s.sources[src]
	if h == nil {
		h = &health{}
		s.sources[src] = h
	}
	return h
}

// healthy notes that src answered: its rest starts over.
func (s *Scraper) healthy(src store.ScrapeSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if h := s.sources[src]; h != nil {
		h.rest = 0
	}
}

// refuse stops asking src until a setting changes.
func (s *Scraper) refuse(ctx context.Context, src store.ScrapeSource, f *Failure) {
	s.mu.Lock()
	h := s.healthOf(src)
	first := !h.refused
	h.refused, h.failure = true, f
	s.mu.Unlock()
	if first {
		s.log.WarnContext(ctx, "artist pass: a source refused Kitsune: asking it no more until a setting changes",
			"source", string(src), "why", f.Words)
	}
}

// restSource rests src after it was down or busy: as long as it asked, else a
// minute, doubling up to an hour.
func (s *Scraper) restSource(ctx context.Context, src store.ScrapeSource, f *Failure, err error) {
	s.mu.Lock()
	h := s.healthOf(src)
	h.rest = min(max(h.rest*2, firstRest), longRest) //nolint:mnd // Doubling.
	wait := h.rest
	if se, ok := errors.AsType[*remote.StatusError](err); ok && se.RetryAfter > 0 {
		wait = se.RetryAfter
	}
	h.restUntil, h.failure = f.At.Add(wait), f
	s.mu.Unlock()
	s.log.InfoContext(ctx, "artist pass: a source is down or busy: resting it", "source", string(src), "why", f.Words, "for", wait.String())
}

// noteFailure keeps f as src's last failure.
func (s *Scraper) noteFailure(src store.ScrapeSource, f *Failure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthOf(src).failure = f
}

// describe says what err means for src in words, never quoting err itself,
// which could hold more than a person needs.
func describe(src store.ScrapeSource, err error, at time.Time) *Failure {
	f := &Failure{At: at, Words: sentinelWords(src, err)}
	if se, ok := errors.AsType[*remote.StatusError](err); ok {
		f.Status = se.Status
	}
	name := sourceName(src)
	switch {
	case f.Words != "":
	case f.Status == http.StatusTooManyRequests:
		f.Words = name + " asked Kitsune to slow down"
	case f.Status != 0:
		f.Words = fmt.Sprintf("%s answered %d", name, f.Status)
	case src != store.ScrapeMusicBrainz && (errors.Is(err, matching.ErrUnavailable) || errors.Is(err, matching.ErrRejected)):
		f.Words = "MusicBrainz didn't answer"
	default:
		f.Words = name + " didn't answer"
	}
	return f
}

// sentinelWords says what err means when one of the clients' sentinels does,
// else "".
func sentinelWords(src store.ScrapeSource, err error) string {
	name := sourceName(src)
	switch {
	case errors.Is(err, fanarttv.ErrNoKey):
		return name + " needs a project key"
	case errors.Is(err, remote.ErrRefused) && (src == store.ScrapeFanart || src == store.ScrapeLastfm):
		return name + " refused the key"
	case errors.Is(err, remote.ErrRefused):
		return name + " refused Kitsune's requests"
	case errors.Is(err, fanarttv.ErrNotALink):
		return name + " sent file names, not links"
	case errors.Is(err, fanarttv.ErrBadAnswer), errors.Is(err, wikimedia.ErrBadAnswer), errors.Is(err, matching.ErrBadArt):
		return name + " sent an answer Kitsune can't read"
	case errors.Is(err, remote.ErrTooLarge):
		return name + " sent more than Kitsune takes"
	}
	return ""
}

// askFanart asks fanart.tv about mbid into f.
func (s *Scraper) askFanart(ctx context.Context, cfg *Config, mbid string, f *found) error {
	a, err := s.o.Fanart.Artist(ctx, cfg.Keys, mbid)
	if err != nil {
		return err //nolint:wrapcheck // It names fanart.tv; settle reads its sentinel.
	}
	put := func(kind store.ArtistImageKind, tier int, imgs []fanarttv.Image) {
		for _, img := range imgs {
			f.images[kind] = append(f.images[kind], Candidate{
				Source: store.ImageFromFanart, ID: img.ID, URL: img.URL, Lang: img.Lang, Likes: img.Likes,
				Width: img.Width, Height: img.Height, Tier: tier,
			})
		}
	}
	put(store.ArtistThumb, 0, a.Thumbs)
	put(store.ArtistBackground, 0, a.Backgrounds4K)
	put(store.ArtistBackground, 1, a.Backgrounds)
	put(store.ArtistLogo, 0, a.HDLogos)
	put(store.ArtistLogo, 1, a.Logos)
	put(store.ArtistBanner, 0, a.Banners)
	return nil
}

// askWikimedia finds mbid's Wikidata item through MusicBrainz, then its
// article's lead in the language wanted, else English, and its portrait with
// its credit, into f.
func (s *Scraper) askWikimedia(ctx context.Context, cfg *Config, mbid string, held *store.HeldArt, f *found) error {
	links, err := s.o.MusicBrainz.ArtistLinks(ctx, mbid)
	if errors.Is(err, matching.ErrNotFound) || (err == nil && links.Wikidata == "") {
		return nil // No item: nothing to find.
	}
	if err != nil {
		return err //nolint:wrapcheck // describe words MusicBrainz's.
	}
	if freeBio(held) {
		if f.bio, err = s.wikipediaBio(ctx, cfg, links.Wikidata); err != nil {
			return err
		}
	}
	if free(held, store.ArtistThumb) {
		c, perr := s.portrait(ctx, links.Wikidata)
		if perr != nil {
			return perr
		}
		if c != nil {
			f.images[store.ArtistThumb] = []Candidate{*c}
		}
	}
	return nil
}

// wikipediaBio is the lead of the item's article in the language wanted, else
// English, or nil when it has neither.
func (s *Scraper) wikipediaBio(ctx context.Context, cfg *Config, item string) (*store.Bio, error) {
	langs := []string{cfg.Language}
	if cfg.Language != "en" {
		langs = append(langs, "en")
	}
	titles, err := s.o.Wikimedia.Articles(ctx, item, langs)
	if errors.Is(err, remote.ErrNotFound) {
		return nil, nil //nolint:nilnil // No item, no bio: both say none.
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // It names Wikidata.
	}
	for _, lang := range langs {
		title, ok := titles[lang]
		if !ok {
			continue
		}
		sum, serr := s.o.Wikimedia.Summary(ctx, lang, title)
		if errors.Is(serr, remote.ErrNotFound) {
			continue
		}
		if serr != nil {
			return nil, serr //nolint:wrapcheck // It names Wikipedia.
		}
		return bioOf(&sum), nil
	}
	return nil, nil //nolint:nilnil // As above.
}

// bioOf is a summary as a bio: its first paragraph the summary, all of them
// the body, credited to Wikipedia under its license.
func bioOf(sum *wikimedia.Summary) *store.Bio {
	var paras []string
	for line := range strings.SplitSeq(sum.Extract, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paras = append(paras, line)
		}
	}
	return &store.Bio{
		Source: store.BioFromWikipedia, Lang: sum.Lang, Summary: cmp.Or(firstOf(paras), sum.Extract),
		Body: strings.Join(paras, "\n\n"), URL: sum.PageURL, License: wikipediaLicense, LicenseURL: wikipediaLicenseURL,
	}
}

func firstOf(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// portrait is the item's image on Commons as a candidate, with its credit, or
// nil when it has none.
func (s *Scraper) portrait(ctx context.Context, item string) (*Candidate, error) {
	name, err := s.o.Wikimedia.Portrait(ctx, item)
	if err != nil || name == "" {
		return nil, err //nolint:wrapcheck // It names Wikidata.
	}
	file, err := s.o.Wikimedia.File(ctx, name, portraitWidth)
	if errors.Is(err, remote.ErrNotFound) {
		return nil, nil //nolint:nilnil // A file Commons lacks is no portrait.
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // It names Commons.
	}
	return &Candidate{
		Source: store.ImageFromWikimedia, ID: name, URL: file.URL, Width: file.Width, Height: file.Height,
		Credit: &store.Credit{Author: file.Author, License: file.License, LicenseURL: file.LicenseURL, PageURL: file.PageURL},
	}, nil
}

// feedEvery is how often fanart.tv's feed of artists with new images is read.
const feedEvery = 24 * time.Hour

// readFeed reads fanart.tv's latest feed once a day, and asks fanart.tv again
// at once about every artist on it that the pass has asked about before.
func (s *Scraper) readFeed(ctx context.Context, cfg *Config) {
	now := s.now()
	since, due := s.feedDue(ctx, cfg, now)
	if !due {
		return
	}
	mbids, err := s.o.Fanart.Latest(remote.Background(ctx), cfg.Keys, since)
	if err != nil {
		if ctx.Err() == nil {
			s.settleFeed(ctx, err, now)
		}
		return
	}
	n, err := s.o.Store.AskAgain(ctx, []store.ScrapeSource{store.ScrapeFanart}, mbids, now)
	if err == nil {
		err = s.o.Store.SetScrapeFeedRead(ctx, store.ScrapeFanart, now)
	}
	if err != nil {
		s.log.WarnContext(ctx, "artist pass: keeping fanart.tv's feed failed", "err", err)
		return
	}
	s.log.InfoContext(ctx, "artist pass: read fanart.tv's feed of new art", "artists", len(mbids), "askedAgain", n)
}

// feedDue says whether fanart.tv's feed is due at now, and since when to read it.
func (s *Scraper) feedDue(ctx context.Context, cfg *Config, now time.Time) (time.Time, bool) {
	if !slices.Contains(s.usable(cfg, now, artistSources()), store.ScrapeFanart) {
		return time.Time{}, false
	}
	last, read, err := s.o.Store.ScrapeFeedRead(ctx, store.ScrapeFanart)
	if err != nil || (read && now.Sub(last) < feedEvery) {
		return time.Time{}, false
	}
	return cmp.Or(last, now.Add(-feedEvery)), true
}

// settleFeed notes why reading the feed failed.
func (s *Scraper) settleFeed(ctx context.Context, err error, now time.Time) {
	f := describe(store.ScrapeFanart, err, now)
	if errors.Is(err, remote.ErrRefused) {
		s.refuse(ctx, store.ScrapeFanart, f)
		return
	}
	s.noteFailure(store.ScrapeFanart, f)
}
