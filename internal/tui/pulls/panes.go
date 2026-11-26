package pulls

import (
	"github.com/eggzec/gh-tui/internal/tui/prpanes/slots"
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

// slot is what one pane of the modal is made of.
type slot = slots.Slot[*panesModal]

// paneSlots has one entry per pane, in the order of the panes.
var paneSlots = [paneCount]slot{
	paneDescription - 1:  {Context: ctxDescription, Content: (*panesModal).description, Press: (*panesModal).pressDescription},
	paneConversation - 1: {Context: ctxConversation, Content: (*panesModal).convContent, Press: (*panesModal).convPress},
	paneMerge - 1: {
		Context: ctxMerge, Content: (*panesModal).mergeContent, Press: (*panesModal).pressMerge,
		Hovered: (*panesModal).hoveredMerge, Preview: (*panesModal).mergePreview, Info: (*panesModal).mergeInfo,
	},
	paneInvolved - 1: {
		Context: ctxInvolved, Content: (*panesModal).involvedContent, Press: (*panesModal).involvedPress,
		Hovered: (*panesModal).hoveredInvolved, Preview: (*panesModal).involvedPreview, Info: (*panesModal).involvedInfo,
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

// follow moves the previews to the rows that the lists have under their
// cursors.
func (m *panesModal) follow() { m.slots.Follow(m) }

// offset is how far the preview of the focused pane is scrolled.
func (m *panesModal) offset() int { return m.slots.Offset(m.ctl.Nav.Focus()) }
