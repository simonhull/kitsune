package navidrome_test

import (
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/navidrome"
	"github.com/simonhull/kitsune/internal/store"
	"github.com/simonhull/kitsune/internal/store/navidromedb"
)

func TestWrite(t *testing.T) {
	t.Parallel()
	w := world{
		songs:  []navidromedb.Song{song("s1", pathOne), song("s2", pathGone)},
		tracks: []store.ImportTrack{ktrack('1', pathOne, albumA)},
		notes: []navidromedb.Annotation{
			{ItemID: "s1", ItemType: navidromedb.ItemSong, PlayCount: 2, PlayDate: ts(2, 0), Rating: 3},
			{ItemID: "s2", ItemType: navidromedb.ItemSong, PlayCount: 1, PlayDate: ts(1, 0)},
		},
	}
	plan, err := w.plan(t)
	if err != nil {
		t.Fatal(err)
	}
	written := []store.ImportCounts{{
		Plays:   map[string]store.ImportCount{navidrome.SourceDated: {Added: 1}, navidrome.SourceUndated: {There: 1}},
		Ratings: store.ImportCount{Kept: 1},
	}}
	tests := []struct {
		name     string
		written  []store.ImportCounts
		verbose  bool
		want     []string
		wantNone []string
	}{
		{
			name: "dry run",
			want: []string{
				"Navidrome user Simon → Kitsune user simon\n",
				"                   source  matched  by path  by MBID  unmatched\n",
				"  plays                 3        2        2        0          1\n",
				"    dated               2        1        1        0          1\n",
				"    undated             1        1        1        0          0\n",
				"  album hearts          0        0        -        -          0\n",
				"  artist hearts         0        0        -        0          0\n",
				"Songs with plays or marks that matched nothing: 1\n",
				"Navidrome users with no Kitsune account: none\n",
				"Matched by path: 1 of 2 songs with plays or marks\n",
			},
			wantNone: []string{"added", pathGone},
		},
		{
			name: "written, verbose", written: written, verbose: true,
			want: []string{
				"                   source  matched  by path  by MBID  unmatched  added  already there  kept\n",
				"  plays                 3        2        2        0          1      1              1     -\n",
				"  ratings               1        1        1        0          0      0              0     1\n",
				"  " + pathGone + "\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			if writeErr := navidrome.Write(&b, &plan, tt.written, tt.verbose); writeErr != nil {
				t.Fatal(writeErr)
			}
			out := b.String()
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			for _, not := range tt.wantNone {
				if strings.Contains(out, not) {
					t.Errorf("output has %q:\n%s", not, out)
				}
			}
		})
	}
}
