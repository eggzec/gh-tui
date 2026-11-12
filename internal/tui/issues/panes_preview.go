package issues

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// peekKey names the view of the conversation that the previews draw, which
// keeps its own folds apart from the view the Conversation pane shows.
const peekKey = "preview"

// drawnPeek is the conversation drawn as a preview, kept until its width or the
// conversation changes.
type drawnPeek struct {
	ver   uint64
	width int
	body  string
	set   bool
}

// previewContent frames the preview box b of a zoomed side pane. Only the
// Involved pane has one: the conversation of the issue, read-only. focused is
// whether it has the focus.
func (m *detailModal) previewContent(b panes.Box, focused bool) panes.Content {
	if b.Pane != paneInvolved {
		return panes.Content{}
	}
	title := "#" + strconv.Itoa(m.number)
	c := panes.Content{TopLeft: titleForms(title, b, m.icons.Ellipsis), Focus: focused}
	body, ok := m.peekBody(b.Rect.Inner().W)
	if !ok {
		text := "no conversation"
		if !m.convOn || !m.conv.Ready() {
			text = "loading"
		}
		c.Body = m.theme.Muted.Render(text)
		return c
	}
	m.previewOff = min(m.previewOff, previewLast(body, b.Rect.Inner().H))
	c.Body, _ = panes.Window(body, m.previewOff, b.Rect.Inner().H)
	return c
}

// previewLast is the largest offset of body in a box rows tall.
func previewLast(body string, rows int) int {
	return max(strings.Count(body, "\n")+1-rows, 0)
}

// scrollPreview moves the preview of the zoomed Involved pane by the move msg
// is, as far as its text goes, and reports whether msg was a move. The
// preview shows the whole conversation, so one offset serves any row.
func (m *detailModal) scrollPreview(l panes.Layout, msg tea.KeyPressMsg) bool {
	b, ok := l.Box(paneInvolved, panes.Preview)
	if !ok {
		return false
	}
	rows := b.Rect.Inner().H
	last := 0
	if body, ok := m.peekBody(b.Rect.Inner().W); ok {
		last = previewLast(body, rows)
	}
	k := m.keys.panes.scroll
	off := min(m.previewOff, last)
	switch {
	case keymap.Matches(msg, k.Up):
		off--
	case keymap.Matches(msg, k.Down):
		off++
	case keymap.Matches(msg, k.PageUp):
		off -= rows
	case keymap.Matches(msg, k.PageDown):
		off += rows
	case keymap.Matches(msg, k.HalfPageUp):
		off -= max(1, rows/2)
	case keymap.Matches(msg, k.HalfPageDown):
		off += max(1, rows/2)
	case keymap.Matches(msg, k.Home):
		off = 0
	case keymap.Matches(msg, k.End):
		off = last
	default:
		return false
	}
	m.previewOff = min(max(off, 0), last)
	return true
}

// peekBody draws the conversation width cells wide, once per width until the
// conversation changes. ok is false while it is not read.
func (m *detailModal) peekBody(width int) (string, bool) {
	if !m.convOn {
		return "", false
	}
	d := &m.peek
	if d.set && d.ver == m.peekVer && d.width == width {
		return d.body, true
	}
	body, ok := m.conv.Peek(peekKey, conv.Narrow{}, width)
	if ok {
		// Until it is read the text isn't final: draw it again next time.
		*d = drawnPeek{ver: m.peekVer, width: width, body: body, set: true}
	}
	return body, ok
}
