package store

import (
	"errors"
	"testing"
	"time"
)

// pin pins an image drawn from seed as the artist's kind, uploaded by a new
// administrator, and returns its hash.
func pin(t *testing.T, s *Store, artist int64, kind ArtistImageKind, seed string) [32]byte {
	t.Helper()
	img := testArtwork(seed)
	img.Source = ArtworkUpload
	err := s.PinArtistImage(t.Context(), &PinWrite{ArtistID: artist, Kind: kind, Source: PinFromUpload, Image: *img, By: pinner(t, s)}, testNow())
	if err != nil {
		t.Fatal(err)
	}
	return img.Hash
}

// pinner writes an administrator, once, and returns its ID.
func pinner(t *testing.T, s *Store) int64 {
	t.Helper()
	if _, err := s.w.ExecContext(t.Context(), "INSERT OR IGNORE INTO users (id, name, password_hash, role, created_at) VALUES (9, 'simon', 'h', 'admin', 1)"); err != nil {
		t.Fatal(err)
	}
	return 9
}

// shownBackground is the hash of the background newWrite(0)'s artist shows.
func shownBackground(t *testing.T, s *Store) [32]byte {
	t.Helper()
	d, err := s.Artist(t.Context(), newWrite(0).Artists[0].ULID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Images.Background == nil {
		return [32]byte{}
	}
	return d.Images.Background.Hash
}

func TestAPinLiesOverWhatIsBeneath(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	artist := scrapeArtist(t, s)
	beneath := remoteArt("fanart bg")
	mustWriteRemote(t, s, &RemoteWrite{ArtistID: artist, Images: []RemoteImage{{Kind: ArtistBackground, Source: ImageFromFanart, RemoteID: "7", Image: beneath}}})
	pinned := pin(t, s, artist, ArtistBackground, "upload")

	d, err := s.Artist(t.Context(), newWrite(0).Artists[0].ULID)
	if err != nil {
		t.Fatal(err)
	}
	if bg := d.Images.Background; bg == nil || bg.Hash != pinned || bg.Source != sourcePinned {
		t.Fatalf("background = %+v, want the pin", bg)
	}
	p, err := s.ArtistPin(t.Context(), artist, ArtistBackground)
	if err != nil || p == nil || p.Source != PinFromUpload || p.By == "" || !p.At.Equal(testNow()) {
		t.Fatalf("pin = %+v, %v", p, err)
	}
	b, err := s.ArtistImageBeneath(t.Context(), artist, ArtistBackground)
	if err != nil || b == nil || b.Hash != beneath.Hash || b.Source != ImageFromFanart || b.RemoteID != "7" {
		t.Fatalf("beneath = %+v, %v; want fanart.tv's, kept", b, err)
	}
	held, err := s.ArtistHeld(t.Context(), artist)
	if err != nil || held.Images[ArtistBackground].Source != sourcePinned {
		t.Errorf("held = %+v, %v; the pass must see the pin", held.Images, err)
	}

	// Unpinning shows what is beneath at once, and asks the sources again.
	mustWriteRemote(t, s, &RemoteWrite{ArtistID: artist, Answers: []ScrapeAnswer{{Source: ScrapeFanart, State: ScrapeState{
		Status: ScrapeFound, FetchedAt: testNow(), NextAt: testNow().Add(30 * 24 * time.Hour),
	}}}})
	later := testNow().Add(time.Hour)
	gone, err := s.UnpinArtistImage(t.Context(), artist, ArtistBackground, later)
	if err != nil || !gone {
		t.Fatalf("unpin = %v, %v", gone, err)
	}
	if got := shownBackground(t, s); got != beneath.Hash {
		t.Errorf("after unpinning the background is %x, want fanart.tv's", got)
	}
	if st := statesOf(t, s)[ScrapeFanart]; !st.NextAt.Equal(later) {
		t.Errorf("fanart.tv's next ask = %v, want now", st.NextAt)
	}
	if again, err := s.UnpinArtistImage(t.Context(), artist, ArtistBackground, later); err != nil || again {
		t.Errorf("unpin again = %v, %v; want nothing to unpin", again, err)
	}
}

func TestPruneKeepsPins(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	artist := scrapeArtist(t, s)
	pinned := pin(t, s, artist, ArtistThumb, "kept")
	pruned, err := s.PruneOrphans(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range pruned.Artwork {
		if h == pinned {
			t.Fatal("prune took a pinned image")
		}
	}
	if n := count(t, s, "SELECT count(*) FROM artwork WHERE hash = ?", pinned[:]); n != 1 {
		t.Errorf("%d artwork rows for the pin, want 1", n)
	}
}

func TestPinsGoWithTheirArtist(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	artist := scrapeArtist(t, s)
	pin(t, s, artist, ArtistLogo, "logo")
	if err := s.HoldArtist(t.Context(), artist, scrapeMBID, pinner(t, s), testNow()); err != nil {
		t.Fatal(err)
	}
	// The artist leaves the store as every artist does: its tracks no longer
	// name it, and the prune takes it.
	if _, err := s.w.ExecContext(t.Context(), "DELETE FROM track_artists WHERE artist_id = ?", artist); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PruneOrphans(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT (SELECT count(*) FROM artist_pins) + (SELECT count(*) FROM artist_holds)"); n != 0 {
		t.Errorf("%d pins and holds left, want none", n)
	}
}

func TestPinWritesAreChecked(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	artist := scrapeArtist(t, s)
	img := *testArtwork("x")
	for _, w := range []PinWrite{
		{ArtistID: artist, Kind: ArtistThumb, Source: "deezer", Image: img},
		{ArtistID: artist, Kind: ArtistThumb, Source: PinFromFanart, Image: img},
		{ArtistID: artist, Kind: ArtistThumb, Source: PinFromUpload, RemoteID: "1", Image: img},
		{ArtistID: artist, Kind: "cover", Source: PinFromUpload, Image: img},
		{ArtistID: artist, Kind: ArtistThumb, Source: PinFromUpload},
	} {
		if err := s.PinArtistImage(t.Context(), &w, testNow()); !errors.Is(err, ErrInvalidWrite) {
			t.Errorf("pin %+v: %v, want ErrInvalidWrite", w, err)
		}
	}
}

func TestNotThisArtist(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	artist := scrapeArtist(t, s)
	folder := testArtwork("folder")
	folder.Source = ArtworkFolder
	if _, err := s.WriteExtras(t.Context(), &ExtrasWrite{
		ArtistImages: []ArtistImageWrite{{ArtistID: artist, Kind: ArtistThumb, Image: folder, File: folderFile("A/artist.jpg")}},
	}, testNow()); err != nil {
		t.Fatal(err)
	}
	mustWriteRemote(t, s, &RemoteWrite{
		ArtistID: artist,
		Images:   []RemoteImage{{Kind: ArtistBackground, Source: ImageFromFanart, RemoteID: "3", Image: remoteArt("bg")}},
		Bios:     []RemoteBio{{Source: BioFromWikipedia, Bio: &Bio{Summary: "Wiki.", Body: "Wiki.", License: "CC BY-SA 4.0"}}},
	})
	upload := pin(t, s, artist, ArtistLogo, "upload")
	fetched := remoteArt("pinned fanart")
	by := pinner(t, s)
	if err := s.PinArtistImage(t.Context(), &PinWrite{ArtistID: artist, Kind: ArtistBanner, Source: PinFromFanart, RemoteID: "9", Image: *fetched, By: by}, testNow()); err != nil {
		t.Fatal(err)
	}

	if err := s.HoldArtist(t.Context(), artist, scrapeMBID, by, testNow()); err != nil {
		t.Fatal(err)
	}
	d, err := s.Artist(t.Context(), newWrite(0).Artists[0].ULID)
	if err != nil {
		t.Fatal(err)
	}
	switch img := d.Images; {
	case img.Thumb == nil || img.Thumb.Hash != folder.Hash:
		t.Errorf("thumb = %+v, want the folder's, kept", img.Thumb)
	case img.Logo == nil || img.Logo.Hash != upload:
		t.Errorf("logo = %+v, want the upload's pin, kept", img.Logo)
	case img.Background != nil || img.Banner != nil || d.Bio != nil:
		t.Errorf("fetched art left: background %+v, banner %+v, bio %+v", img.Background, img.Banner, d.Bio)
	}
	a, err := s.ScrapeArtistByID(t.Context(), newWrite(0).Artists[0].ULID)
	if err != nil || !a.Held {
		t.Errorf("artist = %+v, %v; want it held", a, err)
	}
	all, err := s.ScrapeArtists(t.Context())
	if err != nil || len(all) != 1 || !all[0].Held {
		t.Errorf("artists = %+v, %v; want it held", all, err)
	}

	// A new MBID ends the hold with no write.
	if _, err = s.w.ExecContext(t.Context(), "UPDATE artists SET mbid = '00000000-0000-4000-8000-0000000000aa' WHERE id = ?", artist); err != nil {
		t.Fatal(err)
	}
	if a, err = s.ScrapeArtistByID(t.Context(), newWrite(0).Artists[0].ULID); err != nil || a.Held {
		t.Errorf("after a new MBID: %+v, %v; want it asked about again", a, err)
	}
	if err := s.HoldArtist(t.Context(), artist, "not an mbid", by, testNow()); !errors.Is(err, ErrInvalidWrite) {
		t.Errorf("hold by a bad MBID: %v", err)
	}
}

func TestPreviews(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	p := picture(testArtwork("preview").Hash[:], 400, 225, "#112233", "#010203", 0)
	if err := s.PutArtworkPreview(t.Context(), ImageFromFanart, "12", &p, testNow()); err != nil {
		t.Fatal(err)
	}
	got, err := s.ArtworkPreview(t.Context(), ImageFromFanart, "12", testNow().Add(-time.Hour))
	if err != nil || got == nil || *got != p {
		t.Fatalf("preview = %+v, %v; want %+v", got, err, p)
	}
	if got, err = s.ArtworkPreview(t.Context(), ImageFromFanart, "12", testNow().Add(time.Hour)); got != nil || err != nil {
		t.Errorf("a stale preview = %+v, %v; want none", got, err)
	}

	// A preview whose hash an artwork row also uses keeps its files.
	shared := testArtwork("shared")
	shared.Source = ArtworkRemote
	q := picture(shared.Hash[:], 400, 400, "#112233", "#010203", 0)
	if err = s.PutArtworkPreview(t.Context(), ImageFromWikimedia, "File:A.jpg", &q, testNow()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.WriteRemote(t.Context(), &RemoteWrite{ArtistID: scrapeArtist(t, s), MBID: scrapeMBID, Images: []RemoteImage{{Kind: ArtistThumb, Source: ImageFromWikimedia, RemoteID: "File:A.jpg", Image: shared}}}, testNow()); err != nil {
		t.Fatal(err)
	}
	unused, err := s.SweepPreviews(t.Context(), testNow().Add(time.Minute))
	if err != nil || len(unused) != 1 || unused[0] != p.Hash {
		t.Errorf("sweep = %x, %v; want the first preview's hash alone", unused, err)
	}
	if n := count(t, s, "SELECT count(*) FROM artwork_previews"); n != 0 {
		t.Errorf("%d previews left", n)
	}
}
