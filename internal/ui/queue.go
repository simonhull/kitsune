package ui

import (
	"fmt"
	"strings"
)

// QueueTrack is a track in the playback queue.
type QueueTrack struct {
	ID         string
	Title      string
	Artist     string
	Album      string
	AlbumID    string
	Year       int
	DurationMs int
	Format     string
}

// Queue is the playback queue panel.
type Queue struct {
	styles  *Styles
	tracks  []QueueTrack
	current int // index of currently playing track (-1 = nothing playing)
	cursor  int
	offset  int
	width   int
	height  int
	focused bool
}

// NewQueue creates an empty queue.
func NewQueue(styles *Styles) *Queue {
	return &Queue{styles: styles, current: -1}
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

// OffsetVal returns the current scroll offset (for mouse click mapping).
func (q *Queue) OffsetVal() int {
	return q.offset
}

// SetCursor sets the cursor to a specific index (clamped to valid range).
func (q *Queue) SetCursor(idx int) {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(q.tracks) {
		idx = len(q.tracks) - 1
	}
	if idx < 0 {
		idx = 0
	}
	q.cursor = idx
	q.scrollIntoView()
}

// --- Queue operations ---

func (q *Queue) Replace(tracks []QueueTrack, startIdx int) {
	q.tracks = tracks
	q.current = startIdx
	q.cursor = startIdx
	q.scrollIntoView()
}

func (q *Queue) Len() int        { return len(q.tracks) }

func (q *Queue) Current() *QueueTrack {
	if q.current >= 0 && q.current < len(q.tracks) {
		return &q.tracks[q.current]
	}
	return nil
}

func (q *Queue) Next() *QueueTrack {
	if q.current+1 < len(q.tracks) {
		q.current++
		return &q.tracks[q.current]
	}
	q.current = -1
	return nil
}

func (q *Queue) JumpTo() *QueueTrack {
	if q.cursor >= 0 && q.cursor < len(q.tracks) {
		q.current = q.cursor
		return &q.tracks[q.current]
	}
	return nil
}

func (q *Queue) Remove() bool {
	if q.cursor < 0 || q.cursor >= len(q.tracks) {
		return false
	}

	removedCurrent := q.cursor == q.current
	q.tracks = append(q.tracks[:q.cursor], q.tracks[q.cursor+1:]...)

	if q.current > q.cursor {
		q.current--
	} else if removedCurrent {
		q.current = -1
	}

	if q.cursor >= len(q.tracks) {
		q.cursor = max(0, len(q.tracks)-1)
	}
	q.scrollIntoView()
	return removedCurrent
}

func (q *Queue) MoveUp() {
	if q.cursor <= 0 || q.cursor >= len(q.tracks) {
		return
	}
	q.tracks[q.cursor], q.tracks[q.cursor-1] = q.tracks[q.cursor-1], q.tracks[q.cursor]
	if q.current == q.cursor {
		q.current--
	} else if q.current == q.cursor-1 {
		q.current++
	}
	q.cursor--
	q.scrollIntoView()
}

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
		return q.styles.QueueDim.Render("queue empty")
	}

	titleWidth := q.width - 2

	var b strings.Builder

	header := q.styles.QueueHeader.Width(q.width).Render(
		fmt.Sprintf("Queue (%d)", len(q.tracks)-q.currentOrZero()))
	b.WriteString(header)
	b.WriteByte('\n')

	listHeight := q.height - 2
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

	line := fmt.Sprintf("%s%-*s %s", prefix, availWidth, title, q.styles.QueueDim.Render(dur))

	if idx == q.cursor && q.focused {
		return q.styles.QueueCursor.Width(q.width).Render(line)
	}
	if idx == q.current {
		return q.styles.QueueNow.Render(line)
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
	if q.height <= 2 {
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
