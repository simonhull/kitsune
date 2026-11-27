package lastfm_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/simonhull/kitsune/internal/lastfm"
	"github.com/simonhull/kitsune/internal/lastfm/lastfmtest"
)

const (
	tideMBID = "6f1a58bf-9b1b-49cf-a44a-6cefad7ae04f"
	tidePage = "https://www.last.fm/music/Low+Tide+Choir"
)

func TestArtistInfoStripsTheBioToParagraphs(t *testing.T) {
	t.Parallel()
	fake := lastfmtest.New(t)
	fake.AddArtist(lastfmtest.Artist{
		MBID:    tideMBID,
		Summary: lastfmtest.Bio(tidePage, false, `Low Tide Choir is a <a href="https://www.last.fm/tag/folk">folk</a> band.`),
		Content: lastfmtest.Bio(tidePage, true,
			`Low Tide Choir is a <a href="https://www.last.fm/tag/folk">folk</a> band &amp; choir.`,
			``,
			`They formed in 2009.<br/>In a harbour town.`),
	})
	c := fake.Client()

	bio, err := c.ArtistInfo(t.Context(), tideMBID, "en")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"Low Tide Choir is a folk band & choir.", "They formed in 2009.", "In a harbour town."}
	if !slices.Equal(bio.Paragraphs, want) {
		t.Errorf("paragraphs = %q, want %q", bio.Paragraphs, want)
	}
	if bio.Summary != "Low Tide Choir is a folk band." || bio.ReadMore != tidePage || bio.MBID != tideMBID {
		t.Errorf("bio = %+v", bio)
	}
	calls := fake.CallsTo("artist.getInfo")
	if len(calls) != 1 || calls[0].Params.Has("api_sig") || calls[0].Params.Get("mbid") != tideMBID {
		t.Errorf("calls = %+v, want one unsigned call by MBID", calls)
	}
}

func TestArtistInfoSaysWhatFailed(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		fail []int
		want error
	}{
		"unknown":   {want: lastfm.ErrNotFound},
		"bad key":   {fail: []int{10}, want: lastfm.ErrKeyRefused},
		"suspended": {fail: []int{26}, want: lastfm.ErrKeyRefused},
		"down":      {fail: []int{503}, want: lastfm.ErrTemporary},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := lastfmtest.New(t)
			fake.Fail(tc.fail...)
			_, err := fake.Client().ArtistInfo(t.Context(), tideMBID, "")
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestParagraphsKeepsOnlyLastfmsReadMore(t *testing.T) {
	t.Parallel()
	paras, more := lastfm.Paragraphs(`Text. <a href="https://example.com/x">Read more on Last.fm</a>`)
	if more != "" || !slices.Equal(paras, []string{"Text."}) {
		t.Errorf("paragraphs, more = %q, %q", paras, more)
	}
	if paras, more = lastfm.Paragraphs(""); paras != nil || more != "" {
		t.Errorf("empty = %q, %q", paras, more)
	}
}
