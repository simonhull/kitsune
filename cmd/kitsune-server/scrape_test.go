package main

import (
	"errors"
	"testing"

	"github.com/simonhull/kitsune/internal/settings"
)

func TestScrapeStandInsRefuseBadURLs(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"fanart.test", "ftp://fanart.test", "http://"} {
		if _, err := standInRoot(bad, errBadFanartURL); !errors.Is(err, errBadFanartURL) {
			t.Errorf("%q: err = %v, want errBadFanartURL", bad, err)
		}
	}
	if root, err := standInRoot("http://127.0.0.1:9/", errBadWikimediaURL); err != nil || root != "http://127.0.0.1:9" {
		t.Errorf("a good root: %q, %v", root, err)
	}
	if root, err := standInRoot("", errBadWikimediaURL); err != nil || root != "" {
		t.Errorf("none: %q, %v", root, err)
	}
}

func TestScrapeConfigCarriesTheKeys(t *testing.T) {
	t.Parallel()
	snap := &settings.Snapshot{
		MetadataRemote: true, MetadataFanart: true, MetadataLanguage: "de", FanartAPIKey: "p", FanartClientKey: "c",
	}
	c := scrapeConfig(snap)
	if !c.Remote || !c.Fanart || c.Wikipedia || c.Language != "de" || c.Keys.API != "p" || c.Keys.Client != "c" {
		t.Errorf("config = %+v", c)
	}
}
