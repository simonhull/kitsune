package m3u

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Song is one song a written playlist names.
type Song struct {
	Path     string // Slash-separated, as the file will name it.
	Duration time.Duration
	Artist   string
	Title    string
}

// Write writes songs as an .m3u8: "#EXTM3U", then for each song
// "#EXTINF:<whole seconds>,<artist> - <title>" and its path. It is UTF-8 with no
// byte-order mark and \n line ends. A line break in a name becomes a space, so
// every song is two lines whatever its tags hold.
//
// # Errors
//
// What w returns.
func Write(w io.Writer, songs []Song) error {
	b := bufio.NewWriter(w)
	if _, err := b.WriteString("#EXTM3U\n"); err != nil {
		return fmt.Errorf("write m3u: %w", err)
	}
	for i := range songs {
		s := &songs[i]
		info := oneLine(s.Title)
		if artist := oneLine(s.Artist); artist != "" {
			info = artist + " - " + info
		}
		seconds := int64((s.Duration + time.Second/2) / time.Second)
		if _, err := fmt.Fprintf(b, "#EXTINF:%d,%s\n%s\n", seconds, info, oneLine(s.Path)); err != nil {
			return fmt.Errorf("write m3u: %w", err)
		}
	}
	if err := b.Flush(); err != nil {
		return fmt.Errorf("write m3u: %w", err)
	}
	return nil
}

// oneLine returns s with every line break a space.
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == ' ' {
			return ' '
		}
		return r
	}, s)
}

// RelativeTo returns the song at rel, a slash-separated path under a root, as
// a path from dir, a folder under the same root ("." for the root itself), with
// "../" where needed.
func RelativeTo(dir, rel string) string {
	from := splitPath(dir)
	to := splitPath(rel)
	same := 0
	for same < len(from) && same < len(to)-1 && from[same] == to[same] {
		same++
	}
	up := strings.Repeat("../", len(from)-same)
	return up + strings.Join(to[same:], "/")
}

// splitPath returns the segments of a cleaned slash-separated path, none for ".".
func splitPath(p string) []string {
	p = path.Clean(p)
	if p == "." || p == "/" {
		return nil
	}
	return strings.Split(strings.Trim(p, "/"), "/")
}

// MostFileName is the longest a written playlist's file name may be, in bytes,
// before its extension.
const MostFileName = 120

// FileName returns the name a playlist's file takes: name with path separators
// and control characters dropped, no leading dot, cut to [MostFileName] bytes
// on a character boundary, and ".m3u8" after it. A name with nothing left is
// "playlist".
func FileName(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	clean = strings.TrimLeft(strings.TrimSpace(clean), ".")
	for len(clean) > MostFileName {
		_, size := utf8.DecodeLastRuneInString(clean)
		clean = clean[:len(clean)-size]
	}
	clean = strings.TrimSpace(clean)
	if clean == "" {
		clean = "playlist"
	}
	return clean + ".m3u8"
}
