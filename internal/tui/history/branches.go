package history

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	historysvc "github.com/eggzec/gh-tui/internal/service/history"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// maxBranchPages bounds the pages of branches read, of a hundred each, so
// that a repository with thousands of branches doesn't read them all.
const maxBranchPages = 10

// branchPageAt is how close to the last branch loaded the cursor gets
// before the next page is read. It pages the list; it isn't a read ahead.
const branchPageAt = 10

// branches is the branch pane: the branches of the repository, the default
// one first, with a filter that narrows them as the user types.
type branches struct {
	items []core.Branch
	// next is the cursor of the next page, and pages the pages read.
	next    string
	pages   int
	loaded  bool
	loading bool
	err     error
	// kept reports that a page shown was served from what an earlier read
	// kept, because GitHub couldn't be reached or rate limited the read,
	// so the branches are read again once it answers.
	kept bool
	// follow names the branch the cursor was on when the branches were
	// read again from the first page, until a page brings it back or the
	// user moves, so that the cursor returns to it from a later page.
	follow string

	cursor, top int
	// defaultBranch is listed first, and the others are compared with it.
	defaultBranch string
	// compares holds how far each branch compared is from the default.
	compares map[string]core.Compare
	// seq counts the moves of the cursor, so that only the last rest
	// compares.
	seq int
	// filter narrows the branches while it is open, and is nil otherwise.
	filter *picker.Model
}

// branchesMsg carries a page of branches read after cursor.
type branchesMsg struct {
	id     int64
	cursor string
	page   core.Page[core.Branch]
	err    error
}

// branchRestMsg reports that the cursor rested on a branch.
type branchRestMsg struct {
	id  int64
	seq int
}

// compareMsg carries how far branch is from the default branch.
type compareMsg struct {
	id     int64
	branch string
	cmp    core.Compare
	err    error
}

func (b *branches) init(defaultBranch string) {
	b.defaultBranch = defaultBranch
	b.compares = map[string]core.Compare{}
}

// selected returns the branch under the cursor.
func (b *branches) selected() (core.Branch, bool) {
	if b.cursor < 0 || b.cursor >= len(b.items) {
		return core.Branch{}, false
	}
	return b.items[b.cursor], true
}

// index returns the index of the branch named name, or -1.
func (b *branches) index(name string) int {
	return slices.IndexFunc(b.items, func(br core.Branch) bool { return br.Name == name })
}

// loadBranches reads the page of branches after cursor. With again set,
// the read asks GitHub rather than serving the page an earlier session kept
// once more.
func (m *Modal) loadBranches(cursor string, again bool) tea.Cmd {
	m.branches.loading = true
	svc, ctx, repo, id := m.svc, m.ctx, m.repo, m.id
	return tea.Batch(m.startSpinner(), func() tea.Msg {
		ctx, end := obs.Begin(ctx, "history.branches")
		p, err := svc.Branches(ctx, historysvc.BranchesQuery{Repo: repo, Cursor: cursor, Again: again})
		end(err, "span", "tui", "repo", repo.String(), "first", cursor == "", "stale", p.Stale, "offline", p.Offline, "limited", p.Limited)
		return branchesMsg{id: id, cursor: cursor, page: p, err: err}
	})
}

// receiveBranches takes a page of branches. A first page kept by an earlier
// session is shown while it is read again.
func (m *Modal) receiveBranches(msg branchesMsg) tea.Cmd {
	b := &m.branches
	if msg.cursor != "" && msg.cursor != b.next {
		return nil
	}
	b.loading = false
	if msg.err != nil {
		b.err = msg.err
		return nil
	}
	b.err = nil
	selected, _ := b.selected()
	if msg.cursor == "" {
		if b.follow == "" {
			b.follow = selected.Name
		}
		b.items, b.pages, b.kept = nil, 0, false
	}
	b.kept = b.kept || msg.page.Offline || msg.page.Limited
	b.items = append(slices.Clip(b.items), msg.page.Items...)
	b.pages++
	b.next = msg.page.Next
	if b.pages >= maxBranchPages {
		b.next = ""
	}
	// The default branch leads, as it is the one most often looked at.
	if i := b.index(b.defaultBranch); i > 0 {
		d := b.items[i]
		b.items = slices.Insert(slices.Delete(b.items, i, i+1), 0, d)
	}
	first := !b.loaded
	b.loaded = true
	switch name := cmp.Or(b.follow, selected.Name); {
	case first:
		b.cursor = max(b.index(m.graph.shown()), 0)
		b.follow = ""
	case name != "":
		if i := b.index(name); i >= 0 {
			b.cursor, b.follow = i, ""
		}
	}
	if b.next == "" {
		// No page left to bring it back.
		b.follow = ""
	}
	b.clamp()
	m.scrollBranches()
	if b.filter != nil {
		b.filter.SetItems(m.filterItems())
	}
	cmds := []tea.Cmd{m.moreBranches()}
	if msg.page.Stale && msg.cursor == "" {
		cmds = append(cmds, m.loadBranches("", true))
	}
	if first {
		cmds = append(cmds, m.branchMoved())
	}
	return tea.Batch(cmds...)
}

func (b *branches) clamp() {
	b.cursor = min(max(b.cursor, 0), max(len(b.items)-1, 0))
}

// moreBranches reads the next page when the cursor nears the end of the
// branches loaded, or while the filter is open, which filters them all.
func (m *Modal) moreBranches() tea.Cmd {
	b := &m.branches
	if b.next == "" || b.loading || b.err != nil {
		return nil
	}
	if b.filter == nil && b.follow == "" && b.cursor+branchPageAt < len(b.items) {
		return nil
	}
	// A later page is appended where it goes rather than shown and read
	// again, so it reads past what an earlier session kept of it.
	return m.loadBranches(b.next, true)
}

// scrollBranches keeps the cursor in the window of the branch pane.
func (m *Modal) scrollBranches() {
	b := &m.branches
	h := max(m.bodyHeight(), 1)
	b.top = min(b.top, b.cursor)
	if b.cursor >= b.top+h {
		b.top = b.cursor - h + 1
	}
	b.top = max(min(b.top, len(b.items)-h), 0)
}

// pressBranches handles a key in the branch pane.
func (m *Modal) pressBranches(msg tea.KeyPressMsg) tea.Cmd {
	b := &m.branches
	k := m.keys.List
	page := max(m.bodyHeight(), 1)
	before := b.cursor
	switch {
	case key.Matches(msg, m.keys.Select):
		br, ok := b.selected()
		if !ok {
			return nil
		}
		return m.showBranch(br.Name)
	case key.Matches(msg, m.keys.Filter):
		return m.openFilter()
	case key.Matches(msg, m.keys.Retry):
		if b.err == nil {
			return nil
		}
		return m.retryBranches()
	case key.Matches(msg, k.Up):
		b.cursor--
	case key.Matches(msg, k.Down):
		b.cursor++
	case key.Matches(msg, k.PageUp):
		b.cursor -= page
	case key.Matches(msg, k.PageDown):
		b.cursor += page
	case key.Matches(msg, k.Home):
		b.cursor = 0
	case key.Matches(msg, k.End):
		b.cursor = len(b.items) - 1
	default:
		return nil
	}
	b.follow = ""
	b.clamp()
	m.scrollBranches()
	if b.cursor == before {
		return nil
	}
	return tea.Batch(m.branchMoved(), m.moreBranches())
}

// branchMoved starts the delay after which the branch under the cursor is
// compared with the default branch, and the branches around it.
func (m *Modal) branchMoved() tea.Cmd {
	return tea.Batch(m.branchRest(), m.compareAround())
}

// branchRest starts the delay after which the branch under the cursor is
// compared with the default branch, unless it was.
func (m *Modal) branchRest() tea.Cmd {
	b := &m.branches
	b.seq++
	br, ok := b.selected()
	if !ok || !m.comparable(br.Name) {
		return nil
	}
	if _, ok := b.compares[br.Name]; ok {
		return nil
	}
	if c, ok := m.svc.CachedCompare(m.repo, b.defaultBranch, br.Name); ok {
		b.compares[br.Name] = c
		return nil
	}
	msg := branchRestMsg{id: m.id, seq: b.seq}
	return tea.Tick(m.opts.prefetch.branches.Rest, func(time.Time) tea.Msg { return msg })
}

// branchPair names a comparison of head with base.
type branchPair struct {
	base, head string
}

// compareAround compares, once the cursor rests, the branches in a window
// around it with the default branch, as the prefetch settings say, so
// that they show at once. The branch under the cursor is compared by
// itself, whatever they say.
func (m *Modal) compareAround() tea.Cmd {
	b := &m.branches
	at := func(j int) (branchPair, bool) {
		if j == b.cursor || j < 0 || j >= len(b.items) || !m.comparable(b.items[j].Name) {
			return branchPair{}, false
		}
		return branchPair{b.defaultBranch, b.items[j].Name}, true
	}
	// Until the branches are listed, there is no window.
	if !b.loaded {
		at = nil
	}
	return m.compares.Window(at, b.cursor)
}

// readCompare compares the branches of k ahead of their use, for
// m.compares.
func (m *Modal) readCompare(ctx context.Context, k branchPair) error {
	_, err := m.svc.Compare(ctx, m.repo, k.base, k.head)
	return err
}

// cachedCompare reports whether the comparison of k is in memory.
func (m *Modal) cachedCompare(k branchPair) bool {
	_, ok := m.svc.CachedCompare(m.repo, k.base, k.head)
	return ok
}

// comparable reports whether name can be compared with the default branch.
func (m *Modal) comparable(name string) bool {
	return m.branches.defaultBranch != "" && name != m.branches.defaultBranch
}

// compare reads how far the branch the cursor rested on is from the
// default branch, unless the cursor moved on.
func (m *Modal) compare(msg branchRestMsg) tea.Cmd {
	b := &m.branches
	br, ok := b.selected()
	if msg.seq != b.seq || !ok || !m.comparable(br.Name) {
		return nil
	}
	if _, ok := b.compares[br.Name]; ok {
		return nil
	}
	svc, ctx, repo, id, base := m.svc, m.ctx, m.repo, m.id, b.defaultBranch
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "history.compare")
		c, err := svc.Compare(ctx, repo, base, br.Name)
		end(err, "span", "tui", "repo", repo.String(), "head", br.Name)
		return compareMsg{id: id, branch: br.Name, cmp: c, err: err}
	}
}

// retryBranches reads again the page of branches that failed.
func (m *Modal) retryBranches() tea.Cmd {
	b := &m.branches
	b.err = nil
	cursor := ""
	if b.loaded {
		cursor = b.next
	}
	return m.loadBranches(cursor, false)
}

// receiveCompare keeps a comparison. A failed one shows nothing: the
// branch is still there to look at.
func (m *Modal) receiveCompare(msg compareMsg) {
	if msg.err == nil {
		m.branches.compares[msg.branch] = msg.cmp
	}
}

// openFilter opens the filter over the branch pane, and reads the pages
// of branches left, so that it filters them all.
func (m *Modal) openFilter() tea.Cmd {
	b := &m.branches
	if !b.loaded {
		return nil
	}
	f := picker.New(nil,
		picker.WithContext(m.ctx),
		picker.WithItems(m.filterItems()),
		picker.WithGroupHeaders(false),
		picker.WithPlaceholder("Filter branches"),
		picker.WithEmptyText("No branches match. Press esc to see them all."),
		picker.WithStyles(filterStyles(m.theme, m.opts.icons)),
		picker.WithSize(m.paneWidth(branchPane), m.bodyHeight()),
	)
	b.filter = &f
	return tea.Batch(f.Focus(), m.moreBranches())
}

// filterItems lists the branches for the filter.
func (m *Modal) filterItems() []picker.Item {
	items := make([]picker.Item, len(m.branches.items))
	for i, br := range m.branches.items {
		items[i] = picker.Item{Title: br.Name, Value: br.Name}
		if br.Name == m.branches.defaultBranch {
			items[i].Detail = "default"
		}
	}
	return items
}

// filterStyles are the picker's styles for the filter, which the pane
// frames, marking a failed search with the error glyph of ic.
func filterStyles(t ui.Theme, ic ui.Icons) picker.Styles {
	s := t.Picker(ic)
	s.Frame = lipgloss.NewStyle()
	return s
}

// updateFilter passes msg to the filter, and shows the branch chosen in it.
func (m *Modal) updateFilter(msg tea.Msg) tea.Cmd {
	b := &m.branches
	if b.filter == nil {
		return nil
	}
	switch msg := msg.(type) {
	case picker.ChosenMsg:
		if msg.ID != b.filter.ID() {
			return nil
		}
		b.filter, b.follow = nil, ""
		name, _ := msg.Item.Value.(string)
		if i := b.index(name); i >= 0 {
			b.cursor = i
			m.scrollBranches()
		}
		return tea.Batch(m.showBranch(name), m.branchMoved())
	case picker.CancelMsg:
		if msg.ID == b.filter.ID() {
			b.filter = nil
		}
		return nil
	}
	var cmd tea.Cmd
	*b.filter, cmd = b.filter.Update(msg)
	return cmd
}

// isBase reports whether the files are shown at name, or at a commit
// chosen on it.
func (m *Modal) isBase(name string) bool {
	if m.base.Ref == "" {
		return name != "" && name == m.defaultBranch
	}
	return m.base.Ref == name || m.base.Branch == name
}

// branchLines renders the branch pane's body, h lines of w cells.
func (m *Modal) branchLines(w, h int) []string {
	b := &m.branches
	if b.filter != nil {
		// The picker renders its size exactly.
		return padLines(strings.Split(b.filter.View(), "\n"), w, h)
	}
	lines := make([]string, 0, h)
	focused := m.focus == branchPane
	for i := b.top; i < len(b.items) && len(lines) < h; i++ {
		lines = append(lines, m.branchRow(b.items[i], i == b.cursor, focused, w))
	}
	if len(lines) < h {
		lines = append(lines, m.branchStatus(w)...)
	}
	return padLines(lines, w, h)
}

// branchStatus is the rows after the branches, in w cells: loading, an
// error, or that there are none.
func (m *Modal) branchStatus(w int) []string {
	b := &m.branches
	var s string
	switch {
	case b.err != nil:
		return m.errorLines("load the branches", m.repo.String(), b.err, false, m.st.noGutter, w)
	case b.loading:
		s = m.spin.View() + m.st.muted.Render("Loading"+m.st.ic.Ellipsis)
	case b.loaded && len(b.items) == 0:
		s = m.st.muted.Render("No branches.")
	default:
		return nil
	}
	return wrap(s, w, m.st.noGutter)
}

// branchRow renders one branch: its name, how far it is from the default
// branch once compared, and whether the files are shown at it.
func (m *Modal) branchRow(br core.Branch, cursor, focused bool, w int) string {
	gutter := m.st.noGutter
	if cursor {
		gutter = m.st.blurGutter
		if focused {
			gutter = m.st.gutter
		}
	}
	var tags []string
	tagsW := 0
	if c, ok := m.branches.compares[br.Name]; ok && br.Name != m.branches.defaultBranch {
		t := m.st.ic.Up + strconv.Itoa(c.AheadBy) + m.st.ic.Down + strconv.Itoa(c.BehindBy)
		tags, tagsW = append(tags, m.st.subtle.Render(t)), ansi.StringWidth(t)
	}
	if m.isBase(br.Name) {
		if tagsW > 0 {
			tagsW++
		}
		tags, tagsW = append(tags, m.st.baseMark), tagsW+1
	}
	style := m.st.text
	switch {
	case br.Name == m.graph.shown():
		style = m.st.accent
	case br.Name == m.branches.defaultBranch:
		style = m.st.strong
	}
	room := w - 2
	nameRoom := room
	if tagsW > 0 && room-tagsW-1 >= min(ansi.StringWidth(br.Name), 8) {
		nameRoom = room - tagsW - 1
	} else {
		tags, tagsW = nil, 0
	}
	name := termtext.Truncate(ui.OneLine(br.Name), nameRoom, m.st.ic.Ellipsis)
	line := gutter + style.Render(name)
	if tagsW == 0 {
		return fit(line, w)
	}
	pad := room - ansi.StringWidth(name) - tagsW
	return line + strings.Repeat(" ", max(pad, 1)) + strings.Join(tags, " ")
}
