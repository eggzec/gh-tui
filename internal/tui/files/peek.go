package files

import (
	"path"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// peek is the passive preview beside the zoomed tree: it shows the file
// under the cursor from what the service has cached, and a one-line
// placeholder for what it hasn't. It reads nothing and asks for nothing, so
// moving the cursor sends no request; the reads ahead fill the cache, and
// the peek draws again when they report.
type peek struct {
	fileBody
	w, h int
	// at is what the box shows, so that a message that changes nothing of
	// it draws nothing again.
	at peekAt
	// cmd is the highlight that the last fill started, until PreviewCmd
	// hands it on. A newer fill makes an older result stale, and the pager
	// drops it.
	cmd tea.Cmd
}

// peekAt names what the peek last drew.
type peekAt struct {
	valid bool
	path  string
	sha   string
	state string
	w, h  int
}

// newPeek returns the peek of s.
func (s *Section) newPeek() *peek {
	p := &peek{}
	p.glyphs = func() ui.Icons { return s.icons }
	p.raw = func() bool { return s.rawMarkdown }
	p.pager = pager.New(pager.WithKeyMap(pager.NewKeyMap(ui.In(s.rawKeys, "preview"))),
		pager.WithRenderedNotes(renderedNumbersNote, renderedChopNote))
	// No images in the markdown, and no files looked up: the text only.
	p.img = fileImage{images: s.images, shown: shownText, ellipsis: s.icons.Ellipsis, cached: true}
	return p
}

// SetPreviewSize implements ui.Splitter.
func (s *Section) SetPreviewSize(width, height int) {
	p := s.peek
	p.w, p.h = max(width, 0), max(height, 0)
	p.pager.SetSize(p.w, p.h)
	s.syncPeek(true)
}

// PreviewTitle implements ui.Splitter.
func (s *Section) PreviewTitle() string {
	if s.tree != nil {
		if e := s.selected(); e.Path != "" {
			return e.Path
		}
	}
	return "Preview"
}

// PreviewCmd implements ui.Splitter. It hands on the highlight of the file
// the peek shows, once.
func (s *Section) PreviewCmd() tea.Cmd {
	p := s.peek
	if p == nil {
		return nil
	}
	cmd := p.cmd
	p.cmd = nil
	return cmd
}

// updatePeek lets the peek's pager take the message of its highlight. Keys
// and the mouse are not for it: the peek is passive.
func (s *Section) updatePeek(msg tea.Msg) {
	p := s.peek
	if p == nil || p.w == 0 || p.h == 0 {
		return
	}
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
		return
	}
	p.pager, _ = p.pager.Update(msg)
}

// PreviewView implements ui.Splitter.
func (s *Section) PreviewView() string {
	p := s.peek
	if p.w == 0 || p.h == 0 {
		return ""
	}
	return p.img.view(&p.pager)
}

// setPeekTheme styles the peek in t.
func (s *Section) setPeekTheme(t ui.Theme) {
	p := s.peek
	p.img.ellipsis = s.icons.Ellipsis
	p.pager.SetStyles(t.Pager(s.icons))
	p.md.setTheme(t, s.icons)
	p.pager.Rerender()
	s.syncPeek(true)
}

// syncPeek shows the file under the cursor in the peek, from the cache
// only. It does nothing unless what is shown changed, or force says the
// cache or the look did.
func (s *Section) syncPeek(force bool) {
	p := s.peek
	if p == nil || p.w == 0 || p.h == 0 {
		return
	}
	var e core.TreeEntry
	if s.tree != nil {
		e = s.selected()
	}
	b, hit, state := s.peekState(e)
	at := peekAt{valid: true, path: e.Path, sha: e.SHA, state: state, w: p.w, h: p.h}
	if !force && p.at == at {
		return
	}
	p.at = at
	switch {
	case e.Path == "":
		p.clear("", s.emptyPeek())
	case e.Dir():
		p.clear(e.Path, s.dirText(e))
	case e.Submodule():
		p.clear(e.Path, e.Path+" is a submodule, with its files in another repository.")
	case state == "binary":
		p.clear(e.Path, "Binary file, not shown")
	case state == "image":
		// The cache names an image by its repository, so the peek asks
		// with the one shown now.
		p.img.repo = s.repo
		p.img.set(e, core.Blob{Size: e.Size}, nil)
		if p.img.draw(&p.pager) {
			p.clear(e.Path, "Binary file, not shown")
		}
	case hit:
		p.img.clear()
		// The highlight fill returns is CPU only, no I/O: the peek queues it
		// for the app to run, and draws the text plain until it lands.
		p.cmd, _ = fill(&p.pager, e, b, nil, s.icons, p.rendering())
	case e.Size > s.prefetch.cursorMax && !e.Symlink():
		p.clear(e.Path, ui.TooLargeText(e.Path, core.ErrTooLarge))
	default:
		p.clear(e.Path, s.notRead())
	}
}

// peekState classifies e: whether the service has its content, and a name
// for what the peek will show.
func (s *Section) peekState(e core.TreeEntry) (b core.Blob, hit bool, state string) {
	switch {
	case e.Path == "":
		return b, false, "none"
	case e.Dir() || e.Submodule():
		return b, false, "tree"
	case binaryExt[strings.ToLower(path.Ext(e.Name))]:
		if s.peek.drawsImage(e) {
			return b, false, "image"
		}
		return b, false, "binary"
	}
	b, hit = s.svc.CachedBlob(s.blobQuery(e))
	if hit {
		return b, true, "hit"
	}
	return b, false, "miss"
}

func (s *Section) emptyPeek() string {
	if s.tree == nil {
		return "No repository selected"
	}
	return ""
}

// notRead is what the peek says of a file that is not in the cache.
func (s *Section) notRead() string {
	text := "Not read yet."
	if k := s.keys.Tree.Open.Help().Key; k != "" {
		text += " Press " + k + " to open it."
	}
	return text
}

// dirText is what the peek says of a folder: how many entries its cached
// listing has, or nothing but its name.
func (s *Section) dirText(e core.TreeEntry) string {
	if t, ok := s.svc.CachedTree(filesvc.TreeQuery{Repo: s.repo, Ref: e.SHA}); ok {
		return strconv.Itoa(len(t.Entries)) + " entries"
	}
	return e.Path
}

var _ ui.Splitter = (*Section)(nil)
