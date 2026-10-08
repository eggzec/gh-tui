package search

import (
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// TestFilterSpecs checks that the form of each kind opens on a query and
// writes it back: its fields in order, then the sort, then the words.
func TestFilterSpecs(t *testing.T) {
	tests := []struct {
		kind         core.SearchKind
		query, wrote string
	}{
		{core.SearchRepos, "tea", "tea"},
		{
			core.SearchRepos,
			"tea sort:stars-desc user:charmbracelet is:public archived:false fork:true stars:>1000 language:go",
			"language:go stars:>1000 fork:true archived:false is:public user:charmbracelet sort:stars-desc tea",
		},
		{core.SearchRepos, "fork:only language:zig sort:updated-asc", "language:zig fork:only sort:updated-asc"},
		// What the fields don't take stays as it was typed.
		{core.SearchRepos, "tea stars:10..50 topic:tui", "tea stars:10..50 topic:tui"},
		{
			core.SearchIssues,
			`render org:charmbracelet repo:charmbracelet/bubbletea label:"good first issue" assignee:@me author:meowgorithm is:open sort:comments-desc`,
			`is:open author:meowgorithm assignee:@me label:"good first issue" repo:charmbracelet/bubbletea org:charmbracelet sort:comments-desc render`,
		},
		{
			core.SearchPulls,
			"review-requested:@me is:draft is:merged sort:created-asc",
			"is:merged is:draft review-requested:@me sort:created-asc",
		},
		// Drafts and reviews are for pull requests only.
		{core.SearchIssues, "is:draft review:approved", "is:draft review:approved"},
		{core.SearchCode, "NewCmd path:pkg/cmd org:cli repo:cli/cli language:go", "language:go repo:cli/cli org:cli path:pkg/cmd NewCmd"},
		// Code search takes no sort.
		{core.SearchCode, "tea sort:indexed-desc", "tea sort:indexed-desc"},
	}
	for _, tt := range tests {
		form := filterform.New(spec(tt.kind, tt.query), filterform.WithQuery(tt.query))
		if got := form.Query(); got != tt.wrote {
			t.Errorf("%s: the form of %q writes\n%q, want\n%q", tt.kind, tt.query, got, tt.wrote)
		}
	}
}

// TestFilterSorts checks that choosing a sort of repositories sorts the
// way GitHub's results are best read: most first.
func TestFilterSorts(t *testing.T) {
	form := filterform.New(spec(core.SearchRepos, "tea"), filterform.WithQuery("tea"), filterform.WithTab(filterform.SortTab),
		filterform.WithKeyMap(ui.FilterFormKeys(config.Default().Keys, "filter")))
	form.Focus()
	form, _ = form.Update(keyPress("right"))
	if got := form.Query(); got != "sort:stars-desc tea" {
		t.Errorf("choosing the stars wrote %q, want them descending", got)
	}
}

func TestFilterIsPerKind(t *testing.T) {
	s := newSection(t, newFake(), 120, 30)
	typeText(t, s, "tea language:go")
	keys := func() []string {
		f, ok := s.Filter()
		if !ok {
			t.Fatal("the page should have a filter")
		}
		if f.Query != "tea language:go" {
			t.Errorf("the filter opens on %q, want the query", f.Query)
		}
		out := make([]string, 0, len(f.Spec.Fields))
		for _, fl := range f.Spec.Fields {
			out = append(out, fl.Key)
		}
		return out
	}
	if got := keys(); !slices.Equal(got, []string{"language", "stars", "forks", "archived", "visibility", "owner"}) {
		t.Errorf("the repositories filter by %v", got)
	}
	press(t, s, "esc", "]", "]")
	if got := keys(); !slices.Contains(got, "review") || !slices.Contains(got, "draft") {
		t.Errorf("the pull requests filter by %v, want reviews and drafts", got)
	}
	if f, _ := s.Filter(); f.Subject != "Pull requests" {
		t.Errorf("the filter names %q, want the pull requests", f.Subject)
	}
}

func TestApplyFilterSearchesOnce(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30)
	typeText(t, s, "tea")
	press(t, s, "esc")
	before, _ := svc.stats()
	prefetches := svc.prefetches

	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "language:go sort:stars-desc tea"}))
	if s.input.Value() != "language:go sort:stars-desc tea" || s.Query() != "language:go sort:stars-desc tea" {
		t.Errorf("the query is %q, and the results are for %q; want the filter's", s.input.Value(), s.Query())
	}
	n, code := svc.stats()
	if n-before != 1 || svc.prefetches-prefetches != 2 || code != 0 {
		t.Errorf("applying made %d searches, %d prefetches and %d code searches; want one search, as enter does",
			n-before, svc.prefetches-prefetches, code)
	}
	if last := svc.queries[len(svc.queries)-1]; last.Text != "language:go sort:stars-desc tea" || last.Kind != core.SearchRepos {
		t.Errorf("searched %+v, want the repositories of the filter's query", last)
	}
	if s.area != resultsArea || s.recent[0] != "language:go sort:stars-desc tea" {
		t.Error("applying should show the results and remember the search")
	}

	// Applying the query in force searches nothing.
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "language:go sort:stars-desc tea"}))
	if n2, _ := svc.stats(); n2 != n {
		t.Error("applying the same query searched again")
	}
}

func TestApplyFilterSearchesCode(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30)
	typeText(t, s, "tea")
	press(t, s, "esc", "]", "]", "]")
	_, before := svc.stats()
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "language:go tea"}))
	if _, code := svc.stats(); code != before+1 {
		t.Errorf("applying on the code made %d code searches, want one", code-before)
	}
	if !strings.Contains(flat(s), "language:go tea") {
		t.Errorf("the query box should show the filter's query:\n%s", screen(s))
	}
}
