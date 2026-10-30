package logview

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/errline"
)

// View renders the rows in the window and the status line, in exactly
// Height lines of Width cells. Only the rows in the window are rendered.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	var b strings.Builder
	rows := 0
	switch {
	case m.state == stateReady && len(m.vis) > 0:
		// The pager draws the rows through the hooks, which read the look.
		m.syncLook()
		m.store.look.ranges = m.pg.RangesFunc()
		m.pg.WriteBody(&b)
		rows = m.bodyHeight()
	case m.state == stateFailed:
		for _, l := range m.errorLines(m.width, m.bodyHeight()) {
			b.WriteString(m.fit(l, m.width))
			b.WriteByte('\n')
			rows++
		}
	default:
		// A message longer than the width wraps, as far as the height
		// lets it.
		for l := range strings.SplitSeq(ansi.Wrap(m.message(), m.width, ""), "\n") {
			if rows >= m.bodyHeight() {
				break
			}
			b.WriteString(m.fit(l, m.width))
			b.WriteByte('\n')
			rows++
		}
	}
	for ; rows < m.bodyHeight(); rows++ {
		spaces(&b, m.width)
		b.WriteByte('\n')
	}
	b.WriteString(m.statusLine())
	return b.String()
}

// bodyHeight returns the rows above the status line.
func (m *Model) bodyHeight() int { return max(m.height-1, 0) }

// message returns the placeholder shown instead of rows.
func (m *Model) message() string {
	s := m.styles
	switch m.state {
	case stateLoading:
		return m.spin.View() + s.Message.Render("Loading the log"+cmp.Or(s.Ellipsis, "…"))
	case stateReady:
		return s.Message.Render("No output yet.")
	default:
		return s.Message.Render("Nothing to show.")
	}
}

// errorWords returns what the view says of the failed load, and the hint
// after it.
func (m *Model) errorWords() (text, hint string) {
	if m.errorText != nil && m.err != nil {
		return m.errorText(m.err)
	}
	msg := "unknown error"
	if m.err != nil {
		msg, _, _ = strings.Cut(m.err.Error(), "\n")
	}
	return "Couldn't load the log: " + msg, ""
}

// errorLines renders the failed load in at most height lines of width
// cells, as the app's error lines are.
func (m *Model) errorLines(width, height int) []string {
	s := m.styles
	st := errline.Styles{
		Mark: s.ErrorGlyph, Separator: s.ErrorSeparator, Ellipsis: s.ErrorEllipsis,
		Text: s.LoadError, Hint: s.Message,
	}
	return errline.Fit(st, m.errText, m.errHint, width, height)
}

// appendSince appends a time since the start of a section: minutes,
// seconds and tenths under an hour, and hours, minutes and seconds after.
func appendSince(buf []byte, d time.Duration) []byte {
	d = max(d, 0)
	buf = append(buf, '+')
	if d < time.Hour {
		buf = appendTwo(buf, int(d/time.Minute))
		buf = append(buf, ':')
		buf = appendTwo(buf, int(d/time.Second%60))
		buf = append(buf, '.')
		return strconv.AppendInt(buf, int64(d/(time.Second/10)%10), 10)
	}
	buf = strconv.AppendInt(buf, int64(d/time.Hour), 10)
	buf = append(buf, ':')
	buf = appendTwo(buf, int(d/time.Minute%60))
	buf = append(buf, ':')
	return appendTwo(buf, int(d/time.Second%60))
}

// appendTwo appends n with at least two digits.
func appendTwo(buf []byte, n int) []byte {
	if n < 10 {
		buf = append(buf, '0')
	}
	return strconv.AppendInt(buf, int64(n), 10)
}

// appendDuration appends how long a step took, the way GitHub shows it:
// "0s", "42s", "1m 5s" or "1h 2m".
func appendDuration(buf []byte, d time.Duration) []byte {
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return append(strconv.AppendInt(buf, int64(d/time.Second), 10), 's')
	case d < time.Hour:
		buf = append(strconv.AppendInt(buf, int64(d/time.Minute), 10), "m "...)
		return append(strconv.AppendInt(buf, int64(d/time.Second%60), 10), 's')
	default:
		buf = append(strconv.AppendInt(buf, int64(d/time.Hour), 10), "h "...)
		return append(strconv.AppendInt(buf, int64(d/time.Minute%60), 10), 'm')
	}
}

// statusLine renders the title on the left and where the cursor is on the
// right, or the search input while it is open.
func (m *Model) statusLine() string {
	if m.pg.Prompting() {
		return m.pg.PromptView(m.width)
	}
	left := m.titleView
	switch {
	case m.opt:
		left = m.esc.prompt.wrap("-")
	case m.flash != "" && m.flashInfo:
		left = m.esc.status.wrap(m.flash)
	case m.flash != "":
		left = m.esc.noMatches.wrap(m.flash)
	case m.pg.Note() != "":
		left = m.esc.noMatches.wrap(m.pg.Note())
	}
	var parts []string
	if s := m.pg.MatchStatus(); s != "" {
		parts = append(parts, m.esc.status.wrap(s))
	}
	switch {
	case m.jumped == Warning:
		parts = append(parts, m.esc.status.wrap(fmt.Sprintf("warning %d/%d", m.at+1, len(m.warns))))
	case m.jumped == Error:
		parts = append(parts, m.esc.status.wrap(fmt.Sprintf("error %d/%d", m.at+1, len(m.errs))))
	case len(m.errs) == 1:
		parts = append(parts, m.esc.status.wrap("1 error"))
	case len(m.errs) > 1:
		parts = append(parts, m.esc.status.wrap(fmt.Sprintf("%d errors", len(m.errs))))
	}
	if m.live && m.follow {
		parts = append(parts, m.esc.status.wrap("follow"))
	}
	if m.state == stateReady && len(m.vis) > 0 && m.bodyHeight() > 0 {
		parts = append(parts, m.esc.status.wrap(m.position()))
	}
	right := strings.Join(parts, "  ")
	rw := ansi.StringWidth(right)
	if rw+2 > m.width {
		return m.fit(right, m.width)
	}
	return m.fit(left, m.width-rw-2) + "  " + right
}

// position is where the cursor is. With nothing folded it is the line of
// the log, as the gutter numbers it, and the percent counts lines. While
// folds hide rows, it is the row among those shown, and the percent
// counts rows, so a log folded to its step titles doesn't read as a few
// lines of hundreds.
func (m *Model) position() string {
	cur, bottom := m.cursorRow(), max(m.pg.Bottom(), 0)
	if len(m.vis) < len(m.rows) || m.n == 0 {
		pct := (m.locate(bottom) + 1) * 100 / len(m.vis)
		return fmt.Sprintf("row %d/%d  %d%%", m.locate(cur)+1, len(m.vis), pct)
	}
	line := min(m.rows[cur].src+1, m.n)
	pct := min(m.rows[bottom].src+1, m.n) * 100 / m.n
	return fmt.Sprintf("line %d/%d  %d%%", line, m.n, pct)
}

// spaces writes n spaces.
func spaces(b *strings.Builder, n int) {
	const blank = "                                                                "
	for n > len(blank) {
		b.WriteString(blank)
		n -= len(blank)
	}
	if n > 0 {
		b.WriteString(blank[:n])
	}
}

// fit truncates styled text to exactly width cells, ending it with the
// ellipsis where it is cut, or pads it.
func (m *Model) fit(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		tail := m.esc.ellipsis
		if m.esc.ellipsisWidth > width {
			tail = ""
		}
		s = ansi.Truncate(s, width, tail)
		w = ansi.StringWidth(s)
	}
	if w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}
