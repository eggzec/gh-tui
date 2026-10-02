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

func TestRepoRowLines(t *testing.T) {
	s := newSection(t, flagged(), 190, 22, WithIcons(ui.NewIcons(config.IconsASCII)))
	archived := flagged().repos[2]
	first, second, _ := strings.Cut(ansi.Strip(s.renderHit(archived, false, 80)), "\n")
	for _, want := range []string{"octo/tea-archived A", "o Jupyter...", "* 250k", " 1y"} {
		if !strings.Contains(first, want) {
			t.Errorf("first line %q, want %q in it", first, want)
		}
	}
	if !strings.HasPrefix(second, "  Kept for history") || !strings.HasSuffix(strings.TrimRight(second, " "), "...") {
		t.Errorf("second line %q, want only the description, cut", second)
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
