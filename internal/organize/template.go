// Package organize says where a song's file belongs: a path template over its
// tags, the rule that makes any value safe to put in a path, and the plan of
// what a run over the library would move and what it couldn't.
//
// It is pure: it knows nothing of the store, the scanner or a disk. What it
// needs of a disk, the names in a folder, a caller hands it as a function, the
// way internal/smartlist knows nothing of SQL. [Parse] is the only way to make
// a [Template].
package organize

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Field is a token a template names, as it is written between < and >.
type Field string

// The fields, in the order the token list offers them.
const (
	AlbumArtist  Field = "Album Artist" // The album's owner: its first album artist, else its songs' first artist.
	Artist       Field = "Artist"       // The song's first artist.
	Artists      Field = "Artists"      // Every artist of the song, joined by ", ".
	Album        Field = "Album"
	Year         Field = "Year" // The album's year.
	OriginalYear Field = "Original Year"
	Disc         Field = "Disc"       // Empty on a one-disc album.
	DiscTotal    Field = "Disc Total" // Empty on a one-disc album.
	Track        Field = "Track"      // Two digits, three past 99 songs on a disc.
	Title        Field = "Title"
	Genre        Field = "Genre"     // The album's first display genre.
	DiscName     Field = "Disc Name" // The disc's subtitle.
	Format       Field = "Format"    // FLAC, MP3 and so on.
)

// extField is the extension, which every path ends in and no template names.
const extField Field = "Ext"

// Fields lists every field a template may name, in the token list's order.
func Fields() []Field {
	return []Field{AlbumArtist, Artist, Artists, Album, Year, OriginalYear, Disc, DiscTotal, Track, Title, Genre, DiscName, Format}
}

// numeric reports whether f is a number, which may take a width.
func (f Field) numeric() bool { return f == Track || f == Disc || f == DiscTotal }

// The limits of a template.
const (
	MostTemplateRunes = 500 // The longest a template may be.
	mostWidth         = 4   // The widest a number may be padded.
	trackWidth        = 2   // A track number's width, unless a disc holds more songs.
)

// The defaults: the shape most of a library already has, and compilations
// where people keep them.
const (
	DefaultTemplate     = "<Album Artist>/[<Year> - ]<Album>/[<Disc>-]<Track> - <Title>"
	DefaultCompilations = "Compilations/<Album>/<Track> - <Title>"
)

// ErrTemplate is what a [*TemplateError] unwraps to.
var ErrTemplate = errors.New("bad template")

// TemplateError says where a template went wrong and why.
type TemplateError struct {
	// At is where, in characters (code points) from the start: the "<", "[" or
	// "]" at fault, or the template's length when it ends too soon.
	At     int
	Reason string // What is wrong, in a few words, for a person.
}

func (e *TemplateError) Error() string {
	return fmt.Sprintf("%v at %d: %s", ErrTemplate, e.At, e.Reason)
}

func (*TemplateError) Unwrap() error { return ErrTemplate }

// nodeKind is what one piece of a template is.
type nodeKind uint8

const (
	literal nodeKind = iota // Text as typed.
	token                   // A field, with its fallbacks.
	slash                   // A folder break.
	group                   // An optional group, gone when every field in it is empty.
)

// node is one piece of a template.
type node struct {
	kind   nodeKind
	text   string  // A literal's text.
	fields []Field // A token's fields, the first with a value wins.
	width  int     // A token's forced width; 0 for the field's own.
	inner  []node  // A group's pieces.
}

// Template is a parsed path template. Its zero value is no template: use
// [Parse].
type Template struct {
	text  string
	nodes []node
}

// String returns the template as it was written.
func (t Template) String() string { return t.text }

// Parse reads a template. A template is literal text, fields in angle brackets
// ("<Album>"), folder breaks ("/") and optional groups in square brackets
// ("[<Year> - ]"). A field may name fallbacks ("<Album Artist|Artist>") and a
// number may take a width ("<Track:3>"). It must end in <Title> or <Track>,
// outside any group; the extension is added after it.
//
// # Errors
//
// A [*TemplateError] saying where: an unknown field, an unclosed "<" or "[",
// a stray "]" or ">", a group inside a group or with no field, an empty folder
// name, a width on a field that isn't a number, or a template that doesn't end
// in <Title> or <Track>.
func Parse(text string) (Template, error) {
	p := parser{runes: []rune(text)}
	if len(p.runes) == 0 {
		return Template{}, &TemplateError{At: 0, Reason: "a template can't be empty"}
	}
	if len(p.runes) > MostTemplateRunes {
		return Template{}, &TemplateError{At: MostTemplateRunes, Reason: fmt.Sprintf("a template is at most %d characters", MostTemplateRunes)}
	}
	nodes, err := p.nodes(false)
	if err != nil {
		return Template{}, err
	}
	if folderErr := checkFolders(nodes, p.starts); folderErr != nil {
		return Template{}, folderErr
	}
	last := len(nodes) - 1
	if nodes[last].kind != token || (nodes[last].fields[0] != Title && nodes[last].fields[0] != Track) {
		return Template{}, &TemplateError{At: len(p.runes), Reason: "a template ends in <Title> or <Track>"}
	}
	return Template{text: text, nodes: nodes}, nil
}

// parser reads a template one character at a time.
type parser struct {
	runes  []rune
	at     int
	starts []int // Where each top-level node began, for errors about it.
}

// pieces is what one call of [parser.nodes] has read so far.
type pieces struct {
	out     []node
	text    strings.Builder
	textAt  int
	inGroup bool
}

// add appends n, which began at at.
func (p *parser) add(ps *pieces, n *node, at int) {
	ps.out = append(ps.out, *n)
	if !ps.inGroup {
		p.starts = append(p.starts, at)
	}
}

// flush ends the literal text read so far.
func (p *parser) flush(ps *pieces) {
	if ps.text.Len() > 0 {
		p.add(ps, &node{kind: literal, text: ps.text.String()}, ps.textAt)
		ps.text.Reset()
	}
}

// nodes reads pieces until the end, or a "]" when inGroup.
func (p *parser) nodes(inGroup bool) ([]node, error) {
	ps := &pieces{inGroup: inGroup}
	for p.at < len(p.runes) {
		if r := p.runes[p.at]; !strings.ContainsRune("</[]>", r) {
			if ps.text.Len() == 0 {
				ps.textAt = p.at
			}
			ps.text.WriteRune(r)
			p.at++
			continue
		}
		closed, err := p.step(ps)
		if err != nil {
			return nil, err
		}
		if closed {
			return ps.out, nil
		}
	}
	p.flush(ps)
	return ps.out, nil
}

// step reads what starts with one of the characters "</[]>": a field, a
// folder break or a group. It reports whether it read the "]" that ends the
// group being read.
func (p *parser) step(ps *pieces) (bool, error) {
	at, r := p.at, p.runes[p.at]
	p.flush(ps)
	var n node
	var err error
	switch r {
	case '<':
		n, err = p.token()
	case '/':
		p.at++
		n = node{kind: slash}
	case '[':
		if ps.inGroup {
			return false, &TemplateError{At: at, Reason: "a group can't hold another group"}
		}
		n, err = p.group()
	case ']':
		if !ps.inGroup {
			return false, &TemplateError{At: at, Reason: "this ] closes no ["}
		}
		return true, nil
	default: // '>'
		return false, &TemplateError{At: at, Reason: "this > closes no <"}
	}
	if err != nil {
		return false, err
	}
	p.add(ps, &n, at)
	return false, nil
}

// group reads an optional group, from its "[" to its "]".
func (p *parser) group() (node, error) {
	open := p.at
	p.at++
	inner, err := p.nodes(true)
	if err != nil {
		return node{}, err
	}
	if p.at >= len(p.runes) {
		return node{}, &TemplateError{At: open, Reason: "this [ is never closed"}
	}
	p.at++ // The "]".
	if !slices.ContainsFunc(inner, func(n node) bool { return n.kind == token }) {
		return node{}, &TemplateError{At: open, Reason: "a group needs a field, or it never shows"}
	}
	return node{kind: group, inner: inner}, nil
}

// token reads a field, from its "<" to its ">".
func (p *parser) token() (node, error) {
	open := p.at
	end := slices.Index(p.runes[open:], '>')
	if end < 0 || slices.ContainsFunc(p.runes[open+1:open+end], func(r rune) bool { return strings.ContainsRune("<[]/", r) }) {
		return node{}, &TemplateError{At: open, Reason: "this < is never closed"}
	}
	body := string(p.runes[open+1 : open+end])
	p.at = open + end + 1
	name, widthText, hasWidth := strings.Cut(body, ":")
	n := node{kind: token}
	for alt := range strings.SplitSeq(name, "|") {
		f, err := field(strings.TrimSpace(alt), open)
		if err != nil {
			return node{}, err
		}
		n.fields = append(n.fields, f)
	}
	if hasWidth {
		w, err := width(n.fields, widthText, open)
		if err != nil {
			return node{}, err
		}
		n.width = w
	}
	return n, nil
}

// field returns the field called name, in a token that opens at open.
func field(name string, open int) (Field, error) {
	if f, ok := lookup(name); ok {
		return f, nil
	}
	if strings.EqualFold(name, string(extField)) {
		return "", &TemplateError{At: open, Reason: "the extension is added for you"}
	}
	return "", &TemplateError{At: open, Reason: fmt.Sprintf("no field is called %q", name)}
}

// width reads the width text gives fields, in a token that opens at open.
func width(fields []Field, text string, open int) (int, error) {
	w, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || w < 1 || w > mostWidth {
		return 0, &TemplateError{At: open, Reason: fmt.Sprintf("a width is a number from 1 to %d", mostWidth)}
	}
	if slices.ContainsFunc(fields, func(f Field) bool { return !f.numeric() }) {
		return 0, &TemplateError{At: open, Reason: "only Track, Disc and Disc Total take a width"}
	}
	return w, nil
}

// lookup returns the field called name, ignoring case.
func lookup(name string) (Field, bool) {
	for _, f := range Fields() {
		if strings.EqualFold(name, string(f)) {
			return f, true
		}
	}
	return "", false
}

// checkFolders refuses a folder name that is always empty: a "/" first, last,
// or right after another. starts says where each top-level node began.
func checkFolders(nodes []node, starts []int) error {
	for i := range nodes {
		if nodes[i].kind != slash {
			continue
		}
		if i == 0 || i == len(nodes)-1 || nodes[i-1].kind == slash {
			return &TemplateError{At: starts[i], Reason: "a folder name can't be empty"}
		}
	}
	return nil
}

// Song is what a template reads of a song.
type Song struct {
	Title, Album, Genre, DiscName, Format string
	// AlbumArtist is the album's owner, as the library defines it: its first
	// album artist, else its songs' first artist.
	AlbumArtist string
	Artists     []string
	Year        int // The album's; 0 for none.
	// OriginalYear is the song's; 0 for none.
	OriginalYear int
	Disc         int // 0 for none.
	Discs        int // How many discs the album has, by the library's count.
	Track        int // 0 for none.
	DiscSongs    int // How many songs the song's disc has.
	Compilation  bool
}

// value returns what f is for s, unpadded and unsafe.
func (s *Song) value(f Field) string {
	if get, ok := values[f]; ok {
		return get(s)
	}
	return ""
}

// values read each field of a song.
//
//nolint:gochecknoglobals // A read-only table of the fields, like Fields.
var values = map[Field]func(s *Song) string{
	AlbumArtist: func(s *Song) string { return s.AlbumArtist },
	Artist: func(s *Song) string {
		if len(s.Artists) > 0 {
			return s.Artists[0]
		}
		return ""
	},
	Artists:      func(s *Song) string { return strings.Join(s.Artists, ", ") },
	Album:        func(s *Song) string { return s.Album },
	Year:         func(s *Song) string { return number(s.Year) },
	OriginalYear: func(s *Song) string { return number(s.OriginalYear) },
	Disc:         func(s *Song) string { return discs(s, s.Disc) },
	DiscTotal:    func(s *Song) string { return discs(s, s.Discs) },
	Track:        func(s *Song) string { return number(s.Track) },
	Title:        func(s *Song) string { return s.Title },
	Genre:        func(s *Song) string { return s.Genre },
	DiscName:     func(s *Song) string { return s.DiscName },
	Format:       func(s *Song) string { return strings.ToUpper(s.Format) },
}

// discs returns n for a song on an album of more than one disc, else "".
func discs(s *Song, n int) string {
	if s.Discs > 1 {
		return number(n)
	}
	return ""
}

// number returns n in decimal, or "" for 0 or less.
func number(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// caps cuts what a value may be, in bytes, to fit a long path.
type caps map[Field]int

// fill returns what token n is for s: its first field with a value, made safe
// for a path and padded.
func (n *node) fill(s *Song, c caps) string {
	for _, f := range n.fields {
		v := clean(s.value(f))
		if v == "" {
			continue
		}
		if most, ok := c[f]; ok {
			v = cut(v, most)
		}
		width := n.width
		if width == 0 && f == Track {
			width = max(trackWidth, len(strconv.Itoa(s.DiscSongs)))
		}
		if pad := width - len(v); pad > 0 && f.numeric() {
			v = strings.Repeat("0", pad) + v
		}
		return v
	}
	return ""
}

// render writes nodes for s. It reports whether a token in them had a value.
func render(b *strings.Builder, nodes []node, s *Song, c caps) bool {
	filled := false
	for i := range nodes {
		n := &nodes[i]
		switch n.kind {
		case literal:
			b.WriteString(n.text)
		case slash:
			b.WriteByte('/')
		case token:
			v := n.fill(s, c)
			filled = filled || v != ""
			b.WriteString(v)
		case group:
			var inner strings.Builder
			if render(&inner, n.inner, s, c) {
				b.WriteString(inner.String())
				filled = true
			}
		}
	}
	return filled
}

// ErrNoName means a song has nothing for its file's name: the last folder
// break's fields are all empty.
var ErrNoName = errors.New("no tags for the template")

// ErrTooLong means a song's path is past [MostPathBytes] even with its title
// and album cut short.
var ErrTooLong = errors.New("the path is too long")

// Path returns where t files s: a slash-separated path relative to its root,
// every folder and file name made [Safe], ending in ext, the file's extension
// with its dot. A path past [MostPathBytes] has its title cut first, then its
// album.
//
// # Errors
//
// [ErrNoName] when the file name would hold no field's value, and [ErrTooLong]
// when no cut makes the path short enough.
func (t Template) Path(s *Song, ext string) (string, error) {
	c := caps{}
	p, err := t.path(s, ext, c)
	if err != nil {
		return "", err
	}
	for _, f := range []Field{Title, Album} {
		over := len(p) - MostPathBytes
		if over <= 0 {
			return p, nil
		}
		c[f] = max(len(clean(s.value(f)))-over, 1)
		if p, err = t.path(s, ext, c); err != nil {
			return "", err
		}
	}
	if len(p) > MostPathBytes {
		return "", ErrTooLong
	}
	return p, nil
}

// path renders t for s within the caps c.
func (t Template) path(s *Song, ext string, c caps) (string, error) {
	var b strings.Builder
	render(&b, t.nodes, s, c)
	segments := strings.Split(b.String(), "/")
	last := len(segments) - 1
	stem := segments[last]
	if !t.nameFilled(s, c) {
		return "", ErrNoName
	}
	for i := range last {
		segments[i] = Safe(segments[i])
	}
	segments[last] = safeFile(stem, ext)
	return strings.Join(segments, "/"), nil
}

// nameFilled reports whether a field after the template's last folder break has
// a value for s.
func (t Template) nameFilled(s *Song, c caps) bool {
	from := 0
	for i := range t.nodes {
		if t.nodes[i].kind == slash {
			from = i + 1
		}
	}
	var b strings.Builder
	return render(&b, t.nodes[from:], s, c) && strings.TrimSpace(b.String()) != ""
}
