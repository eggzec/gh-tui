package issues

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	refssvc "github.com/eggzec/gh-tui/internal/service/refs"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/dev"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/involved"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// sideKeyContexts are the contexts whose tree keys the side panes use: the
// trees are those of the pull request modal, so the keys are the same.
const ctxSideInvolved = "pull_involved"

// sideKeys returns the tree keys of the side panes: the context has no mark
// action, which a tree of people or issues has nothing to mark for.
func sideKeys(keys config.Keymap) treeview.KeyMap {
	look := ui.Lookup(keys, ctxSideInvolved)
	if s, ok := look.(keymap.Scoped); ok {
		look = noMark{s}
	}
	return treeview.NewKeyMap(look)
}

// noMark is a lookup that gives no keys to the mark action.
type noMark struct{ keymap.Scoped }

func (n noMark) Of(action string) []string {
	if action == "mark" {
		return nil
	}
	return n.Scoped.Of(action)
}

// sidePanes are the Involved and Development panes: the people of the issue,
// and the pull requests that close it.
type sidePanes struct {
	inv    involved.Model
	invSt  involved.Styles
	dev    dev.Model
	devSt  dev.Styles
	linked bool
	deps   treeview.Model
}

// linksMsg is what a read of the references of the issue brought.
type linksMsg struct {
	id   int64
	refs core.References
	err  error
}

// newSide makes the panes with the keys of the trees.
func newSide(keys config.Keymap) *sidePanes {
	p := &sidePanes{inv: involved.New(), dev: dev.New(dev.Options{HideCloses: true})}
	p.inv.SetKeyMap(sideKeys(keys))
	p.dev.SetKeyMap(dev.NewKeyMap(keys))
	return p
}

// timelineItems returns the cached timeline of the issue, page by page.
func (m *detailModal) timelineItems() []core.TimelineItem {
	var out []core.TimelineItem
	cursor := ""
	for {
		p, ok := m.svc.CachedTimeline(issuesvc.TimelineQuery{Repo: m.repo, Number: m.number, Cursor: cursor})
		if !ok {
			return out
		}
		out = append(out, p.Items...)
		if p.Next == "" {
			return out
		}
		cursor = p.Next
	}
}

// refreshSide shows the people and the links the modal has now.
func (m *detailModal) refreshSide() {
	p := m.side
	if p == nil {
		return
	}
	p.invSt = involved.NewStyles(m.theme, m.icons)
	p.inv.SetStyles(m.theme.Tree(m.icons))
	p.deps.SetStyles(m.theme.Tree(m.icons))
	d := involved.Data{Author: m.issue.Author, Assignees: m.issue.Assignees, Items: m.timelineItems()}
	p.inv.SetData(d, involved.Options{Styles: p.invSt, HideReviewers: true})
	p.devSt = dev.NewStyles(m.theme, m.icons)
	p.dev.SetStyles(p.devSt)
	if m.refSvc != nil {
		if r, ok := m.refSvc.CachedReferences(m.refQuery(false)); ok {
			p.setLinks(r)
		}
	}
}

func (p *sidePanes) setLinks(r core.References) {
	p.dev.SetLinks(core.PullLinks{Linked: r.Closing})
	p.linked = true
}

func (m *detailModal) refQuery(again bool) refssvc.Query {
	return refssvc.Query{Repo: m.repo, Number: m.number, Again: again}
}

// readLinks reads the pull requests that close the issue.
func (m *detailModal) readLinks() tea.Cmd {
	if m.refSvc == nil {
		return nil
	}
	svc, ctx, id, q := m.refSvc, m.ctx, m.id, m.refQuery(false)
	return func() tea.Msg {
		r, err := svc.References(ctx, q)
		return linksMsg{id: id, refs: r, err: err}
	}
}

// receiveLinks takes the references that a read brought.
func (m *detailModal) receiveLinks(msg linksMsg) {
	if m.side == nil || msg.id != m.id {
		return
	}
	if msg.err != nil {
		if m.ctx.Err() == nil {
			m.side.dev.SetError(msg.err)
		}
		return
	}
	m.side.setLinks(msg.refs)
}

// sideBody is the body of the Involved or Development pane in box b.
func (m *detailModal) sideBody(b panes.Box) (string, bool) {
	p := m.side
	if p == nil {
		return "", false
	}
	if b.Pane == paneBlockers {
		return m.depsBody(b), true
	}
	if b.Pane != paneInvolved && b.Pane != paneDevelopment {
		return "", false
	}
	in := b.Rect.Inner()
	focus := m.nav.Focus() == b.Pane
	if b.Pane == paneInvolved {
		p.inv.SetSize(in.W, in.H)
		if focus {
			p.inv.Focus()
		} else {
			p.inv.Blur()
		}
		return p.inv.View(), true
	}
	p.dev.SetSize(in.W, in.H)
	if focus {
		p.dev.Focus()
	} else {
		p.dev.Blur()
	}
	return p.dev.View(), true
}

// pressSide gives a key to the tree of the focused side pane, and reports
// whether there was one.
func (m *detailModal) pressSide(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	p := m.side
	if p == nil {
		return nil, false
	}
	var cmd tea.Cmd
	switch m.nav.Focus() {
	case paneInvolved:
		p.inv, cmd = p.inv.Update(msg)
	case paneDevelopment:
		if !p.dev.Loaded() {
			return nil, false
		}
		p.dev, cmd = p.dev.Update(msg)
	case paneBlockers:
		p.deps, cmd = p.deps.Update(msg)
	default:
		return nil, false
	}
	return cmd, true
}

// enterSide opens the issue or pull request of the row that enter reached in
// the Development tree, over the modal.
func (m *detailModal) enterSide(msg treeview.OpenMsg) tea.Cmd {
	p := m.side
	if p == nil {
		return nil
	}
	if msg.Tree == p.inv.ID() {
		var cmd tea.Cmd
		p.inv, cmd = p.inv.Update(msg)
		return cmd
	}
	if msg.Tree == p.deps.ID() {
		return m.enterDeps(msg)
	}
	if msg.Tree != p.dev.ID() {
		return nil
	}
	it, ok := p.dev.Item(msg.ID)
	if !ok {
		return nil
	}
	switch it.Kind {
	case dev.Issue, dev.Blocker, dev.Pull:
	default:
		return nil
	}
	r := it.Ref
	if r.Problem != "" {
		return ui.Notify(0, fmt.Sprintf("Can't read %s: %s.", r.Target, r.Problem))
	}
	if r.Target.Kind == core.KindPull {
		return func() tea.Msg { return ui.OpenPullMsg{Repo: r.Target.Repo, Number: r.Target.Number, Over: m} }
	}
	return func() tea.Msg { return ui.OpenIssueMsg{Repo: r.Target.Repo, Number: r.Target.Number, Over: m} }
}
