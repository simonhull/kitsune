package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	artists  []*ArtistNode
	visible  []VisibleRow
	cursor   int
	offset   int
	width    int
	height   int
	focused  bool
}

// NewLibrary creates a library browser and loads artists from the database.
func NewLibrary(database *db.DB) *Library {
	lib := &Library{
		database: database,
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

// --- Navigation ---

// MoveUp moves the cursor up one row.
func (l *Library) MoveUp() {
	if l.cursor > 0 {
		l.cursor--
		l.scrollIntoView()
	}
}

// MoveDown moves the cursor down one row.
func (l *Library) MoveDown() {
	if l.cursor < len(l.visible)-1 {
		l.cursor++
		l.scrollIntoView()
	}
}

// MoveTop moves the cursor to the first row.
func (l *Library) MoveTop() {
	l.cursor = 0
	l.scrollIntoView()
}

// MoveBottom moves the cursor to the last row.
func (l *Library) MoveBottom() {
	if len(l.visible) > 0 {
		l.cursor = len(l.visible) - 1
		l.scrollIntoView()
	}
}

// HalfPageDown moves cursor down half a page.
func (l *Library) HalfPageDown() {
	l.cursor += l.height / 2
	if l.cursor >= len(l.visible) {
		l.cursor = len(l.visible) - 1
	}
	l.scrollIntoView()
}

// HalfPageUp moves cursor up half a page.
func (l *Library) HalfPageUp() {
	l.cursor -= l.height / 2
	if l.cursor < 0 {
		l.cursor = 0
	}
	l.scrollIntoView()
}

// Expand expands the node at the cursor (artists/albums) or is a no-op on tracks.
func (l *Library) Expand() {
	row := l.CursorRow()
	if row == nil {
		return
	}

	switch row.Depth {
	case 0: // Artist
		if !row.Artist.Expanded {
			l.loadAlbums(row.Artist)
			row.Artist.Expanded = true
			l.rebuildVisible()
		}
	case 1: // Album
		if !row.Album.Expanded {
			l.loadTracks(row.Album)
			row.Album.Expanded = true
			l.rebuildVisible()
		}
	}
}

// Collapse collapses the current node, or moves to parent if already collapsed/track.
func (l *Library) Collapse() {
	row := l.CursorRow()
	if row == nil {
		return
	}

	switch row.Depth {
	case 0: // Artist
		if row.Artist.Expanded {
			row.Artist.Expanded = false
			l.rebuildVisible()
		}
	case 1: // Album
		if row.Album.Expanded {
			row.Album.Expanded = false
			l.rebuildVisible()
		} else {
			// Move to parent artist.
			l.moveToCursorParent()
		}
	case 2: // Track — move to parent album.
		l.moveToCursorParent()
	}
}

// Toggle expands if collapsed, collapses if expanded.
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
		// Tracks don't expand — Enter will eventually play.
	}
}

// --- View ---

// View renders the tree panel.
func (l *Library) View() string {
	if len(l.visible) == 0 {
		return dimStyle.Render("empty library")
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
	case 0: // Artist
		arrow := "▸"
		if row.Artist.Expanded {
			arrow = "▾"
		}
		line = fmt.Sprintf("%s %s", arrow, row.Artist.Name)
		if row.Artist.AlbumCount > 0 {
			count := dimStyle.Render(fmt.Sprintf(" (%d)", row.Artist.AlbumCount))
			line += count
		}

	case 1: // Album
		arrow := "▸"
		if row.Album.Expanded {
			arrow = "▾"
		}
		yearStr := ""
		if row.Album.Year > 0 {
			yearStr = dimStyle.Render(fmt.Sprintf(" %d", row.Album.Year))
		}
		line = fmt.Sprintf("  %s %s%s", arrow, row.Album.Name, yearStr)

	case 2: // Track
		dur := formatDuration(row.Track.DurationMs)
		num := fmt.Sprintf("%02d", row.Track.TrackNum)
		// Right-align duration.
		titleWidth := l.width - 10 // indent(4) + num(2) + spaces(2) + duration(~5)
		if titleWidth < 10 {
			titleWidth = 10
		}
		title := row.Track.Title
		if len(title) > titleWidth {
			title = title[:titleWidth-1] + "…"
		}
		line = fmt.Sprintf("    %s  %-*s %s", num, titleWidth, title, dimStyle.Render(dur))
	}

	if selected && l.focused {
		return cursorStyle.Width(l.width).Render(line)
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
		return // Already loaded.
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
		return // Already loaded.
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

	// Clamp cursor.
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

// --- Styles ---

var (
	cursorStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#333333")).
			Foreground(lipgloss.Color("#FF6B35"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))
)
