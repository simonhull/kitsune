package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/db"
	"github.com/simonhull/kitsune/internal/player"
	"github.com/simonhull/kitsune/internal/subsonic"
	"github.com/simonhull/kitsune/internal/ui"
)

type Model struct {
	cfg     config.Config
	db      *db.DB
	client  *subsonic.Client
	spinner spinner.Model
	library *ui.Library
	player  *player.Player

	// Sync state.
	syncing bool
	syncMsg string
	syncErr string

	// Player state (cached for view).
	nowPlaying *player.NowPlaying
	paused     bool
	playErr    string

	// Layout.
	width  int
	height int
	ready  bool
}

func New(cfg config.Config, database *db.DB, client *subsonic.Client, p *player.Player) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B35"))

	return Model{
		cfg:     cfg,
		db:      database,
		client:  client,
		spinner: s,
		player:  p,
		syncing: client != nil,
	}
}

func (m Model) Init() tea.Cmd {
	if m.client != nil {
		return tea.Batch(m.spinner.Tick, m.runSync)
	}
	return func() tea.Msg {
		return syncDoneMsg{result: &subsonic.SyncResult{}}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, keys.Quit) {
			if m.player != nil {
				m.player.Stop()
			}
			return m, tea.Quit
		}

		if key.Matches(msg, keys.Pause) && m.player != nil && m.nowPlaying != nil {
			m.player.TogglePause()
			m.paused = !m.paused
			return m, nil
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
				row := m.library.CursorRow()
				if row != nil && row.Depth == 2 && row.Track != nil {
					// Enter on a track → play it.
					return m, m.playTrack(row.Track)
				}
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
			m.library.SetSize(m.width, m.libraryHeight())
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
		m.library = ui.NewLibrary(m.db)
		m.library.SetSize(m.width, m.libraryHeight())

	case syncErrMsg:
		m.syncing = false
		m.syncErr = msg.Error()
		m.library = ui.NewLibrary(m.db)
		m.library.SetSize(m.width, m.libraryHeight())

	case playStartedMsg:
		m.nowPlaying = msg.info
		m.paused = false
		m.playErr = ""
		// Resize library to account for now playing bar.
		if m.library != nil {
			m.library.SetSize(m.width, m.libraryHeight())
		}
		// Wait for track to end.
		return m, m.waitForTrackEnd

	case playErrMsg:
		m.playErr = msg.Error()

	case trackEndedMsg:
		m.nowPlaying = nil
		m.paused = false
		if m.library != nil {
			m.library.SetSize(m.width, m.libraryHeight())
		}
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
			Height(m.libraryHeight()).
			Padding(1, 2).
			Render(inner)
	} else if m.library != nil {
		content = m.library.View()
	}

	// Now playing bar.
	var nowPlaying string
	if m.nowPlaying != nil {
		nowPlaying = m.renderNowPlaying()
	}

	// Status bar.
	var statusText string
	if m.playErr != "" {
		statusText = errStyle.Render(m.playErr)
	} else if m.syncErr != "" {
		statusText = errStyle.Render("sync: " + m.syncErr)
	} else if m.syncMsg != "" {
		statusText = m.syncMsg
	} else {
		statusText = dimStyle.Render("j/k: move  l: expand  h: collapse  enter: play  space: pause  q: quit")
	}
	status := statusStyle.Width(m.width).Render(statusText)

	parts := []string{header, content}
	if nowPlaying != "" {
		parts = append(parts, nowPlaying)
	}
	parts = append(parts, status)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderNowPlaying() string {
	np := m.nowPlaying
	icon := "▶"
	if m.paused {
		icon = "⏸"
	}

	dur := formatDuration(np.DurationMs)
	info := fmt.Sprintf("%s %s — %s", icon, np.Title, np.Artist)

	// Right-align duration.
	padding := m.width - lipgloss.Width(info) - len(dur) - 4 // borders/padding
	if padding < 1 {
		padding = 1
	}

	line := fmt.Sprintf("%s%s%s", info, strings.Repeat(" ", padding), dimStyle.Render(dur))
	return nowPlayingStyle.Width(m.width).Render(line)
}

// libraryHeight returns the available height for the library panel.
func (m Model) libraryHeight() int {
	// header(2) + status(2) = 4 lines overhead.
	h := m.height - 4
	if m.nowPlaying != nil {
		h -= 2 // now playing bar.
	}
	return max(1, h)
}

// --- Messages ---

type syncDoneMsg struct {
	result *subsonic.SyncResult
}
type syncErrMsg struct{ error }
type playStartedMsg struct{ info *player.NowPlaying }
type playErrMsg struct{ error }
type trackEndedMsg struct{}

// --- Commands ---

func (m Model) runSync() tea.Msg {
	result, err := subsonic.Sync(context.Background(), m.client, m.db.Conn, slog.Default())
	if err != nil {
		return syncErrMsg{err}
	}
	return syncDoneMsg{result: result}
}

func (m Model) playTrack(track *ui.TrackNode) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil || m.player == nil {
			return playErrMsg{fmt.Errorf("no player available")}
		}

		// Determine stream format.
		format := strings.ToLower(track.Format)
		streamFormat := "" // raw
		if format == "m4a" || format == "aac" || format == "wma" {
			streamFormat = "mp3" // request transcode
		}

		streamURL := m.client.StreamURL(track.ID, streamFormat)

		info := player.NowPlaying{
			TrackID:    track.ID,
			Title:      track.Title,
			Artist:     track.Artist,
			Album:      track.Album,
			DurationMs: track.DurationMs,
			Format:     track.Format,
		}

		if err := m.player.Play(streamURL, format, info); err != nil {
			return playErrMsg{err}
		}
		return playStartedMsg{info: &info}
	}
}

func (m Model) waitForTrackEnd() tea.Msg {
	if m.player == nil {
		return nil
	}
	<-m.player.Done()
	return trackEndedMsg{}
}

func formatDuration(ms int) string {
	totalSec := ms / 1000
	min := totalSec / 60
	sec := totalSec % 60
	return fmt.Sprintf("%d:%02d", min, sec)
}

// --- Keybindings ---

var keys = struct {
	Quit     key.Binding
	Pause    key.Binding
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
	Pause:    key.NewBinding(key.WithKeys(" ")),
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

	nowPlayingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B35")).
			Bold(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderTop(true).
			BorderForeground(lipgloss.Color("#FF6B35")).
			Padding(0, 1)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Italic(true)

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF4444"))
)
