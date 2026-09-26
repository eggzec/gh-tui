package main

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	dashsvc "github.com/eggzec/gh-tui/internal/service/dashboard"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	pullsvc "github.com/eggzec/gh-tui/internal/service/pulls"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
	"github.com/eggzec/gh-tui/internal/tui"
)

// cachedDash holds the first page of the viewer's own repositories.
type cachedDash []core.Repo

func (c cachedDash) CachedRepos(q dashsvc.ReposQuery) (core.Page[core.Repo], bool) {
	return core.Page[core.Repo]{Items: c}, c != nil && q.Viewer && q.Cursor == ""
}

type cachedRepos []core.Repo

func (c cachedRepos) CachedList(q reposvc.ListQuery) (core.Page[core.Repo], bool) {
	return core.Page[core.Repo]{Items: c}, c != nil && q.Cursor == ""
}

type cachedPulls map[core.RepoRef][]core.PullRequest

func (c cachedPulls) CachedList(q pullsvc.ListQuery) (core.Page[core.PullRequest], bool) {
	if q.State != core.StateOpen || q.Cursor != "" || q.Filter != "" {
		return core.Page[core.PullRequest]{}, false
	}
	items, ok := c[q.Repo]
	return core.Page[core.PullRequest]{Items: items}, ok
}

type cachedIssues map[core.RepoRef][]core.Issue

func (c cachedIssues) CachedList(q issuesvc.ListQuery) (core.Page[core.Issue], bool) {
	if q.State != core.FilterOpen || q.Cursor != "" || q.Filter != "" {
		return core.Page[core.Issue]{}, false
	}
	items, ok := c[q.Repo]
	return core.Page[core.Issue]{Items: items}, ok
}

func ref(owner, name string) core.RepoRef { return core.RepoRef{Owner: owner, Name: name} }

func TestRecallRepos(t *testing.T) {
	r := recall{
		pinned: []core.RepoRef{ref("cli", "cli")},
		here:   ref("eggzec", "gh-tui"),
		dash:   cachedDash{{Ref: ref("eggzec", "dotfiles")}},
		repos:  cachedRepos{{Ref: ref("eggzec", "gh-tui")}, {Ref: ref("eggzec", "site")}},
	}
	want := []core.RepoRef{ref("eggzec", "gh-tui"), ref("cli", "cli"), ref("eggzec", "dotfiles"), ref("eggzec", "gh-tui"), ref("eggzec", "site")}
	if got := r.Repos(); !slices.Equal(got, want) {
		t.Errorf("Repos = %v, want %v", got, want)
	}
	empty := recall{dash: cachedDash(nil), repos: cachedRepos(nil)}
	if got := empty.Repos(); len(got) != 0 {
		t.Errorf("Repos with nothing cached = %v, want none", got)
	}
}

func TestRecallNumbers(t *testing.T) {
	repo := ref("eggzec", "gh-tui")
	issue := func(n int, title string) core.Issue { return core.Issue{Repo: repo, Number: n, Title: title} }
	r := recall{
		pulls:  cachedPulls{repo: {{Issue: issue(131, "Command mode")}, {Issue: issue(12, "Old")}}},
		issues: cachedIssues{repo: {issue(130, "Parser"), issue(12, "Old")}},
	}
	want := []tui.Numbered{{Number: 131, Title: "Command mode"}, {Number: 130, Title: "Parser"}, {Number: 12, Title: "Old"}}
	if got := r.Numbers(repo); !slices.Equal(got, want) {
		t.Errorf("Numbers = %v, want %v", got, want)
	}
	if got := r.Numbers(ref("cli", "cli")); len(got) != 0 {
		t.Errorf("Numbers of a repository with nothing cached = %v, want none", got)
	}
}
