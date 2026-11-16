package m3u_test

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/m3u"
)

const (
	aMP3      = "a.mp3"
	bMP3      = "b.mp3"
	bitter    = "Falling Up/01.flac"
	srvMusic  = "/srv/music"
	lists     = "Playlists"
	nested    = "A/b.mp3"
	liveColon = "Live: 1999/a.mp3"
)

func paths(p *m3u.Playlist) []string {
	out := make([]string, len(p.Lines))
	for i := range p.Lines {
		out[i] = p.Lines[i].Path
	}
	return out
}

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		data     string
		utf8     bool
		want     []string
		wantName string
	}{
		{name: "plain", data: "a.mp3\nb.flac\n", want: []string{aMP3, "b.flac"}},
		{name: "utf8 bom", data: "\xef\xbb\xbf#EXTM3U\nCafé/é.mp3\n", utf8: true, want: []string{"Café/é.mp3"}},
		{name: "utf8 no bom in m3u", data: "Café/é.mp3\n", want: []string{"Café/é.mp3"}},
		{name: "windows-1252", data: "Caf\xe9/It\x92s\x85.mp3\n", want: []string{"Café/It’s….mp3"}},
		{name: "m3u8 invalid stays utf8", data: "Caf\xe9.mp3\n", utf8: true, want: []string{"Caf\uFFFD.mp3"}},
		{name: "crlf", data: "a.mp3\r\nb.mp3\r\n", want: []string{aMP3, bMP3}},
		{name: "lone cr", data: "a.mp3\rb.mp3\r", want: []string{aMP3, bMP3}},
		{name: "no final newline", data: "a.mp3\nb.mp3", want: []string{aMP3, bMP3}},
		{name: "comments and blanks", data: "#EXTM3U\n\n# a note\n  \na.mp3\n", want: []string{aMP3}},
		{name: "playlist name", data: "#EXTM3U\n#PLAYLIST: Night drive \na.mp3\n", want: []string{aMP3}, wantName: "Night drive"},
		{name: "trimmed", data: "  a.mp3 \t\n", want: []string{aMP3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := m3u.Parse([]byte(tt.data), tt.utf8, false)
			if got := paths(&p); !slices.Equal(got, tt.want) || p.Name != tt.wantName || p.PastLimit != 0 {
				t.Errorf("paths %q, name %q, past %d; want %q, %q, 0", got, p.Name, p.PastLimit, tt.want, tt.wantName)
			}
		})
	}
}

func TestParseExtinf(t *testing.T) {
	t.Parallel()
	data := "#EXTM3U\n#EXTINF:243,Falling Up - Bittersweet\nFalling Up/01.flac\n" +
		"#EXTINF:-1 tvg-id=\"x\",Stream, with comma\nb.mp3\nc.mp3\n#EXTINF:12.6,\nd.mp3\n"
	p := m3u.Parse([]byte(data), true, false)
	want := []m3u.Line{
		{Number: 3, Path: bitter, Title: "Falling Up - Bittersweet", Seconds: 243},
		{Number: 5, Path: bMP3, Title: "Stream, with comma"},
		{Number: 6, Path: "c.mp3"},
		{Number: 8, Path: "d.mp3", Seconds: 13},
	}
	if !slices.Equal(p.Lines, want) {
		t.Errorf("lines = %+v, want %+v", p.Lines, want)
	}
}

// wantRead fails t unless p holds the paths want and counts n past the limit.
func wantRead(t *testing.T, p *m3u.Playlist, want []string, n int) {
	t.Helper()
	if got := paths(p); !slices.Equal(got, want) || p.PastLimit != n {
		t.Errorf("paths %q past %d, want %q past %d", got, p.PastLimit, want, n)
	}
}

func TestParseLineLimit(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	for range m3u.MostLines - 1 {
		b.WriteString("#\n")
	}
	b.WriteString("last.mp3\npast1.mp3\n#c\npast2.mp3\n")
	p := m3u.Parse([]byte(b.String()), true, false)
	wantRead(t, &p, []string{"last.mp3"}, 2)
}

func TestParsePathLimit(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	for range m3u.MostPaths + 3 {
		b.WriteString("a.mp3\n")
	}
	p := m3u.Parse([]byte(b.String()), true, false)
	if len(p.Lines) != m3u.MostPaths || p.PastLimit != 3 {
		t.Errorf("kept %d past %d, want %d past 3", len(p.Lines), p.PastLimit, m3u.MostPaths)
	}
}

// A full playlist, an #EXTINF before each of its songs, reads whole.
func TestParseFullPlaylist(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for range m3u.MostPaths {
		b.WriteString("#EXTINF:1,a\na.mp3\n")
	}
	p := m3u.Parse([]byte(b.String()), true, false)
	if len(p.Lines) != m3u.MostPaths || p.PastLimit != 0 {
		t.Errorf("kept %d past %d, want all %d", len(p.Lines), p.PastLimit, m3u.MostPaths)
	}
}

func TestParseLongLine(t *testing.T) {
	t.Parallel()
	data := "a.mp3\n" + strings.Repeat("x", m3u.MostLineBytes+1) + "\nb.mp3\n"
	p := m3u.Parse([]byte(data), true, false)
	wantRead(t, &p, []string{aMP3, bMP3}, 1)
}

// A file past MostBytes is read to its last whole line under the limit, and
// the rest counts as one past it.
func TestParseByteLimit(t *testing.T) {
	t.Parallel()
	line := strings.Repeat("a", 3995) + ".mp3"
	data := bytes.Repeat([]byte(line+"\n"), m3u.MostBytes/len(line)+10)
	p := m3u.Parse(data, true, false)
	if p.PastLimit != 1 || slices.ContainsFunc(p.Lines, func(l m3u.Line) bool { return l.Path != line }) {
		t.Errorf("past %d, or a cut line was read", p.PastLimit)
	}
	p = m3u.Parse([]byte("a.mp3\nb.mp"), true, true)
	wantRead(t, &p, []string{aMP3}, 1)
}

func FuzzParse(f *testing.F) {
	f.Add([]byte("#EXTM3U\n#EXTINF:1,a\na.mp3\r\n\rb"), false, false)
	f.Add([]byte("\xef\xbb\xbf\x92\x85"), true, true)
	f.Fuzz(func(t *testing.T, data []byte, utf8Only, truncated bool) {
		p := m3u.Parse(data, utf8Only, truncated)
		if len(p.Lines) > m3u.MostPaths || p.PastLimit < 0 {
			t.Fatalf("%d lines, %d past", len(p.Lines), p.PastLimit)
		}
		last := 0
		for _, l := range p.Lines {
			if l.Number <= last || l.Number > m3u.MostLines || len(l.Path) > m3u.MostLineBytes || l.Path == "" {
				t.Fatalf("line %+v after %d", l, last)
			}
			last = l.Number
		}
	})
}

func TestResolve(t *testing.T) {
	t.Parallel()
	roots := []string{srvMusic, "/srv/music/more", "/mnt/other"}
	in := func(rel, shown string) m3u.Ref {
		return m3u.Ref{Kind: m3u.InLibrary, Root: srvMusic, Rel: rel, Shown: shown}
	}
	tests := []struct {
		name, line, dir string
		want            m3u.Ref
	}{
		{"relative", "Fangs!/04 Golden Arrows.mp3", lists, in("Playlists/Fangs!/04 Golden Arrows.mp3", "Fangs!/04 Golden Arrows.mp3")},
		{"climb", "../Falling Up/01.flac", lists, in(bitter, "../Falling Up/01.flac")},
		{"climb out", "../../x/y.mp3", lists, m3u.Ref{Kind: m3u.Outside, Shown: "../../x/y.mp3"}},
		{"backslashes", `..\Falling Up\01.flac`, lists, in(bitter, `..\Falling Up\01.flac`)},
		{"absolute", "/srv/music/A/b.mp3", ".", in(nested, nested)},
		{"longest root", "/srv/music/more/b.mp3", ".", m3u.Ref{Kind: m3u.InLibrary, Root: "/srv/music/more", Rel: bMP3, Shown: bMP3}},
		{"component-wise", "/srv/musicbox/b.mp3", ".", m3u.Ref{Kind: m3u.Outside, Shown: "…/musicbox/b.mp3"}},
		{"outside", "/home/me/Dawn Escapes/07 Moonlit.mp3", ".", m3u.Ref{Kind: m3u.Outside, Shown: "…/Dawn Escapes/07 Moonlit.mp3"}},
		{"file url", "file:///srv/music/A%20B/c%23.mp3", ".", in("A B/c#.mp3", "A B/c#.mp3")},
		{"file url localhost", "file://localhost/mnt/other/x.mp3", ".", m3u.Ref{Kind: m3u.InLibrary, Root: "/mnt/other", Rel: "x.mp3", Shown: "x.mp3"}},
		{"file url host", "file://nas/share/x.mp3", ".", m3u.Ref{Kind: m3u.Outside, Shown: "…/share/x.mp3"}},
		{"file url drive", "file:///C:/Music/a/b.mp3", ".", m3u.Ref{Kind: m3u.Outside, Shown: "…/a/b.mp3"}},
		{"https", "https://radio.example/stream.mp3", ".", m3u.Ref{Kind: m3u.NotFile, Shown: "https://…"}},
		{"http upper", "HTTP://x/y", ".", m3u.Ref{Kind: m3u.NotFile, Shown: "http://…"}},
		{"drive", `C:\Users\me\Music\a.mp3`, ".", m3u.Ref{Kind: m3u.Outside, Shown: "…/Music/a.mp3"}},
		{"unc", `\\nas\share\a.mp3`, ".", m3u.Ref{Kind: m3u.Outside, Shown: "…/share/a.mp3"}},
		{"colon folder", liveColon, ".", in(liveColon, liveColon)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := m3u.Resolve(tt.line, srvMusic, tt.dir, roots); got != tt.want {
				t.Errorf("Resolve(%q) = %+v, want %+v", tt.line, got, tt.want)
			}
		})
	}
}

func TestShown(t *testing.T) {
	t.Parallel()
	for line, want := range map[string]string{
		"../a/b.mp3":              "../a/b.mp3",
		"/srv/music/x/y/z.mp3":    "…/y/z.mp3",
		`C:\Music\x.mp3`:          "…/Music/x.mp3",
		"https://radio/x.mp3":     "https://…",
		"file:///srv/music/a.mp3": "…/music/a.mp3",
		liveColon:                 liveColon,
	} {
		if got := m3u.Shown(line); got != want {
			t.Errorf("Shown(%q) = %q, want %q", line, got, want)
		}
	}
}

func TestWrite(t *testing.T) {
	t.Parallel()
	var b bytes.Buffer
	err := m3u.Write(&b, []m3u.Song{
		{Path: "Falling Up/2004 - Crashings/01 Bittersweet.flac", Duration: 243_400 * time.Millisecond, Artist: "Falling Up", Title: "Bittersweet"},
		{Path: "a/b.mp3", Duration: time.Second, Title: "Two\nlines"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "#EXTM3U\n#EXTINF:243,Falling Up - Bittersweet\nFalling Up/2004 - Crashings/01 Bittersweet.flac\n#EXTINF:1,Two lines\na/b.mp3\n"
	if b.String() != want {
		t.Errorf("wrote %q, want %q", b.String(), want)
	}
	back := m3u.Parse(b.Bytes(), true, false)
	if got := paths(&back); !slices.Equal(got, []string{"Falling Up/2004 - Crashings/01 Bittersweet.flac", "a/b.mp3"}) {
		t.Errorf("read back %q", got)
	}
}

func TestRelativeTo(t *testing.T) {
	t.Parallel()
	tests := []struct{ dir, rel, want string }{
		{".", nested, nested},
		{"", bMP3, bMP3},
		{lists, nested, "../A/b.mp3"},
		{"Playlists/Mine", nested, "../../A/b.mp3"},
		{"A", nested, bMP3},
		{"A/B", "A/c.mp3", "../c.mp3"},
		{"A", "A/B/c.mp3", "B/c.mp3"},
	}
	for _, tt := range tests {
		if got := m3u.RelativeTo(tt.dir, tt.rel); got != tt.want {
			t.Errorf("RelativeTo(%q, %q) = %q, want %q", tt.dir, tt.rel, got, tt.want)
		}
	}
}

func TestFileName(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, want string }{
		{"Falling Up", "Falling Up.m3u8"},
		{"a/b\\c", "abc.m3u8"},
		{"..hidden", "hidden.m3u8"},
		{"tab\there", "tabhere.m3u8"},
		{"...", "playlist.m3u8"},
		{strings.Repeat("é", 70), strings.Repeat("é", 60) + ".m3u8"},
	}
	for _, tt := range tests {
		if got := m3u.FileName(tt.name); got != tt.want {
			t.Errorf("FileName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}
