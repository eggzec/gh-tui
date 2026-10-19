package pulls

import (
	"context"
	"slices"
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/diffview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// filesDiff is the diff beside the tree of the zoomed Files pane: the file
// under the tree's cursor, or the last file it was on while the cursor is on
// a row that is no file.
type filesDiff struct {
	// key names the read of the files that src and order are of.
	key   string
	src   *diffSource
	order []string
	// path is the file shown, and on says that d is built for it.
	path string
	on   bool
	d    diff.Model
	// w and h are the size d was given.
	w, h   int
	cancel context.CancelFunc
	// styles are the diff's, for the m.gen they were made at.
	styles    diff.Styles
	stylesGen int
	styled    bool
}

// drop ends the read of the diff and forgets it. The source stays, so that
// the files are not read again when a diff is built over it.
func (pv *filesDiff) drop() {
	if pv.cancel != nil {
		pv.cancel()
		pv.cancel = nil
	}
	pv.on, pv.d = false, diff.Model{}
}

// previewKeys are the keys of the diff beside the tree: those that move in it.
func (k diffKeys) previewKeys() diff.KeyMap {
	d := k.diff
	for _, b := range []*key.Binding{&d.Left, &d.Right, &d.NextFile, &d.PrevFile, &d.Fold, &d.Search, &d.Confirm, &d.Cancel, &d.CancelEmpty, &d.Next, &d.Prev, &d.Retry} {
		b.SetEnabled(false)
	}
	return d
}

// syncFilesDiff makes the diff beside the tree the diff of the file the tree
// follows, once the files of the range are read. It builds a diff when the
// file, the range or the box changes, and drops it when the pane has no box
// for it. It returns the read of the first page of a diff it built.
func (m *panesModal) syncFilesDiff() tea.Cmd {
	f := &m.files
	pv := &f.pv
	b, ok := m.layout().Box(panes.Files, panes.Preview)
	if !ok || !f.loaded || !f.read.done || f.read.err != "" {
		if pv.on {
			pv.drop()
			f.seq++
		}
		return nil
	}
	if read := f.read.key + "#" + strconv.Itoa(f.read.version); read != pv.key || pv.src == nil {
		pv.drop()
		pv.key, pv.src, pv.order = read, m.diffSource(), diffview.Order(f.read.files)
		f.seq++
	}
	want := pv.path
	if p, ok := m.hoveredFile(); ok && slices.Contains(pv.order, p) {
		want = p
	}
	if !slices.Contains(pv.order, want) {
		want = ""
		if len(pv.order) > 0 {
			want = pv.order[0]
		}
	}
	in := b.Rect.Inner()
	var cmd tea.Cmd
	switch {
	case want == "":
		pv.drop()
		pv.path = ""
	case want != pv.path || !pv.on:
		cmd = m.buildFilesDiff(want, in.W, in.H)
	case pv.w != in.W || pv.h != in.H:
		pv.d.SetSize(in.W, in.H)
		pv.w, pv.h = in.W, in.H
		f.seq++
	}
	if focused := pv.on && m.focus == panes.Files && m.sub[panes.Files-1] == panes.Preview && m.top() == nil; focused != pv.d.Focused() {
		if focused {
			pv.d.Focus()
		} else {
			pv.d.Blur()
		}
		f.seq++
	}
	return cmd
}

// buildFilesDiff shows the file at path in a new diff of w by h cells, and
// returns the read of its first page.
func (m *panesModal) buildFilesDiff(path string, w, h int) tea.Cmd {
	f := &m.files
	pv := &f.pv
	pv.drop()
	if !pv.styled || pv.stylesGen != m.gen {
		pv.styles, pv.stylesGen, pv.styled = m.theme.Diff(m.icons), m.gen, true
	}
	ctx, cancel := context.WithCancel(m.ctx)
	pv.cancel = cancel
	pv.path, pv.on = path, true
	pv.d = diff.New(pv.src.fetch(path),
		diff.WithContext(ctx),
		diff.WithKeyMap(m.keys.diff.previewKeys()),
		diff.WithStyles(pv.styles),
		diff.WithFocused(false),
	)
	pv.d.SetSize(w, h)
	pv.w, pv.h = w, h
	f.seq++
	return pv.d.Init()
}

// restyleFilesDiff gives the diff the colours of a new theme at once, not
// when the next file is built.
func (m *panesModal) restyleFilesDiff() {
	pv := &m.files.pv
	if !pv.on {
		return
	}
	pv.styles, pv.stylesGen = m.theme.Diff(m.icons), m.gen
	pv.d.SetStyles(pv.styles)
	m.files.seq++
}

// resizeFilesDiff gives the diff the size of its box, after a resize of the
// modal, before the next drawing.
func (m *panesModal) resizeFilesDiff() {
	pv := &m.files.pv
	if !pv.on {
		return
	}
	b, ok := m.layout().Box(panes.Files, panes.Preview)
	if !ok {
		return
	}
	if in := b.Rect.Inner(); in.W != pv.w || in.H != pv.h {
		pv.d.SetSize(in.W, in.H)
		pv.w, pv.h = in.W, in.H
		m.files.seq++
	}
}

// filesDiffUpdate gives the diff the messages of its reads, which no other
// part of the modal takes.
func (m *panesModal) filesDiffUpdate(msg tea.Msg) tea.Cmd {
	pv := &m.files.pv
	if !pv.on || !pv.d.Handles(msg) {
		return nil
	}
	var cmd tea.Cmd
	pv.d, cmd = pv.d.Update(msg)
	m.files.seq++
	return cmd
}

// pressFilesDiff takes a key while the diff has the focus: it moves in it,
// and any other key does nothing.
func (m *panesModal) pressFilesDiff(msg tea.KeyPressMsg) tea.Cmd {
	pv := &m.files.pv
	if !pv.on || !pv.d.Focused() {
		return nil
	}
	var cmd tea.Cmd
	pv.d, cmd = pv.d.Update(msg)
	m.files.seq++
	return cmd
}

// filesPreview is the body of the preview beside the tree: the diff of the
// file shown, or a line that says why there is none.
func (m *panesModal) filesPreview(panes.Box) string {
	f := &m.files
	pv := &f.pv
	switch {
	case f.read.err != "":
		return m.theme.Subtle.Render(f.read.err)
	case !f.read.done:
		return m.theme.Subtle.Render("Loading the files…")
	case !pv.on && len(pv.order) == 0 && pv.src != nil:
		return m.theme.Subtle.Render("No files in this range.")
	case !pv.on:
		return m.theme.Subtle.Render("Loading the diff…")
	}
	if err := pv.d.Err(); err != nil {
		return m.theme.Subtle.Render("The diff couldn't be read: " + ui.OneLine(err.Error()))
	}
	if pv.d.Len() == 0 && !pv.d.Done() {
		return m.theme.Subtle.Render("Loading the diff…")
	}
	return pv.d.View()
}

// filesPreviewTitle is the title of the preview beside the tree: that of the
// diff of one file, with the file's place among those of the range.
func (m *panesModal) filesPreviewTitle(b panes.Box) string {
	pv := &m.files.pv
	if pv.path == "" || !pv.on {
		return "Diff"
	}
	i, n := diffview.Place(pv.order, pv.path)
	pct := pv.d.Cursor() * 100 / max(pv.d.Len()-1, 1)
	return diffview.Title(diffview.ThisFile, m.rangeLabel(), pv.path, i, n, viewNote, pct, b.Rect.Inner().W)
}

// rangeLabel names the range of commits the Files pane shows.
func (m *panesModal) rangeLabel() string {
	f := &m.files
	cs := f.commits.Commits
	if r := f.rng; len(cs) > 0 && r.First >= 0 && r.Last < len(cs) && r.First <= r.Last {
		return diffview.RangeLabel(len(cs), r.First, r.Last, cs[r.First].OID, cs[r.Last].OID)
	}
	head := m.detail.HeadSHA
	return diffview.RangeLabel(1, 0, 0, head, head)
}
