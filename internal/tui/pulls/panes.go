package pulls

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// ctxPreview is the context of the keys of the preview beside the list of a
// zoomed pane.
const ctxPreview = "pull_preview"

// The contexts of the keys of the conversation and the changed files.
const (
	ctxConversation = "pull_conversation"
	ctxFiles        = "pull_files"
)

// slot is what one pane of the modal is made of: the context of its keys,
// and what it shows.
type slot struct {
	context string
	content func(m *panesModal, b panes.Box) panes.Content
	// hovered names the row of the pane's list under the cursor, and its
	// text, for the pane's preview. ok is false when no row has anything to
	// preview, and the preview then stays on the last row that had. A pane
	// with no preview has none.
	hovered func(m *panesModal) (id, title string, ok bool)
	// preview is the whole body of the pane's preview of the hovered row.
	preview func(m *panesModal, b panes.Box) string
	// info are the forms of the right-hand info of the preview's frame, such as
	// the state of the hovered row. A pane may have none.
	info func(m *panesModal, b panes.Box) []string
	// press takes a key that the modal doesn't, while the pane's list has the
	// focus. ok is false for a key it has no use for.
	press func(m *panesModal, k tea.KeyPressMsg) (cmd tea.Cmd, ok bool)
	// title is the title of the preview's frame, in place of the hovered row's
	// text, for a preview that follows more than the row. A pane may have none.
	title func(m *panesModal, b panes.Box) string
	// pressPreview takes a key while the preview has the focus. A preview with
	// none only scrolls.
	pressPreview func(m *panesModal, k tea.KeyPressMsg) tea.Cmd
}

// slots has one entry per pane, in the order of the panes.
var slots = [paneCount]slot{
	paneDescription - 1:  {context: ctxDescription, content: (*panesModal).description, press: (*panesModal).pressDescription},
	paneConversation - 1: {context: ctxConversation, content: (*panesModal).convContent, press: (*panesModal).convPress},
	paneMerge - 1: {
		context: ctxMerge, content: (*panesModal).mergeContent, press: (*panesModal).pressMerge,
		hovered: (*panesModal).hoveredMerge, preview: (*panesModal).mergePreview, info: (*panesModal).mergeInfo,
	},
	paneInvolved - 1: {
		context: ctxInvolved, content: (*panesModal).involvedContent, press: (*panesModal).involvedPress,
		hovered: (*panesModal).hoveredInvolved, preview: (*panesModal).involvedPreview, info: (*panesModal).involvedInfo,
	},
	paneDevelopment - 1: devSlot(),
	paneFiles - 1:       filesSlot(),
}

// placeholder is what a pane shows until it has content of its own. Its
// title is the pane's.
func (m *panesModal) placeholder(panes.Box) panes.Content {
	return panes.Content{Body: m.theme.Subtle.Render("Not shown yet.")}
}

// placeholderPreview is what a preview shows until its pane has one.
func (m *panesModal) placeholderPreview(panes.Box) string {
	return m.theme.Subtle.Render("Not shown yet.")
}

// hovered is the row of a list that a preview shows.
type hovered struct{ id, title string }

// previewKey names a preview: the row of a pane's list it shows.
type previewKey struct {
	pane panes.Pane
	id   string
}

// follow moves the previews to the rows that the lists have under their
// cursors. A list with no row to preview leaves its preview where it was.
func (m *panesModal) follow() {
	for i, s := range m.slots {
		if s.hovered == nil {
			continue
		}
		if id, title, ok := s.hovered(m); ok {
			m.hover[i] = hovered{id, title}
		}
	}
}

// offset is how far the preview of the focused pane is scrolled.
func (m *panesModal) offset() int {
	return m.previews[previewKey{m.focus, m.hover[m.focus-1].id}]
}

// previewContent frames the preview in box b: its pane's body, scrolled to
// where this row was left, under the row's text.
func (m *panesModal) previewContent(b panes.Box) panes.Content {
	h := m.hover[b.Pane-1]
	s := m.slots[b.Pane-1]
	if s.preview == nil {
		return panes.Content{TopLeft: m.titleForms(h.title, b)}
	}
	lines := strings.Split(s.preview(m, b), "\n")
	off := min(m.previews[previewKey{b.Pane, h.id}], max(0, len(lines)-b.Rect.Inner().H))
	title := h.title
	if s.title != nil {
		title = s.title(m, b)
	}
	c := panes.Content{TopLeft: m.titleForms(title, b), Body: strings.Join(lines[off:], "\n")}
	if s.info != nil {
		c.TopRight = s.info(m, b)
	}
	return c
}
