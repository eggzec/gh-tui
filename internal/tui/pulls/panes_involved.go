package pulls

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/involved"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// ctxInvolved is the context of the keys of the people on the pull request.
const ctxInvolved = "pull_involved"

// involvedPane is what the involved pane keeps: the tree of the people, what it
// was built from, and a count of the changes to what it draws, which the
// drawn panes are keyed by.
type involvedPane struct {
	tree involved.Model
	st   involved.Styles
	data involved.Data
	ver  int
}

// newInvolved returns the pane, with the keys of the tree.
func newInvolved(k foldtree.KeyMap) *involvedPane {
	p := &involvedPane{tree: involved.New()}
	p.tree.SetKeyMap(k)
	p.tree.Focus()
	return p
}

// involvedData is what the pane shows of the pull request: its people, and the
// conversation as far as it is cached. It reads nothing.
func (m *panesModal) involvedData() involved.Data {
	d := involved.Data{Author: m.detail.Author, Assignees: m.detail.Assignees, Reviewers: m.detail.Reviewers}
	if m.svc == nil {
		return d
	}
	cursor := ""
	for {
		p, ok := m.svc.CachedTimeline(pulls.TimelineQuery{Repo: m.repo, Number: m.number, Cursor: cursor})
		if !ok {
			break
		}
		d.Items = append(d.Items, p.Items...)
		if p.Next == "" {
			break
		}
		cursor = p.Next
	}
	cursor = ""
	for {
		p, ok := m.svc.CachedThreads(pulls.ThreadsQuery{Repo: m.repo, Number: m.number, Head: m.detail.HeadSHA, Cursor: cursor})
		if !ok {
			break
		}
		d.Threads = append(d.Threads, p.Threads...)
		if p.Next == "" {
			break
		}
		cursor = p.Next
	}
	return d
}

// refreshInvolved shows the people of the detail as it is now.
func (m *panesModal) refreshInvolved() {
	p := m.inv
	p.data = m.involvedData()
	p.tree.SetData(p.data, involved.Options{Styles: p.st})
	p.ver++
}

// restyleInvolved draws the pane in the theme and icons of the modal.
func (m *panesModal) restyleInvolved() {
	p := m.inv
	p.st = involved.NewStyles(m.theme, m.icons)
	p.tree.SetStyles(foldtree.DefaultStyles(m.theme.Dark))
	m.refreshInvolved()
}

// involvedContent is the pane: the tree of the people, in the size of its box.
func (m *panesModal) involvedContent(b panes.Box) panes.Content {
	in := b.Rect.Inner()
	m.inv.tree.SetSize(in.W, in.H)
	return panes.Content{Body: m.inv.tree.View()}
}

// involvedPress hands a key to the tree.
func (m *panesModal) involvedPress(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	var cmd tea.Cmd
	m.inv.tree, cmd = m.inv.tree.Update(msg)
	m.inv.ver++
	return cmd, true
}

// involvedEnter takes the enter that the tree reports: a fold toggled, or the
// person to show the comments of.
func (m *panesModal) involvedEnter(msg foldtree.EnterMsg) tea.Cmd {
	var cmd tea.Cmd
	m.inv.tree, cmd = m.inv.tree.Update(msg)
	m.inv.ver++
	return cmd
}

// focusPerson shows the conversation, or says there is nothing to show of the
// person. A team never wrote a comment.
func (m *panesModal) focusPerson(msg involved.FocusPersonMsg) tea.Cmd {
	if msg.Team != "" || !involved.HasComments(m.inv.data, msg.Login) {
		return ui.Notify(toast.Info, "no comments from "+msg.Name)
	}
	m.focus = panes.Conversation
	m.resetSub()
	return nil
}

// involvedLayer is the keys of the pane, for the help.
func (m *panesModal) involvedLayer() keyhelp.Layer {
	k := m.inv.tree.KeyMap()
	// The tree's mark key is no key of this pane.
	all := []key.Binding{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown, k.Home, k.End, k.Expand, k.Collapse, k.ToggleAll, k.Enter}
	return ui.ContextLayer(ctxInvolved, all, k.ShortHelp())
}
