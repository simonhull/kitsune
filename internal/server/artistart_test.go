package server

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

// What artFixture's library holds beside its songs.
const (
	artistFolder = "Band"
	nfoBio       = "Band formed in 1987."
)

// artFixture is a scanned library of one artist's album in its own folders,
// with every artist image, a bio and the album's extras beside the songs.
type artFixture struct {
	*editFixture

	artist api.ArtistDetail
}

// paint returns a w x h PNG of c; transparent makes all but its middle third
// transparent, as a logo is.
func paint(t *testing.T, w, h int, c color.NRGBA, transparent bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			px := c
			if transparent && (y < h/3 || y >= 2*h/3) {
				px.A = 0
			}
			img.SetNRGBA(x, y, px)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeExtras writes the artist's and the album's files into root.
func writeExtras(t *testing.T, root string) {
	t.Helper()
	files := map[string][]byte{
		"artist.jpg":         paint(t, 600, 600, color.NRGBA{R: 0xC0, G: 0x40, B: 0x40, A: 0xFF}, false),
		"fanart.jpg":         paint(t, 3840, 2160, color.NRGBA{R: 0x40, G: 0x40, B: 0xC0, A: 0xFF}, false),
		"clearlogo.png":      paint(t, 800, 310, color.NRGBA{R: 0xF0, G: 0xA0, B: 0x20, A: 0xFF}, true),
		"banner.jpg":         paint(t, 1000, 185, color.NRGBA{R: 0x20, G: 0x90, B: 0x40, A: 0xFF}, false),
		"artist.nfo":         []byte("<artist><biography>" + nfoBio + "\nThey split in 1994.</biography></artist>"),
		"Album/back.jpg":     paint(t, 500, 500, color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF}, false),
		"Album/discart.png":  paint(t, 500, 500, color.NRGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xFF}, true),
		"Album/cd2.png":      paint(t, 400, 400, color.NRGBA{R: 0x30, G: 0x10, B: 0x10, A: 0xFF}, true),
		"Album/not-art.webp": []byte("not an image"),
	}
	for rel, data := range files {
		path := filepath.Join(root, artistFolder, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func newArtFixture(t *testing.T) *artFixture {
	t.Helper()
	fx := newEditFixture(t, &testServerOptions{Artwork: artwork.NewCache(t.TempDir())}, map[string]string{
		artistFolder + "/Album/one.flac": sampleFLAC, artistFolder + "/Album/two.flac": sampleFLAC,
	})
	writeExtras(t, fx.root)
	if _, err := fx.Scanner.Scan(t.Context(), library.Quick); err != nil {
		t.Fatal(err)
	}
	fx.reload(t)
	af := &artFixture{editFixture: fx}
	fx.getJSON(t, fx.token, artistsPath+"/"+fx.album.Artists[0].ID, &af.artist)
	return af
}

func TestArtistCarriesItsFolder(t *testing.T) {
	t.Parallel()
	fx := newArtFixture(t)
	wantFolderImages(t, &fx.artist.Images)
	if b := fx.artist.Bio; b == nil || b.Source != "nfo" || b.Summary != nfoBio || !strings.HasPrefix(b.Body, nfoBio+"\n\n") || b.URL != "" {
		t.Errorf("bio = %+v", fx.artist.Bio)
	}
	wantAlbumExtras(t, &fx.album)
	var list api.ArtistList
	fx.getJSON(t, fx.token, artistsPath, &list)
	if len(list.Artists) != 1 || list.Artists[0].Thumb == nil || list.Artists[0].Thumb.Hash != fx.artist.Images.Thumb.Hash {
		t.Errorf("artists = %+v, want the thumb on the entry", list.Artists)
	}
}

// wantAlbumExtras checks a's extras are writeExtras's, and that the cover its
// songs hold is said to be the files'.
func wantAlbumExtras(t *testing.T, a *api.AlbumDetail) {
	t.Helper()
	ex := a.Extras
	if ex.Back == nil || len(ex.Discs) != 2 || ex.Discs[0].Disc != 0 || ex.Discs[1].Disc != 2 || !ex.Discs[0].Image.Alpha {
		t.Errorf("extras = %+v, want a back cover and discs 0 and 2 with alpha", ex)
	}
	if want := map[bool]string{true: api.CoverFromFiles}[a.Artwork != nil]; ex.CoverSource != want {
		t.Errorf("coverSource = %q with artwork %+v, want %q", ex.CoverSource, a.Artwork, want)
	}
}

// wantFolderImages checks imgs are writeExtras's: every kind from the folder
// with no credit, the logo alone with alpha, the background 4K.
func wantFolderImages(t *testing.T, imgs *api.ArtistImages) {
	t.Helper()
	for name, img := range map[string]*api.ExtraImage{"thumb": imgs.Thumb, "background": imgs.Background, "logo": imgs.Logo, "banner": imgs.Banner} {
		if img == nil || img.Source != "folder" || img.Credit != nil {
			t.Fatalf("%s = %+v, want a folder's, with no credit", name, img)
		}
	}
	if !imgs.Logo.Alpha || imgs.Thumb.Alpha || imgs.Background.Width != 3840 {
		t.Errorf("logo %+v, thumb %+v, background %+v: want the logo alone with alpha, a 4K background", imgs.Logo, imgs.Thumb, imgs.Background)
	}
}

// Nothing a response carries of an artist's art names a root or a file.
func TestArtistArtNamesNoPath(t *testing.T) {
	t.Parallel()
	fx := newArtFixture(t)
	for _, path := range []string{artistsPath, artistsPath + "/" + fx.artist.ID, fx.albumPath()} {
		body := string(fx.send(t, call{method: http.MethodGet, path: path, header: bearer(fx.token)}).Body)
		for _, leak := range []string{fx.root, artistFolder + "/", "artist.jpg", "artist.nfo", "discart"} {
			if strings.Contains(body, leak) {
				t.Errorf("GET %s holds %q", path, leak)
			}
		}
	}
}

func TestArtworkServesEveryKind(t *testing.T) {
	t.Parallel()
	fx := newArtFixture(t)
	imgs := fx.artist.Images
	tests := []struct {
		name, hash string
		size       string
		typ        string
		width      int
	}{
		{"a 4K background at 3840", imgs.Background.Hash, "3840", mediaJPEG, 3840},
		{"a 4K background at 1920", imgs.Background.Hash, "1920", mediaJPEG, 1920},
		{"a 4K background at 640", imgs.Background.Hash, "640", mediaJPEG, 640},
		{"a background for a cover's size", imgs.Background.Hash, "1024", mediaJPEG, 640},
		{"a logo as PNG", imgs.Logo.Hash, "800", mediaPNG, 800},
		{"a logo at 400", imgs.Logo.Hash, "512", mediaPNG, 400},
		{"a thumb at 512", imgs.Thumb.Hash, "512", mediaJPEG, 512},
		{"a disc as PNG", fx.album.Extras.Discs[0].Image.Hash, "400", mediaPNG, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := fx.send(t, call{method: http.MethodGet, path: artworkPath + tt.hash + "?size=" + tt.size, header: bearer(fx.token)})
			if resp.Status != http.StatusOK {
				t.Fatalf("status = %d: %s", resp.Status, resp.Body)
			}
			if got := resp.Header.Get("Content-Type"); got != tt.typ {
				t.Errorf("Content-Type = %q, want %q", got, tt.typ)
			}
			cfg, _, err := image.DecodeConfig(bytes.NewReader(resp.Body))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Width != tt.width {
				t.Errorf("width = %d, want %d", cfg.Width, tt.width)
			}
		})
	}
}

// An artist with nothing in its folder still has images, all absent.
func TestArtistWithNoFolderHasNoArt(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{}, twoFLACs())
	var d api.ArtistDetail
	fx.getJSON(t, fx.token, artistsPath+"/"+fx.album.Artists[0].ID, &d)
	if d.Images != (api.ArtistImages{}) || d.Bio != nil {
		t.Errorf("images = %+v, bio = %+v, want none", d.Images, d.Bio)
	}
	if fx.album.Extras.Discs == nil || len(fx.album.Extras.Discs) != 0 || fx.album.Extras.Back != nil {
		t.Errorf("extras = %+v, want none, with an empty list of discs", fx.album.Extras)
	}
}

// A scan that changes an artist's art tells everyone, by the artist's ID.
func TestScanTellsEveryoneOfAnArtistsArt(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{Artwork: artwork.NewCache(t.TempDir())}, map[string]string{
		artistFolder + "/Album/one.flac": sampleFLAC,
	})
	admin := fx.openStream(t, fx.token, "")
	someone := fx.openStream(t, fx.tokenFor(t, store.RoleUser), "")
	admin.want(events.Ready)
	someone.want(events.Ready)
	writeExtras(t, fx.root)
	if _, err := fx.Scanner.Scan(t.Context(), library.Quick); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*eventStream{admin, someone} {
		var ev api.EventArtists
		decodeEvent(t, s.until(events.Artists), &ev)
		if len(ev.IDs) != 1 || ev.IDs[0] != fx.album.Artists[0].ID {
			t.Errorf("artists event = %+v, want [%s]", ev, fx.album.Artists[0].ID)
		}
	}
}
