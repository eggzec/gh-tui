package files

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

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
	// find is searched for once the content is shown, and preset holds
	// while that search is the one shown, until the user starts another.
	// line is the line the preview opens on, if set.
	find   string
	preset bool
	line   int
	// ret is the modal the preview reopens when it closes, if set.
	ret ui.Modal
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
// is on host, which words what went wrong with v. Its load runs under ctx
// until it closes.
func newPreview(ctx context.Context, svc Service, host string, repo core.RepoRef, ref string, e core.TreeEntry, open key.Binding, v ui.Voice) *preview {
	ctx, cancel := context.WithCancel(ctx)
	// The preview loads the file once, and opens it on GitHub with open.
	v.Retry, v.Open = key.Binding{}, open
	pg := pager.New(pager.WithErrorText(fileErrorText(repo, v)))
	p := &preview{ctx: ctx, cancel: cancel, svc: svc, host: host, repo: repo, ref: ref, entry: e, open: open, pager: pg}
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

// show puts the content, or why it isn't shown, in the pager.
func (p *preview) show(b core.Blob, err error) tea.Cmd {
	cmd, ok := fill(&p.pager, p.entry, b, err, p.open)
	if ok {
		// The search starts from the line, if there is one.
		p.pager.GoToLine(p.line)
		cmd = tea.Batch(cmd, p.pager.SetSearch(p.find))
		p.preset = p.find != ""
	}
	return cmd
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
	case pager.CloseMsg:
		if msg.ID != p.pager.ID() {
			return nil
		}
		return p.close()
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

// View renders the pager.
func (p *preview) View() string {
	return p.pager.View()
}

// SetSize sets the size of the pager.
func (p *preview) SetSize(width, height int) {
	p.pager.SetSize(width, height)
}

// SetTheme styles the pager.
func (p *preview) SetTheme(t ui.Theme) {
	p.pager.SetStyles(t.Pager())
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
