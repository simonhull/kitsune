package settings_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/organize"
	"github.com/simonhull/kitsune/internal/settings"
)

// The organizer's keys start at its defaults.
func TestOrganizeSettingsDefaults(t *testing.T) {
	t.Parallel()
	snap := open(t, openStore(t)).Snapshot()
	if snap.OrganizeTemplate != organize.DefaultTemplate || snap.OrganizeCompilations != organize.DefaultCompilations ||
		!snap.OrganizeCompanions || snap.OrganizeOnEdit || snap.OrganizeRewritePlaylists || len(snap.OrganizeRoots) != 0 {
		t.Errorf("defaults = %+v", snap.Values)
	}
}

// They take a template that parses and folders with their own, and clearing
// the folders leaves none.
func TestOrganizeSettingsChange(t *testing.T) {
	t.Parallel()
	live := open(t, openStore(t))
	next, err := live.Change(t.Context(), map[string][]byte{
		string(settings.OrganizeTemplate): []byte(`"<Artist>/<Album>/<Track> <Title>"`),
		string(settings.OrganizeRoots):    []byte(`{"/books": {"template": "<Album>/<Title>", "compilations": "VA/<Album>/<Title>"}}`),
		string(settings.OrganizeOnEdit):   []byte(`true`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.OrganizeTemplate != "<Artist>/<Album>/<Track> <Title>" || !next.OrganizeOnEdit ||
		next.OrganizeRoots["/books"].Template != "<Album>/<Title>" {
		t.Errorf("after the change = %+v", next.Values)
	}
	if _, err = live.Change(t.Context(), map[string][]byte{string(settings.OrganizeRoots): []byte(`null`)}); err != nil {
		t.Fatal(err)
	}
	if got := live.Snapshot(); len(got.OrganizeRoots) != 0 || got.Source(settings.OrganizeRoots) != settings.SourceDefault {
		t.Errorf("cleared roots = %+v from %s, want none, the default", got.OrganizeRoots, got.Source(settings.OrganizeRoots))
	}
}

// A template that doesn't parse, and a folder that isn't absolute, are refused
// without the value quoted.
func TestOrganizeSettingsRefuseBadValues(t *testing.T) {
	t.Parallel()
	live := open(t, openStore(t))
	for key, raw := range map[settings.Key]string{
		settings.OrganizeTemplate: `"<Album>/<Tilte>"`,
		settings.OrganizeRoots:    `{"books": {"template": "<Title>", "compilations": "<Title>"}}`,
	} {
		_, err := live.Change(t.Context(), map[string][]byte{string(key): []byte(raw)})
		if !errors.Is(err, settings.ErrBadValue) || strings.Contains(err.Error(), "Tilte") {
			t.Errorf("%s = %v, want a bad value that doesn't quote it", key, err)
		}
	}
}
