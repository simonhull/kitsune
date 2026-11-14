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

// MaxSongs is the most songs a record holds, across its tracks, later and the
// order of a shuffle the player made.
const MaxSongs = 1000

// MaxSeed is the largest seed a shuffle the player made may have: the client's
// generator takes 32 bits.
const MaxSeed = 1<<32 - 1

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

// Repeat is what the player does at the end of the queue, or of a song.
type Repeat string

// The repeats there are.
const (
	// RepeatOff comes to rest at the end of the queue.
	RepeatOff Repeat = "off"
	// RepeatAll goes round again from the queue's first song.
	RepeatAll Repeat = "all"
	// RepeatOne plays the song again from the start.
	RepeatOne Repeat = "one"
)

// Shuffled is a queue the player's Shuffle button reordered itself, with no
// session: the seed it used, and the queue's own order from the current song
// on, which turning it off goes back to.
type Shuffled struct {
	Seed  int64
	Order []string
}

// Record is a person's remembered playback, as a client saved it.
type Record struct {
	Kind Kind
	// Tracks are the queue's track ULIDs in play order, and Later the songs a
	// list only names so far.
	Tracks, Later []string
	// Continues is the album ULID a queue goes on from, down the Library
	// list: an album, or songs once the list has put another album after it.
	Continues string
	// Index is the current song's place in Tracks.
	Index      int
	PositionMs int64
	// Ordered says album gain applies.
	Ordered bool
	// Shuffle is the session a shuffle queue plays in, nil for the others.
	Shuffle *ShuffleAt
	// Repeat is off, all or one. A record saved before it is off.
	Repeat Repeat
	// Shuffled is set while the player's own shuffle is on, for any kind but
	// shuffle, whose session is its shuffle.
	Shuffled *Shuffled
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
	errContinues  = errors.New("continues goes only with kind album or songs")
	errShuffleNeg = errors.New("shuffle position is negative")
	errRepeat     = errors.New("unknown repeat")
	errShuffled   = errors.New("shuffled goes with any kind but shuffle")
	errOrder      = errors.New("shuffled needs its order")
	errSeed       = fmt.Errorf("shuffled seed outside 0 to %d", MaxSeed)
)

func check(rec *Record) error {
	for _, step := range []func(*Record) error{checkLabels, checkPlace, checkPairs, checkShuffled, checkIDs} {
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
	if !slices.Contains([]Repeat{RepeatOff, RepeatAll, RepeatOne}, rec.Repeat) {
		return errRepeat
	}
	return nil
}

// checkShuffled checks the player's own shuffle: its kind, its seed and that it
// names an order.
func checkShuffled(rec *Record) error {
	switch sh := rec.Shuffled; {
	case sh == nil:
		return nil
	case rec.Kind == Shuffle:
		return errShuffled
	case len(sh.Order) == 0:
		return errOrder
	case sh.Seed < 0 || sh.Seed > MaxSeed:
		return errSeed
	}
	return nil
}

// checkPlace checks the queue's size and the place in it.
func checkPlace(rec *Record) error {
	switch {
	case len(rec.Tracks) == 0:
		return errNoTracks
	case len(rec.IDs()) > MaxSongs:
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
	case rec.Continues != "" && rec.Kind != Album && rec.Kind != Songs:
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
	// Repeat is absent for off, as in every record saved before it.
	Repeat   Repeat         `json:"repeat,omitzero"`
	Shuffled *queueShuffled `json:"shuffled,omitzero"`
}

// queueShuffled is a record's [Shuffled] as the store keeps it.
type queueShuffled struct {
	Seed  int64    `json:"seed"`
	Order []string `json:"order"`
}

// Encode returns rec as the store keeps it. Its UpdatedAt is the store's to set.
//
// # Errors
//
// It fails only if the queue can't be encoded as JSON.
func Encode(rec *Record) (store.Playback, error) {
	kept := queue{Tracks: rec.Tracks, Later: rec.Later, Continues: rec.Continues}
	if rec.Repeat != RepeatOff {
		kept.Repeat = rec.Repeat
	}
	if rec.Shuffled != nil {
		kept.Shuffled = &queueShuffled{Seed: rec.Shuffled.Seed, Order: rec.Shuffled.Order}
	}
	q, err := json.Marshal(kept)
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
		Index: p.At, PositionMs: p.PositionMs, Ordered: p.Ordered, Repeat: q.Repeat,
		ClientID: p.ClientID, Device: Device(p.Device),
	}
	if rec.Repeat == "" {
		rec.Repeat = RepeatOff
	}
	if p.ShuffleID != "" {
		rec.Shuffle = &ShuffleAt{ID: p.ShuffleID, After: p.ShuffleAfter}
	}
	if q.Shuffled != nil {
		rec.Shuffled = &Shuffled{Seed: q.Shuffled.Seed, Order: q.Shuffled.Order}
	}
	if err := Check(&rec); err != nil {
		return Record{}, err
	}
	return rec, nil
}

// IDs returns every track ULID rec names, tracks then later, then a shuffle's
// own order.
func (rec *Record) IDs() []string {
	var order []string
	if rec.Shuffled != nil {
		order = rec.Shuffled.Order
	}
	out := make([]string, 0, len(rec.Tracks)+len(rec.Later)+len(order))
	return append(append(append(out, rec.Tracks...), rec.Later...), order...)
}

// Song is a song of a resolved record: its track and its album as they are now.
type Song struct {
	ID, AlbumID string
}

// Resolved is a record as the library has it now: gone songs left out, and each
// song with its album.
type Resolved struct {
	Tracks, Later []Song
	// Order is a shuffle's own order, as the library has it now: nil when the
	// record has no [Shuffled], and when none of its order is left.
	Order      []Song
	Index      int
	PositionMs int64
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
	out.Later = liveSongs(rec.Later, live)
	if rec.Shuffled != nil {
		out.Order = liveSongs(rec.Shuffled.Order, live)
	}
	if out.Index < 0 {
		return Resolved{}, false
	}
	if out.Moved {
		out.PositionMs = 0
	}
	return out, true
}

// liveSongs returns those of ids live has, each with its album, in order.
func liveSongs(ids []string, live map[string]string) []Song {
	var out []Song
	for _, id := range ids {
		if album, ok := live[id]; ok {
			out = append(out, Song{ID: id, AlbumID: album})
		}
	}
	return out
}
