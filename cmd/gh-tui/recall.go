package main

import (
	"cmp"
	"slices"

	"github.com/eggzec/gh-tui/internal/core"
	dashsvc "github.com/eggzec/gh-tui/internal/service/dashboard"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	pullsvc "github.com/eggzec/gh-tui/internal/service/pulls"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
	"github.com/eggzec/gh-tui/internal/tui"
)

// recall is what the command line completes from: the lists the services
// hold in memory, which the dashboard, the search page and the panes
// read. It never does I/O.
type recall struct {
	// pinned are the repositories of the config, and here that of the
	// current directory, if any.
	pinned []core.RepoRef
	here   core.RepoRef
	dash   interface {
		CachedHeader() (core.Header, bool)
		CachedRepos(q dashsvc.ReposQuery) (core.Page[core.Repo], bool)
	}
	repos interface {
		CachedList(q reposvc.ListQuery) (core.Page[core.Repo], bool)
	}
	pulls interface {
		CachedList(q pullsvc.ListQuery) (core.Page[core.PullRequest], bool)
	}
	issues interface {
		CachedList(q issuesvc.ListQuery) (core.Page[core.Issue], bool)
	}
}

var _ tui.Recall = recall{}

// Repos returns the repository of the current directory and those pinned,
// then the viewer's own on the dashboard, then those the search page
// starts with, each most recently updated first. It may repeat one.
func (r recall) Repos() []core.RepoRef {
	out := make([]core.RepoRef, 0, 1+len(r.pinned))
	if r.here != (core.RepoRef{}) {
		out = append(out, r.here)
	}
	out = append(out, r.pinned...)
	if p, ok := r.dash.CachedRepos(dashsvc.ReposQuery{Viewer: true}); ok {
		for i := range p.Items {
			out = append(out, p.Items[i].Ref)
		}
	}
	if p, ok := r.repos.CachedList(reposvc.ListQuery{}); ok {
		for i := range p.Items {
			out = append(out, p.Items[i].Ref)
		}
	}
	return out
}

// Numbers returns the open pull requests and issues of repo on the first
// pages that its panes show, the latest first.
func (r recall) Numbers(repo core.RepoRef) []tui.Numbered {
	var out []tui.Numbered
	if p, ok := r.pulls.CachedList(pullsvc.ListQuery{Repo: repo, State: core.StateOpen}); ok {
		for i := range p.Items {
			out = append(out, tui.Numbered{Number: p.Items[i].Number, Title: p.Items[i].Title})
		}
	}
	if p, ok := r.issues.CachedList(issuesvc.ListQuery{Repo: repo, State: core.FilterOpen}); ok {
		for i := range p.Items {
			out = append(out, tui.Numbered{Number: p.Items[i].Number, Title: p.Items[i].Title})
		}
	}
	slices.SortFunc(out, func(a, b tui.Numbered) int { return cmp.Compare(b.Number, a.Number) })
	return slices.CompactFunc(out, func(a, b tui.Numbered) bool { return a.Number == b.Number })
}

// Owners returns the viewer's organizations, as the dashboard read them,
// then the owners of the repositories Repos returns. It may repeat one.
func (r recall) Owners() []string {
	var out []string
	if h, ok := r.dash.CachedHeader(); ok {
		for _, o := range h.Orgs {
			out = append(out, o.Login)
		}
	}
	for _, repo := range r.Repos() {
		out = append(out, repo.Owner)
	}
	return out
}
