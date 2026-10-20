package search

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// flagged is a fake whose repositories are of every kind: forks, private,
// archived, templates and mirrors, with and without a language, a
// description or an age.
func flagged() *fakeService {
	f := newFake()
	fork := repo("octo", "tea-fork", "A fork of a terminal tool", "Go", 12, 5*24*time.Hour)
	fork.Repo.Fork = true
	private := repo("octo", "tea-private", "", "TypeScript", 1_234, 2*time.Hour)
	private.Repo.Private = true
	archived := repo("octo", "tea-archived", "Kept for history, and described at some length so that the description needs cutting", "Jupyter Notebook", 250_000, 400*24*time.Hour)
	archived.Repo.Archived = true
	everything := repo("octo", "tea-with-a-rather-long-name-of-its-own", "Every mark at once", "", 7, 0)
	everything.Repo.Fork, everything.Repo.Private, everything.Repo.Archived = true, true, true
	everything.Repo.Template, everything.Repo.Mirror = true, true
	template := repo("octo", "tea-template", "Start a terminal tool", "Rust", 999, 40*24*time.Hour)
	template.Repo.Template = true
	template.Repo.UpdatedAt = time.Time{}
	mirror := repo("octo", "tea-mirror", "A mirror", "C", 10_500, time.Minute)
	mirror.Repo.Mirror = true
	f.repos = []core.SearchHit{fork, private, archived, everything, template, mirror}
	return f
}

func TestRepoRows(t *testing.T) {
	for _, set := range []string{config.IconsNerd, config.IconsUnicode, config.IconsASCII} {
		for _, size := range [][2]int{{80, 22}, {140, 22}, {190, 22}} {
			t.Run(fmt.Sprintf("%s %d columns", set, size[0]), func(t *testing.T) {
				s := newSection(t, flagged(), size[0], size[1], WithIcons(ui.NewIcons(set)))
				typeText(t, s, "tea")
				golden.RequireEqual(t, s.View())
			})
		}
	}
}

// TestRepoRowsAlign checks that the language, stars and age line up from
// row to row, at every width a row can have.
func TestRepoRowsAlign(t *testing.T) {
	s := newSection(t, flagged(), 190, 22, WithIcons(ui.NewIcons(config.IconsASCII)))
	stars := regexp.MustCompile(`\* [0-9.km]+`)
	repos := flagged().repos
	for width := 30; width <= 190; width++ {
		ends := make([]int, 0, len(repos))
		for _, hit := range repos {
			row := s.renderHit(hit, false, width)
			first, second, _ := strings.Cut(row, "\n")
			if w, w2 := ansi.StringWidth(first), ansi.StringWidth(second); w != width || w2 != width {
				t.Fatalf("width %d: lines of %d and %d cells", width, w, w2)
			}
			plain := ansi.Strip(first)
			loc := stars.FindAllStringIndex(plain, -1)
			ends = append(ends, ansi.StringWidth(plain[:loc[len(loc)-1][1]]))
		}
		for _, e := range ends[1:] {
			if e != ends[0] {
				t.Fatalf("width %d: the stars end at %v, want one column", width, ends)
			}
		}
	}
}

// TestRepoHeader checks that the titles head the columns of the rows, drop
// with them at narrow widths, and that the ages start in one column.
func TestRepoHeader(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		width int
		want  []string
		not   []string
	}{
		{190, []string{"Name", "Language", "*", "Updated"}, nil},
		{80, []string{"Name", "Language", "Updated"}, nil},
		{44, []string{"Name"}, []string{"Language"}},
	} {
		s := newSection(t, flagged(), tc.width, 22, WithIcons(ui.NewIcons(config.IconsASCII)))
		typeText(t, s, "tea")
		view := ansi.Strip(s.View())
		if n := len(strings.Split(view, "\n")); n != 22 {
			t.Errorf("width %d: %d lines, want 22", tc.width, n)
		}
		var header string
		for l := range strings.SplitSeq(view, "\n") {
			if strings.Contains(l, "Name") {
				header = l
			}
		}
		if header == "" {
			t.Fatalf("width %d: no header in\n%s", tc.width, view)
		}
		for _, w := range tc.want {
			if !strings.Contains(header, w) {
				t.Errorf("width %d: header %q lacks %q", tc.width, header, w)
			}
		}
		for _, w := range tc.not {
			if strings.Contains(header, w) {
				t.Errorf("width %d: header %q has %q", tc.width, header, w)
			}
		}
	}

	// The age is left-aligned: "5d" and "2h" and "1y" all start where the
	// title does.
	s := newSection(t, flagged(), 190, 22, WithIcons(ui.NewIcons(config.IconsASCII)))
	typeText(t, s, "tea")
	var header string
	starts := map[string]int{}
	for l := range strings.SplitSeq(ansi.Strip(s.View()), "\n") {
		if strings.Contains(l, "Name") {
			header = l
		}
	}
	before, _, ok := strings.Cut(header, updatedTitle)
	if !ok {
		t.Fatalf("no %q in header %q", updatedTitle, header)
	}
	col := ansi.StringWidth(before)
	for l := range strings.SplitSeq(ansi.Strip(s.View()), "\n") {
		for _, age := range []string{"2h", "1y"} {
			if strings.Contains(l, "/tea-private") && age == "2h" || strings.Contains(l, "/tea-archived") && age == "1y" {
				j := strings.LastIndex(l, age)
				if j < 0 {
					t.Fatalf("no %q in %q", age, l)
				}
				starts[age] = ansi.StringWidth(l[:j])
			}
		}
	}
	if len(starts) != 2 {
		t.Fatalf("found ages %v, want 2h and 1y", starts)
	}
	for age, st := range starts {
		if st != col {
			t.Errorf("age %s starts at column %d, want %d", age, st, col)
		}
	}
}

func TestHitHeader(t *testing.T) {
	t.Parallel()
	for _, width := range []int{80, 40} {
		s := newSection(t, newFake(), width, 22)
		typeText(t, s, "tea")
		press(t, s, "esc")
		for range kinds {
			if s.Kind() == core.SearchIssues {
				break
			}
			press(t, s, "]")
		}
		if s.Kind() != core.SearchIssues {
			t.Fatalf("width %d: kind is %v, want issues", width, s.Kind())
		}
		if !strings.Contains(ansi.Strip(s.View()), "Item") {
			t.Errorf("width %d: no header over the issues", width)
		}
	}
}

func TestRepoRowLines(t *testing.T) {
	s := newSection(t, flagged(), 190, 22, WithIcons(ui.NewIcons(config.IconsASCII)))
	archived := flagged().repos[2]
	first, second, _ := strings.Cut(ansi.Strip(s.renderHit(archived, false, 80)), "\n")
	for _, want := range []string{"A octo/tea-archived", "o Jupyter...", "* 250k", " 1y"} {
		if !strings.Contains(first, want) {
			t.Errorf("first line %q, want %q in it", first, want)
		}
	}
	if !strings.HasPrefix(first, "A octo/tea-archived") {
		t.Errorf("first line %q, want the flags before the name", first)
	}
	// The description is cut in the middle, so its start and end both show.
	if trimmed := strings.TrimRight(second, " "); !strings.HasPrefix(second, "  Kept for history") ||
		!strings.Contains(trimmed, "...") || !strings.HasSuffix(trimmed, "needs cutting") {
		t.Errorf("second line %q, want only the description, cut in the middle", second)
	}
	if strings.Contains(second, "Jupyter") || strings.Contains(second, "1y") {
		t.Errorf("second line %q, want the language and age on the first", second)
	}
}

// However narrow the row, the number of an issue shows whole; the
// repository before it is cut first.
func TestHitKeepsTheNumber(t *testing.T) {
	s := newSection(t, newFake(), 80, 22)
	hit := issue(core.SearchIssues, "charmbracelet/bubbletea", 1203, "Terminal tea renders twice after resize", core.StateOpen, false)
	for _, width := range []int{30, 40, 56, 80, 120} {
		first, _, _ := strings.Cut(ansi.Strip(s.renderHit(hit, false, width)), "\n")
		if !strings.Contains(first, "#1203") {
			t.Errorf("at %d cells the row is %q, want the number in it", width, first)
		}
		if w := ansi.StringWidth(first); w != width {
			t.Errorf("at %d cells the row is %d wide", width, w)
		}
	}
}

// Lines of a fragment without text don't take the rows of a result.
func TestFragmentViewSkipsBlankLines(t *testing.T) {
	text := "package tea\n\n// Program is a terminal user interface.\n   \ntype Program struct {\n"
	f := core.Fragment{Text: text, Matches: [][2]int{{8, 11}}}
	got := make([]string, 0, 3)
	for _, l := range fragmentView(f, 3) {
		got = append(got, l.text)
	}
	want := []string{"package tea", "// Program is a terminal user interface.", "type Program struct {"}
	if !slices.Equal(got, want) {
		t.Errorf("fragmentView = %q, want %q", got, want)
	}
}

// A file takes its path and the lines that matched, with no blank row
// after them when they fill the result.
func TestCodeHitIsCompact(t *testing.T) {
	s := newSection(t, newFake(), 80, 22)
	hit := core.CodeHit{
		Repo: core.RepoRef{Owner: "cli", Name: "cli"}, Path: "tea.go",
		Fragments: []core.Fragment{{Text: "one tea\ntwo\nthree\nfour", Matches: [][2]int{{4, 7}}}},
	}
	lines := strings.Split(ansi.Strip(s.renderCode(hit, false, 40)), "\n")
	want := []string{"cli/cli · tea.go", "  one tea", "  two", "  three"}
	if len(lines) != len(want) {
		t.Fatalf("renderCode = %q, want %d lines", lines, len(want))
	}
	for i, l := range lines {
		if strings.TrimRight(l, " ") != want[i] || ansi.StringWidth(l) != 40 {
			t.Errorf("line %d = %q, want %q in 40 cells", i, l, want[i])
		}
	}
}
