package ui

import (
	"fmt"
	"strings"

	"github.com/simonhull/kitsune/internal/db"
)

// ContentRowKind distinguishes row types in the content browser.
type ContentRowKind int

const (
	ContentArtist ContentRowKind = iota
	ContentAlbum
	ContentTrack
)

// ContentRow is a single row in the content browser's flat list.
type ContentRow struct {
	Kind     ContentRowKind
	ArtistID string
	AlbumID  string
	TrackID  string
	// Display fields.
	ArtistName string
	AlbumName  string
	AlbumYear  int
	TrackNum   int
	TrackTitle string
	DurationMs int
	Format     string
}

// ContentBrowser shows tracks grouped by Artist → Album, all expanded.
type ContentBrowser struct {
	styles   *Styles
	database *db.DB
	// Full data (all artists/albums/tracks).
	allRows []ContentRow
	// Filtered view (subset or all).
	visible []ContentRow
	cursor  int
	offset  int
	width   int
	height  int
	focused bool
	// Current artist filter (empty = show all).
	filterArtistID string
}

// NewContentBrowser creates and eagerly loads the content browser.
func NewContentBrowser(database *db.DB, styles *Styles) *ContentBrowser {
	cb := &ContentBrowser{
		styles:   styles,
		database: database,
		focused:  true,
	}
	cb.loadAll()
	cb.visible = cb.allRows
	return cb
}

func (cb *ContentBrowser) loadAll() {
	artists, err := cb.database.AllArtists()
	if err != nil {
		return
	}

	for _, artist := range artists {
		cb.allRows = append(cb.allRows, ContentRow{
			Kind:       ContentArtist,
			ArtistID:   artist.ID,
			ArtistName: artist.Name,
		})

		albums, err := cb.database.AlbumsForArtist(artist.ID)
		if err != nil {
			continue
		}

		for _, album := range albums {
			cb.allRows = append(cb.allRows, ContentRow{
				Kind:      ContentAlbum,
				ArtistID:  artist.ID,
				AlbumID:   album.ID,
				AlbumName: album.Name,
				AlbumYear: album.Year,
			})

			tracks, err := cb.database.TracksForAlbum(album.ID)
			if err != nil {
				continue
			}

			for _, t := range tracks {
				cb.allRows = append(cb.allRows, ContentRow{
					Kind:       ContentTrack,
					ArtistID:   artist.ID,
					AlbumID:    album.ID,
					TrackID:    t.ID,
					ArtistName: artist.Name,
					AlbumName:  album.Name,
					AlbumYear:  album.Year,
					TrackNum:   t.TrackNum,
					TrackTitle: t.Title,
					DurationMs: t.DurationMs,
					Format:     t.Format,
				})
			}
		}
	}
}

func (cb *ContentBrowser) SetSize(w, h int) { cb.width = w; cb.height = h }
func (cb *ContentBrowser) SetFocused(f bool) { cb.focused = f }
func (cb *ContentBrowser) Offset() int       { return cb.offset }

// FilterByArtist shows only the given artist's content.
func (cb *ContentBrowser) FilterByArtist(artistID string) {
	cb.filterArtistID = artistID
	cb.rebuildVisible()
	cb.cursor = 0
	cb.offset = 0
}

// ClearFilter shows all content.
func (cb *ContentBrowser) ClearFilter() {
	cb.filterArtistID = ""
	cb.visible = cb.allRows
	cb.cursor = 0
	cb.offset = 0
}

// ScrollToArtist scrolls to the given artist's header row.
func (cb *ContentBrowser) ScrollToArtist(artistID string) {
	for i, row := range cb.visible {
		if row.Kind == ContentArtist && row.ArtistID == artistID {
			cb.cursor = i
			cb.scrollIntoView()
			return
		}
	}
}

// ScrollToAlbum scrolls to the given album's header row.
func (cb *ContentBrowser) ScrollToAlbum(albumID string) {
	for i, row := range cb.visible {
		if row.Kind == ContentAlbum && row.AlbumID == albumID {
			cb.cursor = i
			cb.scrollIntoView()
			return
		}
	}
}

// ScrollToTrack scrolls to the given track row.
func (cb *ContentBrowser) ScrollToTrack(trackID string) {
	for i, row := range cb.visible {
		if row.Kind == ContentTrack && row.TrackID == trackID {
			cb.cursor = i
			cb.scrollIntoView()
			return
		}
	}
}

// CursorRow returns the current row under the cursor.
func (cb *ContentBrowser) CursorRow() *ContentRow {
	if cb.cursor >= 0 && cb.cursor < len(cb.visible) {
		return &cb.visible[cb.cursor]
	}
	return nil
}

// SetCursor sets cursor to specific row index.
func (cb *ContentBrowser) SetCursor(idx int) {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cb.visible) {
		idx = len(cb.visible) - 1
	}
	if idx < 0 {
		idx = 0
	}
	cb.cursor = idx
	cb.scrollIntoView()
}

// --- Navigation ---

func (cb *ContentBrowser) MoveUp() {
	if cb.cursor > 0 {
		cb.cursor--
		cb.scrollIntoView()
	}
}

func (cb *ContentBrowser) MoveDown() {
	if cb.cursor < len(cb.visible)-1 {
		cb.cursor++
		cb.scrollIntoView()
	}
}

func (cb *ContentBrowser) MoveTop() {
	cb.cursor = 0
	cb.scrollIntoView()
}

func (cb *ContentBrowser) MoveBottom() {
	if len(cb.visible) > 0 {
		cb.cursor = len(cb.visible) - 1
		cb.scrollIntoView()
	}
}

func (cb *ContentBrowser) HalfPageDown() {
	cb.cursor += cb.height / 2
	if cb.cursor >= len(cb.visible) {
		cb.cursor = len(cb.visible) - 1
	}
	cb.scrollIntoView()
}

func (cb *ContentBrowser) HalfPageUp() {
	cb.cursor -= cb.height / 2
	if cb.cursor < 0 {
		cb.cursor = 0
	}
	cb.scrollIntoView()
}

// --- View ---

func (cb *ContentBrowser) View() string {
	if len(cb.visible) == 0 {
		return cb.styles.Dim.Render("empty library")
	}

	var b strings.Builder
	end := cb.offset + cb.height
	if end > len(cb.visible) {
		end = len(cb.visible)
	}

	for i := cb.offset; i < end; i++ {
		row := cb.visible[i]
		line := cb.renderRow(row, i == cb.cursor)
		b.WriteString(line)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func (cb *ContentBrowser) renderRow(row ContentRow, selected bool) string {
	var line string

	switch row.Kind {
	case ContentArtist:
		line = fmt.Sprintf("  %s", row.ArtistName)

	case ContentAlbum:
		yearStr := ""
		if row.AlbumYear > 0 {
			yearStr = cb.styles.Dim.Render(fmt.Sprintf(" %d", row.AlbumYear))
		}
		line = fmt.Sprintf("    %s%s", row.AlbumName, yearStr)

	case ContentTrack:
		dur := formatDuration(row.DurationMs)
		num := fmt.Sprintf("%02d", row.TrackNum)
		overhead := 6 + 2 + 2 + 1 + len(dur) // indent(6) + num(2) + gap(2) + space(1) + dur
		titleWidth := cb.width - overhead
		if titleWidth < 5 {
			titleWidth = 5
		}
		title := row.TrackTitle
		if len(title) > titleWidth {
			title = title[:titleWidth-1] + "…"
		}
		line = fmt.Sprintf("      %s  %-*s %s", num, titleWidth, title, cb.styles.Dim.Render(dur))
	}

	if selected && cb.focused {
		return cb.styles.Cursor.Width(cb.width).Render(line)
	}
	return line
}

// --- Internal ---

func (cb *ContentBrowser) rebuildVisible() {
	if cb.filterArtistID == "" {
		cb.visible = cb.allRows
		return
	}

	cb.visible = cb.visible[:0]
	for _, row := range cb.allRows {
		if row.ArtistID == cb.filterArtistID {
			cb.visible = append(cb.visible, row)
		}
	}
}

func (cb *ContentBrowser) scrollIntoView() {
	if cb.height <= 0 {
		return
	}
	if cb.cursor < cb.offset {
		cb.offset = cb.cursor
	}
	if cb.cursor >= cb.offset+cb.height {
		cb.offset = cb.cursor - cb.height + 1
	}
}
