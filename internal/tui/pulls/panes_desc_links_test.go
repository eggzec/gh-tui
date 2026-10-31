package pulls

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// With links on, a link in the description shows its text and not its
// address. This test is serial because it sets the process-wide links switch.
func TestDescriptionHidesLinkAddress(t *testing.T) {
	was := termtext.LinksOn()
	termtext.SetLinks(true)
	t.Cleanup(func() { termtext.SetLinks(was) })

	m := described(t, "See [the docs](https://example.com/guide) now.")
	if s := paneText(m); !strings.Contains(s, "the docs") || strings.Contains(s, "example.com") {
		t.Errorf("the pane shows the address or lacks the text:\n%s", s)
	}
}
