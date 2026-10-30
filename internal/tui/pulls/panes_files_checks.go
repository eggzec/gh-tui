package pulls

import (
	"context"
	"maps"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

// checkReader reads the checks of a commit for a checktree.Loader from the
// checks service.
type checkReader struct{ svc ChecksService }

func (r checkReader) CachedCommitChecks(q checktree.Query) (core.CommitChecks, bool) {
	return r.svc.CachedCommitChecks(actionssvc.CommitChecksQuery{Repo: q.Repo, SHA: q.SHA, Notes: q.Notes})
}

func (r checkReader) CommitChecks(ctx context.Context, q checktree.Query) (core.CommitChecks, error) {
	return r.svc.CommitChecks(ctx, actionssvc.CommitChecksQuery{Repo: q.Repo, SHA: q.SHA, Notes: q.Notes})
}

// filesRows builds the rows of the Files pane for a tree width wide. A commit
// that has checks opens them under it when a checks service reads them.
func (m *panesModal) filesRows(width int, folded bool) []foldtree.Row {
	f := &m.files
	now := time.Now()
	d := m.filesData()
	targets := checktree.Targets{}
	if m.checks != nil {
		d.Children = func(oid string, depth int) []foldtree.Row {
			rows, t := f.loader.Rows(oid, checktree.Options{
				Mode:   checktree.All,
				Depth:  depth,
				Prefix: "checks/" + oid + "/",
				Now:    now,
				Width:  width,
			}, f.checkStyles)
			maps.Copy(targets, t)
			return rows
		}
	}
	f.targets = targets
	o := files.Options{Width: width, Styles: f.styles, Dates: m.dates, Now: now, Folded: folded}
	if page := m.detail.URL; page != "" {
		o.FileURL = func(path string) string { return page + "/files#" + ui.DiffAnchor(path) }
	}
	return files.Rows(d, o)
}

// wantChecks asks for the checks of each commit whose fold is open, once. It
// waits for the rows of the current commits, since the tree knows a fold only
// from them.
func (m *panesModal) wantChecks() tea.Cmd {
	f := &m.files
	if m.checks == nil || !f.built.ok || f.built.version != f.version {
		return nil
	}
	var cmds []tea.Cmd
	for _, c := range f.commits.Commits {
		if c.Checks == core.CheckNone || f.wanted[c.OID] || f.tree.Folded(files.CommitID(c.OID)) {
			continue
		}
		if f.wanted == nil {
			f.wanted = map[string]bool{}
		}
		f.wanted[c.OID] = true
		var cmd tea.Cmd
		f.loader, cmd = f.loader.Want(c.OID, false)
		cmds = append(cmds, cmd)
		f.checksVer++
	}
	return tea.Batch(cmds...)
}

// checksMsg gives the loader a message that is its own, or that it takes up
// what is kept for, and draws the rows again.
func (m *panesModal) checksMsg(msg tea.Msg) tea.Cmd {
	if m.checks == nil {
		return nil
	}
	f := &m.files
	var cmd tea.Cmd
	f.loader, cmd = f.loader.Update(msg)
	f.checksVer++
	f.seq++
	return cmd
}
