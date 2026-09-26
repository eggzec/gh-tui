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
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// narrowWidth is the width inside the frame below which the panes don't fit
// side by side, so the modal shows one at a time.
const narrowWidth = 90

// pageSize is how many commits the graph reads at a time. It is fixed, so
// that the pages cached survive a resize.
const pageSize = 50

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
		commit:        newCommit(),
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
	return tea.Batch(m.loadBranches(""), m.graph.model.Init(), m.startSpinner())
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
	m.spin.Style = t.Accent
	m.graph.model.SetStyles(t.Graph())
	m.commit.pager.SetStyles(t.Pager())
	if m.branches.filter != nil {
		m.branches.filter.SetStyles(filterStyles(t))
	}
	m.commit.header = nil
}

// Help lists the keys of the focused pane.
func (m *Modal) Help() help.KeyMap {
	return helpKeys{m: m}
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

// trim cuts s at the first line.
func trim(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}
