package pulls

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

func theme(dark bool) ui.Theme {
	p, _ := config.Default().Palette(dark)
	return ui.NewTheme(p, dark)
}

// threadModal returns a section width wide with the modal of a pull request
// open on comments, in a dark or light theme.
func threadModal(tb testing.TB, comments []core.Comment, dark bool, width, height int) (*host, *detailModal) {
	tb.Helper()
	svc := newFakeService()
	svc.thread = comments
	h := started(tb, svc, width, height)
	h.SetTheme(theme(dark))
	press(tb, h, "enter")
	m := h.modal()
	if m == nil {
		tb.Fatal("enter didn't open the pull request")
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
				_, m := threadModal(t, uitest.Comments(clock), dark, width, 120)
				v := m.View()
				for i, l := range strings.Split(v, "\n") {
					if w := ansi.StringWidth(l); w != width {
						t.Errorf("line %d is %d cells wide, want %d", i+1, w, width)
					}
				}
				golden.RequireEqual(t, v)
			})
		}
	}
}

func TestCommentsShowAsMarkdown(t *testing.T) {
	_, m := threadModal(t, uitest.Comments(clock), true, 80, 120)
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	v := strings.Join(lines, "\n")
	for _, want := range []string{
		"hubot · 5h",
		"│ What happened",
		"│ The modal shows raw markdown\n",
		"│ [✓] I searched the issues",
		"│ 🖼 screenshot (https://github.com/user-attachments/assets/1234)",
		"│ ▸ Stack trace",
		"│   graph LR",
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
	h, m := threadModal(t, uitest.Thread(50, clock), true, 80, 30)
	runs := m.thread.MarkdownRenders()
	if runs == 0 {
		t.Fatal("nothing was rendered")
	}
	for _, k := range []string{"j", "j", "G", "k", "g"} {
		press(t, h, k)
		_ = m.View()
	}
	drain(t, h, h.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)}))
	if got := m.thread.MarkdownRenders(); got != runs {
		t.Fatalf("scrolling and reloading rendered %d times", got-runs)
	}
	m.SetSize(100, 30)
	drain(t, h, h.Update(nil))
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

// BenchmarkViewThread draws the modal of a pull request with 50 comments,
// scrolled into them.
func BenchmarkViewThread(b *testing.B) {
	h, m := threadModal(b, uitest.Thread(50, clock), true, 120, 40)
	for range 60 {
		press(b, h, "j")
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

// BenchmarkUpdateThread scrolls, reloads and resizes the modal of a pull
// request with 50 comments.
func BenchmarkUpdateThread(b *testing.B) {
	b.Run("scroll", func(b *testing.B) {
		h, _ := threadModal(b, uitest.Thread(50, clock), true, 120, 40)
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
		h, _ := threadModal(b, uitest.Thread(50, clock), true, 120, 40)
		sync := ui.SyncMsg{Key: pulls.SyncKey(repo)}
		b.ReportAllocs()
		for b.Loop() {
			drain(b, h, h.Update(sync))
		}
	})
	b.Run("resize", func(b *testing.B) {
		_, m := threadModal(b, uitest.Thread(50, clock), true, 120, 40)
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
