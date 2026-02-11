package ui

import (
	"strings"

	"github.com/simonhull/kitsune/internal/db"
)

// ArtistNav is the slim left panel showing just artist names.
type ArtistNav struct {
	styles  *Styles
	artists []ArtistRow
	cursor  int
	offset  int
	width   int
	height  int
	focused bool
	// selectedID is the currently filtered artist (empty = no filter).
	selectedID string
}

// ArtistRow is a minimal artist entry for the nav panel.
type ArtistRow struct {
	ID   string
	Name string
}

// NewArtistNav creates an artist nav panel and loads artists from the database.
func NewArtistNav(database *db.DB, styles *Styles) *ArtistNav {
	nav := &ArtistNav{styles: styles}
	artists, err := database.AllArtists()
	if err != nil {
		return nav
	}
	nav.artists = make([]ArtistRow, len(artists))
	for i, a := range artists {
		nav.artists[i] = ArtistRow{ID: a.ID, Name: a.Name}
	}
	return nav
}

func (n *ArtistNav) SetSize(w, h int) { n.width = w; n.height = h }
func (n *ArtistNav) SetFocused(f bool) { n.focused = f }
func (n *ArtistNav) SelectedID() string { return n.selectedID }
func (n *ArtistNav) Offset() int        { return n.offset }

// Select confirms the current cursor as the filter.
func (n *ArtistNav) Select() string {
	if n.cursor >= 0 && n.cursor < len(n.artists) {
		n.selectedID = n.artists[n.cursor].ID
		return n.selectedID
	}
	return ""
}

// ClearFilter removes the artist filter.
func (n *ArtistNav) ClearFilter() {
	n.selectedID = ""
}

// SelectByID sets cursor and selection to the given artist ID.
func (n *ArtistNav) SelectByID(artistID string) {
	for i, a := range n.artists {
		if a.ID == artistID {
			n.cursor = i
			n.selectedID = artistID
			n.scrollIntoView()
			return
		}
	}
}

// SetCursor sets cursor to a specific row.
func (n *ArtistNav) SetCursor(idx int) {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(n.artists) {
		idx = len(n.artists) - 1
	}
	if idx < 0 {
		idx = 0
	}
	n.cursor = idx
	n.scrollIntoView()
}

func (n *ArtistNav) MoveUp() {
	if n.cursor > 0 {
		n.cursor--
		n.scrollIntoView()
	}
}

func (n *ArtistNav) MoveDown() {
	if n.cursor < len(n.artists)-1 {
		n.cursor++
		n.scrollIntoView()
	}
}

func (n *ArtistNav) MoveTop() {
	n.cursor = 0
	n.scrollIntoView()
}

func (n *ArtistNav) MoveBottom() {
	if len(n.artists) > 0 {
		n.cursor = len(n.artists) - 1
		n.scrollIntoView()
	}
}

func (n *ArtistNav) HalfPageDown() {
	n.cursor += n.height / 2
	if n.cursor >= len(n.artists) {
		n.cursor = len(n.artists) - 1
	}
	n.scrollIntoView()
}

func (n *ArtistNav) HalfPageUp() {
	n.cursor -= n.height / 2
	if n.cursor < 0 {
		n.cursor = 0
	}
	n.scrollIntoView()
}

func (n *ArtistNav) View() string {
	if len(n.artists) == 0 {
		return n.styles.Dim.Render("no artists")
	}

	var b strings.Builder
	end := n.offset + n.height
	if end > len(n.artists) {
		end = len(n.artists)
	}

	for i := n.offset; i < end; i++ {
		a := n.artists[i]
		name := a.Name
		availWidth := n.width - 2 // 1 padding each side
		if len(name) > availWidth {
			name = name[:availWidth-1] + "…"
		}
		line := " " + name

		isCursor := i == n.cursor && n.focused
		isSelected := a.ID == n.selectedID

		switch {
		case isCursor:
			b.WriteString(n.styles.Cursor.Width(n.width).Render(line))
		case isSelected:
			b.WriteString(n.styles.QueueNow.Render(line))
		default:
			b.WriteString(line)
		}

		if i < end-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func (n *ArtistNav) scrollIntoView() {
	if n.height <= 0 {
		return
	}
	if n.cursor < n.offset {
		n.offset = n.cursor
	}
	if n.cursor >= n.offset+n.height {
		n.offset = n.cursor - n.height + 1
	}
}
