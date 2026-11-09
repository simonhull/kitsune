package server

import (
	"math"
	"testing"

	"github.com/simonhull/kitsune/api"
)

// sineFLAC is the pcm package's 1 kHz sine at -20 dBFS: -20 LUFS, so a gain
// of +2 dB to the ReplayGain reference, and a true peak of 0.1.
const sineFLAC = "../../pcm/testdata/sine_24.flac"

const (
	// trackGainKey is the tag that gives a file its track gain.
	trackGainKey = "REPLAYGAIN_TRACK_GAIN"
	// taggedGain is a track gain as a file tags it.
	taggedGain = "-6.5 dB"
)

// measuredSines is two sines on one album, measured.
func measuredSines(t *testing.T) *editFixture {
	t.Helper()
	fx := newEditFixture(t, &testServerOptions{}, map[string]string{fileOne: sineFLAC, fileTwo: sineFLAC})
	if rg := fx.album.Tracks[0].ReplayGain; rg != nil {
		t.Fatalf("replayGain before any measure = %+v, want none", rg)
	}
	if _, err := fx.Scanner.MeasureLoudness(t.Context()); err != nil {
		t.Fatal(err)
	}
	fx.reload(t)
	return fx
}

func TestReplayGainMeasured(t *testing.T) {
	t.Parallel()
	fx := measuredSines(t)
	rg := fx.album.Tracks[0].ReplayGain
	if rg == nil || rg.Source != api.GainMeasured {
		t.Fatalf("replayGain = %+v, want a measured one", rg)
	}
	for what, got := range map[string]float64{"track gain": rg.TrackGain, "album gain": rg.AlbumGain} {
		if math.Abs(got-2) > 0.1 {
			t.Errorf("%s = %.2f, want +2", what, got)
		}
	}
	if math.Abs(rg.TrackPeak-0.1) > 0.001 || rg.AlbumPeak != rg.TrackPeak {
		t.Errorf("peaks = %v and %v, want 0.1", rg.TrackPeak, rg.AlbumPeak)
	}
	if got := fx.getLibrary(t, fx.token).Loudness; got != (api.LoudnessStatus{Ungained: 2, Measured: 2}) {
		t.Errorf("loudness = %+v, want both songs measured", got)
	}
}

// A gain tagged in the file wins over Kitsune's own.
func TestReplayGainTagsWin(t *testing.T) {
	t.Parallel()
	fx := measuredSines(t)
	fx.retag(t, map[string][]string{trackGainKey: {taggedGain}})
	rg := fx.album.Tracks[0].ReplayGain
	if rg == nil || rg.Source != api.GainFromTags || rg.TrackGain != -6.5 || rg.AlbumGain != 0 {
		t.Errorf("replayGain = %+v, want the tags' -6.5", rg)
	}
}

// gainsOf reads each track's gain and its source from the index, whole or of
// some albums, by track ID.
func (fx *editFixture) gainsOf(t *testing.T, query string) map[string]api.TrackFields {
	t.Helper()
	var index api.LibraryIndex
	fx.getJSON(t, fx.token, indexPath+"?fields=gain"+query, &index)
	out := map[string]api.TrackFields{}
	for k := range index.Artists {
		artist := &index.Artists[k]
		for j := range artist.Albums {
			tracks := artist.Albums[j].Tracks
			for i := range tracks {
				out[tracks[i].ID] = fieldsOf(&tracks[i])
			}
		}
	}
	return out
}

// wantGains checks that every track of the fixture has a gain from source
// near want.
func (fx *editFixture) wantGains(t *testing.T, got map[string]api.TrackFields, source string, want float64) {
	t.Helper()
	if len(got) != len(fx.album.Tracks) {
		t.Fatalf("index has %d tracks, want %d", len(got), len(fx.album.Tracks))
	}
	for id := range got {
		f := got[id]
		if f.TrackGain == nil || f.TrackGainSource != source || math.Abs(*f.TrackGain-want) > 0.1 {
			t.Errorf("track %s: gain %v from %q, want %+.1f from %q", id, f.TrackGain, f.TrackGainSource, want, source)
		}
	}
}

// The index's gain is the one album pages play at: a measure where no gain is
// tagged, and it is there as soon as the measure is, though the index's
// library rows were kept before it.
func TestLibraryIndexSendsMeasuredGain(t *testing.T) {
	t.Parallel()
	fx := newEditFixture(t, &testServerOptions{}, map[string]string{fileOne: sineFLAC, fileTwo: sineFLAC})
	for id, f := range fx.gainsOf(t, "") {
		if f.TrackGain != nil || f.TrackGainSource != "" {
			t.Errorf("track %s before any measure: gain %v from %q, want none", id, f.TrackGain, f.TrackGainSource)
		}
	}
	if _, err := fx.Scanner.MeasureLoudness(t.Context()); err != nil {
		t.Fatal(err)
	}
	fx.wantGains(t, fx.gainsOf(t, ""), api.GainMeasured, 2)
	fx.wantGains(t, fx.gainsOf(t, "&albums="+fx.album.ID), api.GainMeasured, 2)

	fx.retag(t, map[string][]string{trackGainKey: {taggedGain}})
	fx.wantGains(t, fx.gainsOf(t, ""), api.GainFromTags, -6.5)
	fx.wantGains(t, fx.gainsOf(t, "&albums="+fx.album.ID), api.GainFromTags, -6.5)
}
