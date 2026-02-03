package ui

import (
	"fmt"
	"strings"

	"github.com/simonhull/kitsune/internal/db"
)

// --- Tree data types ---

type ArtistNode struct {
	ID         string
	Name       string
	AlbumCount int
	Expanded   bool
	Albums     []*AlbumNode
}

type AlbumNode struct {
	ID         string
	Name       string
	Year       int
	SongCount  int
	DurationMs int
	Expanded   bool
	Tracks     []*TrackNode
}

type TrackNode struct {
	ID         string
	Title      string
	Artist     string
	Album      string
	TrackNum   int
	DiscNum    int
	DurationMs int
	Format     string
}

// VisibleRow is a flattened entry in the visible tree.
type VisibleRow struct {
	Depth  int // 0=artist, 1=album, 2=track
	Artist *ArtistNode
	Album  *AlbumNode
	Track  *TrackNode
}

// --- Library browser ---

// Library is the tree browser panel for the music library.
type Library struct {
	database *db.DB
	styles   *Styles
	artists  []*ArtistNode
	visible  []VisibleRow
	cursor   int
	offset   int
	width    int
	height   int
	focused  bool
}

// NewLibrary creates a library browser and loads artists from the database.
func NewLibrary(database *db.DB, styles *Styles) *Library {
	lib := &Library{
		database: database,
		styles:   styles,
		focused:  true,
	}
	lib.loadArtists()
	lib.rebuildVisible()
	return lib
}

// SetSize updates the panel dimensions.
func (l *Library) SetSize(width, height int) {
	l.width = width
	l.height = height
}

// SetFocused sets whether this panel has focus.
func (l *Library) SetFocused(focused bool) {
	l.focused = focused
}

// CursorRow returns the currently selected visible row, or nil.
func (l *Library) CursorRow() *VisibleRow {
	if l.cursor >= 0 && l.cursor < len(l.visible) {
		return &l.visible[l.cursor]
	}
	return nil
}

// Offset returns the current scroll offset (for mouse click mapping).
func (l *Library) Offset() int {
	return l.offset
}

// SetCursor sets the cursor to a specific row index (clamped to valid range).
func (l *Library) SetCursor(idx int) {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(l.visible) {
		idx = len(l.visible) - 1
	}
	if idx < 0 {
		idx = 0
	}
	l.cursor = idx
	l.scrollIntoView()
}

// --- Navigation ---

func (l *Library) MoveUp() {
	if l.cursor > 0 {
		l.cursor--
		l.scrollIntoView()
	}
}

func (l *Library) MoveDown() {
	if l.cursor < len(l.visible)-1 {
		l.cursor++
		l.scrollIntoView()
	}
}

func (l *Library) MoveTop() {
	l.cursor = 0
	l.scrollIntoView()
}

func (l *Library) MoveBottom() {
	if len(l.visible) > 0 {
		l.cursor = len(l.visible) - 1
		l.scrollIntoView()
	}
}

func (l *Library) HalfPageDown() {
	l.cursor += l.height / 2
	if l.cursor >= len(l.visible) {
		l.cursor = len(l.visible) - 1
	}
	l.scrollIntoView()
}

func (l *Library) HalfPageUp() {
	l.cursor -= l.height / 2
	if l.cursor < 0 {
		l.cursor = 0
	}
	l.scrollIntoView()
}

func (l *Library) Expand() {
	row := l.CursorRow()
	if row == nil {
		return
	}

	switch row.Depth {
	case 0:
		if !row.Artist.Expanded {
			l.loadAlbums(row.Artist)
			row.Artist.Expanded = true
			l.rebuildVisible()
		}
	case 1:
		if !row.Album.Expanded {
			l.loadTracks(row.Album)
			row.Album.Expanded = true
			l.rebuildVisible()
		}
	}
}

func (l *Library) Collapse() {
	row := l.CursorRow()
	if row == nil {
		return
	}

	switch row.Depth {
	case 0:
		if row.Artist.Expanded {
			row.Artist.Expanded = false
			l.rebuildVisible()
		}
	case 1:
		if row.Album.Expanded {
			row.Album.Expanded = false
			l.rebuildVisible()
		} else {
			l.moveToCursorParent()
		}
	case 2:
		l.moveToCursorParent()
	}
}

func (l *Library) Toggle() {
	row := l.CursorRow()
	if row == nil {
		return
	}

	switch row.Depth {
	case 0:
		if row.Artist.Expanded {
			l.Collapse()
		} else {
			l.Expand()
		}
	case 1:
		if row.Album.Expanded {
			l.Collapse()
		} else {
			l.Expand()
		}
	case 2:
		// Tracks don't expand.
	}
}

// --- View ---

func (l *Library) View() string {
	if len(l.visible) == 0 {
		return l.styles.Dim.Render("empty library")
	}

	var b strings.Builder
	end := l.offset + l.height
	if end > len(l.visible) {
		end = len(l.visible)
	}

	for i := l.offset; i < end; i++ {
		row := l.visible[i]
		line := l.renderRow(row, i == l.cursor)
		b.WriteString(line)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func (l *Library) renderRow(row VisibleRow, selected bool) string {
	var line string

	switch row.Depth {
	case 0:
		arrow := "▸"
		if row.Artist.Expanded {
			arrow = "▾"
		}
		line = fmt.Sprintf("%s %s", arrow, row.Artist.Name)
		if row.Artist.AlbumCount > 0 {
			count := l.styles.Dim.Render(fmt.Sprintf(" (%d)", row.Artist.AlbumCount))
			line += count
		}

	case 1:
		arrow := "▸"
		if row.Album.Expanded {
			arrow = "▾"
		}
		yearStr := ""
		if row.Album.Year > 0 {
			yearStr = l.styles.Dim.Render(fmt.Sprintf(" %d", row.Album.Year))
		}
		line = fmt.Sprintf("  %s %s%s", arrow, row.Album.Name, yearStr)

	case 2:
		dur := formatDuration(row.Track.DurationMs)
		num := fmt.Sprintf("%02d", row.Track.TrackNum)
		titleWidth := l.width - 10
		if titleWidth < 10 {
			titleWidth = 10
		}
		title := row.Track.Title
		if len(title) > titleWidth {
			title = title[:titleWidth-1] + "…"
		}
		line = fmt.Sprintf("    %s  %-*s %s", num, titleWidth, title, l.styles.Dim.Render(dur))
	}

	if selected && l.focused {
		return l.styles.Cursor.Width(l.width).Render(line)
	}
	return line
}

// --- Internal ---

func (l *Library) loadArtists() {
	artists, err := l.database.AllArtists()
	if err != nil {
		return
	}

	l.artists = make([]*ArtistNode, len(artists))
	for i, a := range artists {
		l.artists[i] = &ArtistNode{
			ID:         a.ID,
			Name:       a.Name,
			AlbumCount: a.AlbumCount,
		}
	}
}

func (l *Library) loadAlbums(artist *ArtistNode) {
	if len(artist.Albums) > 0 {
		return
	}

	albums, err := l.database.AlbumsForArtist(artist.ID)
	if err != nil {
		return
	}

	artist.Albums = make([]*AlbumNode, len(albums))
	for i, a := range albums {
		artist.Albums[i] = &AlbumNode{
			ID:         a.ID,
			Name:       a.Name,
			Year:       a.Year,
			SongCount:  a.SongCount,
			DurationMs: a.DurationMs,
		}
	}
}

func (l *Library) loadTracks(album *AlbumNode) {
	if len(album.Tracks) > 0 {
		return
	}

	tracks, err := l.database.TracksForAlbum(album.ID)
	if err != nil {
		return
	}

	album.Tracks = make([]*TrackNode, len(tracks))
	for i, t := range tracks {
		album.Tracks[i] = &TrackNode{
			ID:         t.ID,
			Title:      t.Title,
			Artist:     t.Artist,
			Album:      album.Name,
			TrackNum:   t.TrackNum,
			DiscNum:    t.DiscNum,
			DurationMs: t.DurationMs,
			Format:     t.Format,
		}
	}
}

func (l *Library) rebuildVisible() {
	l.visible = l.visible[:0]

	for _, artist := range l.artists {
		l.visible = append(l.visible, VisibleRow{Depth: 0, Artist: artist})

		if artist.Expanded {
			for _, album := range artist.Albums {
				l.visible = append(l.visible, VisibleRow{Depth: 1, Artist: artist, Album: album})

				if album.Expanded {
					for _, track := range album.Tracks {
						l.visible = append(l.visible, VisibleRow{
							Depth: 2, Artist: artist, Album: album, Track: track,
						})
					}
				}
			}
		}
	}

	if l.cursor >= len(l.visible) {
		l.cursor = max(0, len(l.visible)-1)
	}
	l.scrollIntoView()
}

func (l *Library) scrollIntoView() {
	if l.height <= 0 {
		return
	}
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+l.height {
		l.offset = l.cursor - l.height + 1
	}
}

func (l *Library) moveToCursorParent() {
	if l.cursor <= 0 {
		return
	}

	row := l.visible[l.cursor]
	targetDepth := row.Depth - 1
	if targetDepth < 0 {
		return
	}

	for i := l.cursor - 1; i >= 0; i-- {
		if l.visible[i].Depth == targetDepth {
			l.cursor = i
			l.scrollIntoView()
			return
		}
	}
}

func formatDuration(ms int) string {
	totalSec := ms / 1000
	m := totalSec / 60
	s := totalSec % 60
	return fmt.Sprintf("%d:%02d", m, s)
}
