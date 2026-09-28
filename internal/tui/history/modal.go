// Package history is the History modal of the repository screen: the
// branches of the repository, the graph of the branch chosen, and what the
// commit under the graph's cursor changed, in three panes of one frame. A
// branch or a commit can become the base that the Files pane shows the
// repository at.
//
// The commit pane follows the graph's cursor. A commit whose detail is in
// memory shows at once; any other is read once the cursor rests on it,
// together with the commits around it, so that moving on by a few rows
// finds them in memory. What a commit changed never changes, so reading it
// again, even in a later session, makes no request.
//
// On a narrow terminal the modal shows one pane at a time, with a
// breadcrumb of where it is, and zoomed it shows the focused pane alone at
// any width.
package history

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// narrowWidth is the width inside the frame below which the panes don't fit
// side by side, so the modal shows one at a time.
const narrowWidth = 90

// pane is one of the modal's panes, in the order the focus moves through
// them.
type pane int

const (
	branchPane pane = iota
	graphPane
	commitPane
)

// numPanes is how many panes there are.
const numPanes = 3

var lastID atomic.Int64

// Modal is the History modal. Create one with [New].
type Modal struct {
	id int64
	// ctx bounds the modal's reads, and cancel ends them when it closes.
	ctx    context.Context
	cancel context.CancelFunc
	svc    Service
	repo   core.RepoRef
	// defaultBranch is the repository's default branch, or empty if the
	// app hasn't read it yet; base is what the files are shown at.
	defaultBranch string
	base          ui.BaseMsg
	keys          KeyMap
	opts          options
	format        format

	focus    pane
	branches branches
	graph    commits
	commit   commit
	// ahead reads the details of the commits around the cursor.
	ahead *ui.Ahead[string]
	// seq counts the moves of the graph's cursor, so that only the last
	// rest reads.
	seq int
	// zoom shows the focused pane alone, as a narrow modal does.
	zoom bool

	spin     spinner.Model
	spinning bool

	width, height int
	theme         ui.Theme
	st            styles
	errs          ui.ErrorStyles
}

var _ ui.Modal = (*Modal)(nil)

// Opener returns what the app opens the history with, for tui.WithHistory:
// a new modal each time, which reads what it shows from svc and takes its
// keys from the configured keys.
func Opener(svc Service, keys map[string][]string, opts ...Option) func(ctx context.Context, repo core.RepoRef, defaultBranch string, base ui.BaseMsg) (ui.Modal, tea.Cmd) {
	return func(ctx context.Context, repo core.RepoRef, defaultBranch string, base ui.BaseMsg) (ui.Modal, tea.Cmd) {
		m := New(ctx, svc, repo, defaultBranch, base, keys, opts...)
		load := m.Init()
		return m, load
	}
}

// CommitOpener returns what the app opens the history on a commit with, for
// tui.WithCommit: a new modal each time, on the history of commit sha of
// repo, with the commit pane focused on it, such as for a notification of
// the commit. defaultBranch may be empty.
func CommitOpener(svc Service, keys map[string][]string, opts ...Option) func(ctx context.Context, repo core.RepoRef, sha, defaultBranch string) (ui.Modal, tea.Cmd) {
	return func(ctx context.Context, repo core.RepoRef, sha, defaultBranch string) (ui.Modal, tea.Cmd) {
		m := New(ctx, svc, repo, defaultBranch, ui.BaseMsg{}, keys, append(slices.Clip(opts), onCommit(sha))...)
		load := m.Init()
		return m, load
	}
}

// New returns the history of repo, which opens on the branch that base was
// chosen from, or else on defaultBranch, with the graph focused. ctx bounds
// its reads until it closes. Call Init once it is open.
func New(ctx context.Context, svc Service, repo core.RepoRef, defaultBranch string, base ui.BaseMsg, keys map[string][]string, opts ...Option) *Modal {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	if o.voice == nil {
		v := ui.NewVoice(keys, "")
		o.voice = &v
	}
	ctx, cancel := context.WithCancel(ctx)
	m := &Modal{
		id:            lastID.Add(1),
		ctx:           ctx,
		cancel:        cancel,
		svc:           svc,
		repo:          repo,
		defaultBranch: defaultBranch,
		base:          base,
		keys:          newKeyMap(keys),
		opts:          o,
		format:        newFormat(o.cfg, o.loc),
		focus:         graphPane,
		spin:          spinner.New(spinner.WithSpinner(spinner.Dot)),
		commit:        newCommit(o.editor),
	}
	m.ahead = ui.NewAhead("commit", m.readDetail, m.cachedDetail, 0, 0)
	m.branches.init(defaultBranch)
	start := base.Branch
	if base.Ref == "" || start == "" {
		start = defaultBranch
	}
	if o.commit != "" {
		// GitHub lists the history of a commit as it does a branch's,
		// with the commit first, where the commit pane follows the
		// cursor.
		start, m.focus = o.commit, commitPane
	}
	m.graph.show(m, start)
	// Assume a dark terminal until the app sets the theme.
	p, _ := config.Default().Palette(true)
	m.SetTheme(ui.NewTheme(p, true))
	return m
}

// Init reads the branches and the first commits of the branch shown.
func (m *Modal) Init() tea.Cmd {
	return tea.Batch(m.loadBranches("", false), m.graph.model.Init(), m.startSpinner())
}

// Title names the repository.
func (m *Modal) Title() string {
	return "History · " + m.repo.String()
}

// SetSize sizes the panes to the room inside the frame.
func (m *Modal) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.layout()
}

// SetTheme styles the modal and the bubbles in it.
func (m *Modal) SetTheme(t ui.Theme) {
	m.theme = t
	m.st = newStyles(t)
	m.errs = t.Errors(m.opts.icons)
	m.spin.Style = t.Accent
	m.graph.model.SetStyles(t.Graph(m.opts.icons))
	m.commit.pager.SetStyles(t.Pager(m.opts.icons))
	if m.branches.filter != nil {
		m.branches.filter.SetStyles(filterStyles(t, m.opts.icons))
	}
	m.commit.header = nil
}

// KeyLayers implements ui.Keyed. The filter of the branches and a search
// of the patch each take every key while open; otherwise the modal's own
// keys come first, named for what they do in the focused pane, and then
// those of the pane: its list, the graph, or the pager of a patch.
func (m *Modal) KeyLayers() []keyhelp.Layer {
	patch := m.focus == commitPane && m.commit.patch
	switch {
	case m.focus == branchPane && m.branches.filter != nil:
		return []keyhelp.Layer{keyhelp.FromHelp("filter", *m.branches.filter, true)}
	case patch && m.commit.pager.Capturing():
		return []keyhelp.Layer{keyhelp.FromHelp("pager", m.commit.pager, true)}
	}
	k := m.keys.state(m)
	own := keyhelp.Layer{Source: "history", Bindings: k.own(), Short: k.ShortHelp()}
	switch {
	case patch:
		return []keyhelp.Layer{own, keyhelp.FromHelp("pager", m.commit.pager, false)}
	case m.focus == graphPane:
		g := m.graph.model.KeyMap()
		g.Choose = named(g.Choose, "diff")
		return []keyhelp.Layer{own, keyhelp.FromHelp("graph", g, false)}
	}
	// The pane moves with the list's keys, and the modal's own select and
	// retry take the place of its choose and retry.
	list := k.List
	list.Choose.SetEnabled(false)
	list.Retry.SetEnabled(false)
	source := "branches"
	if m.focus == commitPane {
		source = "files"
	}
	return []keyhelp.Layer{own, keyhelp.FromHelp(source, list, false)}
}

// state returns k as the modal takes it now, named for what the keys do
// in the focused pane. Back shows every pane again while one is zoomed,
// and closes the modal from the branches; in a patch the pager takes it.
func (k KeyMap) state(m *Modal) KeyMap {
	patch := m.focus == commitPane && m.commit.patch
	k.ResetBase.SetEnabled(k.ResetBase.Enabled() && m.base.Ref != "")
	k.Zoom.SetEnabled(k.Zoom.Enabled() && !m.narrow())
	switch {
	case m.zoomed():
		k.Back = named(k.Back, "unzoom")
	case patch:
		k.Back.SetEnabled(false)
	case m.focus == branchPane:
		k.Back = named(k.Back, "close")
	}
	k.UseAsBase.SetEnabled(k.UseAsBase.Enabled() && !patch)
	failed := false
	switch m.focus {
	case branchPane:
		k.Select = named(k.Select, "graph")
		failed = m.branches.err != nil
	case graphPane:
		// The graph opens a commit, and retries, with its own keys.
		k.Select.SetEnabled(false)
	case commitPane:
		k.Select = named(k.Select, "patch")
		k.Select.SetEnabled(k.Select.Enabled() && !patch)
		failed = !patch && (m.commit.err != nil || m.commit.filesErr != nil)
	}
	k.Filter.SetEnabled(k.Filter.Enabled() && m.focus == branchPane)
	k.Retry.SetEnabled(k.Retry.Enabled() && failed)
	return k
}

// narrow reports whether the modal shows one pane at a time.
func (m *Modal) narrow() bool {
	return m.width < narrowWidth
}

// zoomed reports whether the zoom shows. A narrow modal shows one pane
// anyway, so there the back key keeps its other uses.
func (m *Modal) zoomed() bool {
	return m.zoom && !m.narrow()
}

// close ends the modal's reads and asks the app to close it.
func (m *Modal) close() tea.Cmd {
	m.cancel()
	return ui.CloseModal(m)
}

// spinning starts the spinner while something loads, unless it runs.
func (m *Modal) startSpinner() tea.Cmd {
	if m.spinning {
		return nil
	}
	m.spinning = true
	return m.spin.Tick
}

// loading reports whether a pane waits for something the spinner shows.
func (m *Modal) loading() bool {
	return m.branches.loading || m.commit.loading || m.commit.filesLoading
}

// now reads the clock.
func (m *Modal) now() time.Time {
	return m.opts.now()
}

// graphName names the history the graph shows: its branch, or the commit
// it was opened on.
func (m *Modal) graphName() string {
	switch shown := m.graph.shown(); shown {
	case "":
		return "Default branch"
	case m.opts.commit:
		return short(shown)
	default:
		return shown
	}
}

// label names a base in the header: the branch, or the commit on the
// branch it was chosen from.
func label(branch, sha string) string {
	if sha == "" {
		return branch
	}
	if branch == "" {
		return short(sha)
	}
	return branch + " @ " + short(sha)
}
