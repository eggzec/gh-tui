package dashboard

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

func TestParseRepoFilter(t *testing.T) {
	tests := []struct {
		query string
		want  repoFilter
	}{
		{"", repoFilter{}},
		{"is:private", repoFilter{visibility: "private"}},
		{"is:Public fork:false archived:false", repoFilter{visibility: "public", forks: "false", archived: "false"}},
		{"template:true language:Go", repoFilter{templates: true, language: "go"}},
		{"is:template fork:only", repoFilter{templates: true, forks: "only"}},
		{`language:"jupyter notebook"`, repoFilter{language: "jupyter notebook"}},
		{"sort:stars-desc", repoFilter{sort: filterform.Sort{By: "stars", Desc: true}}},
		{"sort:name-asc", repoFilter{sort: filterform.Sort{By: "name"}}},
		{"sort:updated", repoFilter{sort: filterform.Sort{By: "updated", Desc: true}}},
		{"sort:forks-desc", repoFilter{}},
		{"gh tui topic:cli", repoFilter{words: []string{"gh", "tui"}}},
	}
	for _, tt := range tests {
		got := parseRepoFilter(tt.query)
		tt.want.query = tt.query
		if got.query != tt.want.query || got.visibility != tt.want.visibility || got.forks != tt.want.forks ||
			got.archived != tt.want.archived || got.templates != tt.want.templates || got.language != tt.want.language ||
			got.sort != tt.want.sort || !slices.Equal(got.words, tt.want.words) {
			t.Errorf("parseRepoFilter(%q) = %+v, want %+v", tt.query, got, tt.want)
		}
	}
}

// filterRepos is a small set of repositories with one of each kind.
func filterRepos() []core.Repo {
	r := func(name, lang string, stars int) core.Repo {
		return core.Repo{Ref: core.RepoRef{Owner: "octocat", Name: name}, Language: lang, Stars: stars}
	}
	all := []core.Repo{
		r("gh-tui", "Go", 40),
		r("dotfiles", "Shell", 3),
		r("secret", "Go", 1),
		r("fork-of-cli", "Go", 0),
		r("old-site", "HTML", 12),
		r("template-go", "Go", 7),
	}
	all[2].Private = true
	all[3].Fork = true
	all[4].Archived = true
	all[5].Template = true
	for i := range all {
		all[i].UpdatedAt = now.Add(-time.Duration(i) * time.Hour)
	}
	return all
}

func repoNames(repos []core.Repo) []string {
	out := make([]string, len(repos))
	for i := range repos {
		out[i] = repos[i].Ref.Name
	}
	return out
}

func TestRepoFilterApply(t *testing.T) {
	tests := []struct {
		query string
		want  []string
	}{
		{"", []string{"gh-tui", "dotfiles", "secret", "fork-of-cli", "old-site", "template-go"}},
		{"is:private", []string{"secret"}},
		{"is:public language:go", []string{"gh-tui", "fork-of-cli", "template-go"}},
		{"fork:false archived:false", []string{"gh-tui", "dotfiles", "secret", "template-go"}},
		{"fork:only", []string{"fork-of-cli"}},
		{"archived:true", []string{"old-site"}},
		{"template:true", []string{"template-go"}},
		{"language:go sort:stars-desc", []string{"gh-tui", "template-go", "secret", "fork-of-cli"}},
		{"sort:stars-asc language:go", []string{"fork-of-cli", "secret", "template-go", "gh-tui"}},
		{"sort:name-asc archived:false fork:false", []string{"dotfiles", "gh-tui", "secret", "template-go"}},
		{"sort:updated-asc is:public", []string{"template-go", "old-site", "fork-of-cli", "dotfiles", "gh-tui"}},
		// Words match names fuzzily, every word, the best matches first.
		{"ght", []string{"gh-tui"}},
		{"o t", []string{"old-site", "template-go", "dotfiles"}},
		{"fork:true", []string{"gh-tui", "dotfiles", "secret", "fork-of-cli", "old-site", "template-go"}},
		{"go is:private", []string{}},
		// Tokens the filter doesn't read keep everything.
		{"topic:cli", []string{"gh-tui", "dotfiles", "secret", "fork-of-cli", "old-site", "template-go"}},
	}
	for _, tt := range tests {
		f := parseRepoFilter(tt.query)
		if got := repoNames(f.apply(filterRepos())); !slices.Equal(got, tt.want) {
			t.Errorf("filter %q keeps %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestRepoFilterChips(t *testing.T) {
	tests := []struct{ query, want string }{
		{"", ""},
		{"is:private fork:false archived:false template:true language:go sort:stars-desc",
			"private · no forks · no archived · templates · go · stars ↓"},
		{"gh tui sort:name-asc", `"gh tui" · name ↑`},
		{"topic:cli", "topic:cli"},
	}
	for _, tt := range tests {
		f := parseRepoFilter(tt.query)
		if got := f.chips(); got != tt.want {
			t.Errorf("chips of %q = %q, want %q", tt.query, got, tt.want)
		}
	}
}

// TestFilterSpec checks that the form of the filter opens on a query and
// writes it back, and that its fields write what the filter reads.
func TestFilterSpec(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38)
	f, ok := s.Filter()
	if !ok {
		t.Fatal("the repositories should have a filter")
	}
	if f.Subject != "Repositories" || f.Query != "" {
		t.Errorf("Filter = %q on %q, want the repositories unfiltered", f.Subject, f.Query)
	}
	langs := make([]string, 0, 4)
	for _, it := range f.Spec.Fields[5].Options {
		langs = append(langs, it.Label)
	}
	// The first page of 100 is read: 25 of each language, and 25 with
	// none, which isn't offered.
	if want := []string{"Any", "Go", "Rust", "TypeScript"}; !slices.Equal(langs, want) {
		t.Errorf("the languages offered are %v, want %v", langs, want)
	}
	for _, q := range []string{
		"repo is:private fork:false archived:false template:true language:rust sort:stars-desc",
		"is:public sort:name-asc",
		"language:go sort:updated-desc",
	} {
		form := filterform.New(f.Spec, filterform.WithQuery(q))
		if got := form.Query(); got != q {
			t.Errorf("the form of %q writes %q", q, got)
		}
	}
	// The default sort writes nothing once applied.
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "language:go sort:updated-desc"}))
	if got := s.repos.filter().query; got != "language:go" {
		t.Errorf("the filter in force is %q, want the default sort left out", got)
	}
	// A language no repository read has is offered once it filters.
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "language:zig"}))
	f, _ = s.Filter()
	if !slices.ContainsFunc(f.Spec.Fields[5].Options, func(it filterform.Item) bool { return it.Value == "zig" }) {
		t.Error("the language of the filter should be offered")
	}

	press(t, s, "3")
	if _, ok := s.Filter(); ok {
		t.Error("only the repositories should have a filter")
	}
}

func TestFilterLists(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, nil, 140, 38)
	calls := len(svc.calls)
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "is:private language:go sort:stars-desc"}))

	// The filter runs over every repository: the first page is cached, and
	// the other two are read once.
	if got := svc.calls[calls:]; !slices.Equal(got, []string{"repos @me@100", "repos @me@200"}) {
		t.Errorf("the filter read %v, want the two pages the list hadn't", got)
	}
	o := s.repos.current()
	pf := parseRepoFilter("is:private language:go")
	want := pf.apply(repos("octocat", 250))
	slices.SortStableFunc(want, func(a, b core.Repo) int { return b.Stars - a.Stars })
	if o.feed.Len() != len(want) || len(want) == 0 {
		t.Fatalf("the list holds %d repositories, want %d", o.feed.Len(), len(want))
	}
	for i := range want {
		if r, _ := o.feed.Item(i); r.Ref != want[i].Ref || !r.Private || r.Language != "Go" {
			t.Fatalf("row %d is %v, want %v", i, r.Ref, want[i].Ref)
		}
	}
	if view := screen(s); !strings.Contains(view, "Repositories · private · go · stars ↓") {
		t.Errorf("the pane title should name the filter:\n%s", view)
	}

	// Applying it again reads nothing, and switching tabs filters them too.
	calls = len(svc.calls)
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "is:private language:go sort:stars-desc"}))
	if len(svc.calls) != calls {
		t.Errorf("the same filter read %v", svc.calls[calls:])
	}
	press(t, s, "]")
	if n := s.repos.current().feed.Len(); n != 0 {
		t.Errorf("github lists %d repositories, want none: it has no private Go one", n)
	}
	if view := screen(s); !strings.Contains(view, "No repository of github matches the filter.") {
		t.Errorf("an empty filtered list should say so:\n%s", view)
	}

	// F clears the filter of every tab.
	press(t, s, "F")
	if s.repos.filter().active() || s.repos.current().feed.Len() != 5 {
		t.Errorf("F should clear the filter; github lists %d", s.repos.current().feed.Len())
	}
	press(t, s, "[")
	if o := s.repos.current(); o.feed.Len() != 100 || o.feed.Done() {
		t.Errorf("yours list %d repositories after F, want the first page of the list", o.feed.Len())
	}
	if strings.Contains(screen(s), "stars ↓") {
		t.Error("the chips should go with the filter")
	}
}

func TestFilterIsCapped(t *testing.T) {
	svc := newFake()
	svc.repos["@me"] = repos("octocat", 1500)
	s := newSection(t, svc, nil, 140, 38)
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "sort:name-asc"}))
	if got := s.repos.current().feed.Len(); got != 1000 {
		t.Errorf("the filter lists %d repositories, want the cap of 1000", got)
	}
	if n := svc.count("repos @me@1000"); n != 0 {
		t.Error("the filter should stop reading at the cap")
	}
}

func TestFilterFailure(t *testing.T) {
	svc := newFake()
	svc.fail["repos @me@100"] = errors.New("github: 502 Bad Gateway")
	s := newSection(t, svc, nil, 140, 38)
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "is:private"}))
	if err := s.repos.current().feed.Err(); err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("the list's error is %v, want the failed page", err)
	}
	// Once the page reads, a retry lists what the filter keeps.
	delete(svc.fail, "repos @me@100")
	press(t, s, "r")
	if got := s.repos.current().feed.Len(); got != 50 {
		t.Errorf("the filter lists %d repositories after the retry, want the 50 private ones", got)
	}
}
