package markdown

import (
	"strings"
	"testing"
)

// The text of a link keeps its underline while links are on, as it does
// with them off.
func TestRenderLinkTextUnderlined(t *testing.T) {
	const src = "See [the docs](https://example.com/docs) now."
	linksOn(t, true)
	raw := New(DefaultStyle(true)).Render(src, 80)
	i := strings.Index(raw, "the docs")
	if i < 0 {
		t.Fatalf("no text: %q", raw)
	}
	if !strings.Contains(raw[:i], "\x1b[4m") || !strings.Contains(raw[i:], "\x1b[24m") {
		t.Errorf("the text of the link is not underlined: %q", raw)
	}
}
