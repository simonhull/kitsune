package app

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/db"
)

type Model struct {
	cfg      config.Config
	db       *db.DB
	tracks   int
	width    int
	height   int
	ready    bool
}

func New(cfg config.Config, database *db.DB) Model {
	return Model{
		cfg: cfg,
		db:  database,
	}
}

func (m Model) Init() tea.Cmd {
	return m.countTracks
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, keys.Quit) {
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

	case trackCountMsg:
		m.tracks = int(msg)
	}

	return m, nil
}

func (m Model) View() string {
	if !m.ready {
		return ""
	}

	header := headerStyle.Width(m.width).Render("🦊 kitsune")

	// Show config + db status.
	var lines string
	if m.cfg.Library.Path != "" {
		lines = fmt.Sprintf("library: %s", m.cfg.Library.Path)
	} else {
		lines = dimStyle.Render("no library path configured — edit " + config.Path())
	}
	lines += "\n" + fmt.Sprintf("tracks in db: %d", m.tracks)

	contentHeight := m.height - 4
	content := lipgloss.NewStyle().
		Height(contentHeight).
		Padding(1, 2).
		Render(lines)

	status := statusStyle.Width(m.width).
		Render("q: quit")

	return lipgloss.JoinVertical(lipgloss.Left, header, content, status)
}

// trackCountMsg carries the number of tracks in the database.
type trackCountMsg int

// countTracks queries the database for the total track count.
func (m Model) countTracks() tea.Msg {
	var count int
	m.db.Conn.QueryRow("SELECT COUNT(*) FROM tracks").Scan(&count)
	return trackCountMsg(count)
}

var keys = struct {
	Quit key.Binding
}{
	Quit: key.NewBinding(key.WithKeys("q", "ctrl+c")),
}

var (
	headerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FF6B35")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(lipgloss.Color("#333333")).
		Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderForeground(lipgloss.Color("#333333")).
		Padding(0, 1)

	dimStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		Italic(true)
)
