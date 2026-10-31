package thread

import (
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// With links on, a link in the body shows its text and not its address.
// This test is serial because it sets the process-wide links switch.
func TestBodyHidesLinkAddress(t *testing.T) {
	was := termtext.LinksOn()
	termtext.SetLinks(true)
	t.Cleanup(func() { termtext.SetLinks(was) })

	m := newTest(newSource(1, 1), nil, 60, 20)
	s := DefaultStyles(true)
	s.MarkdownLink = color.RGBA{R: 1, G: 2, B: 3, A: 255}
	m.SetStyles(s)
	m.SetDocument("  Title", "See [the docs](https://example.com/guide) now.")
	raw := m.View()
	view := ansi.Strip(raw)
	if !strings.Contains(view, "the docs") || strings.Contains(view, "example.com") {
		t.Errorf("view shows the address or lacks the text:\n%s", view)
	}
	if before, _, ok := strings.Cut(raw, "the docs"); !ok || !strings.Contains(before, "38;2;1;2;3m") {
		t.Errorf("the link is not in the link colour: %q", raw)
	}
}
