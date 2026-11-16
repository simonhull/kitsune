package library

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/simonhull/kitsune/internal/organize"
	"github.com/simonhull/kitsune/internal/store"
)

// OrganizeConfig is how the organizer files songs: the settings organize.*.
type OrganizeConfig struct {
	organize.Templates

	// Roots holds the templates of library roots that don't use the main pair.
	Roots map[string]organize.Templates
	// Companions brings covers, cue sheets, logs, lyrics and notes along.
	Companions bool
	// OnEdit moves a song after a tag edit, when it was where the templates put it.
	OnEdit bool
	// RewritePlaylists rewrites the lines of .m3u files that name a moved song.
	RewritePlaylists bool
}

// DefaultOrganize is the organizer's configuration before any setting.
func DefaultOrganize() OrganizeConfig {
	main, err := organize.Parse(organize.DefaultTemplate)
	if err != nil {
		panic("library: the default template doesn't parse: " + err.Error())
	}
	comp, err := organize.Parse(organize.DefaultCompilations)
	if err != nil {
		panic("library: the default compilations template doesn't parse: " + err.Error())
	}
	return OrganizeConfig{Template: main, Compilations: comp, Companions: true}
}

// SetOrganize makes c how the organizer files songs. It doesn't wait: a run
// keeps the templates its preview had.
func (s *Scanner) SetOrganize(c *OrganizeConfig) {
	cp := *c
	s.organizeCfg.Store(&cp)
}

// Organize returns how the organizer files songs now. Don't change it.
func (s *Scanner) Organize() *OrganizeConfig { return s.organizeCfg.Load() }

// OrganizeScope names the songs a preview is of: songs, or every song of some
// albums, by public ID. Neither is the whole library.
type OrganizeScope struct {
	Tracks, Albums []string
}

// whole reports whether the scope is the whole library.
func (sc *OrganizeScope) whole() bool { return len(sc.Tracks) == 0 && len(sc.Albums) == 0 }

// OrganizePreview is what organizing some songs would do. Every path in it is
// relative to its song's root, slash-separated.
type OrganizePreview struct {
	Templates  organize.Templates
	Companions bool
	Scoped     bool // Some songs, not the whole library.
	// Songs are the songs previewed, in the Library's order, and Plan what
	// would happen to each, at the same index.
	Songs []store.OrganizeSong
	Plan  organize.Plan
	// Genres are each song's album's first display genre, at the same index.
	Genres []string
}

// PreviewOrganize works out where c's templates file the songs of scope, and
// what is in their way, from what the store keeps of every song's tags: no
// audio file is read. On disk it lists only the folders songs would land in,
// for files Kitsune doesn't know, and with companions the folders they leave.
// It holds no lock: a run checks each song again before moving it.
//
// It returns an error wrapping [store.ErrNotFound] when an ID of scope names no
// song or album in the library.
func (s *Scanner) PreviewOrganize(ctx context.Context, c *OrganizeConfig, scope OrganizeScope) (*OrganizePreview, error) {
	lib, err := s.st.OrganizeLibrary(ctx)
	if err != nil {
		return nil, fmt.Errorf("preview: %w", err)
	}
	roots := s.live().Roots
	in, err := inScope(lib.Songs, scope, roots)
	if err != nil {
		return nil, err
	}
	p := &OrganizePreview{Templates: c.Templates, Companions: c.Companions, Scoped: !scope.whole()}
	inputs := make([]organize.Input, 0, len(lib.Songs))
	others := make([]organize.Item, 0, len(lib.Missing)+len(lib.Songs))
	for i := range lib.Songs {
		song := &lib.Songs[i]
		item := organize.Item{Key: song.ID, Root: song.Root, Path: filepath.ToSlash(song.RelPath)}
		if !in[i] {
			others = append(others, item)
			continue
		}
		genre := firstOf(s.genres.Of(song.Album))
		p.Songs = append(p.Songs, *song)
		p.Genres = append(p.Genres, genre)
		inputs = append(inputs, organize.Input{Item: item, Song: templateSong(song, genre)})
	}
	for _, m := range lib.Missing {
		others = append(others, organize.Item{Key: m.ID, Root: m.Root, Path: filepath.ToSlash(m.RelPath)})
	}
	lister := &folderLister{roots: openRoots{}}
	defer lister.roots.close()
	p.Plan = organize.MakePlan(inputs, &organize.Options{
		Templates: c.Templates, Roots: c.Roots, Companions: c.Companions, Others: others, List: lister.list,
	})
	return p, nil
}

// inScope marks which of songs scope holds, among those under a configured
// root. It returns an error wrapping [store.ErrNotFound] for an ID that names
// none.
func inScope(songs []store.OrganizeSong, scope OrganizeScope, roots []string) ([]bool, error) {
	in := make([]bool, len(songs))
	tracks := setOf(scope.Tracks)
	albums := setOf(scope.Albums)
	foundTracks, foundAlbums := map[string]bool{}, map[string]bool{}
	for i := range songs {
		song := &songs[i]
		if !slices.Contains(roots, song.Root) {
			continue // A root gone from the settings: the next scan marks its songs missing.
		}
		switch {
		case scope.whole():
			in[i] = true
		case tracks[song.ULID]:
			in[i], foundTracks[song.ULID] = true, true
		case albums[song.Album]:
			in[i], foundAlbums[song.Album] = true, true
		}
	}
	if len(foundTracks) != len(tracks) || len(foundAlbums) != len(albums) {
		return nil, fmt.Errorf("preview: %w", store.ErrNotFound)
	}
	return in, nil
}

// setOf returns ids as a set.
func setOf(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// firstOf returns the first of list, or "".
func firstOf(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

// templateSong is what a template reads of song, whose album's first display
// genre is genre.
func templateSong(song *store.OrganizeSong, genre string) organize.Song {
	return organize.Song{
		Title: song.Title, Album: song.AlbumTitle, Genre: genre, DiscName: song.DiscName, Format: song.Format,
		AlbumArtist: song.AlbumArtist, Artists: song.Artists, Year: song.Year, OriginalYear: song.OriginalYear,
		Disc: song.Disc, Discs: song.Discs, Track: song.Track, DiscSongs: song.DiscSongs, Compilation: song.Compilation,
	}
}

// TemplateSong is what a template reads of song: [organize.Song] from the
// store's row, with its album's first display genre.
func (s *Scanner) TemplateSong(song *store.OrganizeSong) organize.Song {
	return templateSong(song, firstOf(s.genres.Of(song.Album)))
}

// OrganizeLibrary returns every live song as the organizer reads it, in the
// Library's order.
func (s *Scanner) OrganizeLibrary(ctx context.Context) (*store.OrganizeLibrary, error) {
	lib, err := s.st.OrganizeLibrary(ctx)
	if err != nil {
		return nil, fmt.Errorf("organize library: %w", err)
	}
	return lib, nil
}

// folderLister lists folders under the roots for a plan, through [os.Root],
// one read per folder: the plan asks each once.
type folderLister struct {
	roots openRoots
}

// list returns the names in dir, slash-separated, under root, or none when it
// can't be read: a folder that isn't there holds nothing in the way.
func (l *folderLister) list(root, dir string) []organize.Entry {
	entries, err := l.roots.readDir(root, filepath.FromSlash(dir))
	if err != nil {
		return nil
	}
	out := make([]organize.Entry, len(entries))
	for i, e := range entries {
		out[i] = organize.Entry{Name: e.Name(), Dir: e.IsDir()}
	}
	return out
}
