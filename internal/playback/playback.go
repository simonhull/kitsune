// Package playback says what a person's remembered playback holds: where they
// left off, as a queue of track ULIDs, the song they were on and how far in. It
// checks a record on the way in, and resolves one against the live library on
// the way out. The store keeps the queue as text and knows none of this.
package playback

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/simonhull/kitsune/internal/store"
)

// MaxSongs is the most songs a record holds, across its tracks and later.
const MaxSongs = 1000

// MaxPositionMs is the furthest into a song a record may be: a day.
const MaxPositionMs = 24 * 60 * 60 * 1000

// ErrInvalid means a record breaks one of its rules.
var ErrInvalid = errors.New("invalid playback")

// Kind is what kind of queue a record holds.
type Kind string

// The kinds there are.
const (
	// Album is one album's songs, in the order playing.
	Album Kind = "album"
	// Songs is songs picked from several albums.
	Songs Kind = "songs"
	// List is a list's songs: those taken in so far, then later.
	List Kind = "list"
	// Shuffle is the current song or run of a shuffle session.
	Shuffle Kind = "shuffle"
)

// Device is the coarse form of the browser that saved a record.
type Device string

// The forms there are.
const (
	Phone    Device = "phone"
	Tablet   Device = "tablet"
	Computer Device = "computer"
)

// ShuffleAt is where a shuffle session stood: its ULID, and the position right
// after the song or run the record holds.
type ShuffleAt struct {
	ID    string
	After int64
}

// Record is a person's remembered playback, as a client saved it.
type Record struct {
	Kind Kind
	// Tracks are the queue's track ULIDs in play order, and Later the songs a
	// list only names so far.
	Tracks, Later []string
	// Continues is the album ULID an album queue goes on from, down the
	// Library list.
	Continues string
	// Index is the current song's place in Tracks.
	Index      int
	PositionMs int64
	// Ordered says album gain applies.
	Ordered bool
	// Shuffle is the session a shuffle queue plays in, nil for the others.
	Shuffle  *ShuffleAt
	ClientID string
	Device   Device
}

// Check returns nil when rec keeps every rule a record has, and otherwise an
// error wrapping [ErrInvalid] saying which it breaks.
func Check(rec *Record) error {
	if err := check(rec); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return nil
}

var (
	errNoTracks   = errors.New("no tracks")
	errTooMany    = fmt.Errorf("more than %d songs", MaxSongs)
	errIndex      = errors.New("index past the tracks")
	errPosition   = errors.New("position outside a day")
	errKind       = errors.New("unknown kind")
	errDevice     = errors.New("unknown device")
	errClient     = errors.New("client ID must be 16 to 64 lowercase hex characters")
	errULID       = errors.New("not a ULID")
	errShuffle    = errors.New("shuffle goes with kind shuffle, and only with it")
	errLater      = errors.New("later goes only with kind list")
	errContinues  = errors.New("continues goes only with kind album")
	errShuffleNeg = errors.New("shuffle position is negative")
)

func check(rec *Record) error {
	for _, step := range []func(*Record) error{checkLabels, checkPlace, checkPairs, checkIDs} {
		if err := step(rec); err != nil {
			return err
		}
	}
	return nil
}

// checkLabels checks the kind, the device and the client ID.
func checkLabels(rec *Record) error {
	if !slices.Contains([]Kind{Album, Songs, List, Shuffle}, rec.Kind) {
		return errKind
	}
	if !slices.Contains([]Device{Phone, Tablet, Computer}, rec.Device) {
		return errDevice
	}
	if !isClientID(rec.ClientID) {
		return errClient
	}
	return nil
}

// checkPlace checks the queue's size and the place in it.
func checkPlace(rec *Record) error {
	switch {
	case len(rec.Tracks) == 0:
		return errNoTracks
	case len(rec.Tracks)+len(rec.Later) > MaxSongs:
		return errTooMany
	case rec.Index < 0 || rec.Index >= len(rec.Tracks):
		return errIndex
	case rec.PositionMs < 0 || rec.PositionMs > MaxPositionMs:
		return errPosition
	}
	return nil
}

// checkPairs checks that each field that goes with one kind comes only with it.
func checkPairs(rec *Record) error {
	switch {
	case (rec.Shuffle != nil) != (rec.Kind == Shuffle):
		return errShuffle
	case len(rec.Later) > 0 && rec.Kind != List:
		return errLater
	case rec.Continues != "" && rec.Kind != Album:
		return errContinues
	case rec.Shuffle != nil && rec.Shuffle.After < 0:
		return errShuffleNeg
	}
	return nil
}

// checkIDs checks that every ID has a ULID's shape.
func checkIDs(rec *Record) error {
	if rec.Shuffle != nil && !IsULID(rec.Shuffle.ID) {
		return fmt.Errorf("shuffle %q: %w", rec.Shuffle.ID, errULID)
	}
	if rec.Continues != "" && !IsULID(rec.Continues) {
		return fmt.Errorf("continues %q: %w", rec.Continues, errULID)
	}
	for _, id := range rec.IDs() {
		if !IsULID(id) {
			return fmt.Errorf("track %q: %w", id, errULID)
		}
	}
	return nil
}

const (
	ulidLen      = 26
	ulidAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ" // Crockford base32.
	ulidFirstMax = '7'                                // 26 characters hold 130 bits; a ULID is 128.
	clientIDMin  = 16
	clientIDMax  = 64
)

// IsULID reports whether id has a ULID's shape: 26 Crockford base32
// characters, upper case, at most 128 bits.
func IsULID(id string) bool {
	if len(id) != ulidLen || id[0] > ulidFirstMax {
		return false
	}
	for i := range len(id) {
		if strings.IndexByte(ulidAlphabet, id[i]) < 0 {
			return false
		}
	}
	return true
}

func isClientID(id string) bool {
	if len(id) < clientIDMin || len(id) > clientIDMax {
		return false
	}
	for i := range len(id) {
		c := id[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// queue is a record's queue as the store keeps it.
type queue struct {
	Tracks    []string `json:"tracks"`
	Later     []string `json:"later,omitzero"`
	Continues string   `json:"continues,omitzero"`
}

// Encode returns rec as the store keeps it. Its UpdatedAt is the store's to set.
//
// # Errors
//
// It fails only if the queue can't be encoded as JSON.
func Encode(rec *Record) (store.Playback, error) {
	q, err := json.Marshal(queue{Tracks: rec.Tracks, Later: rec.Later, Continues: rec.Continues})
	if err != nil {
		return store.Playback{}, fmt.Errorf("encode queue: %w", err)
	}
	out := store.Playback{
		Kind: string(rec.Kind), Queue: q, At: rec.Index, PositionMs: rec.PositionMs,
		Ordered: rec.Ordered, ClientID: rec.ClientID, Device: string(rec.Device),
	}
	if rec.Shuffle != nil {
		out.ShuffleID, out.ShuffleAfter = rec.Shuffle.ID, rec.Shuffle.After
	}
	return out, nil
}

// Decode returns the record p keeps, checked again, since a rule can tighten.
//
// # Errors
//
// It returns an error wrapping [ErrInvalid] for a record that breaks a rule,
// or one whose queue isn't the JSON [Encode] writes.
func Decode(p *store.Playback) (Record, error) {
	var q queue
	if err := json.Unmarshal(p.Queue, &q); err != nil {
		return Record{}, fmt.Errorf("%w: queue: %w", ErrInvalid, err)
	}
	rec := Record{
		Kind: Kind(p.Kind), Tracks: q.Tracks, Later: q.Later, Continues: q.Continues,
		Index: p.At, PositionMs: p.PositionMs, Ordered: p.Ordered,
		ClientID: p.ClientID, Device: Device(p.Device),
	}
	if p.ShuffleID != "" {
		rec.Shuffle = &ShuffleAt{ID: p.ShuffleID, After: p.ShuffleAfter}
	}
	if err := Check(&rec); err != nil {
		return Record{}, err
	}
	return rec, nil
}

// IDs returns every track ULID rec names, tracks then later.
func (rec *Record) IDs() []string {
	out := make([]string, 0, len(rec.Tracks)+len(rec.Later))
	return append(append(out, rec.Tracks...), rec.Later...)
}

// Song is a song of a resolved record: its track and its album as they are now.
type Song struct {
	ID, AlbumID string
}

// Resolved is a record as the library has it now: gone songs left out, and each
// song with its album.
type Resolved struct {
	Tracks, Later []Song
	Index         int
	PositionMs    int64
	// Moved says the song saved as current is gone: Index is the next that
	// exists, at 0 ms.
	Moved bool
}

// Resolve returns rec against live, the album ULID of every live track by its
// ULID. Gone songs are dropped; when the current one is gone, the next live one
// after it becomes current at 0 ms. It reports false when none is left at or
// after the current song.
func Resolve(rec *Record, live map[string]string) (Resolved, bool) {
	out := Resolved{Index: -1, PositionMs: rec.PositionMs}
	for i, id := range rec.Tracks {
		album, ok := live[id]
		if !ok {
			continue
		}
		if out.Index < 0 && i >= rec.Index {
			out.Index = len(out.Tracks)
			out.Moved = i != rec.Index
		}
		out.Tracks = append(out.Tracks, Song{ID: id, AlbumID: album})
	}
	for _, id := range rec.Later {
		if album, ok := live[id]; ok {
			out.Later = append(out.Later, Song{ID: id, AlbumID: album})
		}
	}
	if out.Index < 0 {
		return Resolved{}, false
	}
	if out.Moved {
		out.PositionMs = 0
	}
	return out, true
}
