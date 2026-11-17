package library

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/loudness"
	"github.com/simonhull/kitsune/internal/store"
)

// What a ReplayGain run reads and writes of one file. It writes through
// audiometa's ReplayGain scheme, never by raw key, and reads the tags back by
// their keys to know what the file holds: what it held before (its prior, which
// an undo puts back as spelled), and whether a write landed.

// The ReplayGain tags a file may hold, in audiometa's scheme.
const (
	keyTrackGain = "REPLAYGAIN_TRACK_GAIN"
	keyTrackPeak = "REPLAYGAIN_TRACK_PEAK"
	keyAlbumGain = "REPLAYGAIN_ALBUM_GAIN"
	keyAlbumPeak = "REPLAYGAIN_ALBUM_PEAK"
	keyR128Track = "R128_TRACK_GAIN"
	keyR128Album = "R128_ALBUM_GAIN"
)

// gainKeys returns every ReplayGain tag audiometa's RemoveReplayGain removes.
func gainKeys() []string {
	return []string{keyTrackGain, keyTrackPeak, keyAlbumGain, keyAlbumPeak, keyR128Track, keyR128Album}
}

// The precision the tags hold: a gain to 0.01 dB, a peak to six decimals.
const (
	gainStep = 0.01
	peakStep = 0.000001
)

// errGainChanged means a file's ReplayGain tags weren't what a run expected
// when it came to write it: another tool tagged it, or it was retagged since.
var errGainChanged = errors.New("the file's ReplayGain tags changed")

// gainValuesOf is what a run writes for a song Kitsune plays at m: the gains
// to ReplayGain 2's -18 LUFS and the true peaks, rounded to what a tag holds,
// so the journal says exactly what the file reads after.
func gainValuesOf(m *store.GainMeasure) store.GainValues {
	v := store.GainValues{TrackGain: roundTo(loudness.Gain(m.Track), gainStep), TrackPeak: roundTo(m.TrackPeak, peakStep)}
	if m.HasAlbum {
		v.AlbumGain, v.AlbumPeak, v.HasAlbum = roundTo(loudness.Gain(m.Album), gainStep), roundTo(m.AlbumPeak, peakStep), true
	}
	return v
}

// roundTo rounds v to the nearest step, as the tags' text does, without a
// negative zero.
func roundTo(v, step float64) float64 {
	r := math.Round(v/step) * step
	if r == 0 {
		return 0
	}
	return r
}

// gainTags are the ReplayGain tags a file holds: each key as spelled, with
// its values.
type gainTags map[string][]string

// gainTagsOf reads f's ReplayGain tags.
func gainTagsOf(f *audiometa.File) gainTags {
	out := gainTags{}
	for _, key := range gainKeys() {
		if values := f.Tags.Get(key); len(values) > 0 {
			out[f.Tags.Name(key)] = values
		}
	}
	return out
}

// encode returns t as the journal keeps it.
func (t gainTags) encode() (string, error) {
	b, err := json.Marshal(t, json.Deterministic(true))
	if err != nil {
		return "", fmt.Errorf("encode the tags before: %w", err)
	}
	return string(b), nil
}

// decodeGainTags reads tags the journal kept.
func decodeGainTags(text string) (gainTags, error) {
	out := gainTags{}
	if text == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, fmt.Errorf("decode the tags before: %w", err)
	}
	return out, nil
}

// get returns the first value of key, in any spelling, and whether t has it.
func (t gainTags) get(key string) (string, bool) {
	for name, values := range t {
		if strings.EqualFold(name, key) && len(values) > 0 {
			return values[0], true
		}
	}
	return "", false
}

// same reports whether t and u hold the same tags, spelling and values alike.
func (t gainTags) same(u gainTags) bool {
	if len(t) != len(u) {
		return false
	}
	for name, values := range t {
		other, ok := u[name]
		if !ok || strings.Join(values, "\x00") != strings.Join(other, "\x00") {
			return false
		}
	}
	return true
}

// tagged reports whether t holds a gain a player would apply: a track or album
// gain other than zero. A peak alone, or a gain of 0.00 dB, is what the store
// reads as none.
func (t gainTags) tagged() bool {
	for _, key := range []string{keyTrackGain, keyAlbumGain, keyR128Track, keyR128Album} {
		if text, ok := t.get(key); ok {
			if g, err := parseGain(text); err != nil || g != 0 {
				return true
			}
		}
	}
	return false
}

// reads reports whether t reads v: the track gain and peak, and the album's
// when v has them, to the tags' precision.
func (t gainTags) reads(v *store.GainValues) bool {
	type want struct {
		key   string
		value float64
		parse func(string) (float64, error)
		step  float64
	}
	wants := []want{{keyTrackGain, v.TrackGain, parseGain, gainStep}, {keyTrackPeak, v.TrackPeak, parsePeak, peakStep}}
	if v.HasAlbum {
		wants = append(wants, want{keyAlbumGain, v.AlbumGain, parseGain, gainStep}, want{keyAlbumPeak, v.AlbumPeak, parsePeak, peakStep})
	}
	for _, w := range wants {
		text, ok := t.get(w.key)
		if !ok {
			return false
		}
		got, err := w.parse(text)
		if err != nil || math.Abs(got-w.value) > w.step/2 {
			return false
		}
	}
	return true
}

// parseGain reads a gain as taggers write it: "-6.50 dB", any case, with or
// without the unit.
func parseGain(text string) (float64, error) {
	s := strings.TrimSpace(text)
	if len(s) > 2 && strings.EqualFold(s[len(s)-2:], "db") {
		s = strings.TrimSpace(s[:len(s)-2])
	}
	g, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("gain %q: %w", text, err)
	}
	return g, nil
}

// parsePeak reads a linear peak.
func parsePeak(text string) (float64, error) {
	p, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return 0, fmt.Errorf("peak %q: %w", text, err)
	}
	return p, nil
}

// writeGain is the edit that writes v into a file that holds prior, and only
// such a file: a fresh read that holds anything else returns [errGainChanged].
func writeGain(prior gainTags, v *store.GainValues) func(f *audiometa.File) error {
	return func(f *audiometa.File) error {
		if !gainTagsOf(f).same(prior) {
			return errGainChanged
		}
		if err := f.SetTrackReplayGain(v.TrackGain, v.TrackPeak); err != nil {
			return fmt.Errorf("track gain: %w", err)
		}
		if v.HasAlbum {
			if err := f.SetAlbumReplayGain(v.AlbumGain, v.AlbumPeak); err != nil {
				return fmt.Errorf("album gain: %w", err)
			}
		}
		return nil
	}
}

// unwriteGain is the edit that takes out what a run wrote, v, and puts back
// what the file held before, prior, as spelled. A fresh read that doesn't read
// v returns [errGainChanged].
func unwriteGain(prior gainTags, v *store.GainValues) func(f *audiometa.File) error {
	return func(f *audiometa.File) error {
		if !gainTagsOf(f).reads(v) {
			return errGainChanged
		}
		f.RemoveReplayGain()
		for name, values := range prior {
			f.Tags.SetNamed(name, values...)
		}
		return nil
	}
}
