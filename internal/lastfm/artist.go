package lastfm

import (
	"context"
	"errors"
	"html"
	"net/url"
	"regexp"
	"strings"
)

// What [Client.ArtistInfo] may fail with besides [ErrTemporary] and
// [ErrRejected].
var (
	// ErrNotFound means Last.fm knows no artist by the MBID asked.
	ErrNotFound = errors.New("last.fm has no such artist")
	// ErrKeyRefused means Last.fm refused the API key: invalid or suspended.
	// Asking again with it fails the same way.
	ErrKeyRefused = errors.New("last.fm refused the API key")
)

// Last.fm error codes artist.getInfo tells apart.
const (
	codeInvalidParameters = 6 // For artist.getInfo: no such artist.
	codeInvalidAPIKey     = 10
	codeSuspendedAPIKey   = 26
)

// Bio is an artist's bio on Last.fm, as plain text: links dropped, HTML
// stripped, and the "Read more" line kept only as ReadMore.
type Bio struct {
	MBID       string   // The MBID Last.fm answered for, which a caller compares with the one asked.
	Summary    string   // The bio's first paragraph, as Last.fm summarizes it.
	Paragraphs []string // The whole bio, by paragraph; nil when Last.fm has none.
	ReadMore   string   // The page the bio links to, on last.fm.
}

// ArtistInfo returns the bio Last.fm has of the artist with MusicBrainz ID
// mbid, in lang (a two-letter code) where it has one. The call is unsigned:
// it needs the API key alone, which travels in the body as every parameter
// does, never in the URL. The caller compares Bio.MBID with mbid: Last.fm has
// been known to answer for another.
//
// # Errors
//
// The error wraps [ErrNotFound] for an artist Last.fm doesn't know,
// [ErrKeyRefused] for a refused key, else [ErrTemporary] or [ErrRejected].
func (c *Client) ArtistInfo(ctx context.Context, mbid, lang string) (Bio, error) {
	const method = "artist.getInfo"
	params := url.Values{"mbid": {mbid}, "autocorrect": {"0"}}
	if lang != "" {
		params.Set("lang", lang)
	}
	var resp struct {
		Artist struct {
			MBID string `json:"mbid"`
			URL  string `json:"url"`
			Bio  struct {
				Summary string `json:"summary"`
				Content string `json:"content"`
			} `json:"bio"`
		} `json:"artist"`
	}
	err := c.post(ctx, method, params, &resp, false)
	if e, ok := errors.AsType[*Error](err); ok {
		switch e.Code {
		case codeInvalidParameters:
			e.kind = ErrNotFound
		case codeInvalidAPIKey, codeSuspendedAPIKey:
			e.kind = ErrKeyRefused
		}
		return Bio{}, e
	}
	if err != nil {
		return Bio{}, err
	}
	a := &resp.Artist
	paras, more := Paragraphs(a.Bio.Content)
	summary, _ := Paragraphs(a.Bio.Summary)
	b := Bio{MBID: a.MBID, Paragraphs: paras, ReadMore: more}
	if len(summary) > 0 {
		b.Summary = summary[0]
	} else if len(paras) > 0 {
		b.Summary = paras[0]
	}
	if b.ReadMore == "" && isLastfmPage(a.URL) {
		b.ReadMore = a.URL
	}
	return b, nil
}

var (
	// readMore is Last.fm's closing link, and everything after it: the
	// license line, which a credit says instead.
	readMore = regexp.MustCompile(`(?is)<a\s[^>]*href="([^"]*)"[^>]*>\s*Read more on Last\.fm\s*</a>.*$`)
	// lineBreak is a tag that breaks a line.
	lineBreak = regexp.MustCompile(`(?i)<br\s*/?>|</p>`)
	// tag is any tag; a link's words stay, the link goes.
	tag = regexp.MustCompile(`<[^>]*>`)
)

// Paragraphs is Last.fm's bio HTML as plain paragraphs, with the page its
// "Read more on Last.fm" link names ("" when it has none, or one that isn't
// last.fm's). Links are dropped, keeping their words.
func Paragraphs(s string) ([]string, string) {
	more := ""
	if m := readMore.FindStringSubmatch(s); m != nil {
		if u := html.UnescapeString(m[1]); isLastfmPage(u) {
			more = u
		}
		s = s[:len(s)-len(m[0])]
	}
	s = lineBreak.ReplaceAllString(s, "\n")
	s = html.UnescapeString(tag.ReplaceAllString(s, ""))
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			out = append(out, line)
		}
	}
	return out, more
}

// isLastfmPage reports whether u is a page on last.fm, over https or http.
func isLastfmPage(u string) bool {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "https" && p.Scheme != "http") {
		return false
	}
	host := strings.ToLower(p.Hostname())
	return host == "last.fm" || strings.HasSuffix(host, ".last.fm")
}
