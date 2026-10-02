package files

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// preview shows a file in a pager, in a modal over the screen. It is opened
// for one file and closed when the pager asks.
type preview struct {
	ctx    context.Context
	cancel context.CancelFunc
	svc    Service
	host   string
	repo   core.RepoRef
	// ref is the base the file was listed at, for its page on GitHub.
	ref   string
	entry core.TreeEntry
	open  key.Binding
	pager pager.Model
	// icons mark a failed load.
	icons ui.Icons
	// find is searched for once the content is shown, and preset holds
	// while that search is the one shown, until the user starts another.
	// line is the line the preview opens on, if set.
	find   string
	preset bool
	line   int
	// ret is the modal the preview reopens when it closes, if set.
	ret ui.Modal
	// failed is why the file failed to load, or nil.
	failed error
	// images draws the file where it is an image the terminal shows.
	// loaded is set once the content arrived, which blob holds; asImage
	// is set while the file is one to draw as an image, whether it is
	// drawn, on its way or failed,
	// shown is what the preview shows, and pic the lines of the image.
	images  *ui.Images
	loaded  bool
	blob    core.Blob
	asImage bool
	shown   shown
	pic     []string
}

// shown is what the preview shows of its file.
type shown int

const (
	shownNothing shown = iota
	// shownText is the pager's own view of the file: its text, or why it
	// isn't shown.
	shownText
	// shownLoading says the image is on its way.
	shownLoading
	// shownImage is the image.
	shownImage
)

// imageExts are the extensions of the files the preview tries to draw as
// images. Whether a file is one comes from its content; the name only
// says which files to try, so a file of another name is never decoded.
var imageExts = []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}

// imageFile reports whether the file at name is one to draw as an image.
func imageFile(name string) bool {
	return slices.Contains(imageExts, strings.ToLower(path.Ext(name)))
}

// entryMsg carries the entry of the file of the preview whose pager has
// the ID, found by its path.
type entryMsg struct {
	id    int64
	entry core.TreeEntry
	err   error
}

// blobMsg carries the content of the file of the preview whose pager has
// the ID, so that the results of a closed preview are ignored.
type blobMsg struct {
	id   int64
	blob core.Blob
	err  error
}

// newPreview returns a preview of e, a file of repo at ref, whose page
// is on host, which words what went wrong with v and opens the file in
// editor, if set, and marks a failed load with the error glyph of ic. It
// draws an image file with images, where the terminal shows them. Its load
// runs under ctx until it closes.
func newPreview(ctx context.Context, svc Service, host string, repo core.RepoRef, ref string, e core.TreeEntry, open key.Binding, v ui.Voice, editor string, ic ui.Icons, images *ui.Images) *preview {
	ctx, cancel := context.WithCancel(ctx)
	// The preview loads the file once, and opens it on GitHub with open.
	v.Retry, v.Open = key.Binding{}, open
	pg := pager.New(pager.WithErrorText(fileErrorText(repo, v)), pager.WithEditor(editor))
	p := &preview{ctx: ctx, cancel: cancel, svc: svc, host: host, repo: repo, ref: ref, entry: e, open: open, pager: pg, icons: ic, images: images}
	p.pager.Focus()
	return p
}

// load shows the content at once when it is cached, and fetches it
// otherwise. A file known by its path alone is found first.
func (p *preview) load() tea.Cmd {
	if p.entry.SHA == "" {
		return p.findFile()
	}
	q := filesvc.BlobQuery{Repo: p.repo, SHA: p.entry.SHA, Size: p.entry.Size}
	if b, ok := p.svc.CachedBlob(q); ok {
		return p.show(b, nil)
	}
	svc, ctx, id := p.svc, p.ctx, p.pager.ID()
	fetch := func() tea.Msg {
		ctx, end := obs.Begin(ctx, "open.file")
		b, err := svc.Blob(ctx, q)
		end(err, "span", "tui", "repo", q.Repo.String(), "size", q.Size)
		return blobMsg{id: id, blob: b, err: err}
	}
	return tea.Batch(p.pager.SetLoading(p.entry.Path), fetch)
}

// findFile finds the entry of the file by its path, from the root of the
// commit of ref down, one directory at a time. The trees of a commit never
// change, so the service keeps them, and a second look costs nothing.
func (p *preview) findFile() tea.Cmd {
	svc, ctx, id, repo, ref, name := p.svc, p.ctx, p.pager.ID(), p.repo, p.ref, p.entry.Path
	fetch := func() tea.Msg {
		ctx, end := obs.Begin(ctx, "open.file.find")
		e, err := findEntry(ctx, svc, repo, ref, name)
		end(err, "span", "tui", "repo", repo.String(), "depth", strings.Count(name, "/")+1)
		return entryMsg{id: id, entry: e, err: err}
	}
	return tea.Batch(p.pager.SetLoading(name), fetch)
}

// errNoFile reports that a path names no file at a commit.
var errNoFile = errors.New("no such file at this commit")

// fileErrorText returns how a pager says that a file of repo failed to
// load, with v.
func fileErrorText(repo core.RepoRef, v ui.Voice) func(error) (text, hint string) {
	say := ui.ErrorText("load the file", repo.String(), v)
	return func(err error) (text, hint string) {
		if errors.Is(err, errNoFile) {
			return "No such file at this commit.", ""
		}
		return say(err)
	}
}

// findEntry returns the entry of the file at path name of the commit of
// ref, with its path from the root.
func findEntry(ctx context.Context, svc Service, repo core.RepoRef, ref, name string) (core.TreeEntry, error) {
	parts := strings.Split(strings.Trim(name, "/"), "/")
	at := ref
	for i, part := range parts {
		t, err := svc.Tree(ctx, filesvc.TreeQuery{Repo: repo, Ref: at})
		if err != nil {
			return core.TreeEntry{}, fmt.Errorf("find %s: %w", name, err)
		}
		j := slices.IndexFunc(t.Entries, func(e core.TreeEntry) bool { return e.Name == part })
		if j < 0 {
			return core.TreeEntry{}, fmt.Errorf("find %s: %w", name, errNoFile)
		}
		e := t.Entries[j]
		if i == len(parts)-1 {
			if e.Dir() || e.Submodule() {
				return core.TreeEntry{}, fmt.Errorf("find %s: %w", name, errNoFile)
			}
			e.Path = strings.Join(parts, "/")
			return e, nil
		}
		if !e.Dir() {
			return core.TreeEntry{}, fmt.Errorf("find %s: %w", name, errNoFile)
		}
		at = e.SHA
	}
	return core.TreeEntry{}, fmt.Errorf("find %s: %w", name, errNoFile)
}

// show puts the content, or why it isn't shown, in the preview.
func (p *preview) show(b core.Blob, err error) tea.Cmd {
	p.failed, p.blob, p.loaded = err, b, true
	p.asImage = err == nil && p.images != nil && !p.entry.Symlink() && imageFile(p.entry.Path)
	p.shown = shownNothing
	return p.draw()
}

// draw shows the file as an image while it is one the terminal shows, and
// otherwise as the pager shows any file. The pager changes only when what
// it shows does, so drawing again keeps the place in the text.
func (p *preview) draw() tea.Cmd {
	rows, st := p.imageRows()
	want := shownText
	switch st {
	case ui.ImageShown:
		want = shownImage
	case ui.ImageLoading:
		want = shownLoading
	case ui.ImageFailed, ui.ImageOff:
		// It shows as it did before images, such as a binary file. A
		// failure that may mend is asked for again once GitHub answers
		// again, which draws the preview again.
	}
	p.pic = rows
	if want == p.shown {
		return nil
	}
	p.shown = want
	switch want {
	case shownImage:
		p.pager.SetMessage(p.entry.Path, "")
		return nil
	case shownLoading:
		p.pager.SetMessage(p.entry.Path, "Loading the image…")
		return nil
	case shownNothing, shownText:
	}
	cmd, ok := fill(&p.pager, p.entry, p.blob, p.failed, p.open)
	if ok {
		// The search starts from the line, if there is one.
		p.pager.GoToLine(p.line)
		cmd = tea.Batch(cmd, p.pager.SetSearch(p.find))
		p.preset = p.find != ""
	}
	return cmd
}

// imageRows returns the lines of the image of the file, fitted to the
// pager's room above its status line, and how far the image got.
func (p *preview) imageRows() ([]string, ui.ImageState) {
	if !p.asImage {
		return nil, ui.ImageOff
	}
	w, h := p.pager.Width(), p.pager.Height()-1
	if w <= 0 || h <= 0 {
		if p.images.Drawing() {
			return nil, ui.ImageLoading
		}
		return nil, ui.ImageOff
	}
	src := ui.ImageSource{Repo: p.repo, SHA: p.entry.SHA, Size: p.blob.Size}
	return p.images.Fit(src, ui.ImageSize{Cols: w, Rows: h})
}

// fill puts the content of the file of e in pg, or why it isn't shown,
// naming open as the key that opens it in the browser instead. It reports
// whether it put the content, and returns the command that highlights it.
func fill(pg *pager.Model, e core.TreeEntry, b core.Blob, err error, open key.Binding) (tea.Cmd, bool) {
	name := e.Path
	switch {
	case errors.Is(err, core.ErrTooLarge):
		pg.SetMessage(name, "Too large to preview"+browserHint(open))
	case err != nil:
		pg.SetError(name, err)
	case b.Binary:
		pg.SetMessage(name, "Binary file, not shown"+browserHint(open))
	case e.Symlink():
		// The blob of a link holds its target.
		pg.SetMessage(name, "Symbolic link → "+string(b.Content))
	default:
		return pg.SetContent(name, string(b.Content)), true
	}
	return nil, false
}

// browserHint names open, the key that opens a file in the browser.
func browserHint(open key.Binding) string {
	if k := open.Help().Key; k != "" && open.Enabled() {
		return " · " + k + " opens it in the browser"
	}
	return ""
}

// Title returns the path of the file.
func (p *preview) Title() string {
	return p.entry.Path
}

// Link implements ui.Linked: the page of the file at the ref it was
// listed at.
func (p *preview) Link() string {
	return webURL(p.host, p.repo, p.ref, p.entry)
}

// Update takes the preview's content and passes the rest to the pager. The
// open key opens the file in the browser unless the pager's search input
// takes it. The close key closes the preview at once while the search
// shown is the one it opened with, which the user didn't ask for.
func (p *preview) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case blobMsg:
		if msg.id != p.pager.ID() {
			return nil
		}
		return p.show(msg.blob, msg.err)
	case entryMsg:
		if msg.id != p.pager.ID() {
			return nil
		}
		if msg.err != nil {
			return p.show(core.Blob{}, msg.err)
		}
		p.entry = msg.entry
		return p.load()
	case ui.ImagesMsg:
		if !p.loaded {
			return nil
		}
		return p.draw()
	case pager.CloseMsg:
		if msg.ID != p.pager.ID() {
			return nil
		}
		return p.close()
	case ui.OnlineMsg:
		// The file that failed for want of an answer is read again.
		if !ui.Unreached(p.failed) {
			return nil
		}
		p.failed = nil
		return p.load()
	case tea.KeyPressMsg:
		if !p.pager.Capturing() && key.Matches(msg, p.open) {
			return ui.Open(webURL(p.host, p.repo, p.ref, p.entry))
		}
		if p.preset && key.Matches(msg, p.pager.KeyMap().Close) {
			return p.close()
		}
	}
	var cmd tea.Cmd
	p.pager, cmd = p.pager.Update(msg)
	if p.pager.Capturing() {
		p.preset = false
	}
	return cmd
}

// close ends the preview's reads, and closes it, or reopens the modal it
// was opened from.
func (p *preview) close() tea.Cmd {
	p.cancel()
	if p.ret != nil {
		return ui.Reopen(p.ret)
	}
	return ui.CloseModal(p)
}

// View renders the pager, or the image above the pager's status line. The
// lines of the image reach the terminal as they are, in no style.
func (p *preview) View() string {
	v := p.pager.View()
	if p.shown != shownImage {
		return v
	}
	w, h := p.pager.Width(), p.pager.Height()-1
	var b strings.Builder
	for i := range h {
		line := ""
		if i < len(p.pic) {
			line = p.pic[i]
		}
		if ansi.StringWidth(line) > w {
			line = ansi.Truncate(line, w, "")
		}
		b.WriteString(line)
		b.WriteString(strings.Repeat(" ", w-ansi.StringWidth(line)))
		b.WriteByte('\n')
	}
	b.WriteString(v[strings.LastIndexByte(v, '\n')+1:])
	return b.String()
}

// SetSize sets the size of the pager, and fits the image to it.
func (p *preview) SetSize(width, height int) {
	p.pager.SetSize(width, height)
	if p.loaded && p.asImage {
		// The image or the loading line; neither has a command.
		_ = p.draw()
	}
}

// SetTheme styles the pager.
func (p *preview) SetTheme(t ui.Theme) {
	p.pager.SetStyles(t.Pager(p.icons))
}

// KeyLayers implements ui.Keyed: the open key, unless the pager's search
// input takes it, and then the pager's keys.
func (p *preview) KeyLayers() []keyhelp.Layer {
	open := p.open
	open.SetEnabled(open.Enabled() && !p.pager.Capturing())
	return []keyhelp.Layer{
		{Source: "file", Bindings: []key.Binding{open}, Short: []key.Binding{open}},
		keyhelp.FromHelp("pager", p.pager, p.pager.Capturing()),
	}
}
