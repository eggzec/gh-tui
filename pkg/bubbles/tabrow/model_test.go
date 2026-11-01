package tabrow

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var update = flag.Bool("update", false, "rewrite the golden files")

var items = []Item{
	{Label: "Repositories", Short: "Repos", Count: "12"},
	{Label: "Stars", Count: "3"},
	{Label: "Followers"},
	{Label: "Following"},
	{Label: "Organizations", Short: "Orgs"},
}

// plain gives styles that add no escapes, so rows compare as text.
func plain() Styles { return Styles{} }

func row(w, cur int, opts ...Option) Model {
	m := New(append([]Option{WithStyles(plain())}, opts...)...)
	m.SetItems(items, cur)
	m.SetWidth(w)
	return m
}

func TestLadder(t *testing.T) {
	tests := []struct {
		width int
		want  string
	}{
		{80, "Repositories 12  Stars 3  Followers  Following  Organizations"},
		{55, "Repos 12  Stars 3  Followers  Following  Orgs"},
		{44, "Repos 12 Stars 3 Followers Following Orgs"},
		{20, "[1] 2 3 4 5"},
		{8, "[1] 2 3 "},
	}
	for _, tt := range tests {
		got := row(tt.width, 0).View()
		if got != tt.want {
			t.Errorf("width %d: got %q, want %q", tt.width, got, tt.want)
		}
	}
}

func TestWidthNeverExceeded(t *testing.T) {
	for _, scroll := range []bool{false, true} {
		var opts []Option
		if scroll {
			opts = append(opts, WithMarkers("<", ">"))
		}
		for w := 1; w <= 90; w++ {
			for cur := range items {
				if got := ansi.StringWidth(row(w, cur, opts...).View()); got > w {
					t.Fatalf("scroll=%v width %d cur %d: row is %d wide", scroll, w, cur, got)
				}
			}
		}
	}
}

func TestScrollKeepsCurrent(t *testing.T) {
	for cur, it := range items {
		name := it.Label
		if it.Count != "" {
			name += " " + it.Count
		}
		// The current name plus the markers that surround it must fit.
		need := ansi.StringWidth(name)
		if cur > 0 {
			need += 3
		}
		if cur < len(items)-1 {
			need += 3
		}
		for w := need; w <= 90; w++ {
			got := row(w, cur, WithMarkers("<", ">")).View()
			if !strings.Contains(got, name) {
				t.Errorf("width %d cur %d: %q lacks %q", w, cur, got, name)
			}
		}
	}
}

func TestCurrentAndOther(t *testing.T) {
	cur := lipgloss.NewStyle().Underline(true)
	oth := lipgloss.NewStyle().Italic(true)
	m := New(WithStyles(Styles{Current: cur, Other: oth}))
	m.SetItems(items, 1)
	m.SetWidth(200)
	got := m.View()
	for _, want := range []string{oth.Render("Repositories 12"), cur.Render("Stars 3"), oth.Render("Followers")} {
		if !strings.Contains(got, want) {
			t.Errorf("row %q lacks %q", got, want)
		}
	}
}

func TestMarkers(t *testing.T) {
	// Off, a narrow row steps down the ladder; on, it scrolls to the current.
	off := row(30, 4).View()
	if strings.Contains(off, "<") || strings.Contains(off, ">") {
		t.Errorf("without markers: %q", off)
	}
	on := row(30, 4, WithMarkers("<", ">")).View()
	if !strings.Contains(on, "Organizations") || !strings.HasPrefix(on, "<") {
		t.Errorf("scrolled to the last: %q", on)
	}
	first := row(30, 0, WithMarkers("<", ">")).View()
	if !strings.Contains(first, "Repositories 12") || !strings.HasSuffix(first, ">") || strings.HasPrefix(first, "<") {
		t.Errorf("at the first: %q", first)
	}
}

func TestGolden(t *testing.T) {
	var b strings.Builder
	for _, w := range []int{80, 55, 44, 20, 8} {
		b.WriteString(row(w, 1).View())
		b.WriteString("\n")
	}
	b.WriteString(row(30, 2, WithMarkers("<", ">")).View())
	b.WriteString("\n")
	path := filepath.Join("testdata", "ladder.golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
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

func BenchmarkView(b *testing.B) {
	m := New()
	m.SetItems(items, 2)
	m.SetWidth(40)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}
