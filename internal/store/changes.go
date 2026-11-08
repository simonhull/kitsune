package store

import (
	"slices"
	"sync"
)

// changedAlbums gathers the public IDs of the albums library writes touched, so
// the library's writer can say what changed. It holds each once.
type changedAlbums struct {
	mu  sync.Mutex
	ids map[string]struct{}
}

// add records ids, after the transaction that touched them committed.
func (c *changedAlbums) add(ids []string) {
	if len(ids) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ids == nil {
		c.ids = make(map[string]struct{}, len(ids))
	}
	for _, id := range ids {
		c.ids[id] = struct{}{}
	}
}

// TakeChangedAlbums returns the public IDs of the albums that [Store.WriteBatch]
// and [Store.MarkMissing] touched since the last call, sorted, and forgets them.
// A track that moved touches both its albums; an album that no longer has a live
// track is in the list too, since it has left the index. The library's one
// writer calls it after its writes, to say what changed.
func (s *Store) TakeChangedAlbums() []string {
	s.changed.mu.Lock()
	defer s.changed.mu.Unlock()
	out := make([]string, 0, len(s.changed.ids))
	for id := range s.changed.ids {
		out = append(out, id)
	}
	s.changed.ids = nil
	slices.Sort(out)
	return out
}
