package tabrow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func boxRow(w, cur int) Model {
	return row(w, cur, WithBox(lipgloss.RoundedBorder()))
}

func lines(s string) []string { return strings.Split(s, "\n") }

func TestBoxWidthNeverExceeded(t *testing.T) {
	for _, aside := range []string{"", "Go"} {
		for w := 1; w <= 90; w++ {
			for cur := range items {
				m := boxRow(w, cur)
				m.SetAside(aside)
				got := lines(m.View())
				if len(got) != 3 {
					t.Fatalf("width %d cur %d: %d lines", w, cur, len(got))
				}
				for i, l := range got {
					if n := ansi.StringWidth(l); n != w {
						t.Fatalf("aside %q width %d cur %d line %d: %d wide: %q", aside, w, cur, i, n, l)
					}
				}
			}
		}
	}
}

func TestBoxASCII(t *testing.T) {
	m := row(30, 1, WithBox(lipgloss.ASCIIBorder()))
	got := m.View()
	for _, r := range got {
		if r > 127 && r != '\n' && r != '‹' && r != '›' {
			t.Fatalf("non-ASCII %q in %q", r, got)
		}
	}
	if !strings.Contains(got, "+") || !strings.Contains(got, "| Stars |") {
		t.Errorf("ASCII box missing: \n%s", got)
	}
}

func TestBoxAside(t *testing.T) {
	m := boxRow(80, 0)
	m.SetAside("Go")
	mid := lines(m.View())[1]
	if !strings.HasSuffix(mid, "Go") {
		t.Errorf("aside not at the right end: %q", mid)
	}
	// Too narrow for the aside beside even the scrolled tab: dropped.
	m = boxRow(20, 0)
	m.SetAside("Go Language")
	if strings.Contains(m.View(), "Go Language") {
		t.Errorf("aside kept in a narrow row:\n%s", m.View())
	}
}

func TestBoxScrollKeepsCurrent(t *testing.T) {
	for cur, it := range items {
		short := it.Short
		if short == "" {
			short = it.Label
		}
		for w := 24; w <= 90; w++ {
			got := boxRow(w, cur).View()
			if !strings.Contains(got, "│ "+short+" │") && !strings.Contains(got, "│ "+it.Label+" │") {
				t.Errorf("width %d cur %d: box lacks the tab:\n%s", w, cur, got)
			}
		}
	}
}

func TestBoxHeight(t *testing.T) {
	if boxRow(40, 0).Height() != 3 || row(40, 0).Height() != 1 {
		t.Error("heights are 3 boxed and 1 flat")
	}
}

func TestBoxGolden(t *testing.T) {
	var b strings.Builder
	for _, w := range []int{80, 40} {
		for _, cur := range []int{0, 2, 4} {
			fmt.Fprintf(&b, "width %d, tab %d\n", w, cur)
			m := boxRow(w, cur)
			m.SetAside("Go")
			b.WriteString(m.View())
			b.WriteString("\n\n")
		}
	}
	path := filepath.Join("testdata", "box.golden")
	if *update {
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if b.String() != string(want) {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func BenchmarkViewBox(b *testing.B) {
	m := New(WithBox(lipgloss.RoundedBorder()))
	m.SetItems(items, 2)
	m.SetWidth(40)
	m.SetAside("Go")
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}
