package ui

import "github.com/charmbracelet/lipgloss"

// Styles holds all lipgloss styles derived from a Theme.
type Styles struct {
	// Library panel.
	Cursor lipgloss.Style
	Dim    lipgloss.Style

	// Queue panel.
	QueueHeader lipgloss.Style
	QueueCursor lipgloss.Style
	QueueNow    lipgloss.Style
	QueueDim    lipgloss.Style

	// Now playing.
	NpContainer lipgloss.Style
	NpTitle     lipgloss.Style
	NpDim       lipgloss.Style
	NpTime      lipgloss.Style
	NpBarFilled lipgloss.Style
	NpBarEmpty  lipgloss.Style

	// App chrome.
	Header  lipgloss.Style
	Status  lipgloss.Style
	Divider lipgloss.Style
	AppDim  lipgloss.Style
	Error   lipgloss.Style

	// Now playing bar (legacy single-line, kept for reference).
	NowPlaying lipgloss.Style
}

// NewStyles creates a complete style set from a theme.
func NewStyles(t Theme) Styles {
	return Styles{
		// Library.
		Cursor: lipgloss.NewStyle().
			Background(t.Surface).
			Foreground(t.Accent),
		Dim: lipgloss.NewStyle().
			Foreground(t.Dim),

		// Queue.
		QueueHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(t.Accent).
			Padding(0, 1),
		QueueCursor: lipgloss.NewStyle().
			Background(t.Surface).
			Foreground(t.Accent),
		QueueNow: lipgloss.NewStyle().
			Foreground(t.Accent).
			Bold(true),
		QueueDim: lipgloss.NewStyle().
			Foreground(t.Dim),

		// Now playing.
		NpContainer: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderTop(true).
			BorderForeground(t.Accent).
			Padding(0, 1),
		NpTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(t.Fg),
		NpDim: lipgloss.NewStyle().
			Foreground(t.Dim),
		NpTime: lipgloss.NewStyle().
			Foreground(t.Dim),
		NpBarFilled: lipgloss.NewStyle().
			Foreground(t.Accent),
		NpBarEmpty: lipgloss.NewStyle().
			Foreground(t.Border),

		// App chrome.
		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(t.Accent).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(t.Border).
			Padding(0, 1),
		Status: lipgloss.NewStyle().
			Foreground(t.Dim).
			BorderStyle(lipgloss.NormalBorder()).
			BorderTop(true).
			BorderForeground(t.Border).
			Padding(0, 1),
		Divider: lipgloss.NewStyle().
			Foreground(t.Border),
		AppDim: lipgloss.NewStyle().
			Foreground(t.Dim).
			Italic(true),
		Error: lipgloss.NewStyle().
			Foreground(t.Error),

		NowPlaying: lipgloss.NewStyle().
			Foreground(t.Accent).
			Bold(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderTop(true).
			BorderForeground(t.Accent).
			Padding(0, 1),
	}
}
