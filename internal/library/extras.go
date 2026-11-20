package library

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/store"
)

// A scan reads what Kodi and Jellyfin keep beside the music: an artist's
// thumb, background, logo and banner and its artist.nfo bio in the artist's
// folder, and an album's disc art and back cover in its folder. Embedded
// pictures are never read for them. Nothing here ever fails a scan: a bad
// image is a warning, and the store keeps what it had.

// extrasSurvey is what a walk of one root found for the extras step.
type extrasSurvey struct {
	root  string
	files []seenFile
	// listed reports whether the walk read the folder, relative to the root,
	// whole: only then does a file it didn't see count as gone.
	listed func(dir string) bool
}

// walkedListed is a whole walk's listed: every folder but an unreadable one,
// or one holding an unreadable entry.
func walkedListed(unreadable []string) func(string) bool {
	return func(dir string) bool {
		for _, u := range unreadable {
			if u == dir || filepath.Dir(u) == dir || strings.HasPrefix(dir, u+string(filepath.Separator)) {
				return false
			}
		}
		return true
	}
}

// targetKind says what an extra sets.
type targetKind int

const (
	artistImage targetKind = iota
	albumImage
	artistBio
)

// extraTarget is one image or bio a scan may set.
type extraTarget struct {
	kind       targetKind
	artist     store.LayoutArtist    // For an artist's image or bio.
	artistKind store.ArtistImageKind // For an artist's image.
	album      int64                 // For an album's extra.
	albumULID  string
	albumKind  store.AlbumImageKind
	disc       int
}

// art is the artwork kind the target's image renders as.
func (t *extraTarget) art() artwork.Kind {
	if t.kind == albumImage {
		if t.albumKind == store.AlbumDisc {
			return discKind.art
		}
		return backKind.art
	}
	k := artistKinds[t.artistKind]
	return k.art
}

// extraJob is a target with the files that may set it, best first, which the
// step reads until one serves.
type extraJob struct {
	target extraTarget
	root   string
	files  []*seenFile
	stored store.StoredExtra // What the store holds: a folder's is taken away if no file serves.
}

// extrasPlan is what the extras step will do.
type extrasPlan struct {
	listed  func(folder) bool
	full    bool
	jobs    []extraJob
	deletes []extraTarget // Folder images and bios whose files are gone.
}

// consider decides one target from its candidates, all in the folder at, and
// what the store holds of it. A pin is never touched; a target whose folders
// this scan didn't read is left as it is; an unchanged file is not read again
// but by a full scan.
func (p *extrasPlan) consider(t *extraTarget, at folder, cands []*seenFile, stored store.StoredExtra) {
	switch {
	case stored.Pinned:
	case len(cands) == 0:
		if stored.File != nil && p.listed(folder{root: stored.File.Root, dir: filepath.Dir(stored.File.RelPath)}) {
			p.deletes = append(p.deletes, *t)
		}
	case !p.listed(at), !p.full && stored.File.Same(fileRef(at.root, cands[0])):
	default:
		p.jobs = append(p.jobs, extraJob{target: *t, root: at.root, files: cands, stored: stored})
	}
}

// fileRef is the store's name for a file a walk saw under root.
func fileRef(root string, f *seenFile) *store.FileRef {
	return &store.FileRef{Root: root, RelPath: f.rel, Size: f.size, MTime: f.mtime}
}

// folderFiles indexes the files the surveys saw by folder.
func folderFiles(surveys []extrasSurvey) map[folder][]*seenFile {
	out := map[folder][]*seenFile{}
	for i := range surveys {
		sv := &surveys[i]
		for j := range sv.files {
			f := &sv.files[j]
			at := folder{root: sv.root, dir: filepath.Dir(f.rel)}
			out[at] = append(out[at], f)
		}
	}
	return out
}

// planExtras works out what the extras step does for every artist and album
// the layout knows, from the files the surveys saw.
func planExtras(l *store.Layout, surveys []extrasSurvey, roots []string, mode Mode) *extrasPlan {
	listed := make(map[string]func(string) bool, len(surveys))
	for i := range surveys {
		listed[surveys[i].root] = surveys[i].listed
	}
	p := &extrasPlan{
		full: mode == Full,
		listed: func(f folder) bool {
			fn, ok := listed[f.root]
			return ok && fn(f.dir)
		},
	}
	files := folderFiles(surveys)
	albums := albumFolders(l.Tracks)
	p.planArtists(l, artistFolders(l, albums, roots), files)
	p.planAlbums(l, albums, files)
	return p
}

// planArtists considers every artist with an artist folder, or with a folder's
// image or bio in the store.
func (p *extrasPlan) planArtists(l *store.Layout, folders map[int64][]folder, files map[folder][]*seenFile) {
	for _, artist := range artistsToConsider(l, folders) {
		fs := folders[artist.ID]
		for _, kind := range slices.Sorted(maps.Keys(artistKinds)) {
			k := artistKinds[kind]
			at, cands := firstWith(fs, files, k.candidates)
			t := extraTarget{kind: artistImage, artist: artist, artistKind: kind}
			p.consider(&t, at, cands, l.ArtistImages[store.ArtistImageKey{ArtistID: artist.ID, Kind: kind}])
		}
		at, cands := firstWith(fs, files, nfoCandidates)
		p.consider(&extraTarget{kind: artistBio, artist: artist}, at, cands, l.Bios[artist.ID])
	}
}

// artistsToConsider returns, by row, the album owners with a folder, or with a
// folder's image or bio in the store. One that owns no album is left out: it
// goes at the prune.
func artistsToConsider(l *store.Layout, folders map[int64][]folder) []store.LayoutArtist {
	want := map[int64]bool{}
	for id := range folders {
		want[id] = true
	}
	for k, e := range l.ArtistImages {
		want[k.ArtistID] = want[k.ArtistID] || e.File != nil
	}
	for id, e := range l.Bios {
		want[id] = want[id] || e.File != nil
	}
	seen := map[int64]bool{}
	var out []store.LayoutArtist
	for _, owners := range l.Owners {
		for _, o := range owners {
			if want[o.ID] && !seen[o.ID] {
				seen[o.ID] = true
				out = append(out, o)
			}
		}
	}
	slices.SortFunc(out, func(a, b store.LayoutArtist) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

// firstWith returns the first of folders with files that pick chooses, and those.
func firstWith(folders []folder, files map[folder][]*seenFile, pick func([]*seenFile) []*seenFile) (folder, []*seenFile) {
	for _, f := range folders {
		if cands := pick(files[f]); len(cands) > 0 {
			return f, cands
		}
	}
	return folder{}, nil
}

// planAlbums considers every live album with a folder of its own, or with a
// folder's extra in the store.
func (p *extrasPlan) planAlbums(l *store.Layout, albums map[int64]folder, files map[folder][]*seenFile) {
	ulids := map[int64]string{}
	for i := range l.Tracks {
		ulids[l.Tracks[i].AlbumID] = l.Tracks[i].AlbumULID
	}
	stored := storedDiscs(l, albums)
	for _, id := range slices.Sorted(maps.Keys(stored)) {
		if ulid, live := ulids[id]; live { // One with no live track goes at the prune.
			at, hasFolder := albums[id]
			var in []*seenFile
			if hasFolder {
				in = files[at]
			}
			p.planAlbum(l, albumAt{id: id, ulid: ulid, at: at}, in, stored[id])
		}
	}
}

// albumAt is an album and its folder.
type albumAt struct {
	id   int64
	ulid string
	at   folder
}

// storedDiscs returns the albums to consider, each with the discs the store
// holds a folder's disc art for.
func storedDiscs(l *store.Layout, albums map[int64]folder) map[int64]map[int]bool {
	out := map[int64]map[int]bool{}
	for id := range albums {
		out[id] = map[int]bool{}
	}
	for k, e := range l.AlbumImages {
		if e.File == nil {
			continue
		}
		if out[k.AlbumID] == nil {
			out[k.AlbumID] = map[int]bool{}
		}
		if k.Kind == store.AlbumDisc {
			out[k.AlbumID][k.Disc] = true
		}
	}
	return out
}

// planAlbum considers one album's back cover and disc art, from the files in
// its folder.
func (p *extrasPlan) planAlbum(l *store.Layout, a albumAt, in []*seenFile, discs map[int]bool) {
	back := extraTarget{kind: albumImage, album: a.id, albumULID: a.ulid, albumKind: store.AlbumBack}
	p.consider(&back, a.at, backKind.candidates(in), l.AlbumImages[store.AlbumImageKey{AlbumID: a.id, Kind: store.AlbumBack}])
	found := discCandidates(in)
	for disc := range found {
		discs[disc] = true
	}
	for _, disc := range slices.Sorted(maps.Keys(discs)) {
		t := extraTarget{kind: albumImage, album: a.id, albumULID: a.ulid, albumKind: store.AlbumDisc, disc: disc}
		p.consider(&t, a.at, found[disc], l.AlbumImages[store.AlbumImageKey{AlbumID: a.id, Kind: store.AlbumDisc, Disc: disc}])
	}
}

// extraResult is what reading a job's files gave: an image or a bio from the
// file that served, or nothing.
type extraResult struct {
	image *store.ArtworkWrite
	bio   *store.NfoBio
	file  store.FileRef
}

// badExtras remembers the files that served no image or bio, by name, size and
// mtime, so a scan reads them again only once they change.
type badExtras struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func badKey(f *store.FileRef) string {
	return f.Root + "\x00" + f.RelPath + "\x00" + strconv.FormatInt(f.Size, 10) + "\x00" + strconv.FormatInt(f.MTime.UnixMilli(), 10)
}

func (b *badExtras) has(f *store.FileRef) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.seen[badKey(f)]
	return ok
}

func (b *badExtras) add(f *store.FileRef) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.seen == nil {
		b.seen = map[string]struct{}{}
	}
	b.seen[badKey(f)] = struct{}{}
}

// readExtras is the extras step of a scan: it works out every artist's and
// album's folder images and bio from the files the surveys saw, reads those
// that are new or changed, and stores them. It publishes the artists it
// changed; the albums go with the scan's library change. It fails only when
// ctx ends or the store does.
func (s *Scanner) readExtras(ctx context.Context, surveys []extrasSurvey, mode Mode, stats *Stats) error {
	if len(surveys) == 0 {
		return nil
	}
	l, err := s.st.FolderLayout(ctx)
	if err != nil {
		return fmt.Errorf("extras: %w", err)
	}
	p := planExtras(l, surveys, s.live().Roots, mode)
	if s.artwork == nil {
		p.biosAlone() // Nothing renders an image.
	}
	if len(p.jobs) == 0 && len(p.deletes) == 0 {
		return nil
	}
	results, err := s.runExtras(ctx, p.jobs)
	if err != nil {
		return err
	}
	w := extrasWrite(p, results)
	if w.Empty() {
		return nil
	}
	changed, err := s.st.WriteExtras(ctx, w, s.now())
	if err != nil {
		return fmt.Errorf("extras: %w", err)
	}
	stats.Extras = len(w.ArtistImages) + len(w.AlbumImages) + len(w.Bios)
	s.notifyArtists(l, changed)
	return nil
}

// notifyArtists tells the notifier which artists' images or bio changed.
func (s *Scanner) notifyArtists(l *store.Layout, changed []int64) {
	if len(changed) == 0 {
		return
	}
	ulids := map[int64]string{}
	for _, owners := range l.Owners {
		for _, o := range owners {
			ulids[o.ID] = o.ULID
		}
	}
	var ids []string
	for _, id := range changed {
		if u, ok := ulids[id]; ok {
			ids = append(ids, u)
		}
	}
	slices.Sort(ids)
	if ids = slices.Compact(ids); len(ids) > 0 {
		s.notify.ArtistsChanged(ids)
	}
}

// runExtras reads the jobs' files on the scan's workers. A result with nothing
// in it means no file served.
func (s *Scanner) runExtras(ctx context.Context, jobs []extraJob) ([]extraResult, error) {
	results := make([]extraResult, len(jobs))
	roots := openJobRoots(jobs)
	defer func() {
		for _, r := range roots {
			_ = r.Close() // Read-only.
		}
	}()
	next := make(chan int)
	var wg sync.WaitGroup
	for range min(s.workers, len(jobs)) {
		wg.Go(func() {
			for i := range next {
				if root := roots[jobs[i].root]; root != nil {
					results[i] = s.serveExtra(ctx, root, &jobs[i])
				}
			}
		})
	}
	for i := range jobs {
		if ctx.Err() != nil {
			break
		}
		next <- i
	}
	close(next)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("extras: %w", err)
	}
	return results, nil
}

// openJobRoots opens each root the jobs read under. A root that won't open is
// left out: it vanished since the walk, which is the next scan's business, and
// its jobs serve nothing.
func openJobRoots(jobs []extraJob) map[string]*os.Root {
	roots := map[string]*os.Root{}
	for i := range jobs {
		if _, ok := roots[jobs[i].root]; ok {
			continue
		}
		if root, err := os.OpenRoot(jobs[i].root); err == nil {
			roots[jobs[i].root] = root
		}
	}
	return roots
}

// serveExtra reads the job's files in order until one serves.
func (s *Scanner) serveExtra(ctx context.Context, root *os.Root, j *extraJob) extraResult {
	for _, f := range j.files {
		ref := fileRef(j.root, f)
		if s.badExtras.has(ref) || ctx.Err() != nil {
			continue
		}
		limit := int64(maxArtworkBytes)
		if j.target.kind == artistBio {
			limit = maxNfoBytes
		}
		data := readLimited(root, f.rel, limit)
		if data == nil {
			s.badExtra(ctx, ref, "unreadable or too large")
			continue
		}
		if j.target.kind == artistBio {
			bio, err := parseNfo(data, j.target.artist.MBID)
			if err != nil {
				s.badExtra(ctx, ref, err.Error())
				continue
			}
			return extraResult{bio: bio, file: *ref}
		}
		if art, ok := s.putExtra(ctx, j.target.art(), data, ref); ok {
			return extraResult{image: art, file: *ref}
		}
	}
	return extraResult{}
}

// putExtra renders data as kind. A bad image is remembered and logged.
func (s *Scanner) putExtra(ctx context.Context, kind artwork.Kind, data []byte, ref *store.FileRef) (*store.ArtworkWrite, bool) {
	if s.artwork == nil {
		return nil, false
	}
	img, err := s.artwork.Put(ctx, kind, data)
	switch {
	case err == nil:
		return &store.ArtworkWrite{
			Hash: img.Hash, Width: img.Width, Height: img.Height,
			Accent: img.Accent, Ground: img.Ground, Source: store.ArtworkFolder, Alpha: img.Alpha,
		}, true
	case ctx.Err() != nil:
	case errors.Is(err, artwork.ErrBadImage):
		s.badExtra(ctx, ref, "not a decodable image")
	case errors.Is(err, artwork.ErrTooLarge):
		s.badExtra(ctx, ref, "too many pixels")
	default:
		s.log.WarnContext(ctx, "storing an artist or album image failed", "err", err)
	}
	return nil, false
}

// badExtra remembers a file that served nothing and logs why, by its path
// under its root: a log line never names a root.
func (s *Scanner) badExtra(ctx context.Context, ref *store.FileRef, why string) {
	s.badExtras.add(ref)
	s.log.WarnContext(ctx, "an artist or album file was skipped", "path", strings.ToValidUTF8(ref.RelPath, "�"), "why", why)
}

// extrasWrite turns the plan and what its jobs read into the store's write. A
// job whose files all failed takes away what a folder gave it before.
func extrasWrite(p *extrasPlan, results []extraResult) *store.ExtrasWrite {
	w := &store.ExtrasWrite{}
	for i := range p.deletes {
		addExtra(w, &p.deletes[i], &extraResult{})
	}
	for i := range p.jobs {
		if j, r := &p.jobs[i], &results[i]; !j.unchanged(r) {
			addExtra(w, &j.target, r)
		}
	}
	return w
}

// unchanged reports whether r leaves the store as it is: no file served and the
// store holds no folder's, or the file it holds served the image it holds, as a
// full scan reading every file again finds.
func (j *extraJob) unchanged(r *extraResult) bool {
	if r.image == nil && r.bio == nil {
		return j.stored.File == nil
	}
	if !j.stored.File.Same(&r.file) {
		return false
	}
	return r.bio != nil || r.image.Hash == j.stored.Hash
}

// biosAlone drops every image from the plan, for a scanner with no cache to
// render one.
func (p *extrasPlan) biosAlone() {
	bio := func(t *extraTarget) bool { return t.kind == artistBio }
	p.jobs = slices.DeleteFunc(p.jobs, func(j extraJob) bool { return !bio(&j.target) })
	p.deletes = slices.DeleteFunc(p.deletes, func(t extraTarget) bool { return !bio(&t) })
}

// addExtra adds to w what r sets of t: an empty r takes it away.
func addExtra(w *store.ExtrasWrite, t *extraTarget, r *extraResult) {
	switch t.kind {
	case artistImage:
		w.ArtistImages = append(w.ArtistImages, store.ArtistImageWrite{ArtistID: t.artist.ID, Kind: t.artistKind, Image: r.image, File: r.file})
	case albumImage:
		w.AlbumImages = append(w.AlbumImages, store.AlbumImageWrite{
			AlbumID: t.album, AlbumULID: t.albumULID, Kind: t.albumKind, Disc: t.disc, Image: r.image, File: r.file,
		})
	case artistBio:
		w.Bios = append(w.Bios, store.BioWrite{ArtistID: t.artist.ID, Bio: r.bio, File: r.file})
	}
}
