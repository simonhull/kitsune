package main

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/lastfm"
)

func TestLastfmClient(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.DiscardHandler)
	set := config.LastFM{APIKey: "k", SharedSecret: "s"}

	if c, err := lastfmClient(config.LastFM{}, "http://fake.test", log); c != nil || err != nil {
		t.Errorf("not set up = %v, %v, want no client", c, err)
	}
	c, err := lastfmClient(set, "", log)
	if err != nil || c.APIBase != lastfm.DefaultAPIBase || c.AuthBase != lastfm.DefaultAuthBase {
		t.Errorf("real = %+v, %v, want Last.fm's own endpoints", c, err)
	}
	c, err = lastfmClient(set, "http://127.0.0.1:9/", log)
	if err != nil || c.APIBase != "http://127.0.0.1:9/2.0/" || c.AuthBase != "http://127.0.0.1:9/api/auth/" {
		t.Errorf("fake = %+v, %v, want the fake's endpoints", c, err)
	}
}

func TestLastfmClientRefusesABadFake(t *testing.T) {
	t.Parallel()
	set := config.LastFM{APIKey: "k", SharedSecret: "s"}
	for _, bad := range []string{"127.0.0.1:9", "ftp://x", "/path"} {
		if _, err := lastfmClient(set, bad, slog.New(slog.DiscardHandler)); !errors.Is(err, errBadLastfmURL) {
			t.Errorf("%q: err = %v, want errBadLastfmURL", bad, err)
		}
	}
}

func TestMatcherRefusesABadStandIn(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"musicbrainz.test", "ftp://musicbrainz.test", "http://"} {
		if _, err := newMatcher(bad, nil, slog.New(slog.DiscardHandler)); !errors.Is(err, errBadMusicBrainzURL) {
			t.Errorf("%q: err = %v, want errBadMusicBrainzURL", bad, err)
		}
	}
}
