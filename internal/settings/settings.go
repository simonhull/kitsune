// Package settings decides which server settings there are, what each may hold,
// and what the server runs with now.
//
// The database keeps every value. server.toml, the environment and flags only
// seed a key that has no value there, and only cmd/ seeds ([Apply]); after that
// the database is the truth, and a restart never undoes a change made in the
// browser. A clearable key cleared in the browser has no value, so the next
// start seeds it again.
//
// A [Service] holds the live values as an immutable [Snapshot], swapped
// atomically on every change, and tells the parts that own a key when it
// changes ([Service.Subscribe]). The Last.fm shared secret is a secret: it is
// stored as written, because the server signs with it, and never logged or
// sent to a client.
package settings

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/simonhull/kitsune/internal/store"
)

// Source says where a key's value came from.
type Source string

// The sources. Only [SourceDefault] is never stored: it means no value.
const (
	SourceDefault Source = "default"
	SourceWeb     Source = "web"
	SourceFile    Source = "server.toml"
	SourceEnv     Source = "env"
	SourceFlag    Source = "flag"
)

// From names where k's value came from for a person: the variable for
// [SourceEnv], such as KITSUNE_LASTFM_API_KEY, the flag for [SourceFlag], and
// "server.toml" for [SourceFile]. It is "" for the others.
func (s Source) From(k Key) string {
	switch s {
	case SourceEnv:
		return k.EnvName()
	case SourceFlag:
		return "-set " + string(k)
	case SourceFile:
		return string(SourceFile)
	case SourceDefault, SourceWeb:
	}
	return ""
}

// KeyError is a value one key doesn't take, or a key there isn't. It wraps
// [ErrUnknownKey] or [ErrBadValue]. Its message never holds the value.
type KeyError struct {
	Key Key
	Err error
}

func (e *KeyError) Error() string { return string(e.Key) + ": " + e.Err.Error() }

// Unwrap returns the reason.
func (e *KeyError) Unwrap() error { return e.Err }

// badValue returns the error for a value key doesn't take.
func badValue(key Key, reason error) *KeyError {
	return &KeyError{Key: key, Err: fmt.Errorf("%w: %w", ErrBadValue, reason)}
}

// Snapshot is every setting's value at one moment, and where each came from.
// It never changes: a change makes a new one.
type Snapshot struct {
	Values

	sources map[Key]Source // Keys with a stored value.
	encoded map[Key][]byte // Every key's canonical JSON.
}

// Source returns where k's value came from, [SourceDefault] when nothing set it.
func (s *Snapshot) Source(k Key) Source {
	if src, ok := s.sources[k]; ok {
		return src
	}
	return SourceDefault
}

// JSON returns k's value as JSON. Never send a secret's to a client.
func (s *Snapshot) JSON(k Key) []byte { return slices.Clone(s.encoded[k]) }

// newSnapshot returns the snapshot of v, with the given sources.
func newSnapshot(v *Values, sources map[Key]Source) *Snapshot {
	encoded := make(map[Key][]byte, len(defs()))
	for k, d := range defs() {
		encoded[k] = d.encode(v)
	}
	return &Snapshot{Values: *v, sources: sources, encoded: encoded}
}

// Service holds the live settings. It is safe for concurrent use.
type Service struct {
	st      *store.Store
	mu      sync.Mutex // Held by the one change at a time.
	cur     atomic.Pointer[Snapshot]
	started *Snapshot // What the server started with, for keys that wait for a restart.

	subsMu sync.Mutex
	subs   []subscriber
}

// subscriber is a channel told about changes to keys.
type subscriber struct {
	keys []Key
	ch   chan struct{}
}

// Open reads the stored settings from st. It returns an error naming the key
// when a stored value no longer passes its key's rule, so the server never runs
// on a value it can't read: `kitsune-server settings reset <key>` clears it.
// A row for a key there isn't, left by a later version, is ignored.
func Open(ctx context.Context, st *store.Store) (*Service, error) {
	rows, err := st.Settings(ctx)
	if err != nil {
		return nil, fmt.Errorf("open settings: %w", err)
	}
	v := Defaults()
	sources := map[Key]Source{}
	all := defs()
	for _, row := range rows {
		k := Key(row.Key)
		d, ok := all[k]
		if !ok {
			continue
		}
		if err = d.decode(&v, row.Value); err != nil {
			return nil, fmt.Errorf("open settings: %w", badValue(k, err))
		}
		sources[k] = Source(row.Source)
	}
	snap := newSnapshot(&v, sources)
	s := &Service{st: st, started: snap}
	s.cur.Store(snap)
	return s, nil
}

// Snapshot returns the live settings. It never waits.
func (s *Service) Snapshot() *Snapshot { return s.cur.Load() }

// PendingRestart reports whether k waits for a restart and has changed since
// the server started.
func (s *Service) PendingRestart(k Key) bool {
	return k.RestartRequired() && !bytes.Equal(s.Snapshot().encoded[k], s.started.encoded[k])
}

// Change sets every key in values to its JSON value, as set in the browser,
// and returns the new snapshot. A null or, for a clearable key, an empty value
// ("" or []) clears the key: it takes its default, and the next start seeds it
// again. Every value is checked before any is stored, and all are stored in
// one transaction, so a refused change changes nothing.
//
// # Errors
//
// A *[KeyError] names the first key, in [Keys] order, that doesn't exist or
// whose value its rule refuses; anything else is the store's.
func (s *Service) Change(ctx context.Context, values map[string][]byte) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.Snapshot()
	next, changes, err := plan(cur, values)
	if err != nil {
		return nil, err
	}
	if err = s.st.ChangeSettings(ctx, changes); err != nil {
		return nil, fmt.Errorf("change settings: %w", err)
	}
	s.cur.Store(next)
	s.notify(cur, next)
	return next, nil
}

// plan checks values against cur and returns the snapshot they make and the
// store writes that make it.
func plan(cur *Snapshot, values map[string][]byte) (*Snapshot, []store.SettingChange, error) {
	for _, name := range slices.Sorted(maps.Keys(values)) {
		if _, err := Lookup(name); err != nil {
			return nil, nil, &KeyError{Key: Key(name), Err: ErrUnknownKey}
		}
	}
	v := cur.Values
	defaults := Defaults()
	sources := maps.Clone(cur.sources)
	var changes []store.SettingChange
	all := defs()
	for _, k := range Keys() {
		raw, ok := values[string(k)]
		if !ok {
			continue
		}
		d := all[k]
		if d.empty(raw) {
			d.reset(&v, &defaults)
			delete(sources, k)
			changes = append(changes, store.SettingChange{Key: string(k)})
			continue
		}
		if err := d.decode(&v, raw); err != nil {
			return nil, nil, badValue(k, err)
		}
		sources[k] = SourceWeb
		changes = append(changes, store.SettingChange{Key: string(k), Value: d.encode(&v), Source: string(SourceWeb)})
	}
	return newSnapshot(&v, sources), changes, nil
}

// Subscribe returns a channel that receives after every change to any of keys.
// Changes that come while the last is unread make one receive, so a reader
// reads [Service.Snapshot] when it receives, not a value per change.
func (s *Service) Subscribe(keys ...Key) <-chan struct{} {
	ch := make(chan struct{}, 1)
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	s.subs = append(s.subs, subscriber{keys: keys, ch: ch})
	return ch
}

// notify tells every subscriber whose keys changed between before and after.
func (s *Service) notify(before, after *Snapshot) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for _, sub := range s.subs {
		changed := slices.ContainsFunc(sub.keys, func(k Key) bool {
			return !bytes.Equal(before.encoded[k], after.encoded[k])
		})
		if !changed {
			continue
		}
		select {
		case sub.ch <- struct{}{}:
		default: // A receive is already pending; it reads the latest snapshot.
		}
	}
}
