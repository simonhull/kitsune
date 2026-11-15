package store_test

import (
	"strconv"
	"testing"

	"github.com/simonhull/kitsune/internal/store"
)

// BenchmarkMovePlaylistEntry moves the first entry of a playlist to its end and
// back, the longest move there is: with sparse positions each move writes one
// row, whatever the playlist's length, and only a used-up gap renumbers it.
func BenchmarkMovePlaylistEntry(b *testing.B) {
	for _, n := range []int{151, store.MostPlaylistEntries} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			l := newPlaylistLib(b)
			is := make([]int, n)
			for i := range is {
				is[i] = i % len(l.tracks)
			}
			id := l.make(b, "Long", is...)
			d, err := l.s.Playlist(b.Context(), l.user, id, nil)
			if err != nil {
				b.Fatal(err)
			}
			moving, second, last := d.Entries[0].ID, d.Entries[1].ID, d.Entries[n-1].ID
			there := false
			for b.Loop() {
				if there {
					err = l.s.MovePlaylistEntry(b.Context(), l.user, id, moving, second, false)
				} else {
					err = l.s.MovePlaylistEntry(b.Context(), l.user, id, moving, last, true)
				}
				if err != nil {
					b.Fatal(err)
				}
				there = !there
			}
		})
	}
}
