package matching

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ArtistLinksFor is how long an artist's links are kept: they rarely change.
const ArtistLinksFor = 7 * 24 * time.Hour

// wikidataPattern is a Wikidata item's ID.
var wikidataPattern = regexp.MustCompile(`^Q[1-9]\d*$`)

// ArtistLinks are what MusicBrainz knows of an artist elsewhere, as far as
// Kitsune follows it: its Wikidata item, through which Wikipedia's articles in
// every language and its portrait are found.
type ArtistLinks struct {
	// Wikidata is the artist's Wikidata item, such as Q2831, or "".
	Wikidata string
}

// IsMBID reports whether id is shaped as a MusicBrainz ID.
func IsMBID(id string) bool { return mbidPattern.MatchString(id) }

// ArtistLinks looks up the artist id with its URL relations, kept for a week.
// It returns an error wrapping [ErrNotFound] for an id that isn't an MBID or
// that MusicBrainz doesn't know.
func (c *MusicBrainz) ArtistLinks(ctx context.Context, id string) (ArtistLinks, error) {
	if !mbidPattern.MatchString(id) {
		return ArtistLinks{}, fmt.Errorf("artist %q: %w", id, ErrNotFound)
	}
	key := "artist/" + id + "?" + url.Values{"inc": {"url-rels"}, fmtParam: {fmtJSON}}.Encode()
	body, err := c.getFresh(ctx, key, ArtistLinksFor)
	if err != nil {
		return ArtistLinks{}, fmt.Errorf("artist %s: %w", id, err)
	}
	var a mbArtistRels
	if err = json.Unmarshal(body, &a); err != nil {
		return ArtistLinks{}, fmt.Errorf("artist %s: %w: %w", id, ErrUnavailable, err)
	}
	return a.links(), nil
}

// mbArtistRels is an artist with its URL relations, as MusicBrainz's JSON has it.
type mbArtistRels struct {
	Relations []struct {
		Type string `json:"type"`
		URL  struct {
			Resource string `json:"resource"`
		} `json:"url"`
	} `json:"relations"`
}

func (a *mbArtistRels) links() ArtistLinks {
	var out ArtistLinks
	for _, r := range a.Relations {
		u, err := url.Parse(r.URL.Resource)
		if err != nil || u.Scheme != "https" {
			continue
		}
		if id, ok := wikidataItem(u); ok && r.Type == "wikidata" {
			out.Wikidata = id
			break
		}
	}
	return out
}

// wikidataItem is the item u names on Wikidata, if it names one.
func wikidataItem(u *url.URL) (string, bool) {
	id, ok := strings.CutPrefix(u.Path, "/wiki/")
	return id, ok && u.Host == "www.wikidata.org" && wikidataPattern.MatchString(id)
}
