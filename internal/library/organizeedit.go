package library

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"sync"

	"github.com/simonhull/kitsune/internal/organize"
	"github.com/simonhull/kitsune/internal/store"
)

// Organizing after an edit maintains, never imposes: with organize.on_edit on,
// an edit notes which of its songs sat where the templates put them before it
// wrote, and once it has answered, [Scanner.RunOrganizeEdits] files those
// songs by their new tags, as one run of kind edit. A song someone filed by
// hand is left where it is, and so is one whose new path is taken.

// editsPending gathers the songs edits left for the organizer: track row →
// where its file was when the edit began.
type editsPending struct {
	mu    sync.Mutex
	songs map[int64]string
	want  chan struct{} // Holds a token when songs wait.
}

// add records songs, and wakes the organizer.
func (e *editsPending) add(songs map[int64]string) {
	if len(songs) == 0 {
		return
	}
	e.mu.Lock()
	if e.songs == nil {
		e.songs = map[int64]string{}
	}
	maps.Copy(e.songs, songs)
	e.mu.Unlock()
	signal(e.want)
}

// take returns the songs waiting, and forgets them.
func (e *editsPending) take() map[int64]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.songs
	e.songs = nil
	return out
}

// inPlaceBefore returns which of the files an edit is about to write sit where
// c's templates put them now, by track row, with their paths. The caller holds
// the library lock. A failure is logged: the edit goes on, and organizes
// nothing.
func (s *Scanner) inPlaceBefore(ctx context.Context, edits []fileEdit) map[int64]string {
	c := s.Organize()
	if !c.OnEdit {
		return nil
	}
	lib, err := s.st.OrganizeLibrary(ctx)
	if err != nil {
		s.log.WarnContext(ctx, "reading the library before an edit failed; the edit organizes nothing", "err", err)
		return nil
	}
	wanted := make(map[int64]bool, len(edits))
	for i := range edits {
		wanted[edits[i].file.ID] = true
	}
	out := map[int64]string{}
	for i := range lib.Songs {
		song := &lib.Songs[i]
		if !wanted[song.ID] {
			continue
		}
		if to, pathErr := c.target(song, firstOf(s.genres.Of(song.Album))); pathErr == nil && slashPath(to) == song.RelPath {
			out[song.ID] = song.RelPath
		}
	}
	return out
}

// writtenOf returns those of inPlace whose files an edit wrote.
func writtenOf(inPlace map[int64]string, written []fileEdit) map[int64]string {
	if len(inPlace) == 0 {
		return nil
	}
	out := make(map[int64]string, len(written))
	for i := range written {
		id := written[i].file.ID
		if at, ok := inPlace[id]; ok {
			out[id] = at
		}
	}
	return out
}

// target returns where c's templates file song, whose album's first display
// genre is genre: a slash-separated path under its root.
func (c *OrganizeConfig) target(song *store.OrganizeSong, genre string) (string, error) {
	ts := c.Templates
	if own, ok := c.Roots[song.Root]; ok {
		ts = own
	}
	t := ts.Template
	if song.Compilation {
		t = ts.Compilations
	}
	in := templateSong(song, genre)
	return t.Path(&in, filepath.Ext(song.RelPath)) //nolint:wrapcheck // organize's errors say what is missing.
}

// RunOrganizeEdits files the songs edits leave it, as they come, until ctx
// ends: each batch of them is one run of kind edit, which waits for a scan or
// another run to end. The app runs it as its background work.
func (s *Scanner) RunOrganizeEdits(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.edits.want:
		}
		if songs := s.edits.take(); len(songs) > 0 {
			s.organizeEdited(ctx, songs)
		}
	}
}

// organizeEdited files songs, each still at the path it had before its edit,
// by its new tags, when the path the templates give it now is free.
func (s *Scanner) organizeEdited(ctx context.Context, songs map[int64]string) {
	c := s.Organize()
	lib, err := s.st.OrganizeLibrary(ctx)
	if err != nil {
		s.log.WarnContext(ctx, "organizing after an edit failed", "err", err)
		return
	}
	p := &OrganizePreview{Templates: c.Templates, Companions: c.Companions}
	var inputs []organize.Input
	var others []organize.Item
	for i := range lib.Songs {
		song := &lib.Songs[i]
		item := organize.Item{Key: song.ID, Root: song.Root, Path: filepath.ToSlash(song.RelPath)}
		if was, ok := songs[song.ID]; ok && was == song.RelPath {
			genre := firstOf(s.genres.Of(song.Album))
			p.Songs, p.Genres = append(p.Songs, *song), append(p.Genres, genre)
			inputs = append(inputs, organize.Input{Item: item, Song: templateSong(song, genre)})
			continue
		}
		others = append(others, item)
	}
	for _, m := range lib.Missing {
		others = append(others, organize.Item{Key: m.ID, Root: m.Root, Path: filepath.ToSlash(m.RelPath)})
	}
	if len(inputs) == 0 {
		return
	}
	lister := &folderLister{roots: openRoots{}}
	defer lister.roots.close()
	p.Plan = organize.MakePlan(inputs, &organize.Options{
		Templates: c.Templates, Roots: c.Roots, Companions: c.Companions, Others: others, List: lister.list,
	})
	if p.Plan.Counts.Renames+p.Plan.Counts.Moves == 0 {
		return
	}
	s.runEdited(ctx, p)
}

// runEdited runs p's moves as a run of kind edit, waiting out a scan or another
// run first.
func (s *Scanner) runEdited(ctx context.Context, p *OrganizePreview) {
	for {
		t := &organizeTask{kind: store.OrganizeKindEdit, rewritePlaylists: s.Organize().RewritePlaylists}
		t.batches = runBatches(p, nil)
		_, run, err := s.startTask(ctx, t, &store.OrganizeRunStart{Kind: store.OrganizeKindEdit, Template: p.Templates.Template.String()})
		switch {
		case err == nil:
			run(ctx)
			return
		case errors.Is(err, ErrBusy):
			if s.claim.waitFree(ctx) != nil {
				return
			}
			if waitErr := s.lock.waitFree(ctx); waitErr != nil {
				return
			}
		default:
			s.log.WarnContext(ctx, "organizing after an edit failed", "err", err)
			return
		}
	}
}

// OrganizeTarget returns where the saved templates file song: a
// slash-separated path under its root. Its errors are [organize.ErrNoName]
// and [organize.ErrTooLong].
func (s *Scanner) OrganizeTarget(song *store.OrganizeSong) (string, error) {
	return s.Organize().target(song, firstOf(s.genres.Of(song.Album)))
}
