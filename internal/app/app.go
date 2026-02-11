package app

import (
	"context"
	"math/rand/v2"
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
	focusArtistNav focus = iota
	focusContent
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
	nav     *ui.ArtistNav
	content *ui.ContentBrowser
	queue   *ui.Queue
	player  *player.Player
	focus   focus
	styles  ui.Styles

	// Now playing.
	nowPlaying *ui.NowPlayingPanel
	albumArt   *ui.AlbumArt
	artData    []byte
	artAlbumID string

	// Command palette.
	palette *ui.Palette

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
		palette:    ui.NewPalette(database, &styles),
		syncing:    client != nil,
		focus:      focusContent,
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
		// Command palette captures all input when open.
		if m.palette.IsOpen() {
			return m.updatePalette(msg)
		}

		if key.Matches(msg, keys.Quit) {
			if m.player != nil {
				m.player.Stop()
			}
			if m.albumArt.Supported() {
				m.albumArt.ClearAll()
			}
			return m, tea.Quit
		}

		if key.Matches(msg, keys.Palette) && !m.syncing {
			m.palette.SetSize(m.width, m.contentHeight())
			m.palette.Open()
			return m, nil
		}

		if key.Matches(msg, keys.Pause) && m.player != nil && m.queue.Current() != nil {
			m.player.TogglePause()
			m.paused = !m.paused
			return m, nil
		}

		if key.Matches(msg, keys.Tab) && !m.syncing {
			m.cycleFocus()
			return m, nil
		}

		if key.Matches(msg, keys.Shuffle) && !m.syncing {
			tracks := m.content.AllVisibleTracks()
			if len(tracks) > 0 {
				rand.Shuffle(len(tracks), func(i, j int) { tracks[i], tracks[j] = tracks[j], tracks[i] })
				m.replaceQueue(tracks, 0)
				return m, m.playQueueTrack(m.queue.Current())
			}
			return m, nil
		}

		if !m.syncing {
			switch m.focus {
			case focusArtistNav:
				return m.updateArtistNav(msg)
			case focusContent:
				return m.updateContent(msg)
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
		m.palette.SetSize(m.width, m.contentHeight())

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
		m.nav = ui.NewArtistNav(m.db, &m.styles)
		m.nav.SetFocused(m.focus == focusArtistNav)
		m.content = ui.NewContentBrowser(m.db, &m.styles)
		m.content.SetFocused(m.focus == focusContent)
		m.resizePanels()

	case syncErrMsg:
		m.syncing = false
		m.syncErr = msg.Error()
		m.nav = ui.NewArtistNav(m.db, &m.styles)
		m.content = ui.NewContentBrowser(m.db, &m.styles)
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
		switch m.focus {
		case focusArtistNav:
			if m.nav != nil {
				m.nav.MoveUp()
			}
		case focusContent:
			if m.content != nil {
				m.content.MoveUp()
			}
		case focusQueue:
			m.queue.CursorUp()
		}

	case tea.MouseButtonWheelDown:
		switch m.focus {
		case focusArtistNav:
			if m.nav != nil {
				m.nav.MoveDown()
			}
		case focusContent:
			if m.content != nil {
				m.content.MoveDown()
			}
		case focusQueue:
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

	navWidth, contentWidth, _ := m.tripleWidths()

	if y < contentTop || y >= contentBottom {
		return *m, nil
	}

	if x < navWidth {
		// Artist nav click.
		if m.focus != focusArtistNav {
			m.setFocus(focusArtistNav)
		}
		if m.nav != nil {
			row := y - contentTop + m.nav.Offset()
			m.nav.SetCursor(row)
			artistID := m.nav.Select()
			if artistID != "" && m.content != nil {
				m.content.FilterByArtist(artistID)
			}
		}
		return *m, nil
	}

	if x < navWidth+1+contentWidth {
		// Content browser click.
		if m.focus != focusContent {
			m.setFocus(focusContent)
		}
		if m.content != nil {
			row := y - contentTop + m.content.Offset()
			m.content.SetCursor(row)
			if cur := m.content.CursorRow(); cur != nil && cur.Kind == ui.ContentTrack {
				return m.handleContentEnter()
			}
		}
		return *m, nil
	}

	// Queue click.
	if m.focus != focusQueue {
		m.setFocus(focusQueue)
	}
	row := y - contentTop - 2 + m.queue.OffsetVal()
	if row >= 0 {
		m.queue.SetCursor(row)
		track := m.queue.JumpTo()
		if track != nil {
			return *m, m.playQueueTrack(track)
		}
	}
	return *m, nil
}

// --- Command palette ---

func (m *Model) updatePalette(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.palette.Close()
		return *m, nil

	case tea.KeyEnter:
		sel := m.palette.Selected()
		if sel == nil {
			return *m, nil
		}
		m.palette.Close()
		return m.handlePaletteSelect(sel)

	case tea.KeyUp, tea.KeyCtrlK:
		m.palette.CursorUp()
		return *m, nil

	case tea.KeyDown, tea.KeyCtrlJ:
		m.palette.CursorDown()
		return *m, nil

	case tea.KeyCtrlN:
		m.palette.CursorDown()
		return *m, nil

	case tea.KeyCtrlP:
		m.palette.CursorUp()
		return *m, nil

	case tea.KeyBackspace:
		m.palette.Backspace()
		return *m, nil

	case tea.KeySpace:
		m.palette.Type(" ")
		return *m, nil

	case tea.KeyRunes:
		m.palette.Type(string(msg.Runes))
		return *m, nil
	}

	return *m, nil
}

func (m *Model) handlePaletteSelect(sel *ui.PaletteResult) (Model, tea.Cmd) {
	switch sel.Kind {
	case "artist":
		// Navigate to artist (filter, don't play).
		if m.nav != nil {
			m.nav.SelectByID(sel.ArtistID)
		}
		if m.content != nil {
			m.content.FilterByArtist(sel.ArtistID)
		}
		return *m, nil

	case "album":
		// Filter to artist, scroll to album.
		if m.nav != nil {
			m.nav.SelectByID(sel.ArtistID)
		}
		if m.content != nil {
			m.content.FilterByArtist(sel.ArtistID)
			m.content.ScrollToAlbum(sel.AlbumID)
		}
		return *m, nil

	case "track":
		// Filter to artist, scroll to track, and play.
		if m.nav != nil {
			m.nav.SelectByID(sel.ArtistID)
		}
		if m.content != nil {
			m.content.FilterByArtist(sel.ArtistID)
			m.content.ScrollToTrack(sel.ID)
		}
		// Queue album from this track onward.
		tracks, err := m.db.TracksForAlbum(sel.AlbumID)
		if err != nil || len(tracks) == 0 {
			return *m, nil
		}
		startIdx := 0
		for i, t := range tracks {
			if t.ID == sel.ID {
				startIdx = i
				break
			}
		}
		m.replaceQueue(tracks, startIdx)
		return *m, m.playQueueTrack(m.queue.Current())
	}

	return *m, nil
}

// --- Input handling per focus ---

func (m *Model) updateArtistNav(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.nav == nil {
		return *m, nil
	}

	switch {
	case key.Matches(msg, keys.Up):
		m.nav.MoveUp()
	case key.Matches(msg, keys.Down):
		m.nav.MoveDown()
	case key.Matches(msg, keys.Toggle):
		artistID := m.nav.Select()
		if artistID != "" && m.content != nil {
			m.content.FilterByArtist(artistID)
		}
	case key.Matches(msg, keys.Collapse), key.Matches(msg, keys.Escape):
		m.nav.ClearFilter()
		if m.content != nil {
			m.content.ClearFilter()
		}
	case key.Matches(msg, keys.Top):
		m.nav.MoveTop()
	case key.Matches(msg, keys.Bottom):
		m.nav.MoveBottom()
	case key.Matches(msg, keys.HalfDown):
		m.nav.HalfPageDown()
	case key.Matches(msg, keys.HalfUp):
		m.nav.HalfPageUp()
	}

	return *m, nil
}

func (m *Model) updateContent(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.content == nil {
		return *m, nil
	}

	switch {
	case key.Matches(msg, keys.Up):
		m.content.MoveUp()
	case key.Matches(msg, keys.Down):
		m.content.MoveDown()
	case key.Matches(msg, keys.Toggle):
		return m.handleContentEnter()
	case key.Matches(msg, keys.Top):
		m.content.MoveTop()
	case key.Matches(msg, keys.Bottom):
		m.content.MoveBottom()
	case key.Matches(msg, keys.HalfDown):
		m.content.HalfPageDown()
	case key.Matches(msg, keys.HalfUp):
		m.content.HalfPageUp()
	}

	return *m, nil
}

func (m *Model) handleContentEnter() (Model, tea.Cmd) {
	row := m.content.CursorRow()
	if row == nil {
		return *m, nil
	}

	switch row.Kind {
	case ui.ContentArtist:
		tracks, err := m.db.TracksForArtist(row.ArtistID)
		if err != nil || len(tracks) == 0 {
			return *m, nil
		}
		m.replaceQueue(tracks, 0)
		return *m, m.playQueueTrack(m.queue.Current())

	case ui.ContentAlbum:
		tracks, err := m.db.TracksForAlbum(row.AlbumID)
		if err != nil || len(tracks) == 0 {
			return *m, nil
		}
		m.replaceQueue(tracks, 0)
		return *m, m.playQueueTrack(m.queue.Current())

	case ui.ContentTrack:
		tracks, err := m.db.TracksForAlbum(row.AlbumID)
		if err != nil || len(tracks) == 0 {
			return *m, nil
		}
		startIdx := 0
		for i, t := range tracks {
			if t.ID == row.TrackID {
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
	if m.palette.IsOpen() {
		content = m.palette.View()
	} else if m.syncing {
		inner := m.spinner.View() + " syncing library..."
		content = lipgloss.NewStyle().
			Height(m.contentHeight()).
			Padding(1, 2).
			Render(inner)
	} else {
		content = m.renderTriplePanels()
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
	hints := "j/k: move  enter: play  space: pause  s: shuffle  tab: switch  ctrl+p: search  q: quit"
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

func (m Model) renderTriplePanels() string {
	navWidth, contentWidth, queueWidth := m.tripleWidths()
	ch := m.contentHeight()

	var navView string
	if m.nav != nil {
		navView = m.nav.View()
	}

	var contentView string
	if m.content != nil {
		contentView = m.content.View()
	}

	queueView := m.queue.View()

	divider := m.styles.Divider.Height(ch).Render("│")

	left := lipgloss.NewStyle().Width(navWidth).Height(ch).Render(navView)
	middle := lipgloss.NewStyle().Width(contentWidth).Height(ch).Render(contentView)
	right := lipgloss.NewStyle().Width(queueWidth).Height(ch).Render(queueView)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, middle, divider, right)
}

// --- Layout helpers ---

func (m Model) tripleWidths() (int, int, int) {
	queueWidth := m.width * 30 / 100
	if queueWidth < 25 {
		queueWidth = 25
	}
	if queueWidth > 50 {
		queueWidth = 50
	}

	navWidth := m.width * 20 / 100
	if navWidth < 20 {
		navWidth = 20
	}

	// 2 dividers.
	contentWidth := m.width - navWidth - queueWidth - 2
	if contentWidth < 20 {
		contentWidth = 20
	}

	return navWidth, contentWidth, queueWidth
}

func (m Model) contentHeight() int {
	h := m.height - 4
	if m.queue.Current() != nil {
		h -= m.nowPlaying.Height()
	}
	return max(1, h)
}

func (m *Model) resizePanels() {
	navWidth, contentWidth, queueWidth := m.tripleWidths()
	ch := m.contentHeight()
	if m.nav != nil {
		m.nav.SetSize(navWidth, ch)
	}
	if m.content != nil {
		m.content.SetSize(contentWidth, ch)
	}
	m.queue.SetSize(queueWidth, ch)
	m.nowPlaying.SetWidth(m.width)

	if m.albumArt.Supported() {
		m.nowPlaying.SetArtCols(m.albumArt.CellSize())
	}
}

func (m *Model) setFocus(f focus) {
	m.focus = f
	if m.nav != nil {
		m.nav.SetFocused(f == focusArtistNav)
	}
	if m.content != nil {
		m.content.SetFocused(f == focusContent)
	}
	m.queue.SetFocused(f == focusQueue)
}

func (m *Model) cycleFocus() {
	switch m.focus {
	case focusArtistNav:
		m.setFocus(focusContent)
	case focusContent:
		m.setFocus(focusQueue)
	case focusQueue:
		m.setFocus(focusArtistNav)
	}
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
	Palette  key.Binding
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
	Escape   key.Binding
	Shuffle  key.Binding
}{
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c")),
	Pause:    key.NewBinding(key.WithKeys(" ")),
	Palette:  key.NewBinding(key.WithKeys("ctrl+p")),
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
	Escape:   key.NewBinding(key.WithKeys("esc", "backspace")),
	Shuffle:  key.NewBinding(key.WithKeys("s")),
}
