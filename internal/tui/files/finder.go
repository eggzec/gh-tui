package files

import (
	"context"
	"path"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/finder"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
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
	// icons renders the icons of the files in the theme of the finder,
	// which may be themed before the section is.
	icons *fileIcons
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
	// ahead reads the files around the cursor, as the finder's prefetch
	// settings say.
	ahead *ui.Ahead[filesvc.BlobQuery]
	// failed is why the file shown failed to load, or nil.
	failed error
	// img shows the file in the pager as the file preview does: as its
	// image where it is an image file the terminal shows. named is set
	// while the pager names the file binary by its name alone, unread.
	img   fileImage
	named bool
	// md renders the markdown files shown, unless the section shows them
	// as their source.
	md markdownView

	width, height int
	theme         ui.Theme
	sep           string
}

// previewWidth is the width inside the frame below which the preview is
// hidden unless the user shows it.
const previewWidth = 100

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

// ShortHelp implements help.KeyMap.
func (k finderKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Reveal, k.Preview, k.Browser}
}

// FullHelp implements help.KeyMap: the keys of the finder beside the
// bubble's.
func (k finderKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

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
		// It got no messages while closed.
		f.md.reset()
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
		title: "Find file" + s.icons.Separator + s.repo.String(),
		ctx:   ctx,
		stop:  stop,
		keys:  newFinderKeys(),
		theme: s.theme,
	}
	if s.idx != nil {
		f.sha = s.idx.sha
	}
	if s.baseLabel != "" {
		f.title += s.icons.Separator + s.baseLabel
	}
	f.ahead = s.fileAhead(s.prefetch.finder)
	f.ahead.Reset(ctx)
	src := s.src
	host, repo, ref := s.host, s.repo, s.ref
	// The finder lists the files again only when it opens again, and o
	// types into its query.
	v := s.voice
	v.Retry, v.Open = key.Binding{}, key.Binding{}
	// The preview loads a file again only when it is chosen again, and
	// opens it on GitHub with the browser key.
	pv := s.voice
	pv.Retry, pv.Open = key.Binding{}, f.keys.Browser
	f.pager = pager.New(pager.WithErrorText(fileErrorText(repo, pv)), pager.WithResizeRest(resizeRest))
	f.img = fileImage{images: s.images, repo: repo, shown: shownText, ellipsis: s.icons.Ellipsis}
	f.md.setFiles(ctx, s.svc, repo, ref, s.images)
	f.pager.SetReserve(f.md.extra)
	f.icons = newFileIcons(s.icons, s.theme)
	f.find = finder.New(func(ctx context.Context) (finder.Listing, error) { return listFiles(ctx, src) },
		finder.WithContext(ctx),
		finder.WithKeyMap(f.keys.find),
		finder.WithStyles(s.theme.Finder(s.icons)),
		finder.WithIcons(f.icons.item),
		finder.WithRecent(s.recentFiles()),
		// Each file links to its page at the base.
		finder.WithLinks(func(it finder.Item) string {
			if e, ok := entryOfItem(it); ok {
				return webURL(host, repo, ref, e)
			}
			return ""
		}),
		finder.WithErrorText(ui.ErrorText("list the files", s.repo.String(), v)),
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
	case ui.AheadMsg:
		return f.ahead.Rested(msg)
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
		f.failed = msg.err
		if msg.err == nil {
			// Counts as a use only if the window read the file ahead.
			f.ahead.Opened(f.s.blobQuery(msg.entry))
		}
		return f.showFile(msg.entry, msg.blob, msg.err)
	case ui.ImagesMsg:
		if it, _ := f.find.Selected(); f.named {
			// An image file named binary while the terminal showed no
			// images is read now that it may.
			if e, ok := entryOfItem(it); ok && f.drawsImage(e) {
				f.current = false
				return f.moved()
			}
		}
		// The pictures of markdown drawn again, as they arrived, or
		// images began or stopped being drawn.
		if f.md.stale() {
			f.pager.Rerender()
		}
		return tea.Batch(f.redraw(), f.md.lookUp())
	case imageEntryMsg:
		if !f.md.take(msg) {
			return nil
		}
		f.pager.Rerender()
		return f.md.lookUp()
	case ui.OnlineMsg:
		// A rate limit is the token's, and has lifted unless one holds.
		if !msg.Limited {
			f.ahead.Resume()
		}
		return f.online()
	case ui.ReopenedMsg:
		if msg.Modal != ui.Modal(f) {
			return nil
		}
		// It got no messages while hidden.
		f.md.reset()
		f.current = false
		return tea.Batch(f.moved(), f.pager.Settle())
	case tea.KeyPressMsg:
		if cmd, ok := f.press(msg); ok {
			return cmd
		}
	}
	var cmd, pcmd tea.Cmd
	f.find, cmd = f.find.Update(msg)
	f.pager, pcmd = f.pager.Update(msg)
	return tea.Batch(cmd, pcmd, f.moved(), f.readAhead())
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
		return ui.Open(webURL(f.s.host, f.s.repo, f.s.ref, e)), true
	case key.Matches(msg, f.keys.Preview):
		show := !f.preview
		f.toggled = &show
		f.layout()
		f.current = false
		if !f.preview {
			f.stopRead()
		}
		return tea.Batch(f.moved(), f.pager.Settle()), true
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
	f.shown, f.current, f.failed = it.Path, true, nil
	f.seq++
	f.stopRead()
	f.img.clear()
	f.named = false
	e, ok := entryOfItem(it)
	if !ok {
		f.pager.SetMessage("", "")
		return nil
	}
	if binaryExt[strings.ToLower(path.Ext(e.Name))] && !f.drawsImage(e) {
		f.named = true
		f.pager.SetMessage(e.Path, "Binary file, not shown"+browserHint(f.keys.Browser, f.s.icons))
		return nil
	}
	if b, ok := f.s.svc.CachedBlob(f.s.blobQuery(e)); ok {
		// Counts as a use only if the window read the file ahead.
		f.ahead.Opened(f.s.blobQuery(e))
		return f.showFile(e, b, nil)
	}
	// The name shows at once, and the content once the cursor rests.
	f.pager.SetMessage(e.Path, "")
	msg := finderRestMsg{f: f, seq: f.seq}
	return tea.Tick(f.s.prefetch.finder.Rest, func(time.Time) tea.Msg { return msg })
}

// drawsImage reports whether the preview draws e as an image, so it reads
// e although its name says it is binary.
func (f *finderModal) drawsImage(e core.TreeEntry) bool {
	return !e.Symlink() && imageFile(e.Path) && f.s.images.Drawing()
}

// showFile shows e, whose content is b, or which failed to load with err,
// in the preview, as the file preview shows it.
func (f *finderModal) showFile(e core.TreeEntry, b core.Blob, err error) tea.Cmd {
	f.img.set(e, b, err)
	if !f.img.draw(&f.pager) {
		return nil
	}
	cmd, _ := fill(&f.pager, e, b, err, f.keys.Browser, f.s.icons, f.rendering())
	return tea.Batch(cmd, f.md.lookUp())
}

// rendering returns what renders the markdown files shown, or nil while
// the section shows them as their source.
func (f *finderModal) rendering() *markdownView {
	if f.s.rawMarkdown {
		return nil
	}
	return &f.md
}

// redraw draws the image of the file shown again, as when the images or
// the size of the preview changed.
func (f *finderModal) redraw() tea.Cmd {
	if !f.img.redraw(&f.pager) {
		return nil
	}
	cmd, _ := fill(&f.pager, f.img.entry, f.img.blob, f.img.err, f.keys.Browser, f.s.icons, f.rendering())
	return cmd
}

// readAhead reads, once the cursor rests, the files in a window around it
// while the preview shows, as the finder's prefetch settings say. The
// preview reads the file under the cursor itself, whatever they say.
func (f *finderModal) readAhead() tea.Cmd {
	f.ahead.Configure(f.s.prefetch.finder)
	i := f.find.Index()
	at := func(j int) (filesvc.BlobQuery, bool) {
		it, ok := f.find.At(j)
		e, isEntry := entryOfItem(it)
		if j == i || !ok || !isEntry || !worthReading(e, f.s.prefetch.finderMax) {
			return filesvc.BlobQuery{}, false
		}
		return f.s.blobQuery(e), true
	}
	// Until the paths are listed, or while the preview is hidden, there
	// is no window.
	if !f.preview || f.find.Loading() {
		at = nil
	}
	return f.ahead.Window(at, i)
}

// online loads again, now that GitHub answers again, what failed for want
// of an answer from it: the paths, the file the preview shows, and the
// image files of the markdown it shows.
func (f *finderModal) online() tea.Cmd {
	var read tea.Cmd
	if f.preview && f.cancel == nil && ui.Unreached(f.failed) {
		f.failed = nil
		read = f.read()
	}
	// The image files whose look-ups failed are looked up again.
	if f.md.online() {
		f.pager.Rerender()
	}
	return tea.Batch(ui.RetryUnreached(&f.find), read, f.md.lookUp())
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
	svc, q, seq := f.s.svc, f.s.blobQuery(e), f.seq
	fetch := func() tea.Msg {
		defer cancel()
		// The file under the cursor is no guess, so it isn't counted as read
		// ahead.
		b, err := svc.Blob(ctx, q)
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
	right := strings.Split(f.img.view(&f.pager), "\n")
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

// Settle implements ui.Settler: the preview renders markdown again at its
// new width once the resize rests.
func (f *finderModal) Settle() tea.Cmd { return f.pager.Settle() }

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
	f.md.setHeight(f.height - 1)
	f.pager.SetSize(f.width-lw-3, f.height)
	// The image fitted anew, or the loading line; one that no longer
	// fits shows as without images, and only the highlighting of a text
	// is dropped, as the file preview's resize drops it.
	_ = f.redraw()
}

// SetTheme styles the finder and the preview.
func (f *finderModal) SetTheme(t ui.Theme) {
	f.theme = t
	// The icons take the theme first, so that the finder draws once, in
	// the new styles and with the new icons.
	f.icons.setTheme(t)
	f.find.SetStyles(t.Finder(f.s.icons))
	f.pager.SetStyles(t.Pager(f.s.icons))
	f.md.setTheme(t, f.s.icons)
	f.pager.Rerender()
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Border))
	f.sep = " " + border.Render(f.s.icons.Border.Left) + " "
}

// KeyLayers implements ui.Keyed: the finder's own keys, with the bubble's,
// whose query types the rest.
func (f *finderModal) KeyLayers() []keyhelp.Layer {
	k := f.keys
	if f.preview {
		k.Preview.SetHelp(k.Preview.Help().Key, "hide preview")
	}
	return []keyhelp.Layer{ui.MergeLayers("finder", keyhelp.FromHelp("", k, false), keyhelp.FromHelp("", f.find, true))}
}
