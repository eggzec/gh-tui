package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const blocks = "```go\nx := 1\n```\n\n<!--\n```hidden\n```\n-->\n\n~~~Mermaid\ngraph LR\n  A-->B\n~~~\n"

func TestBlocks(t *testing.T) {
	got := Blocks(blocks)
	if len(got) != 2 {
		t.Fatalf("got %d blocks, want 2: %+v", len(got), got)
	}
	if got[0].Lang != "go" || got[0].Full != "```go\nx := 1\n```" || got[0].Collapsed != "" || got[0].URL != "" {
		t.Errorf("block 0 = %+v", got[0])
	}
	d := got[1]
	if d.Lang != "mermaid" || d.Collapsed != "◆ flowchart · 2 lines · View diagram ↗" ||
		!strings.HasPrefix(d.URL, "https://mermaid.live/view#pako:") {
		t.Errorf("block 1 = %+v", d)
	}
}

// A diagram shows collapsed until it is opened, and then shows its code
// under its head; each way is rendered once.
func TestCollapsedBlocks(t *testing.T) {
	r := New(DefaultStyle(true))
	collapsed := ansi.Strip(r.Render(blocks, 40))
	if !strings.Contains(collapsed, "◆ flowchart") || strings.Contains(collapsed, "graph LR") {
		t.Errorf("the diagram isn't collapsed:\n%s", collapsed)
	}
	opened := ansi.Strip(r.Render(blocks, 40, 1))
	if !strings.Contains(opened, "◆ flowchart") || !strings.Contains(opened, "graph LR") {
		t.Errorf("the diagram didn't open:\n%s", opened)
	}
	r.Render(blocks, 40)
	r.Render(blocks, 40, 1)
	if r.Renders() != 2 {
		t.Errorf("rendered %d times, want once collapsed and once open", r.Renders())
	}
}
