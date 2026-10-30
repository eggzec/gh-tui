package search

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

func texts(cs []candidate) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Text)
	}
	return out
}

func TestCompleteQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		kind   core.SearchKind
		line   string
		cursor int
		want   []string
	}{
		{"empty", core.SearchIssues, "", 0, nil},
		{"name prefix", core.SearchIssues, "is", 2, []string{"is:"}},
		{"names by prefix", core.SearchIssues, "a", 1, []string{"author:", "assignee:"}},
		{"value prefix", core.SearchIssues, "is:op", 5, []string{"is:open"}},
		{"all values", core.SearchPulls, "is:", 3, []string{"is:open", "is:closed", "is:merged", "is:draft"}},
		{"negated", core.SearchIssues, "-is:cl", 6, []string{"-is:closed"}},
		{"negated name", core.SearchIssues, "-la", 3, []string{"-label:"}},
		{"person", core.SearchIssues, "author:@", 8, []string{"author:@me"}},
		{"review in pulls", core.SearchPulls, "review:ap", 9, []string{"review:approved"}},
		{"not in issues", core.SearchIssues, "review:", 7, nil},
		{"sort", core.SearchRepos, "sort:st", 7, []string{"sort:stars"}},
		{"commit sort", core.SearchCommits, "sort:a", 6, []string{"sort:author-date"}},
		{"language", core.SearchRepos, "language:ru", 11, []string{"language:rust", "language:ruby"}},
		{"case", core.SearchIssues, "IS:OP", 5, []string{"is:open"}},
		{"exact is no candidate", core.SearchIssues, "is:open", 7, nil},
		{"unknown", core.SearchIssues, "zzz", 3, nil},
		{"quoted", core.SearchIssues, `label:"is`, 9, nil},
		{"space in quotes", core.SearchIssues, `label:"good a`, 13, nil},
		{"tab separates", core.SearchIssues, "is:open\tau", 10, []string{"author:"}},
		{"second word", core.SearchIssues, "tea is:op", 9, []string{"is:open"}},
		{"cursor mid word", core.SearchIssues, "is:op tea", 5, []string{"is:open"}},
		{"code has no sort", core.SearchCode, "sort:", 5, nil},
		{"topics have none", core.SearchTopics, "is", 2, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := texts(completeQuery(tt.kind, tt.line, tt.cursor, nil))
			if !slices.Equal(got, tt.want) {
				t.Errorf("complete(%q, %d) = %q, want %q", tt.line, tt.cursor, got, tt.want)
			}
		})
	}
}

func TestCompleteQuerySpan(t *testing.T) {
	t.Parallel()
	cs := completeQuery(core.SearchIssues, "tea is:op more", 9, nil)
	if len(cs) != 1 || cs[0].Start != 4 || cs[0].End != 9 {
		t.Fatalf("span = %+v, want 4..9", cs)
	}
}

type fakeValues struct{ repos, owners []string }

func (f fakeValues) Repos() []string  { return f.repos }
func (f fakeValues) Owners() []string { return f.owners }

func TestCompleteQueryValues(t *testing.T) {
	t.Parallel()
	vals := fakeValues{
		repos:  []string{"octo/hello", "Octo/world", "other/thing", "octo/hello"},
		owners: []string{"octo", "other", "bob"},
	}
	tests := []struct {
		name string
		kind core.SearchKind
		line string
		vals Values
		want []string
	}{
		{"repo all", core.SearchIssues, "repo:", vals, []string{"repo:octo/hello", "repo:Octo/world", "repo:other/thing"}},
		{"repo prefix", core.SearchIssues, "repo:o/", vals, nil},
		{"repo owner prefix", core.SearchIssues, "repo:octo/", vals, []string{"repo:octo/hello", "repo:Octo/world"}},
		{"negated repo", core.SearchIssues, "-repo:oth", vals, []string{"-repo:other/thing"}},
		{"org", core.SearchIssues, "org:o", vals, []string{"org:octo", "org:other"}},
		{"user", core.SearchIssues, "user:b", vals, []string{"user:bob"}},
		{"no provider", core.SearchIssues, "repo:", nil, nil},
		{"unknown qualifier", core.SearchIssues, "zzz:o", vals, nil},
		{"fixed values kept", core.SearchIssues, "is:op", vals, []string{"is:open"}},
		{"kind lacks qualifier", core.SearchTopics, "repo:", vals, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := texts(completeQuery(tt.kind, tt.line, len(tt.line), tt.vals))
			if !slices.Equal(got, tt.want) {
				t.Errorf("complete(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestTabCompletesValue(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 120, 30)
	s.SetValues(fakeValues{repos: []string{"octo/hello"}})
	s.Focus()
	press(t, s, "i")
	typeOnly(t, s, "tea repo:oc")
	press(t, s, "tab")
	if got := s.input.Value(); got != "tea repo:octo/hello" {
		t.Fatalf("after tab, query = %q", got)
	}
}

func TestTabCompletesInQuery(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 120, 30)
	s.Focus()
	press(t, s, "i")
	typeOnly(t, s, "tea is:")
	press(t, s, "tab")
	if got := s.input.Value(); got != "tea is:public" {
		t.Fatalf("after tab, query = %q", got)
	}
	press(t, s, "tab")
	if got := s.input.Value(); got != "tea is:private" {
		t.Fatalf("after 2 tabs, query = %q", got)
	}
	press(t, s, "shift+tab", "shift+tab")
	if got := s.input.Value(); got != "tea is:" {
		t.Fatalf("shift+tab back to the typed line, query = %q", got)
	}
	if !s.typing {
		t.Fatal("tab left the query")
	}
}

func TestTabSwitchesPaneOutsideQuery(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 120, 30)
	s.Focus()
	press(t, s, "tab")
	if s.area != resultsArea || s.typing {
		t.Fatalf("tab outside the query: area %v typing %v", s.area, s.typing)
	}
}

func TestCompleteRowFits80(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 80, 22)
	s.Focus()
	press(t, s, "i")
	typeOnly(t, s, "s")
	view := s.View()
	lines := strings.Split(view, "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "stars:") || !strings.Contains(last, "sort:") {
		t.Fatalf("candidate row missing: %q", last)
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > 80 {
			t.Fatalf("line %d cells wide: %q", w, l)
		}
	}
	if row := s.completeRow(12); ansi.StringWidth(row) > 12 {
		t.Fatalf("row %q wider than 12", row)
	}
}
