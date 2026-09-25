package files

import (
	"context"
	"path"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/finder"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// finderModal finds a file of the tree by typing some letters of its path,
// in a modal over the screen: the paths that match on the left, best first,
// and the content of the selected file on the right. It lists the files
// of the listing the tree shows, at its base, and is kept for that listing,
// so that opening it again costs nothing.
type finderModal struct {
	s *Section
	// src is the source it finds in, and sha names the listing it lists,
	// once known. title names them.
	src   *source
	sha   string
	title string
	ctx   context.Context
	stop  context.CancelFunc
	keys  finderKeys

	find finder.Model
	// pager shows the content of the selected file, while preview says
	// so. Below previewWidth columns the preview hides by default, and
	// toggled holds the user's choice once the toggle is pressed.
	pager   pager.Model
	preview bool
	toggled *bool
	// shown is the path whose content the pager shows or waits for,
	// while current holds, and seq counts the times it changed, so that
	// only the latest read lands. cancel stops the read in flight.
	shown   string
	current bool
	seq     int
	cancel  context.CancelFunc
	delay   time.Duration

	width, height int
	theme         ui.Theme
	sep           string
}

// previewWidth is the width inside the frame below which the preview is
// hidden unless the user shows it.
const previewWidth = 100

// defaultFinderDelay is how long the cursor rests on a file before its
// content is read, when the hover prefetch doesn't set it.
const defaultFinderDelay = 100 * time.Millisecond

// finderKeys are the keys of the finder besides those of the bubble. None
// is a letter, since letters go to the query.
type finderKeys struct {
	find finder.KeyMap
	// Reveal shows the file in the tree, Browser on GitHub, and Preview
	// shows or hides its content.
	Reveal  key.Binding
	Browser key.Binding
	Preview key.Binding
}

func newFinderKeys() finderKeys {
	return finderKeys{
		find:    finder.DefaultKeyMap(),
		Reveal:  key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("^t", "open in tree")),
		Browser: key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("^o", "open on GitHub")),
		Preview: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "preview")),
	}
}

// ShortHelp returns the bindings for the short help view.
func (k finderKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.find.Up, k.find.Down, k.find.Choose, k.Reveal, k.Preview, k.Browser, k.find.Cancel}
}

// FullHelp returns the bindings for the full help view.
func (k finderKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.find.Up, k.find.Down, k.find.PageUp, k.find.PageDown},
		{k.find.Choose, k.Reveal, k.Preview, k.Browser, k.find.Cancel},
	}
}

// FindFile opens the finder over the files the tree shows, and loads their
// listing if it isn't cached, with one request. It implements
// ui.FileFinder.
func (s *Section) FindFile() (ui.Modal, tea.Cmd) {
	if s.tree == nil {
		return nil, nil
	}
	// The tree loads too, so that a file found can be shown in it.
	start := s.start()
	if f := s.finder; f != nil && f.src == s.src {
		f.current = false
		reset := f.find.Reset(s.recentFiles())
		return f, tea.Batch(start, reset, f.moved())
	}
	if s.finder != nil {
		s.finder.close()
	}
	f := s.newFinder()
	s.finder = f
	return f, tea.Batch(start, f.find.Init())
}

func (s *Section) newFinder() *finderModal {
	ctx, stop := context.WithCancel(s.treeCtx)
	f := &finderModal{
		s:     s,
		src:   s.src,
		title: "Find file · " + s.repo.String(),
		ctx:   ctx,
		stop:  stop,
		keys:  newFinderKeys(),
		pager: pager.New(),
		delay: s.hover.delay,
		theme: s.theme,
	}
	if s.idx != nil {
		f.sha = s.idx.sha
	}
	if s.baseLabel != "" {
		f.title += " · " + s.baseLabel
	}
	if f.delay <= 0 {
		f.delay = defaultFinderDelay
	}
	src := s.src
	f.find = finder.New(func(ctx context.Context) (finder.Listing, error) { return listFiles(ctx, src) },
		finder.WithContext(ctx),
		finder.WithKeyMap(f.keys.find),
		finder.WithStyles(s.theme.Finder()),
		finder.WithRecent(s.recentFiles()),
	)
	f.find.Focus()
	f.SetTheme(s.theme)
	return f
}

// listFiles lists the files of src for the finder, from the cached listing
// or else with one request. A truncated listing still lists what it holds.
func listFiles(ctx context.Context, src *source) (_ finder.Listing, err error) {
	ctx, end := obs.Begin(ctx, "files.find")
	defer func() { end(err, "span", "tui", "repo", src.repo.String(), "ref", src.ref) }()
	q := filesvc.TreeQuery{Repo: src.repo, Ref: src.ref}
	t, ok := src.svc.CachedAll(q)
	if !ok {
		if t, err = src.svc.All(ctx, q); err != nil {
			return finder.Listing{}, err
		}
	}
	n := 0
	for _, e := range t.Entries {
		if e.Type == core.EntryBlob {
			n++
		}
	}
	// The items point into entries, which boxes no copies.
	entries := make([]core.TreeEntry, 0, n)
	items := make([]finder.Item, 0, n)
	for _, e := range t.Entries {
		if e.Type != core.EntryBlob {
			continue
		}
		entries = append(entries, e)
		it := finder.Item{Path: e.Path, Value: &entries[len(entries)-1]}
		if !e.Symlink() {
			it.Detail = ui.Size(e.Size)
		}
		items = append(items, it)
	}
	l := finder.Listing{Items: items}
	if t.Truncated {
		l.Note = "listing incomplete"
	}
	return l, nil
}

// entryOfItem returns the entry an item of the finder stands for.
func entryOfItem(it finder.Item) (core.TreeEntry, bool) {
	e, ok := it.Value.(*core.TreeEntry)
	if !ok || e == nil {
		return core.TreeEntry{}, false
	}
	return *e, true
}

// close stops the finder's reads, for a finder replaced by another.
func (f *finderModal) close() {
	f.find.Close()
	f.stopRead()
	f.stop()
}

func (f *finderModal) stopRead() {
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
}

// Title names the repository and the base the files are listed at.
func (f *finderModal) Title() string {
	return f.title
}

// finderRestMsg reports that the cursor of the finder rested on a path for
// the delay.
type finderRestMsg struct {
	f   *finderModal
	seq int
}

// finderBlobMsg carries the content of the file the cursor rested on.
type finderBlobMsg struct {
	f     *finderModal
	seq   int
	entry core.TreeEntry
	blob  core.Blob
	err   error
}

// Update handles the finder's keys and its reads, and passes the rest to
// the bubbles. A path picked opens in the file preview, which replaces the
// finder and reopens it when it closes.
func (f *finderModal) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case finder.ChosenMsg:
		if msg.ID != f.find.ID() {
			return nil
		}
		e, ok := entryOfItem(msg.Item)
		if !ok {
			return nil
		}
		return f.s.open(e, f)
	case finder.CancelMsg:
		if msg.ID != f.find.ID() {
			return nil
		}
		f.stopRead()
		return ui.CloseModal(f)
	case finderRestMsg:
		if msg.f != f || msg.seq != f.seq {
			return nil
		}
		return f.read()
	case finderBlobMsg:
		if msg.f != f || msg.seq != f.seq {
			return nil
		}
		f.cancel = nil
		cmd, _ := fill(&f.pager, msg.entry, msg.blob, msg.err, f.keys.Browser)
		return cmd
	case ui.ReopenedMsg:
		if msg.Modal != ui.Modal(f) {
			return nil
		}
		f.current = false
		return f.moved()
	case tea.KeyPressMsg:
		if cmd, ok := f.press(msg); ok {
			return cmd
		}
	}
	var cmd, pcmd tea.Cmd
	f.find, cmd = f.find.Update(msg)
	f.pager, pcmd = f.pager.Update(msg)
	return tea.Batch(cmd, pcmd, f.moved())
}

// press handles the finder's own keys and reports whether msg was one.
func (f *finderModal) press(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, f.keys.Reveal):
		it, ok := f.find.Selected()
		if !ok {
			return nil, true
		}
		f.stopRead()
		return tea.Batch(ui.CloseModal(f), f.s.reveal(it.Path)), true
	case key.Matches(msg, f.keys.Browser):
		it, ok := f.find.Selected()
		if !ok {
			return nil, true
		}
		e, _ := entryOfItem(it)
		return ui.Open(webURL(f.s.repo, f.s.ref, e)), true
	case key.Matches(msg, f.keys.Preview):
		show := !f.preview
		f.toggled = &show
		f.layout()
		f.current = false
		if !f.preview {
			f.stopRead()
		}
		return f.moved(), true
	}
	return nil, false
}

// moved starts reading the file under the cursor, once the cursor rests
// on it, if the preview shows. A cached file shows at once.
func (f *finderModal) moved() tea.Cmd {
	if !f.preview {
		return nil
	}
	it, _ := f.find.Selected()
	if f.current && it.Path == f.shown {
		return nil
	}
	f.shown, f.current = it.Path, true
	f.seq++
	f.stopRead()
	e, ok := entryOfItem(it)
	if !ok {
		f.pager.SetMessage("", "")
		return nil
	}
	if binaryExt[strings.ToLower(path.Ext(e.Name))] {
		f.pager.SetMessage(e.Path, "Binary file, not shown"+browserHint(f.keys.Browser))
		return nil
	}
	if b, ok := f.s.svc.CachedBlob(f.s.blobQuery(e)); ok {
		cmd, _ := fill(&f.pager, e, b, nil, f.keys.Browser)
		return cmd
	}
	// The name shows at once, and the content once the cursor rests.
	f.pager.SetMessage(e.Path, "")
	msg := finderRestMsg{f: f, seq: f.seq}
	return tea.Tick(f.delay, func(time.Time) tea.Msg { return msg })
}

// read reads the file the cursor rested on.
func (f *finderModal) read() tea.Cmd {
	it, _ := f.find.Selected()
	e, ok := entryOfItem(it)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithCancel(obs.WithTrace(f.ctx, "prefetch.finder"))
	f.cancel = cancel
	svc, q, seen, seq := f.s.svc, f.s.blobQuery(e), f.s.seen, f.seq
	fetch := func() tea.Msg {
		defer cancel()
		b, err := readBlob(ctx, svc, seen, q)
		if ctx.Err() != nil {
			return nil
		}
		return finderBlobMsg{f: f, seq: seq, entry: e, blob: b, err: err}
	}
	return tea.Batch(f.pager.SetLoading(e.Path), fetch)
}

// View renders the finder, and the preview beside it when it shows.
func (f *finderModal) View() string {
	if !f.preview {
		return f.find.View()
	}
	left := strings.Split(f.find.View(), "\n")
	right := strings.Split(f.pager.View(), "\n")
	var b strings.Builder
	for i := range min(len(left), len(right)) {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(left[i])
		b.WriteString(f.sep)
		b.WriteString(right[i])
	}
	return b.String()
}

// SetSize shares the size between the finder and the preview.
func (f *finderModal) SetSize(width, height int) {
	f.width, f.height = max(width, 0), max(height, 0)
	f.layout()
}

// The finder takes two fifths of the width beside the preview, within
// these bounds.
const (
	minFinderWidth = 32
	maxFinderWidth = 64
)

func (f *finderModal) layout() {
	f.preview = f.s.findPreview && f.width >= previewWidth
	if f.toggled != nil {
		f.preview = *f.toggled
	}
	lw := min(max(f.width*2/5, minFinderWidth), maxFinderWidth)
	if !f.preview || lw+3 >= f.width {
		f.preview = f.preview && lw+3 < f.width
		f.find.SetSize(f.width, f.height)
		return
	}
	f.find.SetSize(lw, f.height)
	f.pager.SetSize(f.width-lw-3, f.height)
}

// SetTheme styles the finder and the preview.
func (f *finderModal) SetTheme(t ui.Theme) {
	f.theme = t
	f.find.SetStyles(t.Finder())
	f.pager.SetStyles(t.Pager())
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Border))
	f.sep = " " + border.Render("│") + " "
}

// Help returns the keys of the finder.
func (f *finderModal) Help() help.KeyMap {
	k := f.keys
	k.Preview.SetHelp(k.Preview.Help().Key, "preview")
	if f.preview {
		k.Preview.SetHelp(k.Preview.Help().Key, "hide preview")
	}
	return k
}
