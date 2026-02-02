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
	"github.com/simonhull/kitsune/internal/subsonic"
	"github.com/simonhull/kitsune/internal/ui"
)

type Model struct {
	cfg     config.Config
	db      *db.DB
	client  *subsonic.Client
	spinner spinner.Model
	library *ui.Library

	// Sync state.
	syncing bool
	syncMsg string
	syncErr string

	// Layout.
	width  int
	height int
	ready  bool
}

func New(cfg config.Config, database *db.DB, client *subsonic.Client) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B35"))

	return Model{
		cfg:     cfg,
		db:      database,
		client:  client,
		spinner: s,
		syncing: client != nil,
	}
}

func (m Model) Init() tea.Cmd {
	if m.client != nil {
		return tea.Batch(m.spinner.Tick, m.runSync)
	}
	// No Subsonic — load library from existing DB.
	return func() tea.Msg {
		return syncDoneMsg{result: &subsonic.SyncResult{}}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, keys.Quit) {
			return m, tea.Quit
		}

		// Delegate to library when not syncing.
		if !m.syncing && m.library != nil {
			switch {
			case key.Matches(msg, keys.Up):
				m.library.MoveUp()
			case key.Matches(msg, keys.Down):
				m.library.MoveDown()
			case key.Matches(msg, keys.Expand):
				m.library.Expand()
			case key.Matches(msg, keys.Collapse):
				m.library.Collapse()
			case key.Matches(msg, keys.Toggle):
				m.library.Toggle()
			case key.Matches(msg, keys.Top):
				m.library.MoveTop()
			case key.Matches(msg, keys.Bottom):
				m.library.MoveBottom()
			case key.Matches(msg, keys.HalfDown):
				m.library.HalfPageDown()
			case key.Matches(msg, keys.HalfUp):
				m.library.HalfPageUp()
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		if m.library != nil {
			m.library.SetSize(m.width, m.contentHeight())
		}

	case spinner.TickMsg:
		if m.syncing {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case syncDoneMsg:
		m.syncing = false
		if msg.result.Tracks > 0 {
			m.syncMsg = fmt.Sprintf("%d artists · %d albums · %d tracks  %s",
				msg.result.Artists, msg.result.Albums, msg.result.Tracks,
				dimStyle.Render("(synced in "+msg.result.Elapsed.Round(time.Millisecond).String()+")"))
		}
		// Build the library tree from the DB.
		m.library = ui.NewLibrary(m.db)
		m.library.SetSize(m.width, m.contentHeight())

	case syncErrMsg:
		m.syncing = false
		m.syncErr = msg.Error()
		// Still try to load whatever's in the DB.
		m.library = ui.NewLibrary(m.db)
		m.library.SetSize(m.width, m.contentHeight())
	}

	return m, nil
}

func (m Model) View() string {
	if !m.ready {
		return ""
	}

	header := headerStyle.Width(m.width).Render("🦊 kitsune")

	var content string
	if m.syncing {
		inner := m.spinner.View() + " syncing library..."
		content = lipgloss.NewStyle().
			Height(m.contentHeight()).
			Padding(1, 2).
			Render(inner)
	} else if m.library != nil {
		content = m.library.View()
	}

	// Status bar.
	var statusText string
	if m.syncErr != "" {
		statusText = errStyle.Render("sync error: " + m.syncErr)
	} else if m.syncMsg != "" {
		statusText = m.syncMsg
	} else {
		statusText = "j/k: move  l: expand  h: collapse  q: quit"
	}
	status := statusStyle.Width(m.width).Render(statusText)

	return lipgloss.JoinVertical(lipgloss.Left, header, content, status)
}

// contentHeight returns the available height for the library panel.
func (m Model) contentHeight() int {
	// header (2 lines: text + border) + status (2 lines: border + text)
	return max(1, m.height-4)
}

// --- Messages ---

type syncDoneMsg struct {
	result *subsonic.SyncResult
}

type syncErrMsg struct{ error }

// --- Commands ---

func (m Model) runSync() tea.Msg {
	result, err := subsonic.Sync(context.Background(), m.client, m.db.Conn, slog.Default())
	if err != nil {
		return syncErrMsg{err}
	}
	return syncDoneMsg{result: result}
}

// --- Keybindings ---

var keys = struct {
	Quit     key.Binding
	Up       key.Binding
	Down     key.Binding
	Expand   key.Binding
	Collapse key.Binding
	Toggle   key.Binding
	Top      key.Binding
	Bottom   key.Binding
	HalfDown key.Binding
	HalfUp   key.Binding
}{
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c")),
	Up:       key.NewBinding(key.WithKeys("k", "up")),
	Down:     key.NewBinding(key.WithKeys("j", "down")),
	Expand:   key.NewBinding(key.WithKeys("l", "right")),
	Collapse: key.NewBinding(key.WithKeys("h", "left")),
	Toggle:   key.NewBinding(key.WithKeys("enter")),
	Top:      key.NewBinding(key.WithKeys("g")),
	Bottom:   key.NewBinding(key.WithKeys("G")),
	HalfDown: key.NewBinding(key.WithKeys("ctrl+d")),
	HalfUp:   key.NewBinding(key.WithKeys("ctrl+u")),
}

// --- Styles ---

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

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF4444"))
)
