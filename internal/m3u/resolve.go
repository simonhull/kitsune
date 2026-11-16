package m3u

import (
	"net/url"
	"path"
	"strings"
)

// Kind says where a line's path leads.
type Kind int

const (
	// InLibrary is a path under a library root.
	InLibrary Kind = iota
	// Outside is an absolute path under no root, a Windows drive path, or a
	// relative one that climbs out of its root.
	Outside
	// NotFile is a URL of any scheme but file:.
	NotFile
)

// Ref is where a line's path leads.
type Ref struct {
	Kind Kind
	// Root and Rel are the library root and the slash-separated path under it,
	// for [InLibrary].
	Root, Rel string
	// Shown is what may be shown of the line: never an absolute path, which
	// could name a root. A relative line shows as written; an absolute one in
	// the library shows as Rel; one outside, only its last two segments after
	// "…/"; a URL, only its scheme and "//…".
	Shown string
}

// shownSegments is how many of an absolute path's last segments may be shown.
const shownSegments = 2

// Resolve says where line, a path from a playlist file, leads. The file is at
// dir, a slash-separated folder relative to root ("." for root itself); roots
// are every library root. It is string work only: nothing is looked up on a
// disk, so a symlink is never followed.
//
// Backslashes become slashes, a file: URL is decoded to its path, and any other
// URL is [NotFile]. An absolute path is in the library when a root holds it,
// component-wise, the longest such root winning; a relative one is resolved
// against the file's own folder, and is [Outside] when it climbs out of root.
func Resolve(line, root, dir string, roots []string) Ref {
	p := strings.ReplaceAll(line, `\`, "/")
	if scheme, isURL := urlScheme(p); isURL {
		if !strings.EqualFold(scheme, "file") {
			return Ref{Kind: NotFile, Shown: strings.ToLower(scheme) + "://…"}
		}
		decoded, ok := fileURLPath(p)
		if !ok {
			return Ref{Kind: Outside, Shown: lastTwo(p)}
		}
		p = decoded
	}
	switch {
	case isDrivePath(p), strings.HasPrefix(p, "//"):
		return Ref{Kind: Outside, Shown: lastTwo(p)}
	case strings.HasPrefix(p, "/"):
		return absolute(p, roots)
	}
	rel := path.Join(dir, p)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return Ref{Kind: Outside, Shown: line}
	}
	return Ref{Kind: InLibrary, Root: root, Rel: rel, Shown: line}
}

// Shown returns what may be shown of line, a path as a playlist file wrote it,
// knowing no roots: a relative path as written, a URL only its scheme and
// "//…", and an absolute path only its last two segments after "…/".
func Shown(line string) string {
	p := strings.ReplaceAll(line, `\`, "/")
	if _, isURL := urlScheme(p); !isURL && !isDrivePath(p) && !strings.HasPrefix(p, "/") {
		return line
	}
	return Resolve(line, "", ".", nil).Shown
}

// absolute resolves an absolute path against the roots, the longest root that
// holds it winning. The root comes back as configured, which tracks are stored
// under.
func absolute(p string, roots []string) Ref {
	clean := path.Clean(p)
	var best Ref
	longest := 0
	for _, r := range roots {
		cleanRoot := path.Clean(strings.ReplaceAll(r, `\`, "/"))
		if cleanRoot != "/" && strings.HasPrefix(clean, cleanRoot+"/") && len(cleanRoot) > longest {
			rel := clean[len(cleanRoot)+1:]
			best, longest = Ref{Kind: InLibrary, Root: r, Rel: rel, Shown: rel}, len(cleanRoot)
		}
	}
	if longest == 0 {
		return Ref{Kind: Outside, Shown: lastTwo(clean)}
	}
	return best
}

// urlScheme returns the scheme of a URL: at least two characters, so a drive
// letter ("C:") is never one, and followed by "//" unless it is file:, so a
// folder named "Live: 1999" is never one either.
func urlScheme(p string) (string, bool) {
	colon := strings.IndexByte(p, ':')
	if colon < len("ab") {
		return "", false
	}
	scheme := p[:colon]
	if !strings.HasPrefix(p[colon+1:], "//") && !strings.EqualFold(scheme, "file") {
		return "", false
	}
	for i := range len(scheme) {
		if !isSchemeByte(scheme[i], i == 0) {
			return "", false
		}
	}
	return scheme, true
}

// isSchemeByte reports whether c may be in a URL scheme: a letter, or after the
// first, a digit, '+', '-' or '.'.
func isSchemeByte(c byte, first bool) bool {
	if isLetter(c) {
		return true
	}
	return !first && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.')
}

// isLetter reports whether c is an ASCII letter.
func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// fileURLPath decodes a file: URL to its path, reporting false for one with
// another host or that doesn't parse. A drive path keeps its drive, and loses
// the slash before it.
func fileURLPath(p string) (string, bool) {
	u, err := url.Parse(p)
	if err != nil || (u.Host != "" && !strings.EqualFold(u.Host, "localhost")) {
		return "", false
	}
	decoded := u.Path
	if u.Opaque != "" { // "file:relative/path" has no slashes after the scheme.
		if decoded, err = url.PathUnescape(u.Opaque); err != nil {
			return "", false
		}
	}
	if decoded == "" {
		return "", false
	}
	if trimmed := strings.TrimPrefix(decoded, "/"); isDrivePath(trimmed) {
		return trimmed, true
	}
	return decoded, true
}

// isDrivePath reports whether p starts with a Windows drive, "C:".
func isDrivePath(p string) bool {
	return len(p) >= len("C:") && p[1] == ':' && isLetter(p[0])
}

// lastTwo returns the last two segments of p after "…/", which is all of an
// absolute path outside the library that may be shown.
func lastTwo(p string) string {
	parts := strings.FieldsFunc(p, func(r rune) bool { return r == '/' })
	parts = parts[max(0, len(parts)-shownSegments):]
	return "…/" + strings.Join(parts, "/")
}
