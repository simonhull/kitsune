package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/simonhull/kitsune/internal/db"
)

// PaletteResult is a selectable item in the command palette.
type PaletteResult struct {
	Kind     string // "artist", "album", "track"
	ID       string
	Title    string
	Artist   string
	Album    string
	AlbumID  string
	ArtistID string
	Year     int
}

// Palette is the ctrl+p command palette / fuzzy finder overlay.
type Palette struct {
	styles   *Styles
	database *db.DB
	open     bool
	input    string
	results  []PaletteResult
	cursor   int
	width    int
	height   int
}

// NewPalette creates a command palette.
func NewPalette(database *db.DB, styles *Styles) *Palette {
	return &Palette{
		styles:   styles,
		database: database,
	}
}

// IsOpen returns whether the palette is visible.
func (p *Palette) IsOpen() bool {
	return p.open
}

// Open shows the palette and clears previous state.
func (p *Palette) Open() {
	p.open = true
	p.input = ""
	p.results = nil
	p.cursor = 0
}

// Close hides the palette.
func (p *Palette) Close() {
	p.open = false
	p.input = ""
	p.results = nil
	p.cursor = 0
}

// SetSize updates the available dimensions for the overlay.
func (p *Palette) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// Input returns the current search input.
func (p *Palette) Input() string {
	return p.input
}

// Type adds a character to the input and re-searches.
func (p *Palette) Type(ch string) {
	p.input += ch
	p.search()
}

// Backspace removes the last character.
func (p *Palette) Backspace() {
	if len(p.input) > 0 {
		p.input = p.input[:len(p.input)-1]
		p.search()
	}
}

// CursorUp moves selection up.
func (p *Palette) CursorUp() {
	if p.cursor > 0 {
		p.cursor--
	}
}

// CursorDown moves selection down.
func (p *Palette) CursorDown() {
	if p.cursor < len(p.results)-1 {
		p.cursor++
	}
}

// Selected returns the currently highlighted result, or nil.
func (p *Palette) Selected() *PaletteResult {
	if p.cursor >= 0 && p.cursor < len(p.results) {
		return &p.results[p.cursor]
	}
	return nil
}

func (p *Palette) search() {
	p.cursor = 0
	if p.input == "" {
		p.results = nil
		return
	}

	dbResults, err := p.database.Search(p.input, 50)
	if err != nil {
		p.results = nil
		return
	}

	p.results = make([]PaletteResult, len(dbResults))
	for i, r := range dbResults {
		p.results[i] = PaletteResult{
			Kind:     r.Kind,
			ID:       r.ID,
			Title:    r.Title,
			Artist:   r.Artist,
			Album:    r.Album,
			AlbumID:  r.AlbumID,
			ArtistID: r.ArtistID,
			Year:     r.Year,
		}
	}
}

// View renders the palette as a centered panel in the content area.
func (p *Palette) View() string {
	if !p.open {
		return ""
	}

	// Palette box is 60% of terminal width, centered.
	palWidth := p.width * 60 / 100
	if palWidth < 40 {
		palWidth = 40
	}
	if palWidth > p.width-4 {
		palWidth = p.width - 4
	}
	innerWidth := palWidth - 6 // border(2) + padding(4)

	// Each result is 2 lines tall; budget for input + divider + results.
	maxResultLines := p.height - 10
	if maxResultLines < 6 {
		maxResultLines = 6
	}
	maxResults := maxResultLines / 2

	// Input row.
	prompt := p.styles.NpBarFilled.Render("❯ ")
	inputText := p.input
	if len(inputText) > innerWidth-4 {
		inputText = inputText[len(inputText)-innerWidth+4:]
	}
	cursor := p.styles.NpTitle.Render("█")
	inputRow := prompt + inputText + cursor

	// Divider.
	divider := p.styles.Dim.Render(strings.Repeat("─", innerWidth))

	// Results.
	var rows []string
	rows = append(rows, inputRow)
	rows = append(rows, divider)

	if len(p.results) == 0 && p.input != "" {
		rows = append(rows, p.styles.Dim.Render("  no results"))
	} else if len(p.results) == 0 {
		rows = append(rows, p.styles.Dim.Render("  type to search artists, albums, tracks"))
	}

	// Scrolled window of results.
	offset := 0
	if p.cursor >= maxResults {
		offset = p.cursor - maxResults + 1
	}
	end := offset + maxResults
	if end > len(p.results) {
		end = len(p.results)
	}

	for i := offset; i < end; i++ {
		r := p.results[i]
		line := p.renderResult(r, i == p.cursor, innerWidth)
		rows = append(rows, line)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	box := paletteBoxStyle(p.styles).
		Width(palWidth).
		Render(content)

	// Center the box in the content area.
	return lipgloss.Place(p.width, p.height,
		lipgloss.Center, lipgloss.Top,
		box,
		lipgloss.WithWhitespaceChars(" "))
}

func (p *Palette) renderResult(r PaletteResult, selected bool, maxWidth int) string {
	var icon, primary, secondary string

	switch r.Kind {
	case "artist":
		icon = "♫ "
		primary = r.Title
		secondary = "artist"
	case "album":
		icon = "💿 "
		primary = r.Title
		detail := r.Artist
		if r.Year > 0 {
			detail += fmt.Sprintf(" (%d)", r.Year)
		}
		secondary = detail
	case "track":
		icon = "♪ "
		primary = r.Title
		secondary = r.Artist + " — " + r.Album
	}

	// Truncate.
	availWidth := maxWidth - 4 // icon + padding
	if len(primary) > availWidth {
		primary = primary[:availWidth-1] + "…"
	}

	secWidth := availWidth
	if len(secondary) > secWidth {
		secondary = secondary[:secWidth-1] + "…"
	}

	line := fmt.Sprintf("  %s%s\n  %s%s",
		icon,
		p.styles.NpTitle.Render(primary),
		strings.Repeat(" ", len(icon)),
		p.styles.Dim.Render(secondary))

	if selected {
		return paletteCursorStyle(p.styles).Width(maxWidth + 4).Render(line)
	}
	return line
}

// --- Styles ---

func paletteBoxStyle(s *Styles) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.NpBarFilled.GetForeground()).
		Padding(1, 1)
}

func paletteCursorStyle(s *Styles) lipgloss.Style {
	return lipgloss.NewStyle().
		Background(s.Cursor.GetBackground())
}
