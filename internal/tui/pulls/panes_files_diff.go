package pulls

import (
	"context"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/diffview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
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
	// path is the file the tree follows, and on says that o is built.
	path string
	on   bool
	// o is the diff, the one the diff modal is: it takes the same keys and
	// shows the same drafts and threads.
	o *diffOverlay
	// w and h are the size o was given.
	w, h int
}

// drop ends the reads of the diff and forgets it. The source stays, so that
// the files are not read again when a diff is built over it.
func (pv *filesDiff) drop() {
	if pv.o != nil {
		pv.o.finish()
	}
	pv.on, pv.o = false, nil
}

// filesDiffFocused reports that the diff beside the tree has the keys.
func (m *panesModal) filesDiffFocused() bool {
	pv := &m.files.pv
	return pv.on && m.focus == paneFiles && m.sub[paneFiles-1] == panes.Preview && m.top() == nil
}

// syncFilesDiff makes the diff beside the tree the diff of the file the tree
// follows, once the files of the range are read. It builds a diff when the
// file, the range or the box changes, and drops it when the pane has no box
// for it. It returns the read of the first page of a diff it built.
func (m *panesModal) syncFilesDiff() tea.Cmd {
	f := &m.files
	pv := &f.pv
	b, ok := m.layout().Box(paneFiles, panes.Preview)
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
	// A file the diff stepped to is under the tree's cursor only once the pane
	// is drawn, which reveals it.
	if slices.Contains(pv.order, m.found) {
		want = m.found
	} else if p, ok := m.hoveredFile(); ok && slices.Contains(pv.order, p) {
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
		pv.setSize(in.W, in.H)
		f.seq++
	}
	if focused := m.filesDiffFocused(); pv.on && focused != pv.o.d.Focused() {
		if focused {
			pv.o.d.Focus()
		} else {
			pv.o.d.Blur()
		}
		f.seq++
	}
	return cmd
}

// setSize gives the diff the size of its box.
func (pv *filesDiff) setSize(w, h int) {
	pv.o.w, pv.o.h = w, h
	pv.o.d.SetSize(w, h)
	pv.w, pv.h = w, h
}

// buildFilesDiff shows the file at path in a new diff of w by h cells, and
// returns the read of its first page.
func (m *panesModal) buildFilesDiff(path string, w, h int) tea.Cmd {
	f := &m.files
	pv := &f.pv
	pv.drop()
	ctx, cancel := context.WithCancel(m.ctx)
	o := &diffOverlay{m: m, src: pv.src, keys: m.keys.diff, mode: diffview.ThisFile, path: path, ctx: ctx, cancel: cancel, w: w, h: h}
	o.showCachedViewed()
	o.drafts = m.reviewDrafts()
	// The threads come before the first build, so that cached ones are drawn
	// with it.
	read := o.loadThreads()
	cmd := tea.Batch(o.build(), o.readViewed(), read)
	pv.o, pv.path, pv.on = o, path, true
	pv.w, pv.h = w, h
	f.seq++
	return cmd
}

// restyleFilesDiff gives the diff the colours of a new theme at once, not
// when the next file is built.
func (m *panesModal) restyleFilesDiff() {
	pv := &m.files.pv
	if !pv.on {
		return
	}
	o := pv.o
	o.d.SetStyles(m.theme.Diff(m.icons))
	o.seen = m.theme.Success.Render(m.icons.Yes)
	o.changed = m.theme.Subtle.Render(m.icons.Changed)
	// The blocks are drawn with the theme as well.
	o.setBlocks()
	m.files.seq++
}

// resizeFilesDiff gives the diff the size of its box, after a resize of the
// modal, before the next drawing.
func (m *panesModal) resizeFilesDiff() {
	pv := &m.files.pv
	if !pv.on {
		return
	}
	b, ok := m.layout().Box(paneFiles, panes.Preview)
	if !ok {
		return
	}
	if in := b.Rect.Inner(); in.W != pv.w || in.H != pv.h {
		pv.setSize(in.W, in.H)
		m.files.seq++
	}
}

// filesDiffUpdate gives the diff the messages of its reads, which no other
// part of the modal takes.
func (m *panesModal) filesDiffUpdate(msg tea.Msg) tea.Cmd {
	pv := &m.files.pv
	if !pv.on || !pv.o.d.Handles(msg) {
		return nil
	}
	cmd := pv.o.update(msg)
	m.files.seq++
	return cmd
}

// pressFilesDiff takes a key while the diff has the focus: the diff takes it as
// that of the diff modal does, and the tree follows the file it moves to.
func (m *panesModal) pressFilesDiff(msg tea.KeyPressMsg) tea.Cmd {
	pv := &m.files.pv
	if !pv.on || !pv.o.d.Focused() {
		return nil
	}
	o := pv.o
	cmd := o.update(msg)
	switch {
	case o.mode == diffview.ThisFile && o.path != pv.path:
		// The diff went to another file, such as when it left the whole patch.
		pv.path = o.path
		m.reveal(o.path)
	case slices.Contains(pv.order, m.found):
		// It stepped to another file in the whole patch.
		pv.path = m.found
	}
	m.files.seq++
	return cmd
}

// clearFilesDiff drops what is transient in the diff that has the focus, such as
// a range being picked, and says whether there was any.
func (m *panesModal) clearFilesDiff() bool {
	if !m.filesDiffFocused() || !m.files.pv.o.clear() {
		return false
	}
	m.files.seq++
	return true
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
		return m.theme.Subtle.Render("Loading the files" + m.icons.Ellipsis)
	case !pv.on && len(pv.order) == 0 && pv.src != nil:
		return m.theme.Subtle.Render(ui.None("files in this range"))
	case !pv.on:
		return m.theme.Subtle.Render("Loading the diff" + m.icons.Ellipsis)
	}
	d := &pv.o.d
	if err := d.Err(); err != nil {
		return m.theme.Subtle.Render("The diff couldn't be read: " + ui.OneLine(err.Error()))
	}
	if d.Len() == 0 && !d.Done() {
		return m.theme.Subtle.Render("Loading the diff" + m.icons.Ellipsis)
	}
	return d.View()
}

// filesPreviewTitle is the title of the preview beside the tree: that of the
// diff of one file, with the file's place among those of the range.
func (m *panesModal) filesPreviewTitle(b panes.Box) string {
	pv := &m.files.pv
	if pv.path == "" || !pv.on {
		return "Diff"
	}
	o := pv.o
	path := o.current()
	if path == "" {
		path = pv.path
	}
	i, n := diffview.Place(pv.order, path)
	pct := o.d.Cursor() * 100 / max(o.d.Len()-1, 1)
	// The pane is narrow: the key stays out of its title, so the path and the
	// percentage fit. The status line hints it.
	return m.diffGlyphs().Title(o.mode, m.rangeLabel(), path, i, n, o.viewLabel(path), pct, b.Rect.Inner().W)
}

// rangeLabel names the range of commits the Files pane shows.
func (m *panesModal) rangeLabel() string {
	f := &m.files
	cs := f.commits.Commits
	if r := f.rng; len(cs) > 0 && r.First >= 0 && r.Last < len(cs) && r.First <= r.Last {
		return m.diffGlyphs().RangeLabel(len(cs), r.First, r.Last, cs[r.First].OID, cs[r.Last].OID)
	}
	head := m.detail.HeadSHA
	return m.diffGlyphs().RangeLabel(1, 0, 0, head, head)
}
