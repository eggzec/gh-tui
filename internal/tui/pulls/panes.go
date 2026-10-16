package pulls

import (
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
}

// slots has one entry per pane, in the order of the panes.
var slots = [panes.Count]slot{
	panes.Description - 1:  {"pull_description", (*panesModal).placeholder},
	panes.Conversation - 1: {ctxConversation, (*panesModal).placeholder},
	panes.Merge - 1:        {"pull_merge", (*panesModal).placeholder},
	panes.Involved - 1:     {"pull_involved", (*panesModal).placeholder},
	panes.Development - 1:  {"pull_development", (*panesModal).placeholder},
	panes.Files - 1:        {ctxFiles, (*panesModal).placeholder},
}

// placeholder is what a pane shows until it has content of its own. Its
// title is the pane's.
func (m *panesModal) placeholder(panes.Box) panes.Content {
	return panes.Content{Body: m.theme.Subtle.Render("Not shown yet.")}
}
