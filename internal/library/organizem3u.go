package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/simonhull/kitsune/internal/m3u"
	"github.com/simonhull/kitsune/internal/store"
)

// playlistBackup is the suffix of the copy a run keeps of a playlist file it
// rewrote, for its undo.
const playlistBackup = ".kitsune-bak"

// rewritePlaylists rewrites, in each root songs moved in, the playlist files
// with lines that named a moved song: only those lines, a relative path staying
// relative and an absolute one absolute. A file that isn't UTF-8 (an .m3u in
// Windows-1252) is left alone, since writing it back could change what its
// other lines say. Each original is kept beside it as "<name>.kitsune-bak" (or
// a free name like it), linked before anything is written, and each file is
// read right before it is written and written only if it hasn't changed in
// between. An undo rewrites the lines back and removes the copy when the file
// is the copy again. It runs once the run has ended, under the library lock.
func (s *Scanner) rewritePlaylists(ctx context.Context, t *organizeTask) {
	if err := s.lock.lock(ctx, heldByPass); err != nil {
		return
	}
	defer s.lock.unlock()
	if !s.live().TagWrites {
		return
	}
	for _, root := range slices.Sorted(maps.Keys(t.moved)) {
		files, err := s.st.FilePlaylistStates(ctx, root)
		if err != nil {
			s.log.WarnContext(ctx, "listing playlist files to rewrite failed", "err", err)
			continue
		}
		for _, rel := range slices.Sorted(maps.Keys(files)) {
			done, rewriteErr := s.rewritePlaylist(ctx, t, root, rel)
			if rewriteErr != nil {
				s.log.WarnContext(ctx, "rewriting a playlist file after an organize run failed; it is as it was",
					"err", StripRoot(rewriteErr.Error(), root))
				continue
			}
			if done && t.kind != store.OrganizeKindUndo {
				if countErr := s.st.LandOrganize(ctx, t.id, nil, store.OrganizeTally{Playlists: 1}); countErr != nil {
					s.log.WarnContext(ctx, "counting a rewritten playlist file failed", "err", countErr)
				}
			}
		}
	}
}

// rewritePlaylist rewrites the playlist file rel under root for t's moves and
// reports whether it changed it.
func (s *Scanner) rewritePlaylist(ctx context.Context, t *organizeTask, root, rel string) (bool, error) {
	roots := openRoots{}
	defer roots.close()
	r, err := roots.root(root)
	if err != nil {
		return false, err
	}
	out, err := s.rewritten(r, t, root, rel)
	if err != nil || out == nil {
		return false, err
	}
	if t.kind == store.OrganizeKindUndo {
		return true, restorePlaylist(r, root, rel, out)
	}
	backup, err := keepOriginal(r, rel)
	if err != nil {
		return false, err
	}
	if _, err = writePlaylistFile(root, rel, out, true); err != nil {
		_ = r.Remove(backup) // Nothing was rewritten: no copy to keep.
		return false, err
	}
	if err = s.st.AddOrganizeDone(ctx, t.id, store.MovePlaylist, root, rel, backup); err != nil {
		s.log.WarnContext(ctx, "journaling a rewritten playlist failed", "err", err)
	}
	return true, nil
}

// rewritten reads the playlist file rel under r and returns it rewritten for
// t's moves, or nil when no line changes or it isn't UTF-8. It fails when the
// file changes while it is read.
func (s *Scanner) rewritten(r *os.Root, t *organizeTask, root, rel string) ([]byte, error) {
	before, err := r.Lstat(rel)
	if err != nil {
		return nil, fmt.Errorf("look at the playlist: %w", err)
	}
	data, err := readPlaylistBytes(root, rel)
	if err != nil {
		return nil, err
	}
	if len(data) > m3u.MostBytes || !utf8.Valid(data) {
		return nil, nil
	}
	out, changed := rewriteLines(data, root, path.Dir(filepath.ToSlash(rel)), s.live().Roots, t.moved[root])
	if !changed {
		return nil, nil
	}
	now, err := r.Lstat(rel)
	if err != nil || !now.ModTime().Equal(before.ModTime()) || now.Size() != before.Size() {
		return nil, fmt.Errorf("the playlist changed while it was read: %w", errPathTaken)
	}
	return out, nil
}

// restorePlaylist writes out, an undo's rewrite of the playlist file rel, and
// takes away the copy its run kept once the file is that copy again.
func restorePlaylist(r *os.Root, root, rel string, out []byte) error {
	if _, err := writePlaylistFile(root, rel, out, true); err != nil {
		return err
	}
	if kept, err := readPlaylistBytes(root, rel+playlistBackup); err == nil && bytes.Equal(kept, out) {
		_ = r.Remove(rel + playlistBackup) // The copy has done its work.
	}
	return nil
}

// keepOriginal links the playlist file rel, under r, to a free name beside it
// before a run rewrites it, and returns that name.
func keepOriginal(r *os.Root, rel string) (string, error) {
	for n := 1; n <= mostFreeTries; n++ {
		backup := rel + playlistBackup
		if n > 1 {
			backup = fmt.Sprintf("%s%s-%d", rel, playlistBackup, n)
		}
		err := r.Link(rel, backup)
		switch {
		case err == nil:
			return backup, nil
		case !errors.Is(err, fs.ErrExist):
			return "", fmt.Errorf("keep the original: %w", err)
		}
	}
	return "", fmt.Errorf("keep the original: %w", errPathTaken)
}

// rewriteLines returns data with each path line that names a song in moved
// (from → to, slash-separated under root) naming where it went, and whether
// any line changed. Line endings and every other line stay byte for byte.
func rewriteLines(data []byte, root, dir string, roots []string, moved map[string]string) ([]byte, bool) {
	var out bytes.Buffer
	out.Grow(len(data))
	changed := false
	for len(data) > 0 {
		end := bytes.IndexAny(data, "\r\n")
		line, rest := data, []byte(nil)
		if end >= 0 {
			line, rest = data[:end], data[end:]
		}
		eol := lineEnd(rest)
		next, ok := rewriteLine(string(line), root, dir, roots, moved)
		changed = changed || ok
		out.WriteString(next)
		out.Write(rest[:eol])
		data = rest[eol:]
	}
	return out.Bytes(), changed
}

// crlf is the longest line ending.
const crlf = "\r\n"

// lineEnd returns the length of the line ending rest starts with.
func lineEnd(rest []byte) int {
	switch {
	case bytes.HasPrefix(rest, []byte(crlf)):
		return len(crlf)
	case len(rest) > 0:
		return 1
	}
	return 0
}

// rewriteLine returns line naming where its song went, when it is a path to a
// song in moved, and whether it changed.
func rewriteLine(line, root, dir string, roots []string, moved map[string]string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.Contains(trimmed, "://") || strings.Contains(trimmed, `\`) {
		return line, false
	}
	ref := m3u.Resolve(trimmed, root, dir, roots)
	if ref.Kind != m3u.InLibrary || ref.Root != root {
		return line, false
	}
	to, ok := moved[ref.Rel]
	if !ok {
		return line, false
	}
	lead := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	if strings.HasPrefix(trimmed, "/") {
		return lead + strings.TrimSuffix(filepath.ToSlash(root), "/") + "/" + to, true
	}
	return lead + m3u.RelativeTo(dir, to), true
}
