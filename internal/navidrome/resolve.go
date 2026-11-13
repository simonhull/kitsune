package navidrome

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
	"github.com/simonhull/kitsune/internal/store/navidromedb"
)

// Sentinels for an option that names nothing, which is a typo rather than
// something to import around.
var (
	// ErrMapFrom means a -map OLD is no Navidrome song's path.
	ErrMapFrom = errors.New("no Navidrome song has that path")
	// ErrMapTo means a -map NEW is no Kitsune track's path.
	ErrMapTo = errors.New("no Kitsune track has that path")
	// ErrRoot means a -root names no Navidrome library.
	ErrRoot = errors.New("no Navidrome library has that path")
)

// how says how a Navidrome song found its Kitsune track.
type how uint8

const (
	unmatched how = iota
	byPath        // A song, by its path.
	byName        // An artist, by its match key.
	byMBID        // Either, by a MusicBrainz ID.
)

// match is a Navidrome song's Kitsune track, if it has one.
type match struct {
	track, album string // ULIDs; empty when unmatched.
	how          how
}

// resolver finds Kitsune's tracks and artists for Navidrome's.
type resolver struct {
	tracks   map[pathKey]*store.ImportTrack
	release  map[string][]*store.ImportTrack // By lower-case release-track MBID.
	record   map[string][]*store.ImportTrack // By lower-case recording MBID.
	byKey    map[string]string               // Artist ULIDs by match key.
	byArtist map[string][]string             // Artist ULIDs by lower-case MBID.
}

// pathKey is a file's place: a root and a path relative to it.
type pathKey struct{ root, rel string }

func newResolver(t *store.ImportTargets) *resolver {
	r := &resolver{
		tracks:   make(map[pathKey]*store.ImportTrack, len(t.Tracks)),
		release:  map[string][]*store.ImportTrack{},
		record:   map[string][]*store.ImportTrack{},
		byKey:    make(map[string]string, len(t.Artists)),
		byArtist: map[string][]string{},
	}
	for i := range t.Tracks {
		tr := &t.Tracks[i]
		r.tracks[pathKey{tr.Root, tr.RelPath}] = tr
		for _, id := range lowerSet(tr.ReleaseTrackMBIDs) {
			r.release[id] = append(r.release[id], tr)
		}
		for _, id := range lowerSet(tr.RecordingMBIDs) {
			r.record[id] = append(r.record[id], tr)
		}
	}
	for _, a := range t.Artists {
		r.byKey[a.MatchKey] = a.ULID
		if a.MBID != "" {
			id := strings.ToLower(a.MBID)
			r.byArtist[id] = append(r.byArtist[id], a.ULID)
		}
	}
	return r
}

// lowerSet returns the non-empty IDs lower-cased, each once.
func lowerSet(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.ToLower(strings.TrimSpace(id)); id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// resolveSongs finds every Navidrome song's Kitsune track: by path under the
// paired root, after -map; else by release-track MBID, then recording MBID,
// each only when exactly one Navidrome song and one Kitsune track carry it.
func (r *resolver) resolveSongs(src *Source, opts *Options) (map[string]match, error) {
	roots, err := pairRoots(src.Libraries, opts.Roots)
	if err != nil {
		return nil, err
	}
	if mapErr := r.checkMap(src, roots, opts.Paths); mapErr != nil {
		return nil, mapErr
	}
	ndRelease := map[string]int{}
	ndRecord := map[string]int{}
	for i := range src.Songs {
		ndRelease[strings.ToLower(strings.TrimSpace(src.Songs[i].ReleaseTrackMBID))]++
		ndRecord[strings.ToLower(strings.TrimSpace(src.Songs[i].RecordingMBID))]++
	}
	out := make(map[string]match, len(src.Songs))
	for i := range src.Songs {
		s := &src.Songs[i]
		rel := s.Path
		if to, ok := opts.Paths[s.Path]; ok {
			rel = to
		}
		t, h := r.tracks[pathKey{roots[s.LibraryID], rel}], byPath
		if t == nil {
			t, h = cmp.Or(unique(r.release, ndRelease, s.ReleaseTrackMBID), unique(r.record, ndRecord, s.RecordingMBID)), byMBID
		}
		if t != nil {
			out[s.ID] = match{track: t.ULID, album: t.Album, how: h}
		}
	}
	return out, nil
}

// unique returns the one Kitsune track carrying id, when exactly one
// Navidrome song carries it too.
func unique(kitsune map[string][]*store.ImportTrack, navidrome map[string]int, id string) *store.ImportTrack {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" || navidrome[id] != 1 || len(kitsune[id]) != 1 {
		return nil
	}
	return kitsune[id][0]
}

// pairRoots gives each Navidrome library its Kitsune root: the same path,
// unless -root says otherwise.
func pairRoots(libs []navidromedb.Library, override map[string]string) (map[int64]string, error) {
	roots := make(map[int64]string, len(libs))
	used := map[string]bool{}
	for _, l := range libs {
		roots[l.ID] = l.Path
		if to, ok := override[l.Path]; ok {
			roots[l.ID] = to
			used[l.Path] = true
		}
	}
	for _, from := range slices.Sorted(maps.Keys(override)) {
		if !used[from] {
			return nil, fmt.Errorf("-root %s: %w", from, ErrRoot)
		}
	}
	return roots, nil
}

// checkMap checks that every -map names a Navidrome song and a Kitsune track
// under that song's root.
func (r *resolver) checkMap(src *Source, roots map[int64]string, paths map[string]string) error {
	songs := make(map[string]int64, len(src.Songs))
	for i := range src.Songs {
		songs[src.Songs[i].Path] = src.Songs[i].LibraryID
	}
	var errs []error
	for _, from := range slices.Sorted(maps.Keys(paths)) {
		lib, ok := songs[from]
		if !ok {
			errs = append(errs, fmt.Errorf("-map %s: %w", from, ErrMapFrom))
			continue
		}
		if _, ok = r.tracks[pathKey{roots[lib], paths[from]}]; !ok {
			errs = append(errs, fmt.Errorf("-map to %s: %w", paths[from], ErrMapTo))
		}
	}
	return errors.Join(errs...)
}

// artist says how a Navidrome artist matches a Kitsune artist: by match key, else by an
// MBID exactly one Kitsune artist has. Various Artists is never among them.
func (r *resolver) artist(a *navidromedb.Artist) how {
	if _, ok := r.byKey[library.MatchKey(a.Name)]; ok {
		return byName
	}
	if ids := r.byArtist[strings.ToLower(strings.TrimSpace(a.MBID))]; a.MBID != "" && len(ids) == 1 {
		return byMBID
	}
	return unmatched
}
