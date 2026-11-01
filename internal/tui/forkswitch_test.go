package tui

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// TestForkSwitchKey checks that the key goes to the parent of a fork, else to
// the fork of the viewer, and says so when there is none.
func TestForkSwitchKey(t *testing.T) {
	t.Parallel()
	parent := core.RepoRef{Owner: "upstream", Name: "gh-tui"}
	mine := core.RepoRef{Owner: "me", Name: "gh-tui"}
	known := func(r core.Repo) core.Repo {
		r.Ref, r.Caps.Known = testRepo, true
		return r
	}
	tests := []struct {
		name  string
		repo  core.Repo
		want  core.RepoRef
		toast string
	}{
		{name: "fork", repo: known(core.Repo{Fork: true, Parent: parent}), want: parent},
		{name: "forked", repo: known(core.Repo{ViewerFork: mine}), want: mine},
		{name: "both", repo: known(core.Repo{Fork: true, Parent: parent, ViewerFork: mine}), want: parent},
		{name: "neither", repo: known(core.Repo{}), want: testRepo, toast: "eggzec/gh-tui has no fork or parent to go to."},
		{name: "unread", repo: core.Repo{Ref: testRepo}, want: testRepo, toast: "Still reading eggzec/gh-tui, so press the key again in a moment."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repos := &fakeRepos{
				cached: map[core.RepoRef]core.Repo{testRepo: tt.repo},
				remote: map[core.RepoRef]core.Repo{parent: {Ref: parent}, mine: {Ref: mine}},
			}
			m, _ := newGotoApp(t, repos, WithRepo(testRepo))
			tap(t, m, "U")
			if m.screen != repoScreen || m.repo != tt.want {
				t.Errorf("screen %d with %v, want the repository screen of %v", m.screen, m.repo, tt.want)
			}
			if tt.toast != "" && !hasToast(m, tt.toast) {
				t.Errorf("toasts lack %q: %s", tt.toast, toasted(m))
			}
			if tt.toast == "" && !m.canGoBack() {
				t.Error("the back stack is empty after the switch")
			}
		})
	}
}
