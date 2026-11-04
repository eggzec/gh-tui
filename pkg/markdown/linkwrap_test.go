package markdown

import (
	"strconv"
	"strings"
	"testing"
)

// A link wrapped over lines is a hyperlink on each of them: balanced, and
// on every line where its text is drawn underlined.
func TestRenderWrappedLinkIsLinkedOnEveryLine(t *testing.T) {
	// Serial: links are a process-wide setting.
	const text = "the very long documentation page of the project"
	srcs := map[string]string{
		"paragraph": "Read [" + text + "](https://example.com/d) now and later on.",
		"list":      "- see [" + text + "](https://example.com/d) now and later on\n  - nested [" + text + "](https://example.com/d) now",
		"ordered":   "1. see [" + text + "](https://example.com/d) now and later on",
	}
	for name, src := range srcs {
		for _, width := range []int{20, 25, 30, 35, 40, 50} {
			t.Run(name+"/"+strconv.Itoa(width), func(t *testing.T) {
				linksOn(t, true)
				raw := New(DefaultStyle(true)).Render(src, width)
				drawn, _ := underlinedCells(raw)
				for i, l := range strings.Split(raw, "\n") {
					opened, closed := hyperlinks(l)
					if opened != closed {
						t.Errorf("line %d opens %d hyperlinks and closes %d: %q", i, opened, closed, l)
					}
					if strings.TrimSpace(drawn[i]) != "" && opened == 0 {
						t.Errorf("line %d draws %q underlined with no hyperlink: %q", i, drawn[i], l)
					}
				}
				if strings.ContainsFunc(raw, isLinkRune) {
					t.Errorf("a character of the links is left: %q", raw)
				}
			})
		}
	}
}
