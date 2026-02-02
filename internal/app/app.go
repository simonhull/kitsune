package app

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	width  int
	height int
	ready  bool
}

func New() Model {
	return Model{}
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

	// Fill the content area
	contentHeight := m.height - 3 // header + status
	content := lipgloss.NewStyle().
		Height(contentHeight).
		Render("")

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
)
