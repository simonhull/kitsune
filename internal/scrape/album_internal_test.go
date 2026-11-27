package scrape

import (
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store"
)

func TestWantsLastfmOnlyForABioNoOneElseGave(t *testing.T) {
	t.Parallel()
	wikiBio := &found{source: store.ScrapeWikimedia, asked: true, bio: &store.Bio{Source: store.BioFromWikipedia}}
	wikiNone := &found{source: store.ScrapeWikimedia, asked: true}
	for name, tc := range map[string]struct {
		held   *store.HeldBio
		before []*found
		want   bool
	}{
		"nothing anywhere":               {want: true},
		"artist.nfo's":                   {held: &store.HeldBio{Source: "nfo"}},
		"Wikipedia gave one now":         {before: []*found{wikiBio}},
		"Wikipedia's kept, not asked":    {held: &store.HeldBio{Source: store.BioFromWikipedia}},
		"Wikipedia's gone now":           {held: &store.HeldBio{Source: store.BioFromWikipedia}, before: []*found{wikiNone}, want: true},
		"Last.fm's own, asked again":     {held: &store.HeldBio{Source: store.BioFromLastfm}, want: true},
		"Wikipedia answered with none":   {before: []*found{wikiNone}, want: true},
		"Wikipedia failed, nothing held": {before: []*found{{source: store.ScrapeWikimedia, status: store.ScrapeFailed}}, want: true},
	} {
		held := &store.HeldArt{Bio: tc.held}
		if got := wantsLastfm(held, tc.before); got != tc.want {
			t.Errorf("%s: wants Last.fm = %v, want %v", name, got, tc.want)
		}
	}
}

func TestDiscCandidatesByDisc(t *testing.T) {
	t.Parallel()
	cands := []Candidate{{ID: "1"}, {ID: "2"}, {ID: "3"}}
	ids := func(cs []Candidate) []string {
		out := make([]string, 0, len(cs))
		for _, c := range cs {
			out = append(out, c.ID)
		}
		return out
	}
	if got := ids(DiscCandidates(cands, []int{1, 2, 0}, store.AllDiscs)); !slices.Equal(got, []string{"1", "3"}) {
		t.Errorf("every disc = %v, want the first disc's and the unsaid", got)
	}
	if got := ids(DiscCandidates(cands, []int{2, 2, 3}, store.AllDiscs)); !slices.Equal(got, []string{"1", "2", "3"}) {
		t.Errorf("every disc, none of the first = %v, want any", got)
	}
	if got := ids(DiscCandidates(cands, []int{1, 2, 2}, 2)); !slices.Equal(got, []string{"2", "3"}) {
		t.Errorf("disc 2 = %v", got)
	}
	if got := DiscCandidates(cands, []int{1, 1, 1}, 3); len(got) != 0 {
		t.Errorf("disc 3 = %v, want none", got)
	}
}

func TestAlbumSourcesAskOnlyForWhatIsMissing(t *testing.T) {
	t.Parallel()
	release, group := "6f1a58bf-9b1b-49cf-a44a-6cefad7ae04f", "7f1a58bf-9b1b-49cf-a44a-6cefad7ae04f"
	backKey := store.AlbumImageKey{AlbumID: 1, Kind: store.AlbumBack}
	discKey := store.AlbumImageKey{AlbumID: 1, Kind: store.AlbumDisc, Disc: 2}
	folder := store.HeldAlbumImage{Source: fromFolder}
	for name, tc := range map[string]struct {
		a    store.ScrapeAlbum
		want []store.ScrapeSource
	}{
		"no MBID":                       {a: store.ScrapeAlbum{}},
		"no cover":                      {a: store.ScrapeAlbum{MBID: release}, want: []store.ScrapeSource{store.ScrapeMusicBrainz, store.ScrapeArchive}},
		"own cover, folder's back":      {a: store.ScrapeAlbum{MBID: release, GroupMBID: group, OwnCover: true, Held: map[store.AlbumImageKey]store.HeldAlbumImage{backKey: folder}}, want: []store.ScrapeSource{store.ScrapeFanart}},
		"everything from the folders":   {a: store.ScrapeAlbum{MBID: release, OwnCover: true, Held: map[store.AlbumImageKey]store.HeldAlbumImage{backKey: folder, discKey: folder}}},
		"a group, no release":           {a: store.ScrapeAlbum{GroupMBID: group}, want: []store.ScrapeSource{store.ScrapeFanart}},
		"own cover, no back, no group":  {a: store.ScrapeAlbum{MBID: release, OwnCover: true}, want: []store.ScrapeSource{store.ScrapeMusicBrainz, store.ScrapeArchive}},
		"a group to look up, disc held": {a: store.ScrapeAlbum{MBID: release, OwnCover: true, Held: map[store.AlbumImageKey]store.HeldAlbumImage{discKey: folder}}, want: []store.ScrapeSource{store.ScrapeArchive}},
	} {
		tc.a.ID = 1
		var got []store.ScrapeSource
		for _, src := range albumSources() {
			if albumSourceDue(src, &tc.a, time.Now()) {
				got = append(got, src)
			}
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: due = %v, want %v", name, got, tc.want)
		}
	}
}
