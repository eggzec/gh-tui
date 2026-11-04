package issues

import (
	"context"
	"errors"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// depser reads the issues that block an issue, or that it blocks.
type depser interface {
	Dependencies(ctx context.Context, repo core.RepoRef, n int, kind string) ([]core.Dependency, error)
}

// maxDepDepth is how many levels the Blockers tree opens below its roots.
const maxDepDepth = 5

// The two roots of the Blockers tree, named as the service names the kinds.
const (
	depBlockedBy = "blocked_by"
	depBlocking  = "blocking"
)

// depNode is the Value of a node of the Blockers tree. trail holds the numbers
// from the issue down to the node, to spot a cycle.
type depNode struct {
	kind  string
	dep   core.Dependency
	trail []int
}

// newDeps makes the Blockers tree, which reads the issues that wait for this
// one, or that this one waits for, level by level as its rows open.
func (m *detailModal) newDeps(keys config.Keymap) treeview.Model {
	t := treeview.New(treeview.Lazy(m.depChildren),
		treeview.WithContext(m.ctx),
		treeview.WithKeyMap(sideKeys(keys)),
		treeview.WithStyles(m.theme.Tree(m.icons)),
		treeview.WithIcons(m.depIcon),
		treeview.WithEmpty("none", ""),
	)
	return t
}

func (m *detailModal) depIcon(n treeview.Node) string {
	v, ok := n.Value.(depNode)
	if !ok || v.dep.Number == 0 {
		return ""
	}
	return m.icons.State(depState(v.dep))
}

func depState(d core.Dependency) ui.State {
	if d.IsPullRequest {
		return ui.PullState(d.State, false)
	}
	return ui.IssueState(core.Issue{State: d.State, Reason: d.Reason})
}

// depChildren loads the rows below parent: the two roots at the top, and
// below a root or an issue the issues it waits for or blocks.
func (m *detailModal) depChildren(ctx context.Context, parent treeview.Node) ([]treeview.Node, error) {
	if parent.ID == "" {
		return depRoots(m.number), nil
	}
	pn, ok := parent.Value.(depNode)
	if !ok || len(pn.trail) == 0 {
		return nil, nil
	}
	repo := pn.dep.Repo
	if repo == (core.RepoRef{}) {
		repo = m.repo
	}
	ds, ok := m.svc.(depser)
	if !ok {
		return nil, errors.New("dependencies are not available")
	}
	deps, err := ds.Dependencies(ctx, repo, pn.trail[len(pn.trail)-1], pn.kind)
	if errors.Is(err, core.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return depRows(parent.ID, pn, deps), nil
}

// depRoots are the two top rows of the tree.
func depRoots(self int) []treeview.Node {
	return []treeview.Node{
		{ID: depBlockedBy, Name: "Blocked by", Branch: true, Value: depNode{kind: depBlockedBy, trail: []int{self}}},
		{ID: depBlocking, Name: "Blocking", Branch: true, Value: depNode{kind: depBlocking, trail: []int{self}}},
	}
}

// depRows are the rows for the dependencies deps of the row pn with the ID
// id. An issue already on the trail is a cycle, and one past maxDepDepth
// levels is cut: neither opens further.
func depRows(id string, pn depNode, deps []core.Dependency) []treeview.Node {
	out := make([]treeview.Node, 0, len(deps))
	for _, d := range deps {
		trail := append(slices.Clone(pn.trail), d.Number)
		right := ""
		switch {
		case slices.Contains(pn.trail, d.Number):
			right = "cycle"
		case len(trail) > maxDepDepth:
			right = "deeper"
		}
		out = append(out, treeview.Node{
			ID:     id + ">#" + strconv.Itoa(d.Number),
			Name:   "#" + strconv.Itoa(d.Number) + " " + ui.OneLine(d.Title),
			Right:  right,
			Branch: right == "",
			Action: true,
			Link:   d.URL,
			Value:  depNode{kind: pn.kind, dep: d, trail: trail},
		})
	}
	return out
}

// depsBody is the body of the Blockers pane in box b.
func (m *detailModal) depsBody(b panes.Box) string {
	t := &m.side.deps
	in := b.Rect.Inner()
	t.SetSize(in.W, in.H)
	if m.nav.Focus() == b.Pane {
		t.Focus()
	} else {
		t.Blur()
	}
	return t.View()
}

// enterDeps handles enter on a row of the Blockers tree: an issue or pull
// request opens over the modal, and a root folds or unfolds.
func (m *detailModal) enterDeps(msg treeview.OpenMsg) tea.Cmd {
	p := m.side
	if v, ok := msg.Node.Value.(depNode); ok && v.dep.Number != 0 {
		d := v.dep
		repo := d.Repo
		if repo == (core.RepoRef{}) {
			repo = m.repo
		}
		if d.IsPullRequest {
			return func() tea.Msg { return ui.OpenPullMsg{Repo: repo, Number: d.Number, Over: m} }
		}
		return func() tea.Msg { return ui.OpenIssueMsg{Repo: repo, Number: d.Number, Over: m} }
	}
	var cmd tea.Cmd
	p.deps, cmd = p.deps.Update(msg)
	return cmd
}
