package matching_test

import (
	"bytes"
	"errors"
	"image"
	_ "image/png" // The stand-in's covers.
	"testing"

	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/matching/musicbrainztest"
	"github.com/simonhull/kitsune/internal/remote"
)

func TestReleaseGroup(t *testing.T) {
	t.Parallel()
	id, group := musicbrainztest.ID("tidelines"), musicbrainztest.ID("tidelines group")
	mb := musicbrainztest.NewInMemory(
		matching.Release{ID: id, GroupID: group, Title: "Tidelines"},
		matching.Release{ID: musicbrainztest.ID("loose"), Title: "Loose"},
	)
	c := matching.NewMusicBrainz(mb.Options())

	got, err := c.ReleaseGroup(matching.Background(t.Context()), id)
	if err != nil || got != group {
		t.Fatalf("group = %q, %v; want %q", got, err, group)
	}
	if calls := mb.Calls(); len(calls) != 1 || calls[0].Path != "release/"+id {
		t.Errorf("calls = %+v", calls)
	}
	if got, err = c.ReleaseGroup(t.Context(), musicbrainztest.ID("loose")); err != nil || got != "" {
		t.Errorf("no group: %q, %v", got, err)
	}
	if _, err = c.ReleaseGroup(t.Context(), musicbrainztest.ID("nobody")); !errors.Is(err, matching.ErrNotFound) {
		t.Errorf("unknown: %v, want not found", err)
	}
}

func TestCoverArtFindsFrontAndBack(t *testing.T) {
	t.Parallel()
	both, front, none := musicbrainztest.ID("both"), musicbrainztest.ID("front"), musicbrainztest.ID("none")
	mb := musicbrainztest.NewInMemory(
		matching.Release{ID: both, Cover: true},
		matching.Release{ID: front, Cover: true},
		matching.Release{ID: none},
	)
	mb.AddBack(both)
	c := matching.NewMusicBrainz(mb.Options())
	ctx := matching.Background(t.Context())

	art, err := c.CoverArt(ctx, both)
	if err != nil || art.Front == nil || art.Back == nil || art.Front.ID == art.Back.ID {
		t.Fatalf("both = %+v, %v", art, err)
	}
	frontBytes, err := c.DownloadArt(ctx, art.Front.URL)
	if err != nil {
		t.Fatal(err)
	}
	backBytes, err := c.DownloadArt(ctx, art.Back.URL)
	if err != nil || bytes.Equal(frontBytes, backBytes) {
		t.Fatalf("back = %d bytes, %v; want another image", len(backBytes), err)
	}
	if _, _, err = image.Decode(bytes.NewReader(frontBytes)); err != nil {
		t.Errorf("front doesn't decode: %v", err)
	}
	if art, err = c.CoverArt(ctx, front); err != nil || art.Front == nil || art.Back != nil {
		t.Errorf("front only = %+v, %v", art, err)
	}
	if _, err = c.CoverArt(ctx, none); !errors.Is(err, remote.ErrNotFound) {
		t.Errorf("none: %v, want not found", err)
	}
}

func TestDownloadArtFetchesOnlyTheArchive(t *testing.T) {
	t.Parallel()
	mb := musicbrainztest.NewInMemory()
	c := matching.NewMusicBrainz(mb.Options())
	for _, u := range []string{"https://example.com/front.jpg", "front.jpg", "ftp://coverartarchive.org/x", "http://musicbrainz.test/coverart/../x"} {
		if _, err := c.DownloadArt(t.Context(), u); !errors.Is(err, matching.ErrBadArt) {
			t.Errorf("%q: %v, want bad art", u, err)
		}
	}
}
