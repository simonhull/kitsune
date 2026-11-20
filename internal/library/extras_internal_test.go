package library

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/store"
)

func TestExtraNames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		kind store.ArtistImageKind
		want bool
	}{
		{"artist.jpg", store.ArtistThumb, true},
		{"Thumb.PNG", store.ArtistThumb, true},
		{"folder.webp", store.ArtistThumb, true},
		{"artist.gif", store.ArtistThumb, false},
		{"fanart.jpg", store.ArtistBackground, true},
		{"fanart1.jpg", store.ArtistBackground, true},
		{"Backdrop-2.jpeg", store.ArtistBackground, true},
		{"background.png", store.ArtistBackground, true},
		{"background1.png", store.ArtistBackground, false},
		{"clearlogo.png", store.ArtistLogo, true},
		{"logo.jpg", store.ArtistLogo, true},
		{"banner.jpg", store.ArtistBanner, true},
		{"cover.jpg", store.ArtistThumb, false},
	}
	for _, tt := range tests {
		k := artistKinds[tt.kind]
		if _, ok := k.rank(tt.name); ok != tt.want {
			t.Errorf("%s as %s = %v, want %v", tt.name, tt.kind, ok, tt.want)
		}
		if tt.want && !isExtraFile(tt.name) {
			t.Errorf("a walk drops %s", tt.name)
		}
	}
	if isExtraFile("cover.jpg") || isExtraFile("01 Song.flac") {
		t.Error("a walk keeps a file no extra is named")
	}
	if !isExtraFile("Artist.NFO") || !isExtraFile("back.jpg") {
		t.Error("a walk drops an artist.nfo or a back cover")
	}
}

// Earlier names win, then earlier extensions, then a numbered name, then the
// name sorted first.
func TestExtraCandidatesOrder(t *testing.T) {
	t.Parallel()
	files := func(names ...string) []*seenFile {
		out := make([]*seenFile, len(names))
		for i, n := range names {
			out[i] = &seenFile{rel: filepath.Join("Band", n)}
		}
		return out
	}
	k := artistKinds[store.ArtistBackground]
	got := k.candidates(files("fanart2.jpg", "background.jpg", "backdrop.jpg", "fanart.png", "fanart1.jpg", "fanart.jpg", "cover.jpg"))
	names := make([]string, 0, len(got))
	for _, f := range got {
		names = append(names, filepath.Base(f.rel))
	}
	want := []string{"fanart.jpg", "fanart.png", "backdrop.jpg", "background.jpg", "fanart1.jpg", "fanart2.jpg"}
	if !slices.Equal(names, want) {
		t.Errorf("candidates = %v, want %v", names, want)
	}
	k = artistKinds[store.ArtistLogo]
	logo := k.candidates(files("logo.jpg", "clearlogo.jpg", "clearlogo.png"))
	if filepath.Base(logo[0].rel) != "clearlogo.png" {
		t.Errorf("best logo = %s, want the PNG", logo[0].rel)
	}
}

func TestDiscArtNames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		disc       int
		ok         bool
		betterThan string // Another name for the disc it beats, or "".
	}{
		{"cdart.png", store.AllDiscs, true, "discart.png"},
		{"discart.png", store.AllDiscs, true, "disc.png"},
		{"disc.jpg", store.AllDiscs, true, ""},
		{"cd2.png", 2, true, "cd2.jpg"},
		{"discart-3.png", 3, true, "cd3.png"},
		{"Disc 1.PNG", 1, true, ""},
		{"cdart0.png", 0, false, ""},
		{"track2.png", 0, false, ""},
		{"disc.gif", 0, false, ""},
	}
	for _, tt := range tests {
		got, ok := discOf(tt.name)
		if ok != tt.ok || (ok && got.disc != tt.disc) {
			t.Errorf("discOf(%s) = %+v %v, want disc %d %v", tt.name, got, ok, tt.disc, tt.ok)
		}
		if tt.betterThan != "" {
			if other, _ := discOf(tt.betterThan); got.rank >= other.rank {
				t.Errorf("%s ranks %d, not before %s's %d", tt.name, got.rank, tt.betterThan, other.rank)
			}
		}
	}
}

// layoutOf builds a layout from tracks given as "album:path", with owners by album.
func layoutOf(tracks []string, owners map[int64][]int64) *store.Layout {
	l := &store.Layout{Owners: map[int64][]store.LayoutArtist{}}
	for _, t := range tracks {
		album, rel, _ := strings.Cut(t, ":")
		id := int64(album[0] - '0')
		l.Tracks = append(l.Tracks, store.LayoutTrack{AlbumID: id, AlbumULID: album, Root: "/m", RelPath: rel})
	}
	for album, artists := range owners {
		for _, a := range artists {
			l.Owners[album] = append(l.Owners[album], store.LayoutArtist{ID: a})
		}
	}
	return l
}

func TestArtistFolders(t *testing.T) {
	t.Parallel()
	const a, b = 1, 2
	tests := []struct {
		name   string
		tracks []string
		owners map[int64][]int64
		want   map[int64][]string
	}{
		{
			name:   "the folder above an artist's albums",
			tracks: []string{bandOne1, "1:Band/One/02.flac", "2:Band/Two/01.flac"},
			owners: map[int64][]int64{1: {a}, 2: {a}},
			want:   map[int64][]string{a: {bandFolder}},
		},
		{
			name:   "disc folders fold into the album's",
			tracks: []string{"1:Band/One/CD1/01.flac", "1:Band/One/CD2/01.flac"},
			owners: map[int64][]int64{1: {a}},
			want:   map[int64][]string{a: {bandFolder}},
		},
		{
			name:   "two artists' albums in one folder are nobody's",
			tracks: []string{"1:Mixed/One/01.flac", "2:Mixed/Two/01.flac"},
			owners: map[int64][]int64{1: {a}, 2: {b}},
			want:   map[int64][]string{},
		},
		{
			name:   "a compilation in the folder makes it nobody's",
			tracks: []string{bandOne1, "2:Band/Hits/01.flac"},
			owners: map[int64][]int64{1: {a}},
			want:   map[int64][]string{},
		},
		{
			name:   "a shared album in the folder makes it nobody's",
			tracks: []string{bandOne1, "2:Band/Split/01.flac"},
			owners: map[int64][]int64{1: {a}, 2: {a, b}},
			want:   map[int64][]string{},
		},
		{
			name:   "another artist's songs deeper down make it nobody's",
			tracks: []string{bandOne1, "2:Band/One/Bonus/Guest/01.flac"},
			owners: map[int64][]int64{1: {a}, 2: {b}},
			want:   map[int64][]string{b: {"Band/One/Bonus"}}, // That folder holds the guest's album alone.
		},
		{
			name:   "the root, for a library of one artist",
			tracks: []string{"1:One/01.flac", "2:Two/01.flac"},
			owners: map[int64][]int64{1: {a}, 2: {a}},
			want:   map[int64][]string{a: {"."}},
		},
		{
			name:   "an album in the root has no folder above it",
			tracks: []string{"1:01.flac"},
			owners: map[int64][]int64{1: {a}},
			want:   map[int64][]string{},
		},
		{
			name:   "an album split over folders claims none",
			tracks: []string{bandOne1, "1:Elsewhere/One/02.flac"},
			owners: map[int64][]int64{1: {a}},
			want:   map[int64][]string{},
		},
		{
			name:   "two folders of one artist, by path",
			tracks: []string{"1:Zed/One/01.flac", "2:Band/Two/01.flac"},
			owners: map[int64][]int64{1: {a}, 2: {a}},
			want:   map[int64][]string{a: {"Band", "Zed"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := layoutOf(tt.tracks, tt.owners)
			got := map[int64][]string{}
			for artist, fs := range artistFolders(l, albumFolders(l.Tracks), []string{"/m"}) {
				for _, f := range fs {
					got[artist] = append(got[artist], f.dir)
				}
			}
			if !maps.EqualFunc(got, tt.want, slices.Equal) {
				t.Errorf("artist folders = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseNfo(t *testing.T) {
	t.Parallel()
	const mbid = "5b11f4ce-a62d-471e-81fc-a69a8278c7da"
	tests := []struct {
		name, nfo, mbid string
		summary, body   string
		err             error
	}{
		{
			name:    "Kodi's own",
			nfo:     `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?><artist><name>Band</name><musicBrainzArtistID>` + mbid + `</musicBrainzArtistID><biography>Band formed in 1987.&#10;&#10;They split in 1994.</biography></artist>`,
			mbid:    mbid,
			summary: formed, body: formed + "\n\nThey split in 1994.",
		},
		{
			name:    "names in any case, CRLF and entities",
			nfo:     "<Artist><Biography>Rock &amp; roll.\r\n  Then more.  </Biography></Artist>",
			summary: "Rock & roll.", body: "Rock & roll.\n\nThen more.",
		},
		{
			name:    "a biography deeper down is not the artist's",
			nfo:     "<artist><album><biography>An album's.</biography></album><biography>The artist's.</biography></artist>",
			summary: "The artist's.", body: "The artist's.",
		},
		{
			name:    "an unknown MBID on our side takes the file as it is",
			nfo:     "<artist><musicbrainzartistid>" + mbid + "</musicbrainzartistid><biography>Bio.</biography></artist>",
			summary: "Bio.", body: "Bio.",
		},
		{name: "another artist's", nfo: "<artist><musicbrainzartistid>" + mbid + "</musicbrainzartistid><biography>Bio.</biography></artist>", mbid: "00000000-0000-4000-8000-000000000000", err: errNfoOtherMBID},
		{name: "no biography", nfo: "<artist><name>Band</name></artist>", err: errNfoNoBio},
		{name: "an empty biography", nfo: "<artist><biography>  \n </biography></artist>", err: errNfoNoBio},
		{name: "an album.nfo", nfo: "<album><biography>Bio.</biography></album>", err: errNfoUnreadable},
		{name: "not XML", nfo: "Band formed in 1987.", err: errNfoNoBio},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseNfo([]byte(tt.nfo), tt.mbid)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("parseNfo = %+v, %v; want %v", got, err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Summary != tt.summary || got.Body != tt.body {
				t.Errorf("bio = %q / %q, want %q / %q", got.Summary, got.Body, tt.summary, tt.body)
			}
		})
	}
}

// picture returns a w x h PNG of one color; alpha clears all but its middle
// third, as a logo's ground is.
func picture(t *testing.T, w, h int, c color.NRGBA, alpha bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			px := c
			if alpha && (y < h/3 || y >= 2*h/3) {
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

func shade(n uint8) color.NRGBA { return color.NRGBA{R: 0x30 + n*0x10, G: 0x60, B: 0x90, A: 0xFF} }

const (
	bandName   = "Band"
	bandFolder = "Band"
	bandAlbum  = "Band/One"
	bandTitle  = "One"
	formed     = "Band formed in 1987."
	bandOne1   = "1:Band/One/01.flac"
)

// extrasLib is a library with art, holding one artist's album in its own
// folders, scanned once with every extra in place.
type extrasLib struct {
	*artLib

	heard *heard
}

func newExtrasLib(t *testing.T) *extrasLib {
	t.Helper()
	l := &extrasLib{artLib: newArtLib(t), heard: &heard{}}
	l.opts.Notify = l.heard
	l.scanner = NewScanner(l.st, l.opts)
	l.songs(
		tagged{bandAlbum + "/01.flac", bandName, bandTitle, "First", 2001, 1},
		tagged{bandAlbum + "/02.flac", bandName, bandTitle, "Second", 2001, 2},
	)
	root := l.roots[0]
	l.write(t, root, bandFolder+"/artist.jpg", picture(t, 600, 600, shade(1), false))
	l.write(t, root, bandFolder+"/fanart.jpg", picture(t, 1920, 1080, shade(2), false))
	l.write(t, root, bandFolder+"/clearlogo.png", picture(t, 800, 310, shade(3), true))
	l.write(t, root, bandFolder+"/banner.jpg", picture(t, 1000, 185, shade(4), false))
	l.write(t, root, bandFolder+"/artist.nfo", []byte("<artist><biography>Band formed in 1987.\nThey split in 1994.</biography></artist>"))
	l.write(t, root, bandAlbum+"/back.jpg", picture(t, 500, 500, shade(5), false))
	l.write(t, root, bandAlbum+"/cdart.png", picture(t, 500, 500, shade(6), true))
	l.write(t, root, bandAlbum+"/cd2.png", picture(t, 500, 500, shade(7), true))
	return l
}

// band returns the artist page's record of Band.
func (l *extrasLib) band() store.ArtistDetail {
	l.t.Helper()
	artists, err := l.st.Artists(l.t.Context(), 0)
	if err != nil {
		l.t.Fatal(err)
	}
	for i := range artists {
		if artists[i].Name == bandName {
			d, aerr := l.st.Artist(l.t.Context(), artists[i].ID)
			if aerr != nil {
				l.t.Fatal(aerr)
			}
			return d
		}
	}
	l.t.Fatal("no Band")
	return store.ArtistDetail{}
}

// album returns the album page's record of One.
func (l *extrasLib) album() store.AlbumDetail {
	l.t.Helper()
	d := l.band()
	a, err := l.st.Album(l.t.Context(), d.Albums[0].ID)
	if err != nil {
		l.t.Fatal(err)
	}
	return a
}

func (l *extrasLib) wantRendered(what string, img *store.ExtraImage, size int) {
	l.t.Helper()
	if img == nil {
		l.t.Fatalf("%s: none", what)
	}
	p, err := l.cache.Path(img.Hash, size)
	if err != nil {
		l.t.Fatalf("%s: no rendition: %v", what, err)
	}
	if artwork.IsPNG(p) != img.Alpha {
		l.t.Errorf("%s: rendition %s, alpha %v", what, filepath.Base(p), img.Alpha)
	}
}

func TestScanReadsArtistAndAlbumFolders(t *testing.T) {
	t.Parallel()
	l := newExtrasLib(t)
	if stats := l.scan(Quick); stats.Extras != 8 {
		t.Errorf("Extras = %d, want 8", stats.Extras)
	}
	d := l.band()
	if got, want := summary(&d), bandAsWritten; got != want {
		t.Errorf("Band = %s, want %s", got, want)
	}
	l.wantArtistRendered(&d)
	if d.Images.Thumb.Source != folderIs || d.Images.Thumb.Credit != nil {
		t.Errorf("thumb from %q with credit %+v, want a folder's with none", d.Images.Thumb.Source, d.Images.Thumb.Credit)
	}
	if d.Bio.Source != "nfo" || d.Bio.Body != formed+"\n\nThey split in 1994." {
		t.Errorf("bio = %+v", d.Bio)
	}
	a := l.album()
	l.wantAlbumExtras(&a)
	notes := l.heard.take()
	if len(notes.artists) != 1 || !slices.Equal(notes.artists[0], []string{d.ID}) {
		t.Errorf("artists heard = %v, want [%s]", notes.artists, d.ID)
	}
	if !slices.Contains(notes.changedAlbums(), a.ID) {
		t.Errorf("albums heard = %v, want %s among them", notes.changedAlbums(), a.ID)
	}
}

// bandAsWritten is newExtrasLib's Band, as summary has it.
const bandAsWritten = `thumb:600 background:1920 logo:800 banner:1000 bio:"Band formed in 1987."`

// summary says what an artist has: each image by the width of its source (0
// for none), and its bio's summary.
func summary(d *store.ArtistDetail) string {
	width := func(img *store.ExtraImage) int {
		if img == nil {
			return 0
		}
		return img.Width
	}
	bio := ""
	if d.Bio != nil {
		bio = d.Bio.Summary
	}
	i := &d.Images
	return fmt.Sprintf("thumb:%d background:%d logo:%d banner:%d bio:%q", width(i.Thumb), width(i.Background), width(i.Logo), width(i.Banner), bio)
}

// wantArtistRendered checks every image of d has its renditions, in its format:
// the logo's PNG, the rest JPEG.
func (l *extrasLib) wantArtistRendered(d *store.ArtistDetail) {
	l.t.Helper()
	l.wantRendered("thumb", d.Images.Thumb, artwork.Size512)
	l.wantRendered("background", d.Images.Background, artwork.Size1920)
	l.wantRendered("logo", d.Images.Logo, artwork.Size800)
	l.wantRendered("banner", d.Images.Banner, artwork.Size1920)
	if !d.Images.Logo.Alpha || d.Images.Thumb.Alpha || d.Images.Background.Alpha {
		l.t.Errorf("alpha: logo %v, thumb %v, background %v; want the logo's alone", d.Images.Logo.Alpha, d.Images.Thumb.Alpha, d.Images.Background.Alpha)
	}
}

// wantAlbumExtras checks One has newExtrasLib's back cover and disc art.
func (l *extrasLib) wantAlbumExtras(a *store.AlbumDetail) {
	l.t.Helper()
	ex := &a.Extras
	if ex.Back == nil || len(ex.Discs) != 2 || ex.Discs[0].Disc != store.AllDiscs || ex.Discs[1].Disc != 2 {
		l.t.Fatalf("extras = %+v, want a back cover and discs 0 and 2", ex)
	}
	l.wantRendered("back", ex.Back, artwork.Size512)
	l.wantRendered("disc", &ex.Discs[0].ExtraImage, artwork.Size800)
	if !ex.Discs[0].Alpha {
		l.t.Error("the disc art was flattened")
	}
}

// An unchanged folder costs a stat: nothing is read or written, and nobody
// hears of it. A full scan reads it again, and changes nothing either.
func TestUnchangedFoldersAreNotReadAgain(t *testing.T) {
	t.Parallel()
	l := newExtrasLib(t)
	l.scan(Quick)
	l.heard.take()
	if stats := l.scan(Quick); stats.Extras != 0 {
		t.Errorf("a quick scan of unchanged folders wrote %d extras", stats.Extras)
	}
	if stats := l.scan(Full); stats.Extras != 0 {
		t.Errorf("a full scan of unchanged folders wrote %d extras", stats.Extras)
	}
	if notes := l.heard.take(); len(notes.artists) != 0 {
		t.Errorf("artists heard = %v, want none", notes.artists)
	}
}

// A full scan puts every file again, so a cache that lost its renditions has
// them back.
func TestFullScanRendersALostImageAgain(t *testing.T) {
	t.Parallel()
	l := newExtrasLib(t)
	l.scan(Quick)
	thumb := l.band().Images.Thumb
	if err := l.cache.Delete(thumb.Hash); err != nil {
		t.Fatal(err)
	}
	l.scan(Full)
	l.wantRendered("thumb", l.band().Images.Thumb, artwork.Size512)
}

func TestFolderChangesReachTheArtist(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(l *extrasLib)
		want   string // As summary has it.
	}{
		{
			name:   "a file removed takes its image away",
			change: func(l *extrasLib) { remove(l.t, l.roots[0], bandFolder+"/fanart.jpg") },
			want:   `thumb:600 background:0 logo:800 banner:1000 bio:"Band formed in 1987."`,
		},
		{
			name: "a better name wins",
			change: func(l *extrasLib) {
				l.write(l.t, l.roots[0], bandFolder+"/fanart.png", picture(l.t, 1280, 720, shade(9), false))
				remove(l.t, l.roots[0], bandFolder+"/fanart.jpg")
				l.write(l.t, l.roots[0], bandFolder+"/backdrop.jpg", picture(l.t, 1024, 576, shade(8), false))
			},
			want: `thumb:600 background:1280 logo:800 banner:1000 bio:"Band formed in 1987."`,
		},
		{
			name: "a file changed is read again",
			change: func(l *extrasLib) {
				l.write(l.t, l.roots[0], bandFolder+"/artist.nfo", []byte("<artist><biography>Band reformed.</biography></artist>"))
			},
			want: `thumb:600 background:1920 logo:800 banner:1000 bio:"Band reformed."`,
		},
		{
			name: "a broken image falls back to the next name",
			change: func(l *extrasLib) {
				l.write(l.t, l.roots[0], bandFolder+"/artist.jpg", garbage())
				l.write(l.t, l.roots[0], bandFolder+"/thumb.jpg", picture(l.t, 300, 300, shade(10), false))
			},
			want: `thumb:300 background:1920 logo:800 banner:1000 bio:"Band formed in 1987."`,
		},
		{
			name:   "a broken image with nothing after it takes the image away",
			change: func(l *extrasLib) { l.write(l.t, l.roots[0], bandFolder+"/banner.jpg", garbage()) },
			want:   `thumb:600 background:1920 logo:800 banner:0 bio:"Band formed in 1987."`,
		},
		{
			name: "another artist's album in the folder takes everything away",
			change: func(l *extrasLib) {
				l.songs(tagged{bandFolder + "/Guest/01.flac", "Guest", "Guest Album", "Guesting", 2002, 1})
			},
			want: `thumb:0 background:0 logo:0 banner:0 bio:""`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := newExtrasLib(t)
			l.scan(Quick)
			l.heard.take()
			tt.change(l)
			l.scan(Quick)
			d := l.band()
			if got := summary(&d); got != tt.want {
				t.Errorf("Band = %s, want %s", got, tt.want)
			}
			if notes := l.heard.take(); len(notes.artists) != 1 {
				t.Errorf("artists heard = %v, want Band once", notes.artists)
			}
		})
	}
}

// A file that served nothing is read again only once it changes.
func TestABrokenFileIsReadOnce(t *testing.T) {
	t.Parallel()
	l := newExtrasLib(t)
	l.write(t, l.roots[0], bandFolder+"/banner.jpg", garbage())
	l.scan(Quick)
	if l.band().Images.Banner != nil {
		t.Fatal("a broken banner was stored")
	}
	if !l.scanner.badExtras.has(fileRef(l.roots[0], seen(t, l.roots[0], bandFolder+"/banner.jpg"))) {
		t.Error("the broken banner was not remembered")
	}
	if stats := l.scan(Quick); stats.Extras != 0 {
		t.Errorf("Extras = %d after a scan of the same broken file", stats.Extras)
	}
}

// seen is the walk's record of root/rel.
func seen(t *testing.T, root, rel string) *seenFile {
	t.Helper()
	st, err := os.Stat(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return &seenFile{rel: rel, size: st.Size(), mtime: st.ModTime()}
}

// The prune at the end of a scan keeps an image that only an artist uses,
// and its files.
func TestPruneKeepsAnArtistsOwnImage(t *testing.T) {
	t.Parallel()
	l := newExtrasLib(t)
	l.scan(Quick)
	l.scan(Full)
	d := l.band()
	l.wantRendered("thumb after a full scan", d.Images.Thumb, artwork.Size512)
	l.wantRendered("background after a full scan", d.Images.Background, artwork.Size640)
}

// A pinned image is the administrator's: no scan replaces it or takes it away.
func TestAPinOutranksTheFolder(t *testing.T) {
	t.Parallel()
	l := newExtrasLib(t)
	l.scan(Quick)
	pin := l.band().Images.Thumb.Hash
	db, err := sql.Open("sqlite3", "file:"+l.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	// What a later phase's pin writes: the image an administrator chose.
	if _, err = db.ExecContext(t.Context(), "UPDATE artist_images SET source = 'pinned', file_root = NULL, file_path = NULL, file_size = NULL, file_mtime = NULL WHERE kind = 'thumb'"); err != nil {
		t.Fatal(err)
	}
	l.write(t, l.roots[0], bandFolder+"/artist.jpg", picture(t, 700, 700, shade(11), false))
	l.scan(Quick)
	if got := l.band().Images.Thumb; got == nil || got.Hash != pin || got.Source != "pinned" {
		t.Errorf("thumb = %+v, want the pin kept", got)
	}
	remove(t, l.roots[0], bandFolder+"/artist.jpg")
	l.scan(Full)
	if got := l.band().Images.Thumb; got == nil || got.Hash != pin {
		t.Errorf("thumb = %+v after its file went, want the pin kept", got)
	}
}

// The watcher's scan of an artist folder, which holds no song, reads what
// changed there.
func TestTargetedScanReadsAnArtistFolder(t *testing.T) {
	t.Parallel()
	l := newExtrasLib(t)
	remove(t, l.roots[0], bandFolder+"/banner.jpg")
	l.scan(Quick)
	l.heard.take()
	l.write(t, l.roots[0], bandFolder+"/banner.jpg", picture(t, 1000, 185, shade(12), false))
	if _, err := l.scanner.ScanDirs(t.Context(), l.roots[0], []string{bandFolder}); err != nil {
		t.Fatal(err)
	}
	if l.band().Images.Banner == nil {
		t.Error("the targeted scan didn't read the new banner")
	}
	remove(t, l.roots[0], bandFolder+"/banner.jpg")
	if _, err := l.scanner.ScanDirs(t.Context(), l.roots[0], []string{bandFolder}); err != nil {
		t.Fatal(err)
	}
	if l.band().Images.Banner != nil {
		t.Error("the targeted scan kept a banner whose file is gone")
	}
	// A scan of the album's folder alone says nothing of the artist's.
	l.write(t, l.roots[0], bandFolder+"/banner.jpg", picture(t, 1000, 185, shade(13), false))
	if _, err := l.scanner.ScanDirs(t.Context(), l.roots[0], []string{bandAlbum}); err != nil {
		t.Fatal(err)
	}
	if l.band().Images.Banner != nil {
		t.Error("a scan of the album's folder read the artist's")
	}
	if notes := l.heard.take(); len(notes.artists) != 2 {
		t.Errorf("artists heard = %v, want Band twice", notes.artists)
	}
}

// Without an artwork cache a scan stores no image, and the bio still.
func TestNoCacheReadsBiosAlone(t *testing.T) {
	t.Parallel()
	l := newExtrasLib(t)
	l.opts.Artwork = nil
	l.scanner = NewScanner(l.st, l.opts)
	l.scan(Quick)
	d := l.band()
	if d.Images != (store.ArtistImages{}) {
		t.Errorf("images = %+v without a cache", d.Images)
	}
	if d.Bio == nil {
		t.Error("no bio without a cache")
	}
}
