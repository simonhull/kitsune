package shuffler

import (
	"encoding/json/v2"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/shuffle"
)

// keptPages is how many of the last pages a session keeps, to give again to a
// device that asks for one it missed.
const keptPages = 4

// state is a session's own, kept by the store as it is: a finite order, or an
// endless shuffle's state, and the last pages given.
type state struct {
	// Songs is how many songs the scope held that shuffle could pick, at the
	// last page worked out.
	Songs   int           `json:"songs"`
	Endless *endlessState `json:"endless,omitzero"`
	Order   []placed      `json:"order,omitzero"`
	Given   []given       `json:"given"`
}

// placed is one unit of a finite order: its songs, and how flow leaned to it.
type placed struct {
	Tracks []int64 `json:"t"`
	Flow   float64 `json:"f,omitzero"`
	After  int64   `json:"a,omitzero"`
	Sim    float64 `json:"s,omitzero"`
}

// given is a page given: the position it starts at, the position after it, and
// each pick at its position. A finite page may skip a position whose unit has
// gone since the order was made.
type given struct {
	From  int64       `json:"from"`
	To    int64       `json:"to"`
	Picks []givenPick `json:"picks"`
}

// givenPick is one pick of a page given.
type givenPick struct {
	At     int64   `json:"at"`
	Tracks []int64 `json:"tracks"`
	Why    Why     `json:"why"`
}

// endlessState is [shuffle.EndlessState] without its seed and position, which
// the session row holds, and with the units played this cycle in columns,
// since there can be tens of thousands of them.
type endlessState struct {
	First   int64     `json:"first,omitzero"`
	Recent  []recent  `json:"recent"`
	Units   []int64   `json:"units"`
	At      []int64   `json:"at"`
	Weights []float64 `json:"weights"`
	// Given and GivenAt are every unit given, at the position it last was.
	Given   []int64  `json:"given"`
	GivenAt []uint64 `json:"givenAt"`
}

// recent is one of the last units given, for spacing across pages.
type recent struct {
	Artists []shuffle.ArtistID `json:"r"`
	Albums  []shuffle.AlbumID  `json:"l"`
}

// endlessOf is st as a session keeps it.
func endlessOf(st *shuffle.EndlessState) *endlessState {
	out := &endlessState{
		First:   int64(st.First),
		Recent:  make([]recent, len(st.Recent)),
		Units:   make([]int64, len(st.Played)),
		At:      make([]int64, len(st.Played)),
		Weights: make([]float64, len(st.Played)),
		Given:   make([]int64, len(st.Last)),
		GivenAt: make([]uint64, len(st.Last)),
	}
	for i, g := range st.Last {
		out.Given[i], out.GivenAt[i] = int64(g.Unit), g.At
	}
	for i, r := range st.Recent {
		out.Recent[i] = recent{Artists: r.Artists, Albums: r.Albums}
	}
	for i, p := range st.Played {
		out.Units[i], out.At[i], out.Weights[i] = int64(p.Unit), p.At.UnixMilli(), p.Weight
	}
	return out
}

// resume is the endless shuffle e was taken from, at position from seed.
func (e *endlessState) resume(seed, position uint64) (*shuffle.Endless, error) {
	if len(e.At) != len(e.Units) || len(e.Weights) != len(e.Units) || len(e.GivenAt) != len(e.Given) {
		return nil, fmt.Errorf("endless state: %d units, %d times, %d weights: %w", len(e.Units), len(e.At), len(e.Weights), errBadState)
	}
	st := shuffle.EndlessState{
		Seed: seed, Position: position, First: shuffle.TrackID(e.First),
		Recent: make([]shuffle.Recent, len(e.Recent)),
		Played: make([]shuffle.Played, len(e.Units)),
		Last:   make([]shuffle.Given, len(e.Given)),
	}
	for i := range e.Given {
		st.Last[i] = shuffle.Given{Unit: shuffle.TrackID(e.Given[i]), At: e.GivenAt[i]}
	}
	for i, r := range e.Recent {
		st.Recent[i] = shuffle.Recent{Artists: r.Artists, Albums: r.Albums}
	}
	for i := range e.Units {
		st.Played[i] = shuffle.Played{Unit: shuffle.TrackID(e.Units[i]), At: time.UnixMilli(e.At[i]), Weight: e.Weights[i]}
	}
	return shuffle.ResumeEndless(&st), nil
}

// encode is the state as the store keeps it.
func (st *state) encode() ([]byte, error) {
	b, err := json.Marshal(st)
	if err != nil {
		return nil, fmt.Errorf("encode shuffle state: %w", err)
	}
	return b, nil
}

// decodeState reads a state the store kept.
func decodeState(b []byte) (*state, error) {
	var st state
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("decode shuffle state: %w: %w", errBadState, err)
	}
	return &st, nil
}

// remember adds a page given, keeping the last [keptPages].
func (st *state) remember(g given) {
	st.Given = append(st.Given, g)
	if n := len(st.Given); n > keptPages {
		st.Given = st.Given[n-keptPages:]
	}
}

// givenFrom returns what was given from after on, from the page that holds it,
// or false when no page kept does.
func (st *state) givenFrom(after int64) (given, bool) {
	for _, g := range st.Given {
		if after < g.From || after >= g.To {
			continue
		}
		out := given{From: after, To: g.To}
		for i := range g.Picks {
			if g.Picks[i].At >= after {
				out.Picks = g.Picks[i:]
				break
			}
		}
		return out, true
	}
	return given{}, false
}
