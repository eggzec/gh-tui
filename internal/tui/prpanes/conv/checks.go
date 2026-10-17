package conv

import (
	"maps"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

// checkOptions says how the rows of the checks of a commit are drawn.
func (m Model) checkOptions(prefix string, depth int) checktree.Options {
	return checktree.Options{Depth: depth, Prefix: prefix, Now: m.opts.Now, Width: m.width}
}

// wantChecks asks for the checks of each commit row that is open and was not
// asked for yet, which is when they are read.
func (m Model) wantChecks() (Model, tea.Cmd) {
	if m.src.Checks == nil {
		return m, nil
	}
	v := m.views[m.cur]
	var cmds []tea.Cmd
	var added map[string]bool
	for _, r := range v.rows {
		if r.Kind != KindCommit || !r.Fold || v.tree.Folded(r.ID) {
			continue
		}
		n := v.nodes[r.ID]
		if n == nil || m.wanted[n.item.Commit.OID] || added[n.item.Commit.OID] {
			continue
		}
		if added == nil {
			added = maps.Clone(m.wanted)
			if added == nil {
				added = map[string]bool{}
			}
		}
		added[n.item.Commit.OID] = true
		var cmd tea.Cmd
		m.loader, cmd = m.loader.Want(n.item.Commit.OID, false)
		cmds = append(cmds, cmd)
	}
	if added == nil {
		return m, nil
	}
	m.wanted = added
	m.checksVer++
	m.sync()
	return m, tea.Batch(cmds...)
}

// updateChecks gives a message to the loader of the checks first. A change of
// what it holds builds the rows again. Keys and the model's own messages are
// not the loader's, so they cost nothing.
func (m Model) updateChecks(msg tea.Msg) (Model, tea.Cmd) {
	if len(m.wanted) == 0 {
		return m, nil
	}
	switch msg.(type) {
	case tea.KeyPressMsg, tea.MouseMsg, foldtree.EnterMsg, loadMsg, commitsMsg:
		return m, nil
	}
	before := m.checksSig()
	var cmd tea.Cmd
	m.loader, cmd = m.loader.Update(msg)
	// A modal that changed the checks may leave rows of the same shape, so the
	// messages that say so build them again.
	switch msg.(type) {
	case ui.OnlineMsg, ui.ReopenedMsg:
		m.checksVer++
	default:
		if m.checksSig() != before {
			m.checksVer++
		}
	}
	m.sync()
	return m, cmd
}

// checksSig says what the rows of the opened commits show in short: how many
// rows each has and the first of them, which tells a note from a tree.
func (m Model) checksSig() string {
	var b strings.Builder
	st := m.opts.Styles.checks
	for sha := range m.wanted {
		rows, _ := m.loader.Rows(sha, m.checkOptions("", 0), st)
		b.WriteString(sha)
		b.WriteString(":")
		b.WriteString(strconv.Itoa(len(rows)))
		if len(rows) > 0 {
			b.WriteString(":")
			b.WriteString(rows[0].ID)
			b.WriteString(rows[0].Lines[0])
		}
		b.WriteString(";")
	}
	return b.String()
}
