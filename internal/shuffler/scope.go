package shuffler

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store"
)

// ErrBadScope means a scope or mode isn't one there is, or lacks what its kind
// needs.
var ErrBadScope = errors.New("bad shuffle scope")

// Kind is what a shuffle draws from.
type Kind string

// The kinds of scope.
const (
	// Library is every song.
	Library Kind = "library"
	// Rated is what the Library list's 4★-and-up filter shows: songs whose
	// own stars are 4★ or more.
	Rated Kind = "rated"
	// Album is one album's songs.
	Album Kind = "album"
	// Artist is the songs an artist is on, as an artist or an album artist.
	Artist Kind = "artist"
	// Tracks is songs named one by one, such as a selection.
	Tracks Kind = "tracks"
)

// goodRating is the least a song under the [Rated] filter has: 4★, so 3½★ is
// out and 4½★ is in.
const goodRating rating.Rating = 8

// MostTracks is the most songs a [Tracks] scope may name.
const MostTracks = 10_000

// Scope is what a shuffle draws from, as a request names it.
type Scope struct {
	Kind Kind `json:"kind"`
	// ID is the album's or the artist's ULID.
	ID string `json:"id,omitzero"`
	// Tracks are the songs' ULIDs, for [Tracks].
	Tracks []string `json:"tracks,omitzero"`
}

// hearted is the scope of the Library list's Hearted filter, which went when
// stars became the only like. A shuffle session or playback record that still
// names it goes on as [Rated].
const hearted = "hearted"

// ParseScope decodes a scope as a shuffle session keeps it, the Hearted scope
// going on as [Rated].
//
// # Errors
//
// An error wrapping [ErrBadScope] for text that isn't a scope.
func ParseScope(text string) (Scope, error) {
	var sc Scope
	if err := json.Unmarshal([]byte(text), &sc); err != nil {
		return Scope{}, fmt.Errorf("decode scope: %w: %w", ErrBadScope, err)
	}
	if sc.Kind == hearted {
		sc.Kind = Rated
	}
	return sc, nil
}

// check reports whether the scope has what its kind needs, and nothing else.
func (sc *Scope) check() error {
	if !sc.fits() {
		return fmt.Errorf("%q: %w", sc.Kind, ErrBadScope)
	}
	return nil
}

// fits reports whether the scope has what its kind needs, and nothing else.
func (sc *Scope) fits() bool {
	named, listed := sc.ID != "", sc.Tracks != nil
	switch sc.Kind {
	case Library, Rated:
		return !named && !listed
	case Album, Artist:
		return named && !listed
	case Tracks:
		return !named && len(sc.Tracks) > 0 && len(sc.Tracks) <= MostTracks && !slices.Contains(sc.Tracks, "")
	}
	return false
}

// Mode is how a shuffle goes.
type Mode string

// The modes.
const (
	// Finite plays every unit once.
	Finite Mode = "finite"
	// Endless goes on, page by page.
	Endless Mode = "endless"
)

// valid reports whether m is one of the modes.
func (m Mode) valid() bool { return m == Finite || m == Endless }

// keep says whether a song is in scope.
type keep func(song *store.ShuffleSong) bool

// keeper works out which songs sc holds, from the store. It returns an error
// wrapping [store.ErrNotFound] for an album or artist that isn't there.
func keeper(ctx context.Context, st *store.Store, sc *Scope) (keep, error) {
	switch sc.Kind {
	case Library:
		return func(*store.ShuffleSong) bool { return true }, nil
	case Rated:
		return func(s *store.ShuffleSong) bool { return s.Rating >= goodRating }, nil
	case Album:
		album, err := st.AlbumRow(ctx, sc.ID)
		return func(s *store.ShuffleSong) bool { return s.AlbumID == album }, err //nolint:wrapcheck // The store names it.
	case Artist:
		rows, err := st.ArtistTrackRows(ctx, sc.ID)
		on := setOf(rows)
		return func(s *store.ShuffleSong) bool { return on[s.ID] }, err //nolint:wrapcheck // The store names it.
	case Tracks:
		return keepNamed(ctx, st, sc.Tracks)
	}
	return nil, fmt.Errorf("%q: %w", sc.Kind, ErrBadScope)
}

// keepNamed keeps the songs with the given ULIDs.
func keepNamed(ctx context.Context, st *store.Store, ids []string) (keep, error) {
	rows, err := st.TrackRows(ctx, ids)
	if err != nil {
		return nil, err //nolint:wrapcheck // The store names it.
	}
	named := make(map[int64]bool, len(rows))
	for _, row := range rows {
		named[row] = true
	}
	return func(s *store.ShuffleSong) bool { return named[s.ID] }, nil
}

// setOf is ids as a set.
func setOf(ids []int64) map[int64]bool {
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}
