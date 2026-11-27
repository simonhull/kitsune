package main

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"

	"github.com/simonhull/kitsune/internal/lastfm"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/scrape"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv"
	"github.com/simonhull/kitsune/internal/scrape/wikimedia"
	"github.com/simonhull/kitsune/internal/server"
	"github.com/simonhull/kitsune/internal/settings"
)

// fanartURLEnv and wikimediaURLEnv name the roots of stand-ins to talk to in
// place of fanart.tv (its API at /v3.2/, its images at /fanart/) and of the
// Wikimedia sites (/wikidata/, /wikipedia/, /commons/, /upload/). Only tests
// and manual checks set them; production never needs them.
const (
	fanartURLEnv    = "KITSUNE_FANARTTV_URL"
	wikimediaURLEnv = "KITSUNE_WIKIMEDIA_URL"
)

var (
	errBadFanartURL    = errors.New(fanartURLEnv + " must be an absolute http or https URL")
	errBadWikimediaURL = errors.New(wikimediaURLEnv + " must be an absolute http or https URL")
)

// standInRoot is u without its trailing slash, when it is an absolute http
// URL, or "" and bad when it isn't. An empty u is no stand-in.
func standInRoot(u string, bad error) (string, error) {
	if u == "" {
		return "", nil
	}
	parsed, err := url.Parse(u)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", bad
	}
	return strings.TrimSuffix(u, "/"), nil
}

// newScraper returns the artist pass over a's store, covers and scanner,
// asking the process's one MusicBrainz client, and the stand-ins l names.
// It is off until applyMetadata gives it the settings.
func newScraper(l *launch, a *app, mb *matching.MusicBrainz, log *slog.Logger) (*scrape.Scraper, error) {
	fanartRoot, err := standInRoot(l.fanartURL, errBadFanartURL)
	if err != nil {
		return nil, err
	}
	wikimediaRoot, err := standInRoot(l.wikimediaURL, errBadWikimediaURL)
	if err != nil {
		return nil, err
	}
	fo := fanarttv.Options{BaseURL: fanartRoot, AssetsURL: fanartRoot, Version: agentVersion(), Logger: log}
	if fanartRoot != "" {
		log.Warn("talking to a stand-in fanart.tv, not the real one", "env", fanartURLEnv, "url", fanartRoot)
	}
	wo := wikimedia.Options{BaseURL: wikimediaRoot, Version: agentVersion(), Logger: log}
	if wikimediaRoot != "" {
		log.Warn("talking to stand-in Wikimedia sites, not the real ones", "env", wikimediaURLEnv, "url", wikimediaRoot)
	}
	events := server.NewLibraryEvents(a.events)
	return scrape.New(scrape.Options{
		Store: a.store, Artwork: a.covers, MusicBrainz: mb, Fanart: fanarttv.New(fo), Wikimedia: wikimedia.New(wo),
		Library: a.scanner, Notify: events.ArtistsChanged, NotifyAlbums: events.AlbumsChanged, Logger: log,
	}), nil
}

// scrapeConfig is the artist pass's part of snap, with Last.fm asked through
// lastfmURL's stand-in when that is set.
func scrapeConfig(snap *settings.Snapshot, lastfmURL string) scrape.Config {
	c := scrape.Config{
		Remote: snap.MetadataRemote, Fanart: snap.MetadataFanart, Wikipedia: snap.MetadataWikipedia,
		LastfmBios: snap.MetadataLastfmBios, Archive: snap.MetadataArchive,
		Language: snap.MetadataLanguage, Keys: fanarttv.Keys{API: snap.FanartAPIKey, Client: snap.FanartClientKey},
	}
	if snap.LastfmAPIKey != "" {
		// Bios need the API key alone: the call is unsigned. Its own client,
		// so the scrobbler's, which needs the secret too, is untouched.
		bios := lastfm.New(snap.LastfmAPIKey, "")
		if root, err := standInRoot(lastfmURL, errBadLastfmURL); err == nil && root != "" {
			bios.APIBase = root + "/2.0/"
		}
		c.Lastfm = bios
	}
	return c
}

// applyMetadata gives the artist pass the live settings. It logs what is on,
// never a key.
func (a *app) applyMetadata(ctx context.Context) {
	c := scrapeConfig(a.live.Snapshot(), a.lastfmURL)
	a.scraper.Configure(c)
	a.log.InfoContext(ctx, "artist lookups applied", "remote", c.Remote, "fanarttv", c.Fanart,
		"fanarttvKey", c.Keys.API != "", "wikipedia", c.Wikipedia, "lastfmBios", c.LastfmBios,
		"lastfmKey", c.Lastfm != nil, "coverartarchive", c.Archive, "language", c.Language)
}

// kickOnLibrary is the scanner's notifier, which also asks the artist pass to
// look again whenever the library changes: a scan, an edit or a match may have
// given an artist an MBID.
type kickOnLibrary struct {
	library.Notifier

	kick func() // Set before anything scans.
}

// LibraryChanged implements [library.Notifier].
func (k *kickOnLibrary) LibraryChanged(albums []string, all bool) {
	k.Notifier.LibraryChanged(albums, all)
	if k.kick != nil {
		k.kick()
	}
}
