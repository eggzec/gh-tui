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
	got := m.View()
	if strings.Contains(got, "\n") {
		t.Fatalf("pill row spans lines: %q", got)
	}
	plain := ansi.Strip(got)
	var sb strings.Builder
	for _, it := range items {
		sb.WriteString(" ")
		sb.WriteString(it.Label)
		sb.WriteString(" ")
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

func TestPillShortLabels(t *testing.T) {
	m := pillRow(45, 2)
	if got, want := ansi.Strip(m.View()), " Repos  Stars  Followers  Following  Orgs "; got != want {
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

// The label of the first tab starts in the column where the header's first
// text does, so the pill reaches one cell to the left of it.
func TestPillAlignsWithHeader(t *testing.T) {
	for _, header := range []string{"    #  Title", "\x1b[2m    #\x1b[m", "  # x"} {
		m := pillRow(80, 0)
		m.AlignWith(header)
		got := ansi.Strip(m.View())
		plain := ansi.Strip(header)
		want := len(plain) - len(strings.TrimLeft(plain, " "))
		if col := strings.Index(got, items[0].Label); col != want {
			t.Errorf("header %q: label at %d, want %d (row %q)", header, col, want, got)
		}
	}
	m := pillRow(80, 0)
	m.AlignWith("")
	if got := ansi.Strip(m.View()); !strings.HasPrefix(got, " "+items[0].Label) {
		t.Errorf("blank header moved the row: %q", got)
	}
	m = pillRow(6, 0)
	m.AlignWith("      #")
	if w := ansi.StringWidth(m.View()); w > 6 {
		t.Errorf("indented row is %d wide, over 6", w)
	}
}
