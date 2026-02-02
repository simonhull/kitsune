package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// QueueTrack is a track in the playback queue.
type QueueTrack struct {
	ID         string
	Title      string
	Artist     string
	Album      string
	AlbumID    string
	DurationMs int
	Format     string
}

// Queue is the playback queue panel.
type Queue struct {
	tracks  []QueueTrack
	current int // index of currently playing track (-1 = nothing playing)
	cursor  int
	offset  int
	width   int
	height  int
	focused bool
}

// NewQueue creates an empty queue.
func NewQueue() *Queue {
	return &Queue{current: -1}
}

// SetSize updates the panel dimensions.
func (q *Queue) SetSize(width, height int) {
	q.width = width
	q.height = height
}

// SetFocused sets whether this panel has focus.
func (q *Queue) SetFocused(focused bool) {
	q.focused = focused
}

// --- Queue operations ---

// Replace clears the queue and adds new tracks, starting playback at startIdx.
func (q *Queue) Replace(tracks []QueueTrack, startIdx int) {
	q.tracks = tracks
	q.current = startIdx
	q.cursor = startIdx
	q.scrollIntoView()
}

// Len returns the number of tracks in the queue.
func (q *Queue) Len() int {
	return len(q.tracks)
}

// Current returns the currently playing track, or nil.
func (q *Queue) Current() *QueueTrack {
	if q.current >= 0 && q.current < len(q.tracks) {
		return &q.tracks[q.current]
	}
	return nil
}

// Next advances to the next track and returns it, or nil if at end.
func (q *Queue) Next() *QueueTrack {
	if q.current+1 < len(q.tracks) {
		q.current++
		return &q.tracks[q.current]
	}
	q.current = -1
	return nil
}

// JumpTo sets the current track to the cursor position and returns it.
func (q *Queue) JumpTo() *QueueTrack {
	if q.cursor >= 0 && q.cursor < len(q.tracks) {
		q.current = q.cursor
		return &q.tracks[q.current]
	}
	return nil
}

// Remove removes the track at the cursor. Returns true if currently playing track was removed.
func (q *Queue) Remove() bool {
	if q.cursor < 0 || q.cursor >= len(q.tracks) {
		return false
	}

	removedCurrent := q.cursor == q.current
	q.tracks = append(q.tracks[:q.cursor], q.tracks[q.cursor+1:]...)

	// Adjust current index.
	if q.current > q.cursor {
		q.current--
	} else if removedCurrent {
		// Current was removed — don't advance, let caller handle.
		q.current = -1
	}

	// Clamp cursor.
	if q.cursor >= len(q.tracks) {
		q.cursor = max(0, len(q.tracks)-1)
	}
	q.scrollIntoView()
	return removedCurrent
}

// MoveUp swaps the track at cursor with the one above it.
func (q *Queue) MoveUp() {
	if q.cursor <= 0 || q.cursor >= len(q.tracks) {
		return
	}
	q.tracks[q.cursor], q.tracks[q.cursor-1] = q.tracks[q.cursor-1], q.tracks[q.cursor]
	// Adjust current if it was one of the swapped tracks.
	if q.current == q.cursor {
		q.current--
	} else if q.current == q.cursor-1 {
		q.current++
	}
	q.cursor--
	q.scrollIntoView()
}

// MoveDown swaps the track at cursor with the one below it.
func (q *Queue) MoveDown() {
	if q.cursor < 0 || q.cursor >= len(q.tracks)-1 {
		return
	}
	q.tracks[q.cursor], q.tracks[q.cursor+1] = q.tracks[q.cursor+1], q.tracks[q.cursor]
	if q.current == q.cursor {
		q.current++
	} else if q.current == q.cursor+1 {
		q.current--
	}
	q.cursor++
	q.scrollIntoView()
}

// --- Navigation ---

func (q *Queue) CursorUp() {
	if q.cursor > 0 {
		q.cursor--
		q.scrollIntoView()
	}
}

func (q *Queue) CursorDown() {
	if q.cursor < len(q.tracks)-1 {
		q.cursor++
		q.scrollIntoView()
	}
}

// --- View ---

func (q *Queue) View() string {
	if len(q.tracks) == 0 {
		return queueDimStyle.Render("queue empty")
	}

	titleWidth := q.width - 2 // padding

	var b strings.Builder

	// Header.
	header := queueHeaderStyle.Width(q.width).Render(
		fmt.Sprintf("Queue (%d)", len(q.tracks)-q.currentOrZero()))
	b.WriteString(header)
	b.WriteByte('\n')

	listHeight := q.height - 2 // header + header newline
	if listHeight < 1 {
		return b.String()
	}

	end := q.offset + listHeight
	if end > len(q.tracks) {
		end = len(q.tracks)
	}

	for i := q.offset; i < end; i++ {
		t := q.tracks[i]
		line := q.renderTrack(t, i, titleWidth)
		b.WriteString(line)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func (q *Queue) renderTrack(t QueueTrack, idx int, maxWidth int) string {
	// Icon for current track.
	prefix := "  "
	if idx == q.current {
		prefix = "▶ "
	}

	dur := formatDuration(t.DurationMs)
	availWidth := maxWidth - len(prefix) - len(dur) - 1
	if availWidth < 5 {
		availWidth = 5
	}

	title := t.Title
	if len(title) > availWidth {
		title = title[:availWidth-1] + "…"
	}

	line := fmt.Sprintf("%s%-*s %s", prefix, availWidth, title, queueDimStyle.Render(dur))

	if idx == q.cursor && q.focused {
		return queueCursorStyle.Width(q.width).Render(line)
	}
	if idx == q.current {
		return queueNowStyle.Render(line)
	}
	return line
}

func (q *Queue) currentOrZero() int {
	if q.current >= 0 {
		return q.current
	}
	return 0
}

func (q *Queue) scrollIntoView() {
	if q.height <= 2 { // account for header
		return
	}
	listHeight := q.height - 2
	if q.cursor < q.offset {
		q.offset = q.cursor
	}
	if q.cursor >= q.offset+listHeight {
		q.offset = q.cursor - listHeight + 1
	}
}

// --- Styles ---

var (
	queueHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FF6B35")).
				Padding(0, 1)

	queueCursorStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#333333")).
				Foreground(lipgloss.Color("#FF6B35"))

	queueNowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B35")).
			Bold(true)

	queueDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))
)
