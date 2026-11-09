package matching

import (
	"regexp"
	"slices"
	"strings"
	"time"
)

// mbidPattern is the shape of a MusicBrainz ID.
var mbidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// MusicBrainz's special label and catalog number for a release without one.
const (
	mbNoLabel   = "157afde4-4bf5-4039-8ad2-5a15acc85176"
	mbNoCatalog = "[none]"
)

// mbRelease is a release as MusicBrainz's JSON has it, from a lookup or a search.
type mbRelease struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Status       string     `json:"status"`
	Date         string     `json:"date"`
	Country      string     `json:"country"`
	Barcode      string     `json:"barcode"`
	ArtistCredit []mbCredit `json:"artist-credit"`
	ReleaseGroup struct {
		ID               string `json:"id"`
		FirstReleaseDate string `json:"first-release-date"`
	} `json:"release-group"`
	LabelInfo []struct {
		CatalogNumber string `json:"catalog-number"`
		Label         struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"label"`
	} `json:"label-info"`
	Media []struct {
		Position   int       `json:"position"`
		Format     string    `json:"format"`
		Title      string    `json:"title"`
		TrackCount int       `json:"track-count"`
		Tracks     []mbTrack `json:"tracks"`
	} `json:"media"`
	Genres          []mbGenre `json:"genres"`
	CoverArtArchive struct {
		Front bool `json:"front"`
	} `json:"cover-art-archive"`
}

type mbGenre struct {
	Name  string `json:"name"`
	Count int    `json:"count"` // Votes.
}

type mbCredit struct {
	Name       string `json:"name"`
	JoinPhrase string `json:"joinphrase"`
	Artist     struct {
		ID string `json:"id"`
	} `json:"artist"`
}

type mbTrack struct {
	ID           string     `json:"id"`
	Position     int        `json:"position"`
	Title        string     `json:"title"`
	Length       int64      `json:"length"` // Milliseconds.
	ArtistCredit []mbCredit `json:"artist-credit"`
	Recording    struct {
		ID     string   `json:"id"`
		Length int64    `json:"length"`
		ISRCs  []string `json:"isrcs"`
	} `json:"recording"`
}

// release returns r as a [Release]. listed says r came from a lookup, so its
// media hold every track.
func (r *mbRelease) release(listed bool) Release {
	out := Release{
		Source: SourceMusicBrainz, ID: r.ID, GroupID: r.ReleaseGroup.ID, Title: strings.TrimSpace(r.Title),
		Artists: credits(r.ArtistCredit), Date: r.Date, Original: r.ReleaseGroup.FirstReleaseDate,
		Country: r.Country, Barcode: strings.TrimSpace(r.Barcode), Status: r.Status, Cover: r.CoverArtArchive.Front,
	}
	out.Labels = r.labels()
	out.Genres = r.genres()
	for i := range r.Media {
		m := &r.Media[i]
		medium := Medium{Position: m.Position, Format: m.Format, Title: strings.TrimSpace(m.Title), TrackCount: m.TrackCount}
		if listed {
			medium.Tracks = make([]ReleaseTrack, len(m.Tracks))
			for j := range m.Tracks {
				medium.Tracks[j] = m.Tracks[j].track()
			}
			medium.TrackCount = len(m.Tracks)
		}
		out.Media = append(out.Media, medium)
	}
	return out
}

// labels returns the labels and catalog numbers, each pair once, without
// MusicBrainz's stand-ins for none.
func (r *mbRelease) labels() []Label {
	var out []Label
	for _, li := range r.LabelInfo {
		name := strings.TrimSpace(li.Label.Name)
		if li.Label.ID == mbNoLabel {
			name = ""
		}
		catalog := strings.TrimSpace(li.CatalogNumber)
		if strings.EqualFold(catalog, mbNoCatalog) {
			catalog = ""
		}
		if l := (Label{Name: name, CatalogNumber: catalog}); (name != "" || catalog != "") && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// genres returns the genres, most voted first.
func (r *mbRelease) genres() []string {
	sorted := slices.Clone(r.Genres)
	slices.SortStableFunc(sorted, func(a, b mbGenre) int { return b.Count - a.Count })
	out := make([]string, len(sorted))
	for i, g := range sorted {
		out[i] = g.Name
	}
	return out
}

func (t *mbTrack) track() ReleaseTrack {
	length := t.Length
	if length == 0 {
		length = t.Recording.Length
	}
	return ReleaseTrack{
		ID: t.ID, RecordingID: t.Recording.ID, Position: t.Position, Title: strings.TrimSpace(t.Title),
		Artists: credits(t.ArtistCredit), Length: time.Duration(length) * time.Millisecond, ISRCs: t.Recording.ISRCs,
	}
}

func credits(in []mbCredit) []Credit {
	out := make([]Credit, len(in))
	for i, c := range in {
		out[i] = Credit{Name: strings.TrimSpace(c.Name), ID: c.Artist.ID, Join: c.JoinPhrase}
	}
	return out
}
