package store

import (
	"errors"
	"testing"
	"time"
)

// wikiBio is a bio as Wikipedia gives one.
func wikiBio() *Bio {
	const lead = "Wiki."
	return &Bio{Summary: lead, Body: lead, URL: "https://en.wikipedia.org/wiki/X", License: "CC BY-SA 4.0", Lang: "en"}
}

// pin pins an image drawn from seed as the artist's kind, uploaded by an
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

// shown is what newWrite(0)'s artist shows.
func shown(t *testing.T, s *Store) ArtistDetail {
	t.Helper()
	d, err := s.Artist(t.Context(), newWrite(0).Artists[0].ULID)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// shownBackground is the hash of the background newWrite(0)'s artist shows.
func shownBackground(t *testing.T, s *Store) [32]byte {
	t.Helper()
	if bg := shown(t, s).Images.Background; bg != nil {
		return bg.Hash
	}
	return [32]byte{}
}

// pinnedArt is newWrite(0)'s artist with fanart.tv's background (ID 7)
// beneath an uploaded pin: the artist's row and both images' hashes.
type pinnedArt struct {
	s               *Store
	artist          int64
	beneath, pinned [32]byte
}

func pinnedOverFanart(t *testing.T) pinnedArt {
	t.Helper()
	s := openTest(t)
	artist := scrapeArtist(t, s)
	art := remoteArt("fanart bg")
	mustWriteRemote(t, s, &RemoteWrite{ArtistID: artist, Images: []RemoteImage{{Kind: ArtistBackground, Source: ImageFromFanart, RemoteID: "7", Image: art}}})
	return pinnedArt{s: s, artist: artist, beneath: art.Hash, pinned: pin(t, s, artist, ArtistBackground, "upload")}
}

func TestAPinLiesOverWhatIsBeneath(t *testing.T) {
	t.Parallel()
	f := pinnedOverFanart(t)
	if bg := shown(t, f.s).Images.Background; bg == nil || bg.Hash != f.pinned || bg.Source != sourcePinned {
		t.Fatalf("background = %+v, want the pin", bg)
	}
	p, err := f.s.ArtistPin(t.Context(), f.artist, ArtistBackground)
	if err != nil || p == nil || p.Source != PinFromUpload || p.By != "simon" || !p.At.Equal(testNow()) {
		t.Fatalf("pin = %+v, %v", p, err)
	}
}

func TestWhatIsBeneathAPinIsKept(t *testing.T) {
	t.Parallel()
	f := pinnedOverFanart(t)
	h, err := f.s.ArtistHeld(t.Context(), f.artist)
	if err != nil || h.Images[ArtistBackground].Source != sourcePinned {
		t.Errorf("held = %+v, %v; the pass must see the pin", h.Images, err)
	}
	b, err := f.s.ArtistImageBeneath(t.Context(), f.artist, ArtistBackground)
	if err != nil || b == nil || b.Hash != f.beneath || b.Source != ImageFromFanart || b.RemoteID != "7" {
		t.Fatalf("beneath = %+v, %v; want fanart.tv's, kept", b, err)
	}
}

func TestUnpinningShowsWhatIsBeneath(t *testing.T) {
	t.Parallel()
	f := pinnedOverFanart(t)
	s, artist, beneath := f.s, f.artist, f.beneath
	mustWriteRemote(t, s, &RemoteWrite{ArtistID: artist, Answers: []ScrapeAnswer{{Source: ScrapeFanart, State: ScrapeState{
		Status: ScrapeFound, FetchedAt: testNow(), NextAt: testNow().Add(30 * 24 * time.Hour),
	}}}})
	later := testNow().Add(time.Hour)
	gone, err := s.UnpinArtistImage(t.Context(), artist, ArtistBackground, later)
	if err != nil || !gone {
		t.Fatalf("unpin = %v, %v", gone, err)
	}
	if got := shownBackground(t, s); got != beneath {
		t.Errorf("after unpinning the background is %x, want fanart.tv's", got)
	}
	if st := statesOf(t, s)[ScrapeFanart]; !st.NextAt.Equal(later) {
		t.Errorf("fanart.tv's next ask = %v, want now: the pin kept it from filling the kind", st.NextAt)
	}
	if gone, err = s.UnpinArtistImage(t.Context(), artist, ArtistBackground, later); err != nil || gone {
		t.Errorf("unpin again = %v, %v; want nothing to unpin", gone, err)
	}
}

func TestPruneKeepsPins(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	pinned := pin(t, s, scrapeArtist(t, s), ArtistThumb, "kept")
	if _, err := s.PruneOrphans(t.Context()); err != nil {
		t.Fatal(err)
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

// held is newWrite(0)'s artist with a folder's thumb, fanart.tv's background,
// Wikipedia's bio, an uploaded logo pinned and fanart.tv's banner pinned, then
// held back by Not this artist: the artist's row, and the hashes of the
// folder's thumb and the upload.
type held struct {
	s              *Store
	artist         int64
	folder, upload [32]byte
}

func heldArtist(t *testing.T) held {
	t.Helper()
	s := openTest(t)
	artist := scrapeArtist(t, s)
	thumb := testArtwork("folder")
	thumb.Source = ArtworkFolder
	if _, err := s.WriteExtras(t.Context(), &ExtrasWrite{
		ArtistImages: []ArtistImageWrite{{ArtistID: artist, Kind: ArtistThumb, Image: thumb, File: folderFile("A/artist.jpg")}},
	}, testNow()); err != nil {
		t.Fatal(err)
	}
	mustWriteRemote(t, s, &RemoteWrite{
		ArtistID: artist,
		Images:   []RemoteImage{{Kind: ArtistBackground, Source: ImageFromFanart, RemoteID: "3", Image: remoteArt("bg")}},
		Bios:     []RemoteBio{{Source: BioFromWikipedia, Bio: wikiBio()}},
	})
	upload := pin(t, s, artist, ArtistLogo, "upload")
	by := pinner(t, s)
	banner := PinWrite{ArtistID: artist, Kind: ArtistBanner, Source: PinFromFanart, RemoteID: "9", Image: *remoteArt("pinned fanart"), By: by}
	if err := s.PinArtistImage(t.Context(), &banner, testNow()); err != nil {
		t.Fatal(err)
	}
	if err := s.HoldArtist(t.Context(), artist, scrapeMBID, by, testNow()); err != nil {
		t.Fatal(err)
	}
	return held{s: s, artist: artist, folder: thumb.Hash, upload: upload}
}

func TestNotThisArtistTakesWhatWasFetched(t *testing.T) {
	t.Parallel()
	h := heldArtist(t)
	s, folder, upload := h.s, h.folder, h.upload
	d := shown(t, s)
	switch img := d.Images; {
	case img.Thumb == nil || img.Thumb.Hash != folder:
		t.Errorf("thumb = %+v, want the folder's, kept", img.Thumb)
	case img.Logo == nil || img.Logo.Hash != upload:
		t.Errorf("logo = %+v, want the upload's pin, kept", img.Logo)
	case img.Background != nil || img.Banner != nil || d.Bio != nil:
		t.Errorf("fetched art left: background %+v, banner %+v, bio %+v", img.Background, img.Banner, d.Bio)
	}
	if err := s.HoldArtist(t.Context(), 1, "not an mbid", pinner(t, s), testNow()); !errors.Is(err, ErrInvalidWrite) {
		t.Errorf("hold by a bad MBID: %v", err)
	}
}

func TestAHoldLastsWhileTheMBIDDoes(t *testing.T) {
	t.Parallel()
	h := heldArtist(t)
	s, artist := h.s, h.artist
	id := newWrite(0).Artists[0].ULID
	a, err := s.ScrapeArtistByID(t.Context(), id)
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
	if a, err = s.ScrapeArtistByID(t.Context(), id); err != nil || a.Held {
		t.Errorf("after a new MBID: %+v, %v; want it asked about again", a, err)
	}
}

func TestPreviewsAreKeptADay(t *testing.T) {
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
}

func TestSweepingPreviewsKeepsWhatIsUsed(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	alone := picture(testArtwork("preview").Hash[:], 400, 225, "#112233", "#010203", 0)
	if err := s.PutArtworkPreview(t.Context(), ImageFromFanart, "12", &alone, testNow()); err != nil {
		t.Fatal(err)
	}
	// A preview whose hash an artwork row also uses keeps its files.
	shared := remoteArt("shared")
	both := picture(shared.Hash[:], 400, 400, "#112233", "#010203", 0)
	if err := s.PutArtworkPreview(t.Context(), ImageFromWikimedia, "File:A.jpg", &both, testNow()); err != nil {
		t.Fatal(err)
	}
	mustWriteRemote(t, s, &RemoteWrite{ArtistID: scrapeArtist(t, s), Images: []RemoteImage{{Kind: ArtistThumb, Source: ImageFromWikimedia, RemoteID: "File:A.jpg", Image: shared}}})
	unused, err := s.SweepPreviews(t.Context(), testNow().Add(time.Minute))
	if err != nil || len(unused) != 1 || unused[0] != alone.Hash {
		t.Errorf("sweep = %x, %v; want the first preview's hash alone", unused, err)
	}
	if n := count(t, s, "SELECT count(*) FROM artwork_previews"); n != 0 {
		t.Errorf("%d previews left", n)
	}
}
