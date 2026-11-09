package server

import (
	"math"
	"testing"

	"github.com/simonhull/kitsune/api"
)

// sineFLAC is the pcm package's 1 kHz sine at -20 dBFS: -20 LUFS, so a gain
// of +2 dB to the ReplayGain reference, and a true peak of 0.1.
const sineFLAC = "../../pcm/testdata/sine_24.flac"

// trackGainKey is the tag that gives a file its track gain.
const trackGainKey = "REPLAYGAIN_TRACK_GAIN"

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
	fx.retag(t, map[string][]string{trackGainKey: {"-6.5 dB"}})
	rg := fx.album.Tracks[0].ReplayGain
	if rg == nil || rg.Source != api.GainFromTags || rg.TrackGain != -6.5 || rg.AlbumGain != 0 {
		t.Errorf("replayGain = %+v, want the tags' -6.5", rg)
	}
}
