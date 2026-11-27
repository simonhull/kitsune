package scrape_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/matching/musicbrainztest"
	"github.com/simonhull/kitsune/internal/scrape"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv/fanarttvtest"
	"github.com/simonhull/kitsune/internal/scrape/wikimedia"
	"github.com/simonhull/kitsune/internal/scrape/wikimedia/wikimediatest"
	"github.com/simonhull/kitsune/internal/store"
)

// personalKey is a person's fanart.tv key, which nothing may ever quote.
const personalKey = "fanart-personal-key-of-someone"

// ccBySA is the license of every stand-in text and portrait.
const ccBySA = "CC BY-SA 4.0"

// guard fails the test on any request that would leave httptest's loopback
// listeners: nothing a test runs ever talks to a real service.
type guard struct {
	t    *testing.T
	next http.RoundTripper
}

func (g guard) RoundTrip(req *http.Request) (*http.Response, error) {
	host, _, err := net.SplitHostPort(req.URL.Host)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		g.t.Errorf("a request left httptest: %s", req.URL.Redacted())
		return nil, http.ErrHandlerTimeout
	}
	return g.next.RoundTrip(req) //nolint:wrapcheck // A transport's own error.
}

// clientTimeout is how long a request may take in these tests: a slow
// stand-in takes longer.
const clientTimeout = 500 * time.Millisecond

func guarded(t *testing.T, c *http.Client) *http.Client {
	t.Helper()
	return &http.Client{Transport: guard{t: t, next: c.Transport}, Timeout: clientTimeout}
}

// syncLog is a log the test reads while the pass writes it.
type syncLog struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *syncLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p) //nolint:wrapcheck // A builder never fails.
}

func (l *syncLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// rig is a store with artists, the three stand-ins and a scraper over them.
type rig struct {
	st  *store.Store
	ft  *fanarttvtest.Server
	wm  *wikimediatest.Server
	mb  *musicbrainztest.Server
	s   *scrape.Scraper
	log *syncLog
	cfg scrape.Config

	scan *scanClaim
	art  *artwork.Cache

	mu       sync.Mutex
	notified []string
	now      time.Time
}

// scanClaim stands in for the scanner's claim: held, the pass waits, and says
// so on waiting.
type scanClaim struct {
	mu      sync.Mutex
	free    chan struct{} // Closed while nothing holds the claim.
	waiting chan struct{}
}

func newScanClaim() *scanClaim {
	c := &scanClaim{free: make(chan struct{}), waiting: make(chan struct{}, 1)}
	close(c.free)
	return c
}

// WaitScanFree implements scrape.Library.
func (c *scanClaim) WaitScanFree(ctx context.Context) error {
	c.mu.Lock()
	free := c.free
	c.mu.Unlock()
	select {
	case <-free:
		return nil
	default:
	}
	select {
	case c.waiting <- struct{}{}:
	default:
	}
	select {
	case <-free:
		return nil
	case <-ctx.Done():
		return ctx.Err() //nolint:wrapcheck // ctx's own, as the scanner's.
	}
}

func (c *scanClaim) hold() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.free = make(chan struct{})
}

func (c *scanClaim) release() {
	c.mu.Lock()
	defer c.mu.Unlock()
	close(c.free)
}

func newRig(t *testing.T) *rig {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), store.FileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := &rig{st: st, ft: fanarttvtest.New(t), wm: wikimediatest.New(t), mb: musicbrainztest.New(t), log: &syncLog{}, now: time.Now(), scan: newScanClaim(), art: artwork.NewCache(t.TempDir())}
	fo, wo, mo := r.ft.Options(), r.wm.Options(), r.mb.Options()
	fo.HTTP, wo.HTTP, mo.HTTP = guarded(t, fo.HTTP), guarded(t, wo.HTTP), guarded(t, mo.HTTP)
	r.s = scrape.New(scrape.Options{
		Store: st, Artwork: r.art, MusicBrainz: matching.NewMusicBrainz(mo),
		Fanart: fanarttv.New(fo), Wikimedia: wikimedia.New(wo),
		Notify:  func(ids []string) { r.mu.Lock(); r.notified = append(r.notified, ids...); r.mu.Unlock() },
		Library: r.scan,
		Logger:  slog.New(slog.NewTextHandler(r.log, nil)),
		Now:     func() time.Time { r.mu.Lock(); defer r.mu.Unlock(); return r.now },
	})
	// The feed of new art was read just now, so a pass doesn't read it first.
	if err = st.SetScrapeFeedRead(t.Context(), store.ScrapeFanart, r.now); err != nil {
		t.Fatal(err)
	}
	r.cfg = scrape.Config{Remote: true, Fanart: true, Wikipedia: true, Language: "en", Keys: fanarttv.Keys{API: fanarttvtest.Key, Client: personalKey}}
	r.s.Configure(r.cfg)
	return r
}

// advance moves the scraper's clock on by d.
func (r *rig) advance(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.now = r.now.Add(d)
}

func (r *rig) heard() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.notified)
}

// mbidOf is the i-th test artist's MBID.
func mbidOf(i int) string { return musicbrainztest.ID("artist " + strconv.Itoa(i)) }

// addArtist writes an album by the i-th artist, with its MBID unless bare,
// and returns the artist's public ID.
func (r *rig) addArtist(t *testing.T, i int, bare bool) string {
	t.Helper()
	ulid := fmt.Sprintf("%026d", 100+i)
	a := store.ArtistWrite{ULID: ulid, MatchKey: fmt.Sprintf("artist %d", i), Name: fmt.Sprintf("Artist %d", i)}
	if !bare {
		a.MBID = mbidOf(i)
	}
	w := store.TrackWrite{
		ULID: fmt.Sprintf("01J%023d", i), Root: "/music", RelPath: fmt.Sprintf("a/%d.flac", i), Size: 1000,
		MTime: time.UnixMilli(1_761_000_000_000), Digest: new(sha256.Sum256([]byte(strconv.Itoa(i)))),
		Format: "flac", Codec: "flac", Duration: 200 * time.Second, Title: "Song", SortTitle: "Song",
		Album: store.AlbumWrite{
			GroupKey: fmt.Sprintf("album %d", i), ULID: fmt.Sprintf("02J%023d", i), Artist: a.Name,
			Name: fmt.Sprintf("Album %d", i), SortName: fmt.Sprintf("Album %d", i),
		},
		Artists: []store.ArtistWrite{a},
	}
	if err := r.st.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{w}}); err != nil {
		t.Fatal(err)
	}
	return ulid
}

// standInArt gives the i-th artist everything at every source: fanart.tv's
// thumb, backgrounds in 4K and 1080p, a logo and a banner; a Wikidata item
// with an English article and a portrait.
func (r *rig) standInArt(i int) {
	mbid, item := mbidOf(i), "Q"+strconv.Itoa(1000+i)
	r.ft.Add(fanarttvtest.Artist{
		MBID:          mbid,
		Thumbs:        []fanarttvtest.Image{{ID: "11", Lang: "en", Likes: 2}},
		Backgrounds4K: []fanarttvtest.Image{{ID: "21"}},
		Backgrounds:   []fanarttvtest.Image{{ID: "22", Likes: 9}},
		HDLogos:       []fanarttvtest.Image{{ID: "31", Lang: "en"}},
		Banners:       []fanarttvtest.Image{{ID: "41"}},
	})
	r.mb.AddArtist(musicbrainztest.Artist{ID: mbid, Name: fmt.Sprintf("Artist %d", i), Wikidata: item})
	file := fmt.Sprintf("Artist %d.jpg", i)
	r.wm.Add(
		wikimediatest.Item{ID: item, Articles: map[string]string{"en": fmt.Sprintf("Artist %d", i)}, Image: file},
		[]wikimediatest.Article{{Lang: "en", Title: fmt.Sprintf("Artist %d", i), Extract: "Artist is a band.\nThey split."}},
		&wikimediatest.File{Name: file, Author: `<a href="x">Ann</a>`, License: ccBySA, LicenseURL: "https://creativecommons.org/licenses/by-sa/4.0"},
	)
}

func (r *rig) artist(t *testing.T, id string) store.ArtistDetail {
	t.Helper()
	d, err := r.st.Artist(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (r *rig) states(t *testing.T, id string) map[store.ScrapeSource]store.ScrapeState {
	t.Helper()
	a, err := r.st.ScrapeArtistByID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return a.States
}

func TestAPassFetchesArtAndABio(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	id := r.addArtist(t, 1, false)
	r.standInArt(1)
	r.s.Pass(t.Context())
	d := r.artist(t, id)
	checkImages(t, &d)
	checkBio(t, &d)
	if heard := r.heard(); !slices.Equal(heard, []string{id}) {
		t.Errorf("artists event = %v, want the artist once", heard)
	}
	for src, st := range r.states(t, id) {
		if st.Status != store.ScrapeFound {
			t.Errorf("%s: %+v, want found", src, st)
		}
	}
	// A second pass asks nothing: nothing is due.
	before := len(r.ft.Calls())
	r.s.Pass(t.Context())
	if n := len(r.ft.Calls()); n != before {
		t.Errorf("a second pass asked fanart.tv %d more times", n-before)
	}
}

// checkImages holds d to fanart.tv's four images and its 4K background.
func checkImages(t *testing.T, d *store.ArtistDetail) {
	t.Helper()
	imgs := d.Images
	if imgs.Thumb == nil || imgs.Thumb.Source != fanart || imgs.Background == nil || imgs.Logo == nil || imgs.Banner == nil {
		t.Fatalf("images = %+v, want all four from fanart.tv", imgs)
	}
	if imgs.Background.Width != 384 {
		t.Errorf("background %d wide, want the 4K one (its stand-in is 384)", imgs.Background.Width)
	}
}

// checkBio holds d to Wikipedia's lead with its license.
func checkBio(t *testing.T, d *store.ArtistDetail) {
	t.Helper()
	if b := d.Bio; b == nil || b.Source != store.BioFromWikipedia || b.License != ccBySA || b.URL == "" || b.Summary != "Artist is a band." {
		t.Errorf("bio = %+v, want Wikipedia's lead with its license", b)
	}
}

func TestCommonsGivesAPortraitWithItsCredit(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	id := r.addArtist(t, 1, false)
	r.standInArt(1)
	r.ft.Add(fanarttvtest.Artist{MBID: mbidOf(1), Backgrounds: []fanarttvtest.Image{{ID: "22"}}})
	r.s.Pass(t.Context())
	thumb := r.artist(t, id).Images.Thumb
	if thumb == nil || thumb.Source != commons || thumb.Credit == nil || thumb.Credit.Author != "Ann" || thumb.Credit.License != ccBySA || thumb.Credit.PageURL == "" {
		t.Errorf("thumb = %+v, want Commons' portrait with its credit", thumb)
	}
}

func TestFolderArtAndAnArtistNfoStay(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	id := r.addArtist(t, 1, false)
	r.standInArt(1)
	a, err := r.st.ScrapeArtistByID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	folder := &store.ArtworkWrite{Hash: sha256.Sum256([]byte("folder")), Width: 10, Height: 10, Accent: "#111111", Ground: "#000000", Source: store.ArtworkFolder}
	file := store.FileRef{Root: "/music", RelPath: "a/artist.jpg", Size: 1, MTime: time.UnixMilli(1)}
	if _, err = r.st.WriteExtras(t.Context(), &store.ExtrasWrite{
		ArtistImages: []store.ArtistImageWrite{{ArtistID: a.ID, Kind: store.ArtistThumb, Image: folder, File: file}},
		Bios:         []store.BioWrite{{ArtistID: a.ID, Bio: &store.NfoBio{Summary: "Mine.", Body: "Mine."}, File: file}},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	r.s.Pass(t.Context())
	d := r.artist(t, id)
	if d.Images.Thumb.Source != "folder" || d.Bio.Source != "nfo" || d.Images.Background == nil {
		t.Errorf("thumb from %s, bio from %s, background %v: want the folder's, the nfo's and fanart.tv's", d.Images.Thumb.Source, d.Bio.Source, d.Images.Background)
	}
}

func TestARefusedKeyStopsFanartUntilASettingChanges(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.addArtist(t, 1, false)
	r.addArtist(t, 2, false)
	r.standInArt(1)
	r.standInArt(2)
	r.ft.Want("another key")
	r.s.Pass(t.Context())
	if n := len(r.ft.Calls()); n != 1 {
		t.Errorf("%d calls to fanart.tv, want it asked once and then left alone", n)
	}
	rep, err := r.s.Report(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	fan := rep.Sources[2]
	if fan.State != scrape.SourceRefused || fan.Failure == nil || fan.Failure.Words != "fanart.tv refused the key" {
		t.Errorf("fanart.tv = %+v, want refused, in words", fan)
	}
	r.ft.Want(fanarttvtest.Key)
	r.s.Configure(r.cfg)
	r.s.Pass(t.Context())
	if n := len(r.ft.Calls()); n < 3 {
		t.Errorf("%d calls after the key changed, want both artists asked", n)
	}
}

func TestADownSourceBacksOffAndRests(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	id := r.addArtist(t, 1, false)
	r.addArtist(t, 2, false)
	r.standInArt(1)
	r.standInArt(2)
	r.ft.Fail("", 503, 503, 503, 503)
	r.s.Pass(t.Context())
	if n := len(r.ft.Calls()); n != 4 {
		t.Errorf("%d calls, want one artist's four tries and then a rest", n)
	}
	st := r.states(t, id)[store.ScrapeFanart]
	if st.Status != store.ScrapeFailed || st.Failures != 1 || st.Error != "fanart.tv answered 503" {
		t.Errorf("state = %+v, want one failure in words", st)
	}
	if d := r.artist(t, id); d.Bio == nil {
		t.Error("Wikipedia's bio waited for fanart.tv")
	}
	r.checkResting(t)
	r.advance(2 * time.Hour) // Past the rest and the artist's hour.
	r.s.Pass(t.Context())
	if st = r.states(t, id)[store.ScrapeFanart]; st.Status != store.ScrapeFound {
		t.Errorf("after the rest: %+v, want found", st)
	}
}

// checkResting holds fanart.tv to resting after a 503.
func (r *rig) checkResting(t *testing.T) {
	t.Helper()
	rep, err := r.s.Report(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if fan := rep.Sources[2]; fan.State != scrape.SourceResting || fan.Until.IsZero() || fan.Failure.Status != 503 {
		t.Errorf("fanart.tv = %+v, want it resting after a 503", fan)
	}
}

func TestBareNamesAreAFailureNeverAFetch(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	id := r.addArtist(t, 1, false)
	r.standInArt(1)
	r.ft.BareNames(true)
	r.s.Pass(t.Context())
	if st := r.states(t, id)[store.ScrapeFanart]; st.Status != store.ScrapeFailed || st.Error != "fanart.tv sent file names, not links" {
		t.Errorf("state = %+v", st)
	}
	for _, c := range r.ft.Calls() {
		if strings.HasPrefix(c.Path, "/fanart/") {
			t.Errorf("fetched %s", c.Path)
		}
	}
	if th := r.artist(t, id).Images.Thumb; th == nil || th.Source != commons {
		t.Errorf("thumb = %+v, want Commons' while fanart.tv's answer is bad", th)
	}
}

func TestTheLatestFeedPullsArtistsForward(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	id := r.addArtist(t, 1, false)
	r.standInArt(1)
	r.s.Pass(t.Context())
	asked := len(r.ft.Calls())
	r.ft.SetLatest(mbidOf(1), mbidOf(9))
	r.ft.Add(fanarttvtest.Artist{MBID: mbidOf(1), Thumbs: []fanarttvtest.Image{{ID: "12", Lang: "en", Likes: 7}}})
	r.advance(25 * time.Hour) // A day on: the feed is read again; the artist isn't due for a month.
	r.s.Pass(t.Context())
	if th := r.artist(t, id).Images.Thumb; th == nil || len(r.ft.Calls()) <= asked+1 {
		t.Fatalf("thumb = %+v after %d calls, want the feed read and the artist asked again", th, len(r.ft.Calls())-asked)
	}
	held, err := r.st.ArtistHeld(t.Context(), r.mustRow(t, id))
	if err != nil || held.Images[store.ArtistThumb].RemoteID != "12" {
		t.Errorf("thumb = %+v, %v, want the new one", held.Images[store.ArtistThumb], err)
	}
}

func (r *rig) mustRow(t *testing.T, id string) int64 {
	t.Helper()
	a, err := r.st.ScrapeArtistByID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func TestAPersonsArtistGoesFirst(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.addArtist(t, 1, false)
	second := r.addArtist(t, 2, false)
	r.standInArt(1)
	r.standInArt(2)
	r.s.Want(second)
	r.s.Pass(t.Context())
	calls := r.ft.Calls()
	if len(calls) == 0 || !strings.HasSuffix(calls[0].Path, mbidOf(2)) {
		t.Errorf("first call %+v, want the artist a person opened", calls)
	}
}

func TestAScanPausesThePass(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.addArtist(t, 1, false)
	r.standInArt(1)
	r.scan.hold()
	done := make(chan struct{})
	go func() { defer close(done); r.s.Pass(t.Context()) }()
	<-r.scan.waiting
	if n := len(r.ft.Calls()); n != 0 {
		t.Errorf("%d calls while a scan held the claim", n)
	}
	r.scan.release()
	<-done
	if len(r.ft.Calls()) == 0 {
		t.Error("nothing asked once the scan let go")
	}
}

func TestNothingIsAskedWithLookupsOff(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	id := r.addArtist(t, 1, false)
	r.standInArt(1)
	r.cfg.Remote = false
	r.s.Configure(r.cfg)
	r.s.Want(id)
	r.s.Pass(t.Context())
	if n := len(r.ft.Calls()) + len(r.wm.Calls()) + len(r.mb.Calls()); n != 0 {
		t.Errorf("%d calls with lookups off", n)
	}
}

func TestNoProjectKeyIsNoFanart(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.addArtist(t, 1, false)
	r.standInArt(1)
	r.cfg.Keys = fanarttv.Keys{Client: personalKey}
	r.s.Configure(r.cfg)
	r.s.Pass(t.Context())
	if n := len(r.ft.Calls()); n != 0 {
		t.Errorf("%d calls to fanart.tv without a project key", n)
	}
	rep, err := r.s.Report(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if fan := rep.Sources[2]; fan.State != scrape.SourceNeedsKey || fan.Key != scrape.KeyMissing || fan.ClientKey != scrape.KeySet {
		t.Errorf("fanart.tv = %+v, want it needing its key", fan)
	}
}

func TestReportCountsAlbumArtists(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.addArtist(t, 1, false)
	r.addArtist(t, 2, false)
	r.addArtist(t, 3, true)
	r.standInArt(1)
	r.s.Pass(t.Context())
	rep, err := r.s.Report(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if rep.AlbumArtists != 3 || rep.WithMBID != 2 || rep.WithoutMBID != 1 || rep.LookedUp != 2 || rep.WithArt != 1 || rep.FoundNothing != 1 || rep.NextRefresh.IsZero() {
		t.Errorf("report = %+v", rep)
	}
}
