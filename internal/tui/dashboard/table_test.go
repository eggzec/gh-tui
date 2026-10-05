package dashboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// reposLines returns the headers and rows of the repositories pane without
// styles, and without the tabs.
func reposLines(s *Section) []string {
	b := s.boxes[reposPane]
	if !s.wide {
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
			// Every row puts its description under the header, and ends its
			// age where the header ends.
			if l[desc-1] != ' ' || l[desc] == ' ' {
				t.Errorf("%d columns: the description doesn't start under its header:\n%s\n%s", width, head, l)
			}
			if g := l[lang]; l[lang-1] != ' ' || g != 'o' && g != ' ' {
				t.Errorf("%d columns: the language isn't under its header:\n%s\n%s", width, head, l)
			}
			if l[end-1] == ' ' || strings.TrimRight(l, " ") != l[:end] {
				t.Errorf("%d columns: the age doesn't end under its header:\n%s\n%s", width, head, l)
			}
		}
		if row := lines[2]; !strings.Contains(row, "repo-001  F T  ") {
			t.Errorf("%d columns: the fork and template flags are missing: %q", width, row)
		}
		if row := lines[5]; !strings.Contains(row, "12k") {
			t.Errorf("%d columns: the stars are missing: %q", width, row)
		}
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
