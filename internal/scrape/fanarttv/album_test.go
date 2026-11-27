package fanarttv_test

import (
	"bytes"
	"errors"
	"image/png"
	"testing"

	"github.com/simonhull/kitsune/internal/remote"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv/fanarttvtest"
)

const group = "5b1c0d2e-0000-4000-8000-0000000000a1"

func TestAlbumReadsDiscArt(t *testing.T) {
	t.Parallel()
	ft := fanarttvtest.New(t)
	ft.AddAlbum(fanarttvtest.Album{MBID: group, Discs: []fanarttvtest.Disc{
		{ID: "71", Likes: 2, Disc: 1},
		{ID: "72", Disc: 2},
	}})
	c := fanarttv.New(ft.Options())

	discs, err := c.Album(t.Context(), keys, group)
	if err != nil {
		t.Fatal(err)
	}
	if len(discs) != 2 || discs[0].Disc != 1 || discs[0].Likes != 2 || discs[1].Disc != 2 || discs[0].Width != 1000 {
		t.Fatalf("discs = %+v", discs)
	}
	body, err := c.Download(t.Context(), discs[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Errorf("disc art's corner alpha = %d, want transparent", a)
	}
	checkCalls(t, ft.Calls())
}

func TestAlbumSaysWhatFailed(t *testing.T) {
	t.Parallel()
	ft := fanarttvtest.New(t)
	c := fanarttv.New(ft.Options())
	if _, err := c.Album(t.Context(), keys, group); !errors.Is(err, remote.ErrNotFound) {
		t.Errorf("unknown: %v, want not found", err)
	}
	if _, err := c.Album(t.Context(), fanarttv.Keys{}, group); !errors.Is(err, fanarttv.ErrNoKey) {
		t.Errorf("no key: %v, want ErrNoKey", err)
	}
	ft.AddAlbum(fanarttvtest.Album{MBID: group, Discs: []fanarttvtest.Disc{{ID: "71"}}})
	ft.BareNames(true)
	if _, err := c.Album(t.Context(), keys, group); !errors.Is(err, fanarttv.ErrNotALink) {
		t.Errorf("bare names: %v, want ErrNotALink", err)
	}
}
