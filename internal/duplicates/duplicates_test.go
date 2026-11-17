package duplicates_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/duplicates"
)

// song is a song on album, disc 1, by one artist, length secs seconds, then
// changed by each of fns.
func song(id, title, album string, secs float64, fns ...func(*duplicates.Song)) duplicates.Song {
	s := duplicates.Song{
		ULID: id, Artist: "artist", Title: title, Album: album, Disc: 1,
		Duration: time.Duration(secs * float64(time.Second)),
	}
	for _, fn := range fns {
		fn(&s)
	}
	return s
}

// ab is the group of a and b, by tags.
const ab = "[a b] probably"

// groups returns each group's songs by ULID, and its confidence.
func groups(songs []duplicates.Song, r *duplicates.Result) []string {
	out := make([]string, len(r.Groups))
	for k, g := range r.Groups {
		var ids []string
		for _, i := range g.Songs {
			ids = append(ids, songs[i].ULID)
		}
		out[k] = fmt.Sprintf("%v %s", ids, g.Confidence)
	}
	return out
}

func TestFind(t *testing.T) {
	t.Parallel()
	digest := func(d string) func(*duplicates.Song) { return func(s *duplicates.Song) { s.Digest = d } }
	mbid := func(ids ...string) func(*duplicates.Song) { return func(s *duplicates.Song) { s.Recordings = ids } }
	tests := []struct {
		name  string
		songs []duplicates.Song
		want  []string
	}{
		{
			name:  "the same title and artist within 2 s",
			songs: []duplicates.Song{song("a", "creed", "x", 321), song("b", "creed", "y", 322.9)},
			want:  []string{ab},
		},
		{
			name:  "past 2 s by tags is two songs",
			songs: []duplicates.Song{song("a", "creed", "x", 321), song("b", "creed", "y", 323.1)},
		},
		{
			name: "another artist is another song",
			songs: []duplicates.Song{
				song("a", "creed", "x", 321),
				song("b", "creed", "y", 321, func(s *duplicates.Song) { s.Artist = "other" }),
			},
		},
		{
			name: "one recording within 10 s",
			songs: []duplicates.Song{
				song("a", "creed", "x", 321, mbid("r1")),
				song("b", "creed (live at red rocks)", "y", 330, mbid("r9", "r1")),
			},
			want: nil, // A version word keeps them apart, whatever the recording says.
		},
		{
			name: "one recording within 10 s, titles apart",
			songs: []duplicates.Song{
				song("a", "creed", "x", 321, mbid("r1")),
				song("b", "the creed", "y", 330.5, mbid("r9", "r1")),
			},
			want: []string{"[a b] recording"},
		},
		{
			name: "one recording past 10 s",
			songs: []duplicates.Song{
				song("a", "creed", "x", 321, mbid("r1")),
				song("b", "the creed", "y", 331.5, mbid("r1")),
			},
		},
		{
			name: "the same audio, whatever the tags say",
			songs: []duplicates.Song{
				song("a", "creed", "x", 321, digest("d1")),
				song("b", "track 05", "x", 500, func(s *duplicates.Song) { s.Digest, s.Disc, s.Artist = "d1", 2, "" }),
			},
			want: []string{"[a b] audio"},
		},
		{
			name: "a group takes the strongest evidence any two earn",
			songs: []duplicates.Song{
				song("a", "creed", "x", 321),
				song("b", "creed", "y", 322, mbid("r1")),
				song("c", "creed!", "z", 325, mbid("r1")),
				song("d", "hold me jesus", "x", 180),
			},
			want: []string{"[a b c] recording"},
		},
		{
			name: "version words keep songs apart",
			songs: []duplicates.Song{
				song("a", "searchlights", "x", 200), song("b", "searchlights (live)", "y", 200),
				song("c", "searchlights (acoustic version)", "z", 200),
				song("d", "searchlights (radio edit)", "w", 200),
				song("e", "searchlights (radio edit)", "v", 201),
			},
			want: []string{"[d e] probably"},
		},
		{
			name: "remaster notes join songs",
			songs: []duplicates.Song{
				song("a", "here in america", "x", 236), song("b", "here in america (remastered 2011)", "y", 236.5),
				song("c", "here in america [2011 remaster]", "z", 235.5), song("d", "here in america - remastered", "w", 236),
			},
			want: []string{"[a b c d] probably"},
		},
		{
			name: "a remastered version is still a version",
			songs: []duplicates.Song{
				song("a", "here in america", "x", 236), song("b", "here in america (remastered version)", "y", 236.5),
			},
			want: []string{ab},
		},
		{
			name: "different discs of one album are a reprise",
			songs: []duplicates.Song{
				song("a", "overture", "x", 120),
				song("b", "overture", "x", 121, func(s *duplicates.Song) { s.Disc = 2 }),
			},
		},
		{
			name: "one disc of one album may hold a copy",
			songs: []duplicates.Song{
				song("a", "the lucky one", "x", 240), song("b", "the lucky one", "x", 240),
			},
			want: []string{ab},
		},
		{
			name: "a chain inside a window joins",
			songs: []duplicates.Song{
				song("a", "sans.", "x", 51), song("b", "sans.", "y", 52.5), song("c", "sans.", "z", 54),
			},
			want: []string{"[a b c] probably"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := duplicates.Find(tt.songs)
			if got := groups(tt.songs, &r); !slices.Equal(got, tt.want) {
				t.Errorf("groups = %q, want %q", got, tt.want)
			}
			n := 0
			for _, g := range r.Groups {
				n += len(g.Songs)
			}
			if r.Songs != n {
				t.Errorf("Songs = %d, want %d", r.Songs, n)
			}
		})
	}
}

// A fingerprint more songs share than a library holds copies of one song is
// the same silence, not duplicates.
func TestCrowdedEvidenceIsNone(t *testing.T) {
	t.Parallel()
	songs := make([]duplicates.Song, duplicates.MostInBucket+1)
	for i := range songs {
		songs[i] = song(fmt.Sprintf("s%d", i), fmt.Sprintf("track %d", i), fmt.Sprintf("album %d", i), 30, func(s *duplicates.Song) {
			s.Digest = "silence"
		})
	}
	if r := duplicates.Find(songs); len(r.Groups) != 0 {
		t.Errorf("groups = %d, want none for %d songs of one silence", len(r.Groups), len(songs))
	}
	if r := duplicates.Find(songs[:duplicates.MostInBucket]); len(r.Groups) != 1 {
		t.Errorf("groups = %d, want one for %d copies", len(r.Groups), duplicates.MostInBucket)
	}
}

func TestVersionWords(t *testing.T) {
	t.Parallel()
	want := []string{"live", "acoustic", "demo", "instrumental", "remix", "edit", "mix", "version", "reimagined"}
	if !slices.Equal(duplicates.VersionWords, want) {
		t.Errorf("VersionWords = %q, want %q", duplicates.VersionWords, want)
	}
	for _, w := range want {
		songs := []duplicates.Song{song("a", "fangs", "x", 200), song("b", "fangs ("+w+")", "y", 200)}
		if r := duplicates.Find(songs); len(r.Groups) != 0 {
			t.Errorf("%q joined %q", songs[1].Title, songs[0].Title)
		}
	}
}

func TestKeysAreStable(t *testing.T) {
	t.Parallel()
	a := []duplicates.Song{song("01A", "creed", "x", 321), song("01B", "creed", "y", 321)}
	b := []duplicates.Song{a[1], song("01C", "other", "z", 10), a[0]}
	ra, rb := duplicates.Find(a), duplicates.Find(b)
	if ra.Groups[0].Key != rb.Groups[0].Key {
		t.Errorf("keys %q and %q differ for one group", ra.Groups[0].Key, rb.Groups[0].Key)
	}
	if ra.Groups[0].Key != duplicates.Key([]string{"01B", "01A"}) {
		t.Error("Key depends on order")
	}
	c := append(slices.Clone(a), song("01D", "creed", "z", 321))
	if rc := duplicates.Find(c); rc.Groups[0].Key == ra.Groups[0].Key {
		t.Error("a third copy kept the group's key")
	}
	if duplicates.PairKey("x", "y") != duplicates.PairKey("y", "x") || duplicates.PairKey("x", "y") == duplicates.Key([]string{"x", "y"}) {
		t.Error("PairKey depends on order, or is a group's key")
	}
}

// pairSongs are three songs on a plain and a deluxe album, a song only each
// has, and two more that pair the plain one with another album.
func pairSongs() []duplicates.Song {
	titles := []string{"wish upon a blackstar", "the best it's gonna get", "end of an empire", "against the tide"}
	songs := make([]duplicates.Song, 0, 2*len(titles)+2)
	for i, title := range titles {
		songs = append(songs, song(fmt.Sprintf("p%d", i), title, "plain", float64(200+i*10)))
	}
	songs = append(songs, song("p9", "only on the plain one", "plain", 99))
	for i, title := range titles[:3] {
		songs = append(songs, song(fmt.Sprintf("d%d", i), title, "deluxe", float64(200+i*10)))
	}
	// Two groups pair plain with another album: too few for a pair.
	return append(songs, song("d9", "the lucky one", "deluxe", 77),
		song("o1", titles[3], "other", 230), song("o2", "only on the plain one", "other", 99))
}

// ulidsAt returns the ULIDs of songs at idx.
func ulidsAt(songs []duplicates.Song, idx []int) []string {
	out := make([]string, 0, len(idx))
	for _, i := range idx {
		out = append(out, songs[i].ULID)
	}
	return out
}

func TestAlbumPairs(t *testing.T) {
	t.Parallel()
	songs := pairSongs()
	r := duplicates.Find(songs)
	if len(r.Pairs) != 1 {
		t.Fatalf("pairs = %+v, want one", r.Pairs)
	}
	p := r.Pairs[0]
	if p.Albums != [2]string{"plain", "deluxe"} || len(p.Groups) != 3 || p.Key != duplicates.PairKey("deluxe", "plain") {
		t.Errorf("pair = %+v", p)
	}
	if got := ulidsAt(songs, p.Only[0]); !slices.Equal(got, []string{"p3", "p9"}) {
		t.Errorf("only in plain = %q", got)
	}
	if got := ulidsAt(songs, p.Only[1]); !slices.Equal(got, []string{"d9"}) {
		t.Errorf("only in deluxe = %q", got)
	}
	for k, g := range r.Groups {
		if inPair := slices.Contains(p.Groups, k); (g.Pair == 0) != inPair {
			t.Errorf("group %d: Pair = %d, in the pair %v", k, g.Pair, inPair)
		}
	}
}

func TestKeeper(t *testing.T) {
	t.Parallel()
	day := func(n int) time.Time { return time.Date(2025, 3, n, 0, 0, 0, 0, time.UTC) }
	flac := duplicates.Copy{Lossless: true, Original: true, Bitrate: 1000, Plays: 1, AddedAt: day(1)}
	tests := []struct {
		name   string
		copies []duplicates.Copy
		want   int
		why    []duplicates.Reason
	}{
		{
			name:   "lossless over lossy, whatever else",
			copies: []duplicates.Copy{{Bitrate: 320, Original: true, InPlace: true, Plays: 90, AddedAt: day(1)}, {Lossless: true, Bitrate: 900, AddedAt: day(9)}},
			want:   1, why: []duplicates.Reason{duplicates.Lossless, duplicates.Bitrate},
		},
		{
			// Creed: A Liturgy… over the best-of Songs, whose bitrate is 7% higher and which has more plays.
			name: "an original album over a best-of",
			copies: []duplicates.Copy{
				{Lossless: true, Original: true, Bitrate: 957, InPlace: true, Plays: 1, AddedAt: day(21)},
				{Lossless: true, Bitrate: 1022, InPlace: true, Plays: 2, AddedAt: day(21)},
			},
			want: 0, why: []duplicates.Reason{duplicates.Original},
		},
		{
			name:   "a higher bitrate by over 5%",
			copies: []duplicates.Copy{flac, with2(flac, func(c *duplicates.Copy) { c.Bitrate = 1060 })},
			want:   1, why: []duplicates.Reason{duplicates.Bitrate},
		},
		{
			name:   "a bitrate within 5% is no reason",
			copies: []duplicates.Copy{flac, with2(flac, func(c *duplicates.Copy) { c.Bitrate, c.InPlace = 1040, true })},
			want:   1, why: []duplicates.Reason{duplicates.InPlace},
		},
		{
			name:   "in place over more plays",
			copies: []duplicates.Copy{with2(flac, func(c *duplicates.Copy) { c.Plays = 9 }), with2(flac, func(c *duplicates.Copy) { c.InPlace = true })},
			want:   1, why: []duplicates.Reason{duplicates.InPlace},
		},
		{
			name:   "more plays over earlier",
			copies: []duplicates.Copy{flac, with2(flac, func(c *duplicates.Copy) { c.Plays, c.AddedAt = 2, day(9) })},
			want:   1, why: []duplicates.Reason{duplicates.Plays},
		},
		{
			name:   "earlier last",
			copies: []duplicates.Copy{with2(flac, func(c *duplicates.Copy) { c.AddedAt = day(9) }), flac},
			want:   1, why: []duplicates.Reason{duplicates.Earlier},
		},
		{
			name:   "every way it beats all the others",
			copies: []duplicates.Copy{{Bitrate: 320, AddedAt: day(1)}, {Lossless: true, Original: true, Bitrate: 994, InPlace: true, Plays: 4, AddedAt: day(2)}},
			want:   1, why: []duplicates.Reason{duplicates.Lossless, duplicates.Original, duplicates.Bitrate, duplicates.InPlace, duplicates.Plays},
		},
		{
			name: "with three, what set it apart from the nearest",
			copies: []duplicates.Copy{
				{Bitrate: 320, Plays: 50, AddedAt: day(1)},
				with2(flac, func(c *duplicates.Copy) { c.Plays = 3 }),
				with2(flac, func(c *duplicates.Copy) { c.Plays = 2 }),
			},
			want: 1, why: []duplicates.Reason{duplicates.Plays},
		},
		{
			name:   "alike in everything keeps the first, for no reason",
			copies: []duplicates.Copy{flac, flac},
			want:   0,
		},
		{
			name:   "an unknown bitrate is never higher",
			copies: []duplicates.Copy{with2(flac, func(c *duplicates.Copy) { c.Bitrate = 0 }), with2(flac, func(c *duplicates.Copy) { c.Plays = 0 })},
			want:   0, why: []duplicates.Reason{duplicates.Plays},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, why := duplicates.Keeper(tt.copies)
			if got != tt.want || !slices.Equal(why, tt.why) {
				t.Errorf("Keeper = %d %q, want %d %q", got, why, tt.want, tt.why)
			}
		})
	}
	if got, why := duplicates.Keeper(nil); got != -1 || why != nil {
		t.Errorf("Keeper(nil) = %d %q", got, why)
	}
}

// with2 returns c changed by fn.
func with2(c duplicates.Copy, fn func(*duplicates.Copy)) duplicates.Copy {
	fn(&c)
	return c
}

func BenchmarkFind(b *testing.B) {
	songs := make([]duplicates.Song, 50000)
	for i := range songs {
		songs[i] = song(fmt.Sprintf("%026d", i), fmt.Sprintf("title %d", i%49000), fmt.Sprintf("album %d", i/12), float64(120+i%300))
		songs[i].Artist = fmt.Sprintf("artist %d", i%3000)
		if i%3 == 0 {
			songs[i].Recordings = []string{fmt.Sprintf("rec %d", i)}
		}
	}
	for b.Loop() {
		duplicates.Find(songs)
	}
}
