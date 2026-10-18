package pulls

import (
	"reflect"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/actions"
	pullsvc "github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/merge"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// ctxMerge is the context of the keys of the Merge pane.
const ctxMerge = "pull_merge"

// updateSettle is how long the modal waits after a branch was updated before
// it reads the pull request again: an earlier read may still see the old head.
const updateSettle = 2 * time.Second

// mergeKeys are the keys of the Merge pane: those of its tree, and the key
// that updates the branch.
type mergeKeys struct {
	tree   foldtree.KeyMap
	update key.Binding
}

// mergeLookup gives the actions the Merge context has. No row of its tree has
// a kind to mark, so the tree's mark key has no line in the config and no
// binding here.
type mergeLookup struct {
	ui.Context
	keys config.Keymap
}

// Of implements keymap.Lookup. It asks Has, not Of, for what the config may
// lack, since Of is how the code reads the config.
func (l mergeLookup) Of(action string) []string {
	if !strings.Contains(action, ".") && !l.keys.Has(l.Scope()+"."+action) {
		return nil
	}
	return l.Context.Of(action)
}

func newMergeKeys(keys config.Keymap) mergeKeys {
	tree := foldtree.NewKeyMap(mergeLookup{keys: keys, Context: ui.In(keys, ctxMerge)})
	// Help lists no key that nothing takes.
	tree.Mark = key.Binding{}
	return mergeKeys{
		tree:   tree,
		update: ui.In(keys, ctxMerge).Binding("update_branch", "update branch"),
	}
}

// help returns every binding of the pane, and those its short help shows.
func (k mergeKeys) help() (all, short []key.Binding) {
	for _, row := range k.tree.FullHelp() {
		for _, b := range row {
			if b.Help().Desc != "" {
				all = append(all, b)
			}
		}
	}
	return append(all, k.update), append(k.tree.ShortHelp(), k.update)
}

// mergePane is the state of the Merge pane: what stands in the way of a merge,
// as rows of a tree. It is rebuilt when what it shows changes, never in View.
type mergePane struct {
	keys  mergeKeys
	model merge.Model
	// reader and loader read the checks of the head commit; has is false
	// where there is no reader, and the pane then shows no checks.
	reader commitReader
	has    bool
	loader checktree.Loader
	wanted string
	// watch follows the checks of a head commit while some are pending. stop
	// ends the follow of watched, which is empty while none runs.
	watch   CommitWatch
	stop    func()
	watched string
	// behind is how many commits the head lacks of the base, or -1 while it
	// isn't known, and asked is the read it comes from.
	behind int
	asked  pullsvc.BehindQuery

	styles merge.Styles
	data   merge.Data
	now    func() time.Time
	// width and height are the size of the tree's box last drawn, and version
	// counts the changes of what the pane shows, for the cache of the drawn
	// panes.
	width, height int
	version       int
}

func newMergePane(s *Section, repo core.RepoRef) *mergePane {
	k := newMergeKeys(s.rawKeys)
	p := &mergePane{
		keys:   k,
		model:  merge.New(foldtree.WithKeyMap(k.tree), foldtree.WithFocused(true)),
		behind: -1,
		now:    s.now,
	}
	if s.commits != nil {
		p.reader, p.has = commitReader{s.commits}, true
		p.loader = checktree.NewLoader(p.reader, repo)
		p.watch = s.commitWatch
	}
	return p
}

// syncWatch follows the checks of the head commit while some of them are
// pending, and only then: it stops when none is, when the head changes, and
// once the modal closed.
func (p *mergePane) syncWatch(m *panesModal, sha string, c core.CommitChecks, loaded bool) {
	if p.watch == nil {
		return
	}
	want := !m.closed && loaded && sha != "" && c.Pending()
	if p.stop != nil && (!want || p.watched != sha) {
		p.unwatch()
	}
	if want && p.stop == nil {
		p.stop, p.watched = p.watch(actions.CommitChecksQuery{Repo: m.repo, SHA: sha}), sha
	}
}

// unwatch ends the follow of the checks, if one runs.
func (p *mergePane) unwatch() {
	if p.stop != nil {
		p.stop()
	}
	p.stop, p.watched = nil, ""
}

// synced takes up the checks that a poll stored, when it is those of the
// commit followed.
func (p *mergePane) synced(m *panesModal, msg ui.SyncMsg) tea.Cmd {
	if msg.Err != nil || p.stop == nil || msg.Key != actions.CommitChecksSyncKey(actions.CommitChecksQuery{Repo: m.repo, SHA: p.watched}) {
		return nil
	}
	var cmd tea.Cmd
	p.loader, cmd = p.loader.Reload(p.watched)
	p.rebuild(m, false)
	return cmd
}

// restyle draws the pane with the theme and icons of m.
func (p *mergePane) restyle(m *panesModal) {
	p.styles = merge.NewStyles(m.theme, m.icons, m.icons.Set())
	// No row says that nothing blocks: the ready mark in the title does.
	st := foldtree.DefaultStyles(m.theme.Dark)
	st.EmptyText = ""
	p.model.SetStyles(st)
	p.rebuild(m, true)
}

func (p *mergePane) options() merge.Options {
	return merge.Options{
		Width:  p.width,
		Styles: p.styles,
		Now:    p.now(),
		Keys:   merge.Hints{Update: hintKeys(p.keys.update)},
	}
}

// refresh reads what the pane needs of the detail that m holds: the checks of
// its head, and how far it is behind its base. It rebuilds the rows.
func (p *mergePane) refresh(m *panesModal) tea.Cmd {
	var cmds []tea.Cmd
	if sha := m.detail.HeadSHA; p.has && m.loaded && sha != "" && sha != p.wanted {
		p.wanted = sha
		var cmd tea.Cmd
		p.loader, cmd = p.loader.Want(sha, false)
		cmds = append(cmds, cmd)
	}
	cmds = append(cmds, p.readBehind(m))
	p.rebuild(m, false)
	return tea.Batch(cmds...)
}

// readBehind counts the commits that the head lacks, when it is behind and
// both commits are known, and the service can.
func (p *mergePane) readBehind(m *panesModal) tea.Cmd {
	d := m.detail
	svc, ok := m.svc.(Behinds)
	if !ok || !m.loaded || d.Merge.Status != core.MergeBehind || d.HeadSHA == "" || d.BaseSHA == "" {
		p.behind, p.asked = -1, pullsvc.BehindQuery{}
		return nil
	}
	q := pullsvc.BehindQuery{Repo: m.repo, Head: d.HeadSHA, Base: d.BaseSHA}
	if q != p.asked {
		p.behind = -1
	}
	if c, ok := svc.CachedBehind(q); ok {
		p.behind, p.asked = c.AheadBy, q
		return nil
	}
	if q == p.asked {
		return nil
	}
	p.asked = q
	ctx, id := m.ctx, m.id
	return func() tea.Msg {
		c, err := svc.Behind(ctx, q)
		return mergeBehindMsg{id: id, q: q, n: c.AheadBy, err: err}
	}
}

// rebuild makes the rows from what m holds, and counts a change unless nothing
// changed.
func (p *mergePane) rebuild(m *panesModal, force bool) {
	d := merge.Data{Detail: m.detail, Behind: p.behind}
	if sha := d.Detail.HeadSHA; p.has && sha != "" {
		d.Checks, d.ChecksLoaded = p.reader.CachedCommitChecks(checktree.Query{Repo: m.repo, SHA: sha})
	}
	p.syncWatch(m, d.Detail.HeadSHA, d.Checks, d.ChecksLoaded)
	if !force && reflect.DeepEqual(d, p.data) {
		return
	}
	p.data = d
	p.model.SetRows(merge.Rows(d, p.options()))
	p.version++
}

// fit sizes the tree to the box it is drawn in, and makes the rows again when
// the width changed, which they are cut to.
func (p *mergePane) fit(m *panesModal, w, h int) {
	if w == p.width && h == p.height {
		return
	}
	widthChanged := w != p.width
	p.width, p.height = w, h
	// The branch line, the rule and the actions line take three rows.
	p.model.SetSize(w, max(h-3, 0))
	if widthChanged {
		p.rebuild(m, true)
	}
}

// blockers are the forms of the count that the title shows, longest first.
func blockers(n int) []string {
	long := strconv.Itoa(n) + " blockers"
	if n == 1 {
		long = "1 blocker"
	}
	return []string{long, strconv.Itoa(n)}
}

// mergeContent is the Merge pane: the branches, the blockers, and the actions.
func (m *panesModal) mergeContent(b panes.Box) panes.Content {
	p := m.merge
	in := b.Rect.Inner()
	p.fit(m, in.W, in.H)
	o := p.options()
	var c panes.Content
	if m.loaded {
		if n := merge.Count(p.data); n == 0 {
			// Until the checks are known, they may block.
			if !p.has || p.data.Detail.HeadSHA == "" || p.data.ChecksLoaded {
				c.Info = []string{p.styles.Ready}
			}
		} else {
			c.Info = blockers(n)
		}
	}
	lines := []string{merge.Branch(p.data, o)}
	if in.H > 3 {
		lines = append(lines, p.model.View())
	}
	lines = append(lines, merge.Rule(o), merge.Actions(p.data, o))
	c.Body = strings.Join(lines, "\n")
	return c
}

// pressMerge takes a key of the Merge pane: the key that updates the branch,
// and the keys of its tree.
func (m *panesModal) pressMerge(k tea.KeyPressMsg) (tea.Cmd, bool) {
	p := m.merge
	if keymap.Matches(k, p.keys.update) {
		return m.updateBranch(), true
	}
	var cmd tea.Cmd
	p.model, cmd = p.model.Update(k)
	p.version++
	return cmd, true
}

// mergeBehindMsg carries the count of commits that a read found the head to be
// behind its base.
type mergeBehindMsg struct {
	id  int64
	q   pullsvc.BehindQuery
	n   int
	err error
}

// mergeSentMsg reports that GitHub answered a change that the pane asked for.
type mergeSentMsg struct {
	owner *panesModal
	what  string
	// done is the toast of a change that went through, and settle how long to
	// wait before the pull request is read again.
	done       string
	invalidate bool
	settle     time.Duration
	err        error
}

// mergeRevalidateMsg asks the modal that sent it to read its pull request again.
type mergeRevalidateMsg struct{ owner *panesModal }

// mergeUpdate takes what the Merge pane asked for or waits for: the rows that
// enter chose, the answers of the reads, and the changes it sent.
func (m *panesModal) mergeUpdate(msg tea.Msg) tea.Cmd {
	p := m.merge
	switch msg := msg.(type) {
	case foldtree.EnterMsg:
		var cmd tea.Cmd
		p.model, cmd = p.model.Update(msg)
		p.version++
		return cmd
	case merge.FocusReviewMsg:
		return m.jumpConv(convWant{login: msg.Login, review: core.ReviewStateChangesRequested})
	case merge.UpdateBranchMsg:
		return m.updateBranch()
	case merge.MarkReadyMsg:
		return m.markReady()
	case mergeBehindMsg:
		if msg.id == m.id && msg.q == p.asked && msg.err == nil {
			p.behind = msg.n
			p.rebuild(m, false)
		}
	case mergeSentMsg:
		return m.mergeSent(msg)
	case mergeRevalidateMsg:
		if msg.owner == m {
			return m.revalidate()
		}
	}
	return nil
}

// updateBranch asks whether to update the branch with its base.
func (m *panesModal) updateBranch() tea.Cmd {
	d := m.detail
	up, ok := m.svc.(BranchUpdater)
	if !m.loaded || !ok || d.HeadSHA == "" {
		return ui.Notify(toast.Info, "The branch can't be updated yet.")
	}
	head, base := d.HeadRef, d.BaseRef
	m.ask(ui.Confirm{Question: "Update branch with " + base + "?", Run: func() tea.Cmd {
		op := ui.About(core.Target{Repo: m.repo, Number: m.number}.String(), up.UpdateBranch(m.repo, m.number, d.HeadSHA))
		return m.sendChange(op, mergeSentMsg{
			what: "update the branch of #" + strconv.Itoa(m.number), done: "Updated " + head + " with " + base,
			invalidate: true, settle: updateSettle,
		})
	}})
	return nil
}

// markReady asks whether to mark the draft as ready for review.
func (m *panesModal) markReady() tea.Cmd {
	if !m.loaded {
		return nil
	}
	num := "#" + strconv.Itoa(m.number)
	m.ask(ui.Confirm{Question: "Mark " + num + " ready for review?", Run: func() tea.Cmd {
		op := ui.About(core.Target{Repo: m.repo, Number: m.number}.String(), m.svc.MarkReady(m.repo, m.number))
		// The change is in the cache already, so the draft row goes at once.
		send := m.sendChange(op, mergeSentMsg{what: "mark " + num + " ready for review", done: "Marked " + num + " ready for review."})
		return tea.Batch(send, m.rereadCache())
	}})
	return nil
}

// rereadCache takes the detail that the service holds, which a change or its
// rollback has just changed, and rebuilds the rows from it.
func (m *panesModal) rereadCache() tea.Cmd {
	if d, ok := m.svc.CachedGet(m.repo, m.number); ok {
		m.detail = d
	}
	return m.merge.refresh(m)
}

// sendChange sends op, and answers with sent once GitHub did.
func (m *panesModal) sendChange(op ui.Op, sent mergeSentMsg) tea.Cmd {
	ctx := m.sendCtx
	sent.owner = m
	return func() tea.Msg {
		sent.err = op.Do(ctx)
		return sent
	}
}

// mergeSent tells how a change of the pane went, and reads the pull request
// again once it went through.
func (m *panesModal) mergeSent(msg mergeSentMsg) tea.Cmd {
	if msg.owner != m {
		return nil
	}
	if msg.err != nil {
		// A change that failed was rolled back, which the rows show.
		return tea.Batch(ui.Fail(msg.what, msg.err), m.rereadCache())
	}
	if msg.invalidate {
		m.svc.Invalidate(m.repo)
	}
	read := m.revalidate()
	if msg.settle > 0 {
		read = tea.Tick(msg.settle, func(time.Time) tea.Msg { return mergeRevalidateMsg{owner: m} })
	}
	return tea.Batch(ui.Notify(toast.Success, msg.done), read)
}
