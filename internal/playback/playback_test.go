package playback_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/simonhull/kitsune/internal/playback"
)

// id is a 26-character ULID for n.
func id(n int) string { return fmt.Sprintf("01J%023d", n) }

func ids(from, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = id(from + i)
	}
	return out
}

const otherClient = "abcdef0123456789"

func valid() playback.Record {
	return playback.Record{
		Kind: playback.Album, Tracks: ids(0, 3), Index: 1, PositionMs: 133_000, Ordered: true,
		ClientID: "0123456789abcdef", Device: playback.Computer,
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		edit func(r *playback.Record)
		ok   bool
	}{
		{"valid", func(*playback.Record) {}, true},
		{"1,000 songs", func(r *playback.Record) {
			r.Kind, r.Tracks, r.Later = playback.List, ids(0, 400), ids(400, 600)
		}, true},
		{"1,001 songs across tracks and later", func(r *playback.Record) {
			r.Kind, r.Tracks, r.Later = playback.List, ids(0, 400), ids(400, 601)
		}, false},
		{"no tracks", func(r *playback.Record) { r.Tracks, r.Index = nil, 0 }, false},
		{"index past the end", func(r *playback.Record) { r.Index = 3 }, false},
		{"negative index", func(r *playback.Record) { r.Index = -1 }, false},
		{"a day in", func(r *playback.Record) { r.PositionMs = playback.MaxPositionMs }, true},
		{"over a day in", func(r *playback.Record) { r.PositionMs = playback.MaxPositionMs + 1 }, false},
		{"negative position", func(r *playback.Record) { r.PositionMs = -1 }, false},
		{"unknown kind", func(r *playback.Record) { r.Kind = "radio" }, false},
		{"shuffle on an album", func(r *playback.Record) { r.Shuffle = &playback.ShuffleAt{ID: id(9)} }, false},
		{"shuffle without its session", func(r *playback.Record) { r.Kind = playback.Shuffle }, false},
		{"shuffle with its session", func(r *playback.Record) {
			r.Kind, r.Shuffle = playback.Shuffle, &playback.ShuffleAt{ID: id(9), After: 4}
		}, true},
		{"shuffle with a bad session", func(r *playback.Record) {
			r.Kind, r.Shuffle = playback.Shuffle, &playback.ShuffleAt{ID: "nope"}
		}, false},
		{"shuffle position negative", func(r *playback.Record) {
			r.Kind, r.Shuffle = playback.Shuffle, &playback.ShuffleAt{ID: id(9), After: -1}
		}, false},
		{"later on songs", func(r *playback.Record) { r.Kind, r.Later = playback.Songs, ids(5, 1) }, false},
		{"continues on an album", func(r *playback.Record) { r.Continues = id(8) }, true},
		{"continues on songs", func(r *playback.Record) { r.Kind, r.Continues = playback.Songs, id(8) }, true},
		{"continues on a list", func(r *playback.Record) { r.Kind, r.Continues = playback.List, id(8) }, false},
		{"a bad continues", func(r *playback.Record) { r.Continues = "album" }, false},
		{"a lowercase ULID", func(r *playback.Record) { r.Tracks[0] = "01j00000000000000000000000" }, false},
		{"a ULID past 128 bits", func(r *playback.Record) { r.Tracks[0] = "81J00000000000000000000000" }, false},
		{"a ULID with an I", func(r *playback.Record) { r.Tracks[0] = "01I00000000000000000000000" }, false},
		{"a short ULID", func(r *playback.Record) { r.Tracks[0] = "01J" }, false},
		{"a bad later", func(r *playback.Record) { r.Kind, r.Later = playback.List, []string{"x"} }, false},
		{"client ID too short", func(r *playback.Record) { r.ClientID = "0123456789abcde" }, false},
		{"client ID of 64", func(r *playback.Record) { r.ClientID = string(slices.Repeat([]byte("a"), 64)) }, true},
		{"client ID of 65", func(r *playback.Record) { r.ClientID = string(slices.Repeat([]byte("a"), 65)) }, false},
		{"client ID not hex", func(r *playback.Record) { r.ClientID = "0123456789abcdeg" }, false},
		{"client ID upper case", func(r *playback.Record) { r.ClientID = "0123456789ABCDEF" }, false},
		{"unknown device", func(r *playback.Record) { r.Device = "watch" }, false},
		{"a phone", func(r *playback.Record) { r.Device = playback.Phone }, true},
		{"a tablet", func(r *playback.Record) { r.Device = playback.Tablet }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := valid()
			tt.edit(&rec)
			err := playback.Check(&rec)
			if tt.ok && err != nil {
				t.Errorf("Check = %v, want nil", err)
			}
			if !tt.ok && !errors.Is(err, playback.ErrInvalid) {
				t.Errorf("Check = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestEncodeDecode(t *testing.T) {
	t.Parallel()
	for _, rec := range []playback.Record{
		valid(),
		{
			Kind: playback.List, Tracks: ids(0, 2), Later: ids(2, 2), Index: 1,
			ClientID: otherClient, Device: playback.Phone,
		},
		{
			Kind: playback.Shuffle, Tracks: ids(0, 1), Shuffle: &playback.ShuffleAt{ID: id(7), After: 12},
			ClientID: otherClient, Device: playback.Tablet,
		},
		{Kind: playback.Album, Tracks: ids(0, 1), Continues: id(5), ClientID: otherClient, Device: playback.Phone},
	} {
		stored, err := playback.Encode(&rec)
		if err != nil {
			t.Fatal(err)
		}
		got, err := playback.Decode(&stored)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, rec) {
			t.Errorf("round trip = %+v, want %+v", got, rec)
		}
	}
}

func TestDecodeChecksAgain(t *testing.T) {
	t.Parallel()
	rec := valid()
	stored, err := playback.Encode(&rec)
	if err != nil {
		t.Fatal(err)
	}
	stored.At = 7
	if _, err = playback.Decode(&stored); !errors.Is(err, playback.ErrInvalid) {
		t.Errorf("Decode of a bad index = %v, want ErrInvalid", err)
	}
	stored.At, stored.Queue = 0, []byte("[")
	if _, err = playback.Decode(&stored); !errors.Is(err, playback.ErrInvalid) {
		t.Errorf("Decode of a bad queue = %v, want ErrInvalid", err)
	}
}

func album(n int) string { return fmt.Sprintf("01A%023d", n) }

// liveBut is songs 0 to 5 live, but for skip, three to an album.
func liveBut(skip ...int) map[string]string {
	out := map[string]string{}
	for n := range 6 {
		if !slices.Contains(skip, n) {
			out[id(n)] = album(n / 3)
		}
	}
	return out
}

func TestResolve(t *testing.T) {
	t.Parallel()
	list := playback.Record{Kind: playback.List, Tracks: ids(0, 4), Later: ids(4, 2), Index: 1, PositionMs: 5000}
	tests := []struct {
		name    string
		gone    []int
		ok      bool
		current string
		index   int
		moved   bool
		at      int64
		later   int
	}{
		{"all live", nil, true, id(1), 1, false, 5000, 2},
		{"one behind gone", []int{0}, true, id(1), 0, false, 5000, 2},
		{"current gone", []int{1, 2}, true, id(3), 1, true, 0, 2},
		{"later filtered", []int{4}, true, id(1), 1, false, 5000, 1},
		{"nothing at or after the current song", []int{1, 2, 3}, false, "", 0, false, 0, 0},
		{"every song gone", []int{0, 1, 2, 3, 4, 5}, false, "", 0, false, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := playback.Resolve(&list, liveBut(tt.gone...))
			if ok != tt.ok {
				t.Fatalf("Resolve = %+v, %v; want ok %v", got, ok, tt.ok)
			}
			if !ok {
				return
			}
			if got.Tracks[got.Index].ID != tt.current || got.Index != tt.index || got.Moved != tt.moved ||
				got.PositionMs != tt.at || len(got.Later) != tt.later {
				t.Errorf("Resolve = %+v", got)
			}
		})
	}
}

func TestResolveAttachesAlbums(t *testing.T) {
	t.Parallel()
	list := playback.Record{Kind: playback.List, Tracks: ids(0, 4), Later: ids(4, 2)}
	got, _ := playback.Resolve(&list, liveBut())
	if got.Tracks[3] != (playback.Song{ID: id(3), AlbumID: album(1)}) || got.Later[1].AlbumID != album(1) {
		t.Errorf("Resolve = %+v, want each song's album", got)
	}
}
