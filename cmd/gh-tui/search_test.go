package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
	searchsvc "github.com/eggzec/gh-tui/internal/service/search"
	"github.com/eggzec/gh-tui/internal/tui"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

type fakeSearcher struct {
	got  []searchsvc.Query
	hits []core.SearchHit
	err  error
}

func (f *fakeSearcher) Search(_ context.Context, q searchsvc.Query) (searchsvc.Result, error) {
	f.got = append(f.got, q)
	var r searchsvc.Result
	r.Items = f.hits
	return r, f.err
}

type fakeLister struct {
	repos []core.Repo
	err   error
}

func (f *fakeLister) List(context.Context, reposvc.ListQuery) (core.Page[core.Repo], error) {
	return core.Page[core.Repo]{Items: f.repos}, f.err
}

var (
	ghTUI     = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	bubbletea = core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
)

func titles(items []picker.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Kind + ": " + it.Title
	}
	return out
}

func TestSearchEmptyQueryListsPinnedThenOwn(t *testing.T) {
	s := &fakeSearcher{}
	repos := &fakeLister{repos: []core.Repo{{Ref: ghTUI}, {Ref: core.RepoRef{Owner: "eggzec", Name: "dotfiles"}}}}
	search := searchItems(s, repos, []core.RepoRef{ghTUI, bubbletea})

	items, err := search(t.Context(), picker.Query{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Pinned: eggzec/gh-tui", "Pinned: charmbracelet/bubbletea", "Repositories: eggzec/dotfiles"}
	if got := titles(items); !slices.Equal(got, want) {
		t.Errorf("items = %q, want %q", got, want)
	}
	if items[0].Value != ghTUI {
		t.Errorf("pinned value = %#v, want the ref", items[0].Value)
	}
	if len(s.got) != 0 {
		t.Error("an empty query searched GitHub")
	}

	items, err = search(t.Context(), picker.Query{Scope: tui.KindIssues})
	if err != nil || len(items) != 0 {
		t.Errorf("empty issue query = %v, %v; want nothing", items, err)
	}
}

func TestSearchMapsScopesAndHits(t *testing.T) {
	issue := core.Issue{Repo: ghTUI, Number: 12, Title: "tabs overflow"}
	s := &fakeSearcher{hits: []core.SearchHit{
		{Kind: core.SearchRepos, Repo: core.Repo{Ref: bubbletea, Description: "TUI framework"}},
		{Kind: core.SearchIssues, Issue: issue},
		{Kind: core.SearchPulls, Issue: core.Issue{Repo: ghTUI, Number: 37, Title: "watch"}},
	}}
	search := searchItems(s, &fakeLister{}, nil)

	for scope, kind := range map[string]core.SearchKind{
		"": core.SearchAll, tui.KindRepos: core.SearchRepos, tui.KindIssues: core.SearchIssues, tui.KindPulls: core.SearchPulls,
	} {
		s.got = nil
		items, err := search(t.Context(), picker.Query{Text: "tui", Scope: scope})
		if err != nil {
			t.Fatal(err)
		}
		if len(s.got) != 1 || s.got[0].Kind != kind || s.got[0].Text != "tui" {
			t.Errorf("scope %q searched %+v, want kind %q", scope, s.got, kind)
		}
		want := []string{"Repositories: charmbracelet/bubbletea", "Issues: tabs overflow", "Pull requests: watch"}
		if got := titles(items); !slices.Equal(got, want) {
			t.Errorf("items = %q, want %q", got, want)
		}
		if items[1].Detail != "eggzec/gh-tui#12" {
			t.Errorf("issue detail = %q", items[1].Detail)
		}
		if hit, ok := items[1].Value.(core.SearchHit); !ok || hit.Issue.Number != 12 {
			t.Errorf("issue value = %#v, want the hit", items[1].Value)
		}
	}
}

func TestSearchRewordsRateLimits(t *testing.T) {
	reset := time.Date(2026, 9, 24, 15, 4, 0, 0, time.Local)
	tests := []struct {
		err  error
		want string
	}{
		{&core.RateLimitError{Reset: reset}, "try again at 3:04PM"},
		{core.ErrRateLimited, "try again in a minute"},
		{errors.New("boom"), "boom"},
	}
	for _, tt := range tests {
		search := searchItems(&fakeSearcher{err: tt.err}, &fakeLister{}, nil)
		_, err := search(t.Context(), picker.Query{Text: "x"})
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("error for %v = %v, want it to say %q", tt.err, err, tt.want)
		}
	}

	search := searchItems(&fakeSearcher{}, &fakeLister{err: core.ErrRateLimited}, nil)
	if _, err := search(t.Context(), picker.Query{}); err == nil || !strings.Contains(err.Error(), "slow down") {
		t.Errorf("list error = %v, want the friendly one", err)
	}
}
