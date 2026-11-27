package store

import (
	"errors"
	"testing"
	"time"
)

// scrapeMBID is newWrite(0)'s artist's MBID in these tests.
const scrapeMBID = "5b1c0d2e-0000-4000-8000-0000000000e2"

// scrapeArtist writes newWrite(0) with an MBID on its artist and returns the
// artist's row.
func scrapeArtist(t *testing.T, s *Store) int64 {
	t.Helper()
	w := newWrite(0)
	w.Artists[0].MBID = scrapeMBID
	writeAll(t, s, w)
	return count(t, s, "SELECT id FROM artists WHERE mbid = ?", scrapeMBID)
}

func remoteArt(seed string) *ArtworkWrite {
	a := testArtwork(seed)
	a.Source = ArtworkRemote
	return a
}

// mustWriteRemote stores w and returns whether it changed the artist's art.
func mustWriteRemote(t *testing.T, s *Store, w *RemoteWrite) bool {
	t.Helper()
	if w.MBID == "" {
		w.MBID = scrapeMBID
	}
	changed, err := s.WriteRemote(t.Context(), w, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

// statesOf returns what each source last said of newWrite(0)'s artist.
func statesOf(t *testing.T, s *Store) map[ScrapeSource]ScrapeState {
	t.Helper()
	a, err := s.ScrapeArtistByID(t.Context(), newWrite(0).Artists[0].ULID)
	if err != nil {
		t.Fatal(err)
	}
	return a.States
}

// testNow() is a fixed time, to the millisecond.
func testNow() time.Time { return time.UnixMilli(1_761_000_000_000).UTC() }

func TestScrapeArtistsCarryTheirStates(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	artist := scrapeArtist(t, s)
	writeAll(t, s, newWrite(1)) // No MBID: counted, never asked about.
	failed := ScrapeState{Status: ScrapeFailed, FetchedAt: testNow(), NextAt: testNow().Add(time.Hour), Failures: 2, Error: "fanart.tv answered 503"}
	mustWriteRemote(t, s, &RemoteWrite{ArtistID: artist, Answers: []ScrapeAnswer{{Source: ScrapeFanart, State: failed}}})
	got, err := s.ScrapeArtists(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != artist || got[0].MBID != scrapeMBID || !got[0].Owner || got[1].MBID != "" {
		t.Fatalf("scrape artists = %+v, want the one with an MBID, an owner, and the one without", got)
	}
	if st := got[0].States; st[ScrapeFanart] != failed || len(st) != 1 {
		t.Errorf("states = %+v, want fanart.tv's failure alone", st)
	}
	if st := statesOf(t, s); st[ScrapeFanart] != failed {
		t.Errorf("by ID: state = %+v", st[ScrapeFanart])
	}
}

func TestAnAnswerEndsTheFailures(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	artist := scrapeArtist(t, s)
	failed := ScrapeState{Status: ScrapeFailed, FetchedAt: testNow(), NextAt: testNow().Add(time.Hour), Failures: 3, Error: "timed out"}
	found := ScrapeState{Status: ScrapeFound, FetchedAt: testNow(), NextAt: testNow().Add(30 * 24 * time.Hour)}
	for _, st := range []ScrapeState{failed, found} {
		mustWriteRemote(t, s, &RemoteWrite{ArtistID: artist, Answers: []ScrapeAnswer{{Source: ScrapeWikimedia, State: st}}})
	}
	if st := statesOf(t, s)[ScrapeWikimedia]; st != found {
		t.Errorf("state = %+v, want %+v", st, found)
	}
}

// pinLogo pins a logo on the artist, as an administrator's choice is stored.
func pinLogo(t *testing.T, s *Store, artist int64) {
	t.Helper()
	pin(t, s, artist, ArtistLogo, "pin")
}

func TestRemoteArtNeverReplacesAFolderOrAPin(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	artist := scrapeArtist(t, s)
	folder := testArtwork("folder")
	folder.Source = ArtworkFolder
	if _, err := s.WriteExtras(t.Context(), &ExtrasWrite{
		ArtistImages: []ArtistImageWrite{{ArtistID: artist, Kind: ArtistThumb, Image: folder, File: folderFile("A/artist.jpg")}},
		Bios:         []BioWrite{{ArtistID: artist, Bio: &NfoBio{Summary: hello, Body: hello}, File: folderFile("A/artist.nfo")}},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	pinLogo(t, s, artist)
	w := &RemoteWrite{
		ArtistID: artist,
		Images: []RemoteImage{
			{Kind: ArtistThumb, Source: ImageFromFanart, RemoteID: "1", Image: remoteArt("fanart thumb")},
			{Kind: ArtistLogo, Source: ImageFromFanart, RemoteID: "2", Image: remoteArt("fanart logo")},
			{Kind: ArtistBackground, Source: ImageFromFanart, RemoteID: "3", Image: remoteArt("fanart bg"), Credit: &Credit{Author: "fanart.tv"}},
		},
		Bios: []RemoteBio{{Source: BioFromWikipedia, Bio: wikiBio()}},
	}
	if !mustWriteRemote(t, s, w) || mustWriteRemote(t, s, w) { // A refresh with the same answer changes nothing.
		t.Error("want the background written once")
	}
	checkPrecedence(t, s, artist)
}

// checkPrecedence holds the artist to a folder's thumb, a pinned logo, an
// artist.nfo's bio and fanart.tv's background with its credit.
func checkPrecedence(t *testing.T, s *Store, artist int64) {
	t.Helper()
	held, err := s.ArtistHeld(t.Context(), artist)
	if err != nil {
		t.Fatal(err)
	}
	if held.Images[ArtistThumb].Source != string(ArtworkFolder) || held.Images[ArtistLogo].Source != sourcePinned || held.Bio.Source != "nfo" {
		t.Errorf("held = %+v, bio %+v: a source replaced a folder, a pin or an artist.nfo", held.Images, held.Bio)
	}
	if bg := held.Images[ArtistBackground]; bg.Source != ImageFromFanart || bg.RemoteID != "3" {
		t.Errorf("background = %+v, want fanart.tv's", bg)
	}
	d, err := s.Artist(t.Context(), newWrite(0).Artists[0].ULID)
	if err != nil {
		t.Fatal(err)
	}
	if c := d.Images.Background.Credit; c == nil || c.Author != "fanart.tv" {
		t.Errorf("credit = %+v, want fanart.tv's", c)
	}
}

func TestRemoteWriteSaysWhenItChanged(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	artist := scrapeArtist(t, s)
	w := &RemoteWrite{ArtistID: artist, Images: []RemoteImage{{Kind: ArtistThumb, Source: ImageFromWikimedia, RemoteID: "File:A.jpg", Image: remoteArt("a")}}}
	if !mustWriteRemote(t, s, w) || mustWriteRemote(t, s, w) {
		t.Error("want the thumb written once")
	}
	// Another source's taking away leaves it; its own takes it.
	for _, c := range []struct {
		source string
		want   bool
	}{{ImageFromFanart, false}, {ImageFromWikimedia, true}} {
		if got := mustWriteRemote(t, s, &RemoteWrite{ArtistID: artist, Images: []RemoteImage{{Kind: ArtistThumb, Source: c.source}}}); got != c.want {
			t.Errorf("take away as %s: changed %v, want %v", c.source, got, c.want)
		}
	}
}

func TestAPinSurvivesRefreshes(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	artist := scrapeArtist(t, s)
	mustWriteRemote(t, s, &RemoteWrite{ArtistID: artist, Images: []RemoteImage{{Kind: ArtistBackground, Source: ImageFromFanart, RemoteID: "1", Image: remoteArt("one")}}})
	pinned := pin(t, s, artist, ArtistBackground, "pinned")
	for _, img := range []RemoteImage{
		{Kind: ArtistBackground, Source: ImageFromFanart, RemoteID: "2", Image: remoteArt("two")},
		{Kind: ArtistBackground, Source: ImageFromFanart},
	} {
		mustWriteRemote(t, s, &RemoteWrite{ArtistID: artist, Images: []RemoteImage{img}})
		if got := shownBackground(t, s); got != pinned {
			t.Errorf("after refresh %+v the background is %x, want the pin", img, got)
		}
	}
	for _, source := range []string{sourcePinned, string(ArtworkFolder)} { // Only a source's own: never a pin or a folder's.
		_, err := s.WriteRemote(t.Context(), &RemoteWrite{ArtistID: artist, MBID: scrapeMBID, Images: []RemoteImage{{Kind: ArtistBackground, Source: source}}}, time.Now())
		if !errors.Is(err, ErrInvalidWrite) {
			t.Errorf("taking away as %s: %v, want ErrInvalidWrite", source, err)
		}
	}
	if n := count(t, s, "SELECT count(*) FROM artist_pins WHERE artwork_hash = ?", pinned[:]); n != 1 {
		t.Errorf("%d pins, want the one", n)
	}
}

func TestAskingAgain(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	artist := scrapeArtist(t, s)
	later := testNow().Add(7 * 24 * time.Hour)
	mustWriteRemote(t, s, &RemoteWrite{ArtistID: artist, Answers: []ScrapeAnswer{
		{Source: ScrapeFanart, State: ScrapeState{Status: ScrapeNone, FetchedAt: testNow(), NextAt: later}},
		{Source: ScrapeWikimedia, State: ScrapeState{Status: ScrapeFailed, FetchedAt: testNow(), NextAt: later, Failures: 4, Error: "Wikipedia answered 503"}},
	}})
	soon := testNow().Add(time.Minute)
	n, err := s.AskAgain(t.Context(), []ScrapeSource{ScrapeFanart}, []string{scrapeMBID, "00000000-0000-4000-8000-000000000000"}, soon)
	if err != nil || n != 1 {
		t.Fatalf("ask again = %d, %v; want fanart.tv's state brought forward", n, err)
	}
	if st := statesOf(t, s); !st[ScrapeFanart].NextAt.Equal(soon) || !st[ScrapeWikimedia].NextAt.Equal(later) {
		t.Errorf("states = %+v", st)
	}
	// A folder that gives up an image asks every source again.
	folder := testArtwork("folder")
	folder.Source = ArtworkFolder
	gone := soon.Add(time.Minute)
	for _, img := range []*ArtworkWrite{folder, nil} {
		if _, err = s.WriteExtras(t.Context(), &ExtrasWrite{ArtistImages: []ArtistImageWrite{{ArtistID: artist, Kind: ArtistThumb, Image: img, File: folderFile("A/artist.jpg")}}}, gone); err != nil {
			t.Fatal(err)
		}
	}
	if st := statesOf(t, s); !st[ScrapeWikimedia].NextAt.Equal(gone) {
		t.Errorf("after the folder let go: %+v", st)
	}
}

func TestScrapeFeedRead(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	if _, ok, err := s.ScrapeFeedRead(t.Context(), ScrapeFanart); ok || err != nil {
		t.Fatalf("a new store has read the feed: %v, %v", ok, err)
	}
	if err := s.SetScrapeFeedRead(t.Context(), ScrapeFanart, testNow()); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := s.ScrapeFeedRead(t.Context(), ScrapeFanart); !ok || err != nil || !got.Equal(testNow()) {
		t.Errorf("feed read %v, %v, %v; want %v", got, ok, err, testNow())
	}
}
