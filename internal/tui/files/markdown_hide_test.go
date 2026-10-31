package files

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// With links on, a link in a markdown file shows its text and not its
// address. This test is serial because it sets the process-wide links switch.
func TestPreviewHidesLinkAddress(t *testing.T) {
	was := termtext.LinksOn()
	termtext.SetLinks(true)
	t.Cleanup(func() { termtext.SetLinks(was) })

	f := sampleFake()
	f.addBlob(file("AGENTS.md", 0), "See [the docs](https://example.com/guide) now.\n")
	h := openRow(t, f, rowAgents)
	if v := h.modal(); !strings.Contains(v, "the docs") || strings.Contains(v, "example.com") {
		t.Errorf("the preview shows the address or lacks the text:\n%s", v)
	}
}
