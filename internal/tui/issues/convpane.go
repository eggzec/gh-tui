package issues

// Prototype: the issue modal draws its body, comments and timeline through
// prpanes/conv instead of pkg/bubbles/thread. Nothing here is cached and an
// edit is not optimistic.

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// timeliner is what the service offers beyond Service for the conversation.
type timeliner interface {
	Timeline(ctx context.Context, q issuesvc.TimelineQuery) (core.Page[core.TimelineItem], error)
	EditComment(ctx context.Context, id, body string) error
}

// convHost is the conversation pane of the modal.
type convHost struct {
	model conv.Model
	tl    timeliner
	md    *markdown.Renderer
	head  string
	// editing is the node id of the comment the prompt edits.
	editing string
}

// editedMsg reports a comment edit.
type editedMsg struct{ err error }

// convLookup gives the actions the conversation context has.
type convLookup struct {
	ui.Context
	keys config.Keymap
}

// Of implements keymap.Lookup.
func (l convLookup) Of(action string) []string {
	if !strings.Contains(action, ".") && !l.keys.Has(l.Scope()+"."+action) {
		return nil
	}
	return l.Context.Of(action)
}

// treeKeys returns the keys of the conversation's tree.
func treeKeys(keys config.Keymap) foldtree.KeyMap {
	return foldtree.NewKeyMap(convLookup{Context: ui.In(keys, "conversation"), keys: keys})
}

// buildConv makes the conversation and returns its first reads.
func (m *detailModal) buildConv() tea.Cmd {
	h := m.cv
	if h == nil {
		return nil
	}
	h.model.Close()
	repo, number := m.repo, m.number
	src := conv.Source{
		Timeline: func(ctx context.Context, cursor string) (core.Page[core.TimelineItem], error) {
			p, err := h.tl.Timeline(ctx, issuesvc.TimelineQuery{Repo: repo, Number: number, Cursor: cursor})
			if err != nil || cursor != "" || !m.loaded {
				return p, err
			}
			// The body is the first comment of the conversation.
			body := core.TimelineItem{
				Kind: core.TimelineComment, ID: "body", Actor: m.issue.Author,
				At: m.issue.CreatedAt, UpdatedAt: m.issue.UpdatedAt, Body: m.issue.Body, URL: m.issue.URL,
			}
			p.Items = append([]core.TimelineItem{body}, p.Items...)
			return p, nil
		},
		Repo: repo,
	}
	h.md = markdown.New(m.theme.Thread(m.icons).Markdown)
	h.md.SetGlyphs(m.icons.Markdown)
	h.md.SetLinkColor(m.theme.LinkColor())
	voice := ui.Voice{Icons: &m.icons}
	h.model = conv.New(src,
		conv.WithOptions(conv.Options{
			Styles: conv.NewStyles(m.theme, m.icons, m.icons.Set()),
			MD:     h.md,
			Dates:  m.dates,
			Now:    m.now(),
		}),
		conv.WithTreeKeys(m.keys.tree),
		conv.WithTreeStyles(foldtree.DefaultStyles(m.theme.Dark)),
		conv.WithViewer(m.viewer),
		conv.WithVoice(voice),
		conv.WithSize(m.width, max(m.height-m.cvHeaderLines(), 0)),
	)
	if m.composing == composeNone {
		h.model.Focus()
	}
	return h.model.Init()
}

// cvHeaderLines is the height of the header drawn over the conversation.
func (m *detailModal) cvHeaderLines() int {
	if m.cv == nil || m.cv.head == "" {
		return 0
	}
	return strings.Count(m.cv.head, "\n") + 1
}

func (m *detailModal) paneInit() tea.Cmd {
	if m.cv != nil {
		return m.buildConv()
	}
	return m.thread.Init()
}

func (m *detailModal) paneReload() tea.Cmd {
	if m.cv != nil {
		return m.buildConv()
	}
	return m.thread.Reload()
}

func (m *detailModal) paneUpdate(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if m.cv != nil {
		m.cv.model, cmd = m.cv.model.Update(msg)
		return cmd
	}
	m.thread, cmd = m.thread.Update(msg)
	return cmd
}

func (m *detailModal) paneSize(w, h int) {
	if m.cv != nil {
		m.cv.model.SetSize(w, max(h-m.cvHeaderLines(), 0))
		return
	}
	m.thread.SetSize(w, h)
}

func (m *detailModal) paneFocus(on bool) {
	switch {
	case m.cv != nil && on:
		m.cv.model.Focus()
	case m.cv != nil:
		m.cv.model.Blur()
	case on:
		m.thread.Focus()
	default:
		m.thread.Blur()
	}
}

func (m *detailModal) paneView() string {
	if m.cv == nil {
		return m.thread.View()
	}
	v := m.cv.model.View()
	if m.cv.head == "" {
		return v
	}
	return m.cv.head + "\n" + v
}

func (m *detailModal) paneHeight() int {
	if m.cv == nil {
		return m.thread.Height()
	}
	return m.height - m.cvHeaderLines()
}

// editComment opens the prompt on the viewer's own comment under the cursor.
func (m *detailModal) editComment() tea.Cmd {
	if m.cv == nil {
		return nil
	}
	s, ok := m.cv.model.Current()
	if !ok || s.Item == nil || s.Item.Kind != core.TimelineComment || s.Item.ID == "" || s.Item.ID == "body" {
		return ui.Notify(toast.Info, "put the cursor on one of your comments")
	}
	if !s.Item.CanUpdate && s.Item.Actor.Login != m.viewer {
		return ui.Notify(toast.Info, "you can edit only your own comments")
	}
	m.cv.editing = s.Item.ID
	return m.composeWith(composeEdit, ui.ActComment, s.Item.Body)
}

// submitEdit asks to send body as the new text of the comment being edited.
func (m *detailModal) submitEdit(body string) tea.Cmd {
	body = strings.TrimSpace(body)
	if body == "" || m.cv == nil {
		return nil
	}
	id, h := m.cv.editing, m.cv
	return m.asks(confirmEdit(func() tea.Cmd {
		m.closePrompt()
		return func() tea.Msg {
			if h.tl == nil {
				return editedMsg{errors.New("no service")}
			}
			return editedMsg{h.tl.EditComment(m.sendCtx, id, body)}
		}
	}))
}

func confirmEdit(run func() tea.Cmd) confirm.Ask {
	return confirm.Ask{Question: "Save the edited comment?", Run: run}
}

// gotEdited reloads the conversation after an edit.
func (m *detailModal) gotEdited(msg editedMsg) tea.Cmd {
	if msg.err != nil {
		return ui.Notify(toast.Error, "edit comment: "+ansi.Strip(msg.err.Error()))
	}
	return tea.Batch(m.buildConv(), ui.Notify(toast.Info, "comment edited"))
}
