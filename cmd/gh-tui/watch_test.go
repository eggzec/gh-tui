package main

import (
	"context"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/watch"
)

// subscriptions is a fake watch.Engine.Subscribe that logs what happens,
// and runs each subscribed poll once so the test can tell them apart.
type subscriptions struct {
	log []string
}

func (s *subscriptions) subscribe(key string, fn watch.PollFunc) func() {
	if _, err := fn(context.Background()); err != nil {
		s.log = append(s.log, "poll failed: "+err.Error())
	}
	s.log = append(s.log, "+"+key)
	return func() { s.log = append(s.log, "-"+key) }
}

func (s *subscriptions) take() []string {
	log := s.log
	s.log = nil
	return log
}

func TestRepoWatchSwitchesOver(t *testing.T) {
	var polled []string
	poll := func(kind string) func(core.RepoRef) watch.PollFunc {
		return func(repo core.RepoRef) watch.PollFunc {
			return func(context.Context) (watch.Result, error) {
				polled = append(polled, kind+" "+repo.String())
				return watch.Result{}, nil
			}
		}
	}
	key := func(kind string) func(core.RepoRef) string {
		return func(repo core.RepoRef) string { return kind + ":" + repo.String() }
	}
	subs := &subscriptions{}
	w := &repoWatch{subscribe: subs.subscribe, polls: []repoPoll{
		{key("pulls"), poll("pulls")},
		{key("issues"), poll("issues")},
	}}
	a := core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	b := core.RepoRef{Owner: "cli", Name: "cli"}

	steps := []struct {
		name string
		repo core.RepoRef
		want []string
	}{
		{"nothing selected", core.RepoRef{}, nil},
		{"first repository", a, []string{"+pulls:eggzec/gh-tui", "+issues:eggzec/gh-tui"}},
		{"same repository", a, nil},
		{"another repository", b, []string{"-pulls:eggzec/gh-tui", "-issues:eggzec/gh-tui", "+pulls:cli/cli", "+issues:cli/cli"}},
		{"none", core.RepoRef{}, []string{"-pulls:cli/cli", "-issues:cli/cli"}},
		{"back again", a, []string{"+pulls:eggzec/gh-tui", "+issues:eggzec/gh-tui"}},
	}
	for _, st := range steps {
		w.set(st.repo)
		if got := subs.take(); !slices.Equal(got, st.want) {
			t.Errorf("%s: %q, want %q", st.name, got, st.want)
		}
	}
	want := []string{"pulls eggzec/gh-tui", "issues eggzec/gh-tui", "pulls cli/cli", "issues cli/cli", "pulls eggzec/gh-tui", "issues eggzec/gh-tui"}
	if !slices.Equal(polled, want) {
		t.Errorf("subscribed polls = %q, want %q", polled, want)
	}
}

func TestIssuePollSkipsReposWithoutIssues(t *testing.T) {
	on := core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	off := core.RepoRef{Owner: "laraibg786", Name: "slk"}
	unknown := core.RepoRef{Owner: "cli", Name: "cli"}
	cached := func(ref core.RepoRef) (core.Repo, bool) {
		switch ref {
		case on:
			return core.Repo{Caps: core.RepoCaps{Known: true, Issues: true}}, true
		case off:
			return core.Repo{Caps: core.RepoCaps{Known: true}}, true
		}
		return core.Repo{}, false
	}
	var polled []core.RepoRef
	poll := unless(issuesOff(cached), func(repo core.RepoRef) watch.PollFunc {
		return func(context.Context) (watch.Result, error) {
			polled = append(polled, repo)
			return watch.Result{Changed: true}, nil
		}
	})
	for _, repo := range []core.RepoRef{on, off, unknown} {
		res, err := poll(repo)(t.Context())
		if err != nil || res.Changed == (repo == off) {
			t.Errorf("poll of %v = %+v, %v", repo, res, err)
		}
	}
	if !slices.Equal(polled, []core.RepoRef{on, unknown}) {
		t.Errorf("polled %v, want all but %v", polled, off)
	}
}
