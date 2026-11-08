package library

import (
	"context"
	"encoding/binary"
	"hash/fnv"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
)

// polled is what one look over a polled root found.
type polled struct {
	root, resolved string
	snap           map[string]uint64 // Nil when the root couldn't be walked.
}

// tick is the poll's beat: roots that are off are tried again, a watched root
// whose filesystem moved (an unmount, a remount) is armed afresh and scanned
// whole, and the polled roots are looked over.
func (l *watchLoop) tick(ctx context.Context) {
	l.rearm(ctx)
	mounts := l.w.opts.mounts()
	changed := false
	for _, root := range slices.Sorted(maps.Keys(l.roots)) {
		rs := l.roots[root]
		if rs.mode != Watching {
			continue
		}
		if m, _ := mountOf(mounts, rs.resolved); m != rs.mount {
			l.w.log.InfoContext(ctx, "library folder's filesystem changed; watching it afresh and scanning", "root", root)
			l.release(rs)
			l.arm(ctx, rs)
			l.rescan, changed = true, true
			l.touch()
		}
	}
	if changed {
		l.publish()
	}
	l.look(ctx)
}

// look walks every polled root on its own goroutine, unless a walk is running:
// a slow network share must never hold up the events of a watched root.
func (l *watchLoop) look(ctx context.Context) {
	if l.looking {
		return
	}
	var todo []polled
	for _, root := range slices.Sorted(maps.Keys(l.roots)) {
		if rs := l.roots[root]; rs.mode == Polling {
			todo = append(todo, polled{root: root, resolved: rs.resolved})
		}
	}
	if len(todo) == 0 {
		return
	}
	l.looking = true
	l.scans.Go(func() {
		for i := range todo {
			todo[i].snap = snapshot(ctx, todo[i].resolved)
		}
		l.back <- func() { l.compare(ctx, todo) }
	})
}

// compare takes in a look, and gives a polled root armed while the walk ran a
// look of its own.
func (l *watchLoop) compare(ctx context.Context, found []polled) {
	l.looking = false
	for i := range found {
		l.compareRoot(ctx, &found[i])
	}
	for _, rs := range l.roots {
		if rs.mode == Polling && rs.snap == nil {
			l.look(ctx)
			return
		}
	}
}

// compareRoot takes in the look at one root: one that couldn't be walked is
// off, the first look is its baseline, and after that every folder whose
// signature changed, appeared or went is marked dirty for the settle.
func (l *watchLoop) compareRoot(ctx context.Context, f *polled) {
	rs, ok := l.roots[f.root]
	switch {
	case !ok || rs.mode != Polling || rs.resolved != f.resolved:
		return
	case f.snap == nil:
		l.off(ctx, f.root)
		return
	case rs.snap != nil:
		for _, dir := range changedDirs(rs.snap, f.snap) {
			l.mark(rs, filepath.Join(rs.resolved, dir))
		}
	}
	rs.snap = f.snap
}

// changedDirs returns the folders whose signatures differ between before and
// after, or that only one of them has, sorted.
func changedDirs(before, after map[string]uint64) []string {
	var out []string
	for dir, sig := range after {
		if old, ok := before[dir]; !ok || old != sig {
			out = append(out, dir)
		}
	}
	for dir := range before {
		if _, ok := after[dir]; !ok {
			out = append(out, dir)
		}
	}
	slices.Sort(out)
	return out
}

// snapshot signs every folder under the resolved root a scan reads, by
// relative path: its own mtime, its subfolders' names, and its audio files'
// names, sizes and mtimes. A folder's mtime alone misses a file rewritten in
// place. Each entry's hash is added, so the order of a walk doesn't matter. It
// returns nil when the root can't be walked, or ctx ends.
func snapshot(ctx context.Context, resolved string) map[string]uint64 {
	sigs := map[string]uint64{}
	err := filepath.WalkDir(resolved, func(path string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr //nolint:wrapcheck // Only nil-or-not matters to the caller.
		}
		switch {
		case err != nil && path == resolved:
			return err
		case err != nil:
			return nil //nolint:nilerr // An unreadable entry is the scan's to report.
		}
		return sign(sigs, resolved, path, d)
	})
	if err != nil {
		return nil
	}
	return sigs
}

// sign adds the entry at path to its folder's signature, and a folder's own
// to its own and, by name, to its parent's.
func sign(sigs map[string]uint64, resolved, path string, d fs.DirEntry) error {
	rel, _ := filepath.Rel(resolved, path) // Under resolved by construction.
	switch {
	case d.IsDir() && path != resolved && SkipsFolder(d.Name()):
		return filepath.SkipDir
	case d.IsDir():
		sigs[rel] += entryHash(d)
		if rel != "." {
			sigs[filepath.Dir(rel)] += nameHash(d.Name())
		}
	case IsAudio(d.Name()):
		sigs[filepath.Dir(rel)] += entryHash(d)
	}
	return nil
}

// nameHash hashes a name.
func nameHash(name string) uint64 {
	f := fnv.New64a()
	_, _ = f.Write([]byte(name)) // A hash never fails to write.
	return f.Sum64()
}

// entryHash hashes an entry's name, size and mtime.
func entryHash(d fs.DirEntry) uint64 {
	f := fnv.New64a()
	_, _ = f.Write([]byte(d.Name())) // A hash never fails to write.
	var buf [16]byte
	if info, err := d.Info(); err == nil {
		binary.LittleEndian.PutUint64(buf[:8], uint64(info.Size())) //nolint:gosec // G115: a size's bits, only hashed.
		binary.LittleEndian.PutUint64(buf[8:], uint64(info.ModTime().UnixNano()))
	}
	_, _ = f.Write(buf[:])
	return f.Sum64()
}
