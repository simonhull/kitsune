// Package duplicates finds copies of one recording in a library and suggests
// which copy to keep. It owns what counts as evidence that two songs are one
// recording, the words that keep two versions apart, and the order of reasons a
// keeper is chosen by.
//
// It is pure: it takes plain values and knows nothing of the store, the files
// or the organizer. [Find] keys every song into buckets (its fingerprint, its
// MusicBrainz recordings, its artist and title) and compares lengths only inside
// a bucket, so it is linear in the library, with no comparison of every pair.
package duplicates

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"
	"unicode"
)

const (
	// SameTagsWithin is how far apart two songs' lengths may be and still be
	// one song by their tags.
	SameTagsWithin = 2 * time.Second
	// SameRecordingWithin is the same for two songs naming one MusicBrainz
	// recording.
	SameRecordingWithin = 10 * time.Second
	// PairGroups is how many groups must pair the same two albums before the
	// list shows them as one album pair.
	PairGroups = 3
	// keyBytes is how much of a SHA-256 names a group: 96 bits.
	keyBytes = 12
	// minGroup is the fewest songs a group holds.
	minGroup = 2
)

// Song is one live song as Find reads it.
type Song struct {
	ULID string
	// Artist is the song's first artist's identity key: the library's MatchKey.
	Artist string
	// Title is the title folded as a search folds it.
	Title    string
	Duration time.Duration
	// Digest is the fingerprint pass's hash of the audio, or "" before the
	// pass reaches the song.
	Digest string
	// Recordings are the MusicBrainz recording IDs the song is tagged with.
	Recordings []string
	Album      string // The album's public ID.
	Disc       int
}

// Confidence is how sure a group is that its songs are one recording.
type Confidence uint8

// The confidences, weakest first.
const (
	// SameSong is a match by tags: the same first artist and title, and lengths
	// within [SameTagsWithin].
	SameSong Confidence = iota + 1
	// SameRecording is a match by MusicBrainz recording, lengths within
	// [SameRecordingWithin].
	SameRecording
	// SameAudio is a match by fingerprint: the same sound in both files.
	SameAudio
)

// String names the confidence as the API does.
func (c Confidence) String() string {
	switch c {
	case SameSong:
		return "probably"
	case SameRecording:
		return "recording"
	case SameAudio:
		return "audio"
	}
	return ""
}

// Group is songs that look like one recording.
type Group struct {
	// Key names the group while it holds these songs: a hash of their ULIDs.
	Key string
	// Songs are the group's songs, by index into what Find was given, in that
	// order.
	Songs []int
	// Confidence is the strongest any two of its songs earn.
	Confidence Confidence
	// Pair is the index of the album pair the group belongs to, or -1.
	Pair int
}

// AlbumPair is two albums that three or more groups pair, which can be
// resolved in one go by keeping one album.
type AlbumPair struct {
	// Key names the pair: a hash of the two albums' public IDs.
	Key    string
	Albums [2]string // In the order of their first songs.
	// Groups are the pair's groups, by index into the result's.
	Groups []int
	// Only are each album's songs no group pairs with the other album, by
	// index into what Find was given. A merge never trashes them.
	Only [2][]int
}

// Result is what Find found.
type Result struct {
	Groups []Group     // In the order of their first songs.
	Pairs  []AlbumPair // In the order of their first groups.
	// Songs counts the songs in a group.
	Songs int
}

// Find groups songs that look like one recording. Two songs are one when
//
//   - they have the same fingerprint; or
//   - they name one MusicBrainz recording, with lengths within
//     [SameRecordingWithin]; or
//   - they have the same first artist and the same title, once a trailing
//     remaster note is taken off, with lengths within [SameTagsWithin].
//
// Neither of the last two holds when a version word is in one title and not
// the other ([VersionWords]), or when they are on different discs of one
// album, since a song repeated on an album is usually a reprise. A group is
// every song joined to another by any of these.
func Find(songs []Song) Result {
	f := finder{songs: songs, parent: make([]int, len(songs)), best: make([]Confidence, len(songs))}
	for i := range f.parent {
		f.parent[i] = i
	}
	titles := make([]baseTitle, len(songs))
	for i := range songs {
		titles[i] = parseTitle(songs[i].Title)
	}
	f.titles = titles
	f.bucket(func(i int, add func(string)) {
		if d := songs[i].Digest; d != "" {
			add(d)
		}
	}, SameAudio, 0)
	f.bucket(func(i int, add func(string)) {
		for _, r := range songs[i].Recordings {
			if r != "" {
				add(r)
			}
		}
	}, SameRecording, SameRecordingWithin)
	f.bucket(func(i int, add func(string)) {
		if a, t := songs[i].Artist, titles[i].text; a != "" && t != "" {
			add(a + "\x00" + t)
		}
	}, SameSong, SameTagsWithin)
	return f.result()
}

// finder joins songs into groups, as a union-find over their indexes.
type finder struct {
	songs  []Song
	titles []baseTitle
	parent []int
	best   []Confidence // The strongest evidence joining each root's group.
}

// bucket keys every song by keys and joins, inside each bucket, the songs
// within window of each other that may be one, as evidence c. A zero window
// joins every song in the bucket.
func (f *finder) bucket(keys func(i int, add func(string)), c Confidence, window time.Duration) {
	buckets := map[string][]int{}
	for i := range f.songs {
		keys(i, func(k string) {
			b := buckets[k]
			if len(b) > 0 && b[len(b)-1] == i {
				return // A song tagged with one recording twice.
			}
			buckets[k] = append(b, i)
		})
	}
	for _, b := range buckets {
		switch {
		case len(b) < minGroup:
		case window == 0:
			for _, j := range b[1:] {
				f.join(b[0], j, c)
			}
		default:
			f.joinWithin(b, c, window)
		}
	}
}

// joinWithin joins the songs of bucket b whose lengths are within window of
// each other and that nothing keeps apart, as evidence c. It sorts b by
// length, so each song is compared only with those just after it.
func (f *finder) joinWithin(b []int, c Confidence, window time.Duration) {
	slices.SortStableFunc(b, func(x, y int) int { return cmp.Compare(f.songs[x].Duration, f.songs[y].Duration) })
	for k, i := range b {
		for _, j := range b[k+1:] {
			if f.songs[j].Duration-f.songs[i].Duration > window {
				break
			}
			if !f.apart(i, j) {
				f.join(i, j, c)
			}
		}
	}
}

// apart reports whether songs i and j are kept apart however much else
// matches: a version word in one title and not the other, or different discs
// of one album.
func (f *finder) apart(i, j int) bool {
	a, b := &f.songs[i], &f.songs[j]
	if a.Album == b.Album && a.Disc != b.Disc {
		return true
	}
	return f.titles[i].versions != f.titles[j].versions
}

// root returns the root of i's group.
func (f *finder) root(i int) int {
	for f.parent[i] != i {
		f.parent[i] = f.parent[f.parent[i]]
		i = f.parent[i]
	}
	return i
}

// join puts i and j in one group, which c is evidence for.
func (f *finder) join(i, j int, c Confidence) {
	ri, rj := f.root(i), f.root(j)
	best := max(f.best[ri], f.best[rj], c)
	if ri != rj {
		// The smaller index stays the root, so a group's order never depends
		// on the order of joins.
		ri, rj = min(ri, rj), max(ri, rj)
		f.parent[rj] = ri
	}
	f.best[ri] = best
}

// result gathers the groups and their album pairs.
func (f *finder) result() Result {
	var out Result
	at := map[int]int{} // Group index, by root.
	for i := range f.songs {
		r := f.root(i)
		if f.best[r] == 0 {
			continue // Alone.
		}
		k, ok := at[r]
		if !ok {
			k = len(out.Groups)
			at[r] = k
			out.Groups = append(out.Groups, Group{Confidence: f.best[r], Pair: -1})
		}
		out.Groups[k].Songs = append(out.Groups[k].Songs, i)
		out.Songs++
	}
	for k := range out.Groups {
		g := &out.Groups[k]
		ulids := make([]string, len(g.Songs))
		for n, i := range g.Songs {
			ulids[n] = f.songs[i].ULID
		}
		g.Key = Key(ulids)
	}
	out.Pairs = pairs(f.songs, out.Groups)
	return out
}

// Key names a group of songs by their public IDs, whatever their order: a
// third copy turning up gives the group another key.
func Key(ulids []string) string {
	sorted := slices.Sorted(slices.Values(ulids))
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:keyBytes])
}

// PairKey names an album pair by its two albums' public IDs, in either order.
func PairKey(a, b string) string {
	return Key([]string{"albums", a, b})
}

// albums is two albums by public ID, the lesser first.
type albums struct{ a, b string }

// pairs finds the album pairs among groups, and marks each group's pair.
func pairs(songs []Song, groups []Group) []AlbumPair {
	count, order := pairCounts(songs, groups)
	var out []AlbumPair
	for _, key := range order {
		gs := count[key]
		if len(gs) < PairGroups {
			continue
		}
		var mine []int
		for _, k := range gs {
			if groups[k].Pair < 0 { // A group pairing more than two albums belongs to its first pair.
				groups[k].Pair = len(out)
				mine = append(mine, k)
			}
		}
		if len(mine) < PairGroups {
			for _, k := range mine {
				groups[k].Pair = -1
			}
			continue
		}
		p := AlbumPair{Key: PairKey(key.a, key.b), Groups: mine}
		p.Albums = firstAlbums(songs, groups, mine, key.a, key.b)
		p.Only = only(songs, groups, &p)
		out = append(out, p)
	}
	return out
}

// pairCounts returns the groups that pair each two albums, and the pairs of
// albums in the order their first groups come.
func pairCounts(songs []Song, groups []Group) (map[albums][]int, []albums) {
	count := map[albums][]int{}
	var order []albums
	for k := range groups {
		seen := albumsOf(songs, groups[k].Songs)
		for x := range seen {
			for _, y := range seen[x+1:] {
				key := albums{min(seen[x], y), max(seen[x], y)}
				if _, ok := count[key]; !ok {
					order = append(order, key)
				}
				count[key] = append(count[key], k)
			}
		}
	}
	return count, order
}

// albumsOf returns the albums of the songs at idx, each once, in order.
func albumsOf(songs []Song, idx []int) []string {
	var out []string
	for _, i := range idx {
		if a := songs[i].Album; !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	return out
}

// firstAlbums returns a and b in the order their songs first appear in the
// pair's groups.
func firstAlbums(songs []Song, groups []Group, mine []int, a, b string) [2]string {
	for _, k := range mine {
		for _, i := range groups[k].Songs {
			switch songs[i].Album {
			case a:
				return [2]string{a, b}
			case b:
				return [2]string{b, a}
			}
		}
	}
	return [2]string{a, b}
}

// only returns each of p's albums' songs that none of p's groups pairs with
// the other album.
func only(songs []Song, groups []Group, p *AlbumPair) [2][]int {
	matched := map[int]bool{}
	for _, k := range p.Groups {
		for _, i := range groups[k].Songs {
			matched[i] = true
		}
	}
	var out [2][]int
	for i := range songs {
		for side, a := range p.Albums {
			if songs[i].Album == a && !matched[i] {
				out[side] = append(out[side], i)
			}
		}
	}
	return out
}

// VersionWords are the words that make a title another version of a song: one
// in a title and not in the other keeps two songs apart. "Searchlights" and
// "Searchlights (Live)" are two recordings.
//
//nolint:gochecknoglobals // A fixed list, never written: the one place it is said.
var VersionWords = []string{"live", "acoustic", "demo", "instrumental", "remix", "edit", "mix", "version", "reimagined"}

// remasterWords mark a trailing note that says only that a recording was
// remastered: "(Remastered 2011)", "(2011 Remaster)", "- Remastered".
//
//nolint:gochecknoglobals // As VersionWords.
var remasterWords = []string{"remaster", "remastered"}

// baseTitle is a folded title as Find compares it.
type baseTitle struct {
	text     string // Without its remaster notes.
	versions uint16 // Which version words it holds, one bit each.
}

// parseTitle reads a folded title: it takes off trailing remaster notes, then
// notes which version words the rest holds.
func parseTitle(folded string) baseTitle {
	t := strings.TrimSpace(folded)
	for {
		next := dropRemaster(t)
		if next == t {
			break
		}
		t = next
	}
	var bt baseTitle
	bt.text = strings.Join(strings.Fields(t), " ")
	for _, w := range words(bt.text) {
		if n := slices.Index(VersionWords, w); n >= 0 {
			bt.versions |= 1 << n
		}
	}
	return bt
}

// dropRemaster returns t without its trailing remaster note, or t when it has
// none: a last bracketed note, or a note after " - ", that holds a remaster
// word.
func dropRemaster(t string) string {
	t = strings.TrimSpace(t)
	if n := len(t); n > 0 && (t[n-1] == ')' || t[n-1] == ']') {
		open := strings.LastIndexAny(t, "([")
		if open > 0 && hasRemaster(t[open+1:n-1]) {
			return strings.TrimSpace(t[:open])
		}
	}
	if dash := strings.LastIndex(t, " - "); dash > 0 && hasRemaster(t[dash+3:]) {
		return strings.TrimSpace(t[:dash])
	}
	return t
}

// hasRemaster reports whether note holds a remaster word.
func hasRemaster(note string) bool {
	return slices.ContainsFunc(words(note), func(w string) bool { return slices.Contains(remasterWords, w) })
}

// words splits s into its runs of letters and digits.
func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}
