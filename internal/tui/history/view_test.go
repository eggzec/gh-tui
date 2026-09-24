package history

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// The sizes inside the frame on terminals of 140 by 40 and 80 by 24.
const (
	wideW, wideH     = 108, 30
	narrowW, narrowH = 60, 18
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		keys          []string
	}{
		{"140 columns", wideW, wideH, nil},
		{"140 columns branches", wideW, wideH, []string{"esc", "j"}},
		{"140 columns filter", wideW, wideH, []string{"esc", "/", "v"}},
		{"140 columns patch", wideW, wideH, []string{"enter", "enter"}},
		{"80 columns graph", narrowW, narrowH, nil},
		{"80 columns branches", narrowW, narrowH, []string{"esc", "j"}},
		{"80 columns commit", narrowW, narrowH, []string{"enter"}},
		{"80 columns patch", narrowW, narrowH, []string{"enter", "enter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, h := newModal(t, newFake(), tt.width, tt.height)
			h.keys(tt.keys...)
			v := m.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

func TestViewFitsAnySize(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {10, 3}, {30, 5}, {89, 12}, {90, 12}, {200, 50}} {
		for _, keys := range [][]string{nil, {"esc"}, {"enter"}, {"enter", "enter"}} {
			m, h := newModal(t, newFake(), wideW, wideH)
			h.keys(keys...)
			m.SetSize(size[0], size[1])
			assertFits(t, m.View(), size[0], size[1])
		}
	}
}

func TestNarrowBreadcrumb(t *testing.T) {
	m, h := newModal(t, newFake(), narrowW, narrowH)
	crumb := func() string { return strings.TrimSpace(ansi.Strip(strings.Split(m.View(), "\n")[0])) }
	s0 := short(main0)
	steps := []struct {
		keys []string
		want string
	}{
		{nil, "Branches › main"},
		{[]string{"enter"}, "Branches › main › " + s0},
		{[]string{"enter"}, "Branches › main › " + s0 + " › commands.go"},
		{[]string{"esc"}, "Branches › main › " + s0},
		{[]string{"esc", "esc"}, "Branches"},
	}
	for _, s := range steps {
		h.keys(s.keys...)
		if got := crumb(); got != s.want {
			t.Errorf("after %q the breadcrumb is %q, want %q", s.keys, got, s.want)
		}
	}
	// A long trail keeps its end.
	m.SetSize(20, narrowH)
	h.keys("enter", "enter", "enter")
	if got := crumb(); !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "commands.go") {
		t.Errorf("breadcrumb at 20 columns = %q, want its end", got)
	}
}

func TestNarrowShowsOnePane(t *testing.T) {
	m, h := newModal(t, newFake(), narrowW, narrowH)
	if s := screen(m); strings.Contains(s, "fix/tabs") || !strings.Contains(s, "main: change 1") {
		t.Errorf("narrow screen shows more than the graph:\n%s", s)
	}
	h.keys("enter")
	if s := screen(m); !strings.Contains(s, "commands.go") || strings.Contains(s, "main: change 1") {
		t.Errorf("narrow screen of the commit:\n%s", s)
	}
	// The patch isn't previewed below the files: there is no room.
	if m.pagerShown() {
		t.Error("the pager shows below the files")
	}
}
