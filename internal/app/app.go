package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/db"
	"github.com/simonhull/kitsune/internal/library"
)

type Model struct {
	cfg      config.Config
	db       *db.DB
	spinner  spinner.Model
	tracks   int
	scanning bool
	scanMsg  string
	width    int
	height   int
	ready    bool
}

func New(cfg config.Config, database *db.DB) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B35"))

	return Model{
		cfg:     cfg,
		db:      database,
		spinner: s,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.startScan)
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

	case spinner.TickMsg:
		if m.scanning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case scanStartMsg:
		m.scanning = true
		return m, nil

	case scanDoneMsg:
		m.scanning = false
		m.tracks = msg.tracks
		m.scanMsg = fmt.Sprintf("scanned %d tracks in %s (%d errors)",
			msg.tracks, msg.elapsed.Round(time.Millisecond), msg.errors)
	}

	return m, nil
}

func (m Model) View() string {
	if !m.ready {
		return ""
	}

	header := headerStyle.Width(m.width).Render("🦊 kitsune")

	// Status info.
	var lines string
	if m.cfg.Library.Path != "" {
		lines = fmt.Sprintf("library: %s\n", m.cfg.Library.Path)
	} else {
		lines = dimStyle.Render("no library path — edit "+config.Path()) + "\n"
	}

	if m.scanning {
		lines += m.spinner.View() + " scanning library..."
	} else if m.scanMsg != "" {
		lines += m.scanMsg
	} else {
		lines += fmt.Sprintf("tracks: %d", m.tracks)
	}

	contentHeight := m.height - 4
	content := lipgloss.NewStyle().
		Height(contentHeight).
		Padding(1, 2).
		Render(lines)

	status := statusStyle.Width(m.width).
		Render("q: quit")

	return lipgloss.JoinVertical(lipgloss.Left, header, content, status)
}

// Messages
type scanStartMsg struct{}

type scanDoneMsg struct {
	tracks  int
	errors  int
	elapsed time.Duration
}

// Commands
func (m Model) startScan() tea.Msg {
	if m.cfg.Library.Path == "" {
		return scanDoneMsg{}
	}

	start := time.Now()
	result, _ := library.Scan(context.Background(), m.db.Conn, m.cfg.Library.Path, slog.Default())

	return scanDoneMsg{
		tracks:  result.Added,
		errors:  result.Errors,
		elapsed: time.Since(start),
	}
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
