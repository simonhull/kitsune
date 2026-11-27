package scrape

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/remote"
	"github.com/simonhull/kitsune/internal/store"
)

// The kinds of an artist's image, in the order they're decided.
func imageKinds() []store.ArtistImageKind {
	return []store.ArtistImageKind{store.ArtistThumb, store.ArtistBackground, store.ArtistLogo, store.ArtistBanner}
}

// artworkKind is the ladder each kind is rendered on.
func artworkKind(k store.ArtistImageKind) artwork.Kind {
	switch k {
	case store.ArtistBackground:
		return artwork.Background
	case store.ArtistLogo:
		return artwork.Logo
	case store.ArtistBanner:
		return artwork.Banner
	case store.ArtistThumb:
	}
	return artwork.Thumb
}

// found is what one source answered about one artist.
type found struct {
	source  store.ScrapeSource
	asked   bool // Asked and answered, with art or with none.
	status  store.ScrapeStatus
	failure *Failure // Why it failed, when it did.
	skipped bool     // Not asked: nothing it gives was wanted. Counts as asked for its state.
	images  map[store.ArtistImageKind][]Candidate
	bio     *store.Bio
}

// artist asks every source due about a, chooses, downloads what it chose,
// and stores it all in one write. background marks the requests as work done
// ahead; a person's wait goes first.
func (s *Scraper) artist(ctx context.Context, cfg *Config, a *store.ScrapeArtist, background bool) {
	now := s.now()
	due := s.dueSources(cfg, a, now)
	if len(due) == 0 {
		return
	}
	held, err := s.o.Store.ArtistHeld(ctx, a.ID)
	if err != nil {
		s.logFailure(ctx, "reading what an artist holds failed", a, err)
		return
	}
	ask := ctx
	if background {
		ask = remote.Background(ctx)
	}
	answers := s.askAll(ask, cfg, due, a.MBID, &held)
	if ctx.Err() != nil {
		return
	}
	w := s.decide(ask, cfg, a, &held, answers)
	if ctx.Err() != nil {
		return
	}
	changed, err := s.o.Store.WriteRemote(ctx, w, s.now())
	if err != nil {
		s.logFailure(ctx, "storing an artist's art failed", a, err)
		return
	}
	if changed && s.o.Notify != nil {
		s.o.Notify([]string{a.ULID})
	}
}

func (s *Scraper) logFailure(ctx context.Context, msg string, a *store.ScrapeArtist, err error) {
	if ctx.Err() == nil {
		s.log.LogAttrs(ctx, slog.LevelError, "artist pass: "+msg, slog.String("artist", a.ULID), slog.Any("err", err))
	}
}

// askAll asks each source in due about the artist mbid, in order: Last.fm
// last, for a bio the others left wanting.
func (s *Scraper) askAll(ctx context.Context, cfg *Config, due []store.ScrapeSource, mbid string, held *store.HeldArt) []*found {
	answers := make([]*found, 0, len(due))
	for _, src := range due {
		if src == store.ScrapeLastfm {
			answers = append(answers, s.askLastfm(ctx, cfg, mbid, held, answers))
			continue
		}
		answers = append(answers, s.ask(ctx, cfg, src, mbid, held))
	}
	return answers
}

// ask asks src about the artist mbid, for what held leaves to fill.
func (s *Scraper) ask(ctx context.Context, cfg *Config, src store.ScrapeSource, mbid string, held *store.HeldArt) *found {
	f := &found{source: src, images: map[store.ArtistImageKind][]Candidate{}}
	var err error
	switch src {
	case store.ScrapeFanart:
		if !free(held, store.ArtistThumb, store.ArtistBackground, store.ArtistLogo, store.ArtistBanner) {
			f.asked, f.status = true, store.ScrapeFound // The folders hold it all: nothing to ask.
			return f
		}
		err = s.askFanartKept(ctx, cfg, mbid, f)
	case store.ScrapeWikimedia:
		if !free(held, store.ArtistThumb) && !freeBio(held) {
			f.asked, f.status = true, store.ScrapeFound
			return f
		}
		err = s.askWikimediaKept(ctx, cfg, mbid, held, f)
	case store.ScrapeLastfm, store.ScrapeArchive, store.ScrapeMusicBrainz: // Asked elsewhere, or of albums.
	}
	s.settle(ctx, f, err)
	return f
}

// askFanartKept asks fanart.tv about mbid into f, and keeps what it offers for
// Choose artwork, which then asks nothing.
func (s *Scraper) askFanartKept(ctx context.Context, cfg *Config, mbid string, f *found) error {
	err := s.askFanart(ctx, cfg, mbid, f)
	if err == nil || errors.Is(err, remote.ErrNotFound) {
		s.keepOffers(ctx, store.ScrapeFanart, mbid, f.images)
	}
	return err
}

// askWikimediaKept asks Wikimedia about mbid into f, and keeps the portrait it
// offers for Choose artwork when it was asked for one.
func (s *Scraper) askWikimediaKept(ctx context.Context, cfg *Config, mbid string, held *store.HeldArt, f *found) error {
	err := s.askWikimedia(ctx, cfg, mbid, held, f)
	if err == nil && free(held, store.ArtistThumb) {
		s.keepOffers(ctx, store.ScrapeWikimedia, mbid, map[store.ArtistImageKind][]Candidate{store.ArtistThumb: f.images[store.ArtistThumb]})
	}
	return err
}

// settle records how an answer went: found or none, a failure for the artist,
// or a refusal or an outage for the whole source.
func (s *Scraper) settle(ctx context.Context, f *found, err error) {
	now := s.now()
	switch {
	case err == nil:
		f.asked, f.status = true, store.ScrapeFound
		if len(f.images) == 0 && f.bio == nil {
			f.status = store.ScrapeNone
		}
		s.healthy(f.source)
	case errors.Is(err, remote.ErrNotFound):
		f.asked, f.status = true, store.ScrapeNone
		s.healthy(f.source)
	case ctx.Err() != nil:
	case errors.Is(err, remote.ErrRefused):
		s.refuse(ctx, f.source, describe(f.source, err, now))
	default:
		f.status, f.failure = store.ScrapeFailed, describe(f.source, err, now)
		if errors.Is(err, remote.ErrUnavailable) {
			s.restSource(ctx, f.source, f.failure, err)
		} else {
			s.noteFailure(f.source, f.failure)
		}
	}
}

// free reports whether any of kinds is free for a source to fill: no folder's
// image and no pin.
func free(held *store.HeldArt, kinds ...store.ArtistImageKind) bool {
	for _, k := range kinds {
		img, ok := held.Images[k]
		if !ok || (img.Source != fromFolder && img.Source != fromPinned) {
			return true
		}
	}
	return false
}

// freeBio reports whether the bio is free for a source to fill: no artist.nfo's.
func freeBio(held *store.HeldArt) bool { return held.Bio == nil || held.Bio.Source != fromNfo }

// decide chooses each kind and the bio from the answers, downloads what it
// chose, and returns the write.
func (s *Scraper) decide(ctx context.Context, cfg *Config, a *store.ScrapeArtist, held *store.HeldArt, answers []*found) *store.RemoteWrite {
	w := &store.RemoteWrite{ArtistID: a.ID, MBID: a.MBID}
	for _, kind := range imageKinds() {
		s.decideKind(ctx, cfg, kind, held, answers, w)
	}
	bd := DecideBio(held.Bio, bioAnswers(answers))
	switch {
	case bd.Set != nil:
		w.Bios = append(w.Bios, store.RemoteBio{Source: bd.From, Bio: bd.Set})
	case bd.Clear != "":
		w.Bios = append(w.Bios, store.RemoteBio{Source: bd.Clear})
	}
	w.Answers = s.answerStates(a, answers)
	return w
}

// decideKind adds what the answers decide of kind to w, fetching an image
// chosen.
func (s *Scraper) decideKind(ctx context.Context, cfg *Config, kind store.ArtistImageKind, held *store.HeldArt, answers []*found, w *store.RemoteWrite) {
	var h *store.HeldImage
	if img, ok := held.Images[kind]; ok {
		h = &img
	}
	d := Decide(h, imageAnswers(kind, answers), cfg.Language)
	switch {
	case d.Set != nil:
		if img := s.fetch(ctx, kind, d.Set, answers); img != nil {
			w.Images = append(w.Images, *img)
		}
	case d.Clear != "":
		w.Images = append(w.Images, store.RemoteImage{Kind: kind, Source: d.Clear})
		w.AskAgain = append(w.AskAgain, otherSources(d.Clear)...)
	}
}

// answerStates are what each source's answer leaves of its state: a source
// refused, resting or stopped midway leaves its state as it was.
func (s *Scraper) answerStates(a *store.ScrapeArtist, answers []*found) []store.ScrapeAnswer {
	var out []store.ScrapeAnswer
	for _, f := range answers {
		if !f.asked && f.status != store.ScrapeFailed {
			continue
		}
		msg := ""
		if f.failure != nil {
			msg = f.failure.Words
		}
		out = append(out, store.ScrapeAnswer{Source: f.source, State: NextAsk(f.status, stateOf(a, f.source), a.MBID, msg, s.now())})
	}
	return out
}

// fetch downloads and renders c, the image chosen for kind, or returns nil
// when that fails: the source then counts as failed for this artist, and what
// the artist has stays.
func (s *Scraper) fetch(ctx context.Context, kind store.ArtistImageKind, c *Candidate, answers []*found) *store.RemoteImage {
	if s.o.Artwork == nil {
		return nil
	}
	f := answerOf(answers, imageSource(c.Source))
	body, err := s.download(ctx, c)
	if err == nil {
		var img *artwork.Image
		if img, err = s.o.Artwork.Put(ctx, artworkKind(kind), body); err == nil {
			return &store.RemoteImage{
				Kind: kind, Source: c.Source, RemoteID: c.ID, Credit: c.Credit,
				Image: &store.ArtworkWrite{
					Hash: img.Hash, Width: img.Width, Height: img.Height, Accent: img.Accent, Ground: img.Ground,
					Source: store.ArtworkRemote, Alpha: img.Alpha,
				},
			}
		}
	}
	if f != nil && ctx.Err() == nil {
		f.status, f.failure = store.ScrapeFailed, describe(f.source, err, s.now())
		f.asked = false
		s.noteFailure(f.source, f.failure)
	}
	return nil
}

// download fetches c from its source.
func (s *Scraper) download(ctx context.Context, c *Candidate) ([]byte, error) {
	switch c.Source {
	case store.ImageFromFanart:
		return s.o.Fanart.Download(ctx, c.URL) //nolint:wrapcheck // It names fanart.tv.
	case store.ImageFromWikimedia:
		return s.o.Wikimedia.Download(ctx, c.URL) //nolint:wrapcheck // It names Commons.
	}
	return nil, fmt.Errorf("%w: %s", errUnknownSource, c.Source)
}

// errUnknownSource is a candidate from no source the pass knows: a bug.
var errUnknownSource = errors.New("no such source")

// stateOf is what src last said of a, or nil.
func stateOf(a *store.ScrapeArtist, src store.ScrapeSource) *store.ScrapeState {
	if st, ok := a.States[src]; ok {
		return &st
	}
	return nil
}

// answerOf is src's answer, or nil.
func answerOf(answers []*found, src store.ScrapeSource) *found {
	for _, f := range answers {
		if f.source == src {
			return f
		}
	}
	return nil
}

// imageSource is the source an image source name belongs to.
func imageSource(name string) store.ScrapeSource {
	if name == store.ImageFromFanart {
		return store.ScrapeFanart
	}
	return store.ScrapeWikimedia
}

// otherSources are the sources besides the one whose image went, which may
// have one of the kind: asked again at once.
func otherSources(name string) []store.ScrapeSource {
	if name == store.ImageFromFanart {
		return []store.ScrapeSource{store.ScrapeWikimedia}
	}
	return []store.ScrapeSource{store.ScrapeFanart}
}

// imageAnswers are the answers about kind, in precedence order: fanart.tv,
// then Commons. A source not asked this time is in it unanswered.
func imageAnswers(kind store.ArtistImageKind, answers []*found) []Answer {
	order := []struct {
		src  store.ScrapeSource
		name string
	}{{store.ScrapeFanart, store.ImageFromFanart}}
	if kind == store.ArtistThumb {
		order = append(order, struct {
			src  store.ScrapeSource
			name string
		}{store.ScrapeWikimedia, store.ImageFromWikimedia})
	}
	out := make([]Answer, 0, len(order))
	for _, o := range order {
		a := Answer{Source: o.name}
		if f := answerOf(answers, o.src); f != nil && f.asked {
			a.Answered, a.Candidates = true, f.images[kind]
		}
		out = append(out, a)
	}
	return out
}

// bioAnswers are the answers about the bio, in precedence order: Wikipedia's,
// then Last.fm's. Last.fm not asked because another bio stands is no answer.
func bioAnswers(answers []*found) []BioAnswer {
	out := make([]BioAnswer, 0, 2) //nolint:mnd // The two sources of a bio.
	for _, o := range []struct {
		src  store.ScrapeSource
		name string
	}{{store.ScrapeWikimedia, store.BioFromWikipedia}, {store.ScrapeLastfm, store.BioFromLastfm}} {
		a := BioAnswer{Source: o.name}
		if f := answerOf(answers, o.src); f != nil && f.asked && !f.skipped {
			a.Answered, a.Bio = true, f.bio
		}
		out = append(out, a)
	}
	return out
}

// dueSources are the sources to ask about a now: on, not refused or resting,
// and due.
func (s *Scraper) dueSources(cfg *Config, a *store.ScrapeArtist, now time.Time) []store.ScrapeSource {
	var out []store.ScrapeSource
	for _, src := range s.usable(cfg, now, artistSources()) {
		if Due(stateOf(a, src), now) {
			out = append(out, src)
		}
	}
	return out
}
