package issues

import (
	"strconv"

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
// Involved pane has one: the conversation of the issue, read-only.
func (m *detailModal) previewContent(b panes.Box) panes.Content {
	if b.Pane != paneInvolved {
		return panes.Content{}
	}
	title := "#" + strconv.Itoa(m.number)
	c := panes.Content{TopLeft: titleForms(title, b, m.icons.Ellipsis)}
	body, ok := m.peekBody(b.Rect.Inner().W)
	if !ok {
		text := "no conversation"
		if !m.convOn || !m.conv.Ready() {
			text = "loading"
		}
		c.Body = m.theme.Muted.Render(text)
		return c
	}
	c.Body, _ = panes.Window(body, 0, b.Rect.Inner().H)
	return c
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
