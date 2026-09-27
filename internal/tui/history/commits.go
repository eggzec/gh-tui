package history

import (
	"context"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	historysvc "github.com/eggzec/gh-tui/internal/service/history"
	"github.com/eggzec/gh-tui/pkg/bubbles/graph"
)

// commits is the graph pane: the history of one branch.
type commits struct {
	model graph.Model
	// branch is the branch shown, or empty for the default branch before
	// the app has read its name.
	branch string
	// gen counts the branches shown, and ctx bounds the reads for the one
	// shown, which cancel ends.
	gen    int
	ctx    context.Context
	cancel context.CancelFunc
	// stale is set by a fetch that served a first page an earlier session
	// kept, so that the modal reads it again.
	stale *atomic.Bool
	fetch graph.Fetch
}

// headMsg carries the first commit of branch gen, read again after a stale
// first page was shown, or after a sync event.
type headMsg struct {
	id    int64
	gen   int
	first string
	err   error
}

// shown returns the branch whose graph is shown.
func (g *commits) shown() string {
	return g.branch
}

// show replaces the graph with the history of branch, and returns what
// loads it. The first call makes the graph, which Init loads.
func (g *commits) show(m *Modal, branch string) tea.Cmd {
	if g.cancel != nil {
		g.cancel()
	}
	g.ctx, g.cancel = context.WithCancel(m.ctx)
	g.gen++
	g.branch = branch
	g.stale = new(atomic.Bool)
	g.fetch = m.fetchCommits(branch, g.stale)
	m.ahead.Reset(g.ctx)
	m.seq++
	m.commit.clear()
	if g.gen == 1 {
		g.model = graph.New(g.fetch,
			graph.WithContext(m.ctx),
			graph.WithKeyMap(m.keys.Graph),
			graph.WithFocused(m.focus == graphPane),
			graph.WithEmptyText("No commits on this branch."),
		)
		return nil
	}
	return g.model.Reset(g.fetch)
}

// fetchCommits reads the history of branch for the graph, a page at a time.
func (m *Modal) fetchCommits(branch string, stale *atomic.Bool) graph.Fetch {
	svc, repo, format, now, off := m.svc, m.repo, m.format, m.opts.now, m.opts.offline
	return func(ctx context.Context, cursor string) ([]graph.Commit, string, error) {
		ctx, end := obs.Begin(ctx, "history.commits")
		p, err := svc.Commits(ctx, historysvc.CommitsQuery{Repo: repo, Ref: branch, Cursor: cursor, PageSize: pageSize})
		end(err, "span", "tui", "repo", repo.String(), "ref", branch, "first", cursor == "", "stale", p.Stale)
		if err != nil {
			return nil, "", err
		}
		switch {
		case p.Offline:
			off.Mark()
		case p.Limited:
			off.MarkLimited()
		}
		if p.Stale && cursor == "" {
			stale.Store(true)
		}
		t := now()
		out := make([]graph.Commit, len(p.Items))
		for i := range p.Items {
			out[i] = format.graphCommit(p.Items[i], t)
		}
		return out, p.Next, nil
	}
}

// showBranch shows the graph of branch, with the focus on it.
func (m *Modal) showBranch(name string) tea.Cmd {
	m.setFocus(graphPane)
	if name == m.graph.branch && m.graph.gen > 0 {
		return nil
	}
	return m.graph.show(m, name)
}

// updateGraph passes msg to the graph, then reads the first page again if
// the graph showed one kept by an earlier session.
func (m *Modal) updateGraph(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.graph.model, cmd = m.graph.model.Update(msg)
	if m.graph.stale.CompareAndSwap(true, false) {
		cmd = tea.Batch(cmd, m.readHead())
	}
	return cmd
}

// readHead reads the first page of the branch shown again, which asks
// GitHub whether it moved, and reports its first commit.
func (m *Modal) readHead() tea.Cmd {
	g := &m.graph
	svc, repo, ctx, id, gen, branch := m.svc, m.repo, g.ctx, m.id, g.gen, g.branch
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "history.head")
		p, err := svc.Commits(ctx, historysvc.CommitsQuery{Repo: repo, Ref: branch, PageSize: pageSize, Again: true})
		end(err, "span", "tui", "repo", repo.String(), "ref", branch)
		msg := headMsg{id: id, gen: gen, err: err}
		if len(p.Items) > 0 {
			msg.first = p.Items[0].SHA
		}
		return msg
	}
}

// receiveHead shows the history again once the branch moved.
func (m *Modal) receiveHead(msg headMsg) tea.Cmd {
	g := &m.graph
	if msg.gen != g.gen || msg.err != nil {
		return nil
	}
	c, ok := g.model.At(0)
	if ok && c.ID == msg.first || !ok && msg.first == "" {
		return nil
	}
	m.commit.clear()
	return g.model.Reset(g.fetch)
}

// selectedCommit returns the commit under the graph's cursor.
func (m *Modal) selectedCommit() (core.Commit, bool) {
	gc, ok := m.graph.model.Selected()
	if !ok {
		return core.Commit{}, false
	}
	c, ok := gc.Value.(core.Commit)
	return c, ok
}
