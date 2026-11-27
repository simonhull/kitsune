package scrape

import (
	"context"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/remote"
	"github.com/simonhull/kitsune/internal/store"
)

// Choose artwork (plan 072, Phase 4): an administrator's picker of an artist's
// images. It lists what the artist shows and what the sources offer, each with
// a preview the server fetched; it pins one, or an uploaded image, over what
// the folders and the sources give; it unpins; it asks the sources again now;
// and it holds an artist back when its MBID names someone else. Nothing here
// touches the library's files.

// Sentinels the picker's callers branch on.
var (
	// ErrNoCandidate means no candidate of the artist's kind has the ID asked.
	ErrNoCandidate = errors.New("no such candidate")
	// ErrNoArtwork means the server has no artwork cache to render into.
	ErrNoArtwork = errors.New("artist art isn't set up")
	// ErrLookupsOff means metadata.remote is off: no source is asked.
	ErrLookupsOff = errors.New("artist lookups are off")
	// ErrNoMBID means the artist has no MusicBrainz ID, by which alone the
	// sources are asked.
	ErrNoMBID = errors.New("the artist has no MusicBrainz ID")
	// ErrHeld means Not this artist holds the artist back.
	ErrHeld = errors.New("Not this artist holds the artist back") //nolint:staticcheck // ST1005: the action's name, as people read it.
	// ErrSourceFailed means the candidate's source didn't give the image.
	ErrSourceFailed = errors.New("the source didn't give the image")
)

// Where an offer came from, as Choose artwork names it.
const (
	OfferFolder    = store.PinFromFolder
	OfferFanart    = store.PinFromFanart
	OfferWikimedia = store.PinFromWikimedia
	OfferUpload    = store.PinFromUpload
)

// MostOffers is the most offers one kind lists.
const MostOffers = 24

// Offer is one image an artist could show, for Choose artwork.
type Offer struct {
	ID       string // Opaque: what Pin takes back.
	Source   string // One of the Offer names.
	RemoteID string // The source's own ID; "" for a folder's or an upload.
	Lang     string
	Credit   *store.Credit
	Preview  *store.Picture // Nil when it couldn't be fetched.
	// Width and Height are the image's, 0 when its source doesn't say.
	Width, Height int
	Chosen        bool // Shown now.
	Pinned        bool // Shown now because a pin chose it.
	PinnedBy      string
	PinnedAt      time.Time
	url           string // Where a source's image is fetched from.
}

// How a source stood for Choose artwork.
const (
	OfferAnswered    = "answered"
	OfferOff         = "off"
	OfferHeld        = "held"
	OfferUnavailable = "unavailable"
	OfferFailed      = "failed"
)

// OfferSource is how one source stood for an artist's offers.
type OfferSource struct {
	Source string // OfferFanart or OfferWikimedia.
	State  string
	Words  string // Why it failed, in words; never a key or a URL.
}

// Offers is everything Choose artwork lists for one artist and kind.
type Offers struct {
	Kind    store.ArtistImageKind
	MBID    string
	Pinned  bool
	Held    bool
	Offers  []Offer
	Sources []OfferSource
}

// Lookups the pass keeps of what the sources offer each artist, so the picker
// reads them without asking again, as matching's answers are kept.
const (
	keptFanart    = "scrape:fanarttv"
	keptWikimedia = "scrape:wikimedia"
	// offersFor is how long what a source offers is kept, and a preview.
	offersFor = 24 * time.Hour
	// previewWidth is the width a Commons portrait's preview is fetched at.
	previewWidth = 400
	// previewsAtOnce is how many previews one listing fetches at once; each
	// source's gate spaces them all the same.
	previewsAtOnce = 4
	// sweepEvery is how often previews past a day are swept.
	sweepEvery = time.Hour
)

// kept is what a source offered an artist, as it is kept in lookups.
type kept struct {
	Images map[store.ArtistImageKind][]Candidate
}

// keepOffers keeps what src offered the artist mbid, by kind, for the picker.
// It is a courtesy: a failure is logged, never the pass's.
func (s *Scraper) keepOffers(ctx context.Context, src store.ScrapeSource, mbid string, images map[store.ArtistImageKind][]Candidate) {
	body, err := json.Marshal(kept{Images: images})
	if err == nil {
		now := s.now()
		err = s.o.Store.PutLookup(ctx, keptName(src), mbid, body, now, now.Add(-matching.ArtistLinksFor))
	}
	if err != nil && ctx.Err() == nil {
		s.log.WarnContext(ctx, "artist pass: keeping what a source offers failed", "source", string(src), "err", err)
	}
}

// keptOffers returns what src offered the artist mbid within the day, and
// whether it is kept.
func (s *Scraper) keptOffers(ctx context.Context, src store.ScrapeSource, mbid string) (map[store.ArtistImageKind][]Candidate, bool) {
	body, err := s.o.Store.Lookup(ctx, keptName(src), mbid, s.now().Add(-offersFor))
	if err != nil {
		return nil, false
	}
	var k kept
	if json.Unmarshal(body, &k) != nil {
		return nil, false
	}
	return k.Images, true
}

func keptName(src store.ScrapeSource) string {
	if src == store.ScrapeFanart {
		return keptFanart
	}
	return keptWikimedia
}

// Candidates lists what the artist with public ID id could show of kind: its
// pin, what lies beneath it, then what each source offers, with previews. A
// source is asked, at a person's priority, only when what the pass kept is
// more than a day old. It returns an error wrapping [store.ErrNotFound] for an
// unknown artist.
func (s *Scraper) Candidates(ctx context.Context, id string, kind store.ArtistImageKind) (*Offers, error) {
	s.maybeSweep(ctx)
	a, err := s.o.Store.ScrapeArtistByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("candidates: %w", err)
	}
	out := &Offers{Kind: kind, MBID: a.MBID, Held: a.Held}
	err = s.ownOffers(ctx, &a, kind, out)
	if err != nil {
		return nil, err
	}
	for _, src := range s.kindSources(kind) {
		cands, st := s.offered(ctx, &a, src, kind)
		out.Sources = append(out.Sources, st)
		for i := range cands {
			o := offerOf(&cands[i])
			out.add(&o)
		}
	}
	if len(out.Offers) > MostOffers {
		out.Offers = out.Offers[:MostOffers]
	}
	s.previews(ctx, out.Offers)
	return out, nil
}

// kindSources are the sources that offer kind: fanart.tv every kind, Commons
// thumbs.
func (s *Scraper) kindSources(kind store.ArtistImageKind) []store.ScrapeSource {
	if kind == store.ArtistThumb {
		return []store.ScrapeSource{store.ScrapeFanart, store.ScrapeWikimedia}
	}
	return []store.ScrapeSource{store.ScrapeFanart}
}

// ownOffers adds what the artist shows of kind to out: its pin, then what lies
// beneath it, each with its own image as its preview.
func (s *Scraper) ownOffers(ctx context.Context, a *store.ScrapeArtist, kind store.ArtistImageKind, out *Offers) error {
	pin, err := s.o.Store.ArtistPin(ctx, a.ID, kind)
	if err != nil {
		return fmt.Errorf("candidates: %w", err)
	}
	if pin != nil {
		out.Pinned = true
		out.add(&Offer{
			ID: offerID(pin.Source, pin.RemoteID, pin.Hash), Source: pin.Source, RemoteID: pin.RemoteID,
			Credit: pin.Credit, Preview: &pin.Picture, Width: pin.Width, Height: pin.Height,
			Chosen: true, Pinned: true, PinnedBy: pin.By, PinnedAt: pin.At,
		})
	}
	b, err := s.o.Store.ArtistImageBeneath(ctx, a.ID, kind)
	if err != nil {
		return fmt.Errorf("candidates: %w", err)
	}
	if b != nil {
		pic := b.Picture
		out.add(&Offer{
			ID: offerID(b.Source, b.RemoteID, b.Hash), Source: b.Source, RemoteID: b.RemoteID, Credit: b.Credit,
			Preview: &pic, Width: b.Width, Height: b.Height, Chosen: pin == nil,
		})
	}
	return nil
}

// add appends o to the offers, unless one with its ID is there already: then
// that one keeps its place, its preview and whether it is chosen, and takes
// what o says of its language and credit.
func (out *Offers) add(o *Offer) {
	for i := range out.Offers {
		if have := &out.Offers[i]; have.ID == o.ID {
			have.Lang = o.Lang
			if have.Credit == nil {
				have.Credit = o.Credit
			}
			have.url = o.url
			return
		}
	}
	out.Offers = append(out.Offers, *o)
}

// offerID names an offer: its source and the source's ID; a folder's image
// by its source alone, since an artist has one of each kind; an upload by its
// hash.
func offerID(source, remoteID string, hash [32]byte) string {
	switch source {
	case OfferFolder:
		return OfferFolder
	case OfferUpload:
		return OfferUpload + ":" + hex.EncodeToString(hash[:])
	}
	return source + ":" + remoteID
}

func offerOf(c *Candidate) Offer {
	return Offer{
		ID: offerID(c.Source, c.ID, [32]byte{}), Source: c.Source, RemoteID: c.ID, Lang: c.Lang, Credit: c.Credit,
		Width: c.Width, Height: c.Height, url: c.URL,
	}
}

// offered returns what src offers the artist of kind, best first, and how it
// stood: kept within the day, else asked now at a person's priority.
func (s *Scraper) offered(ctx context.Context, a *store.ScrapeArtist, src store.ScrapeSource, kind store.ArtistImageKind) ([]Candidate, OfferSource) {
	st := OfferSource{Source: imageName(src)}
	cfg := s.config.Load()
	switch {
	case a.MBID == "" || !Askable(a.MBID):
		st.State = OfferUnavailable
		return nil, st
	case a.Held:
		st.State = OfferHeld
		return nil, st
	case !cfg.Remote || !s.on(cfg, src):
		st.State = OfferOff
		return nil, st
	}
	images, ok := s.keptOffers(ctx, src, a.MBID)
	if !ok {
		var err error
		if images, err = s.askOffers(ctx, cfg, src, a.MBID); err != nil {
			st.State, st.Words = OfferFailed, describe(src, err, s.now()).Words
			return nil, st
		}
	}
	st.State = OfferAnswered
	return Ranked(images[kind], cfg.Language), st
}

// imageName is the name a source's images carry.
func imageName(src store.ScrapeSource) string {
	if src == store.ScrapeFanart {
		return OfferFanart
	}
	return OfferWikimedia
}

// askOffers asks src what it offers the artist mbid, every kind, at a person's
// priority, and keeps the answer for the pass and the next look.
func (s *Scraper) askOffers(ctx context.Context, cfg *Config, src store.ScrapeSource, mbid string) (map[store.ArtistImageKind][]Candidate, error) {
	f := &found{source: src, images: map[store.ArtistImageKind][]Candidate{}}
	var err error
	switch src {
	case store.ScrapeFanart:
		err = s.askFanart(ctx, cfg, mbid, f)
	case store.ScrapeWikimedia:
		err = s.askPortrait(ctx, mbid, f)
	case store.ScrapeLastfm, store.ScrapeArchive, store.ScrapeMusicBrainz:
	}
	if errors.Is(err, remote.ErrNotFound) {
		err = nil // None to offer.
	}
	if err != nil {
		return nil, err
	}
	s.keepOffers(ctx, src, mbid, f.images)
	return f.images, nil
}

// askPortrait finds mbid's Commons portrait through MusicBrainz and Wikidata,
// into f.
func (s *Scraper) askPortrait(ctx context.Context, mbid string, f *found) error {
	links, err := s.o.MusicBrainz.ArtistLinks(ctx, mbid)
	if errors.Is(err, matching.ErrNotFound) || (err == nil && links.Wikidata == "") {
		return nil
	}
	if err != nil {
		return err //nolint:wrapcheck // describe words MusicBrainz's.
	}
	c, err := s.portrait(ctx, links.Wikidata)
	if err == nil && c != nil {
		f.images[store.ArtistThumb] = []Candidate{*c}
	}
	return err
}

// previews gives each offer from a source its preview: the one made within
// the day, else fetched now and rendered into the artwork cache. An offer
// whose preview can't be fetched goes without.
func (s *Scraper) previews(ctx context.Context, offers []Offer) {
	if s.o.Artwork == nil {
		return
	}
	var wg sync.WaitGroup
	slots := make(chan struct{}, previewsAtOnce)
	for i := range offers {
		if o := &offers[i]; o.Preview == nil && o.url != "" {
			wg.Go(func() {
				slots <- struct{}{}
				defer func() { <-slots }()
				o.Preview = s.preview(ctx, o)
			})
		}
	}
	wg.Wait()
}

// preview returns o's preview, fetching it when none fresh is kept, or nil.
func (s *Scraper) preview(ctx context.Context, o *Offer) *store.Picture {
	if p, err := s.o.Store.ArtworkPreview(ctx, o.Source, o.RemoteID, s.now().Add(-offersFor)); err == nil && p != nil {
		if _, perr := s.o.Artwork.Path(p.Hash, artwork.Size400); perr == nil {
			return p
		}
	}
	body, err := s.fetchPreview(ctx, o)
	if err != nil {
		s.log.DebugContext(ctx, "choose artwork: a preview didn't come", "source", o.Source, "err", err)
		return nil
	}
	s.sweep.RLock() // No sweep takes the files between the render and the note.
	defer s.sweep.RUnlock()
	img, err := s.o.Artwork.Put(ctx, artwork.Preview, body)
	if err != nil {
		s.log.DebugContext(ctx, "choose artwork: a preview didn't render", "source", o.Source, "err", err)
		return nil
	}
	p := store.Picture{Hash: img.Hash, Width: img.Width, Height: img.Height, Accent: img.Accent, Ground: img.Ground, Alpha: img.Alpha}
	if err = s.o.Store.PutArtworkPreview(ctx, o.Source, o.RemoteID, &p, s.now()); err != nil {
		s.log.WarnContext(ctx, "choose artwork: noting a preview failed", "err", err)
	}
	return &p
}

// fetchPreview fetches the source's own small rendition of o.
func (s *Scraper) fetchPreview(ctx context.Context, o *Offer) ([]byte, error) {
	switch o.Source {
	case OfferFanart:
		return s.o.Fanart.Preview(ctx, o.url) //nolint:wrapcheck // It names fanart.tv.
	case OfferWikimedia:
		f, err := s.o.Wikimedia.File(ctx, o.RemoteID, previewWidth)
		if err != nil {
			return nil, err //nolint:wrapcheck // It names Commons.
		}
		return s.o.Wikimedia.Download(ctx, f.URL) //nolint:wrapcheck // As above.
	}
	return nil, fmt.Errorf("%w: %s", errUnknownSource, o.Source)
}

// maybeSweep sweeps previews past a day, at most once an hour.
func (s *Scraper) maybeSweep(ctx context.Context) {
	now := s.now()
	if last := s.swept.Load(); last != nil && now.Sub(*last) < sweepEvery {
		return
	}
	s.swept.Store(&now)
	s.sweepPreviews(ctx)
}

// sweepPreviews forgets previews made more than a day ago, and removes the
// files of those nothing else uses.
func (s *Scraper) sweepPreviews(ctx context.Context) {
	if s.o.Artwork == nil {
		return
	}
	s.sweep.Lock()
	defer s.sweep.Unlock()
	unused, err := s.o.Store.SweepPreviews(ctx, s.now().Add(-offersFor))
	if err != nil {
		if ctx.Err() == nil {
			s.log.WarnContext(ctx, "choose artwork: sweeping previews failed", "err", err)
		}
		return
	}
	for _, h := range unused {
		if err = s.o.Artwork.Delete(h); err != nil {
			s.log.WarnContext(ctx, "choose artwork: removing a preview failed", "err", err)
		}
	}
}

// Pin pins the candidate named by candidate as the kind of the artist with
// public ID id, chosen by the administrator by, and returns the image it now
// shows. What it pins is fetched at a person's priority, unless Kitsune holds
// it already. It returns an error wrapping [store.ErrNotFound] for an unknown
// artist, [ErrNoCandidate] for no such candidate, [ErrSourceFailed] when the
// source didn't give it, and [ErrNoArtwork] with no artwork cache.
func (s *Scraper) Pin(ctx context.Context, id string, kind store.ArtistImageKind, candidate string, by int64) (*store.ExtraImage, error) {
	if s.o.Artwork == nil {
		return nil, ErrNoArtwork
	}
	a, err := s.o.Store.ScrapeArtistByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("pin: %w", err)
	}
	own := &Offers{}
	err = s.ownOffers(ctx, &a, kind, own)
	if err != nil {
		return nil, err
	}
	for i := range own.Offers {
		if o := &own.Offers[i]; o.ID == candidate {
			return s.pinHeld(ctx, &a, kind, o, by)
		}
	}
	o, err := s.findOffer(ctx, &a, kind, candidate)
	if err != nil {
		return nil, err
	}
	body, err := s.download(ctx, &Candidate{Source: o.Source, URL: o.url})
	if err != nil {
		return nil, fmt.Errorf("pin: %w: %w", ErrSourceFailed, err)
	}
	img, err := s.o.Artwork.Put(ctx, artworkKind(kind), body)
	if err != nil {
		return nil, fmt.Errorf("pin: %w: %w", ErrSourceFailed, err)
	}
	return s.storePin(ctx, &store.PinWrite{
		ArtistID: a.ID, Kind: kind, Source: o.Source, RemoteID: o.RemoteID, Credit: o.Credit,
		Image: artworkWrite(img, store.ArtworkRemote), By: by,
	})
}

// pinHeld pins o, an image the artist holds already: its pin (nothing to do)
// or what lies beneath it.
func (s *Scraper) pinHeld(ctx context.Context, a *store.ScrapeArtist, kind store.ArtistImageKind, o *Offer, by int64) (*store.ExtraImage, error) {
	if o.Pinned {
		return &store.ExtraImage{Picture: *o.Preview, Source: fromPinned, Credit: o.Credit}, nil
	}
	src := store.ArtworkRemote
	if o.Source == OfferFolder {
		src = store.ArtworkFolder
	}
	p := o.Preview
	return s.storePin(ctx, &store.PinWrite{
		ArtistID: a.ID, Kind: kind, Source: o.Source, RemoteID: o.RemoteID, Credit: o.Credit, By: by,
		Image: store.ArtworkWrite{Hash: p.Hash, Width: p.Width, Height: p.Height, Accent: p.Accent, Ground: p.Ground, Source: src, Alpha: p.Alpha},
	})
}

// findOffer is the source's candidate named candidate, from what it offers the
// artist of kind now.
func (s *Scraper) findOffer(ctx context.Context, a *store.ScrapeArtist, kind store.ArtistImageKind, candidate string) (*Offer, error) {
	source, _, ok := strings.Cut(candidate, ":")
	if !ok || (source != OfferFanart && source != OfferWikimedia) {
		return nil, ErrNoCandidate
	}
	src := store.ScrapeFanart
	if source == OfferWikimedia {
		src = store.ScrapeWikimedia
	}
	cands, st := s.offered(ctx, a, src, kind)
	if st.State == OfferFailed {
		return nil, fmt.Errorf("pin: %w: %s", ErrSourceFailed, st.Words)
	}
	for i := range cands {
		if o := offerOf(&cands[i]); o.ID == candidate {
			return &o, nil
		}
	}
	return nil, ErrNoCandidate
}

// PinUpload renders data, an image an administrator uploaded, into the
// artwork cache on kind's ladder and pins it as the kind of the artist with
// public ID id. It returns an error wrapping [store.ErrNotFound] for an
// unknown artist, [artwork.ErrBadImage] or [artwork.ErrTooLarge] for data it
// won't render, and [ErrNoArtwork] with no artwork cache.
func (s *Scraper) PinUpload(ctx context.Context, id string, kind store.ArtistImageKind, data []byte, by int64) (*store.ExtraImage, error) {
	if s.o.Artwork == nil {
		return nil, ErrNoArtwork
	}
	a, err := s.o.Store.ScrapeArtistByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	img, err := s.o.Artwork.Put(ctx, artworkKind(kind), data)
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	return s.storePin(ctx, &store.PinWrite{ArtistID: a.ID, Kind: kind, Source: OfferUpload, Image: artworkWrite(img, store.ArtworkUpload), By: by})
}

func artworkWrite(img *artwork.Image, src store.ArtworkSource) store.ArtworkWrite {
	return store.ArtworkWrite{Hash: img.Hash, Width: img.Width, Height: img.Height, Accent: img.Accent, Ground: img.Ground, Source: src, Alpha: img.Alpha}
}

// storePin stores w and returns the image the artist now shows.
func (s *Scraper) storePin(ctx context.Context, w *store.PinWrite) (*store.ExtraImage, error) {
	if err := s.o.Store.PinArtistImage(ctx, w, s.now()); err != nil {
		return nil, fmt.Errorf("pin: %w", err)
	}
	i := &w.Image
	return &store.ExtraImage{
		Hash: i.Hash, Width: i.Width, Height: i.Height, Accent: i.Accent, Ground: i.Ground, Alpha: i.Alpha,
		Source: fromPinned, Credit: w.Credit,
	}, nil
}

// Unpin takes away the pin of kind of the artist with public ID id, and
// reports whether it had one. What lies beneath shows again at once, and the
// pass asks the sources about the artist again at a person's priority.
func (s *Scraper) Unpin(ctx context.Context, id string, kind store.ArtistImageKind) (bool, error) {
	a, err := s.o.Store.ScrapeArtistByID(ctx, id)
	if err != nil {
		return false, fmt.Errorf("unpin: %w", err)
	}
	gone, err := s.o.Store.UnpinArtistImage(ctx, a.ID, kind, s.now())
	if err != nil {
		return false, fmt.Errorf("unpin: %w", err)
	}
	if gone && Askable(a.MBID) && !a.Held {
		s.Want(a.ULID)
	}
	return gone, nil
}

// Refresh asks every source on about the artist with public ID id again now,
// at a person's priority, whatever its schedule says. It returns without
// waiting; what changes is heard through Notify. It returns an error wrapping
// [store.ErrNotFound] for an unknown artist, and [ErrLookupsOff], [ErrNoMBID]
// or [ErrHeld] when no source may be asked.
func (s *Scraper) Refresh(ctx context.Context, id string) error {
	a, err := s.o.Store.ScrapeArtistByID(ctx, id)
	switch {
	case err != nil:
		return fmt.Errorf("refresh: %w", err)
	case !s.config.Load().Remote:
		return ErrLookupsOff
	case !Askable(a.MBID):
		return ErrNoMBID
	case a.Held:
		return ErrHeld
	}
	if _, err = s.o.Store.AskAgain(ctx, artistSources(), []string{a.MBID}, s.now()); err != nil {
		return fmt.Errorf("refresh: %w", err)
	}
	for _, src := range []store.ScrapeSource{store.ScrapeFanart, store.ScrapeWikimedia} { // What they offer, too.
		if err = s.o.Store.DropLookup(ctx, keptName(src), a.MBID); err != nil {
			return fmt.Errorf("refresh: %w", err)
		}
	}
	s.Want(a.ULID)
	return nil
}

// Reject is Not this artist for the artist with public ID id, said by the
// administrator by: what the sources gave it goes, with any pin of it, and no
// source is asked about it again until its MBID changes. It returns an error
// wrapping [store.ErrNotFound] for an unknown artist and [ErrNoMBID] when it
// has none, since nothing was fetched by one.
func (s *Scraper) Reject(ctx context.Context, id string, by int64) error {
	a, err := s.o.Store.ScrapeArtistByID(ctx, id)
	if err != nil {
		return fmt.Errorf("reject: %w", err)
	}
	if !Askable(a.MBID) {
		return ErrNoMBID
	}
	if err = s.o.Store.HoldArtist(ctx, a.ID, a.MBID, by, s.now()); err != nil {
		return fmt.Errorf("reject: %w", err)
	}
	return nil
}
