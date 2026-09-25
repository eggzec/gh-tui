package actions

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		keys          []string
	}{
		{"190 columns", wideW, wideH, nil},
		{"190 columns jobs", wideW, wideH, []string{"tab"}},
		{"190 columns running", wideW, wideH, []string{"j"}},
		{"190 columns zoom", wideW, wideH, []string{"tab", "tab", "z"}},
		{"190 columns filter", wideW, wideH, []string{"f"}},
		{"190 columns confirm", wideW, wideH, []string{"ctrl+r"}},
		{"190 columns failing", wideW, wideH, []string{"]"}},
		{"80 columns runs", narrowW, narrowH, nil},
		{"80 columns jobs", narrowW, narrowH, []string{"enter"}},
		{"80 columns log", narrowW, narrowH, []string{"enter", "enter"}},
		{"80 columns running", narrowW, narrowH, []string{"j", "enter", "enter"}},
		{"80 columns filter", narrowW, narrowH, []string{"f"}},
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
	for _, size := range [][2]int{{1, 1}, {10, 3}, {30, 5}, {109, 12}, {110, 12}, {250, 60}} {
		for _, keys := range [][]string{nil, {"tab"}, {"tab", "tab"}, {"j", "tab", "tab"}, {"z"}, {"f"}, {"ctrl+r"}} {
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
	steps := []struct {
		keys []string
		want string
	}{
		{nil, "Runs"},
		{[]string{"enter"}, "Runs › CI #4812"},
		{[]string{"enter"}, "Runs › CI #4812 › test (ubuntu-latest, 1.26)"},
		{[]string{"esc"}, "Runs › CI #4812"},
		{[]string{"esc"}, "Runs"},
	}
	for _, s := range steps {
		h.keys(s.keys...)
		if got := crumb(); got != s.want {
			t.Errorf("after %q the breadcrumb is %q, want %q", s.keys, got, s.want)
		}
	}
	// A long trail keeps its end.
	m.SetSize(30, narrowH)
	h.keys("enter", "enter")
	if got := crumb(); !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "1.26)") {
		t.Errorf("breadcrumb at 30 columns = %q, want its end", got)
	}
}

func TestNarrowShowsOnePane(t *testing.T) {
	m, h := newModal(t, newFake(), narrowW, narrowH)
	if s := screen(m); strings.Contains(s, "test (macos") || !strings.Contains(s, "CI #4812") {
		t.Errorf("the narrow modal shows more than the runs:\n%s", s)
	}
	h.keys("enter")
	if s := screen(m); !strings.Contains(s, "test (macos") || strings.Contains(s, "docs: add authors") {
		t.Errorf("the narrow jobs:\n%s", s)
	}
}

func TestNarrowBreadcrumbNamesTheTab(t *testing.T) {
	m, h := newModal(t, newFake(), narrowW, narrowH)
	crumb := func() string { return strings.TrimSpace(ansi.Strip(strings.Split(m.View(), "\n")[0])) }
	h.keys("]")
	if got := crumb(); got != "Runs · Failing" {
		t.Errorf("the Failing tab's breadcrumb is %q", got)
	}
	h.keys("[")
	m.filter.Branch = "main"
	if got := crumb(); got != "Runs · filtered" {
		t.Errorf("a filter's breadcrumb is %q", got)
	}
}
