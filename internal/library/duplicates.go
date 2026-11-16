package library

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/simonhull/kitsune/internal/duplicates"
	"github.com/simonhull/kitsune/internal/store"
)

// Duplicates are copies of one recording, found from what the store keeps of
// every song: no audio file is read. A find is kept until the next library
// write or fingerprint stored, as the index's rows are, so opening the page
// again costs nothing.
type Duplicates struct {
	duplicates.Result

	// Songs are every live song, in the Library's order: what the find was
	// given, so a group's songs index them.
	Songs  []store.DuplicateSong
	Albums map[int64]*store.DuplicateAlbum
	// Original says, at the same index, whether the song is on an original
	// album: one its own first artist released, not a compilation, and not
	// tagged a compilation or a soundtrack.
	Original []bool
	// Checked counts the songs the fingerprint pass has read: only those can
	// be in a group by the same audio.
	Checked int

	groupOf map[string]int // Group index, by key.
	pairOf  map[string]int // Pair index, by key.
	songOf  map[string]int // Group index, by song ULID.
	albumOf map[string]int64
}

// Group returns the group with key, and whether there is one.
func (d *Duplicates) Group(key string) (*duplicates.Group, bool) {
	k, ok := d.groupOf[key]
	if !ok {
		return nil, false
	}
	return &d.Groups[k], true
}

// Pair returns the album pair with key, and whether there is one.
func (d *Duplicates) Pair(key string) (*duplicates.AlbumPair, bool) {
	k, ok := d.pairOf[key]
	if !ok {
		return nil, false
	}
	return &d.Pairs[k], true
}

// GroupOf returns the group holding the song with the public ID id, and
// whether one does.
func (d *Duplicates) GroupOf(id string) (*duplicates.Group, bool) {
	k, ok := d.songOf[id]
	if !ok {
		return nil, false
	}
	return &d.Groups[k], true
}

// Album returns the album with the public ID id.
func (d *Duplicates) Album(id string) (*store.DuplicateAlbum, bool) {
	row, ok := d.albumOf[id]
	if !ok {
		return nil, false
	}
	a, ok := d.Albums[row]
	return a, ok
}

// keptDuplicates is a find, with where the writes it read stood.
type keptDuplicates struct {
	mark store.DuplicatesMark
	d    *Duplicates
}

// FindDuplicates finds the library's duplicates, or returns the last find
// when no library write and no fingerprint has landed since.
func (s *Scanner) FindDuplicates(ctx context.Context) (*Duplicates, error) {
	before := s.st.DuplicatesMark()
	if k := s.dupes.Load(); k != nil && before.Settled && k.mark == before {
		return k.d, nil
	}
	lib, err := s.st.DuplicateLibrary(ctx, duplicateTagKeys())
	if err != nil {
		return nil, fmt.Errorf("find duplicates: %w", err)
	}
	d := findDuplicates(lib)
	if after := s.st.DuplicatesMark(); before.Settled && after == before {
		s.dupes.Store(&keptDuplicates{mark: before, d: d})
	}
	return d, nil
}

// duplicateTagKeys are the raw tags finding duplicates reads: the recordings,
// then the release types.
func duplicateTagKeys() []string {
	return slices.Concat(TrackMBIDKeys(), ReleaseTypeKeys())
}

// findDuplicates finds the duplicates of lib.
func findDuplicates(lib *store.DuplicateLibrary) *Duplicates {
	d := &Duplicates{
		Songs: lib.Songs, Albums: lib.Albums, Original: make([]bool, len(lib.Songs)),
		albumOf: make(map[string]int64, len(lib.Albums)),
	}
	for id, a := range lib.Albums {
		d.albumOf[a.ULID] = id
	}
	in := songsIn{d: d, lib: lib, recordings: TrackMBIDKeys(), releaseTypes: ReleaseTypeKeys(), keys: map[string]string{}}
	songs := make([]duplicates.Song, len(lib.Songs))
	for i := range lib.Songs {
		songs[i] = in.song(i)
	}
	d.Result = duplicates.Find(songs)
	d.groupOf = make(map[string]int, len(d.Groups))
	d.songOf = make(map[string]int, d.Result.Songs)
	for k := range d.Groups {
		d.groupOf[d.Groups[k].Key] = k
		for _, i := range d.Groups[k].Songs {
			d.songOf[lib.Songs[i].ULID] = k
		}
	}
	d.pairOf = make(map[string]int, len(d.Pairs))
	for k := range d.Pairs {
		d.pairOf[d.Pairs[k].Key] = k
	}
	return d
}

// songsIn turns a library's songs into what Find reads, noting each one's
// album as original or not, and counting the fingerprinted.
type songsIn struct {
	d                        *Duplicates
	lib                      *store.DuplicateLibrary
	recordings, releaseTypes []string
	keys                     map[string]string // MatchKey of each artist named, once: artists repeat.
}

// song returns song i as Find reads it.
func (in *songsIn) song(i int) duplicates.Song {
	s := &in.lib.Songs[i]
	artist, ok := in.keys[s.Artist]
	if !ok {
		artist = MatchKey(s.Artist)
		in.keys[s.Artist] = artist
	}
	out := duplicates.Song{
		ULID: s.ULID, Artist: artist, Title: s.Folded, Duration: s.Duration, Disc: s.Disc, Digest: string(s.Digest),
	}
	tags := in.lib.Tags[s.ID]
	if a := in.lib.Albums[s.AlbumID]; a != nil {
		out.Album = a.ULID
		in.d.Original[i] = original(a, artist, tags, in.releaseTypes)
	}
	for _, t := range tags {
		if slices.Contains(in.recordings, t.Key) {
			out.Recordings = append(out.Recordings, strings.ToLower(strings.TrimSpace(t.Value)))
		}
	}
	if s.Digest != nil {
		in.d.Checked++
	}
	return out
}

// notOriginal are the release types that make an album no original one.
//
//nolint:gochecknoglobals // A fixed list, never written.
var notOriginal = []string{"compilation", "soundtrack"}

// original reports whether a song by artist (a match key), with tags, on
// album a is on an original album: one the song's own artist released, that
// isn't a compilation and isn't tagged a compilation or a soundtrack.
func original(a *store.DuplicateAlbum, artist string, tags []store.Tag, releaseTypes []string) bool {
	if a.Compilation || a.Owner == "" || a.Owner != artist {
		return false
	}
	for _, t := range tags {
		if !slices.Contains(releaseTypes, t.Key) {
			continue
		}
		for part := range strings.SplitSeq(t.Value, ";") {
			if slices.Contains(notOriginal, strings.ToLower(strings.TrimSpace(part))) {
				return false
			}
		}
	}
	return true
}

// InPlace reports, for each of the live songs ids, whether it sits where the
// organizer's saved templates put it, by row. A song that isn't live, or that
// the templates can't file, is not in place.
func (s *Scanner) InPlace(ctx context.Context, ids []int64) (map[int64]bool, error) {
	lib, err := s.st.OrganizeLibrary(ctx)
	if err != nil {
		return nil, fmt.Errorf("in place: %w", err)
	}
	out := make(map[int64]bool, len(ids))
	for i := range lib.Songs {
		song := &lib.Songs[i]
		if !slices.Contains(ids, song.ID) {
			continue
		}
		to, bad := s.OrganizeTarget(song)
		out[song.ID] = bad == nil && to == strings.ReplaceAll(song.RelPath, "\\", "/")
	}
	return out, nil
}

// MergeResult is what a merge did.
type MergeResult struct {
	store.MergeChange

	Trashed TrashChange
}

// MergeDuplicates keeps the keeper of each merge and trashes its other copies,
// for the user by, holding the library lock: everything people did with those
// copies moves onto the keeper, and then they go to the Trash, in one
// transaction, before any file moves. expect is the merge as its confirm said
// it, as by sees it.
//
// It returns [ErrTagWritesOff] while tag writes are off, [ErrBusy] while a
// scan, an edit or a run holds the library, an error wrapping
// [store.ErrNotFound] when a song isn't live, a [*store.PlanChangedError]
// when the merge would now move something else, and a [*MoveError] when a
// file would not move into the Trash: then every file is back, and the copies
// are back in the library with nothing of their own, since what they had is
// on the keeper.
func (s *Scanner) MergeDuplicates(ctx context.Context, by int64, merges []store.Merge, expect *store.MergePlan) (MergeResult, error) {
	var res MergeResult
	err := s.exclusive(ctx, func() error {
		if !s.live().TagWrites {
			return ErrTagWritesOff
		}
		ids := mergeIDs(merges)
		if _, err := s.st.TrashableTracks(ctx, ids.keep); err != nil {
			return err //nolint:wrapcheck // The store's error already says what failed.
		}
		tracks, err := s.st.TrashableTracks(ctx, ids.trash)
		if err != nil {
			return err //nolint:wrapcheck // As above.
		}
		bg := context.WithoutCancel(ctx) // Once the merge starts, it finishes or is undone, whatever ctx does.
		marks := make([]store.Trashing, len(tracks))
		for i := range tracks {
			marks[i] = store.Trashing{ID: tracks[i].ID, TrashPath: trashPath(tracks[i].ULID, tracks[i].RelPath)}
		}
		change, err := s.st.MergeAndTrash(bg, by, merges, expect, marks, by, s.now())
		if err != nil {
			return err //nolint:wrapcheck // As above.
		}
		res.MergeChange = change
		roots := openRoots{}
		defer roots.close()
		for i := range tracks {
			t := &tracks[i]
			if moveErr := roots.move(t.Root, t.RelPath, marks[i].TrashPath); moveErr != nil {
				s.undoTrash(bg, &roots, tracks[:i], marks)
				s.settleTrash(bg, tracks)
				return &MoveError{Title: t.Title, Err: moveErr}
			}
		}
		s.settleTrash(bg, tracks)
		res.Trashed = changeOf(tracks)
		return nil
	})
	return res, err
}

// mergeTracks are the public IDs of some merges' keepers and of their copies
// to trash.
type mergeTracks struct{ keep, trash []string }

// mergeIDs returns the public IDs of merges' songs.
func mergeIDs(merges []store.Merge) mergeTracks {
	var out mergeTracks
	for i := range merges {
		out.keep = append(out.keep, merges[i].Keep.ULID)
		for _, t := range merges[i].Trash {
			out.trash = append(out.trash, t.ULID)
		}
	}
	return out
}
