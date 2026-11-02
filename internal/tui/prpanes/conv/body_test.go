package conv

import (
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// lexMu keeps tests from highlighting code at the same time. One lexer runs
// at a time in the process, and code that meets another's lexer shows plain,
// which is right in the app, where the render is drawn again, but would make
// the output of a parallel test hang on which of them got there first.
var lexMu sync.Mutex

// buildOf returns the rows of d, built with no other test highlighting.
func buildOf(d Data, o Options) []treeview.Node {
	lexMu.Lock()
	defer lexMu.Unlock()
	return Build(d, o)
}

func TestCodeCut(t *testing.T) {
	t.Parallel()
	long := "x := " + strings.Repeat("a", 100)
	body := "Before.\n\n```go\n" + long + "\n    y := 2\n```\n\nAfter."
	item := core.TimelineItem{Kind: core.TimelineComment, ID: "c1", Actor: core.User{Login: "ann"}, At: at, Body: body}
	for _, set := range []string{config.IconsNerd, config.IconsASCII} {
		const width = 60
		o := opts(set, width)
		out := text(buildOf(Data{Items: []core.TimelineItem{item}}, o), width, true)
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		bar, cut := "▏ ", o.Styles.Glyphs.Cut
		if set == config.IconsASCII {
			bar = "| "
		}
		first, last := -1, -1
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > width {
				t.Errorf("%s: line %d is %d cells wide:\n%s", set, i, w, out)
			}
			if strings.Contains(l, bar) {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first < 0 || last-first != 1 {
			t.Fatalf("%s: want two code lines, found %d..%d:\n%s", set, first, last, out)
		}
		if !strings.HasSuffix(lines[first], cut) {
			t.Errorf("%s: the long line doesn't end in %q:\n%s", set, cut, out)
		}
		for _, i := range []int{first - 1, last + 1} {
			if strings.TrimSpace(lines[i]) == "" {
				t.Errorf("%s: line %d, next to the code, is blank:\n%s", set, i, out)
			}
		}
	}
}
