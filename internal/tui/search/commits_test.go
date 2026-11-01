package search

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// commitsSection returns a page over commits, on the Commits kind with the
// results focused, and the query not yet submitted.
func commitsSection(t *testing.T) (*Section, *fakeService) {
	t.Helper()
	f := newFake()
	repo := core.RepoRef{Owner: "acme", Name: "tea"}
	f.commits = []core.SearchCommit{
		{Repo: repo, SHA: "0123456789abcdef", Subject: "tea brewing fix", Author: "ana"},
		{Repo: repo, SHA: "fedcba9876543210", Subject: "tea leaves", Author: "bo"},
	}
	s := newSection(t, f, 100, 24)
	press(t, s, "esc")
	press(t, s, strconv.Itoa(slices.Index(kinds, core.SearchCommits)+1))
	return s, f
}

func TestCommitsKindLoadsOnSubmit(t *testing.T) {
	t.Parallel()
	s, f := commitsSection(t)
	typeOnly(t, s, "tea")
	if len(f.commitQueries) != 0 {
		t.Fatalf("searched commits %v before the query was submitted", f.commitQueries)
	}
	press(t, s, "enter")
	if len(f.commitQueries) != 1 || f.commitQueries[0].Text != "tea" {
		t.Fatalf("searched commits with %+v, want one query for tea", f.commitQueries)
	}
}

func TestCommitRow(t *testing.T) {
	t.Parallel()
	s, _ := commitsSection(t)
	typeText(t, s, "tea")
	var row string
	for line := range strings.Lines(s.View()) {
		if strings.Contains(line, "tea brewing fix") {
			row = ansi.Strip(line)
		}
	}
	if !strings.Contains(row, s.icons.Glyph(ui.GlyphCommit)) || !strings.Contains(row, "0123456") || strings.Contains(row, "0123456789") {
		t.Errorf("row of a commit = %q, want the glyph and a 7-character SHA", row)
	}
}

func TestCommitEnterOpensCommit(t *testing.T) {
	t.Parallel()
	s, _ := commitsSection(t)
	typeText(t, s, "tea")
	press(t, s, "down")
	app := press(t, s, "enter")
	if len(app) != 1 {
		t.Fatalf("enter sent %v, want one message", app)
	}
	got, ok := app[0].(ui.OpenCommitMsg)
	if !ok || got.SHA != "fedcba9876543210" || got.Repo.String() != "acme/tea" {
		t.Errorf("enter sent %#v, want the second commit opened", app[0])
	}
}

// A commit hit that names no repository, or half of one, opens nothing.
func TestCommitEnterWithoutRepoOpensNothing(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		repo core.RepoRef
	}{
		{"no repository", core.RepoRef{}},
		{"no owner", core.RepoRef{Name: "tea"}},
		{"no name", core.RepoRef{Owner: "acme"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, f := commitsSection(t)
			f.commits = []core.SearchCommit{{Repo: tt.repo, SHA: "0123456789abcdef", Subject: "tea brewing fix"}}
			typeText(t, s, "tea")
			for _, m := range press(t, s, "enter") {
				if _, ok := m.(ui.OpenCommitMsg); ok {
					t.Errorf("enter sent %#v, want no commit opened", m)
				}
			}
		})
	}
}

// An issue or pull request hit whose repository the search could not read
// from its URL opens nothing.
func TestIssueAndPullEnterWithoutRepoOpensNothing(t *testing.T) {
	t.Parallel()
	for _, kind := range []core.SearchKind{core.SearchIssues, core.SearchPulls} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			f := newFake()
			hit := core.SearchHit{Kind: kind, Issue: core.Issue{Number: 7, Title: "tea brewing fix"}}
			if kind == core.SearchIssues {
				f.issues = []core.SearchHit{hit}
			} else {
				f.pulls = []core.SearchHit{hit}
			}
			s := newSection(t, f, 100, 24)
			press(t, s, "esc")
			press(t, s, strconv.Itoa(slices.Index(kinds, kind)+1))
			typeText(t, s, "tea")
			for _, m := range press(t, s, "enter") {
				switch m.(type) {
				case ui.OpenIssueMsg, ui.OpenPullMsg:
					t.Errorf("enter sent %#v, want nothing opened", m)
				}
			}
		})
	}
}

func TestCommitsFilterSpec(t *testing.T) {
	t.Parallel()
	sp := spec(core.SearchCommits, "")
	keys := make([]string, 0, len(sp.Fields))
	for _, f := range sp.Fields {
		keys = append(keys, f.Key)
	}
	for _, want := range []string{"author", "repo", "org", "user"} {
		if !slices.Contains(keys, want) {
			t.Errorf("fields %v lack %s", keys, want)
		}
	}
	if sp.Sort == nil || len(sp.Sort.Options) != 3 {
		t.Fatalf("sort = %+v, want best match, authored and committed", sp.Sort)
	}
	values := make([]string, 0, len(sp.Sort.Options))
	for _, o := range sp.Sort.Options {
		values = append(values, o.Value)
	}
	if want := []string{"", "author-date", "committer-date"}; !slices.Equal(values, want) {
		t.Errorf("sort values = %q, want %q", values, want)
	}
}

func TestCommitsNoCount(t *testing.T) {
	t.Parallel()
	s, _ := commitsSection(t)
	typeText(t, s, "tea")
	view := s.View()
	if !strings.Contains(view, "tea brewing fix") {
		t.Fatalf("the commits are missing:\n%s", view)
	}
	for _, re := range []string{`Commits\W*\d`, `\d+ results?\b`} {
		if m := regexp.MustCompile(re).FindString(view); m != "" {
			t.Errorf("view shows a count for the commits: %q", m)
		}
	}
}
