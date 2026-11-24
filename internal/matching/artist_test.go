package matching_test

import (
	"errors"
	"testing"

	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/matching/musicbrainztest"
)

func TestArtistLinks(t *testing.T) {
	t.Parallel()
	mb := musicbrainztest.NewInMemory()
	id := musicbrainztest.ID("low tide choir")
	mb.AddArtist(
		musicbrainztest.Artist{ID: id, Name: "Low Tide Choir", Wikidata: "Q2831", Wikipedia: map[string]string{"de": "Low_Tide_Choir"}},
		musicbrainztest.Artist{ID: musicbrainztest.ID("odd"), Raw: `{"relations":[{"type":"wikidata","url":{"resource":"https://www.wikidata.org/wiki/P18"}}]}`},
	)
	c := matching.NewMusicBrainz(mb.Options())
	links, err := c.ArtistLinks(matching.Background(t.Context()), id)
	if err != nil {
		t.Fatal(err)
	}
	if links.Wikidata != "Q2831" {
		t.Errorf("links = %+v", links)
	}
	if calls := mb.Calls(); len(calls) != 1 || calls[0].Path != "artist/"+id {
		t.Errorf("calls = %+v", calls)
	}
	if links, err = c.ArtistLinks(t.Context(), musicbrainztest.ID("odd")); err != nil || links.Wikidata != "" {
		t.Errorf("a property for an item: %+v, %v", links, err)
	}
	for _, bad := range []string{"not-an-mbid", musicbrainztest.ID("nobody")} {
		if _, err = c.ArtistLinks(t.Context(), bad); !errors.Is(err, matching.ErrNotFound) {
			t.Errorf("%s: %v, want not found", bad, err)
		}
	}
}
