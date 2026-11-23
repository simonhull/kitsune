// Package fanarttv is a client for fanart.tv's music API (v3.2): an artist's
// images by MusicBrainz ID, the feed of artists with new images, and the
// images themselves. Keys go in headers, never in a URL. It knows nothing of
// the store; the artist pass chooses among what it returns.
package fanarttv

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/simonhull/kitsune/internal/remote"
)

// fanart.tv's own roots, the defaults for [Options].
const (
	DefaultURL       = "https://webservice.fanart.tv/"
	DefaultAssetsURL = "https://assets.fanart.tv/"
)

// Defaults of [Options].
const (
	// defaultInterval spaces the JSON requests: fanart.tv publishes no limit,
	// and answers 429 with Retry-After past it.
	defaultInterval = time.Second
	// defaultImageInterval spaces the image downloads.
	defaultImageInterval = 500 * time.Millisecond
	// maxJSON caps an answer: an artist's list of images is small.
	maxJSON = 2 << 20
	// maxImage caps an image: a 4K background is a few megabytes.
	maxImage = 32 << 20
)

// ErrBadAnswer means fanart.tv answered with something Kitsune can't use: JSON
// that doesn't parse, or an image's address that isn't fanart.tv's, as when in
// November 2025 it sent bare file names. It is never fetched.
var ErrBadAnswer = errors.New("fanart.tv's answer isn't usable")

// ErrNoKey means there is no project key: fanart.tv answers nothing without one.
var ErrNoKey = errors.New("fanart.tv needs a project key")

// Options sets up a [Client]. The zero value talks to fanart.tv itself.
type Options struct {
	// BaseURL is the web service's root, under which /v3.2/ lies. "" means
	// fanart.tv's.
	BaseURL string
	// AssetsURL is where images lie, under /fanart/. An image elsewhere is
	// refused. "" means fanart.tv's.
	AssetsURL string
	// Version is Kitsune's, for the User-Agent.
	Version string
	// HTTP sends the requests. Nil means a client with a 30 s timeout.
	HTTP *http.Client
	// Interval and ImageInterval space the JSON and image requests. 0 means a
	// second and half a second.
	Interval, ImageInterval time.Duration
	// Retries and Backoff are [remote.Options]'.
	Retries int
	Backoff time.Duration
	// Logger notes retries. Nil discards.
	Logger *slog.Logger
}

// Keys are what fanart.tv is asked with: the project key, which it needs, and a
// person's own, which brings new images sooner.
type Keys struct {
	API, Client string
}

// Client asks fanart.tv. One serves the process; it is safe for concurrent use.
type Client struct {
	base, assets string
	json, images *remote.Client
}

// New returns a client set up by o.
func New(o Options) *Client { //nolint:gocritic // hugeParam: built once, at startup.
	agent := remote.UserAgent(o.Version)
	base := remote.Options{Agent: agent, HTTP: o.HTTP, Retries: o.Retries, Backoff: o.Backoff, Logger: o.Logger, Name: "fanart.tv"}
	jsonOpts, imageOpts := base, base
	jsonOpts.Gate, jsonOpts.MaxBody = remote.NewGate(cmp.Or(o.Interval, defaultInterval)), maxJSON
	imageOpts.Gate, imageOpts.MaxBody = remote.NewGate(cmp.Or(o.ImageInterval, defaultImageInterval)), maxImage
	return &Client{
		base:   withSlash(cmp.Or(o.BaseURL, DefaultURL)) + "v3.2/",
		assets: withSlash(cmp.Or(o.AssetsURL, DefaultAssetsURL)) + "fanart/",
		json:   remote.New(jsonOpts), images: remote.New(imageOpts),
	}
}

func withSlash(s string) string {
	if strings.HasSuffix(s, "/") {
		return s
	}
	return s + "/"
}

// Image is one of an artist's images at fanart.tv.
type Image struct {
	ID    string // fanart.tv's, numeric.
	URL   string // Under the assets root, checked.
	Lang  string // A two-letter code, or "" for none.
	Likes int
	// Width and Height are what fanart.tv's rules make an image of its kind.
	Width, Height int
}

// Artist is every image fanart.tv has of an artist, by kind, in its order.
type Artist struct {
	Thumbs        []Image // artistthumb, 1000 × 1000.
	Backgrounds4K []Image // artist4kbackground, 3840 × 2160.
	Backgrounds   []Image // artistbackground, 1920 × 1080.
	HDLogos       []Image // hdmusiclogo, 800 × 310, transparent.
	Logos         []Image // musiclogo, 400 × 155, transparent.
	Banners       []Image // musicbanner, 1000 × 185.
}

// Empty reports whether a has no image at all.
func (a *Artist) Empty() bool {
	return len(a.Thumbs)+len(a.Backgrounds4K)+len(a.Backgrounds)+len(a.HDLogos)+len(a.Logos)+len(a.Banners) == 0
}

// header is keys as fanart.tv takes them.
func (k Keys) header() http.Header {
	h := http.Header{"Accept": {"application/json"}, "Api-Key": {k.API}}
	if k.Client != "" {
		h.Set("Client-Key", k.Client)
	}
	return h
}

// Artist returns the images fanart.tv has of the artist mbid. It returns an
// error wrapping [remote.ErrNotFound] when it has none, [remote.ErrRefused]
// when it refuses the key, [ErrNoKey] without one, and [ErrBadAnswer] for an
// answer that isn't usable.
func (c *Client) Artist(ctx context.Context, keys Keys, mbid string) (Artist, error) {
	if keys.API == "" {
		return Artist{}, ErrNoKey
	}
	body, err := c.json.Get(ctx, "music/"+mbid, c.base+"music/"+url.PathEscape(mbid), keys.header())
	if err != nil {
		return Artist{}, fmt.Errorf("fanart.tv artist: %w", err)
	}
	var raw artistJSON
	if err = json.Unmarshal(body, &raw); err != nil {
		return Artist{}, fmt.Errorf("fanart.tv artist: %w: it doesn't parse", ErrBadAnswer)
	}
	if raw.Status == "error" { // fanart.tv's way of saying none, with a 200.
		return Artist{}, fmt.Errorf("fanart.tv artist: %w", remote.ErrNotFound)
	}
	return c.artist(&raw)
}

// Latest returns the MusicBrainz IDs of the artists with new images since. It
// returns the errors [Client.Artist] does.
func (c *Client) Latest(ctx context.Context, keys Keys, since time.Time) ([]string, error) {
	if keys.API == "" {
		return nil, ErrNoKey
	}
	date := strconv.FormatInt(since.Unix(), 10)
	body, err := c.json.Get(ctx, "music/latest?"+date, c.base+"music/latest?date="+date, keys.header())
	if err != nil {
		return nil, fmt.Errorf("fanart.tv latest: %w", err)
	}
	var raw []struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("fanart.tv latest: %w: it doesn't parse", ErrBadAnswer)
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.ID)
	}
	return out, nil
}

// Download returns the bytes of an image [Client.Artist] returned. One whose
// address isn't under the assets root is [ErrBadAnswer], never fetched.
func (c *Client) Download(ctx context.Context, u string) ([]byte, error) {
	if !c.isAsset(u) {
		return nil, fmt.Errorf("fanart.tv image: %w", ErrBadAnswer)
	}
	body, err := c.images.Get(ctx, u, u, nil)
	if err != nil {
		return nil, fmt.Errorf("fanart.tv image: %w", err)
	}
	return body, nil
}

// artist checks raw's images and gives each kind's its size.
func (c *Client) artist(raw *artistJSON) (Artist, error) {
	var a Artist
	for _, k := range []struct {
		from          []imageJSON
		to            *[]Image
		width, height int
	}{
		{raw.Thumbs, &a.Thumbs, 1000, 1000},
		{raw.Backgrounds4K, &a.Backgrounds4K, 3840, 2160},
		{raw.Backgrounds, &a.Backgrounds, 1920, 1080},
		{raw.HDLogos, &a.HDLogos, 800, 310},
		{raw.Logos, &a.Logos, 400, 155},
		{raw.Banners, &a.Banners, 1000, 185},
	} {
		for _, img := range k.from {
			if !c.isAsset(img.URL) {
				return Artist{}, fmt.Errorf("fanart.tv artist: %w: an image's address isn't fanart.tv's", ErrBadAnswer)
			}
			*k.to = append(*k.to, Image{
				ID: string(img.ID), URL: img.URL, Lang: lang(img.Lang), Likes: int(img.Likes),
				Width: k.width, Height: k.height,
			})
		}
	}
	return a, nil
}

// isAsset reports whether u is an image under the assets root.
func (c *Client) isAsset(u string) bool {
	parsed, err := url.Parse(u)
	return err == nil && strings.HasPrefix(u, c.assets) && parsed.Host != "" && !strings.Contains(parsed.Path, "..")
}

// lang is a two-letter code, or "" for none: fanart.tv says "00" for an image
// with no words.
func lang(s string) string {
	if s == "00" {
		return ""
	}
	return strings.ToLower(s)
}
