package issues

import (
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
	// The conversation shows its comments collapsed; open them all.
	press(tb, h, "*")
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
		"hubot  What happened",
		"The modal shows raw markdown",
		"instead of rendering it.",
		"[✓] I searched the issues",
		"🖼 screenshot",
		"Stack trace",
		"◆ flowchart · 2 lines · View diagram ↗",
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
