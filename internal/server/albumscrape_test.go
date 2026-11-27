package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/lastfm/lastfmtest"
	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/matching/musicbrainztest"
	"github.com/simonhull/kitsune/internal/scrape"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv/fanarttvtest"
	"github.com/simonhull/kitsune/internal/store"
)

// What the tests here hold songs and sources to.
const (
	scrapedSong = "Song"
	keyIsSet    = "set"
)

// releaseOf is the i-th test album's release.
func releaseOf(i int) string { return musicbrainztest.ID("release " + strconv.Itoa(i)) }

// albumWrite is the i-th test album's one track, with its release MBID and,
// when given, a cover from its files.
func albumWrite(i int, cover *store.ArtworkWrite) store.TrackWrite {
	name := "Pressing " + strconv.Itoa(i)
	return store.TrackWrite{
		ULID: fmt.Sprintf("01P%023d", i), Root: musicBase, RelPath: fmt.Sprintf("p/%d.flac", i), Size: 1000,
		MTime: time.UnixMilli(1_761_000_000_000), Digest: new(sha256.Sum256([]byte(name))),
		Format: flacFormat, Codec: flacFormat, Duration: 200 * time.Second, Title: scrapedSong, SortTitle: scrapedSong,
		Album: store.AlbumWrite{
			GroupKey: "pressing " + strconv.Itoa(i), ULID: fmt.Sprintf("02P%023d", i), Artist: name,
			Name: name, SortName: name, MBID: releaseOf(i), Artwork: cover,
		},
		Artists: []store.ArtistWrite{{ULID: fmt.Sprintf("%026d", 700+i), MatchKey: strings.ToLower(name), Name: name}},
	}
}

// addAlbum writes the i-th test album and returns its public ID.
func (ss *scrapeServer) addAlbum(t *testing.T, i int, cover *store.ArtworkWrite) string {
	t.Helper()
	w := albumWrite(i, cover)
	if err := ss.Store.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{w}}); err != nil {
		t.Fatal(err)
	}
	return w.Album.ULID
}

func (ss *scrapeServer) album(t *testing.T, token, id string) api.AlbumDetail {
	t.Helper()
	r := ss.send(t, call{method: http.MethodGet, path: "/albums/" + id, header: bearer(token)})
	if r.Status != http.StatusOK {
		t.Fatalf("album: status %d", r.Status)
	}
	var a api.AlbumDetail
	if err := json.Unmarshal(r.Body, &a); err != nil {
		t.Fatal(err)
	}
	return a
}

// filesCover is a cover as a scan finds one in an album's files.
func filesCover() *store.ArtworkWrite {
	return &store.ArtworkWrite{
		Hash: sha256.Sum256([]byte("a cover in the files")), Width: 600, Height: 600,
		Accent: "#aa3300", Ground: "#110000", Source: store.ArtworkEmbedded,
	}
}

// The Cover Art Archive's front is an album's cover everywhere one is read
// while its files have none, credited, with its back cover; the moment a scan
// finds a cover in the files, the files' is the album's.
func TestArchiveCoverIsTheAlbumsUntilItsFilesHaveOne(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	id := ss.addAlbum(t, 1, nil)
	ss.mb.Add(matching.Release{ID: releaseOf(1), Cover: true})
	ss.mb.AddBack(releaseOf(1))
	tap := ss.Events.Subscribe(events.Viewer{UserID: 99}, "")
	defer ss.Events.Unsubscribe(tap)
	<-tap.C() // Ready.
	token := ss.tokenFor(t, store.RoleUser)

	ss.Scraper.Pass(t.Context())

	ss.checkArchiveCover(t, token, id)
	if heard := drainTap(tap); !slices.ContainsFunc(heard, func(h string) bool { return strings.HasPrefix(h, "library") && strings.Contains(h, id) }) {
		t.Errorf("events = %q, want a library event for the album", heard)
	}
	if m := ss.metadata(t, ss.tokenFor(t, store.RoleAdmin)); m.Albums.FromArchive != 1 || m.Albums.WithoutCover != 0 || m.Sources[4].State != "on" {
		t.Errorf("metadata = %+v", m)
	}

	files := ss.coverInFiles(t)
	if a := ss.album(t, token, id); a.Artwork == nil || a.Artwork.Hash != hex.EncodeToString(files.Hash[:]) ||
		a.Extras.CoverSource != api.CoverFromFiles || a.Extras.Back == nil {
		t.Errorf("after a scan found one: %+v, want the files' cover and the archive's back still", a)
	}
}

// coverInFiles writes another song of the first test album, with a cover in
// its file, as a scan that found one would, and returns the cover.
func (ss *scrapeServer) coverInFiles(t *testing.T) *store.ArtworkWrite {
	t.Helper()
	files := filesCover()
	second := albumWrite(1, files)
	second.ULID, second.RelPath = fmt.Sprintf("01Q%023d", 1), "p/1b.flac"
	if err := ss.Store.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{second}}); err != nil {
		t.Fatal(err)
	}
	return files
}

// checkArchiveCover holds the album id to the archive's front as its cover in
// its page, the album list and the index, and its back, credited.
func (ss *scrapeServer) checkArchiveCover(t *testing.T, token, id string) {
	t.Helper()
	a := ss.album(t, token, id)
	if a.Artwork == nil || a.Extras.CoverSource != api.CoverFromArchive {
		t.Fatalf("album = %+v, want the archive's cover", a)
	}
	if b := a.Extras.Back; b == nil || b.Source != store.ImageFromArchive || b.Credit == nil || !strings.Contains(b.Credit.PageURL, releaseOf(1)) {
		t.Errorf("back = %+v, want the archive's, credited", b)
	}
	for _, path := range []string{"/albums", indexPath} {
		if r := ss.send(t, call{method: http.MethodGet, path: path, header: bearer(token)}); !strings.Contains(string(r.Body), a.Artwork.Hash) {
			t.Errorf("%s doesn't hold the archive's cover", path)
		}
	}
}

// An album whose files hold a cover never takes the archive's front; its back
// still comes, as its folder has none.
func TestArchiveNeverReplacesAFilesCover(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	files := filesCover()
	id := ss.addAlbum(t, 2, files)
	ss.mb.Add(matching.Release{ID: releaseOf(2), Cover: true})
	ss.mb.AddBack(releaseOf(2))
	ss.Scraper.Pass(t.Context())
	a := ss.album(t, ss.tokenFor(t, store.RoleUser), id)
	if a.Artwork == nil || a.Artwork.Hash != hex.EncodeToString(files.Hash[:]) || a.Extras.CoverSource != api.CoverFromFiles {
		t.Errorf("album = %+v, want its own cover", a)
	}
	if a.Extras.Back == nil || a.Extras.Back.Source != store.ImageFromArchive {
		t.Errorf("back = %+v, want the archive's", a.Extras.Back)
	}
}

// An album with a release but no release group gets one from MusicBrainz, and
// by it fanart.tv's disc art, transparent; a folder's disc art puts it aside.
func TestDiscArtByALookedUpReleaseGroup(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	group := musicbrainztest.ID("group 3")
	id := ss.addAlbum(t, 3, filesCover())
	ss.mb.Add(matching.Release{ID: releaseOf(3), GroupID: group})
	ss.ft.AddAlbum(fanarttvtest.Album{MBID: group, Discs: []fanarttvtest.Disc{{ID: "81", Disc: 1}}})
	token := ss.tokenFor(t, store.RoleUser)

	ss.Scraper.Pass(t.Context())

	a := ss.album(t, token, id)
	if len(a.Extras.Discs) != 1 || a.Extras.Discs[0].Disc != 0 || a.Extras.Discs[0].Image.Source != store.ImageFromFanart || !a.Extras.Discs[0].Image.Alpha {
		t.Fatalf("discs = %+v, want fanart.tv's, transparent, for every disc", a.Extras.Discs)
	}
	albums, err := ss.Store.ScrapeAlbums(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(albums, func(x store.ScrapeAlbum) bool { return x.ULID == id }); i < 0 || albums[i].GroupMBID != group {
		t.Errorf("albums = %+v, want the looked-up release group kept", albums)
	}
	calls := ss.ft.Calls()
	if !slices.ContainsFunc(calls, func(c fanarttvtest.Call) bool { return strings.HasSuffix(c.Path, "/albums/"+group) }) {
		t.Errorf("fanart.tv calls = %+v, want the release group asked", calls)
	}

	ss.folderDiscArt(t, token, id, albums[slices.IndexFunc(albums, func(x store.ScrapeAlbum) bool { return x.ULID == id })].ID)
}

// folderDiscArt gives the album id (row) disc art from its folder, which puts
// fanart.tv's aside.
func (ss *scrapeServer) folderDiscArt(t *testing.T, token, id string, row int64) {
	t.Helper()
	art := filesCover()
	art.Hash, art.Alpha = sha256.Sum256([]byte("cdart.png")), true
	w := &store.ExtrasWrite{AlbumImages: []store.AlbumImageWrite{{
		AlbumID: row, AlbumULID: id, Kind: store.AlbumDisc, Disc: store.AllDiscs, Image: art,
		File: store.FileRef{Root: musicBase, RelPath: "p/cdart.png", Size: 10, MTime: time.UnixMilli(1)},
	}}}
	if _, err := ss.Store.WriteExtras(t.Context(), w, time.Now()); err != nil {
		t.Fatal(err)
	}
	if a := ss.album(t, token, id); len(a.Extras.Discs) != 1 || a.Extras.Discs[0].Image.Source != "folder" {
		t.Errorf("discs = %+v, want the folder's alone", a.Extras.Discs)
	}
}

// Last.fm gives the bio of an artist nobody else has one of, credited and
// shortened, and never one it gives for another artist's MBID; an artist with
// Wikipedia's bio never sends Last.fm a request.
func TestLastfmGivesABioOnlyWhenNoOneElseDoes(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	withWiki := ss.addArtist(t, 1)
	lone, other := ss.addArtist(t, 2), ss.addArtist(t, 3)
	for _, i := range []int{2, 3} {
		ss.mb.AddArtist(musicbrainztest.Artist{ID: musicbrainztest.ID("scraped " + strconv.Itoa(i)), Name: "No item"})
	}
	page := "https://www.last.fm/music/Scraped+2"
	ss.lf.AddArtist(
		lastfmtest.Artist{
			MBID:    musicbrainztest.ID("scraped 2"),
			Summary: lastfmtest.Bio(page, false, "Scraped 2 are a duo."),
			Content: lastfmtest.Bio(page, true, `Scraped 2 are a <a href="https://www.last.fm/tag/duo">duo</a>.`, "They tour."),
		},
		lastfmtest.Artist{MBID: musicbrainztest.ID("scraped 3"), Answered: musicbrainztest.ID("someone else"), Content: lastfmtest.Bio(page, true, "Not them.")},
	)
	ss.Scraper.Pass(t.Context())
	token := ss.tokenFor(t, store.RoleUser)

	checkLastfmBio(t, ss.artistDetail(t, token, lone).Bio, page)
	if d := ss.artistDetail(t, token, other); d.Bio != nil {
		t.Errorf("bio = %+v, want none: Last.fm answered for another artist", d.Bio)
	}
	if d := ss.artistDetail(t, token, withWiki); d.Bio == nil || d.Bio.Source != store.BioFromWikipedia {
		t.Errorf("bio = %+v, want Wikipedia's", d.Bio)
	}
	if slices.ContainsFunc(ss.lf.CallsTo("artist.getInfo"), func(c lastfmtest.Call) bool { return c.Params.Get("mbid") == musicbrainztest.ID("scraped 1") }) {
		t.Errorf("Last.fm was asked about an artist Wikipedia gave a bio")
	}
	if lfm := ss.metadata(t, ss.tokenFor(t, store.RoleAdmin)).Sources[3]; lfm.Name != "lastfm" || lfm.State != "on" || lfm.Key != keyIsSet {
		t.Errorf("lastfm = %+v", lfm)
	}
}

// checkLastfmBio holds b to Last.fm's, credited, its links and license line
// gone and its "Read more" kept as its page.
func checkLastfmBio(t *testing.T, b *api.ArtistBio, page string) {
	t.Helper()
	if b == nil || b.Source != store.BioFromLastfm || b.License != "CC BY-SA" || b.URL != page || b.LicenseURL == "" {
		t.Errorf("bio = %+v, want Last.fm's, credited", b)
		return
	}
	if b.Summary != "Scraped 2 are a duo." || b.Body != "Scraped 2 are a duo.\n\nThey tour." {
		t.Errorf("bio text = %q / %q", b.Summary, b.Body)
	}
}

func (ss *scrapeServer) artistDetail(t *testing.T, token, id string) api.ArtistDetail {
	t.Helper()
	var d api.ArtistDetail
	if err := json.Unmarshal(ss.send(t, call{method: http.MethodGet, path: "/artists/" + id, header: bearer(token)}).Body, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// Without Last.fm's API key, bios from it are on but wait for the key, and
// the card says so; nothing is asked.
func TestLastfmWithoutItsKeyNeedsOne(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	cfg := ss.cfg
	cfg.Lastfm = nil
	ss.Scraper.Configure(cfg)
	ss.addArtist(t, 1)
	ss.Scraper.Pass(t.Context())
	admin := ss.tokenFor(t, store.RoleAdmin)
	m := ss.metadata(t, admin)
	if lfm := m.Sources[3]; lfm.State != "needsKey" || lfm.Key != string(scrape.KeyMissing) || !lfm.Enabled {
		t.Errorf("lastfm = %+v, want waiting for its key", lfm)
	}
	if calls := ss.lf.Calls(); len(calls) != 0 {
		t.Errorf("Last.fm was asked %d times without a key", len(calls))
	}
	cfg.Archive = false
	ss.Scraper.Configure(cfg)
	if caa := ss.metadata(t, admin).Sources[4]; caa.State != "off" || caa.Enabled {
		t.Errorf("coverartarchive = %+v, want off", caa)
	}
}
