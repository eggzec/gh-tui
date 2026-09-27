package issues

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

func theme(dark bool) ui.Theme {
	p, _ := config.Default().Palette(dark)
	return ui.NewTheme(p, dark)
}

// threadModal returns a section width wide with the modal of issue #999
// open on comments, in a dark or light theme.
func threadModal(tb testing.TB, comments []core.Comment, dark bool, width, height int) (*host, *detailModal) {
	tb.Helper()
	svc := newFakeService(sampleIssues(12))
	svc.addComments(999, comments...)
	h := started(tb, svc, width, height)
	h.SetTheme(theme(dark))
	press(tb, h, "down", "enter")
	m := h.modal()
	if m == nil {
		tb.Fatal("enter didn't open the issue")
	}
	return h, m
}

func TestViewComments(t *testing.T) {
	for _, dark := range []bool{true, false} {
		name := "light"
		if dark {
			name = "dark"
		}
		for _, width := range []int{80, 120} {
			t.Run(name+"/"+strconv.Itoa(width), func(t *testing.T) {
				_, m := threadModal(t, uitest.Comments(testNow), dark, width, 100)
				v := m.View()
				assertFits(t, v, width, 100)
				golden.RequireEqual(t, v)
			})
		}
	}
}

func TestCommentsShowAsMarkdown(t *testing.T) {
	_, m := threadModal(t, uitest.Comments(testNow), true, 80, 100)
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	v := strings.Join(lines, "\n")
	for _, want := range []string{
		"  hubot · 5h ago\n  What happened",
		"  What happened",
		"  The modal shows raw markdown\n  instead of rendering it.",
		"  [✓] I searched the issues",
		"  🖼 screenshot (https://github.com/user-attachments/assets/1234)",
		"  ▸ Stack trace",
		"›   ◆ flowchart · 2 lines · View diagram ↗",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("the modal lacks %q:\n%s", want, v)
		}
	}
	for _, raw := range []string{"###", "**raw**", "<!--", "<details>", "<img", "```", "\x1b]0;"} {
		if strings.Contains(v, raw) {
			t.Errorf("the modal shows %q:\n%s", raw, v)
		}
	}
}

// Scrolling, and a reload that finds the same comments, only read what was
// rendered; a new width or theme renders again, and an old width is kept.
func TestCommentsRenderOnce(t *testing.T) {
	h, m := threadModal(t, uitest.Thread(50, testNow), true, 80, 30)
	runs := m.thread.MarkdownRenders()
	if runs == 0 {
		t.Fatal("nothing was rendered")
	}
	for _, k := range []string{"j", "j", "G", "k", "g"} {
		press(t, h, k)
		_ = m.View()
	}
	run(t, h, h.Update(ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}))
	if got := m.thread.MarkdownRenders(); got != runs {
		t.Fatalf("scrolling and reloading rendered %d times", got-runs)
	}
	m.SetSize(100, 30)
	run(t, h, h.Update(nil))
	resized := m.thread.MarkdownRenders()
	if resized == runs {
		t.Fatal("a new width didn't render again")
	}
	m.SetSize(80, 30)
	if got := m.thread.MarkdownRenders(); got != resized {
		t.Errorf("going back to a width rendered %d times", got-resized)
	}
	m.SetTheme(theme(false))
	if m.thread.MarkdownRenders() == resized {
		t.Error("a new theme didn't render again")
	}
}

// BenchmarkViewThread draws the modal of an issue with 50 comments,
// scrolled into them.
func BenchmarkViewThread(b *testing.B) {
	h, m := threadModal(b, uitest.Thread(50, testNow), true, 120, 40)
	for range 60 {
		press(b, h, "j")
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

// BenchmarkUpdateThread scrolls, reloads and resizes the modal of an issue
// with 50 comments.
func BenchmarkUpdateThread(b *testing.B) {
	b.Run("scroll", func(b *testing.B) {
		h, _ := threadModal(b, uitest.Thread(50, testNow), true, 120, 40)
		down, up := keyMsg("j"), keyMsg("k")
		b.ReportAllocs()
		i := 0
		for b.Loop() {
			k := down
			if i%40 >= 20 {
				k = up
			}
			i++
			_ = h.Update(k)
		}
	})
	b.Run("reload", func(b *testing.B) {
		h, _ := threadModal(b, uitest.Thread(50, testNow), true, 120, 40)
		sync := ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}
		b.ReportAllocs()
		for b.Loop() {
			run(b, h, h.Update(sync))
		}
	})
	b.Run("resize", func(b *testing.B) {
		_, m := threadModal(b, uitest.Thread(50, testNow), true, 120, 40)
		b.ReportAllocs()
		i := 0
		for b.Loop() {
			m.SetSize(100+20*(i%2), 40)
			i++
		}
	})
}

// A view too narrow for the margins still shows the body, a cell wide.
func TestNarrowCommentsKeepTheirBody(t *testing.T) {
	_, m := threadModal(t, nil, true, 80, 20)
	c := core.Comment{Author: core.User{Login: "octocat"}, Body: "ok"}
	if out := ansi.Strip(m.renderComment(c, 4)); !strings.Contains(out, "o") || !strings.Contains(out, "k") {
		t.Errorf("a narrow comment lost its body: %q", out)
	}
}

// With a diagram on screen, select shows its code, as the help says,
// its head links to it on mermaid.live, and open still opens the issue.
func TestDiagramKeys(t *testing.T) {
	h, m := threadModal(t, uitest.Comments(testNow), true, 80, 100)
	var want string
	for _, c := range uitest.Comments(testNow) {
		if b := markdown.Blocks(c.Body); len(b) > 0 && b[0].Lang == "mermaid" {
			want = b[0].URL
		}
	}
	if !strings.HasPrefix(want, "https://mermaid.live/view#pako:") {
		t.Fatalf("the comments have no diagram with a link: %q", want)
	}
	if v := m.View(); strings.Count(v, "\x1b]8;;"+want+"\x1b\\") != 1 || strings.Count(v, "\x1b]8;;\x1b\\") != 1 {
		t.Errorf("the head doesn't link to %q once:\n%q", want, v)
	}
	var help []string
	for _, b := range m.Help().ShortHelp() {
		if b.Enabled() {
			help = append(help, b.Help().Key+" "+b.Help().Desc)
		}
	}
	if !slices.Contains(help, "↵ diagram code") || !slices.Contains(help, "o browser") {
		t.Errorf("the help doesn't offer the diagram's code and the issue: %q", help)
	}
	if o, ok := has[ui.OpenMsg](press(t, h, "o")); !ok || o.URL != "https://github.com/eggzec/gh-tui/issues/999" {
		t.Errorf("open sent %+v, want the issue", o)
	}
	press(t, h, "enter")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "A[issue] --> B[modal]") {
		t.Errorf("select didn't show the diagram's code:\n%s", v)
	}
}
