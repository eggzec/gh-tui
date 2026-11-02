package pulls

import (
	"log/slog"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// setRows gives the tree its rows, and keeps which of them are files.
func (f *filesPane) setRows(rows []treeview.Node) {
	f.tree.SetRows(rows)
	f.fileIDs = f.fileIDs[:0]
	for i := range rows {
		r := &rows[i]
		if r.Kind == files.KindFile && !r.Skip {
			f.fileIDs = append(f.fileIDs, r.ID)
		}
	}
}

// stepFile moves the cursor to the next file, or with dir -1 the previous one,
// opening the folders above it. From a row that isn't a file it goes to the
// first file, or the last for dir -1, and from the last file or the first it
// stays.
func (m *panesModal) stepFile(dir int) {
	f := &m.files
	if len(f.fileIDs) == 0 {
		return
	}
	cur, _ := f.tree.Selected()
	at := slices.Index(f.fileIDs, cur.ID)
	to := at + dir
	switch {
	case at < 0 && dir > 0:
		to = 0
	case at < 0:
		to = len(f.fileIDs) - 1
	case to < 0 || to >= len(f.fileIDs):
		return
	}
	f.tree.Reveal(f.fileIDs[to])
}

// markFile marks the file at path viewed on GitHub, or takes the mark off, and
// moves on to the next file. Where the marks can't be changed it only says so.
func (m *panesModal) markFile(path string) tea.Cmd {
	f := &m.files
	cmd, _ := m.toggleViewed(&f.vw, path, func(bool) {},
		func(marked bool, err error) tea.Msg {
			return filesViewedSetMsg{m: m, path: path, marked: marked, err: err}
		})
	if f.vw.in && f.vw.pv.Supported {
		m.stepFile(1)
	}
	f.viewedVer++
	f.seq++
	return cmd
}

// syncViewed takes the marks of the cache into the Files pane, which the diff
// changes as well.
func (m *panesModal) syncViewed() {
	f := &m.files
	if m.cachedViewed(&f.vw) {
		f.viewedVer++
		f.seq++
	}
}

// readFilesViewed reads the marks of the files at the head of the detail, which
// the cache may have already. It reads once for each head.
func (m *panesModal) readFilesViewed() tea.Cmd {
	f := &m.files
	q := m.viewedQuery()
	if q.Head == "" || f.viewedAsked == q.Head {
		return nil
	}
	f.viewedAsked = q.Head
	f.vw.failed = false
	m.syncViewed()
	svc, ctx := m.svc, m.ctx
	return func() tea.Msg {
		pv, err := svc.Viewed(ctx, q)
		return filesViewedMsg{m: m, head: q.Head, pv: pv, err: err}
	}
}

// filesViewedRead takes the marks that a read brought. A read that failed
// leaves the files with the marks the cache had, and says so once.
func (m *panesModal) filesViewedRead(msg filesViewedMsg) tea.Cmd {
	f := &m.files
	if msg.m != m || msg.head != f.viewedAsked {
		return nil
	}
	if msg.err != nil {
		if m.ctx.Err() != nil {
			return nil
		}
		f.vw.failed = true
		slog.WarnContext(m.ctx, "viewed files not read", "span", "tui", "err", msg.err)
		return ui.Fail("read the files you viewed of #"+strconv.Itoa(m.number), msg.err)
	}
	f.vw.receive(msg.pv)
	f.viewedVer++
	f.seq++
	return nil
}

// filesViewedSet takes GitHub's answer to a mark or unmark. A refusal has been
// rolled back in the cache, so the files go back to what it says.
func (m *panesModal) filesViewedSet(msg filesViewedSetMsg) {
	f := &m.files
	if msg.m != m {
		return
	}
	if m.settleViewed(&f.vw, msg.path, msg.marked, msg.err) {
		f.viewedVer++
		f.seq++
	}
}
