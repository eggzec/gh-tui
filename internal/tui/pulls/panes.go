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
	// press takes a key that the modal doesn't, while the pane's list has the
	// focus. ok is false for a key it has no use for.
	press func(m *panesModal, k tea.KeyPressMsg) (cmd tea.Cmd, ok bool)
}

// slots has one entry per pane, in the order of the panes.
var slots = [panes.Count]slot{
	panes.Description - 1:  {context: ctxDescription, content: (*panesModal).description, press: (*panesModal).pressDescription},
	panes.Conversation - 1: {context: ctxConversation, content: (*panesModal).placeholder},
	panes.Merge - 1:        {context: "pull_merge", content: (*panesModal).placeholder, preview: (*panesModal).placeholderPreview},
	panes.Involved - 1:     {context: "pull_involved", content: (*panesModal).placeholder, preview: (*panesModal).placeholderPreview},
	panes.Development - 1:  {context: "pull_development", content: (*panesModal).placeholder, preview: (*panesModal).placeholderPreview},
	panes.Files - 1:        filesSlot(),
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
		return panes.Content{Title: h.title}
	}
	lines := strings.Split(s.preview(m, b), "\n")
	off := min(m.previews[previewKey{b.Pane, h.id}], max(0, len(lines)-b.Rect.Inner().H))
	return panes.Content{Title: h.title, Body: strings.Join(lines[off:], "\n")}
}
