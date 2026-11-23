package scrape_test

import (
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/scrape"
	"github.com/simonhull/kitsune/internal/store"
)

func TestPickOrdersByTierLanguageLikesSizeAndID(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		cands []scrape.Candidate
		want  string
	}{
		{"none", nil, ""},
		{"the 4K tier first", []scrape.Candidate{{ID: "1", Tier: 1, Likes: 9}, {ID: "2", Tier: 0}}, "2"},
		{"the language wanted", []scrape.Candidate{{ID: "1", Lang: "en", Likes: 9}, {ID: "2", Lang: "de"}}, "2"},
		{"then English", []scrape.Candidate{{ID: "1", Lang: "", Likes: 9}, {ID: "2", Lang: "en"}}, "2"},
		{"then none", []scrape.Candidate{{ID: "1", Lang: "fr", Likes: 9}, {ID: "2", Lang: ""}}, "2"},
		{"then likes", []scrape.Candidate{{ID: "1", Likes: 1}, {ID: "2", Likes: 4}}, "2"},
		{"then size", []scrape.Candidate{{ID: "1", Width: 1920, Height: 1080}, {ID: "2", Width: 3840, Height: 2160}}, "2"},
		{"then the oldest ID, as a number", []scrape.Candidate{{ID: "10"}, {ID: "9"}}, "9"},
	} {
		got := scrape.Pick(c.cands, "de")
		switch {
		case got == nil && c.want != "":
			t.Errorf("%s: none, want %s", c.name, c.want)
		case got != nil && got.ID != c.want:
			t.Errorf("%s: %s, want %s", c.name, got.ID, c.want)
		}
	}
}

// fanart and commons are the two image sources, by artist_images' names.
const (
	fanart  = store.ImageFromFanart
	commons = store.ImageFromWikimedia
)

func answered(source string, ids ...string) scrape.Answer {
	a := scrape.Answer{Source: source, Answered: true}
	for _, id := range ids {
		a.Candidates = append(a.Candidates, scrape.Candidate{Source: source, ID: id})
	}
	return a
}

func silent(source string) scrape.Answer { return scrape.Answer{Source: source} }

func TestDecideKeepsPinsAndFolders(t *testing.T) {
	t.Parallel()
	for _, src := range []string{"pinned", "folder"} {
		d := scrape.Decide(&store.HeldImage{Source: src}, []scrape.Answer{answered(fanart, "1"), answered(commons, "x")}, "en")
		if d.Set != nil || d.Clear != "" {
			t.Errorf("over a %s: %+v, want it kept", src, d)
		}
	}
}

func TestDecideFollowsPrecedence(t *testing.T) {
	t.Parallel()
	held := func(src, id string) *store.HeldImage { return &store.HeldImage{Source: src, RemoteID: id} }
	for _, c := range []struct {
		name      string
		held      *store.HeldImage
		answers   []scrape.Answer
		set, gone string
	}{
		{"fanart.tv over Commons", nil, []scrape.Answer{answered(fanart, "1"), answered(commons, "x")}, "1", ""},
		{"Commons when fanart.tv has none", nil, []scrape.Answer{answered(fanart), answered(commons, "x")}, "x", ""},
		{"the same image is kept", held(fanart, "1"), []scrape.Answer{answered(fanart, "1")}, "", ""},
		{"a new one replaces it", held(fanart, "1"), []scrape.Answer{answered(fanart, "2")}, "2", ""},
		{"fanart.tv replaces Commons", held(commons, "x"), []scrape.Answer{answered(fanart, "1"), silent(commons)}, "1", ""},
		{"a failed source keeps its own", held(fanart, "1"), []scrape.Answer{silent(fanart), answered(commons, "x")}, "", ""},
		{"a source with none takes its own away", held(fanart, "1"), []scrape.Answer{answered(fanart), silent(commons)}, "", fanart},
		{"unless a later source has one", held(fanart, "1"), []scrape.Answer{answered(fanart), answered(commons, "x")}, "x", ""},
		{"nothing asked keeps", held(commons, "x"), []scrape.Answer{silent(fanart), silent(commons)}, "", ""},
	} {
		d := scrape.Decide(c.held, c.answers, "en")
		var set string
		if d.Set != nil {
			set = d.Set.ID
		}
		if set != c.set || d.Clear != c.gone {
			t.Errorf("%s: set %q, clear %q; want %q, %q", c.name, set, d.Clear, c.set, c.gone)
		}
	}
}

func TestDecideBio(t *testing.T) {
	t.Parallel()
	bio := &store.Bio{Summary: "Hi."}
	wiki := func(b *store.Bio) []scrape.BioAnswer {
		return []scrape.BioAnswer{{Source: store.BioFromWikipedia, Answered: true, Bio: b}}
	}
	if d := scrape.DecideBio(&store.HeldBio{Source: "nfo"}, wiki(bio)); d.Set != nil || d.Clear != "" {
		t.Errorf("over an artist.nfo: %+v", d)
	}
	if d := scrape.DecideBio(nil, wiki(bio)); d.Set != bio || d.From != store.BioFromWikipedia {
		t.Errorf("none held: %+v", d)
	}
	if d := scrape.DecideBio(&store.HeldBio{Source: store.BioFromWikipedia}, wiki(nil)); d.Clear != store.BioFromWikipedia {
		t.Errorf("the article went: %+v", d)
	}
	silentWiki := []scrape.BioAnswer{{Source: store.BioFromWikipedia}}
	if d := scrape.DecideBio(&store.HeldBio{Source: store.BioFromWikipedia}, silentWiki); d.Set != nil || d.Clear != "" {
		t.Errorf("Wikipedia down: %+v, want the bio kept", d)
	}
}

// askedAt is a fixed time, and askedMBID an artist, for NextAsk.
const askedMBID = "5b1c0d2e-0000-4000-8000-0000000000e2"

func askedAt() time.Time { return time.UnixMilli(1_761_000_000_000).UTC() }

func TestFoundIsAskedAgainInAMonthSpread(t *testing.T) {
	t.Parallel()
	now := askedAt()
	found := scrape.NextAsk(store.ScrapeFound, nil, askedMBID, "", now)
	if wait := found.NextAt.Sub(now); wait < 27*24*time.Hour || wait > 33*24*time.Hour {
		t.Errorf("found: asked again in %v, want 27 to 33 days", wait)
	}
	if again := scrape.NextAsk(store.ScrapeFound, nil, askedMBID, "", now); !again.NextAt.Equal(found.NextAt) {
		t.Error("the spread isn't the same for one MBID")
	}
	if !scrape.Due(nil, now) || scrape.Due(&found, now) || !scrape.Due(&found, found.NextAt) {
		t.Error("Due")
	}
}

func TestNoneIsAskedAgainInAWeekThenAMonth(t *testing.T) {
	t.Parallel()
	now := askedAt()
	none := scrape.NextAsk(store.ScrapeNone, nil, askedMBID, "", now)
	if none.NextAt.Sub(now) != 7*24*time.Hour {
		t.Errorf("none the first time: %v", none.NextAt.Sub(now))
	}
	if n := scrape.NextAsk(store.ScrapeNone, &none, askedMBID, "", now); n.NextAt.Sub(now) != 30*24*time.Hour {
		t.Errorf("none again: %v", n.NextAt.Sub(now))
	}
}

func TestFailuresBackOff(t *testing.T) {
	t.Parallel()
	now := askedAt()
	var prev *store.ScrapeState
	for i, want := range []time.Duration{time.Hour, 6 * time.Hour, 24 * time.Hour, 7 * 24 * time.Hour, 7 * 24 * time.Hour} {
		st := scrape.NextAsk(store.ScrapeFailed, prev, askedMBID, "fanart.tv answered 503", now)
		if st.Failures != i+1 || st.NextAt.Sub(now) != want || st.Error == "" {
			t.Errorf("failure %d: %+v, want %v", i+1, st, want)
		}
		prev = &st
	}
}

func TestAskable(t *testing.T) {
	t.Parallel()
	for mbid, want := range map[string]bool{
		"5b1c0d2e-0000-4000-8000-0000000000e2": true,
		"89ad4ac3-39f7-470e-963a-56509c546377": false, // Various Artists
		"125ec42a-7229-4250-afc5-e057484327fe": false, // [unknown]
		"f731ccc4-e22a-43af-a747-64213329e088": false, // [anonymous]
		"5B1C0D2E-0000-4000-8000-0000000000E2": false, // Not as MusicBrainz writes one.
		"":                                     false,
		"not-an-mbid":                          false,
	} {
		if got := scrape.Askable(mbid); got != want {
			t.Errorf("Askable(%q) = %v, want %v", mbid, got, want)
		}
	}
}
