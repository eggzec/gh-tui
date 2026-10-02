package uitest

import (
	"context"
	"image/color"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/imgcaps"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termimg"
)

// TestCell is the size of a cell the images of tests are made for.
var TestCell = imgcaps.Cell{Width: 10, Height: 20}

// ImageHost is a fake image host: it serves every image with a few bytes
// of PNG at once, which fill the box asked for, and records what it was
// asked. A file of a repository it names "blob <repo> <sha>".
type ImageHost struct {
	mu      sync.Mutex
	asked   []string
	sources []ui.ImageSource
}

// Fetch serves src.
func (s *ImageHost) Fetch(_ context.Context, src ui.ImageSource, box ui.ImageBox) (ui.Picture, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := src.URL
	if src.SHA != "" {
		name = "blob " + src.Repo.String() + " " + src.SHA
	}
	s.asked, s.sources = append(s.asked, name), append(s.sources, src)
	return ui.Picture{
		PNG:   []byte("png of " + name),
		Width: box.Cols * box.Cell.Width, Height: box.Rows * box.Cell.Height,
		Cols: box.Cols, Rows: box.Rows,
	}, nil
}

// Asked returns the addresses fetched, in order.
func (s *ImageHost) Asked() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.asked...)
}

// Source returns the source the image at url was fetched as, the first
// time, and whether it was.
func (s *ImageHost) Source(url string) (ui.ImageSource, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, src := range s.sources {
		if src.URL == url {
			return src, true
		}
	}
	return ui.ImageSource{}, false
}

// Avatars returns the images of github.com over src, avatars included, on
// a terminal that shows images when images is set, with cells of TestCell.
func Avatars(src *ImageHost, images bool) *ui.Images {
	a := ui.NewImages(context.Background(), src.Fetch, true)
	a.SetGraphics(ui.Graphics{Images: images, Cell: TestCell})
	return a
}

// LoadAvatars ends an update as the app does: it fetches the images drawn
// since the last load, and hands each to a as it arrives. It returns the
// sequences a wrote to the terminal, and whether the images drawn
// changed, which the app then tells the sections with an ImagesMsg.
func LoadAvatars(tb testing.TB, a *ui.Images) (raw string, changed bool) {
	tb.Helper()
	var b strings.Builder
	var run func(cmd tea.Cmd)
	run = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				run(c)
			}
		case tea.RawMsg:
			b.WriteString(msg.Msg.(string))
		default:
			sent, redraw, handled := a.Update(msg)
			if !handled {
				tb.Fatalf("the images didn't take %T", msg)
			}
			changed = changed || redraw != ui.RedrawNone
			run(sent)
		}
	}
	cmd, redraw := a.Load()
	changed = redraw != ui.RedrawNone
	run(cmd)
	return b.String(), changed
}

// Placeholders checks that every cell of view that shows part of an image
// keeps what names it, as the terminal reads it: its own 256-color
// foreground, with no reverse that would swap it into the background,
// and all three diacritics. It returns how many cells it found, so a
// caller can tell that images showed at all.
func Placeholders(tb testing.TB, view string) int {
	tb.Helper()
	lines := strings.Split(view, "\n")
	w := 0
	for _, l := range lines {
		w = max(w, ansi.StringWidth(l))
	}
	buf := uv.NewScreenBuffer(max(w, 1), max(len(lines), 1))
	uv.NewStyledString(view).Draw(buf, buf.Bounds())
	n := 0
	for y := range lines {
		for x := range w {
			c := buf.CellAt(x, y)
			if c == nil || !strings.HasPrefix(c.Content, string(rune(0x10EEEE))) {
				continue
			}
			n++
			if _, _, _, _, ok := termimg.Cell(c.Content); !ok {
				tb.Errorf("cell %d,%d lost a diacritic: %q", x, y, c.Content)
			}
			if _, ok := c.Style.Fg.(ansi.IndexedColor); !ok || c.Style.Fg == color.Color(ansi.IndexedColor(0)) {
				tb.Errorf("cell %d,%d has foreground %v, not a 256-color index that names an image", x, y, c.Style.Fg)
			}
			if c.Style.Attrs&uv.AttrReverse != 0 {
				tb.Errorf("cell %d,%d is reversed, which hides the image", x, y)
			}
		}
	}
	return n
}
