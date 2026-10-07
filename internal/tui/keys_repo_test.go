package tui

import (
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// newRepoKeyApp returns an app on the dashboard, whose cursor is on a row
// of sel, with the current directory in the repository testRepo.
func newRepoKeyApp(t *testing.T, keys config.Keymap, sel ui.Selection, ok bool) *Model {
	t.Helper()
	cfg := config.Default()
	cfg.Keys = keys
	dash := &selectSection{fakeSection: &fakeSection{title: ui.DashboardTitle}, sel: sel, ok: ok}
	layout := Layout{Files: &fakeSection{title: "Files"}, Dashboard: dash}
	m := New(t.Context(), cfg, layout, WithHere(testRepo))
	m.toast.SetDuration(0)
	m.toast.SetErrorDuration(0)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	run(m, m.Init())
	return m
}

// TestRepoKeyOpensTheRepositoryOfTheRow checks that the repo key opens the
// repository of the row the cursor is on, not that of the current
// directory.
func TestRepoKeyOpensTheRepositoryOfTheRow(t *testing.T) {
	m := newRepoKeyApp(t, config.Default().Keys, ui.RepoSelection(core.Repo{Ref: bubbletea}, ""), true)
	if !m.keys.state(m).Repo.Enabled() {
		t.Error("the repo key is off on a row with a repository")
	}
	run(m, m.key(press(".")))
	if m.screen != repoScreen || m.repo != bubbletea {
		t.Errorf("screen %d with %v, want the repository screen of %v", m.screen, m.repo, bubbletea)
	}
}

// TestRepoKeyIsOffWithoutARepository checks that the repo key does nothing,
// and that help shows it off, where the selection has no repository or
// nothing is selected, and where it is the repository on view.
func TestRepoKeyIsOffWithoutARepository(t *testing.T) {
	for name, tt := range map[string]struct {
		sel ui.Selection
		ok  bool
	}{
		"nothing selected": {},
		"no repository":    {sel: ui.Selection{What: "file", Owner: "mona"}, ok: true},
	} {
		t.Run(name, func(t *testing.T) {
			m := newRepoKeyApp(t, config.Default().Keys, tt.sel, tt.ok)
			if m.keys.state(m).Repo.Enabled() {
				t.Error("the repo key is on")
			}
			run(m, m.key(press(".")))
			if m.screen != dashScreen {
				t.Errorf("screen = %d, want the dashboard still", m.screen)
			}
		})
	}

	t.Run("on view", func(t *testing.T) {
		m := newSelectApp(t, ui.RepoSelection(core.Repo{Ref: testRepo}, ""), true)
		if m.keys.state(m).Repo.Enabled() {
			t.Error("the repo key is on for the repository on view")
		}
	})
}

// TestRepoKeyFollowsTheConfig checks that the repo key can be unbound or
// bound to another key.
func TestRepoKeyFollowsTheConfig(t *testing.T) {
	sel := ui.RepoSelection(core.Repo{Ref: bubbletea}, "")

	keys := config.Default().Keys
	keys.Set(config.ActionRepo, nil)
	m := newRepoKeyApp(t, keys, sel, true)
	run(m, m.key(press(".")))
	if m.screen != dashScreen || m.keys.state(m).Repo.Enabled() {
		t.Errorf("screen %d with the key unbound, want the dashboard still and the key off", m.screen)
	}

	keys = config.Default().Keys
	keys.Set(config.ActionRepo, []string{"ctrl+r"})
	m = newRepoKeyApp(t, keys, sel, true)
	run(m, m.key(press(".")))
	if m.screen != dashScreen {
		t.Errorf("the old key opened screen %d", m.screen)
	}
	run(m, m.key(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}))
	if m.screen != repoScreen || m.repo != bubbletea {
		t.Errorf("screen %d with %v, want the repository screen of %v", m.screen, m.repo, bubbletea)
	}
}

// TestRepoKeyOnASearchResult checks that the repo key opens the repository
// of the result the cursor is on.
func TestRepoKeyOnASearchResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, false)
		pressKeys(t, m, "S", "k", "e", "y", "enter")
		if got := focusOf(m); got != `search: results "key"` {
			t.Fatalf("the keys reach %s, want the results", got)
		}
		pressKeys(t, m, ".")
		if m.screen != repoScreen || m.repo != testRepo {
			t.Errorf("screen %d with %v, want the repository screen of %v", m.screen, m.repo, testRepo)
		}
	})
}

// TestSearchKeyClearsTheQueryOnThePage checks that the search key, pressed
// with the search page on view, starts a new query.
func TestSearchKeyClearsTheQueryOnThePage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, false)
		pressKeys(t, m, "S", "k", "e", "y", "enter")
		if got := focusOf(m); got != `search: results "key"` {
			t.Fatalf("the keys reach %s, want the results", got)
		}
		pressKeys(t, m, "S")
		if got := focusOf(m); got != `search: query ""` {
			t.Errorf("after the search key the page is at %s, want an empty query", got)
		}
	})
}

// TestGotoHere checks that goto . opens the repository of the current
// directory, and says so when there is none.
func TestGotoHere(t *testing.T) {
	m, _ := newGotoApp(t, newGotoRepos(), WithHere(testRepo))
	runCommand(t, m, "goto .")
	if m.screen != repoScreen || m.repo != testRepo {
		t.Errorf("screen %d with %v, want the repository screen of %v", m.screen, m.repo, testRepo)
	}

	m, _ = newGotoApp(t, newGotoRepos())
	runCommand(t, m, "goto .")
	if m.screen != dashScreen || !hasToast(m, "No repository in the current directory.") {
		t.Errorf("screen %d, toasts %s; want the dashboard and the reason", m.screen, toasted(m))
	}
}

// TestCompleteHere checks that goto completes . when there is a
// repository in the current directory.
func TestCompleteHere(t *testing.T) {
	m, _ := newGotoApp(t, newGotoRepos(), WithHere(testRepo))
	got := m.completeTarget(".", 1, 1, true)
	if len(got) != 1 || got[0].Text != "." {
		t.Errorf("completions of . are %v, want just .", got)
	}
	m, _ = newGotoApp(t, newGotoRepos())
	if got := m.completeTarget(".", 1, 1, true); len(got) != 0 {
		t.Errorf("completions of . are %v without a repository here, want none", got)
	}
}
