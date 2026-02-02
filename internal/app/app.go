package app

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/simonhull/kitsune/internal/config"
)

type Model struct {
	cfg    config.Config
	width  int
	height int
	ready  bool
}

func New(cfg config.Config) Model {
	return Model{cfg: cfg}
}

func (m Model) Init() tea.Cmd {
	return nil
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
	}

	return m, nil
}

func (m Model) View() string {
	if !m.ready {
		return ""
	}

	header := headerStyle.Width(m.width).Render("🦊 kitsune")

	// Show config status in the content area.
	var info string
	if m.cfg.Library.Path != "" {
		info = fmt.Sprintf("library: %s", m.cfg.Library.Path)
	} else {
		info = dimStyle.Render("no library path configured — edit " + config.Path())
	}

	contentHeight := m.height - 4 // header + info + status + borders
	content := lipgloss.NewStyle().
		Height(contentHeight).
		Padding(1, 2).
		Render(info)

	status := statusStyle.Width(m.width).
		Render("q: quit")

	return lipgloss.JoinVertical(lipgloss.Left, header, content, status)
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
