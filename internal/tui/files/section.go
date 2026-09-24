// Package files is the Files section: the tree of the selected repository,
// listed whole in one request so that directories expand at once, whose
// files open in a preview over the screen or in the browser.
package files

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

// Section shows the files of the selected repository. Create one with
// [New].
type Section struct {
	ctx  context.Context
	svc  Service
	keys KeyMap

	repo core.RepoRef
	// tree lists the files of repo from src, and is nil until a repository
	// is selected. treeCtx bounds its loads, and cancelTree cancels them
	// when another repository is selected. started reports whether its
	// first load was sent.
	tree       *tree.Model
	src        *source
	started    bool
	treeCtx    context.Context
	cancelTree context.CancelFunc
	// idx is the listing the section last reacted to, and warned reports
	// whether it said that the listing of repo is truncated.
	idx    *index
	warned bool
	// offline tells the user once that GitHub can't be reached, and may be
	// shared with other sections.
	offline *ui.Offline

	// prefetchMax is the largest top-level file read ahead, or 0.
	prefetchMax int64
	hover       hover

	width, height int
	focused       bool
	theme         ui.Theme
	styles        tree.Styles
	// blank is the rendered state shown before a repository is selected,
	// and hint what it tells the user to do.
	blank string
	hint  string
}

// New returns the section, which reads the files from svc and takes its
// keys from the configured keys. ctx bounds its requests.
func New(ctx context.Context, svc Service, keys map[string][]string, opts ...Option) *Section {
	s := &Section{
		ctx:     ctx,
		svc:     svc,
		keys:    newKeyMap(keys),
		styles:  tree.DefaultStyles(true),
		offline: new(ui.Offline),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.hint = "Search for a repository to browse its files."
	if k := ui.Binding(keys, config.ActionSearch, "search").Help().Key; k != "" {
		s.hint = "Press " + k + " to search for one."
	}
	if s.repo != (core.RepoRef{}) {
		s.newTree(s.repo)
	}
	return s
}

// newTree replaces the tree with one of the files of repo, and cancels the
// loads of the old one. The new tree loads nothing until it is started.
func (s *Section) newTree(repo core.RepoRef) {
	if s.cancelTree != nil {
		s.cancelTree()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	src := newSource(s.svc, repo)
	t := tree.New(src.children,
		tree.WithContext(ctx),
		tree.WithExpandAllLimits(expandAllNodes, expandAllDepth),
		tree.WithKeyMap(s.keys.Tree),
		tree.WithStyles(s.styles),
		tree.WithSize(s.width, s.height),
		tree.WithFocused(s.focused),
		tree.WithEmptyText("This repository is empty."),
	)
	s.repo, s.tree, s.src, s.started = repo, &t, src, false
	s.treeCtx, s.cancelTree = ctx, cancel
	s.idx, s.warned = nil, false
	s.hover.reset()
}

// The limits of an expand-all while the tree comes from the listing, where
// it costs no requests. They keep the view usable in a huge repository.
const (
	expandAllNodes = 5000
	expandAllDepth = 64
)

// listingMsg reports that the listing of src was read again for a
// refresh.
type listingMsg struct {
	src *source
}

// start sends the first load of the tree, once.
func (s *Section) start() tea.Cmd {
	if s.tree == nil || s.started {
		return nil
	}
	s.started = true
	return s.tree.Init()
}

// Title returns the title of the section.
func (s *Section) Title() string {
	return ui.FilesTitle
}

// Init loads the top-level files, if a repository is selected.
func (s *Section) Init() tea.Cmd {
	return s.start()
}

// Update follows the selected repository and handles the section's keys;
// everything else goes to the tree. It then reacts to a listing the tree
// has read, and to the cursor moving.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	cmd := s.update(msg)
	if s.tree == nil {
		return cmd
	}
	return tea.Batch(cmd, s.observe(), s.moved())
}

func (s *Section) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ui.RepoMsg:
		if s.tree != nil && sameRef(msg.Repo, s.repo) {
			return nil
		}
		s.newTree(msg.Repo)
		return s.start()
	case hoverMsg:
		if s.tree == nil {
			return nil
		}
		return s.rested(msg)
	case listingMsg:
		if s.tree == nil || msg.src != s.src {
			return nil
		}
		return s.tree.Reload()
	case tree.OpenMsg:
		if s.tree == nil || msg.ID != s.tree.ID() {
			return nil
		}
		return s.preview(msg.Node)
	case tea.KeyPressMsg:
		if cmd, ok := s.press(msg); ok {
			return cmd
		}
	}
	if s.tree == nil {
		return nil
	}
	var cmd tea.Cmd
	*s.tree, cmd = s.tree.Update(msg)
	return cmd
}

// press handles the section's own keys and reports whether msg was one.
func (s *Section) press(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if s.tree == nil || !s.focused {
		return nil, false
	}
	switch {
	case key.Matches(msg, s.keys.Refresh):
		return s.refresh(), true
	case key.Matches(msg, s.keys.Open):
		return ui.Open(webURL(s.repo, s.selected())), true
	}
	return nil, false
}

// refresh asks GitHub whether the listing changed, and then reloads the
// tree from it, keeping what is expanded. Reloading waits for the listing
// so that every directory comes from the same one.
func (s *Section) refresh() tea.Cmd {
	s.svc.Invalidate(s.repo)
	src, ctx := s.src, s.treeCtx
	return func() tea.Msg {
		// A failed read shows in the tree, whose root reads it again.
		_, _ = src.load(ctx)
		return listingMsg{src: src}
	}
}

// observe reacts to a listing the tree read since the last call: it reads
// the small top-level files ahead. A truncated listing is read one
// directory at a time, with a request each, so an expand-all goes back to
// the tree's cautious limits.
func (s *Section) observe() tea.Cmd {
	x := s.src.current()
	if x == nil || x == s.idx {
		return nil
	}
	s.idx = x
	prefetch := s.prefetchTop(s.treeCtx, x)
	if x.offline {
		s.offline.Mark()
		prefetch = tea.Batch(prefetch, s.offline.Notify())
	}
	if !x.truncated {
		s.tree.SetExpandAllLimits(expandAllNodes, expandAllDepth)
		return prefetch
	}
	s.tree.SetExpandAllLimits(tree.DefaultExpandAllNodes, tree.DefaultExpandAllDepth)
	if s.warned {
		return prefetch
	}
	s.warned = true
	return tea.Batch(prefetch,
		ui.Notify(toast.Info, s.repo.String()+" is too large to list at once, so folders load as you open them."))
}

// preview opens the file of n in a modal. A submodule has no content in
// this repository, so it is only named.
func (s *Section) preview(n tree.Node) tea.Cmd {
	e, ok := entryOf(n)
	if !ok {
		return nil
	}
	if e.Submodule() {
		text := e.Path + " is a submodule, with its files in another repository."
		if k := s.keys.Open.Help().Key; k != "" {
			text += " Press " + k + " to open it in the browser."
		}
		return ui.Notify(toast.Info, text)
	}
	p := newPreview(s.ctx, s.svc, s.repo, e, s.keys.Open)
	// The app passes messages to a modal only once it is open, so the load
	// starts after the modal opens.
	return tea.Sequence(ui.OpenModal(p), p.load())
}

// selected returns the entry under the cursor, or the zero entry, which
// stands for the root.
func (s *Section) selected() core.TreeEntry {
	n, _ := s.tree.Selected()
	e, _ := entryOf(n)
	return e
}

// View renders the tree, or what to do before a repository is selected.
func (s *Section) View() string {
	if s.tree == nil {
		return s.blank
	}
	return s.tree.View()
}

// SetSize sets the size of the tree.
func (s *Section) SetSize(width, height int) {
	s.width, s.height = max(width, 0), max(height, 0)
	if s.tree != nil {
		s.tree.SetSize(s.width, s.height)
	}
	s.renderBlank()
}

// SetTheme styles the tree.
func (s *Section) SetTheme(t ui.Theme) {
	s.theme = t
	s.styles = t.Tree()
	if s.tree != nil {
		s.tree.SetStyles(s.styles)
	}
	s.renderBlank()
}

// Focus makes the section react to keys.
func (s *Section) Focus() {
	s.focused = true
	if s.tree != nil {
		s.tree.Focus()
	}
}

// Blur makes the section ignore keys.
func (s *Section) Blur() {
	s.focused = false
	if s.tree != nil {
		s.tree.Blur()
	}
}

// Help returns the keys of the section and its tree.
func (s *Section) Help() help.KeyMap {
	return s.keys
}

// renderBlank renders the state shown before a repository is selected,
// wrapped to fit a narrow pane.
func (s *Section) renderBlank() {
	if s.width <= 0 || s.height <= 0 {
		s.blank = ""
		return
	}
	center := lipgloss.NewStyle().Width(s.width).Align(lipgloss.Center)
	text := lipgloss.JoinVertical(lipgloss.Left,
		center.Inherit(s.theme.Title).Render("No repository selected"),
		"",
		center.Inherit(s.theme.Muted).Render(s.hint),
	)
	lines := strings.Split(lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, text), "\n")
	lines = lines[:min(len(lines), s.height)]
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, s.width, "")
	}
	s.blank = strings.Join(lines, "\n")
}

// sameRef reports whether a and b name the same repository, which GitHub
// matches regardless of case.
func sameRef(a, b core.RepoRef) bool {
	return strings.EqualFold(a.Owner, b.Owner) && strings.EqualFold(a.Name, b.Name)
}
