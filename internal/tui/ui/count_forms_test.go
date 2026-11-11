package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// TestTitleBesideKeepsCount draws the top edge with the title cut to what
// TitleBeside returns, and checks that the shortest count is still in it.
func TestTitleBesideKeepsCount(t *testing.T) {
	st := panes.DefaultStyles(true)
	title := []string{"Pull requests in the repository", "Pull requests", "PRs"}
	count := []string{"1.2k[1234]", "1.2k"}
	for w := 12; w <= 60; w++ {
		got := TitleBeside(title, count, w-5, "…")
		top, _ := st.Edges(panes.Content{TopLeft: got, TopRight: count}, w)
		line := ansi.Strip(top)
		if ansi.StringWidth(line) != w {
			t.Fatalf("w=%d: top edge is %d wide: %q", w, ansi.StringWidth(line), line)
		}
		if !strings.Contains(line, "1.2k") {
			t.Fatalf("w=%d: count lost from %q (title forms %q)", w, line, got)
		}
	}
}

func TestCountForms(t *testing.T) {
	cases := []struct {
		name string
		spec CountSpec
		want []string
	}{
		{"empty", CountSpec{}, []string{"0[0]", "0"}},
		{"unknown total", CountSpec{Loaded: 30, More: true}, []string{"?[30]", "?"}},
		{"total beside loaded", CountSpec{Total: 2000, Loaded: 20, More: true}, []string{"2k[20]", "2k"}},
		{"loaded equals total", CountSpec{Total: 23, Loaded: 23}, []string{"23[23]", "23"}},
		{"total below loaded", CountSpec{Total: 5, Loaded: 30, More: true}, []string{"?[30]", "?"}},
		{"loaded 1000 is exact", CountSpec{Total: 5000, Loaded: 1000, More: true}, []string{"5k[1000]", "5k"}},
		{"loaded 1234 is exact", CountSpec{Loaded: 1234}, []string{"1.2k[1234]", "1.2k"}},
		{"finished list outgrew its total", CountSpec{Total: 5, Loaded: 30}, []string{"30[30]", "30"}},
		{"filter unknown total", CountSpec{Loaded: 30, Matched: 7, More: true, Filtered: true}, []string{"7/?[30]", "7/?", "7"}},
		{"filter loaded equals total", CountSpec{Total: 30, Loaded: 30, Matched: 30, Filtered: true}, []string{"30/30[30]", "30/30", "30"}},
		{"filter total", CountSpec{Total: 2000, Loaded: 20, Matched: 3, Filtered: true}, []string{"3/2k[20]", "3/2k", "3"}},
		{"filter zero matched", CountSpec{Total: 40, Loaded: 20, Matched: 0, More: true, Filtered: true}, []string{"0/40[20]", "0/40", "0"}},
		{"filter matched 1500 exact", CountSpec{Total: 9000, Loaded: 1500, Matched: 1500, Filtered: true}, []string{"1500/9k[1500]", "1500/9k", "1500"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CountForms(c.spec); !slices.Equal(got, c.want) {
				t.Fatalf("CountForms(%+v) = %q, want %q", c.spec, got, c.want)
			}
		})
	}
}
