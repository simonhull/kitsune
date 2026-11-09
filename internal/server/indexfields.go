package server

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/loudness"
	"github.com/simonhull/kitsune/internal/store"
)

// indexFields is what GET /library/index?fields= asks for beyond what the index
// always carries.
type indexFields uint16

const (
	fieldAlbumArtist indexFields = 1 << iota
	fieldGenre
	fieldYear
	fieldComposer
	fieldFormat
	fieldBitrate
	fieldAdded
	fieldPlays
	fieldLastPlayed
	fieldGain
	fieldShuffle
)

// indexFieldNames are the contract's names for the fields.
//
//nolint:gochecknoglobals // A read-only table of the contract's enum.
var indexFieldNames = map[string]indexFields{
	"albumArtist": fieldAlbumArtist,
	"genre":       fieldGenre,
	"year":        fieldYear,
	"composer":    fieldComposer,
	"format":      fieldFormat,
	"bitrate":     fieldBitrate,
	"added":       fieldAdded,
	"plays":       fieldPlays,
	"lastPlayed":  fieldLastPlayed,
	"gain":        fieldGain,
	"shuffle":     fieldShuffle,
}

var errUnknownField = errors.New("unknown field")

// parseIndexFields reads the `fields` parameter: names joined by commas, in any
// order, in one parameter or several. An empty name is no name.
func parseIndexFields(values []string) (indexFields, error) {
	var out indexFields
	for _, value := range values {
		for name := range strings.SplitSeq(value, ",") {
			if name == "" {
				continue
			}
			f, ok := indexFieldNames[name]
			if !ok {
				return 0, fmt.Errorf("%q: %w", name, errUnknownField)
			}
			out |= f
		}
	}
	return out, nil
}

func (f indexFields) has(field indexFields) bool { return f&field != 0 }

// options returns what the store must read for f.
func (f indexFields) options() store.IndexOptions {
	var keys []string
	if f.has(fieldGenre) {
		keys = append(keys, library.GenreKeys()...)
	}
	if f.has(fieldComposer) {
		keys = append(keys, library.ComposerKeys()...)
	}
	return store.IndexOptions{
		Details:      f&(fieldYear|fieldFormat|fieldBitrate|fieldAdded|fieldGain) != 0,
		AlbumArtists: f.has(fieldAlbumArtist),
		Plays:        f.has(fieldPlays) || f.has(fieldLastPlayed),
		Shuffle:      f.has(fieldShuffle),
		Measured:     f.has(fieldGain),
		TagKeys:      keys,
	}
}

// bitsPerKbps turns a stored bitrate into the contract's.
const bitsPerKbps = 1000

// fieldWriter sets the fields asked for on each track of a response.
type fieldWriter struct {
	fields                  indexFields
	genreKeys, composerKeys []string
	genres                  *library.Genres
}

// writer returns what sets f on each track. Genres go through genres, so a
// track's read as its album's do.
func (f indexFields) writer(genres *library.Genres) fieldWriter {
	return fieldWriter{fields: f, genreKeys: library.GenreKeys(), composerKeys: library.ComposerKeys(), genres: genres}
}

// add sets on out what was asked for of t.
func (w *fieldWriter) add(out *api.LibraryTrack, track *store.IndexTrack) {
	t := track.Extra
	if t == nil {
		return
	}
	out.TrackFields = &api.TrackFields{}
	w.addNames(out.TrackFields, t)
	w.addFacts(out.TrackFields, t)
	w.addPlays(out.TrackFields, t)
}

// addNames sets the fields that name people and genres.
func (w *fieldWriter) addNames(out *api.TrackFields, t *store.IndexExtra) {
	f := w.fields
	if f.has(fieldAlbumArtist) {
		out.AlbumArtists = slices.Clone(t.AlbumArtists)
	}
	if f.has(fieldGenre) {
		out.DisplayGenres = w.genres.Display(tagValues(t.Tags, w.genreKeys))
	}
	if f.has(fieldComposer) {
		out.Composers = tagValues(t.Tags, w.composerKeys)
	}
}

// addFacts sets the fields that are about the file.
func (w *fieldWriter) addFacts(out *api.TrackFields, t *store.IndexExtra) {
	f := w.fields
	if f.has(fieldYear) {
		out.Year = t.Year
	}
	if f.has(fieldFormat) {
		out.Format, out.SampleRate, out.BitDepth = t.Format, t.SampleRate, t.BitDepth
	}
	if f.has(fieldFormat) || f.has(fieldBitrate) {
		out.Bitrate = (t.Bitrate + bitsPerKbps/2) / bitsPerKbps
	}
	if f.has(fieldAdded) {
		out.AddedAt = t.AddedAt
	}
	if f.has(fieldGain) {
		setGain(out, t)
	}
}

// setGain sets the gain a track plays at, as album pages say it: its tags' when
// it has a gain tagged, else Kitsune's measure of its file, else none.
func setGain(out *api.TrackFields, t *store.IndexExtra) {
	switch {
	case t.TrackGain != nil:
		out.TrackGain, out.TrackGainSource = new(*t.TrackGain), api.GainFromTags
	case t.Measured != nil:
		out.TrackGain, out.TrackGainSource = new(loudness.Gain(*t.Measured)), api.GainMeasured
	}
}

// addPlays sets the fields that are the caller's own: plays and shuffle rules.
func (w *fieldWriter) addPlays(out *api.TrackFields, t *store.IndexExtra) {
	f := w.fields
	if f.has(fieldPlays) {
		out.Plays = t.Plays
	}
	if f.has(fieldLastPlayed) {
		out.LastPlayedAt = t.LastPlayed
	}
	if m := t.Shuffle; f.has(fieldShuffle) && m != nil {
		out.Shuffle = &api.ShuffleMark{Run: m.Run, Position: m.Position, Length: m.Length, Excluded: m.Excluded}
	}
}

// tagValues returns the trimmed, non-empty values of tags under keys, in order.
// Nil means none.
func tagValues(tags []store.Tag, keys []string) []string {
	var out []string
	for _, tag := range tags {
		if v := strings.TrimSpace(tag.Value); v != "" && slices.Contains(keys, tag.Key) {
			out = append(out, v)
		}
	}
	return out
}
