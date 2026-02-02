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
)

type Model struct {
	cfg     config.Config
	db      *db.DB
	client  *subsonic.Client
	spinner spinner.Model

	// Library stats.
	artists int
	albums  int
	tracks  int

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
		syncing: client != nil, // Start in syncing state if we have a server.
	}
}

func (m Model) Init() tea.Cmd {
	if m.client != nil {
		return tea.Batch(m.spinner.Tick, m.runSync)
	}
	// No Subsonic configured — just show what's in the DB.
	return m.loadCounts
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
		if m.syncing {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case syncDoneMsg:
		m.syncing = false
		m.artists = msg.result.Artists
		m.albums = msg.result.Albums
		m.tracks = msg.result.Tracks
		m.syncMsg = fmt.Sprintf("synced in %s", msg.result.Elapsed.Round(time.Millisecond))

	case syncErrMsg:
		m.syncing = false
		m.syncErr = msg.Error()
		// Still load whatever's in the DB.
		return m, m.loadCounts

	case countsMsg:
		m.artists = msg.artists
		m.albums = msg.albums
		m.tracks = msg.tracks
	}

	return m, nil
}

func (m Model) View() string {
	if !m.ready {
		return ""
	}

	header := headerStyle.Width(m.width).Render("🦊 kitsune")

	// Build content.
	var lines string
	if m.cfg.HasSubsonic() {
		lines = dimStyle.Render(m.cfg.Subsonic.URL) + "\n\n"
	} else {
		lines = dimStyle.Render("no subsonic server — edit "+config.Path()) + "\n\n"
	}

	if m.syncing {
		lines += m.spinner.View() + " syncing library..."
	} else if m.syncErr != "" {
		lines += errStyle.Render("sync error: "+m.syncErr) + "\n\n"
		lines += fmt.Sprintf("%d artists · %d albums · %d tracks", m.artists, m.albums, m.tracks)
	} else {
		lines += fmt.Sprintf("%d artists · %d albums · %d tracks", m.artists, m.albums, m.tracks)
		if m.syncMsg != "" {
			lines += "  " + dimStyle.Render("("+m.syncMsg+")")
		}
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

// --- Messages ---

type syncDoneMsg struct {
	result *subsonic.SyncResult
}

type syncErrMsg struct{ error }

type countsMsg struct {
	artists, albums, tracks int
}

// --- Commands ---

func (m Model) runSync() tea.Msg {
	result, err := subsonic.Sync(context.Background(), m.client, m.db.Conn, slog.Default())
	if err != nil {
		return syncErrMsg{err}
	}
	return syncDoneMsg{result: result}
}

func (m Model) loadCounts() tea.Msg {
	return countsMsg{
		artists: m.db.ArtistCount(),
		albums:  m.db.AlbumCount(),
		tracks:  m.db.TrackCount(),
	}
}

// --- Keybindings ---

var keys = struct {
	Quit key.Binding
}{
	Quit: key.NewBinding(key.WithKeys("q", "ctrl+c")),
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
