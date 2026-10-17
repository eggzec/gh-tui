package pulls

import (
	"slices"
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// filesKeys are the keys of the Files pane: those of its tree, and the two
// that choose the commits.
type filesKeys struct {
	tree foldtree.KeyMap
	// all selects every commit, and since the commits made since the viewer's
	// last review.
	all, since key.Binding
}

// treeUses reports whether msg is a key of the tree.
func (k filesKeys) treeUses(msg tea.KeyPressMsg) bool {
	for _, row := range k.tree.FullHelp() {
		if slices.ContainsFunc(row, func(b key.Binding) bool { return keymap.Matches(msg, b) }) {
			return true
		}
	}
	return false
}

// hints names the keys that the hints point to, as they are bound.
func (k filesKeys) hints(ic ui.Icons) files.Keys {
	name := func(b key.Binding) string {
		if !b.Enabled() {
			return ""
		}
		return ic.Key(b.Help().Key)
	}
	return files.Keys{Mark: name(k.tree.Mark), SelectAll: name(k.all)}
}

// newFilesKeys makes the keys of the Files pane from keys.
func newFilesKeys(keys config.Keymap) filesKeys {
	pane := ui.In(keys, ctxFiles)
	return filesKeys{
		tree:  foldtree.NewKeyMap(ui.Lookup(keys, ctxFiles)),
		all:   pane.Binding("select_all", "all commits"),
		since: pane.Binding("since_review", "since your review"),
	}
}

// layer lists the keys of the pane in help.
func (k filesKeys) layer() keyhelp.Layer {
	var all []key.Binding
	for _, row := range k.tree.FullHelp() {
		all = append(all, row...)
	}
	all = append(all, k.all, k.since)
	short := append(slices.Clone(k.tree.ShortHelp()), k.all, k.since)
	return ui.ContextLayer(ctxFiles, all, short)
}

// filesPane is what the Files pane shows: its commits, which of them are
// selected, and the tree they are drawn in.
type filesPane struct {
	tree foldtree.Model
	keys filesKeys

	// commits are the commits of head, and loaded says they were read. asked is
	// the head whose commits were asked for, and err says that the read failed.
	commits core.PullCommits
	loaded  bool
	head    string
	asked   string
	err     error
	// rng is the run of commits selected.
	rng files.Range

	// seq counts the changes of what the pane shows, which the drawn panes
	// depend on: a key it took, the commits, the range. A change of theme or
	// icons is m.gen's.
	seq int
	// version counts the changes of commits, and the rows are built again when
	// it, the width, the range, the fold or the styles change.
	version int
	built   filesBuilt
	styles  files.Styles
	// stylesGen is the m.gen the styles were made for.
	stylesGen int

	// read is the read of the files of the range.
	read filesRead

	// loader reads the checks of the commits that are open, and checksVer
	// counts the changes of what it holds, which the rows show. wanted are the
	// commits it was asked for, targets what enter on a row of the checks
	// asks for, and checkStyles draw them, for stylesGen.
	loader      checktree.Loader
	checksVer   int
	wanted      map[string]bool
	targets     checktree.Targets
	checkStyles checktree.Styles
}

// filesBuilt is what the rows of the tree were built from.
type filesBuilt struct {
	ok      bool
	width   int
	version int
	rng     files.Range
	folded  bool
	gen     int
	// data is the version of the files read, and checks that of the checks
	// of the commits.
	data   int
	checks int
}

// filesCommitsMsg carries the commits of a pull request at a head to the modal
// that asked for them.
type filesCommitsMsg struct {
	id      int64
	head    string
	commits core.PullCommits
	err     error
}

func newFilesPane(k filesKeys) filesPane {
	return filesPane{tree: foldtree.New(foldtree.WithKeyMap(k.tree)), keys: k}
}

// slotFiles is the slot of the Files pane.
func filesSlot() slot {
	return slot{context: ctxFiles, content: (*panesModal).filesContent, press: (*panesModal).pressFiles}
}

// readCommits reads the commits at the head of the detail, which the cache
// may have already. It reads once for each head.
func (m *panesModal) readCommits() tea.Cmd {
	head := m.detail.HeadSHA
	f := &m.files
	if head == "" || f.asked == head {
		return nil
	}
	f.asked = head
	q := pulls.PullCommitsQuery{Repo: m.repo, Number: m.number, Head: head}
	if pc, ok := m.svc.CachedPullCommits(q); ok {
		m.setCommits(head, pc)
	}
	svc, ctx, id := m.svc, m.ctx, m.id
	return func() tea.Msg {
		pc, err := svc.PullCommits(ctx, q)
		return filesCommitsMsg{id: id, head: head, commits: pc, err: err}
	}
}

// commitsRead takes the commits that a read brought.
func (m *panesModal) commitsRead(msg filesCommitsMsg) tea.Cmd {
	if msg.id != m.id || msg.head != m.files.asked {
		return nil
	}
	if msg.err != nil {
		if m.ctx.Err() != nil {
			return nil
		}
		m.files.err = msg.err
		m.files.seq++
		return ui.Fail("list the commits of #"+strconv.Itoa(m.number), core.About(core.Target{Repo: m.repo, Number: m.number}.String(), msg.err))
	}
	m.setCommits(msg.head, msg.commits)
	return nil
}

// setCommits keeps the commits of head. A new head selects every commit.
func (m *panesModal) setCommits(head string, pc core.PullCommits) {
	f := &m.files
	pc.Commits = slices.Clone(pc.Commits)
	n := len(pc.Commits)
	if head != f.head || !f.loaded {
		f.rng = files.All(n)
	} else if f.rng.Last >= n {
		f.rng = files.All(n)
	}
	f.commits, f.loaded, f.head, f.err = pc, true, head, nil
	f.version++
	f.seq++
}

// filesContent draws the Files pane.
func (m *panesModal) filesContent(b panes.Box) panes.Content {
	f := &m.files
	switch {
	case f.err != nil && !f.loaded:
		return panes.Content{Body: m.theme.Subtle.Render("The commits couldn't be read.")}
	case !f.loaded:
		return panes.Content{Body: m.theme.Subtle.Render("Loading the commits…")}
	}
	in := b.Rect.Inner()
	if f.stylesGen != m.gen {
		f.styles = files.NewStyles(m.theme, m.icons, m.icons.Set())
		f.checkStyles = checktree.NewStyles(m.theme, m.icons, m.icons.Set())
		f.tree.SetStyles(foldtree.DefaultStyles(m.theme.Dark))
		f.stylesGen = m.gen
	}
	folded := f.tree.Folded(files.IDCommits)
	if !f.built.ok {
		// The commits start folded; the tree keeps the user's folds after.
		folded = true
	}
	want := filesBuilt{true, in.W, f.version, f.rng, folded, m.gen, f.read.version, f.checksVer}
	if f.built != want {
		f.tree.SetRows(m.filesRows(in.W, folded))
		f.built = want
	}
	m.revealFound()
	f.tree.SetSize(in.W, in.H)
	if focused := m.focus == panes.Files && m.sub[panes.Files-1] == panes.List && m.top() == nil; focused != f.tree.Focused() {
		if focused {
			f.tree.Focus()
		} else {
			f.tree.Blur()
		}
	}
	c := panes.Content{Body: f.tree.View()}
	if f.read.done && f.read.err == "" {
		n, add, del := files.Totals(f.read.files)
		c.Info = files.Info(n, add, del, f.styles)
	}
	return c
}

// pressFiles takes the keys of the Files pane: those that choose the commits,
// then the tree's.
func (m *panesModal) pressFiles(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	f := &m.files
	if !f.loaded {
		return nil, false
	}
	k := f.keys
	var cmd tea.Cmd
	switch {
	case keymap.Matches(msg, k.all):
		f.rng = files.All(len(f.commits.Commits))
	case keymap.Matches(msg, k.since):
		cmd = m.selectSince()
	case k.treeUses(msg):
		f.tree.Focus()
		var t tea.Cmd
		f.tree, t = f.tree.Update(msg)
		if t != nil {
			// The tree's messages come at once, as the reply to the key.
			cmd = m.filesMsg(t())
		}
		// A commit may have opened.
		cmd = tea.Batch(cmd, m.wantChecks())
	default:
		return nil, false
	}
	f.seq++
	return cmd, true
}

// selectSince selects the commits made since the viewer's last review.
func (m *panesModal) selectSince() tea.Cmd {
	f := &m.files
	r, ok := files.Since(f.commits)
	if !ok {
		return m.hint(files.HintNoneSince)
	}
	f.rng = r
	if files.Gone(f.commits) {
		return m.hint(files.HintGone)
	}
	return nil
}

// hint shows the words of h, unless it has none.
func (m *panesModal) hint(h files.Hint) tea.Cmd {
	text := h.Text(m.files.keys.hints(m.icons))
	if text == "" {
		return nil
	}
	return ui.Notify(toast.Info, text)
}

// filesMsg takes a message of the tree: a space on a commit changes the range,
// and on another row it says what space works on. Those of another tree are
// ignored.
func (m *panesModal) filesMsg(msg tea.Msg) tea.Cmd {
	f := &m.files
	switch msg := msg.(type) {
	case foldtree.MarkMsg:
		if msg.Tree != f.tree.ID() || msg.Kind != files.KindCommit {
			return nil
		}
		i := slices.IndexFunc(f.commits.Commits, func(c core.PullCommit) bool { return files.CommitID(c.OID) == msg.ID })
		if i < 0 {
			return nil
		}
		r, hint := f.rng.Mark(i, len(f.commits.Commits))
		f.rng = r
		if hint != "" {
			return m.hint(hint)
		}
	case foldtree.NoMarkMsg:
		if msg.Tree == f.tree.ID() {
			return m.hint(files.HintNotACommit)
		}
	case foldtree.EnterMsg:
		// Enter on a job or check of a commit opens it; on a file or a commit
		// without checks it does nothing.
		if target := f.targets[msg.ID]; msg.Tree == f.tree.ID() && target != nil {
			return func() tea.Msg { return target }
		}
	}
	return nil
}

// showCommit focuses the Files pane with only the commit oid selected, the
// commits open and the cursor on it. It does nothing for a commit that the
// pull request doesn't have, or before the commits are read.
func (m *panesModal) showCommit(oid string) {
	f := &m.files
	i := slices.IndexFunc(f.commits.Commits, func(c core.PullCommit) bool { return c.OID == oid })
	if i < 0 {
		return
	}
	m.focus = panes.Files
	m.resetSub()
	f.rng = files.Range{First: i, Last: i}
	f.tree.SetFolded(files.IDCommits, false)
	// The rows are built again before the tree can be asked for the commit.
	f.built.ok = false
	f.tree.SetRows(m.filesRows(max(f.built.width, 40), false))
	f.tree.Reveal(files.CommitID(oid))
	f.seq++
}
