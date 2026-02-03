package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// NowPlayingInfo holds the data needed to render the now playing section.
type NowPlayingInfo struct {
	Title      string
	Artist     string
	Album      string
	Year       int
	ElapsedSec float64
	DurationMs int
	Paused     bool
	HasArt     bool
}

// NowPlayingPanel renders the now playing section with seek bar.
type NowPlayingPanel struct {
	styles  *Styles
	width   int
	artCols int
}

// NewNowPlayingPanel creates a new now playing panel.
func NewNowPlayingPanel(styles *Styles) *NowPlayingPanel {
	return &NowPlayingPanel{styles: styles}
}

// SetWidth updates the panel width.
func (n *NowPlayingPanel) SetWidth(width int) {
	n.width = width
}

// SetArtCols sets how many columns to reserve for album art on the left.
func (n *NowPlayingPanel) SetArtCols(cols int) {
	n.artCols = cols
}

// Height returns how many rows the now playing section needs.
func (n *NowPlayingPanel) Height() int {
	return 5
}

// View renders the now playing section.
func (n *NowPlayingPanel) View(info NowPlayingInfo) string {
	if n.width < 20 {
		return ""
	}

	artPad := 0
	if info.HasArt && n.artCols > 0 {
		artPad = n.artCols + 1
	}

	innerWidth := n.width - 4 - artPad
	if innerWidth < 20 {
		innerWidth = 20
		artPad = 0
	}

	prefix := ""
	if artPad > 0 {
		prefix = strings.Repeat(" ", artPad)
	}

	// Row 1: icon + title.
	icon := "▶"
	if info.Paused {
		icon = "⏸"
	}
	title := info.Title
	maxTitleWidth := innerWidth - 2
	if len(title) > maxTitleWidth {
		title = title[:maxTitleWidth-1] + "…"
	}
	row1 := prefix + fmt.Sprintf("%s %s", icon, n.styles.NpTitle.Render(title))

	// Row 2: artist — album (year).
	albumInfo := info.Artist
	if info.Album != "" {
		albumInfo += " — " + info.Album
	}
	if info.Year > 0 {
		albumInfo += fmt.Sprintf(" (%d)", info.Year)
	}
	if len(albumInfo) > innerWidth {
		albumInfo = albumInfo[:innerWidth-1] + "…"
	}
	row2 := prefix + n.styles.NpDim.Render(albumInfo)

	// Row 3: seek bar with timestamps.
	elapsed := int(info.ElapsedSec)
	total := info.DurationMs / 1000
	if total <= 0 {
		total = 1
	}

	elapsedStr := formatTimestamp(elapsed)
	totalStr := formatTimestamp(total)
	timeWidth := len(elapsedStr) + len(totalStr) + 3
	barWidth := innerWidth - timeWidth
	if barWidth < 10 {
		barWidth = 10
	}

	progress := float64(elapsed) / float64(total)
	if progress > 1 {
		progress = 1
	}
	if progress < 0 {
		progress = 0
	}

	filled := int(progress * float64(barWidth))
	empty := barWidth - filled

	bar := n.styles.NpBarFilled.Render(strings.Repeat("━", filled)) +
		n.styles.NpBarEmpty.Render(strings.Repeat("─", empty))

	row3 := prefix + fmt.Sprintf("%s %s %s",
		n.styles.NpTime.Render(elapsedStr),
		bar,
		n.styles.NpTime.Render(totalStr))

	content := lipgloss.JoinVertical(lipgloss.Left, row1, row2, row3)

	return n.styles.NpContainer.Width(n.width).Render(content)
}

func formatTimestamp(totalSec int) string {
	if totalSec < 0 {
		totalSec = 0
	}
	m := totalSec / 60
	s := totalSec % 60
	return fmt.Sprintf("%d:%02d", m, s)
}
