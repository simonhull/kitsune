package scrape

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/simonhull/kitsune/internal/lastfm"
	"github.com/simonhull/kitsune/internal/remote"
	"github.com/simonhull/kitsune/internal/store"
)

// What Last.fm's bios are under, and where its license is: Last.fm says
// "Creative Commons By-SA License" and links version 3.0.
const (
	lastfmLicense    = "CC BY-SA"
	lastfmLicenseURL = "https://creativecommons.org/licenses/by-sa/3.0/"
)

// askLastfm asks Last.fm for the artist mbid's bio when it is still wanted
// after the answers before it: no artist.nfo's, no bio Wikipedia gave this
// time, and none Wikipedia gave before that it wasn't asked to replace. Not
// wanted, Last.fm isn't asked, and its state says found, as a source with
// nothing left to fill does.
func (s *Scraper) askLastfm(ctx context.Context, cfg *Config, mbid string, held *store.HeldArt, before []*found) *found {
	f := &found{source: store.ScrapeLastfm, images: map[store.ArtistImageKind][]Candidate{}}
	if !wantsLastfm(held, before) {
		f.asked, f.status, f.skipped = true, store.ScrapeFound, true
		return f
	}
	bio, err := s.lastfmBio(ctx, cfg, mbid)
	f.bio = bio
	s.settle(ctx, f, err)
	return f
}

// wantsLastfm reports whether a bio is still wanted from Last.fm.
func wantsLastfm(held *store.HeldArt, before []*found) bool {
	if !freeBio(held) {
		return false
	}
	wiki := answerOf(before, store.ScrapeWikimedia)
	if wiki != nil && wiki.asked && wiki.bio != nil {
		return false // Wikipedia gave one: it comes first.
	}
	keptWiki := held.Bio != nil && held.Bio.Source == store.BioFromWikipedia
	return !keptWiki || (wiki != nil && wiki.asked)
}

// lastfmBio is Last.fm's bio of the artist mbid in the language wanted, else
// English, or nil when it has none. An answer for another MBID is none:
// Last.fm has been known to send one.
func (s *Scraper) lastfmBio(ctx context.Context, cfg *Config, mbid string) (*store.Bio, error) {
	langs := []string{cfg.Language}
	if cfg.Language != "en" {
		langs = append(langs, "en")
	}
	for _, lang := range langs {
		if err := s.lastfm.Wait(ctx); err != nil {
			return nil, fmt.Errorf("last.fm gate: %w", err)
		}
		b, err := cfg.Lastfm.ArtistInfo(ctx, mbid, lang)
		if err != nil {
			return nil, lastfmError(err)
		}
		if !strings.EqualFold(b.MBID, mbid) {
			s.log.InfoContext(ctx, "artist pass: Last.fm answered for another artist: its bio is not used", "mbid", mbid)
			return nil, nil //nolint:nilnil // Another artist's bio is no bio.
		}
		if len(b.Paragraphs) > 0 {
			return &store.Bio{
				Source: store.BioFromLastfm, Lang: lang, Summary: b.Summary, Body: strings.Join(b.Paragraphs, "\n\n"),
				URL: b.ReadMore, License: lastfmLicense, LicenseURL: lastfmLicenseURL,
			}, nil
		}
	}
	return nil, nil //nolint:nilnil // No bio in any language wanted.
}

// lastfmError is err as the [remote] sentinels the pass reads, still wrapping
// Last.fm's own error, which never holds the key: it travels in the body.
func lastfmError(err error) error {
	var kind error
	switch {
	case errors.Is(err, lastfm.ErrNotFound):
		kind = remote.ErrNotFound
	case errors.Is(err, lastfm.ErrKeyRefused):
		kind = remote.ErrRefused
	case errors.Is(err, lastfm.ErrTemporary):
		kind = remote.ErrUnavailable
	default:
		kind = remote.ErrRejected
	}
	return fmt.Errorf("%w: %w", kind, err)
}
