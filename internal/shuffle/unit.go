package shuffle

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// TrackID is a song's id in the store.
type TrackID int64

// ArtistID is an artist's id in the store.
type ArtistID int64

// AlbumID is an album's id in the store.
type AlbumID int64

// Stars is a person's rating of a song in half stars, from 1 (½★) to
// [MaxStars] (5★), or [Unrated].
type Stars uint8

const (
	// Unrated is the rating of a song the person hasn't rated.
	Unrated Stars = 0
	// MaxStars is the highest rating, 5★, in half stars.
	MaxStars Stars = 10
	// halvesPerStar is how many of [Stars] make a star.
	halvesPerStar = 2
)

var (
	// ErrBadTrack means a track's fields can't be true: a rating over
	// [MaxStars], stars by heart without stars or a heart, a negative count,
	// more skips than starts, or more heard than listened.
	ErrBadTrack = errors.New("bad track")
	// ErrDuplicateTrack means two tracks share an id.
	ErrDuplicateTrack = errors.New("duplicate track")
	// ErrRunOverlap means a song is in two runs, or twice in one: a cycle.
	ErrRunOverlap = errors.New("song in more than one run")
)

// Track is one song, as shuffle sees it.
type Track struct {
	// ID is the song's id.
	ID TrackID
	// Artists are the song's own artists, the first one primary. On a
	// compilation they are the song's artists, not Various Artists.
	Artists []ArtistID
	// Album is the song's album.
	Album AlbumID
	// Stars is the person's rating, in half stars.
	Stars Stars
	// ByHeart is whether Stars come from the song's heart, the person not
	// having rated it: they weigh as any rating, and say where they came from.
	// It needs Stars and Hearted.
	ByHeart bool
	// Hearted is whether the person hearted the song.
	Hearted bool
	// Skipped is whether the song is skipped in shuffle: it never comes up
	// on its own, but still plays inside its run.
	Skipped bool
	// History is what the person's listening says about the song.
	History History
}

// History is what a person's listening says about one song. The caller
// counts; shuffle smooths and weighs.
type History struct {
	// LastPlayed is when the song last came up for the person, played or
	// skipped; zero for never. A skip counts: on the bench, a song skipped
	// yesterday that recency took for unheard came back sooner than one
	// played through.
	LastPlayed time.Time
	// EarlySkips counts the shuffle starts the person skipped within
	// [EarlySkip] of the start, during the last [SkipMemoryFor].
	EarlySkips int
	// ShuffleStarts counts the song's starts in shuffle that ended completed
	// or skipped. Stopped and interrupted starts say nothing against a song
	// and are left out.
	ShuffleStarts int
	// ShuffleSkips counts those starts that the person skipped.
	ShuffleSkips int
	// Listens counts the song's listens, in shuffle or not, that ended
	// completed or skipped.
	Listens int
	// Heard sums the fraction of the song heard over those listens, each from
	// 0 to 1.
	Heard float64
}

// valid reports whether the history's counts can be true.
func (h *History) valid() bool {
	return h.EarlySkips >= 0 && h.ShuffleStarts >= 0 && h.Listens >= 0 &&
		h.ShuffleSkips >= 0 && h.ShuffleSkips <= h.ShuffleStarts &&
		h.Heard >= 0 && h.Heard <= float64(h.Listens)
}

// Run is a person's ordered list of songs that always play together, from
// the first.
type Run []TrackID

// Unit is what shuffle picks: a run, or a single song.
type Unit struct {
	tracks  []TrackID
	artists []ArtistID // every member's, deduplicated; the first song's primary first
	albums  []AlbumID  // every member's, deduplicated
	artist  []int32    // artists' indexes in the pool
	album   []int32    // albums' indexes in the pool
	facts   facts
}

// facts are a unit's signals, combined over its songs.
type facts struct {
	halfSum    int     // the sum of the rated members' ratings, in half stars
	rated      int     // how many members are rated
	byHeart    int     // how many of those ratings come from a heart
	hearted    bool    // any member is hearted
	lastPlayed int64   // the latest member play, Unix ms; 0 for never
	earlySkips int     // the most early skips of any member
	starts     int     // shuffle starts over all members
	skips      int     // shuffle skips over all members
	listens    int     // listens over all members
	heard      float64 // fractions heard over all members
}

// ID is the unit's id: its first song's.
func (u *Unit) ID() TrackID { return u.tracks[0] }

// Tracks are the unit's songs, in play order.
func (u *Unit) Tracks() []TrackID { return slices.Clone(u.tracks) }

// Len is how many songs the unit plays.
func (u *Unit) Len() int { return len(u.tracks) }

// Artists are the unit's artists, its first song's primary artist first.
func (u *Unit) Artists() []ArtistID { return slices.Clone(u.artists) }

// Albums are the unit's albums.
func (u *Unit) Albums() []AlbumID { return slices.Clone(u.albums) }

// primary is the unit's primary artist, or false for a unit without one.
func (u *Unit) primary() (ArtistID, bool) {
	if len(u.artists) == 0 {
		return 0, false
	}
	return u.artists[0], true
}

// Pool is the units of one shuffle's scope, with what spacing needs to know
// about them. Build it with [NewPool]; it is never changed after.
type Pool struct {
	units     []Unit
	solo      map[TrackID]Unit // songs skipped in shuffle outside any run, to lead when chosen
	artistGap []int32          // by artist index: how far apart its units must be
	albumGap  []int32          // by album index
	artists   int              // how many artists the units have between them
}

// NewPool builds the units of tracks, linked by runs. A run's songs missing
// from tracks are left out of it, and a run with none left is dropped. A
// song skipped in shuffle and in no run is left out, and so is a run whose
// songs are all skipped.
//
// # Errors
//
// [ErrBadTrack] for a track whose fields can't be true, [ErrDuplicateTrack]
// for two tracks with one id, and [ErrRunOverlap] for a song in two runs or
// twice in one.
func NewPool(tracks []Track, runs []Run) (*Pool, error) {
	byID, err := indexTracks(tracks)
	if err != nil {
		return nil, err
	}
	inRun, err := runSongs(runs)
	if err != nil {
		return nil, err
	}
	b := newBuilder(len(tracks))
	p := &Pool{units: b.runs(tracks, runs, byID), solo: make(map[TrackID]Unit)}
	for i := range tracks {
		switch t := &tracks[i]; {
		case inRun[t.ID]:
		case t.Skipped:
			p.solo[t.ID] = b.unit([]*Track{t})
		default:
			p.units = append(p.units, b.unit([]*Track{t}))
		}
	}
	p.index(b)
	return p, nil
}

// indexTracks checks tracks and maps each id to its index.
func indexTracks(tracks []Track) (map[TrackID]int, error) {
	byID := make(map[TrackID]int, len(tracks))
	for i := range tracks {
		t := &tracks[i]
		if t.Stars > MaxStars || t.ByHeart && (t.Stars == Unrated || !t.Hearted) || !t.History.valid() {
			return nil, fmt.Errorf("track %d: %w", t.ID, ErrBadTrack)
		}
		if _, dup := byID[t.ID]; dup {
			return nil, fmt.Errorf("track %d: %w", t.ID, ErrDuplicateTrack)
		}
		byID[t.ID] = i
	}
	return byID, nil
}

// runSongs checks that no song is in two runs, or twice in one, and returns
// the songs that are in one.
func runSongs(runs []Run) (map[TrackID]bool, error) {
	inRun := make(map[TrackID]bool)
	for _, run := range runs {
		for _, id := range run {
			if inRun[id] {
				return nil, fmt.Errorf("track %d: %w", id, ErrRunOverlap)
			}
			inRun[id] = true
		}
	}
	return inRun, nil
}

// Len is how many units the pool holds.
func (p *Pool) Len() int { return len(p.units) }

// Units are the pool's units.
func (p *Pool) Units() []Unit { return slices.Clone(p.units) }

// index gives every artist and album a dense index and works out how far
// apart each one's units must be.
func (p *Pool) index(b *builder) {
	artists := make(map[ArtistID]int32)
	albums := make(map[AlbumID]int32)
	var artistUnits, albumUnits []int32
	for i := range p.units {
		u := &p.units[i]
		u.artist = b.indexes(len(u.artists))
		for j, a := range u.artists {
			u.artist[j] = intern(artists, &artistUnits, a)
		}
		u.album = b.indexes(len(u.albums))
		for j, a := range u.albums {
			u.album[j] = intern(albums, &albumUnits, a)
		}
	}
	n := len(p.units)
	p.artists = len(artistUnits)
	p.artistGap = gaps(artistUnits, ArtistGap, n)
	p.albumGap = gaps(albumUnits, AlbumGap, n)
}

// intern returns key's index in index, adding it if new, and counts one more
// unit for it.
func intern[K comparable](index map[K]int32, counts *[]int32, key K) int32 {
	i, ok := index[key]
	if !ok {
		i = int32(len(*counts)) //nolint:gosec // G115: far fewer than 2^31 artists or albums.
		index[key] = i
		*counts = append(*counts, 0)
	}
	(*counts)[i]++
	return i
}

// lead returns the unit holding first: its index in the pool, or -1 and the
// unit of a song skipped in shuffle, outside any run, that the person chose
// to start from. It returns -1 and nil when no song is first. It scans the
// pool: a shuffle asks once, and a map for it would cost every pool built.
func (p *Pool) lead(first TrackID) (int, *Unit) {
	if first == 0 {
		return -1, nil
	}
	for i := range p.units {
		if slices.Contains(p.units[i].tracks, first) {
			return i, nil
		}
	}
	if u, ok := p.solo[first]; ok {
		return -1, &u
	}
	return -1, nil
}

// builder makes a pool's units, their slices cut from a few large ones, so
// a pool of 50,000 songs takes a handful of allocations, not a quarter of a
// million.
type builder struct {
	tracks  []TrackID
	artists []ArtistID
	albums  []AlbumID
	idx     []int32
}

// newBuilder is a builder for about n songs.
func newBuilder(n int) *builder {
	return &builder{
		tracks:  make([]TrackID, 0, n),
		artists: make([]ArtistID, 0, n+n/4), // some songs have more than one artist
		albums:  make([]AlbumID, 0, n),
		idx:     make([]int32, 0, 2*n+n/4),
	}
}

// cut returns the part of *buf after from, capped so nothing appended to it
// later can reach into what follows.
func cut[T any](buf []T, from int) []T { return buf[from:len(buf):len(buf)] }

// indexes returns n indexes to fill in.
func (b *builder) indexes(n int) []int32 {
	from := len(b.idx)
	b.idx = slices.Grow(b.idx, n)[:from+n]
	return cut(b.idx, from)
}

// runs returns, with room for the rest of tracks' units, a unit for each
// run with a song not skipped in shuffle.
func (b *builder) runs(tracks []Track, runs []Run, byID map[TrackID]int) []Unit {
	units := make([]Unit, 0, len(tracks))
	members := make([]*Track, 0, 1)
	for _, run := range runs {
		members = members[:0]
		for _, id := range run {
			if i, ok := byID[id]; ok {
				members = append(members, &tracks[i])
			}
		}
		if slices.ContainsFunc(members, func(t *Track) bool { return !t.Skipped }) {
			units = append(units, b.unit(members))
		}
	}
	return units
}

// unit combines members, in play order, into a unit.
func (b *builder) unit(members []*Track) Unit {
	t0, a0, al0 := len(b.tracks), len(b.artists), len(b.albums)
	var f facts
	for _, t := range members {
		b.tracks = append(b.tracks, t.ID)
		for _, a := range t.Artists {
			if !slices.Contains(b.artists[a0:], a) {
				b.artists = append(b.artists, a)
			}
		}
		if !slices.Contains(b.albums[al0:], t.Album) {
			b.albums = append(b.albums, t.Album)
		}
		f.add(t)
	}
	return Unit{tracks: cut(b.tracks, t0), artists: cut(b.artists, a0), albums: cut(b.albums, al0), facts: f}
}

// add counts t's signals into f.
func (f *facts) add(t *Track) {
	if t.Stars != Unrated {
		f.halfSum += int(t.Stars)
		f.rated++
		if t.ByHeart {
			f.byHeart++
		}
	}
	f.hearted = f.hearted || t.Hearted
	if !t.History.LastPlayed.IsZero() {
		f.lastPlayed = max(f.lastPlayed, t.History.LastPlayed.UnixMilli())
	}
	f.earlySkips = max(f.earlySkips, t.History.EarlySkips)
	f.starts += t.History.ShuffleStarts
	f.skips += t.History.ShuffleSkips
	f.listens += t.History.Listens
	f.heard += t.History.Heard
}
