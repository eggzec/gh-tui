package desc

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func theme() ui.Theme {
	p, err := config.Default().Palette(true)
	if err != nil {
		panic(err)
	}
	return ui.NewTheme(p, true)
}

// opts returns the options of rows drawn in width cells.
func opts(width int) Options {
	ic := ui.NewIcons(config.IconsNerd)
	md := markdown.New(theme().Thread(ic).Markdown)
	md.SetGlyphs(ic.Markdown)
	return Options{Width: width, Styles: NewStyles(theme(), ic, config.IconsNerd), MD: md}
}

// lexMu keeps tests from highlighting code at the same time. One lexer runs
// at a time in the process, and code that meets another's lexer shows plain,
// which is right in the app, where the render is drawn again, but would make
// the output of a parallel test hang on which of them got there first.
var lexMu sync.Mutex

// rowsOf returns the rows of body, drawn by Rows with no other test
// highlighting.
func rowsOf(body string, o Options) []foldtree.Row {
	lexMu.Lock()
	defer lexMu.Unlock()
	return Rows(body, o)
}

// tree returns the tree of rows in width cells and height lines.
func tree(rows []foldtree.Row, width, height int) foldtree.Model {
	return foldtree.New(
		foldtree.WithSize(width, height),
		foldtree.WithStyles(Tree(foldtree.DefaultStyles(true), ui.NewIcons(config.IconsNerd))),
		foldtree.WithRows(rows),
	)
}

// text draws rows as the pane does, without colors or links, with every fold
// opened when all is set.
func text(rows []foldtree.Row, width int, all bool) string {
	m := tree(rows, width, 400)
	if all {
		openAll(&m, rows)
	}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func openAll(m *foldtree.Model, rows []foldtree.Row) {
	for _, r := range rows {
		if r.Fold {
			m.SetFolded(r.ID, false)
		}
	}
}

func byID(rows []foldtree.Row) map[string]foldtree.Row {
	m := make(map[string]foldtree.Row, len(rows))
	for _, r := range rows {
		m[r.ID] = r
	}
	return m
}

func plain(r foldtree.Row) string { return ansi.Strip(strings.Join(r.Lines, "\n")) }

func TestCounts(t *testing.T) {
	t.Parallel()
	body := "intro\n\n" +
		"<details><summary>Outer</summary>\n\nfirst\n\nsecond\n\n" +
		"<details><summary>Inner</summary>\n\nx\n\ny\n\n</details>\n\n" +
		"<details>\n\nonly\n\n</details>\n\n</details>\n\n" +
		"<details><summary>Single</summary>\n\none\n\n</details>"
	rows := byID(rowsOf(body, opts(80)))
	for id, want := range map[string]string{
		// first, blank and second; the inner summary with its x, blank and y;
		// then the summary of the one without a title and its text. The
		// blank lines next to a block are not drawn, and the ends not counted.
		"d0":     "⋯ 9 lines",
		"d0.0":   "⋯ 3 lines",
		"d0.1":   "⋯ 1 line",
		"d1":     "⋯ 1 line",
		"d0/end": "",
	} {
		r, ok := rows[id]
		if !ok {
			t.Fatalf("no row %s", id)
		}
		if r.Marker != want {
			t.Errorf("%s marker = %q, want %q", id, r.Marker, want)
		}
	}
	if got := plain(rows["d0.1"]); got != "Details" {
		t.Errorf("a block without a summary is titled %q", got)
	}
	if got := plain(rows["d0"]); got != "Outer" {
		t.Errorf("summary = %q", got)
	}
	if !rows["d0"].Closed || !rows["d0"].Fold {
		t.Error("a block is not a closed fold by default")
	}
}

func TestOpenState(t *testing.T) {
	t.Parallel()
	o := opts(80)
	o.Open = map[string]bool{"d1": true}
	rows := byID(rowsOf("<details>a</details>\n\n<details>b</details>", o))
	if !rows["d0"].Closed || rows["d1"].Closed {
		t.Errorf("closed: d0 %v, d1 %v", rows["d0"].Closed, rows["d1"].Closed)
	}
}

func TestDepthAndEnds(t *testing.T) {
	t.Parallel()
	rows := rowsOf("<details><summary>A</summary>a<details><summary>B</summary>b</details></details>", opts(80))
	got := make([]string, 0, len(rows))
	for _, r := range rows {
		got = append(got, fmt.Sprintf("%s@%d", r.ID, r.Depth))
	}
	want := "d0@0 /0.0@1 d0.0@1 d0.0//0.0@2 d0.0/end@2 d0/end@1"
	// The text of the block "d0" has the ID of its path and its place.
	want = strings.Replace(want, "/0.0@1", "d0/0.0@1", 1)
	want = strings.Replace(want, "d0.0//0.0@2", "d0.0/0.0@2", 1)
	if g := strings.Join(got, " "); g != want {
		t.Errorf("rows\n got %s\nwant %s", g, want)
	}
	for _, r := range rows {
		if strings.HasSuffix(r.ID, "/end") && (!r.Skip || plain(r) != "") {
			t.Errorf("%s is not an empty row the cursor skips", r.ID)
		}
	}
}

func TestFenceIsText(t *testing.T) {
	t.Parallel()
	rows := rowsOf("```html\n<details>\n<summary>S</summary>\n```", opts(80))
	for _, r := range rows {
		if r.Fold {
			t.Errorf("%s folds, from a tag in a fence", r.ID)
		}
	}
	if got := text(rows, 80, false); !strings.Contains(got, "<details>") {
		t.Errorf("the tag is not shown:\n%s", got)
	}
}

func TestUnclosedRunsToEnd(t *testing.T) {
	t.Parallel()
	rows := rowsOf("<details><summary>S</summary>\n\none\n\ntwo", opts(80))
	if got := text(rows, 80, true); !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Errorf("an unclosed block lost its content:\n%s", got)
	}
}

func TestTabs(t *testing.T) {
	t.Parallel()
	got := text(rowsOf("```\na\tb\n\tc\n```", opts(80)), 80, false)
	if strings.Contains(got, "\t") {
		t.Error("a tab reached the rows")
	}
	// The tab after "a" stops at the next column of 4, and the one that
	// starts the next line takes the code four cells in.
	if !strings.Contains(got, "a   b\n") || !strings.Contains(got, "\n       c\n") {
		t.Errorf("tabs are not 4-column stops:\n%s", got)
	}
}

func TestEmpty(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"", " \n\t\n", "<details></details>"[:0]} {
		if rows := rowsOf(body, opts(80)); len(rows) != 0 {
			t.Errorf("rowsOf(%q) = %d rows", body, len(rows))
		}
	}
	if rows := rowsOf("text", opts(0)); len(rows) != 0 {
		t.Errorf("a tree without width has %d rows", len(rows))
	}
}

func TestNoMarkdown(t *testing.T) {
	t.Parallel()
	o := opts(20)
	o.MD = nil
	got := text(rowsOf("a plain body that wraps over lines", o), 20, false)
	if !strings.Contains(got, "wraps") {
		t.Errorf("no text:\n%s", got)
	}
}

func TestDiagram(t *testing.T) {
	t.Parallel()
	body := "before\n\n```mermaid\ngraph TD\n  A-->B\n```\n\nafter"
	rows := rowsOf(body, opts(80))
	var d foldtree.Row
	for _, r := range rows {
		if r.Fold {
			d = r
		}
	}
	if d.ID == "" || !d.Closed || len(d.Lines) < 2 {
		t.Fatalf("no closed diagram fold with its code: %+v", d)
	}
	if got := ansi.Strip(d.Lines[0]); !strings.Contains(got, "View diagram") {
		t.Errorf("head = %q", got)
	}
	o := opts(80)
	o.Open = map[string]bool{d.ID: true}
	for _, r := range rowsOf(body, o) {
		if r.ID == d.ID && r.Closed {
			t.Error("the diagram does not start open")
		}
	}
}

func TestHostile(t *testing.T) {
	t.Parallel()
	h := termtexttest.Hostile
	bodies := map[string]string{
		"escapes": h + "\n\n<details><summary>" + h + "</summary>\n\n\t" + h + "\n\n</details>\n\n```\n" + h + "\n```",
		"long":    strings.Repeat("a line of a very long body\n\n", 5000),
		"lines":   strings.Repeat("x\n", 10000),
		"code":    "```\n" + strings.Repeat("w", 400) + "\n```",
		"deep": strings.Repeat("<details><summary>s</summary>\n\n", 50) + "text\n\n" +
			strings.Repeat("</details>\n\n", 50),
		"tags": strings.Repeat("<details><summary>", 3000),
	}
	for name, body := range bodies {
		for _, w := range []int{20, 40, 80, 160} {
			t.Run(fmt.Sprintf("%s-%d", name, w), func(t *testing.T) {
				t.Parallel()
				rows := rowsOf(body, opts(w))
				m := tree(rows, w, 400)
				openAll(&m, rows)
				termtexttest.AssertClean(t, m.View(), w)
				for _, r := range rows {
					for _, l := range r.Lines {
						if strings.Contains(l, "\t") {
							t.Fatalf("%s has a tab: %q", r.ID, l)
						}
					}
				}
			})
		}
	}
}

func BenchmarkRows(b *testing.B) {
	body := strings.Repeat("A paragraph of text with `code` and a [link](https://example.com).\n\n"+
		"<details><summary>More</summary>\n\n- one\n- two\n\n```go\nfunc main() {}\n```\n\n</details>\n\n", 100)
	o := opts(160)
	b.ReportAllocs()
	for b.Loop() {
		rowsOf(body, o)
	}
}

const plainBody = "# Fix the cache\n\nThe cache dropped **entries** on a resize, see #12 and `Cache.Resize`.\n\n" +
	"- keep the order\n- drop the oldest\n\nFixes #12. Thanks to @octo for the report, which came with a long line " +
	"that has to wrap somewhere before the right edge of the pane.\n"

const codeBody = "Before:\n\n```go\nfunc Resize(c *Cache, n int) { for len(c.items) > n { c.drop(c.oldest()) }; c.size = n; c.notify() }\n```\n\n" +
	"After the change:\n\n```\n\tindented\twith tabs\n```\n"

const detailsBody = "Intro paragraph.\n\n" +
	"<details>\n<summary>Benchmarks</summary>\n\nbefore: 12ms\n\nafter: 9ms\n\n</details>\n\n" +
	"<details>\n<summary>Changelog</summary>\n\nThe tags between the two versions:\n\n" +
	"<details><summary>0.42.0</summary>\n\n- security fixes only\n- a second item\n\n</details>\n\n" +
	"<details>\n\nan untitled block\n\n</details>\n\n</details>\n\nOutro paragraph.\n"

const mermaidBody = "The flow:\n\n```mermaid\nsequenceDiagram\n  A->>B: hello\n  B-->>A: hi\n```\n\nAfter the flow.\n"

func TestGolden(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		open       map[string]bool
	}{
		{"plain", plainBody, nil},
		{"code", codeBody, nil},
		{"details", detailsBody, nil},
		{"details-open", detailsBody, map[string]bool{"d0": true, "d1": true, "d1.0": true, "d1.1": true}},
		{"details-mixed", detailsBody, map[string]bool{"d1": true}},
		{"mermaid", mermaidBody, nil},
		{"mermaid-open", mermaidBody, map[string]bool{"/0.m0": true}},
	} {
		for _, w := range []int{80, 120, 160} {
			t.Run(fmt.Sprintf("%s-%d", tc.name, w), func(t *testing.T) {
				t.Parallel()
				o := opts(w)
				o.Open = tc.open
				rows := rowsOf(tc.body, o)
				golden.RequireEqual(t, text(rows, w, false))
			})
		}
	}
}
