package scrape_test

import (
	"encoding/hex"
	"errors"
	"image"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/scrape"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv/fanarttvtest"
	"github.com/simonhull/kitsune/internal/store"
)

// shownByPin is how an image a pin chose says where it came from.
const shownByPin = "pinned"

// admin writes the administrator the picker's tests pin as, and returns its ID.
func admin(t *testing.T, r *rig) int64 {
	t.Helper()
	u, err := r.st.CreateUser(t.Context(), store.NewUser{Name: "simon", PasswordHash: "hash", Role: store.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func offerIDs(o *scrape.Offers) []string {
	ids := make([]string, len(o.Offers))
	for i := range o.Offers {
		ids[i] = o.Offers[i].ID
	}
	return ids
}

// jsonCalls counts the stand-in's answers about the first test artist,
// leaving out images, their previews, albums and the feed.
func jsonCalls(r *rig) int {
	n := 0
	for _, c := range r.ft.Calls() {
		if c.Path == "/v3.2/music/"+mbidOf(1) {
			n++
		}
	}
	return n
}

// candidates lists the artist's offers of kind, failing the test on an error.
func (r *rig) candidates(t *testing.T, id string, kind store.ArtistImageKind) *scrape.Offers {
	t.Helper()
	got, err := r.s.Candidates(t.Context(), id, kind)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// passedRig is a rig with the first test artist, and its public ID.
type passedRig struct {
	rig *rig
	id  string
}

// passed is a rig with the first test artist, everything at every source, and
// a pass over it done.
func passed(t *testing.T) passedRig {
	t.Helper()
	r := newRig(t)
	id := r.addArtist(t, 1, false)
	r.standInArt(1)
	r.s.Pass(t.Context())
	return passedRig{rig: r, id: id}
}

func TestCandidatesAreWhatThePassKept(t *testing.T) {
	t.Parallel()
	p := passed(t)
	r, id := p.rig, p.id
	asked := jsonCalls(r)
	got := r.candidates(t, id, store.ArtistBackground)
	if want := []string{"fanart:21", "fanart:22"}; !slices.Equal(offerIDs(got), want) {
		t.Fatalf("offers = %v, want %v: the one shown first, then as the pass would choose", offerIDs(got), want)
	}
	if o := got.Offers[0]; !o.Chosen || o.Pinned || o.Preview == nil {
		t.Errorf("the background shown = %+v, want it chosen, automatic, with its own image as preview", o)
	}
	checkPreviewed(t, r, &got.Offers[1])
	if n := jsonCalls(r); n != asked {
		t.Errorf("listing asked fanart.tv %d more times, want none: the pass kept its answer", n-asked)
	}
	if s := got.Sources; len(s) != 1 || s[0].Source != scrape.OfferFanart || s[0].State != scrape.OfferAnswered {
		t.Errorf("sources = %+v, want fanart.tv answered", s)
	}
}

// checkPreviewed holds o, fanart.tv's 1080p background, to a preview in the
// artwork cache and its source's size.
func checkPreviewed(t *testing.T, r *rig, o *scrape.Offer) {
	t.Helper()
	if o.Chosen || o.Preview == nil || o.Width != 1920 {
		t.Fatalf("offer = %+v, want a preview and fanart.tv's 1080p size", o)
	}
	if _, err := r.art.Path(o.Preview.Hash, artwork.Size400); err != nil {
		t.Errorf("preview: %v, want it in the artwork cache", err)
	}
}

func TestCandidatesAskWhenNothingIsKept(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	id := r.addArtist(t, 1, false)
	r.standInArt(1)
	got := r.candidates(t, id, store.ArtistThumb)
	if want := []string{"fanart:11", "wikimedia:Artist 1.jpg"}; !slices.Equal(offerIDs(got), want) {
		t.Fatalf("offers = %v, want %v", offerIDs(got), want)
	}
	if p := got.Offers[1]; p.Credit == nil || p.Credit.Author != "Ann" || p.Preview == nil {
		t.Errorf("the portrait = %+v, want its credit and a preview", p)
	}
	calls := len(r.ft.Calls()) + len(r.wm.Calls())
	r.candidates(t, id, store.ArtistThumb)
	if n := len(r.ft.Calls()) + len(r.wm.Calls()); n != calls {
		t.Errorf("a second look asked %d more times, want none: offers and previews are kept a day", n-calls)
	}
}

func TestCandidatesOfAnArtistWithNoMBID(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	got := r.candidates(t, r.addArtist(t, 2, true), store.ArtistThumb)
	if len(got.Offers) != 0 || got.Sources[0].State != scrape.OfferUnavailable {
		t.Errorf("an artist with no MBID: %+v", got)
	}
	if _, err := r.s.Candidates(t.Context(), "nobody", store.ArtistLogo); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("an unknown artist: %v", err)
	}
}

func TestCandidatesWhenASourceFails(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	id := r.addArtist(t, 1, false)
	r.ft.Fail("", 401)
	st := r.candidates(t, id, store.ArtistLogo).Sources[0]
	if st.State != scrape.OfferFailed || st.Words != "fanart.tv refused the key" || strings.Contains(st.Words, fanarttvtest.Key) {
		t.Errorf("a refused key: %+v", st)
	}
	off := r.cfg
	off.Remote = false
	r.s.Configure(off)
	if st = r.candidates(t, id, store.ArtistLogo).Sources[0]; st.State != scrape.OfferOff {
		t.Errorf("lookups off: %+v", st)
	}
}

func TestAPinLiesOverTheSources(t *testing.T) {
	t.Parallel()
	p := passed(t)
	r, id := p.rig, p.id
	img, err := r.s.Pin(t.Context(), id, store.ArtistBackground, "fanart:22", admin(t, r))
	if err != nil {
		t.Fatal(err)
	}
	if img.Source != shownByPin || img.Width != 192 {
		t.Fatalf("pinned %+v, want fanart.tv's 1080p background (its stand-in is 192 wide)", img)
	}
	if bg := r.artist(t, id).Images.Background; bg.Hash != img.Hash || bg.Source != shownByPin {
		t.Fatalf("background = %+v, want the pin", bg)
	}
	checkPinnedOffers(t, r.candidates(t, id, store.ArtistBackground))
	// A month on, the pass asks again and leaves the pin.
	r.advance(40 * 24 * time.Hour)
	r.s.Pass(t.Context())
	if bg := r.artist(t, id).Images.Background; bg.Hash != img.Hash {
		t.Error("a refresh replaced the pin")
	}
}

// checkPinnedOffers holds got to fanart.tv's 1080p background pinned by simon,
// then its 4K one beneath.
func checkPinnedOffers(t *testing.T, got *scrape.Offers) {
	t.Helper()
	if o := got.Offers[0]; o.ID != "fanart:22" || !o.Pinned || !o.Chosen || o.PinnedBy != "simon" || !got.Pinned {
		t.Errorf("first offer = %+v, want the pin, chosen, by simon", o)
	}
	if o := got.Offers[1]; o.ID != "fanart:21" || o.Chosen {
		t.Errorf("second offer = %+v, want fanart.tv's 4K background, beneath", o)
	}
}

func TestAutomaticShowsWhatIsBeneath(t *testing.T) {
	t.Parallel()
	p := passed(t)
	r, id := p.rig, p.id
	beneath := r.artist(t, id).Images.Background.Hash
	if _, err := r.s.Pin(t.Context(), id, store.ArtistBackground, "fanart:22", admin(t, r)); err != nil {
		t.Fatal(err)
	}
	gone, err := r.s.Unpin(t.Context(), id, store.ArtistBackground)
	if err != nil || !gone {
		t.Fatalf("unpin = %v, %v", gone, err)
	}
	if bg := r.artist(t, id).Images.Background; bg == nil || bg.Hash != beneath {
		t.Errorf("after unpinning = %+v, want fanart.tv's 4K background back", bg)
	}
	if gone, err = r.s.Unpin(t.Context(), id, store.ArtistBackground); err != nil || gone {
		t.Errorf("unpin again = %v, %v; want nothing to unpin", gone, err)
	}
}

func TestPinWhatTheArtistHolds(t *testing.T) {
	t.Parallel()
	p := passed(t)
	r, id := p.rig, p.id
	by := admin(t, r)
	shown := r.artist(t, id).Images.Logo
	calls := len(r.ft.Calls())
	img, err := r.s.Pin(t.Context(), id, store.ArtistLogo, "fanart:31", by)
	if err != nil || img.Hash != shown.Hash {
		t.Fatalf("pin the logo shown = %+v, %v; want it, as it is", img, err)
	}
	if n := len(r.ft.Calls()); n != calls {
		t.Errorf("pinning what the artist holds asked fanart.tv %d times", n-calls)
	}
	if _, err = r.s.Pin(t.Context(), id, store.ArtistLogo, "fanart:31", by); err != nil {
		t.Errorf("pinning the pin again: %v", err)
	}
	for _, bad := range []string{"fanart:999", "deezer:1", scrape.OfferFolder, ""} {
		if _, err = r.s.Pin(t.Context(), id, store.ArtistLogo, bad, by); !errors.Is(err, scrape.ErrNoCandidate) {
			t.Errorf("pin %q: %v, want ErrNoCandidate", bad, err)
		}
	}
}

func TestAnUploadIsAPin(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	id := r.addArtist(t, 1, false)
	by := admin(t, r)
	img, err := r.s.PinUpload(t.Context(), id, store.ArtistThumb, fanarttvtest.Draw("mine", image.Point{X: 600, Y: 600}, false), by)
	if err != nil {
		t.Fatal(err)
	}
	want := "upload:" + hex.EncodeToString(img.Hash[:])
	if o := r.candidates(t, id, store.ArtistThumb).Offers[0]; o.ID != want || o.Source != scrape.OfferUpload || !o.Pinned {
		t.Errorf("first offer = %+v, want the upload, pinned", o)
	}
	if _, err = r.s.PinUpload(t.Context(), id, store.ArtistThumb, []byte("not an image"), by); !errors.Is(err, artwork.ErrBadImage) {
		t.Errorf("an upload that isn't an image: %v, want ErrBadImage", err)
	}
}

func TestNotThisArtistTakesWhatWasFetched(t *testing.T) {
	t.Parallel()
	p := passed(t)
	r, id := p.rig, p.id
	if err := r.s.Reject(t.Context(), id, admin(t, r)); err != nil {
		t.Fatal(err)
	}
	if d := r.artist(t, id); d.Images != (store.ArtistImages{}) || d.Bio != nil {
		t.Errorf("after Not this artist: %+v, bio %+v; want nothing fetched left", d.Images, d.Bio)
	}
	got := r.candidates(t, id, store.ArtistThumb)
	if !got.Held || got.Sources[0].State != scrape.OfferHeld || len(got.Offers) != 0 {
		t.Errorf("candidates of a held artist = %+v", got)
	}
	if err := r.s.Refresh(t.Context(), id); !errors.Is(err, scrape.ErrHeld) {
		t.Errorf("refresh a held artist: %v, want ErrHeld", err)
	}
	if err := r.s.Reject(t.Context(), r.addArtist(t, 2, true), 0); !errors.Is(err, scrape.ErrNoMBID) {
		t.Errorf("reject an artist with no MBID: %v, want ErrNoMBID", err)
	}
}

func TestAHeldArtistIsNeverAsked(t *testing.T) {
	t.Parallel()
	p := passed(t)
	r, id := p.rig, p.id
	if err := r.s.Reject(t.Context(), id, admin(t, r)); err != nil {
		t.Fatal(err)
	}
	asked := jsonCalls(r)
	r.advance(40 * 24 * time.Hour)
	r.s.Pass(t.Context())
	r.s.Want(id)
	r.s.Pass(t.Context())
	if n := jsonCalls(r); n != asked {
		t.Errorf("a held artist was asked about %d times", n-asked)
	}
}

func TestLookAgainAsksNow(t *testing.T) {
	t.Parallel()
	p := passed(t)
	r, id := p.rig, p.id
	asked := jsonCalls(r)
	r.s.Pass(t.Context())
	if n := jsonCalls(r); n != asked {
		t.Fatalf("a pass with nothing due asked %d times", n-asked)
	}
	if err := r.s.Refresh(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	r.s.Pass(t.Context())
	if n := jsonCalls(r); n <= asked {
		t.Error("Look again now asked nothing")
	}
}

func TestLookAgainWhenNothingMayBeAsked(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	id := r.addArtist(t, 1, false)
	if err := r.s.Refresh(t.Context(), r.addArtist(t, 2, true)); !errors.Is(err, scrape.ErrNoMBID) {
		t.Errorf("refresh an artist with no MBID: %v, want ErrNoMBID", err)
	}
	off := r.cfg
	off.Remote = false
	r.s.Configure(off)
	if err := r.s.Refresh(t.Context(), id); !errors.Is(err, scrape.ErrLookupsOff) {
		t.Errorf("refresh with lookups off: %v, want ErrLookupsOff", err)
	}
}

func TestPreviewsGoAfterADay(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	id := r.addArtist(t, 1, false)
	r.standInArt(1)
	got := r.candidates(t, id, store.ArtistBanner)
	if len(got.Offers) != 1 || got.Offers[0].Preview == nil {
		t.Fatalf("banners = %+v, want one with a preview", got)
	}
	r.advance(25 * time.Hour)
	r.s.Pass(t.Context()) // Sweeps, as a pass does once an hour.
	if _, err := r.art.Path(got.Offers[0].Preview.Hash, artwork.Size400); !errors.Is(err, artwork.ErrNotFound) {
		t.Errorf("a day-old preview is still in the cache: %v", err)
	}
}
