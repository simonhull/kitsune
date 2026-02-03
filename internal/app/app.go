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

type focus int

const (
	focusLibrary focus = iota
	focusQueue
)

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type Model struct {
	cfg     config.Config
	db      *db.DB
	client  *subsonic.Client
	spinner spinner.Model
	library *ui.Library
	queue   *ui.Queue
	player  *player.Player
	focus   focus
	styles  ui.Styles

	// Now playing.
	nowPlaying *ui.NowPlayingPanel
	albumArt   *ui.AlbumArt
	artData    []byte
	artAlbumID string

	// Sync state.
	syncing bool
	syncMsg string
	syncErr string

	// Player state.
	paused  bool
	playErr string

	// Layout.
	width  int
	height int
	ready  bool
}

func New(cfg config.Config, database *db.DB, client *subsonic.Client, p *player.Player) Model {
	theme := ui.LoadTheme(cfg.Theme)
	styles := ui.NewStyles(theme)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(theme.Accent)

	return Model{
		cfg:        cfg,
		db:         database,
		client:     client,
		spinner:    s,
		player:     p,
		styles:     styles,
		queue:      ui.NewQueue(&styles),
		nowPlaying: ui.NewNowPlayingPanel(&styles),
		albumArt:   ui.NewAlbumArt(8),
		syncing:    client != nil,
		focus:      focusLibrary,
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
			if m.albumArt.Supported() {
				m.albumArt.ClearAll()
			}
			return m, tea.Quit
		}

		if key.Matches(msg, keys.Pause) && m.player != nil && m.queue.Current() != nil {
			m.player.TogglePause()
			m.paused = !m.paused
			return m, nil
		}

		if key.Matches(msg, keys.Tab) && !m.syncing {
			m.toggleFocus()
			return m, nil
		}

		if !m.syncing {
			switch m.focus {
			case focusLibrary:
				return m.updateLibrary(msg)
			case focusQueue:
				return m.updateQueue(msg)
			}
		}

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.resizePanels()

	case spinner.TickMsg:
		if m.syncing {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case tickMsg:
		if m.queue.Current() != nil {
			return m, tickCmd()
		}

	case syncDoneMsg:
		m.syncing = false
		if msg.result.Tracks > 0 {
			m.syncMsg = fmt.Sprintf("%d artists · %d albums · %d tracks %s",
				msg.result.Artists, msg.result.Albums, msg.result.Tracks,
				m.styles.AppDim.Render("("+msg.result.Elapsed.Round(time.Millisecond).String()+")"))
		}
		m.library = ui.NewLibrary(m.db, &m.styles)
		m.library.SetFocused(m.focus == focusLibrary)
		m.resizePanels()

	case syncErrMsg:
		m.syncing = false
		m.syncErr = msg.Error()
		m.library = ui.NewLibrary(m.db, &m.styles)
		m.resizePanels()

	case playStartedMsg:
		m.paused = false
		m.playErr = ""
		if m.client != nil {
			if cur := m.queue.Current(); cur != nil {
				go m.client.NowPlaying(cur.ID)
			}
		}
		var artCmd tea.Cmd
		if cur := m.queue.Current(); cur != nil && cur.AlbumID != m.artAlbumID {
			artCmd = m.fetchCoverArt(cur.AlbumID)
		}
		return m, tea.Batch(m.waitForTrackEnd, tickCmd(), artCmd)

	case coverArtMsg:
		m.artData = msg.data
		m.artAlbumID = msg.albumID

	case playErrMsg:
		m.playErr = msg.Error()

	case trackEndedMsg:
		if m.client != nil {
			if cur := m.queue.Current(); cur != nil {
				go m.client.Scrobble(cur.ID)
			}
		}
		next := m.queue.Next()
		if next != nil {
			return m, m.playQueueTrack(next)
		}
		m.paused = false
		m.resizePanels()
	}

	return m, nil
}

// --- Mouse handling ---

func (m *Model) handleMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionRelease {
			return *m, nil
		}
		return m.handleMouseClick(msg.X, msg.Y)

	case tea.MouseButtonWheelUp:
		if m.focus == focusLibrary && m.library != nil {
			m.library.MoveUp()
		} else if m.focus == focusQueue {
			m.queue.CursorUp()
		}

	case tea.MouseButtonWheelDown:
		if m.focus == focusLibrary && m.library != nil {
			m.library.MoveDown()
		} else if m.focus == focusQueue {
			m.queue.CursorDown()
		}
	}

	return *m, nil
}

func (m *Model) handleMouseClick(x, y int) (Model, tea.Cmd) {
	if m.syncing || !m.ready {
		return *m, nil
	}

	headerHeight := 2
	contentTop := headerHeight
	contentH := m.contentHeight()
	contentBottom := contentTop + contentH

	libWidth, _ := m.splitWidths()

	if y >= contentTop && y < contentBottom && x < libWidth {
		if m.focus != focusLibrary {
			m.toggleFocus()
		}
		if m.library != nil {
			row := y - contentTop + m.library.Offset()
			m.library.SetCursor(row)

			// Artists/albums: expand (click again won't collapse — use h/left for that).
			// Tracks: play from here.
			if cur := m.library.CursorRow(); cur != nil {
				switch cur.Depth {
				case 0: // Artist — expand if collapsed.
					if !cur.Artist.Expanded {
						m.library.Expand()
					}
				case 1: // Album — expand if collapsed.
					if !cur.Album.Expanded {
						m.library.Expand()
					}
				case 2: // Track — play from here.
					return m.handleLibraryEnter()
				}
			}
		}
		return *m, nil
	}

	if y >= contentTop && y < contentBottom && x > libWidth {
		if m.focus != focusQueue {
			m.toggleFocus()
		}
		row := y - contentTop - 2 + m.queue.OffsetVal()
		if row >= 0 {
			m.queue.SetCursor(row)
			// Jump to and play the clicked track.
			track := m.queue.JumpTo()
			if track != nil {
				return *m, m.playQueueTrack(track)
			}
		}
		return *m, nil
	}

	return *m, nil
}

// --- Input handling per focus ---

func (m *Model) updateLibrary(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.library == nil {
		return *m, nil
	}

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
		return m.handleLibraryEnter()
	case key.Matches(msg, keys.Top):
		m.library.MoveTop()
	case key.Matches(msg, keys.Bottom):
		m.library.MoveBottom()
	case key.Matches(msg, keys.HalfDown):
		m.library.HalfPageDown()
	case key.Matches(msg, keys.HalfUp):
		m.library.HalfPageUp()
	}

	return *m, nil
}

func (m *Model) handleLibraryEnter() (Model, tea.Cmd) {
	row := m.library.CursorRow()
	if row == nil {
		return *m, nil
	}

	switch row.Depth {
	case 0:
		tracks, err := m.db.TracksForArtist(row.Artist.ID)
		if err != nil || len(tracks) == 0 {
			return *m, nil
		}
		m.replaceQueue(tracks, 0)
		return *m, m.playQueueTrack(m.queue.Current())

	case 1:
		if !row.Album.Expanded {
			m.library.Expand()
			return *m, nil
		}
		tracks, err := m.db.TracksForAlbum(row.Album.ID)
		if err != nil || len(tracks) == 0 {
			return *m, nil
		}
		m.replaceQueue(tracks, 0)
		return *m, m.playQueueTrack(m.queue.Current())

	case 2:
		tracks, err := m.db.TracksForAlbum(row.Album.ID)
		if err != nil || len(tracks) == 0 {
			return *m, nil
		}
		startIdx := 0
		for i, t := range tracks {
			if t.ID == row.Track.ID {
				startIdx = i
				break
			}
		}
		m.replaceQueue(tracks, startIdx)
		return *m, m.playQueueTrack(m.queue.Current())
	}

	return *m, nil
}

func (m *Model) updateQueue(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Up):
		m.queue.CursorUp()
	case key.Matches(msg, keys.Down):
		m.queue.CursorDown()
	case key.Matches(msg, keys.Toggle):
		track := m.queue.JumpTo()
		if track != nil {
			return *m, m.playQueueTrack(track)
		}
	case key.Matches(msg, keys.Remove):
		removedCurrent := m.queue.Remove()
		if removedCurrent {
			if m.player != nil {
				m.player.Stop()
			}
			next := m.queue.Current()
			if next != nil {
				return *m, m.playQueueTrack(next)
			}
		}
	case key.Matches(msg, keys.MoveUp):
		m.queue.MoveUp()
	case key.Matches(msg, keys.MoveDown):
		m.queue.MoveDown()
	}

	return *m, nil
}

// --- View ---

func (m Model) View() string {
	if !m.ready {
		return ""
	}

	header := m.styles.Header.Width(m.width).Render("🦊 kitsune")

	var content string
	if m.syncing {
		inner := m.spinner.View() + " syncing library..."
		content = lipgloss.NewStyle().
			Height(m.contentHeight()).
			Padding(1, 2).
			Render(inner)
	} else {
		content = m.renderSplitPanels()
	}

	// Now playing section.
	var nowPlaying string
	if cur := m.queue.Current(); cur != nil {
		elapsed := 0.0
		if m.player != nil {
			elapsed = m.player.Elapsed()
		}

		hasArt := m.albumArt.Supported() && len(m.artData) > 0 && m.artAlbumID == cur.AlbumID

		info := ui.NowPlayingInfo{
			Title:      cur.Title,
			Artist:     cur.Artist,
			Album:      cur.Album,
			Year:       cur.Year,
			ElapsedSec: elapsed,
			DurationMs: cur.DurationMs,
			Paused:     m.paused,
			HasArt:     hasArt,
		}

		nowPlaying = m.nowPlaying.View(info)
	}

	// Status bar.
	hints := "j/k: move  enter: play  space: pause  tab: switch  q: quit"
	var statusText string
	if m.playErr != "" {
		statusText = m.styles.Error.Render(m.playErr) + "  " + m.styles.AppDim.Render(hints)
	} else if m.syncErr != "" {
		statusText = m.styles.Error.Render("sync: "+m.syncErr) + "  " + m.styles.AppDim.Render(hints)
	} else {
		statusText = m.styles.AppDim.Render(hints)
	}
	status := m.styles.Status.Width(m.width).Render(statusText)

	parts := []string{header, content}
	if nowPlaying != "" {
		parts = append(parts, nowPlaying)
	}
	parts = append(parts, status)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderSplitPanels() string {
	libWidth, queueWidth := m.splitWidths()

	var libView string
	if m.library != nil {
		libView = m.library.View()
	}

	queueView := m.queue.View()

	divider := m.styles.Divider.Height(m.contentHeight()).Render("│")

	left := lipgloss.NewStyle().Width(libWidth).Height(m.contentHeight()).Render(libView)
	right := lipgloss.NewStyle().Width(queueWidth).Height(m.contentHeight()).Render(queueView)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
}

// --- Layout helpers ---

func (m Model) splitWidths() (int, int) {
	queueWidth := m.width * 30 / 100
	if queueWidth < 25 {
		queueWidth = 25
	}
	if queueWidth > 50 {
		queueWidth = 50
	}
	libWidth := m.width - queueWidth - 1
	return libWidth, queueWidth
}

func (m Model) contentHeight() int {
	h := m.height - 4
	if m.queue.Current() != nil {
		h -= m.nowPlaying.Height()
	}
	return max(1, h)
}

func (m *Model) resizePanels() {
	libWidth, queueWidth := m.splitWidths()
	ch := m.contentHeight()
	if m.library != nil {
		m.library.SetSize(libWidth, ch)
	}
	m.queue.SetSize(queueWidth, ch)
	m.nowPlaying.SetWidth(m.width)

	if m.albumArt.Supported() {
		m.nowPlaying.SetArtCols(m.albumArt.CellSize())
	}
}

func (m *Model) toggleFocus() {
	if m.focus == focusLibrary {
		m.focus = focusQueue
	} else {
		m.focus = focusLibrary
	}
	if m.library != nil {
		m.library.SetFocused(m.focus == focusLibrary)
	}
	m.queue.SetFocused(m.focus == focusQueue)
}

// --- Queue helpers ---

func (m *Model) replaceQueue(tracks []db.TrackRow, startIdx int) {
	queueTracks := make([]ui.QueueTrack, len(tracks))
	for i, t := range tracks {
		queueTracks[i] = ui.QueueTrack{
			ID:         t.ID,
			Title:      t.Title,
			Artist:     t.Artist,
			Album:      t.Album,
			AlbumID:    t.AlbumID,
			Year:       t.Year,
			DurationMs: t.DurationMs,
			Format:     t.Format,
		}
	}
	m.queue.Replace(queueTracks, startIdx)
	m.resizePanels()
}

// --- Messages ---

type syncDoneMsg struct{ result *subsonic.SyncResult }
type syncErrMsg struct{ error }
type playStartedMsg struct{}
type playErrMsg struct{ error }
type trackEndedMsg struct{}

type coverArtMsg struct {
	albumID string
	data    []byte
}

// --- Commands ---

func (m Model) runSync() tea.Msg {
	result, err := subsonic.Sync(context.Background(), m.client, m.db.Conn, slog.Default())
	if err != nil {
		return syncErrMsg{err}
	}
	return syncDoneMsg{result: result}
}

func (m Model) playQueueTrack(track *ui.QueueTrack) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil || m.player == nil || track == nil {
			return playErrMsg{fmt.Errorf("no player available")}
		}

		format := strings.ToLower(track.Format)
		streamFormat := ""
		if format == "m4a" || format == "aac" || format == "wma" {
			streamFormat = "mp3"
		}

		streamURL := m.client.StreamURL(track.ID, streamFormat)
		info := player.NowPlaying{
			TrackID:    track.ID,
			Title:      track.Title,
			Artist:     track.Artist,
			Album:      track.Album,
			AlbumID:    track.AlbumID,
			Year:       track.Year,
			DurationMs: track.DurationMs,
			Format:     track.Format,
		}

		if err := m.player.Play(streamURL, format, info); err != nil {
			return playErrMsg{err}
		}
		return playStartedMsg{}
	}
}

func (m Model) fetchCoverArt(albumID string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil || albumID == "" {
			return coverArtMsg{}
		}
		data, err := m.client.GetCoverArt(albumID, 256)
		if err != nil {
			slog.Debug("cover art fetch failed", "albumID", albumID, "err", err)
			return coverArtMsg{albumID: albumID}
		}
		return coverArtMsg{albumID: albumID, data: data}
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
	Tab      key.Binding
	Up       key.Binding
	Down     key.Binding
	Expand   key.Binding
	Collapse key.Binding
	Toggle   key.Binding
	Top      key.Binding
	Bottom   key.Binding
	HalfDown key.Binding
	HalfUp   key.Binding
	Remove   key.Binding
	MoveUp   key.Binding
	MoveDown key.Binding
}{
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c")),
	Pause:    key.NewBinding(key.WithKeys(" ")),
	Tab:      key.NewBinding(key.WithKeys("tab")),
	Up:       key.NewBinding(key.WithKeys("k", "up")),
	Down:     key.NewBinding(key.WithKeys("j", "down")),
	Expand:   key.NewBinding(key.WithKeys("l", "right")),
	Collapse: key.NewBinding(key.WithKeys("h", "left")),
	Toggle:   key.NewBinding(key.WithKeys("enter")),
	Top:      key.NewBinding(key.WithKeys("g")),
	Bottom:   key.NewBinding(key.WithKeys("G")),
	HalfDown: key.NewBinding(key.WithKeys("ctrl+d")),
	HalfUp:   key.NewBinding(key.WithKeys("ctrl+u")),
	Remove:   key.NewBinding(key.WithKeys("d")),
	MoveUp:   key.NewBinding(key.WithKeys("K")),
	MoveDown: key.NewBinding(key.WithKeys("J")),
}
