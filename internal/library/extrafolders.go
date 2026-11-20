package library

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"github.com/simonhull/kitsune/internal/store"
)

// folder is a folder under a library root.
type folder struct {
	root, dir string // dir is relative to root, "." for the root itself.
}

// albumDir is the folder of a track's file with a disc folder folded into its
// parent, as grouping has it.
func albumDir(rel string) string {
	dir := filepath.Dir(rel)
	if parent, ok := discParent(dir); ok {
		return parent
	}
	return dir
}

// albumFolders returns each album's folder: the one folder every live track
// of it is in, disc folders folded into their parent. An album whose songs lie
// in more than one has none, so no folder speaks for it.
func albumFolders(tracks []store.LayoutTrack) map[int64]folder {
	out := map[int64]folder{}
	split := map[int64]bool{}
	for i := range tracks {
		t := &tracks[i]
		f := folder{root: t.Root, dir: albumDir(t.RelPath)}
		if have, ok := out[t.AlbumID]; ok && have != f {
			split[t.AlbumID] = true
		}
		out[t.AlbumID] = f
	}
	for id := range split {
		delete(out, id)
	}
	return out
}

// soleOwner returns the album's one owner, and false when it has none (a
// compilation, Various Artists) or several.
func soleOwner(l *store.Layout, albumID int64) (store.LayoutArtist, bool) {
	owners := l.Owners[albumID]
	if len(owners) != 1 {
		return store.LayoutArtist{}, false
	}
	return owners[0], true
}

// nobody marks a folder two artists claim, or one holding an album its artist
// doesn't own alone.
const nobody int64 = 0

// artistFolders returns each artist's folders, in roots' order, then by path:
// the folder above one of its albums' folders, when every live track in or
// below it is on an album that artist alone owns. A folder holding another
// artist's album, a compilation's or a shared one is nobody's, so one artist
// never takes another's images. The library's root can be one, for a library
// of one artist.
func artistFolders(l *store.Layout, albums map[int64]folder, roots []string) map[int64][]folder {
	claims := claimFolders(l, albums)
	dropMixed(l, claims)
	order := make(map[string]int, len(roots))
	for i, r := range roots {
		order[r] = i
	}
	out := map[int64][]folder{}
	for f, artist := range claims {
		if artist != nobody {
			out[artist] = append(out[artist], f)
		}
	}
	for _, fs := range out {
		slices.SortFunc(fs, func(a, b folder) int {
			return cmp.Or(cmp.Compare(order[a.root], order[b.root]), strings.Compare(a.root, b.root), strings.Compare(a.dir, b.dir))
		})
	}
	return out
}

// claimFolders has each album's sole owner claim the folder above the album's.
func claimFolders(l *store.Layout, albums map[int64]folder) map[folder]int64 {
	claims := map[folder]int64{}
	for albumID, f := range albums {
		owner, ok := soleOwner(l, albumID)
		if !ok || f.dir == "." {
			continue
		}
		up := folder{root: f.root, dir: filepath.Dir(f.dir)}
		if have, seen := claims[up]; seen && have != owner.ID {
			claims[up] = nobody
			continue
		}
		claims[up] = owner.ID
	}
	return claims
}

// dropMixed takes away every claim on a folder holding, in it or below, a
// track of an album its artist doesn't own alone.
func dropMixed(l *store.Layout, claims map[folder]int64) {
	for i := range l.Tracks {
		t := &l.Tracks[i]
		owner, sole := soleOwner(l, t.AlbumID)
		for dir := filepath.Dir(t.RelPath); ; dir = filepath.Dir(dir) {
			f := folder{root: t.Root, dir: dir}
			if artist, ok := claims[f]; ok && artist != nobody && (!sole || owner.ID != artist) {
				claims[f] = nobody
			}
			if dir == "." {
				break
			}
		}
	}
}
