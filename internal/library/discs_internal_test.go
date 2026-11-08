package library

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

// The names of the disc tests: an artist, two albums and two discs.
const (
	discArtist = "Low Tide Choir"
	glasshouse = "Glasshouse"
	plain      = "Plain"
	discOne    = "The Electric Set"
	discTwo    = "The Room Recordings"
)

// discTrack is one file of a disc test: where it goes and how it is tagged.
type discTrack struct {
	rel, album    string
	disc, trackNo int
	name          string // The disc's name, under DISCSUBTITLE; empty writes none.
}

// writeDiscs copies a FLAC to each track's place and tags it with audiometa.
func (l *testLib) writeDiscs(tracks ...discTrack) {
	l.t.Helper()
	root := l.roots[0]
	for _, tr := range tracks {
		l.copyTo(root, tr.rel, sampleFLAC)
		err := audiometa.Write(filepath.Join(root, tr.rel), func(f *audiometa.File) error {
			f.Tags.Album, f.Tags.AlbumArtist, f.Tags.Artists = tr.album, discArtist, []string{discArtist}
			f.Tags.Title = filepath.Base(tr.rel)
			f.Tags.DiscNumber, f.Tags.TrackNumber = tr.disc, tr.trackNo
			if tr.name != "" {
				f.Tags.Set(keyDiscSubtitle, tr.name)
			}
			return nil
		})
		if err != nil {
			l.t.Fatalf("Write %s: %v", tr.rel, err)
		}
	}
}

// albumNamed returns the album of that title.
func (l *testLib) albumNamed(title string) store.AlbumDetail {
	l.t.Helper()
	page, err := l.st.Albums(l.t.Context(), store.SortAdded, 10, "")
	if err != nil {
		l.t.Fatal(err)
	}
	i := slices.IndexFunc(page.Albums, func(a store.AlbumSummary) bool { return a.Title == title })
	if i < 0 {
		l.t.Fatalf("no album %q in %+v", title, page.Albums)
	}
	d, err := l.st.Album(l.t.Context(), page.Albums[i].ID)
	if err != nil {
		l.t.Fatal(err)
	}
	return d
}

func TestScanNamesEachDiscFromItsFirstNamedTrack(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	l.writeDiscs(
		discTrack{rel: "Glasshouse/1-01.flac", album: glasshouse, disc: 1, trackNo: 1},
		discTrack{rel: "Glasshouse/1-02.flac", album: glasshouse, disc: 1, trackNo: 2, name: discOne},
		discTrack{rel: "Glasshouse/1-03.flac", album: glasshouse, disc: 1, trackNo: 3, name: "Disagrees"},
		discTrack{rel: "Glasshouse/2-01.flac", album: glasshouse, disc: 2, trackNo: 1, name: " " + discTwo + " "},
		discTrack{rel: "Plain/01.flac", album: plain, trackNo: 1},
		discTrack{rel: "Plain/02.flac", album: plain, trackNo: 2},
	)
	l.scan(Quick)
	two := l.albumNamed(glasshouse)
	if len(two.Discs) != 2 {
		t.Fatalf("discs = %+v, want 2", two.Discs)
	}
	each := two.Tracks[0].Duration
	want := []store.Disc{
		{No: 1, Name: discOne, Songs: 3, Duration: 3 * each},
		{No: 2, Name: discTwo, Songs: 1, Duration: each},
	}
	if !slices.Equal(two.Discs, want) {
		t.Errorf("discs = %+v, want %+v", two.Discs, want)
	}
	// A track that disagrees keeps its own name, for the editor.
	names := make([]string, len(two.Tracks))
	for i := range two.Tracks {
		names[i] = two.Tracks[i].DiscSubtitle
	}
	if wantNames := []string{"", discOne, "Disagrees", discTwo}; !slices.Equal(names, wantNames) {
		t.Errorf("track names = %q, want %q", names, wantNames)
	}
	// One disc, with no number and no name, is one entry still.
	one := l.albumNamed(plain)
	if wantOne := []store.Disc{{No: 1, Songs: 2, Duration: 2 * one.Tracks[0].Duration}}; !slices.Equal(one.Discs, wantOne) {
		t.Errorf("one disc = %+v, want %+v", one.Discs, wantOne)
	}
	if each <= 0 || each > time.Hour {
		t.Errorf("track duration = %v", each)
	}
}
