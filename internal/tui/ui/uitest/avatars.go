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

// TestCell is the size of a cell the avatars of tests are made for.
var TestCell = imgcaps.Cell{Width: 10, Height: 20}

// ImageSource is a fake image host: it serves every address with a few
// bytes of PNG at once, and records what it was asked.
type ImageSource struct {
	mu    sync.Mutex
	asked []string
}

// Fetch serves url.
func (s *ImageSource) Fetch(_ context.Context, url string, box ui.ImageBox) (ui.Picture, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked = append(s.asked, url)
	return ui.Picture{PNG: []byte("png of " + url), Width: box.Cols * box.Cell.Width, Height: box.Rows * box.Cell.Height}, nil
}

// Asked returns the addresses fetched, in order.
func (s *ImageSource) Asked() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.asked...)
}

// Avatars returns the avatars of github.com over src, on a terminal that
// shows images when images is set, with cells of TestCell.
func Avatars(src *ImageSource, images bool) *ui.Avatars {
	a := ui.NewAvatars(context.Background(), src.Fetch, true)
	a.SetGraphics(ui.Graphics{Images: images, Cell: TestCell})
	return a
}

// LoadAvatars ends an update as the app does: it fetches the avatars
// drawn since the last load, and hands each to a as it arrives. It returns
// the sequences a wrote to the terminal, and whether the avatars drawn
// changed, which the app then tells the sections with an AvatarsMsg.
func LoadAvatars(tb testing.TB, a *ui.Avatars) (raw string, changed bool) {
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
				tb.Fatalf("the avatars didn't take %T", msg)
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
