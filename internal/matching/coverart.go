package matching

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/simonhull/kitsune/internal/remote"
)

// ReleaseGroupFor is how long a release's group is kept: it never changes.
const ReleaseGroupFor = 7 * 24 * time.Hour

// Defaults of the Cover Art Archive's requests.
const (
	// defaultArtInterval spaces them: the archive states no limit, so Kitsune
	// keeps to MusicBrainz's.
	defaultArtInterval = time.Second
	// maxArt caps an answer: a 1200-pixel scan is a megabyte or two.
	maxArt = 16 << 20
)

// ErrBadArt means the Cover Art Archive answered with something Kitsune can't
// use: JSON that doesn't parse, or an image somewhere other than the archive.
// It is never fetched.
var ErrBadArt = errors.New("the Cover Art Archive's answer isn't usable")

// ReleaseGroup returns the MusicBrainz ID of the release id's group, or ""
// when MusicBrainz names none, looked up at the gate and kept a week. It never
// writes anything anywhere: the caller keeps it as its own.
//
// # Errors
//
// It returns an error wrapping [ErrNotFound] for an id that isn't an MBID or
// that MusicBrainz doesn't know, else what a lookup returns.
func (c *MusicBrainz) ReleaseGroup(ctx context.Context, id string) (string, error) {
	if !mbidPattern.MatchString(id) {
		return "", fmt.Errorf("release %q: %w", id, ErrNotFound)
	}
	key := "release/" + id + "?" + url.Values{"inc": {"release-groups"}, fmtParam: {fmtJSON}}.Encode()
	body, err := c.getFresh(ctx, key, ReleaseGroupFor)
	if err != nil {
		return "", fmt.Errorf("release %s group: %w", id, err)
	}
	var r struct {
		Group struct {
			ID string `json:"id"`
		} `json:"release-group"`
	}
	if err = json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("release %s group: %w: %w", id, ErrUnavailable, err)
	}
	if !mbidPattern.MatchString(r.Group.ID) {
		return "", nil
	}
	return r.Group.ID, nil
}

// ArtImage is one of a release's images at the Cover Art Archive.
type ArtImage struct {
	ID  string // The archive's own ID for the image.
	URL string // Its 1200-pixel rendition where there is one, checked.
}

// CoverArt is what the Cover Art Archive has of a release, as far as Kitsune
// reads it: its front and back covers, each nil when it has none.
type CoverArt struct {
	Front, Back *ArtImage
}

// CoverArt returns the Cover Art Archive's front and back covers of the
// release id, asked through the archive's own gate: [Background] work waits
// behind any person's.
//
// # Errors
//
// It returns an error wrapping [remote.ErrNotFound] when the archive has
// nothing for the release, [ErrBadArt] for an answer it can't use, and the
// other [remote] sentinels for a failure.
func (c *MusicBrainz) CoverArt(ctx context.Context, id string) (CoverArt, error) {
	if !mbidPattern.MatchString(id) {
		return CoverArt{}, fmt.Errorf("cover art of %q: %w", id, remote.ErrNotFound)
	}
	body, err := c.art.Get(ctx, "release/"+id, c.covers+"release/"+id, artJSON())
	if err != nil {
		return CoverArt{}, fmt.Errorf("cover art of %s: %w", id, err)
	}
	var raw struct {
		Images []struct {
			ID         looseID           `json:"id"`
			Image      string            `json:"image"`
			Front      bool              `json:"front"`
			Back       bool              `json:"back"`
			Thumbnails map[string]string `json:"thumbnails"`
		} `json:"images"`
	}
	if err = json.Unmarshal(body, &raw); err != nil {
		return CoverArt{}, fmt.Errorf("cover art of %s: %w: it doesn't parse", id, ErrBadArt)
	}
	var out CoverArt
	for _, img := range raw.Images {
		u := img.Thumbnails["1200"]
		for _, alt := range []string{img.Thumbnails["large"], img.Image} {
			if u == "" {
				u = alt
			}
		}
		u, ok := c.artURL(u)
		if !ok {
			return CoverArt{}, fmt.Errorf("cover art of %s: %w: an image is elsewhere", id, ErrBadArt)
		}
		a := &ArtImage{ID: string(img.ID), URL: u}
		if img.Front && out.Front == nil {
			out.Front = a
		}
		if img.Back && out.Back == nil {
			out.Back = a
		}
	}
	return out, nil
}

// DownloadArt returns the bytes of an image [MusicBrainz.CoverArt] returned. One
// that isn't the archive's is [ErrBadArt], never fetched.
//
// # Errors
//
// As [MusicBrainz.CoverArt]'s.
func (c *MusicBrainz) DownloadArt(ctx context.Context, u string) ([]byte, error) {
	checked, ok := c.artURL(u)
	if !ok {
		return nil, fmt.Errorf("cover art image: %w: it is elsewhere", ErrBadArt)
	}
	body, err := c.art.Get(ctx, checked, checked, nil)
	if err != nil {
		return nil, fmt.Errorf("cover art image: %w", err)
	}
	return body, nil
}

// artURL is u when it is the archive's (coverartarchive.org, archive.org, or
// the host of the root the client was given), over https for the real ones,
// and false otherwise.
func (c *MusicBrainz) artURL(u string) (string, bool) {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "https" && p.Scheme != "http") || p.Host == "" || strings.Contains(p.Path, "..") {
		return "", false
	}
	host := strings.ToLower(p.Hostname())
	if root, rerr := url.Parse(c.covers); rerr == nil && p.Host == root.Host {
		return u, true
	}
	if host == "coverartarchive.org" || host == "archive.org" || strings.HasSuffix(host, ".archive.org") {
		p.Scheme = "https" // The archive's own JSON says http.
		return p.String(), true
	}
	return "", false
}

// artJSON asks the archive for JSON.
func artJSON() http.Header { return http.Header{"Accept": {"application/json"}} }

// looseID reads a JSON number or string as text: the archive's image IDs are
// numbers too large for some readers, sent as either.
type looseID string

// UnmarshalJSON implements json.Unmarshaler.
func (s *looseID) UnmarshalJSON(b []byte) error {
	*s = looseID(bytes.Trim(b, `"`))
	return nil
}
