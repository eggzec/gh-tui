package dashboard

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// reposLines returns the headers and rows of the repositories pane without
// styles, and without the tabs.
func reposLines(s *Section) []string {
	b := s.boxes[reposPane]
	if !s.zoom.CanZoom() {
		b = box{s.width, s.height - s.profileHeight()}
	}
	body := s.reposBody(b.w-2, b.h-2)
	out := make([]string, 0, len(body))
	for _, l := range body[1:] {
		out = append(out, ansi.Strip(l))
	}
	return out
}

func TestReposColumnsLineUp(t *testing.T) {
	svc := newFake()
	rs := repos("octocat", 30)
	rs[1].Fork, rs[1].Template = true, true
	rs[2].Description = strings.Repeat("A long description ", 10)
	rs[4].Stars = 12345
	svc.repos["@me"] = rs
	for _, width := range []int{80, 140, 190} {
		s := newSection(t, svc, nil, width, 40, WithIcons(ui.NewIcons(config.IconsASCII)))
		lines := reposLines(s)
		head := lines[0]
		desc := strings.Index(head, "Description")
		lang := strings.Index(head, "Lang")
		end := strings.Index(head, "Updated") + len("Updated")
		if desc < 0 || lang < 0 || end < len("Updated") {
			t.Fatalf("%d columns: the headers are %q", width, head)
		}
		for _, row := range lines[1:] {
			if strings.TrimSpace(row) == "" {
				continue
			}
			// Every cell is a byte, but for the cursor and the ellipsis.
			l := strings.NewReplacer("▌", " ", "…", ".").Replace(row)
			// Every row puts its description under the header, and starts its
			// age where the header starts.
			if l[desc-1] != ' ' || l[desc] == ' ' {
				t.Errorf("%d columns: the description doesn't start under its header:\n%s\n%s", width, head, l)
			}
			if g := l[lang]; l[lang-1] != ' ' || g != 'o' && g != ' ' {
				t.Errorf("%d columns: the language isn't under its header:\n%s\n%s", width, head, l)
			}
			if at := end - len("Updated"); l[at-1] != ' ' || l[at] == ' ' || len(strings.TrimRight(l, " ")) > end {
				t.Errorf("%d columns: the age doesn't start under its header:\n%s\n%s", width, head, l)
			}
		}
		if row := lines[2]; !strings.Contains(row, "F  repo-001  ") {
			t.Errorf("%d columns: the most telling flag, fork, is missing: %q", width, row)
		}
		if row := lines[5]; !strings.Contains(row, "12k") {
			t.Errorf("%d columns: the stars are missing: %q", width, row)
		}
	}
}

func TestReposCountInTheTrim(t *testing.T) {
	svc := newFake()
	svc.repos["@me"] = repos("octocat", 30)
	s := newSection(t, svc, nil, 140, 40)
	if got := s.reposCount(); len(got) != 1 || got[0] != "30" {
		t.Errorf("no total: the forms are %q", got)
	}
	if top := ansi.Strip(s.frames[reposPane][0]); !strings.Contains(top, " 30 ─") || strings.Contains(top, "repos ─") {
		t.Errorf("the count is not at the right of the title: %q", top)
	}

	// The viewer's total comes first, and the loaded count follows in brackets.
	s.header.value.Profile.Repos = 2000
	s.header.ok = true
	if got := s.reposCount(); len(got) != 2 || got[0] != "2k [30]" || got[1] != "2k" {
		t.Errorf("total: the forms are %q", got)
	}

	// Too narrow for the brackets, the title keeps the total.
	s = newSection(t, svc, nil, 24, 40)
	s.header.value.Profile.Repos = 2000
	s.header.ok = true
	s.renderPane(reposPane)
	if top := ansi.Strip(s.frames[reposPane][0]); strings.Contains(top, "[30]") || !strings.Contains(top, "2k") {
		t.Errorf("narrow: only the total should show: %q", top)
	}

	svc.repos["@me"] = nil
	s = newSection(t, svc, nil, 140, 40)
	if got := s.reposCount(); len(got) != 1 || got[0] != "0" {
		t.Errorf("empty: the forms are %q", got)
	}
}

func TestReposCountUnderFilters(t *testing.T) {
	svc := newFake()
	svc.repos["@me"] = repos("octocat", 30)

	// The pane filter lists all it keeps, so its count is the matches of the total.
	s := newSection(t, svc, nil, 140, 40)
	s.header.value.Profile.Repos = 2000
	s.header.ok = true
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "is:private"}))
	n := s.repos.current().Feed.Shown()
	want := strconv.Itoa(n) + "/2k"
	if got := s.reposCount(); len(got) == 0 || got[0] != want {
		t.Errorf("pane filter with a total: the forms are %q, want %q first", got, want)
	}

	// The quick filter keeps some of the loaded rows.
	s = newSection(t, svc, nil, 140, 40)
	press(t, s, "2")
	press(t, s, "&")
	press(t, s, "0", "0", "1", "enter")
	n = s.repos.current().Feed.Shown()
	if n == 0 || n >= 30 {
		t.Fatalf("the quick filter keeps %d of 30", n)
	}
	m := strconv.Itoa(n)
	if got := s.reposCount(); len(got) != 2 || got[0] != m+" [30]" || got[1] != m {
		t.Errorf("quick filter, no total: the forms are %q", got)
	}
	s.header.value.Profile.Repos = 2000
	s.header.ok = true
	if got := s.reposCount(); len(got) != 3 || got[0] != m+"/2k [30]" || got[1] != m+"/2k" || got[2] != m {
		t.Errorf("quick filter, total: the forms are %q", got)
	}
}

func TestReposCursorOnLastRowShows(t *testing.T) {
	svc := newFake()
	svc.repos["@me"] = repos("octocat", 30)
	s := newSection(t, svc, nil, 140, 20, WithIcons(ui.NewIcons(config.IconsASCII)))
	press(t, s, "G")
	lines := reposLines(s)
	if !strings.Contains(lines[0], "Updated") {
		t.Errorf("the header scrolled away: %q", lines[0])
	}
	last := ""
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			last = l
		}
	}
	if !strings.Contains(last, "repo-029") || !strings.HasPrefix(last, ">") {
		t.Errorf("the cursor isn't on the last row, which is %q", last)
	}
}

func TestReposLanguage(t *testing.T) {
	svc := newFake()
	svc.repos["@me"] = []core.Repo{
		{Ref: core.RepoRef{Owner: "octocat", Name: "a"}, Language: "Go"},
		{Ref: core.RepoRef{Owner: "octocat", Name: "b"}, Language: "Jupyter Notebook"},
	}
	s := newSection(t, svc, nil, 140, 38, WithIcons(ui.NewIcons(config.IconsNerd)))
	nerd := ui.NewIcons(config.IconsNerd)
	lines := reposLines(s)
	if !strings.Contains(lines[1], nerd.Language("Go")) || strings.Contains(lines[1], "Go") {
		t.Errorf("the row should show the glyph of Go, not its name: %q", lines[1])
	}
	// The tabs name the language of the repository under the cursor.
	if tabs := ansi.Strip(s.tabsLine(80)); !strings.HasSuffix(tabs, nerd.Language("Go")+" Go") {
		t.Errorf("the tabs end in %q, want the language under the cursor", tabs)
	}
	press(t, s, "down")
	if tabs := ansi.Strip(s.tabsLine(80)); !strings.HasSuffix(tabs, nerd.Language("Jupyter Notebook")+" Jupyter Notebook") {
		t.Errorf("the tabs end in %q, want Jupyter Notebook", tabs)
	}
}
