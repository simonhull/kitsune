package server

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/library"
)

const (
	// countBudget bounds how long a listing spends counting audio: a folder
	// such as / could hold a whole disk.
	countBudget = 1500 * time.Millisecond
	// maxCountFolders bounds the folders one count opens, whatever the clock.
	maxCountFolders = 20_000
	// maxListed bounds the folders one listing names.
	maxListed = 2000
)

// getFolders lists the folders the server can see under a path, for the folder
// picker: directories only, read through os.Root from /, so neither ".." nor a
// symlink reaches outside it. No file name or size leaves it, only how much
// audio the path holds.
func (s *Server) getFolders(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		p = "/"
	}
	if !strings.HasPrefix(p, "/") || !utf8.ValidString(p) || strings.ContainsRune(p, 0) {
		writeProblem(w, r, http.StatusBadRequest, "path must be an absolute path")
		return
	}
	p = path.Clean(p)
	root, err := os.OpenRoot(s.folderRoot)
	if err != nil {
		s.internalError(w, r, "open the folder root", err)
		return
	}
	defer root.Close() //nolint:errcheck // Read-only.
	listing, err := s.listFolder(r.Context(), root, p)
	if err != nil {
		writeProblem(w, r, http.StatusNotFound, "no folder the server can read there")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, &listing)
}

// rel is p, an absolute path under root, as os.Root takes it.
func rel(p string) string {
	if p == "/" {
		return "."
	}
	return strings.TrimPrefix(p, "/")
}

// listFolder lists the folders in p.
func (s *Server) listFolder(ctx context.Context, root *os.Root, p string) (api.FolderListing, error) {
	dir, err := root.Open(rel(p))
	if err != nil {
		return api.FolderListing{}, err //nolint:wrapcheck // The caller only says there's no folder.
	}
	defer dir.Close() //nolint:errcheck // Read-only.
	info, err := dir.Stat()
	if err != nil || !info.IsDir() {
		return api.FolderListing{}, errors.Join(err, os.ErrNotExist)
	}
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return api.FolderListing{}, err //nolint:wrapcheck // As above.
	}
	roots := s.settings.Snapshot().Roots
	listing := api.FolderListing{
		Path: p, Kind: s.folderKind(p, roots), Folders: s.childFolders(root, p, entries, roots),
		Audio: countAudio(ctx, root, p),
	}
	if p != "/" {
		listing.Parent = path.Dir(p)
	}
	return listing, nil
}

// childFolders are the folders among entries, the contents of p, by name. A
// scan's hidden and NAS folders are left out, as are names that aren't UTF-8,
// which no JSON path can carry.
func (s *Server) childFolders(root *os.Root, p string, entries []os.DirEntry, roots []string) []api.Folder {
	folders := []api.Folder{}
	for _, e := range entries {
		name := e.Name()
		if library.SkipsFolder(name) || !utf8.ValidString(name) || len(folders) == maxListed {
			continue
		}
		if child, ok := folderAt(root, path.Join(p, name), e); ok {
			folders = append(folders, api.Folder{Name: name, Path: child, Kind: s.folderKind(child, roots)})
		}
	}
	slices.SortFunc(folders, func(a, b api.Folder) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.Name, b.Name))
	})
	return folders
}

// folderAt returns the folder the entry e at p is, and whether it is one. A
// symlink is followed only to a folder under root, and the folder it names is
// returned in its place, so the picker opens and adds the real one. os.Root
// refuses every absolute link, even from /, so an absolute target is read as a
// path under root: from the real /, the folder it names.
func folderAt(root *os.Root, p string, e os.DirEntry) (string, bool) {
	if e.IsDir() {
		return p, true
	}
	if e.Type()&os.ModeSymlink == 0 {
		return "", false
	}
	target, err := root.Readlink(rel(p))
	if err != nil || !utf8.ValidString(target) {
		return "", false
	}
	if !path.IsAbs(target) {
		target = path.Join(path.Dir(p), target) // Cleaned: a ".." past / stays at /.
	}
	target = path.Clean(target)
	info, err := root.Stat(rel(target)) // Refused when a further link leads out of root.
	if err != nil || !info.IsDir() {
		return "", false
	}
	return target, true
}

// folderKind says whether p can be added to the library, given its roots.
func (s *Server) folderKind(p string, roots []string) string {
	for _, r := range roots {
		switch {
		case p == r:
			return api.FolderLibrary
		case config.Overlaps(r, p) && strings.HasPrefix(p, r):
			return api.FolderInside
		case config.Overlaps(r, p):
			return api.FolderHolds
		}
	}
	switch {
	case s.dataDir != "" && config.Overlaps(s.dataDir, p):
		return api.FolderData
	case s.configDir != "" && p == s.configDir:
		return api.FolderConfig
	}
	return api.FolderFree
}

// countAudio counts the audio files under p, and the folders that hold them,
// as a scan would find them: hidden and NAS folders left out, symlinks not
// followed. It stops at [countBudget] or [maxCountFolders].
func countAudio(ctx context.Context, root *os.Root, p string) api.AudioCount {
	ctx, cancel := context.WithTimeout(ctx, countBudget)
	defer cancel()
	count := api.AudioCount{Complete: true}
	queue := []string{rel(p)}
	opened := 0
	for len(queue) > 0 {
		if ctx.Err() != nil || opened == maxCountFolders {
			count.Complete = false
			return count
		}
		next := queue[0]
		queue = queue[1:]
		opened++
		files := readFolder(ctx, root, next, &queue)
		count.Files += files
		if files > 0 {
			count.Folders++
		}
	}
	return count
}

// readBatch is how many entries a count reads at a time.
const readBatch = 256

// readFolder adds the folders in dir to queue, to count next, and returns how
// many audio files dir holds. A folder it can't read counts as empty.
func readFolder(ctx context.Context, root *os.Root, dir string, queue *[]string) int {
	f, err := root.Open(dir)
	if err != nil {
		return 0
	}
	defer f.Close() //nolint:errcheck // Read-only.
	files := 0
	for {
		entries, readErr := f.ReadDir(readBatch)
		for _, e := range entries {
			switch {
			case e.IsDir() && !library.SkipsFolder(e.Name()):
				*queue = append(*queue, path.Join(dir, e.Name()))
			case e.Type().IsRegular() && library.IsAudio(e.Name()):
				files++
			}
		}
		if readErr != nil || ctx.Err() != nil {
			return files
		}
	}
}
