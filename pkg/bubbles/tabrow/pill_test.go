package tabrow

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func pillRow(w, cur int) Model {
	acc := lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	mut := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	return row(w, cur, WithPill(PillStyles(acc, mut)))
}

func TestPillFitsAndIsLeftAligned(t *testing.T) {
	m := pillRow(80, 1)
	if m.Height() != 1 {
		t.Fatalf("height %d", m.Height())
	}
	got := m.View()
	plain := ansi.Strip(got)
	var sb strings.Builder
	for _, it := range items {
		sb.WriteString(" " + it.Label + " ")
	}
	if want := sb.String(); plain != want {
		t.Fatalf("got %q, want %q", plain, want)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("more than one line: %q", got)
	}
}

func TestPillNeverWiderThanWidth(t *testing.T) {
	for w := 1; w <= 60; w++ {
		for cur := range items {
			m := pillRow(w, cur)
			if n := ansi.StringWidth(m.View()); n > w {
				t.Fatalf("width %d cur %d: %d wide", w, cur, n)
			}
		}
	}
}

func TestPillNarrowShowsOnlyCurrent(t *testing.T) {
	m := pillRow(len(items[1].Label)+2, 1)
	if got, want := ansi.Strip(m.View()), " "+items[1].Label+" "; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPillCurrentIsSolid(t *testing.T) {
	m := pillRow(80, 0)
	st := m.Styles()
	cur := st.Current.Render(" " + items[0].Label + " ")
	oth := st.Other.Render(" " + items[0].Label + " ")
	if cur == oth {
		t.Fatal("current looks like the others")
	}
	if !strings.HasPrefix(m.View(), cur) {
		t.Fatalf("current tab not first: %q", m.View())
	}
	if !st.Current.GetReverse() || !st.Current.GetBold() {
		t.Fatal("current pill is not reverse and bold")
	}
}

func TestPillAside(t *testing.T) {
	m := pillRow(120, 0)
	m.SetAside("Go")
	got := m.View()
	if ansi.StringWidth(got) != 120 || !strings.HasSuffix(got, "Go") {
		t.Fatalf("aside not at the right edge: %q", got)
	}
	m = pillRow(5, 0)
	m.SetAside("Go")
	if strings.Contains(m.View(), "Go") {
		t.Fatal("aside kept without room")
	}
}
