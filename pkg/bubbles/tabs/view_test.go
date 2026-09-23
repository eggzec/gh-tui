package tabs

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestViewDefault(t *testing.T) {
	golden.RequireEqual(t, New(WithTabs(sections...)).View())
}

func TestViewWidths(t *testing.T) {
	for _, w := range []int{80, 60, 40} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			m := New(WithTabs(sections...), WithActive(2), WithWidth(w))
			golden.RequireEqual(t, m.View())
		})
	}
}

// Every width, down to one column, renders two lines of exactly that width,
// and the active title stays whole as long as it fits at all.
func TestViewFitsWidth(t *testing.T) {
	for active := range sections {
		for w := 1; w <= 80; w++ {
			m := New(WithTabs(sections...), WithActive(active), WithWidth(w))
			lines := strings.Split(m.View(), "\n")
			if len(lines) != m.Height() {
				t.Fatalf("width %d: %d lines, want %d", w, len(lines), m.Height())
			}
			for i, l := range lines {
				if got := ansi.StringWidth(l); got != w {
					t.Errorf("width %d, active %d: line %d is %d wide", w, active, i, got)
				}
			}
			title := sections[active]
			if w >= ansi.StringWidth(title)+(len(sections)-1)*(compactGap+1) &&
				!strings.Contains(ansi.Strip(lines[0]), title) {
				t.Errorf("width %d: active title %q shortened in %q", w, title, ansi.Strip(lines[0]))
			}
		}
	}
}

func TestViewNarrowing(t *testing.T) {
	tests := []struct {
		width int
		want  string
	}{
		{width: 80, want: " Pull requests    Issues    Notifications    Repositories"},
		{width: 55, want: "Pull requests  Issues  Notifications  Repositories"},
		{width: 40, want: "Pull re…  Issues  Notifications  Reposi…"},
		{width: 35, want: "Pull…  Issu…  Notifications  Repo…"},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			m := New(WithTabs(sections...), WithActive(2), WithWidth(tt.width))
			line, _, _ := strings.Cut(ansi.Strip(m.View()), "\n")
			if got := strings.TrimRight(line, " "); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestViewIndicatorUnderActive(t *testing.T) {
	m := New(WithTabs(sections...), WithActive(1), WithWidth(80))
	line, rule, _ := strings.Cut(ansi.Strip(m.View()), "\n")
	start := strings.Index(line, "Issues")
	want := strings.Repeat("─", start) + strings.Repeat("━", len("Issues")) +
		strings.Repeat("─", 80-start-len("Issues"))
	if rule != want {
		t.Errorf("rule = %q\nwant   %q", rule, want)
	}
}

func BenchmarkView(b *testing.B) {
	m := New(WithTabs(sections...), WithWidth(80))
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}
