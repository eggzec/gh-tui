package files

import (
	"context"
	"errors"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// preview shows a file in a pager, in a modal over the screen. It is opened
// for one file and closed when the pager asks.
type preview struct {
	ctx    context.Context
	cancel context.CancelFunc
	svc    Service
	repo   core.RepoRef
	entry  core.TreeEntry
	open   key.Binding
	pager  pager.Model
}

// blobMsg carries the content of the file of the preview whose pager has
// the ID, so that the results of a closed preview are ignored.
type blobMsg struct {
	id   int64
	blob core.Blob
	err  error
}

// newPreview returns a preview of e, a file of repo. Its load runs under
// ctx until it closes.
func newPreview(ctx context.Context, svc Service, repo core.RepoRef, e core.TreeEntry, open key.Binding) *preview {
	ctx, cancel := context.WithCancel(ctx)
	p := &preview{ctx: ctx, cancel: cancel, svc: svc, repo: repo, entry: e, open: open, pager: pager.New()}
	p.pager.Focus()
	return p
}

// load shows the content at once when it is cached, and fetches it
// otherwise.
func (p *preview) load() tea.Cmd {
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

// show puts the content, or why it isn't shown, in the pager.
func (p *preview) show(b core.Blob, err error) tea.Cmd {
	name := p.entry.Path
	switch {
	case errors.Is(err, core.ErrTooLarge):
		p.pager.SetMessage(name, "Too large to preview"+p.browserHint())
	case err != nil:
		p.pager.SetError(name, err)
	case b.Binary:
		p.pager.SetMessage(name, "Binary file, not shown"+p.browserHint())
	case p.entry.Symlink():
		// The blob of a link holds its target.
		p.pager.SetMessage(name, "Symbolic link → "+string(b.Content))
	default:
		return p.pager.SetContent(name, string(b.Content))
	}
	return nil
}

// browserHint names the key that opens the file in the browser.
func (p *preview) browserHint() string {
	if k := p.open.Help().Key; k != "" {
		return " · " + k + " opens it in the browser"
	}
	return ""
}

// Title returns the path of the file.
func (p *preview) Title() string {
	return p.entry.Path
}

// Update takes the preview's content and passes the rest to the pager. The
// open key opens the file in the browser unless the pager's search input
// takes it.
func (p *preview) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case blobMsg:
		if msg.id != p.pager.ID() {
			return nil
		}
		return p.show(msg.blob, msg.err)
	case pager.CloseMsg:
		if msg.ID != p.pager.ID() {
			return nil
		}
		p.cancel()
		return ui.CloseModal(p)
	case tea.KeyPressMsg:
		if !p.pager.Capturing() && key.Matches(msg, p.open) {
			return ui.Open(webURL(p.repo, p.entry))
		}
	}
	var cmd tea.Cmd
	p.pager, cmd = p.pager.Update(msg)
	return cmd
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

// Help returns the keys of the pager and the open key.
func (p *preview) Help() help.KeyMap {
	return previewKeys{pager: p.pager, open: p.open}
}

// previewKeys lists the keys of a preview for help.
type previewKeys struct {
	pager pager.Model
	open  key.Binding
}

// ShortHelp returns the bindings for the short help view. While the search
// input is open, only its own keys work.
func (k previewKeys) ShortHelp() []key.Binding {
	if k.pager.Capturing() {
		return k.pager.ShortHelp()
	}
	return append(k.pager.ShortHelp(), k.open)
}

// FullHelp returns the bindings for the full help view.
func (k previewKeys) FullHelp() [][]key.Binding {
	if k.pager.Capturing() {
		return k.pager.FullHelp()
	}
	return append(k.pager.FullHelp(), []key.Binding{k.open})
}
