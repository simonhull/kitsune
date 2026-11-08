// Package prefs decides which preferences a person may keep and what each may
// hold. The store keeps the values as text and knows none of this; server
// settings never live here.
package prefs

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
)

// MaxValue is the most bytes a value may take, as JSON.
const MaxValue = 4 << 10

var (
	// ErrUnknownKey means no preference has that key.
	ErrUnknownKey = errors.New("unknown preference")
	// ErrBadValue means a value isn't what its key holds.
	ErrBadValue = errors.New("bad preference value")
)

// Key is a preference a person may keep, with the rule its value follows.
type Key struct {
	name  string
	check func(value []byte) ([]byte, error)
}

// Name is the key as the API spells it.
func (k Key) Name() string { return k.name }

// Check returns value as it is kept, or an error wrapping [ErrBadValue] for one
// the key doesn't take: one that isn't JSON, breaks the key's rule, or is over
// [MaxValue]. A value read back is checked again, since a rule can tighten.
func (k Key) Check(value []byte) ([]byte, error) {
	if len(value) > MaxValue {
		return nil, fmt.Errorf("%s: over %d bytes: %w", k.name, MaxValue, ErrBadValue)
	}
	out, err := k.check(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %w", k.name, ErrBadValue, err)
	}
	return out, nil
}

// LibraryColumns is the key of the Library list's song columns.
const LibraryColumns = "library.columns"

// keys are every preference there is, by name.
//
//nolint:gochecknoglobals // A read-only table of the allowed keys.
var keys = map[string]Key{
	LibraryColumns: {name: LibraryColumns, check: checkColumns},
}

// Lookup returns the key named name, or an error wrapping [ErrUnknownKey].
func Lookup(name string) (Key, error) {
	k, ok := keys[name]
	if !ok {
		return Key{}, fmt.Errorf("%q: %w", name, ErrUnknownKey)
	}
	return k, nil
}

// Column is a song column a person may choose. Checkbox, number, title, time
// and the row's menu are always there, so they are none of these.
type Column string

// The columns a person may choose, in no order of theirs.
const (
	ColumnArtist      Column = "artist"
	ColumnAlbumArtist Column = "albumArtist"
	ColumnAlbum       Column = "album"
	ColumnGenre       Column = "genre"
	ColumnYear        Column = "year"
	ColumnRating      Column = "rating"
	ColumnHeart       Column = "heart"
	ColumnPlays       Column = "plays"
	ColumnLastPlayed  Column = "lastPlayed"
	ColumnAdded       Column = "added"
	ColumnComposer    Column = "composer"
	ColumnFormat      Column = "format"
	ColumnBitrate     Column = "bitrate"
)

// Columns lists every column a person may choose.
func Columns() []Column {
	return []Column{
		ColumnArtist, ColumnAlbumArtist, ColumnAlbum, ColumnGenre, ColumnYear, ColumnRating, ColumnHeart,
		ColumnPlays, ColumnLastPlayed, ColumnAdded, ColumnComposer, ColumnFormat, ColumnBitrate,
	}
}

// ColumnsValue is the value of [LibraryColumns]: the columns shown, in their
// order, and whether Artist is left empty where it is the album's.
type ColumnsValue struct {
	Order               []Column `json:"order"`
	ArtistWhenDifferent bool     `json:"artistWhenDifferent"`
}

var (
	errMissing   = errors.New("order and artistWhenDifferent are both required")
	errColumn    = errors.New("no such column")
	errDuplicate = errors.New("a column is named twice")
)

// checkColumns accepts a [ColumnsValue] that names each column once at most.
func checkColumns(value []byte) ([]byte, error) {
	var in struct {
		Order               *[]Column `json:"order"`
		ArtistWhenDifferent *bool     `json:"artistWhenDifferent"`
	}
	if err := json.Unmarshal(value, &in, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if in.Order == nil || in.ArtistWhenDifferent == nil {
		return nil, errMissing
	}
	out := ColumnsValue{Order: *in.Order, ArtistWhenDifferent: *in.ArtistWhenDifferent}
	known := Columns()
	for i, c := range out.Order {
		if !slices.Contains(known, c) {
			return nil, fmt.Errorf("%q: %w", c, errColumn)
		}
		if slices.Contains(out.Order[:i], c) {
			return nil, fmt.Errorf("%q: %w", c, errDuplicate)
		}
	}
	if out.Order == nil {
		out.Order = []Column{}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return b, nil
}
