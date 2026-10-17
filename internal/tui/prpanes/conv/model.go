package conv

import (
	"context"
	"strings"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

// minWidth is the narrowest conversation that is drawn; a narrower one is
// blank.
const minWidth = 20

// nextID gives each model an identity that its messages carry.
var nextID atomic.Int64

// EnterMsg is sent when enter is pressed on a row that doesn't fold, or has an
// action of its own. Which of Item, Thread and Comment are set depends on
// Kind: a comment or review has its Item, a thread its Thread and first
// Comment, and a reply its Thread and Comment.
type EnterMsg struct {
	ID, Kind string
	Item     *core.TimelineItem
	Thread   *core.Thread
	Comment  *core.ThreadComment
}

// Status says what the conversation shows besides its rows. The host draws
// it.
type Status struct {
	// Loading reports that pages are still being read.
	Loading bool
	// Offline, Limited and Stale report that some page shown was kept, because
	// GitHub couldn't be reached, rate limited the read, or hasn't been asked
	// since the page was kept.
	Offline, Limited, Stale bool
	// Err is why a read failed. The rows stay when there are any.
	Err error
	// Total is how many timeline items and threads were read, including the
	// items that have no row of their own.
	Total int
}

// Option configures a [Model].
type Option func(*Model)

// WithSize sets the size of the conversation in cells.
func WithSize(width, height int) Option {
	return func(m *Model) { m.width, m.height = max(width, 0), max(height, 0) }
}

// WithOptions sets how rows are drawn. Its Width is ignored: the rows are
// drawn as wide as the model.
func WithOptions(o Options) Option { return func(m *Model) { m.opts = o } }

// WithTreeKeys sets the keys of the trees. The host matches its own keys with
// keymap.Matches.
func WithTreeKeys(k foldtree.KeyMap) Option { return func(m *Model) { m.keys = k } }

// WithTreeStyles sets the styles of the trees.
func WithTreeStyles(s foldtree.Styles) Option {
	return func(m *Model) { m.treeStyles, m.hasTreeStyles = s, true }
}

// WithViewer sets the login of the viewer, whose unsubmitted review is drawn
// under their name.
func WithViewer(login string) Option { return func(m *Model) { m.viewer = login } }

// WithVoice sets the voice that words an error.
func WithVoice(v ui.Voice) Option { return func(m *Model) { m.voice = v } }

// withBuilt sets a function that is called each time rows are built.
func withBuilt(f func()) Option { return func(m *Model) { m.built = f } }

// Model is the conversation of a pull request: its timeline and its review
// threads, read page by page, in a tree. Make one with [New]. Models are
// values, but their views are shared by the copies.
type Model struct {
	id  int
	src Source
	// ctx ends the reads, and cancel ends it; seq numbers the reads, so that
	// the messages of an earlier one are dropped.
	ctx    context.Context
	cancel context.CancelFunc
	seq    int

	keys          foldtree.KeyMap
	treeStyles    foldtree.Styles
	hasTreeStyles bool
	opts          Options
	viewer        string
	voice         ui.Voice
	width, height int
	focused       bool

	items   []core.TimelineItem
	threads []core.Thread
	// total is how many threads GitHub says there are.
	total int
	feeds [2]feed
	// checks is the state of the checks of each commit, by its OID in lower
	// case, and commitsErr why the last read of them failed.
	checks     map[string]core.CheckState
	commitsErr error

	// version counts the changes of the data, and optsVer those of the
	// options; with the width they say when rows must be built again.
	version int
	optsVer int
	// shown is the groups the filter shows, order how the conversation is
	// sorted, and shape counts the changes of either, which build the rows of
	// the conversation again. hint is the key that changes the filter, for the
	// text of an empty conversation.
	shown [NumGroups]bool
	order Order
	shape int
	hint  string
	views map[string]*view
	cur   string
	built func()
}

// New returns a conversation that reads src when started with [Model.Init]. It
// starts blurred.
func New(src Source, opts ...Option) Model {
	m := Model{
		id:    int(nextID.Add(1)),
		src:   src,
		seq:   1,
		views: map[string]*view{},
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	for g := range m.shown {
		m.shown[g] = true
	}
	for _, o := range opts {
		o(&m)
	}
	m.view("")
	return m
}

// Init returns the pages that were kept, at once, and then the reads of both
// sources, page after page.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.cachedCmd(timeline), m.cachedCmd(threads), m.cachedCommitsCmd())
}

// Close ends the reads in flight.
func (m Model) Close() { m.cancel() }

// SetSize sets the size of the conversation in cells. Only a new width builds
// the rows again.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	for _, v := range m.views {
		v.tree.SetSize(m.width, m.height)
	}
	m.sync()
}

// SetStyles sets the styles of the rows and of the trees, and builds the rows
// again.
func (m *Model) SetStyles(s Styles, tree foldtree.Styles) {
	m.opts.Styles = s
	m.treeStyles, m.hasTreeStyles = tree, true
	for _, v := range m.views {
		v.tree.SetStyles(tree)
	}
	m.optsVer++
	m.sync()
}

// SetOptions sets how rows are drawn, as [WithOptions] does, and builds them
// again.
func (m *Model) SetOptions(o Options) {
	m.opts = o
	m.optsVer++
	m.sync()
}

// Focus makes the conversation take keys.
func (m *Model) Focus() {
	m.focused = true
	for _, v := range m.views {
		v.tree.Focus()
	}
}

// Blur makes the conversation ignore keys.
func (m *Model) Blur() {
	m.focused = false
	for _, v := range m.views {
		v.tree.Blur()
	}
}

// ready reports whether both sources are settled: each has its first page or
// ended in an error, so that one that failed doesn't hide what the other read.
func (m Model) ready() bool {
	tl, th := m.feeds[timeline], m.feeds[threads]
	return (tl.have || tl.done) && (th.have || th.done)
}

// failed reports whether a source ended in an error.
func (m Model) failed() bool {
	for _, f := range m.feeds {
		if f.err != nil {
			return true
		}
	}
	return false
}

// Status says what is shown besides the rows.
func (m Model) Status() Status {
	st := Status{Total: len(m.items) + len(m.threads)}
	for _, f := range m.feeds {
		st.Loading = st.Loading || !f.done
		st.Offline = st.Offline || f.offline
		st.Limited = st.Limited || f.limited
		st.Stale = st.Stale || f.stale
		if st.Err == nil {
			st.Err = f.err
		}
	}
	return st
}

// Selected returns the ID and kind of the row under the cursor.
func (m Model) Selected() (id, kind string, ok bool) {
	v := m.views[m.cur]
	id, ok = v.tree.Selected()
	if !ok {
		return "", "", false
	}
	n := v.nodes[id]
	if n == nil {
		return "", "", false
	}
	return id, n.kind, true
}

// Update reads on when pages arrive, and moves the cursor and the folds on the
// keys of the focused tree.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadMsg:
		if msg.model != m.id || msg.seq != m.seq {
			return m, nil
		}
		return m.load(msg)
	case commitsMsg:
		if msg.model != m.id || msg.seq != m.seq {
			return m, nil
		}
		return m.commits(msg)
	case ui.OnlineMsg:
		if !m.kept() {
			return m, nil
		}
		return m.reload()
	case foldtree.EnterMsg:
		v := m.views[m.cur]
		if msg.Tree != v.tree.ID() {
			return m, nil
		}
		return m, v.enter(msg.ID)
	case tea.KeyPressMsg:
		if !m.ready() {
			return m, nil
		}
		v := m.views[m.cur]
		var cmd tea.Cmd
		v.tree, cmd = v.tree.Update(msg)
		v.syncForms()
		return m, cmd
	}
	return m, nil
}

// kept reports whether something shown was kept, or a read failed for want of
// an answer, which reading again once GitHub answers may mend.
func (m Model) kept() bool {
	for _, f := range m.feeds {
		if f.offline || f.limited || ui.Unreached(f.err) {
			return true
		}
	}
	return ui.Unreached(m.commitsErr)
}

// View draws the conversation in exactly the width and height it has.
func (m Model) View() string {
	if m.width < minWidth || m.height <= 0 {
		return fit(nil, m.width, m.height)
	}
	v := m.views[m.cur]
	// Rows that can't be built from what one source read, such as threads
	// without their reviews, leave the error to say what is missing.
	if !m.ready() || (len(v.rows) == 0 && m.failed()) {
		return fit(m.placeholder(), m.width, m.height)
	}
	return v.tree.View()
}

// placeholder returns the lines that stand for the rows before they can be
// built: the error that stopped the reads, or a loading line.
func (m Model) placeholder() []string {
	for _, f := range m.feeds {
		if f.err != nil {
			text, hint := ui.ErrorText("load the conversation", "", m.voice)(f.err)
			return ui.ErrorLine(m.opts.Styles.errs, text, hint, m.width)
		}
	}
	return []string{m.opts.Styles.dim.Render("loading")}
}

// fit returns lines as a block of exactly width cells and height lines.
func fit(lines []string, width, height int) string {
	if height <= 0 {
		return ""
	}
	out := make([]string, height)
	for i := range out {
		var l string
		if i < len(lines) {
			l = ansi.Truncate(lines[i], width, "")
		}
		if pad := width - lipgloss.Width(l); pad > 0 {
			l += strings.Repeat(" ", pad)
		}
		out[i] = l
	}
	return strings.Join(out, "\n")
}
