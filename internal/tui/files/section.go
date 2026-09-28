// Package files is the Files section: the tree of the selected repository,
// listed whole in one request so that directories expand at once, whose
// files open in a preview over the screen or in the browser. The tree shows
// the head of the default branch, or the base a ui.BaseMsg sets, such as a
// branch or an older commit chosen in the history.
package files

import (
	"cmp"
	"context"
	"path"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
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
	// host is the web host of the user's GitHub, for the links it opens.
	host string
	// ref is the base the files are shown at, a branch or a commit SHA,
	// or empty for the head of the default branch.
	ref string
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
	// voice words the errors of the tree and the finder.
	voice ui.Voice
	// editor is the editor the preview opens a file in, if set.
	editor string

	// finder finds a file of the listing of src, once opened, and
	// findPreview is whether it shows the content of the selected file
	// when there is room. baseLabel names the base, if one is set.
	finder      *finderModal
	findPreview bool
	baseLabel   string
	// recent holds the paths of the files opened in each repository, the
	// most recent first, so that the finder offers them first.
	recent map[string][]string

	// prefetchMax is the largest top-level file read ahead, or 0.
	prefetchMax int64
	hover       hover
	// seen remembers the files read ahead, so that opening one counts as
	// a use.
	seen *obs.Prefetched[filesvc.BlobQuery]

	width, height int
	focused       bool
	theme         ui.Theme
	styles        tree.Styles
	// icons are the glyphs of files, and fileIcons renders them in the
	// theme.
	icons     ui.Icons
	fileIcons *fileIcons
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
		icons:   ui.NewIcons(config.IconsNerd),
		offline: new(ui.Offline),
		voice:   ui.NewVoice(keys, ""),
		seen:    obs.NewPrefetched[filesvc.BlobQuery]("file"),
		// The finder shows a preview where it fits, unless told not to.
		findPreview: true,
		recent:      map[string][]string{},
	}
	for _, opt := range opts {
		opt(s)
	}
	s.fileIcons = newFileIcons(s.icons, s.theme)
	s.hint = "Search for a repository to browse its files."
	if k := ui.Binding(keys, config.ActionSearch, "search").Help().Key; k != "" {
		s.hint = "Press " + k + " to search for one."
	}
	if s.repo != (core.RepoRef{}) {
		s.newTree(s.repo, "")
	}
	return s
}

// newTree replaces the tree with one of the files of repo at ref, and
// cancels the loads of the old one. The new tree loads nothing until it is
// started.
func (s *Section) newTree(repo core.RepoRef, ref string) {
	if s.cancelTree != nil {
		s.cancelTree()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	src := newSource(s.svc, s.host, repo, ref)
	// The tree retries a failed load with the key that expands.
	v := s.voice
	v.Retry = s.keys.Tree.Expand
	t := tree.New(src.children,
		tree.WithContext(ctx),
		tree.WithExpandAllLimits(expandAllNodes, expandAllDepth),
		tree.WithKeyMap(s.keys.Tree),
		tree.WithStyles(s.styles),
		tree.WithSize(s.width, s.height),
		tree.WithFocused(s.focused),
		tree.WithEmptyText("This repository is empty."),
		tree.WithIcons(s.fileIcons.node),
		tree.WithErrorText(ui.ErrorText("load the files", repo.String(), v)),
	)
	s.repo, s.ref, s.tree, s.src, s.started = repo, ref, &t, src, false
	s.treeCtx, s.cancelTree = ctx, cancel
	s.idx, s.warned = nil, false
	s.hover.reset()
	s.baseLabel = ""
	if s.finder != nil {
		s.finder.close()
		s.finder = nil
	}
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
		// Selecting a repository shows the head of its default branch.
		if s.tree != nil && msg.Repo.Same(s.repo) && s.ref == "" {
			return nil
		}
		s.newTree(msg.Repo, "")
		return s.start()
	case ui.BaseMsg:
		if s.tree == nil || !msg.Repo.Same(s.repo) || msg.Ref == s.ref {
			return nil
		}
		s.newTree(s.repo, msg.Ref)
		s.baseLabel = cmp.Or(msg.Label, shortRef(msg.Ref))
		return s.start()
	case ui.SettingsMsg:
		s.configure(msg.Config)
		return nil
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
	case ui.SyncMsg:
		// A ref of the repository moved, such as after a force-push, and
		// its new listing is cached already.
		if msg.Err != nil || s.tree == nil || !s.started || msg.Key != filesvc.SyncKey(s.repo) {
			return nil
		}
		return s.reload()
	case tree.OpenMsg:
		if s.tree == nil || msg.ID != s.tree.ID() {
			return nil
		}
		return s.preview(msg.Node)
	case ui.OpenFileMsg:
		return s.previewFile(msg)
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
		return ui.Open(webURL(s.host, s.repo, s.ref, s.selected())), true
	case s.ref != "" && key.Matches(msg, s.keys.ResetBase):
		return ui.ResetBase(s.repo), true
	}
	return nil, false
}

// refresh asks GitHub whether the listing changed, and then reloads the
// tree from it, keeping what is expanded. Reloading waits for the listing
// so that every directory comes from the same one.
func (s *Section) refresh() tea.Cmd {
	s.svc.Invalidate(s.repo)
	return s.reload()
}

// reload reads the listing and then reloads the tree from it, keeping what
// is expanded.
func (s *Section) reload() tea.Cmd {
	src, ctx := s.src, s.treeCtx
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "files.listing")
		// A failed read shows in the tree, whose root reads it again.
		_, err := src.load(ctx)
		end(err, "span", "tui", "repo", src.repo.String())
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
	if f := s.finder; f != nil {
		switch {
		case f.sha == "":
			f.sha = x.sha
		case f.sha != x.sha:
			// The listing changed, so the next finder lists it again. One
			// that is open keeps what it shows.
			s.finder = nil
		}
	}
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
	return s.open(e, nil)
}

// open previews the file of e in a modal, which reopens ret when it
// closes, if set.
func (s *Section) open(e core.TreeEntry, ret ui.Modal) tea.Cmd {
	s.opened(e.Path)
	s.seen.Opened(s.blobQuery(e))
	p := newPreview(s.ctx, s.svc, s.host, s.repo, s.ref, e, s.keys.Open, s.voice, s.editor)
	p.ret = ret
	// The app passes messages to a modal only once it is open, so the load
	// starts after the modal opens.
	return tea.Sequence(ui.OpenModal(p), p.load())
}

// maxRecent is how many opened files the section remembers per repository.
const maxRecent = 16

// opened remembers that the file at p was opened, as the most recent.
func (s *Section) opened(p string) {
	k := strings.ToLower(s.repo.String())
	r := slices.DeleteFunc(slices.Clone(s.recent[k]), func(q string) bool { return q == p })
	r = slices.Insert(r, 0, p)
	s.recent[k] = r[:min(len(r), maxRecent)]
}

// recentFiles returns the paths of the files opened in the repository, the
// most recent first.
func (s *Section) recentFiles() []string {
	return slices.Clone(s.recent[strings.ToLower(s.repo.String())])
}

// reveal expands the directories of the file at p in the tree, moves the
// cursor to it, and focuses the section.
func (s *Section) reveal(p string) tea.Cmd {
	if s.tree == nil {
		return nil
	}
	ids := make([]string, 0, strings.Count(p, "/")+1)
	for i := range len(p) {
		if p[i] == '/' {
			ids = append(ids, p[:i])
		}
	}
	ids = append(ids, p)
	show := func() tea.Msg { return ui.ShowMsg{Title: ui.FilesTitle} }
	return tea.Batch(s.start(), s.tree.Reveal(ids...), show)
}

// shortRef shortens a commit SHA for a title, and leaves a branch as it
// is.
func shortRef(ref string) string {
	if len(ref) == 40 || len(ref) == 64 {
		return ref[:7]
	}
	return ref
}

// previewFile previews the file of msg, which may be of any repository, at
// its blob, or else found by its path at the commit of msg.Ref, on the
// first match of msg.Find or on msg.Line. The tree keeps its repository.
func (s *Section) previewFile(msg ui.OpenFileMsg) tea.Cmd {
	if msg.Path == "" || msg.SHA == "" && msg.Ref == "" {
		return nil
	}
	e := core.TreeEntry{Path: msg.Path, Name: path.Base(msg.Path), Type: core.EntryBlob, Mode: "100644", SHA: msg.SHA}
	p := newPreview(s.ctx, s.svc, s.host, msg.Repo, msg.Ref, e, s.keys.Open, s.voice, s.editor)
	p.find, p.line, p.ret = msg.Find, msg.Line, msg.Return
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

// SetTheme styles the tree and its icons.
func (s *Section) SetTheme(t ui.Theme) {
	s.theme = t
	s.styles = t.Tree()
	s.fileIcons = newFileIcons(s.icons, t)
	if s.tree != nil {
		s.tree.SetStyles(s.styles)
		s.tree.SetIcons(s.fileIcons.node)
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

// KeyLayers implements ui.Keyed: the section's own keys, with the one
// that resets the base only while there is one to reset, and then the
// tree's. Before a repository is selected, no key does anything.
func (s *Section) KeyLayers() []keyhelp.Layer {
	k := s.keys
	k.ResetBase.SetEnabled(k.ResetBase.Enabled() && s.ref != "")
	own := keyhelp.Layer{Source: ui.FilesTitle, Bindings: k.own(), Short: k.own()}
	moves := keyhelp.FromHelp("tree", k.Tree, false)
	if s.tree == nil {
		return []keyhelp.Layer{ui.Off(own), ui.Off(moves)}
	}
	return []keyhelp.Layer{own, moves}
}

// Ref returns the base the files are shown at, a branch or a commit SHA,
// or empty for the head of the default branch.
func (s *Section) Ref() string {
	return s.ref
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
