package pulls

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// The Files tab of the modal: the tree of the files the pull request
// changes beside the diff of them, or one of the two at a time where the
// modal is too narrow for both. The files are read when the tab is first
// shown, a page at a time as the diff scrolls, and the tree grows with them.
// The first page may be read before, while another tab shows (filesahead.go).

// filesPane is a pane of the Files tab.
type filesPane int

const (
	treePane filesPane = iota
	diffPane
)

// numFilesPanes is how many panes the Files tab has.
const numFilesPanes = 2

// filesPaneTitles name the panes in help.
var filesPaneTitles = [numFilesPanes]string{"files", "diff"}

const (
	// filesWide is the width inside the frame from which the tree and the
	// diff show side by side.
	filesWide = 84
	// treeWidth is the width of the tree beside the diff.
	treeWidth = 32
)

// filesState is the state of the Files tab, once it has been shown.
type filesState struct {
	// cancel stops the reads of the files of this head.
	cancel context.CancelFunc
	// head is the commit the files were read at.
	head string
	set  *fileSet
	diff diff.Model
	tree tree.Model
	// treeStarted says that the tree has loaded its top, which waits for
	// the first files.
	treeStarted bool
	// folded says that the user folded the tree by hand, so that it stops
	// opening the directories of the files that arrive.
	folded bool
	// followed is the file the tree last went to for the diff's cursor.
	followed string
	// settling is the file a finder chose, until the tree is on it. The
	// tree may not hold the file yet when the diff arrives there, since it
	// reads the files a page late, so it is revealed again as it loads.
	settling string
	// kept is set by a read that was served a page GitHub couldn't be
	// asked for, which is read again once it can, and reread says that
	// GitHub answered since.
	kept   *atomic.Bool
	reread bool
	focus  filesPane
	// zoom shows the focused pane alone, where both would fit.
	zoom bool

	spin     spinner.Model
	spinning bool
	// marks are the letters of the statuses, rendered in the theme.
	marks map[diff.Status]string
	theme ui.Theme
}

// loading reports whether the first page of files is on its way.
func (f *filesState) loading() bool {
	return f.diff.Files() == 0 && f.diff.Err() == nil && !f.diff.Done()
}

// canReadFiles reports whether the files can be read: the detail has to
// say which commit is the head.
func (m *detailModal) canReadFiles() bool {
	return m.loaded && m.detail.HeadSHA != ""
}

// onFiles reports whether the Files tab shows.
func (m *detailModal) onFiles() bool { return m.tab == filesTab }

// filesNarrow reports whether the Files tab shows one pane at a time.
func (m *detailModal) filesNarrow() bool { return m.width < filesWide }

// showFiles reads the files when the tab is first shown, once the detail
// has said what the head is, and checks that what was read is of the
// head still. Nothing is read while another tab shows.
func (m *detailModal) showFiles() tea.Cmd {
	switch {
	case m.files != nil:
		return m.checkHead()
	case !m.onFiles() || !m.canReadFiles():
		return nil
	}
	return m.startFiles(nil)
}

// checkHead reads the files again if the head moved since they were read,
// or GitHub answered again after they were served from what was kept, while
// the tab shows. The tab that isn't shown reads them when it is.
func (m *detailModal) checkHead() tea.Cmd {
	f := m.files
	if f == nil || !m.onFiles() {
		return nil
	}
	if !f.reread && (m.detail.HeadSHA == "" || m.detail.HeadSHA == f.head) {
		return nil
	}
	return m.reloadFiles()
}

// reloadFiles reads the files again, with the cursor on the line it was on
// if the file is still there, and the focus and the zoom as they were.
func (m *detailModal) reloadFiles() tea.Cmd {
	old := m.files
	if old == nil {
		return m.showFiles()
	}
	old.cancel()
	var keep *diff.Pos
	path := ""
	if cur, ok := old.diff.CurrentFile(); ok {
		path = cur.Path
		if pos, ok := old.diff.Position(); ok {
			keep = &pos
		}
	}
	cmd := m.startFiles(old)
	f := m.files
	switch {
	case keep != nil:
		f.diff.Seek(*keep)
	case path != "":
		f.diff.SeekFile(path)
	}
	return cmd
}

// startFiles starts reading the files of the head the detail shows, in
// place of those of prev, if any, whose focus and zoom it keeps.
func (m *detailModal) startFiles(prev *filesState) tea.Cmd {
	ctx, cancel := context.WithCancel(m.ctx)
	f := &filesState{
		cancel: cancel,
		head:   m.detail.HeadSHA,
		set:    newFileSet(m.icons.Minus),
		kept:   new(atomic.Bool),
		focus:  diffPane,
		marks:  map[diff.Status]string{},
		theme:  m.theme,
	}
	if prev != nil {
		f.focus, f.zoom, f.folded = prev.focus, prev.zoom, prev.folded
	}
	opts := []diff.Option{
		diff.WithContext(ctx),
		diff.WithKeyMap(m.keys.diff),
		diff.WithStyles(m.theme.Diff(m.icons)),
		diff.WithFocused(f.focus == diffPane),
	}
	if m.detail.ChangedFiles > core.MaxPullFiles {
		opts = append(opts, diff.WithFirstFilesNote(core.MaxPullFiles))
	}
	f.diff = diff.New(m.fetchFiles(f.head, f.kept), opts...)
	f.tree = tree.New(f.set.children,
		tree.WithContext(ctx),
		tree.WithKeyMap(m.keys.tree),
		tree.WithStyles(m.theme.Tree(m.icons)),
		tree.WithIcons(f.icon),
		tree.WithFocused(f.focus == treePane),
		// Empty until the first page says that there are no files, so that
		// a failed read shows no claim about them.
		tree.WithEmptyText(""),
	)
	f.spin = spinner.New(spinner.WithSpinner(m.icons.SpinnerOr(spinner.Dot)))
	f.spin.Style = m.theme.Accent
	f.spinning = true
	m.files = f
	m.layoutFiles()
	return tea.Batch(f.diff.Init(), f.spin.Tick)
}

// fetchFiles reads the pages of the files changed at head, for the diff.
// A page that an earlier session kept is read again at once, which costs
// a request that GitHub answers with no change in most cases, so that the
// diff never shows what the pull request no longer has. A page served
// because GitHub couldn't be reached or limited the read sets kept.
func (m *detailModal) fetchFiles(head string, kept *atomic.Bool) diff.Fetch {
	svc, repo, number := m.svc, m.repo, m.number
	return func(ctx context.Context, cursor string) ([]diff.File, string, error) {
		ctx, end := obs.Begin(ctx, "pull.files")
		q := pulls.FilesQuery{Repo: repo, Number: number, Head: head, Cursor: cursor}
		p, err := svc.Files(ctx, q)
		if err == nil && p.Stale {
			q.Again = true
			p, err = svc.Files(ctx, q)
		}
		end(err, "span", "tui", "repo", repo.String(), "number", number, "first", cursor == "", "files", len(p.Items), "offline", p.Offline, "limited", p.Limited)
		if err != nil {
			return nil, "", err
		}
		if p.Offline || p.Limited {
			kept.Store(true)
		}
		files := make([]diff.File, len(p.Items))
		for i, f := range p.Items {
			files[i] = diff.File{
				Path: f.Path, OldPath: f.PreviousPath, Status: diff.Status(f.Status),
				Additions: f.Additions, Deletions: f.Deletions, Patch: f.Patch, Truncated: f.PatchTruncated,
			}
		}
		return files, p.Next, nil
	}
}

// updateFiles takes what is not a key: the pages the diff fetched, which
// the tree grows with, the tree's own loads, and what follows from them.
func (m *detailModal) updateFiles(msg tea.Msg) tea.Cmd {
	f := m.files
	if f == nil {
		return nil
	}
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if msg.ID != f.spin.ID() {
			break
		}
		if !f.loading() {
			f.spinning = false
			return nil
		}
		var cmd tea.Cmd
		f.spin, cmd = f.spin.Update(msg)
		return cmd
	case tree.OpenMsg:
		if msg.ID != f.tree.ID() {
			return nil
		}
		return m.showFile(msg.Node.ID)
	case diff.FilesMsg:
		if msg.ID != f.diff.ID() {
			return nil
		}
		dirs := f.set.add(msg.Files)
		if msg.Done && f.set.len() == 0 {
			f.tree.SetEmptyText("No files changed.")
		}
		cmds = append(cmds, m.growTree(dirs))
	}
	var diffCmd, treeCmd tea.Cmd
	f.diff, diffCmd = f.diff.Update(msg)
	f.tree, treeCmd = f.tree.Update(msg)
	cmds = append(cmds, diffCmd, treeCmd)
	if !f.treeStarted && f.diff.Err() != nil {
		// Nothing arrives for the tree to show.
		cmds = append(cmds, m.growTree(nil))
	}
	return tea.Batch(append(cmds, m.follow(), m.spinWhileLoading())...)
}

// growTree has the tree read the files it holds now: it loads its top the
// first time, and after that reads again only the directories dirs names,
// which the last page added to. Unless the user folded the tree by hand,
// it then opens every directory.
func (m *detailModal) growTree(dirs []string) tea.Cmd {
	f := m.files
	var cmds []tea.Cmd
	if !f.treeStarted {
		f.treeStarted = true
		cmds = append(cmds, f.tree.Init())
	} else {
		for _, d := range dirs {
			cmds = append(cmds, f.tree.ReloadNode(d))
		}
	}
	if !f.folded {
		cmds = append(cmds, f.tree.ExpandAll())
	}
	return tea.Batch(cmds...)
}

// follow moves the tree's cursor to the file the diff's cursor is in.
func (m *detailModal) follow() tea.Cmd {
	f := m.files
	cur, ok := f.diff.CurrentFile()
	if f.settling != "" {
		_, read := f.set.index(f.settling)
		switch n, on := f.tree.Selected(); {
		case on && n.ID == f.settling:
			f.settling = ""
		case ok && cur.Path == f.settling:
			f.followed = cur.Path
			cmd := f.tree.Reveal(treePath(cur.Path)...)
			if n, on := f.tree.Selected(); on && n.ID == f.settling {
				f.settling = ""
			}
			return cmd
		case read:
			// The diff went elsewhere since.
			f.settling = ""
		case f.diff.Done() && f.set.len() == f.diff.Files():
			// Every page is in and the file isn't.
			path := f.settling
			f.settling = ""
			return ui.Notify(toast.Warning, path+" isn't changed in this pull request.")
		}
	}
	if !ok {
		return nil
	}
	if cur.Path == f.followed {
		return nil
	}
	f.followed = cur.Path
	return f.tree.Reveal(treePath(cur.Path)...)
}

// spinWhileLoading starts the spinner if something loads and it stopped.
func (m *detailModal) spinWhileLoading() tea.Cmd {
	f := m.files
	if f.spinning || !f.loading() {
		return nil
	}
	f.spinning = true
	return f.spin.Tick
}

// showFile puts the diff's cursor on the header of the file at path and
// focuses the diff.
func (m *detailModal) showFile(path string) tea.Cmd {
	f := m.files
	f.diff.SeekFile(path)
	f.followed = path
	f.setFocus(diffPane)
	m.layoutFiles()
	var cmd tea.Cmd
	// Fetches what the seek shows, which the diff does on its next update.
	f.diff, cmd = f.diff.Update(nil)
	return cmd
}

// setFocus focuses pane p and blurs the other.
func (f *filesState) setFocus(p filesPane) {
	f.focus = p
	f.settling = ""
	if p == treePane {
		f.tree.Focus()
		f.diff.Blur()
		return
	}
	f.tree.Blur()
	f.diff.Focus()
}

// pressFiles takes a key on the Files tab: those that refresh and open,
// the panes, and then the keys of the focused pane. Before the files are
// read, only the first two work.
func (m *detailModal) pressFiles(msg tea.KeyPressMsg) tea.Cmd {
	k, f := m.keys, m.files
	switch {
	case key.Matches(msg, k.Refresh):
		return m.refreshFiles()
	case key.Matches(msg, k.Open):
		return m.openFiles()
	case key.Matches(msg, k.Back):
		// The search of the diff is cleared first, then the modal closes.
		if f != nil && f.diff.ClearSearch() {
			return nil
		}
		return m.close()
	case f == nil:
		return nil
	case key.Matches(msg, k.nextPane):
		return m.focusFiles(f.focus + 1)
	case key.Matches(msg, k.prevPane):
		return m.focusFiles(f.focus - 1)
	case key.Matches(msg, k.panes[0]):
		return m.focusFiles(treePane)
	case key.Matches(msg, k.panes[1]):
		return m.focusFiles(diffPane)
	case !m.filesNarrow() && key.Matches(msg, k.zoom):
		f.zoom = !f.zoom
		m.layoutFiles()
		return nil
	}
	// A key of the user's takes the cursor from what a finder chose.
	f.settling = ""
	var cmd tea.Cmd
	if f.focus == treePane {
		if key.Matches(msg, k.tree.Collapse, k.tree.ToggleAll) {
			f.folded = true
		}
		f.tree, cmd = f.tree.Update(msg)
		return cmd
	}
	return m.pressDiff(msg)
}

// searchingDiff reports whether the Files tab shows a diff whose search
// input is open, which takes every key.
func (m *detailModal) searchingDiff() bool {
	return m.onFiles() && m.files != nil && m.files.diff.Capturing()
}

// pressDiff gives a key to the diff.
func (m *detailModal) pressDiff(msg tea.KeyPressMsg) tea.Cmd {
	f := m.files
	var cmd tea.Cmd
	f.diff, cmd = f.diff.Update(msg)
	return tea.Batch(cmd, m.follow(), m.spinWhileLoading())
}

// focusFiles focuses pane p, counting round.
func (m *detailModal) focusFiles(p filesPane) tea.Cmd {
	f := m.files
	f.setFocus((p + numFilesPanes) % numFilesPanes)
	m.layoutFiles()
	return nil
}

// refreshFiles reads again what failed to load, or else the pull request
// and its files.
func (m *detailModal) refreshFiles() tea.Cmd {
	if f := m.files; f != nil && f.diff.Err() != nil {
		cmd := f.diff.Retry()
		return tea.Batch(cmd, m.spinWhileLoading())
	}
	m.svc.Invalidate(m.repo)
	return tea.Batch(m.get(), m.reloadFiles())
}

// openFiles opens the diff of the file under the cursor on GitHub: that of
// the file selected in the tree, while it has the focus, and otherwise the
// one the diff is in. Without a file it opens the page of the files.
func (m *detailModal) openFiles() tea.Cmd {
	if m.detail.URL == "" {
		return nil
	}
	url := m.detail.URL + "/files"
	if p := m.fileUnderCursor(); p != "" {
		url += "#" + ui.DiffAnchor(p)
	}
	return ui.Open(url)
}

// fileUnderCursor returns the path of the file that the cursor of the
// focused pane is on, or empty for none.
func (m *detailModal) fileUnderCursor() string {
	f := m.files
	if f == nil {
		return ""
	}
	if f.focus == treePane {
		if n, ok := f.tree.Selected(); ok {
			if file, ok := n.Value.(diff.File); ok {
				return file.Path
			}
		}
		return ""
	}
	if cur, ok := f.diff.CurrentFile(); ok {
		return cur.Path
	}
	return ""
}

// layoutFiles gives the panes their room: side by side where they fit,
// otherwise the focused one takes it all, below a line of its own.
func (m *detailModal) layoutFiles() {
	f := m.files
	if f == nil {
		return
	}
	body := max(m.height-1, 0)
	tw, dw := m.width, m.width
	if !m.filesNarrow() && !f.zoom {
		tw = min(treeWidth, m.width)
		dw = max(m.width-tw-1, 0)
	}
	f.tree.SetSize(tw, body)
	f.diff.SetSize(dw, body)
}

// restyleFiles draws the tab in the theme of the modal.
func (m *detailModal) restyleFiles() {
	f := m.files
	if f == nil {
		return
	}
	f.theme = m.theme
	clear(f.marks)
	f.diff.SetStyles(m.theme.Diff(m.icons))
	f.tree.SetStyles(m.theme.Tree(m.icons))
	f.tree.SetIcons(f.icon)
	f.spin.Style = m.theme.Accent
}

// statusLetter is the letter that stands for how a file changed.
func statusLetter(s diff.Status) string {
	switch s {
	case diff.StatusAdded:
		return "A"
	case diff.StatusRemoved:
		return "D"
	case diff.StatusRenamed:
		return "R"
	case diff.StatusCopied:
		return "C"
	case diff.StatusModified, diff.StatusChanged, diff.StatusUnchanged:
	}
	return "M"
}

// mark returns the letter of status s, coloured by what it says.
func (f *filesState) mark(s diff.Status) string {
	if r, ok := f.marks[s]; ok {
		return r
	}
	st := f.theme.Warning
	switch s {
	case diff.StatusAdded:
		st = f.theme.Success
	case diff.StatusRemoved:
		st = f.theme.Error
	case diff.StatusRenamed, diff.StatusCopied:
		st = f.theme.Accent
	case diff.StatusModified, diff.StatusChanged, diff.StatusUnchanged:
	}
	r := st.Render(statusLetter(s))
	f.marks[s] = r
	return r
}

// icon is a tree.Icons: a file has the letter of its status, and a
// directory nothing but the tree's own mark.
func (f *filesState) icon(n tree.Node, _ bool) string {
	if file, ok := n.Value.(diff.File); ok {
		return f.mark(file.Status)
	}
	return ""
}

// stat renders how file changed, such as "M +120 −4", in colours.
func (m *detailModal) stat(file diff.File) string {
	f := m.files
	parts := []string{f.mark(file.Status)}
	if file.Additions > 0 {
		parts = append(parts, m.theme.Success.Render("+"+strconv.Itoa(file.Additions)))
	}
	if file.Deletions > 0 {
		parts = append(parts, m.theme.Error.Render(m.icons.Minus+strconv.Itoa(file.Deletions)))
	}
	return strings.Join(parts, " ")
}

// filesView renders the Files tab in exactly the modal's size.
func (m *detailModal) filesView() string {
	f := m.files
	if f == nil {
		return strings.Join(ui.FitLines([]string{m.filesWaiting()}, m.width, m.height), "\n")
	}
	body := max(m.height-1, 0)
	switch {
	case m.filesNarrow():
		lines := append([]string{ui.Fit(m.breadcrumb(), m.width)}, m.paneLines(f.focus, m.width, body)...)
		return strings.Join(lines, "\n")
	case f.zoom:
		lines := append([]string{m.paneTitle(f.focus, m.width)}, m.paneLines(f.focus, m.width, body)...)
		return strings.Join(lines, "\n")
	}
	dw := max(m.width-treeWidth-1, 0)
	left := append([]string{m.paneTitle(treePane, treeWidth)}, m.paneLines(treePane, treeWidth, body)...)
	right := append([]string{m.paneTitle(diffPane, dw)}, m.paneLines(diffPane, dw, body)...)
	sep := m.st.sepLine.Render(m.icons.Border.Left)
	var b strings.Builder
	b.Grow(m.height * (m.width + 64))
	for i := range m.height {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(left[i] + sep + right[i])
	}
	return b.String()
}

// filesWaiting renders the line that stands for the tab until the files can
// be read: that the pull request failed to load, or has no head to read
// the files at, with how to read it again, or that it loads.
func (m *detailModal) filesWaiting() string {
	var text string
	switch {
	case m.failed != nil:
		msg, _, _ := strings.Cut(m.failed.Error(), "\n")
		text = "Couldn't load the pull request: " + termtext.OneLine(msg)
	case m.loaded:
		text = "The pull request has no head commit to read the files at."
	default:
		return m.theme.Muted.Render("Loading" + m.icons.Ellipsis)
	}
	line := m.theme.Error.Render(m.icons.Error + " " + text)
	if k := m.keys.Refresh.Help().Key; k != "" && m.keys.Refresh.Enabled() {
		line += m.theme.Subtle.Render(m.icons.Separator + k + " to retry")
	}
	return line
}

// paneLines renders the body of pane p, h lines of w cells.
func (m *detailModal) paneLines(p filesPane, w, h int) []string {
	f := m.files
	if f.loading() {
		// One spinner says that the files load, in the diff when both
		// show, and else in the pane that does.
		if p == treePane && !m.filesNarrow() && !f.zoom {
			return ui.FitLines(nil, w, h)
		}
		return ui.FitLines([]string{f.spin.View() + " " + m.theme.Muted.Render("Loading files"+m.icons.Ellipsis)}, w, h)
	}
	if p == treePane {
		return ui.FitLines(strings.Split(f.tree.View(), "\n"), w, h)
	}
	return ui.FitLines(strings.Split(f.diff.View(), "\n"), w, h)
}

// paneTitle renders the line over pane p: what it shows, in the accent
// while it has the focus.
func (m *detailModal) paneTitle(p filesPane, w int) string {
	f := m.files
	st := m.theme.Muted
	if p == f.focus {
		st = m.theme.Accent.Bold(true)
	}
	if p == treePane {
		return ui.Fit(st.Render("Files"), w)
	}
	cur, ok := f.diff.CurrentFile()
	if !ok {
		return ui.Fit(st.Render("Diff"), w)
	}
	return ui.Spread(st.Render(termtext.OneLine(cur.Path)), m.stat(cur), w, m.icons.Ellipsis)
}

// breadcrumb renders where the narrow tab is: the number of the file in
// the diff among all, its path, and how it changed.
func (m *detailModal) breadcrumb() string {
	f := m.files
	cur, ok := f.diff.CurrentFile()
	if !ok {
		return m.theme.Accent.Bold(true).Render("Files")
	}
	n, _ := f.set.index(cur.Path)
	total := strconv.Itoa(f.set.len())
	if !f.diff.Done() {
		total = strconv.Itoa(max(m.detail.ChangedFiles, f.set.len()))
	}
	lead := m.theme.Muted.Render(strconv.Itoa(n+1)+"/"+total) + m.theme.Subtle.Render(" "+m.icons.Crumb+" ")
	stat := "  " + m.stat(cur)
	room := max(m.width-ansi.StringWidth(lead)-ansi.StringWidth(stat), 1)
	p := termtext.OneLine(cur.Path)
	if ansi.StringWidth(p) > room {
		// The end of a path says more than its start.
		p = ansi.TruncateLeft(p, ansi.StringWidth(p)-room+ansi.StringWidth(m.icons.Ellipsis), m.icons.Ellipsis)
	}
	return lead + m.theme.Accent.Bold(true).Render(p) + stat
}

// filesLayer returns the layer of the keys of the pane of the Files tab
// that has the focus.
func (m *detailModal) filesLayer() keyhelp.Layer {
	f := m.files
	if f == nil || f.focus == diffPane {
		keys := m.keys.diff
		if f != nil {
			keys = f.diff.KeyMap()
		}
		// The modal's refresh retries a failed read.
		keys.Retry.SetEnabled(false)
		l := ui.ContextHelp(ctxDiff, keys, false)
		// Backspace on an empty search line is the prompt's alone. Listed
		// here, though off, it would hide the app's back key in help.
		l.Bindings = slices.DeleteFunc(l.Bindings, func(b key.Binding) bool { return slices.Equal(b.Keys(), keys.CancelEmpty.Keys()) })
		return l
	}
	return ui.ContextHelp(ctxFiles, m.keys.tree, false)
}

// filesModalLayer returns the layer of the keys of the modal while the
// Files tab shows: its changes and tabs, and the keys of the panes.
func (m *detailModal) filesModalLayer(k keyMap, owner key.Binding) keyhelp.Layer {
	k.zoom.SetEnabled(k.zoom.Enabled() && !m.filesNarrow())
	if f := m.files; f != nil {
		if f.zoom {
			k.zoom = renamed(k.zoom, "unzoom")
		}
		if f.diff.Err() != nil {
			k.Refresh = renamed(k.Refresh, "retry")
		}
	}
	keys := []key.Binding{k.Merge, k.Close, k.Reopen, k.ToggleDraft, k.Checks, k.References, k.FindFile, k.NextTab, k.PrevTab, k.nextPane, k.prevPane, k.jump, k.zoom, k.Open, k.Refresh, k.Back}
	l := ui.ContextLayer(ctxModal, keys, []key.Binding{k.Merge, k.Close, k.Reopen, k.nextPane, k.zoom, k.Open})
	l.Bindings = append(l.Bindings, owner)
	return l
}

// renamed returns b labelled desc in help.
func renamed(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}

// filesOnline reads again, now that GitHub answers again, the files that
// failed for want of an answer from it, and those that were served from
// what was kept for want of one, once the tab shows.
func (m *detailModal) filesOnline() tea.Cmd {
	f := m.files
	var retry tea.Cmd
	if ui.Unreached(f.diff.Err()) {
		retry = tea.Batch(f.diff.Retry(), m.spinWhileLoading())
	}
	if f.kept.Swap(false) {
		f.reread = true
	}
	return tea.Batch(retry, m.checkHead())
}
