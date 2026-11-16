// Package m3u reads and writes playlist files: .m3u and .m3u8. It is pure:
// bytes in, lines out, and paths are strings, never looked up on a disk.
//
// Reading keeps to limits, so a hostile or broken file costs a bounded read: at
// most [MostBytes] of it, [MostLines] lines and [MostLineBytes] in a line. What
// lies past a limit is counted, not read.
package m3u

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// The limits of a read.
const (
	// MostBytes is the most of a file that is read: 4 MiB.
	MostBytes = 4 << 20
	// MostLines is the most lines read, comments and blank ones included: room
	// for [MostPaths] songs with an #EXTINF each, the #EXTM3U before them and a
	// few more comments, since 20,000 would cut a full playlist's last song.
	MostLines = 25_000
	// MostLineBytes is the longest a line may be. A longer one is past the limit.
	MostLineBytes = 4096
	// MostPaths is the most paths kept: as many songs as a playlist holds.
	MostPaths = 10_000
)

// Line is a path a playlist file names, with what its #EXTINF said of it.
type Line struct {
	Number  int    // Its line number in the file, from 1.
	Path    string // As written, trimmed.
	Title   string // From the #EXTINF before it; empty without one.
	Seconds int    // From the #EXTINF before it; 0 without one or for an unknown length.
}

// Playlist is what a playlist file says.
type Playlist struct {
	Name  string // From #PLAYLIST; empty without one.
	Lines []Line
	// PastLimit counts the paths past a limit, which were not read. A file
	// longer than [MostBytes] counts what lies past it as one: it isn't read.
	PastLimit int
}

// Parse reads a playlist file's bytes. utf8 says the file is .m3u8, which is
// always UTF-8; an .m3u is UTF-8 when it is valid UTF-8, else Windows-1252. A
// byte-order mark is dropped, and a line may end in \r\n, \n or \r. truncated
// says data is the first [MostBytes] of a longer file: its last line, which may
// be cut, is not read, and the rest counts as past the limit.
func Parse(data []byte, utf8Only, truncated bool) Playlist {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if len(data) > MostBytes {
		data, truncated = data[:MostBytes], true
	}
	if truncated {
		data = data[:max(0, bytes.LastIndexAny(data, "\r\n"))]
	}
	var r reading
	for line := range splitLines(decode(data, utf8Only)) {
		r.line(line)
	}
	if truncated {
		r.p.PastLimit++
	}
	return r.p
}

// reading is a parse under way.
type reading struct {
	p      Playlist
	info   *Line // The #EXTINF waiting for its path.
	number int   // The line's number.
}

// line reads the next line.
func (r *reading) line(line string) {
	r.number++
	trimmed := strings.TrimSpace(line)
	switch {
	case r.number > MostLines || len(line) > MostLineBytes:
		if isPath(trimmed) {
			r.p.PastLimit++
		}
	case strings.HasPrefix(trimmed, "#EXTINF:"):
		l := extinf(trimmed)
		r.info = &l
	case strings.HasPrefix(trimmed, "#PLAYLIST:"):
		r.p.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "#PLAYLIST:"))
	case !isPath(trimmed):
	case len(r.p.Lines) >= MostPaths:
		r.p.PastLimit++
	default:
		l := Line{Number: r.number, Path: trimmed}
		if r.info != nil {
			l.Title, l.Seconds = r.info.Title, r.info.Seconds
		}
		r.p.Lines = append(r.p.Lines, l)
		r.info = nil
	}
}

// isPath reports whether a trimmed line names a path: it is neither blank nor
// a comment.
func isPath(trimmed string) bool { return trimmed != "" && !strings.HasPrefix(trimmed, "#") }

// decode returns data as text: UTF-8 with invalid bytes replaced, or for an
// .m3u that isn't valid UTF-8, Windows-1252.
func decode(data []byte, utf8Only bool) string {
	if utf8Only || utf8.Valid(data) {
		return strings.ToValidUTF8(string(data), "�")
	}
	out, err := charmap.Windows1252.NewDecoder().Bytes(data)
	if err != nil { // Windows-1252 decodes every byte; this can't happen.
		return strings.ToValidUTF8(string(data), "�")
	}
	return string(out)
}

// splitLines yields the lines of text, which end in \r\n, \n or \r.
func splitLines(text string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for text != "" {
			end := strings.IndexAny(text, "\r\n")
			if end < 0 {
				yield(text)
				return
			}
			if !yield(text[:end]) {
				return
			}
			if text[end] == '\r' && end+1 < len(text) && text[end+1] == '\n' {
				end++
			}
			text = text[end+1:]
		}
	}
}

// mostSeconds bounds a length an #EXTINF may give: past it, the length is unknown.
const mostSeconds = 1e9

// extinf reads "#EXTINF:<seconds>[ attributes],<title>".
func extinf(line string) Line {
	rest := strings.TrimPrefix(line, "#EXTINF:")
	head, title, _ := strings.Cut(rest, ",")
	var l Line
	l.Title = strings.TrimSpace(title)
	field, _, _ := strings.Cut(strings.TrimSpace(head), " ")
	if f, err := strconv.ParseFloat(field, 64); err == nil && f > 0 && f < mostSeconds {
		l.Seconds = int(math.Round(f))
	}
	return l
}
