package organize

import (
	"errors"
	"maps"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Kind is what a plan does with a song.
type Kind uint8

// The kinds.
const (
	InPlace  Kind = iota // The song is where the template puts it.
	Rename               // A new name in the same folder.
	Move                 // Into another folder.
	Conflict             // Another song, or a file Kitsune doesn't know, is at or wants its path.
	Skipped              // The template makes no path for it.
)

// String returns the kind's name in the API.
func (k Kind) String() string {
	switch k {
	case InPlace:
		return "inPlace"
	case Rename:
		return "rename"
	case Move:
		return "move"
	case Conflict:
		return "conflict"
	case Skipped:
		return "skipped"
	}
	return "unknown"
}

// Moves reports whether the kind moves a file.
func (k Kind) Moves() bool { return k == Rename || k == Move }

// Reason says why a song is in conflict, why one was skipped, or why a run
// left it where it was.
type Reason string

// The reasons. A plan gives the first four; a run, the rest.
const (
	NoReason        Reason = ""
	NoTags          Reason = "noTags"          // Nothing for the file's name.
	TooLong         Reason = "tooLong"         // Past MostPathBytes even cut short.
	Claimed         Reason = "claimed"         // Another song is at the path, or wants it.
	UnknownFile     Reason = "unknownFile"     // A file Kitsune doesn't play is at the path.
	OtherFilesystem Reason = "otherFilesystem" // The path is on another filesystem: a move would be a copy.
	Changed         Reason = "changed"         // The song moved or changed since the preview.
	Missing         Reason = "missing"         // The song's file is gone.
	Failed          Reason = "failed"          // The move failed; the log says why.
	MovedSince      Reason = "movedSince"      // An undo found the song moved again, edited or trashed.
	Taken           Reason = "taken"           // An undo found something at the song's old path.
)

// Item is one song with a path to look at: where it is, under which root.
type Item struct {
	Key  int64  // The caller's ID for it.
	Root string // The library root it is under.
	Path string // Slash-separated, relative to Root.
}

// Input is one song to plan.
type Input struct {
	Item

	Song Song
}

// Templates is a pair of templates: one for most albums, one for compilations.
type Templates struct {
	Template, Compilations Template
}

// Entry is a name in a folder on disk.
type Entry struct {
	Name string
	Dir  bool
}

// Options is how a plan is made.
type Options struct {
	Templates

	// Roots holds the templates of roots that don't use the main pair.
	Roots map[string]Templates
	// Companions brings files beside the songs along: see [IsCompanion].
	Companions bool
	// Others are every song not being planned that is still somewhere in the
	// library: a song in the way is in conflict.
	Others []Item
	// List returns the names in the folder dir, slash-separated, under root,
	// or none when there is no such folder. Nil looks at no disk: no file
	// Kitsune doesn't know is ever in the way, and nothing is brought along.
	List func(root, dir string) []Entry
}

// Planned is what a plan does with one song.
type Planned struct {
	Kind     Kind
	Reason   Reason
	To       string // Where the template files it; empty when skipped.
	CaseOnly bool   // The path changes only in case, which a run does in two steps.
	// With are the other songs in a [Claimed] conflict, by Key: those that
	// want the same path, and the song already there.
	With []int64
	// Unknown is the name of the file in the way of an [UnknownFile] conflict.
	Unknown string
}

// Companion is a file that comes along with a song: a cover, a cue sheet, a
// rip log, lyrics or notes.
type Companion struct {
	Root, From, To string
	Song           int64 // The Key of the song it follows, or of one from its folder.
}

// Counts are a plan's totals.
type Counts struct {
	Songs, InPlace, Renames, Moves, Conflicts, Skipped int
}

// Plan is what organizing the songs would do.
type Plan struct {
	Songs      []Planned // One per input, in order.
	Companions []Companion
	Counts     Counts
	Folders    int // How many folders the moves land in.
}

// companionExts are the extensions of the files that come along.
//
//nolint:gochecknoglobals // A read-only table.
var companionExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true, ".bmp": true,
	".cue": true, ".log": true, ".lrc": true, ".txt": true,
}

// IsCompanion reports whether a file called name comes along with the songs
// beside it: a cover, a .cue, a .log, a .lrc or a .txt.
func IsCompanion(name string) bool {
	return !strings.HasPrefix(name, ".") && companionExts[strings.ToLower(path.Ext(name))]
}

// MakePlan works out where each of songs goes and what is in the way: two songs
// that want one path (compared as a case-insensitive filesystem would, since a
// client over SMB can't hold both), a song already there, or a file Kitsune
// doesn't know. A song in conflict or skipped stays where it is.
//
// With [Options.Companions], files beside the songs come along: one named for a
// song (its .lrc, say) follows that song, and the rest of a folder (covers,
// .cue, .log, .txt) follows its songs when they all go to one new folder.
func MakePlan(songs []Input, opts *Options) Plan {
	p := planner{opts: opts, caser: cases.Fold(), lists: map[folder][]Entry{}}
	return p.plan(songs)
}

// folder is a folder under a root.
type folder struct{ root, dir string }

// planner makes one plan.
type planner struct {
	opts  *Options
	caser cases.Caser
	lists map[folder][]Entry // Folders listed so far.
	// at holds where every song is now, folded, keyed to its Key.
	at map[string]int64
}

// plan makes the plan.
func (p *planner) plan(songs []Input) Plan {
	out := Plan{Songs: make([]Planned, len(songs))}
	p.at = make(map[string]int64, len(songs)+len(p.opts.Others))
	for i := range p.opts.Others {
		o := &p.opts.Others[i]
		p.at[p.key(o.Root, o.Path)] = o.Key
	}
	for i := range songs {
		p.at[p.key(songs[i].Root, songs[i].Path)] = songs[i].Key
	}
	for i := range songs {
		out.Songs[i] = p.target(&songs[i])
	}
	p.conflicts(songs, out.Songs)
	if p.opts.Companions && p.opts.List != nil {
		out.Companions = p.companions(songs, out.Songs)
	}
	out.tally(songs)
	return out
}

// tally counts the plan's songs and the folders its moves land in.
func (out *Plan) tally(songs []Input) {
	folders := map[folder]struct{}{}
	for i := range out.Songs {
		pl := &out.Songs[i]
		out.Counts.add(pl.Kind)
		if pl.Kind.Moves() {
			folders[folder{songs[i].Root, path.Dir(pl.To)}] = struct{}{}
		}
	}
	out.Folders = len(folders)
}

// add counts one song of kind k.
func (c *Counts) add(k Kind) {
	c.Songs++
	switch k {
	case InPlace:
		c.InPlace++
	case Rename:
		c.Renames++
	case Move:
		c.Moves++
	case Conflict:
		c.Conflicts++
	case Skipped:
		c.Skipped++
	}
}

// templates returns the templates for songs under root.
func (p *planner) templates(root string) *Templates {
	if t, ok := p.opts.Roots[root]; ok {
		return &t
	}
	return &p.opts.Templates
}

// target works out where one song goes, before anything is in the way.
func (p *planner) target(in *Input) Planned {
	ts := p.templates(in.Root)
	t := ts.Template
	if in.Song.Compilation {
		t = ts.Compilations
	}
	to, err := t.Path(&in.Song, path.Ext(in.Path))
	switch {
	case err == nil:
	case errors.Is(err, ErrTooLong):
		return Planned{Kind: Skipped, Reason: TooLong}
	default:
		return Planned{Kind: Skipped, Reason: NoTags}
	}
	switch {
	case to == in.Path:
		return Planned{Kind: InPlace, To: to}
	case path.Dir(to) == path.Dir(in.Path):
		return Planned{Kind: Rename, To: to, CaseOnly: p.fold(to) == p.fold(in.Path)}
	}
	return Planned{Kind: Move, To: to, CaseOnly: p.fold(to) == p.fold(in.Path)}
}

// conflicts marks the moves something is in the way of.
func (p *planner) conflicts(songs []Input, out []Planned) {
	want := map[string][]int64{}
	for i := range out {
		if out[i].Kind.Moves() {
			k := p.key(songs[i].Root, out[i].To)
			want[k] = append(want[k], songs[i].Key)
		}
	}
	for i := range out {
		if out[i].Kind.Moves() {
			p.conflict(&songs[i], &out[i], want)
		}
	}
}

// conflict marks one move as a conflict when something is in its way: other
// songs that want its path (want, by folded path), a song already there, or a
// file Kitsune doesn't know.
func (p *planner) conflict(in *Input, pl *Planned, want map[string][]int64) {
	k := p.key(in.Root, pl.To)
	for _, other := range want[k] {
		if other != in.Key {
			pl.With = append(pl.With, other)
		}
	}
	if there, ok := p.at[k]; ok && there != in.Key {
		pl.With = append(pl.With, there)
	}
	if len(pl.With) > 0 {
		pl.Kind, pl.Reason = Conflict, Claimed
		return
	}
	if name, ok := p.unknown(in.Root, pl.To, in.Path); ok {
		pl.Kind, pl.Reason, pl.Unknown = Conflict, UnknownFile, name
	}
}

// unknown returns the name of a file or folder at to, under root, that is no
// song's, unless it is from, the song's own file before a change of case.
func (p *planner) unknown(root, to, from string) (string, bool) {
	dir := path.Dir(to)
	want := p.fold(path.Base(to))
	for _, e := range p.list(root, dir) {
		if p.fold(e.Name) != want {
			continue
		}
		rel := path.Join(dir, e.Name)
		if _, known := p.at[p.key(root, rel)]; known && !e.Dir {
			continue // A song: the conflict check has seen it.
		}
		if rel == from {
			continue
		}
		return e.Name, true
	}
	return "", false
}

// list returns the names in dir under root, listing each folder once.
func (p *planner) list(root, dir string) []Entry {
	f := folder{root, dir}
	if names, ok := p.lists[f]; ok {
		return names
	}
	var names []Entry
	if p.opts.List != nil {
		names = p.opts.List(root, dir)
	}
	p.lists[f] = names
	return names
}

// companions works out the files that come along with the moves.
func (p *planner) companions(songs []Input, out []Planned) []Companion {
	src := p.sources(songs, out)
	found := make([]Companion, 0, len(src.moving))
	for _, f := range slices.SortedFunc(maps.Keys(src.moving), compareFolders) {
		found = append(found, p.folderCompanions(f, songs, out, src.moving[f], src.staying[f])...)
	}
	claimed := map[string]int{} // Target, folded → how many companions want it, a song counting twice.
	for i := range found {
		claimed[p.key(found[i].Root, found[i].To)]++
	}
	for i := range out {
		if out[i].Kind.Moves() {
			claimed[p.key(songs[i].Root, out[i].To)] += 2
		}
	}
	return slices.DeleteFunc(found, func(c Companion) bool {
		return claimed[p.key(c.Root, c.To)] != 1 || p.taken(c.Root, c.To)
	})
}

// sourceFolders are the folders songs are in now.
type sourceFolders struct {
	moving  map[folder][]int // Folders songs move out of or within, each with the songs that do.
	staying map[folder]bool  // Folders a song stays in, a song not planned included.
}

// sources returns the folders songs are in now.
func (p *planner) sources(songs []Input, out []Planned) sourceFolders {
	moving := map[folder][]int{}
	staying := map[folder]bool{}
	for i := range out {
		f := folder{songs[i].Root, path.Dir(songs[i].Path)}
		if out[i].Kind.Moves() {
			moving[f] = append(moving[f], i)
		} else {
			staying[f] = true
		}
	}
	for i := range p.opts.Others {
		o := &p.opts.Others[i]
		staying[folder{o.Root, path.Dir(o.Path)}] = true
	}
	return sourceFolders{moving: moving, staying: staying}
}

// folderCompanions returns the companions in the folder f, which the songs
// idx are leaving or renamed in. stays says a song will still be there.
func (p *planner) folderCompanions(f folder, songs []Input, out []Planned, idx []int, stays bool) []Companion {
	stems := map[string]int{} // A song's file name without its extension, folded → its index.
	targets := map[string]struct{}{}
	for _, i := range idx {
		base := path.Base(songs[i].Path)
		stems[p.fold(strings.TrimSuffix(base, path.Ext(base)))] = i
		targets[path.Dir(out[i].To)] = struct{}{}
	}
	whole := !stays && len(targets) == 1
	var to string
	for t := range targets {
		to = t
	}
	entries := p.list(f.root, f.dir)
	found := make([]Companion, 0, len(entries))
	for _, e := range entries {
		if e.Dir || !IsCompanion(e.Name) {
			continue
		}
		from := path.Join(f.dir, e.Name)
		stem := strings.TrimSuffix(e.Name, path.Ext(e.Name))
		if i, ok := stems[p.fold(stem)]; ok {
			newBase := path.Base(out[i].To)
			newStem := strings.TrimSuffix(newBase, path.Ext(newBase))
			found = append(found, Companion{Root: f.root, From: from, To: path.Join(path.Dir(out[i].To), newStem+path.Ext(e.Name)), Song: songs[i].Key})
			continue
		}
		if whole && to != f.dir {
			found = append(found, Companion{Root: f.root, From: from, To: path.Join(to, e.Name), Song: songs[idx[0]].Key})
		}
	}
	return slices.DeleteFunc(found, func(c Companion) bool { return c.From == c.To })
}

// taken reports whether a file is at to, under root, by its folder's listing.
func (p *planner) taken(root, to string) bool {
	want := p.fold(path.Base(to))
	return slices.ContainsFunc(p.list(root, path.Dir(to)), func(e Entry) bool { return p.fold(e.Name) == want })
}

// key is where a path under root is, folded, so that two paths a
// case-insensitive filesystem can't tell apart have one key.
func (p *planner) key(root, rel string) string { return root + "\x00" + p.fold(rel) }

// fold returns s under NFC and case folding: for ASCII, its lower case.
func (p *planner) fold(s string) string {
	ascii := true
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return strings.ToLower(s)
	}
	return norm.NFC.String(p.caser.String(norm.NFC.String(s)))
}

// compareFolders orders folders by root, then path.
func compareFolders(a, b folder) int {
	if c := strings.Compare(a.root, b.root); c != 0 {
		return c
	}
	return strings.Compare(a.dir, b.dir)
}
