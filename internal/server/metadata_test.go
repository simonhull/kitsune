package server

import (
	"crypto/sha256"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/lastfm/lastfmtest"
	"github.com/simonhull/kitsune/internal/matching/musicbrainztest"
	"github.com/simonhull/kitsune/internal/scrape"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv/fanarttvtest"
	"github.com/simonhull/kitsune/internal/scrape/wikimedia/wikimediatest"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	metadataPath = "/admin/metadata"
	// fanartProjectKey and fanartPersonalKey are fanart.tv keys no response,
	// log or event may ever hold.
	fanartProjectKey  = fanarttvtest.Key
	fanartPersonalKey = "fanart-personal-key-in-a-test"
)

// scrapeServer is a test server with the artist pass over stand-ins, lookups
// on and fanart.tv's keys set.
type scrapeServer struct {
	*testServer

	ft  *fanarttvtest.Server
	wm  *wikimediatest.Server
	mb  *musicbrainztest.Server
	lf  *lastfmtest.Server
	cfg scrape.Config
}

func newScrapeServer(t *testing.T) *scrapeServer {
	t.Helper()
	ss := &scrapeServer{ft: fanarttvtest.New(t), wm: wikimediatest.New(t), mb: musicbrainztest.New(t), lf: lastfmtest.New(t)}
	ss.testServer = newTestServer(t, testServerOptions{
		Artwork: artwork.NewCache(t.TempDir()), MusicBrainz: ss.mb,
		Scrape: &scrapeStandIns{Fanart: ss.ft, Wikimedia: ss.wm},
	})
	if err := ss.Store.SetScrapeFeedRead(t.Context(), store.ScrapeFanart, time.Now()); err != nil {
		t.Fatal(err)
	}
	ss.cfg = scrape.Config{
		Remote: true, Fanart: true, Wikipedia: true, LastfmBios: true, Archive: true, Language: "en",
		Keys: fanarttv.Keys{API: fanartProjectKey, Client: fanartPersonalKey}, Lastfm: ss.lf.Client(),
	}
	ss.Scraper.Configure(ss.cfg)
	return ss
}

// addArtist writes an album by the i-th artist, with an MBID the stand-ins
// know, and returns the artist's public ID.
func (ss *scrapeServer) addArtist(t *testing.T, i int) string {
	t.Helper()
	ulid, mbid, name := fmt.Sprintf("%026d", 500+i), musicbrainztest.ID("scraped "+strconv.Itoa(i)), fmt.Sprintf("Scraped %d", i)
	w := store.TrackWrite{
		ULID: fmt.Sprintf("01S%023d", i), Root: musicBase, RelPath: fmt.Sprintf("s/%d.flac", i), Size: 1000,
		MTime: time.UnixMilli(1_761_000_000_000), Digest: new(sha256.Sum256([]byte(name))),
		Format: flacFormat, Codec: flacFormat, Duration: 200 * time.Second, Title: scrapedSong, SortTitle: scrapedSong,
		Album: store.AlbumWrite{
			GroupKey: "scraped " + strconv.Itoa(i), ULID: fmt.Sprintf("02S%023d", i), Artist: name,
			Name: "Album " + strconv.Itoa(i), SortName: "Album " + strconv.Itoa(i),
		},
		Artists: []store.ArtistWrite{{ULID: ulid, MatchKey: strings.ToLower(name), Name: name, MBID: mbid}},
	}
	if err := ss.Store.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{w}}); err != nil {
		t.Fatal(err)
	}
	ss.ft.Add(fanarttvtest.Artist{MBID: mbid, Backgrounds: []fanarttvtest.Image{{ID: "7"}}})
	item := "Q" + strconv.Itoa(900+i)
	ss.mb.AddArtist(musicbrainztest.Artist{ID: mbid, Name: name, Wikidata: item})
	ss.wm.Add(
		wikimediatest.Item{ID: item, Articles: map[string]string{"en": name}, Image: name + ".jpg"},
		[]wikimediatest.Article{{Lang: "en", Title: name, Extract: name + " are a band."}},
		&wikimediatest.File{Name: name + ".jpg", Author: annName, License: "CC BY-SA 4.0", LicenseURL: "https://creativecommons.org/licenses/by-sa/4.0"},
	)
	return ulid
}

func (ss *scrapeServer) metadata(t *testing.T, token string) api.MetadataStatus {
	t.Helper()
	r := ss.send(t, call{method: http.MethodGet, path: metadataPath, header: bearer(token)})
	if r.Status != http.StatusOK {
		t.Fatalf("status %d: %s", r.Status, r.Body)
	}
	for _, key := range []string{fanartProjectKey, fanartPersonalKey} {
		if strings.Contains(string(r.Body), key) {
			t.Errorf("the report holds a key: %s", r.Body)
		}
	}
	var m api.MetadataStatus
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMetadataReportsThePassAndItsSources(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	ss.addArtist(t, 1)
	token := ss.tokenFor(t, store.RoleAdmin)
	before := ss.metadata(t, token)
	if !before.Remote || before.Artists.WithMBID != 1 || before.Artists.LookedUp != 0 || len(before.Sources) != 5 {
		t.Errorf("before = %+v", before)
	}
	ss.Scraper.Pass(t.Context())
	after := ss.metadata(t, token)
	checkAfterAPass(t, &after)
}

// checkAfterAPass holds m to one artist looked up, with art, fanart.tv on with
// both keys set.
func checkAfterAPass(t *testing.T, m *api.MetadataStatus) {
	t.Helper()
	if m.Artists.LookedUp != 1 || m.Artists.WithArt != 1 || m.NextRefresh == nil {
		t.Errorf("after = %+v", m)
	}
	fan := m.Sources[2]
	if fan.Name != "fanarttv" || fan.State != "on" || fan.Key != keyIsSet || fan.ClientKey != keyIsSet {
		t.Errorf("fanart.tv = %+v", fan)
	}
}

func TestMetadataSaysARefusedKeyInWords(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	ss.addArtist(t, 1)
	ss.ft.Want("another")
	ss.Scraper.Pass(t.Context())
	fan := ss.metadata(t, ss.tokenFor(t, store.RoleAdmin)).Sources[2]
	if fan.State != "refused" || fan.Failure == nil || fan.Failure.Words != "fanart.tv refused the key" || fan.Failure.Status != http.StatusUnauthorized {
		t.Errorf("fanart.tv = %+v", fan)
	}
	if logs := ss.Log.String(); strings.Contains(logs, fanartProjectKey) || strings.Contains(logs, fanartPersonalKey) {
		t.Errorf("the log holds a key: %s", logs)
	}
}

func TestMetadataIsForAdministrators(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	ss.send(t, call{method: http.MethodGet, path: metadataPath, header: bearer(ss.tokenFor(t, store.RoleUser))}).problem(t, http.StatusForbidden)
	ss.send(t, call{method: http.MethodGet, path: metadataPath, invalid: true}).problem(t, http.StatusUnauthorized)
}

func TestMetadataWithoutThePassIsUnavailable(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.send(t, call{method: http.MethodGet, path: metadataPath, header: bearer(ts.tokenFor(t, store.RoleAdmin))}).problem(t, http.StatusServiceUnavailable)
}

// Opening an artist with nothing yet asks for it first, without waiting: the
// page answers with what there is, and the art comes in on the artists event.
func TestOpeningAnArtistAsksForItFirst(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	ss.addArtist(t, 1)
	second := ss.addArtist(t, 2)
	token := ss.tokenFor(t, store.RoleUser)
	r := ss.send(t, call{method: http.MethodGet, path: "/artists/" + second, header: bearer(token)})
	if r.Status != http.StatusOK {
		t.Fatalf("status %d", r.Status)
	}
	ss.Scraper.Pass(t.Context())
	calls := ss.ft.Calls()
	if len(calls) == 0 || !strings.HasSuffix(calls[0].Path, musicbrainztest.ID("scraped 2")) {
		t.Errorf("fanart.tv calls %+v, want the artist opened first", calls)
	}
	var d api.ArtistDetail
	r = ss.send(t, call{method: http.MethodGet, path: "/artists/" + second, header: bearer(token)})
	if err := json.Unmarshal(r.Body, &d); err != nil {
		t.Fatal(err)
	}
	checkScraped(t, &d)
}

// checkScraped holds d to fanart.tv's background, Wikipedia's bio and
// Commons' portrait with its credit.
func checkScraped(t *testing.T, d *api.ArtistDetail) {
	t.Helper()
	if d.Images.Background == nil || d.Images.Background.Source != store.ImageFromFanart || d.Bio == nil || d.Bio.License == "" {
		t.Errorf("artist = %+v, want fanart.tv's background and Wikipedia's bio", d)
	}
	if th := d.Images.Thumb; th == nil || th.Credit == nil || th.Credit.Author != annName {
		t.Errorf("thumb = %+v, want Commons' portrait with its credit", th)
	}
}
