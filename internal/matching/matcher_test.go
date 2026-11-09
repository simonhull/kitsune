package matching_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/matching/musicbrainztest"
)

// newMatcher returns a matcher over a stand-in holding releases.
func newMatcher(t *testing.T, releases ...matching.Release) (*matching.Matcher, *musicbrainztest.Server) {
	t.Helper()
	mb := musicbrainztest.NewInMemory(releases...)
	return matching.New(matching.NewMusicBrainz(mb.Options())), mb
}

// evidence returns the kinds and verdicts of c's evidence, as "kind:verdict".
func evidence(c *matching.Candidate) []string {
	out := make([]string, len(c.Evidence))
	for i, e := range c.Evidence {
		out[i] = string(e.Kind) + ":" + string(e.Verdict)
	}
	return out
}

// wantValues checks that got holds every field of want, as want has it.
func wantValues(t *testing.T, what string, got, want matching.Values) {
	t.Helper()
	for field, values := range want {
		if !slices.Equal(got[field], values) {
			t.Errorf("%s %s = %q, want %q", what, field, got[field], values)
		}
	}
}

// wantCandidate checks c's score is from least to most, and its evidence.
func wantCandidate(t *testing.T, name string, c *matching.Candidate, least, most int, want ...string) {
	t.Helper()
	if c.Score < least || c.Score > most {
		t.Errorf("%s scores %d, want %d to %d", name, c.Score, least, most)
	}
	if got := evidence(c); !slices.Equal(got, want) {
		t.Errorf("%s evidence = %q, want %q", name, got, want)
	}
	if c.Release.Media[0].Tracks != nil {
		t.Errorf("%s keeps its tracklist in the list", name)
	}
}

// ids returns the release IDs of candidates, in order.
func ids(candidates []matching.Candidate) []string {
	out := make([]string, len(candidates))
	for i := range candidates {
		out[i] = candidates[i].Release.ID
	}
	return out
}

const agreeingLengths = "lengths:agrees"

// findGlasshouse finds the board's album among the board's releases.
func findGlasshouse(t *testing.T) (*matching.Matcher, matching.Found) {
	t.Helper()
	m, _ := newMatcher(t, glassHouseChorus(), glasshouseVinyl(), glasshouseDeluxe(), glasshouseCD())
	found, err := m.Find(t.Context(), glasshouseAlbum(), &matching.Query{})
	if err != nil {
		t.Fatal(err)
	}
	return m, found
}

func TestFindRanksReleasesByTheirEvidence(t *testing.T) {
	t.Parallel()
	m, found := findGlasshouse(t)
	if found.Query.Album != glasshouse || found.Query.Artist != lowTide {
		t.Errorf("asked %+v, want the album's own title and artist", found.Query)
	}
	// The chorus's Glass House isn't found by the phrase, as on MusicBrainz.
	want := []string{glasshouseCD().ID, glasshouseDeluxe().ID, glasshouseVinyl().ID}
	if got := ids(found.Candidates); !slices.Equal(got, want) {
		t.Fatalf("order = %q, want the CD, the deluxe, then the vinyl %q", got, want)
	}
	review, err := m.Review(t.Context(), glasshouseAlbum(), glassHouseChorus().ID)
	if err != nil {
		t.Fatal(err)
	}
	c := found.Candidates
	wantCandidate(t, "cd", &c[0], 90, 100, agreeingLengths, "titleArtist:agrees", "discs:neutral")
	wantCandidate(t, "deluxe", &c[1], 60, 89, agreeingLengths, "title:differs", "discs:neutral", "songs:differs")
	wantCandidate(t, "vinyl", &c[2], 60, 89, agreeingLengths, "titleArtist:agrees", "discs:differs")
	wantCandidate(t, "chorus", &review.Candidate, 0, 39, "lengths:neutral", "artist:differs", "discs:differs", "songs:differs")
}

func TestEvidenceCarriesItsFacts(t *testing.T) {
	t.Parallel()
	_, found := findGlasshouse(t)
	cd := found.Candidates[0]
	if !strings.HasSuffix(cd.Cover, "/release/"+cd.Release.ID+"/front-250") || cd.Page != "https://musicbrainz.org/release/"+cd.Release.ID {
		t.Errorf("cover %q and page %q, want the release's", cd.Cover, cd.Page)
	}
	if e := cd.Evidence[0]; e.Count != 15 || e.Of != 17 {
		t.Errorf("CD lengths %d of %d, want 15 of 17: the swapped pair differs", e.Count, e.Of)
	}
	if e := found.Candidates[2].Evidence[2]; e.Count != 4 || e.Of != 2 || e.Format != `12" Vinyl` {
		t.Errorf("vinyl discs = %+v, want 4 sides of vinyl, not 2 discs", e)
	}
}

func TestFindLooksUpTheReleaseTheFilesName(t *testing.T) {
	t.Parallel()
	cd, other := glasshouseCD(), glassHouseChorus()
	m, mb := newMatcher(t, cd, other)
	album := glasshouseAlbum()
	for i := range 3 {
		album.Tracks[i].ReleaseID = cd.ID
	}
	// Searched for something else, the tagged release still comes, first.
	found, err := m.Find(t.Context(), album, &matching.Query{Album: "Glass House", Artist: "Low Tide Chorus"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Candidates) != 2 || found.Candidates[0].Release.ID != cd.ID {
		t.Fatalf("candidates = %+v, want the tagged release first", found.Candidates)
	}
	if got := evidence(&found.Candidates[0]); !slices.Contains(got, "tagged:agrees") {
		t.Errorf("evidence = %q, want the tagged songs counted", got)
	}
	if found.Candidates[1].Score >= 40 {
		t.Errorf("an untagged release when the files name another scores %d", found.Candidates[1].Score)
	}
	if calls := mb.Calls(); calls[0].Path != "release/"+cd.ID {
		t.Errorf("first call %q, want the tagged release", calls[0].Path)
	}
}

func TestFindLoosensAPhraseThatFindsNothing(t *testing.T) {
	t.Parallel()
	m, mb := newMatcher(t, glasshouseCD())
	found, err := m.Find(t.Context(), glasshouseAlbum(), &matching.Query{Album: "Glasshouse Sessions", Artist: lowTide})
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Candidates) != 1 {
		t.Fatalf("found %d, want the release by its words", len(found.Candidates))
	}
	calls := mb.Calls()
	if calls[0].Query != `release:"Glasshouse Sessions" AND artist:"Low Tide Choir"` ||
		calls[1].Query != `release:(Glasshouse Sessions) AND artist:(Low Tide Choir)` {
		t.Errorf("queries %q then %q", calls[0].Query, calls[1].Query)
	}
}

func TestFindWithNothingFound(t *testing.T) {
	t.Parallel()
	m, _ := newMatcher(t)
	found, err := m.Find(t.Context(), glasshouseAlbum(), &matching.Query{})
	if err != nil || len(found.Candidates) != 0 {
		t.Errorf("Find = %+v, %v; want nothing, and no error", found, err)
	}
}

func TestReviewPairsAndProposes(t *testing.T) {
	t.Parallel()
	cd := glasshouseCD()
	m, _ := newMatcher(t, cd)
	album := glasshouseAlbum()
	review, err := m.Review(t.Context(), album, cd.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Pairs) != len(album.Tracks) {
		t.Fatalf("%d pairs for %d tracks", len(review.Pairs), len(album.Tracks))
	}
	// The swapped two pair by position, so their lengths show the swap.
	if !slices.EqualFunc(review.Pairs, review.Pairs, func(p, _ matching.Pair) bool { return p.Track == p.Release }) {
		t.Errorf("pairs = %+v, want each track with the release's in order", review.Pairs)
	}
	p := review.Proposal
	wantValues(t, "album", p.Album, matching.Values{
		library.FieldAlbum: {glasshouse}, library.FieldAlbumArtists: {lowTide}, library.FieldDate: {released},
		library.FieldOriginalDate: {released}, library.FieldLabel: {"Saltmarsh Records"},
		library.FieldCatalogNumber: {"SALT 041"}, library.FieldBarcode: {"5060000000417"},
		library.FieldGenres: {"Dream pop", "Ambient pop"},
	})
	if p.AlbumIDs[library.FieldMusicBrainzAlbumID][0] != cd.ID || len(p.AlbumIDs[library.FieldMusicBrainzAlbumArtistID]) != 1 {
		t.Errorf("album IDs = %q", p.AlbumIDs)
	}
	room := p.Tracks[9]
	wantValues(t, "track", room.Proposed, matching.Values{
		library.FieldTitle: {"Breakwater (Room Version)"}, library.FieldTrack: {"1"}, library.FieldTrackTotal: {"8"},
		library.FieldDisc: {"2"}, library.FieldDiscTotal: {"2"}, library.FieldDiscSubtitle: {roomSessions},
		library.FieldArtists: {lowTide}, library.FieldISRC: {"GBSLT2100009"},
	})
	wantValues(t, "track", room.IDs, matching.Values{
		library.FieldMusicBrainzRecordingID: {cd.Media[1].Tracks[0].RecordingID},
		library.FieldMusicBrainzTrackID:     {cd.Media[1].Tracks[0].ID},
	})
	if _, named := p.Tracks[0].Proposed[library.FieldDiscSubtitle]; named {
		t.Error("a medium without a name proposes taking the disc's name away")
	}
}

func TestReviewOfAnUnknownRelease(t *testing.T) {
	t.Parallel()
	m, _ := newMatcher(t)
	for _, id := range []string{musicbrainztest.ID("nothing"), "not-an-mbid"} {
		if _, err := m.Review(t.Context(), glasshouseAlbum(), id); !errorIs(err, matching.ErrNotFound) {
			t.Errorf("Review(%s) = %v, want ErrNotFound", id, err)
		}
	}
}
