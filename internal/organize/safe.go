package organize

import (
	"path"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// The limits of a path the template makes, in bytes of UTF-8.
const (
	// MostSegmentBytes is the longest a folder or file name may be. ext4 and
	// btrfs allow 255; the margin leaves room for a run's temp name beside it.
	MostSegmentBytes = 180
	// MostPathBytes is the longest a whole path under its root may be.
	MostPathBytes = 1000
	// mostExtBytes is the longest a name's extension may be and still count as
	// one: past it, the dot is part of the name.
	mostExtBytes = 12
)

// reserved are the characters that Windows, macOS over SMB, or a path itself
// can't hold in a name. Each becomes an underscore.
const reserved = `<>:"/\|?*`

// Safe returns segment as a folder or file name that every filesystem a
// library is read from can hold: ext4 and btrfs on the server, and Windows and
// macOS clients over SMB. The rule is fixed, in this order:
//
//  1. NFC.
//  2. Control characters (U+0000 to U+001F and U+007F) are removed.
//  3. Each of < > : " / \ | ? * becomes "_".
//  4. Runs of white space become one space, and the ends are trimmed.
//  5. Trailing dots and spaces are removed, since Windows drops them.
//  6. A leading "." or "@" becomes "_": a scan skips such a name, so a song
//     filed there would vanish from the library.
//  7. A Windows device name (CON, PRN, AUX, NUL, COM1 to COM9, LPT1 to LPT9),
//     with or without an extension, gets "_" after it.
//  8. A name past [MostSegmentBytes] is cut at a character boundary, keeping
//     its extension.
//  9. An empty name becomes "_".
//
// It is idempotent: Safe(Safe(s)) == Safe(s).
func Safe(segment string) string { return safe(segment, MostSegmentBytes) }

// safe is [Safe] with most bytes for the name.
func safe(segment string, most int) string {
	s := segment
	for range 8 { // Each round only shortens or settles a name: it ends long before.
		next := settle(clean(s), most)
		if next == s {
			break
		}
		s = next
	}
	if s == "" {
		return "_"
	}
	return s
}

// safeFile returns the safe file name of stem and ext, the extension with its
// dot, which is kept whole: the stem is cut to leave room for it.
func safeFile(stem, ext string) string {
	ext = clean(ext)
	if ext == "." || strings.ContainsRune(ext, ' ') || len(ext) > mostExtBytes {
		ext = ""
	}
	return safe(stem, MostSegmentBytes-len(ext)) + ext
}

// clean applies the character rules, 1 to 4, which a value gets before it
// joins a path and a segment gets again in [Safe].
func clean(s string) string {
	s = norm.NFC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7f:
			continue
		case isSpace(r):
			space = b.Len() > 0
			continue
		case strings.ContainsRune(reserved, r):
			r = '_'
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isSpace reports whether r is white space, as [strings.Fields] counts it.
func isSpace(r rune) bool {
	return strings.ContainsRune("\t\n\v\f\r \u0085 ", r) ||
		r == 0x1680 || (r >= 0x2000 && r <= 0x200a) || r == 0x2028 || r == 0x2029 || r == 0x202f || r == 0x205f || r == 0x3000
}

// settle applies rules 5 to 8 once to a cleaned segment, which may be no
// longer than most bytes.
func settle(s string, most int) string {
	s = strings.TrimRight(s, ". ")
	if s != "" && (s[0] == '.' || s[0] == '@') {
		s = "_" + s[1:]
	}
	s = device(s)
	if len(s) > most {
		ext := extOf(s)
		s = cut(s[:len(s)-len(ext)], most-len(ext)) + ext
	}
	return s
}

// extOf returns name's extension, with the dot. An extension holds no space
// and is at most mostExtBytes long; otherwise the dot belongs to the name, as
// in "Vol. 2", and the extension is "".
func extOf(name string) string {
	ext := path.Ext(name)
	if ext == name || len(ext) < 2 || len(ext) > mostExtBytes || strings.ContainsRune(ext, ' ') {
		return ""
	}
	return ext
}

// cut returns the longest prefix of s at most n bytes long that ends on a
// character boundary.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	n = max(n, 0)
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// devices are the names Windows keeps for itself, in upper case.
//
//nolint:gochecknoglobals // A read-only table, like reserved.
var devices = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// device adds "_" after a Windows device name, which is one with or without
// any extension: "CON", "con.flac" and "Nul.tar.gz" all are.
func device(s string) string {
	stem, rest, _ := strings.Cut(s, ".")
	if !devices[strings.ToUpper(strings.TrimRight(stem, " "))] {
		return s
	}
	if rest == "" && !strings.Contains(s, ".") {
		return stem + "_"
	}
	return stem + "_." + rest
}
