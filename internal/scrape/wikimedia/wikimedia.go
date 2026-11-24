// Package wikimedia is a client for the Wikimedia sites an artist's bio and
// portrait come from: Wikidata (an item's articles and its image, P18),
// Wikipedia (an article's summary) and Wikimedia Commons (a file's address and
// credit). One gate covers all of them, one request a second and one at once,
// well under the limits Wikimedia sets for a client that names itself. It
// knows nothing of the store.
package wikimedia

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/simonhull/kitsune/internal/remote"
)

// Defaults of [Options].
const (
	defaultInterval = time.Second
	// maxJSON caps an answer: a summary, an item's sitelinks or one file's
	// facts are small.
	maxJSON = 2 << 20
	// maxImage caps a portrait, which Commons scales to the width asked.
	maxImage = 16 << 20
)

// ErrBadAnswer means a site answered with something Kitsune can't use: JSON
// that doesn't parse, or an image's address that isn't Commons'.
var ErrBadAnswer = errors.New("wikimedia's answer isn't usable")

// Options sets up a [Client]. The zero value talks to Wikimedia itself.
type Options struct {
	// BaseURL, when set, is a stand-in's root serving /wikidata/w/api.php,
	// /wikipedia/{lang}/api/rest_v1/, /commons/w/api.php and images under
	// /upload/. "" means the real sites.
	BaseURL string
	// Version is Kitsune's, for the User-Agent Wikimedia asks for.
	Version string
	// HTTP sends the requests. Nil means a client with a 30 s timeout.
	HTTP *http.Client
	// Interval spaces the requests. 0 means a second.
	Interval time.Duration
	// Retries and Backoff are [remote.Options]'.
	Retries int
	Backoff time.Duration
	// Logger notes retries. Nil discards.
	Logger *slog.Logger
}

// Client asks Wikimedia's sites. One serves the process; it is safe for
// concurrent use.
type Client struct {
	wikidata, commons, uploads string
	wikipedia                  func(lang string) string // The REST root of a language's Wikipedia.
	json, images               *remote.Client
}

// New returns a client set up by o.
func New(o Options) *Client {
	gate := remote.NewGate(cmp.Or(o.Interval, defaultInterval))
	base := remote.Options{
		Agent: remote.UserAgent(o.Version), HTTP: o.HTTP, Gate: gate, Retries: o.Retries, Backoff: o.Backoff,
		Logger: o.Logger, Name: "Wikimedia",
	}
	jsonOpts, imageOpts := base, base
	jsonOpts.MaxBody, imageOpts.MaxBody = maxJSON, maxImage
	c := &Client{json: remote.New(jsonOpts), images: remote.New(imageOpts)}
	if root := strings.TrimSuffix(o.BaseURL, "/"); root != "" {
		c.wikidata, c.commons, c.uploads = root+"/wikidata/w/api.php", root+"/commons/w/api.php", root+"/upload/"
		c.wikipedia = func(lang string) string { return root + "/wikipedia/" + lang + "/api/rest_v1/" }
	} else {
		c.wikidata, c.commons, c.uploads = "https://www.wikidata.org/w/api.php", "https://commons.wikimedia.org/w/api.php", "https://upload.wikimedia.org/"
		c.wikipedia = func(lang string) string { return "https://" + lang + ".wikipedia.org/api/rest_v1/" }
	}
	return c
}

// What every API call asks.
const (
	paramAction = "action"
	paramFormat = "format"
	formatJSON  = "json"
	schemeHTTPS = "https"
)

// langPattern is a language's code as Wikipedia names its sites.
var langPattern = regexp.MustCompile(`^[a-z]{2,3}$`)

// Articles returns the item's Wikipedia articles in langs, by language, their
// titles as the sites spell them. It returns an error wrapping
// [remote.ErrNotFound] for an item Wikidata doesn't have.
func (c *Client) Articles(ctx context.Context, item string, langs []string) (map[string]string, error) {
	sites := make([]string, 0, len(langs))
	for _, l := range langs {
		if langPattern.MatchString(l) {
			sites = append(sites, l+"wiki")
		}
	}
	q := url.Values{
		paramAction: {"wbgetentities"}, "ids": {item}, "props": {"sitelinks"},
		"sitefilter": {strings.Join(sites, "|")}, paramFormat: {formatJSON},
	}
	var raw struct {
		Entities map[string]struct {
			Missing   *string `json:"missing"`
			Sitelinks map[string]struct {
				Title string `json:"title"`
			} `json:"sitelinks"`
		} `json:"entities"`
	}
	if err := c.getJSON(ctx, c.wikidata+"?"+q.Encode(), &raw); err != nil {
		return nil, fmt.Errorf("wikidata %s: %w", item, err)
	}
	e, ok := raw.Entities[item]
	if !ok || e.Missing != nil {
		return nil, fmt.Errorf("wikidata %s: %w", item, remote.ErrNotFound)
	}
	out := map[string]string{}
	for site, link := range e.Sitelinks {
		if lang, isWiki := strings.CutSuffix(site, "wiki"); isWiki && link.Title != "" {
			out[lang] = link.Title
		}
	}
	return out, nil
}

// Portrait returns the file name of the item's image (Wikidata's P18), or ""
// when it has none.
func (c *Client) Portrait(ctx context.Context, item string) (string, error) {
	q := url.Values{paramAction: {"wbgetclaims"}, "entity": {item}, "property": {"P18"}, paramFormat: {formatJSON}}
	var raw struct {
		Claims map[string][]struct {
			Rank     string `json:"rank"`
			Mainsnak struct {
				Datavalue struct {
					Value jsontext.Value `json:"value"`
				} `json:"datavalue"`
			} `json:"mainsnak"`
		} `json:"claims"`
	}
	if err := c.getJSON(ctx, c.wikidata+"?"+q.Encode(), &raw); err != nil {
		return "", fmt.Errorf("wikidata %s image: %w", item, err)
	}
	var best string
	for _, claim := range raw.Claims["P18"] {
		var file string
		if json.Unmarshal(claim.Mainsnak.Datavalue.Value, &file) != nil || file == "" || claim.Rank == "deprecated" {
			continue
		}
		if best == "" || claim.Rank == "preferred" {
			best = file
		}
	}
	return best, nil
}

// Summary is the lead of a Wikipedia article, as plain text.
type Summary struct {
	Title   string
	Extract string // Paragraphs split by newlines.
	PageURL string
	Lang    string
}

// Summary returns the summary of the article title on lang's Wikipedia. A
// disambiguation page, or one with no text, is [remote.ErrNotFound].
func (c *Client) Summary(ctx context.Context, lang, title string) (Summary, error) {
	if !langPattern.MatchString(lang) {
		return Summary{}, fmt.Errorf("wikipedia: %w", remote.ErrNotFound)
	}
	var raw struct {
		Type        string `json:"type"`
		Title       string `json:"title"`
		Extract     string `json:"extract"`
		Lang        string `json:"lang"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}
	u := c.wikipedia(lang) + "page/summary/" + url.PathEscape(strings.ReplaceAll(title, " ", "_"))
	if err := c.getJSON(ctx, u, &raw); err != nil {
		return Summary{}, fmt.Errorf("wikipedia %s: %w", lang, err)
	}
	if raw.Type == "disambiguation" || strings.TrimSpace(raw.Extract) == "" {
		return Summary{}, fmt.Errorf("wikipedia %s: %w", lang, remote.ErrNotFound)
	}
	page := raw.ContentURLs.Desktop.Page
	if p, err := url.Parse(page); err != nil || p.Scheme != schemeHTTPS {
		page = ""
	}
	return Summary{Title: raw.Title, Extract: strings.TrimSpace(raw.Extract), PageURL: page, Lang: cmp.Or(raw.Lang, lang)}, nil
}

// File is a Commons file as a portrait needs it: where to fetch it at the
// width asked, and the credit its license asks for.
type File struct {
	URL           string // Under Commons' uploads, checked.
	Width, Height int    // Of the file at URL.
	Author        string // Plain text.
	License       string // Such as "CC BY-SA 4.0".
	LicenseURL    string
	PageURL       string // The file's page on Commons.
}

// File returns the Commons file name, scaled to width. It returns an error
// wrapping [remote.ErrNotFound] for a file Commons doesn't have, and
// [ErrBadAnswer] for one whose address isn't Commons'.
func (c *Client) File(ctx context.Context, name string, width int) (File, error) {
	q := url.Values{
		paramAction: {"query"}, "titles": {"File:" + name}, "prop": {"imageinfo"},
		"iiprop": {"url|size|extmetadata"}, "iiurlwidth": {strconv.Itoa(width)},
		"iiextmetadatafilter": {"Artist|LicenseShortName|LicenseUrl"},
		paramFormat:           {formatJSON}, "formatversion": {"2"},
	}
	var raw struct {
		Query struct {
			Pages []struct {
				Missing   bool        `json:"missing"`
				ImageInfo []imageInfo `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := c.getJSON(ctx, c.commons+"?"+q.Encode(), &raw); err != nil {
		return File{}, fmt.Errorf("commons: %w", err)
	}
	if len(raw.Query.Pages) == 0 || raw.Query.Pages[0].Missing || len(raw.Query.Pages[0].ImageInfo) == 0 {
		return File{}, fmt.Errorf("commons: %w", remote.ErrNotFound)
	}
	f := raw.Query.Pages[0].ImageInfo[0].file()
	if !strings.HasPrefix(f.URL, c.uploads) {
		return File{}, fmt.Errorf("commons: %w: an image's address isn't Commons'", ErrBadAnswer)
	}
	return f, nil
}

// imageInfo is a file's imageinfo, as much as Kitsune reads.
type imageInfo struct {
	URL            string `json:"url"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	ThumbURL       string `json:"thumburl"`
	ThumbWidth     int    `json:"thumbwidth"`
	ThumbHeight    int    `json:"thumbheight"`
	DescriptionURL string `json:"descriptionurl"`
	ExtMetadata    map[string]struct {
		Value string `json:"value"`
	} `json:"extmetadata"`
}

func (i *imageInfo) file() File {
	f := File{URL: i.URL, Width: i.Width, Height: i.Height, PageURL: i.DescriptionURL}
	if i.ThumbURL != "" && i.ThumbWidth > 0 {
		f.URL, f.Width, f.Height = i.ThumbURL, i.ThumbWidth, i.ThumbHeight
	}
	f.Author = PlainText(i.ExtMetadata["Artist"].Value)
	f.License = PlainText(i.ExtMetadata["LicenseShortName"].Value)
	if u, err := url.Parse(i.ExtMetadata["LicenseUrl"].Value); err == nil && (u.Scheme == schemeHTTPS || u.Scheme == "http") {
		f.LicenseURL = u.String()
	}
	if u, err := url.Parse(f.PageURL); err != nil || u.Scheme != schemeHTTPS {
		f.PageURL = ""
	}
	return f
}

// tags matches an HTML tag.
var tags = regexp.MustCompile(`<[^>]*>`)

// PlainText is s with its HTML tags taken out, its entities read, and its
// spaces collapsed: Commons' credits are HTML.
func PlainText(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tags.ReplaceAllString(s, " "))), " ")
}

// Download returns the bytes of a file [Client.File] returned. One whose
// address isn't under Commons' uploads is [ErrBadAnswer], never fetched.
func (c *Client) Download(ctx context.Context, u string) ([]byte, error) {
	if !strings.HasPrefix(u, c.uploads) || strings.Contains(u, "..") {
		return nil, fmt.Errorf("commons image: %w", ErrBadAnswer)
	}
	body, err := c.images.Get(ctx, u, u, nil)
	if err != nil {
		return nil, fmt.Errorf("commons image: %w", err)
	}
	return body, nil
}

// getJSON asks for u and reads its answer into v.
func (c *Client) getJSON(ctx context.Context, u string, v any) error {
	body, err := c.json.Get(ctx, u, u, http.Header{"Accept": {"application/json"}})
	if err != nil {
		return err //nolint:wrapcheck // The caller names the site.
	}
	if err = json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%w: it doesn't parse", ErrBadAnswer)
	}
	return nil
}
