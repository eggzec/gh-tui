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
	if got[0].Lang != "go" || got[0].Full != "```go\nx := 1\n```" || got[0].Collapsed != "" {
		t.Errorf("block 0 = %+v", got[0])
	}
	// Mermaid shows as code for now.
	if got[1].Lang != "mermaid" || got[1].Collapsed != "" || got[1].URL != "" {
		t.Errorf("block 1 = %+v", got[1])
	}
}

// A language that collapses its blocks shows them collapsed until they are
// opened, and each way is rendered once.
func TestCollapsedBlocks(t *testing.T) {
	showBlock = func(lang, src string, left *budget) Block {
		b := fenced(lang, src, left)
		if lang == "mermaid" {
			b.Collapsed, b.URL = "◆ diagram", "https://mermaid.live/view"
		}
		return b
	}
	defer func() { showBlock = fenced }()

	if b := Blocks(blocks); b[1].Collapsed != "◆ diagram" || b[1].URL == "" {
		t.Fatalf("the diagram isn't collapsible: %+v", b[1])
	}
	r := New(DefaultStyle(true))
	collapsed := ansi.Strip(r.Render(blocks, 40))
	if !strings.Contains(collapsed, "◆ diagram") || strings.Contains(collapsed, "graph LR") {
		t.Errorf("the diagram isn't collapsed:\n%s", collapsed)
	}
	opened := ansi.Strip(r.Render(blocks, 40, 1))
	if strings.Contains(opened, "◆ diagram") || !strings.Contains(opened, "graph LR") {
		t.Errorf("the diagram didn't open:\n%s", opened)
	}
	r.Render(blocks, 40)
	r.Render(blocks, 40, 1)
	if r.Renders() != 2 {
		t.Errorf("rendered %d times, want once collapsed and once open", r.Renders())
	}
}
