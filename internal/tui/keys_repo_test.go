package tui

import (
	"slices"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	searchpage "github.com/eggzec/gh-tui/internal/tui/search"
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
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	run(m, m.Init())
	return m
}

// TestRepoKeyOpensTheRepositoryOfTheRow checks that the repo key opens the
// repository of the row the cursor is on, not that of the current
// directory.
func TestRepoKeyOpensTheRepositoryOfTheRow(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestRepoKeyOverSearchClosesFirst checks that the repo key in the results
// of the search closes it, then shows the repository of the hit.
func TestRepoKeyOverSearchClosesFirst(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, false)
		pressKeys(t, m, "S", "i", "k", "e", "y", "enter")
		if got := focusOf(m); got != `search: results "key"` {
			t.Fatalf("the keys reach %s, want the results", got)
		}
		pressKeys(t, m, ".")
		if m.searchHost() != nil || m.modals.Top() != nil {
			t.Errorf(". left the search open: top modal %v", m.modals.Top())
		}
		if m.screen != repoScreen {
			t.Errorf(". left screen %d, want the repository's", m.screen)
		}
		if hasToast(m, "Close") {
			t.Errorf("toasts %s, want no refusal", toasted(m))
		}
	})
}

// TestOwnerKeyOverSearchClosesFirst checks the same for the owner key, which
// shows the owner page of the hit.
func TestOwnerKeyOverSearchClosesFirst(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, false)
		pressKeys(t, m, "S", "i", "k", "e", "y", "enter")
		pressKeys(t, m, "@")
		if m.searchHost() != nil || m.modals.Top() != nil {
			t.Errorf("@ left the search open: top modal %v", m.modals.Top())
		}
		if m.screen != ownerScreen || m.ownerLogin == "" {
			t.Errorf("@ left screen %d on owner %q, want an owner page", m.screen, m.ownerLogin)
		}
		if hasToast(m, "Close") {
			t.Errorf("toasts %s, want no refusal", toasted(m))
		}
	})
}

// TestSearchKeyOverTheSearchChangesNothing checks that the search key,
// pressed with the search modal on view, opens no second search and keeps
// the query and its results.
func TestSearchKeyOverTheSearchChangesNothing(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, false)
		pressKeys(t, m, "S", "i", "k", "e", "y", "enter")
		if got := focusOf(m); got != `search: results "key"` {
			t.Fatalf("the keys reach %s, want the results", got)
		}
		pressKeys(t, m, "S")
		if got := focusOf(m); got != `search: results "key"` {
			t.Errorf("after the search key the page is at %s, want the results kept", got)
		}
	})
}

// TestGotoHere checks that goto . opens the repository of the current
// directory, and says so when there is none.
func TestGotoHere(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	m, _ := newGotoApp(t, newGotoRepos(), WithHere(testRepo))
	got := m.completeTarget(".", 1, 1, true)
	if len(got) != 1 || got[0].Text != "." {
		t.Errorf("completions of . are %v, want just .", got)
	}
	for _, line := range []string{"goto .", "open ."} {
		if got := m.complete(line, len(line)); len(got) != 1 || got[0].Text != "." {
			t.Errorf("completions of %q are %v, want just .", line, got)
		}
	}
	m, _ = newGotoApp(t, newGotoRepos())
	if got := m.completeTarget(".", 1, 1, true); len(got) != 0 {
		t.Errorf("completions of . are %v without a repository here, want none", got)
	}
}

// TestBackspaceOnTheSearchPage checks that backspace goes back from the
// search modal while the query isn't typing, and deletes while it is.
func TestBackspaceOnTheSearchPage(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, false)
		pressKeys(t, m, "S", "i", "k", "e", "y", "backspace")
		if m.searchHost() == nil || m.srch.section.(*searchpage.Section).Typed() != "ke" {
			t.Fatalf("backspace while typing: screen %d, query %q, want the page with ke", m.screen, m.srch.section.(*searchpage.Section).Typed())
		}
		pressKeys(t, m, "esc", "backspace")
		if m.searchHost() != nil {
			t.Error("backspace in the results didn't go back")
		}
		pressKeys(t, m, "S", "backspace")
		if m.searchHost() != nil {
			t.Error("backspace on the query in normal mode didn't go back")
		}
	})
}

// TestRepoKeyOnTheQueryDoesNothing checks that with the query of the
// search modal focused but not typing, no result is selected for the repo
// key.
func TestRepoKeyOnTheQueryDoesNothing(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, false)
		pressKeys(t, m, "S", "i", "k", "e", "y", "esc", "1")
		if got := focusOf(m); got != `search: results "key"` {
			t.Fatalf("the keys reach %s, want the results (a digit picks the kind)", got)
		}
		// The repo key is enabled in the search modal; what matters is that
		// it does nothing with no result selected, checked below.
		pressKeys(t, m, ".")
		if m.searchHost() == nil {
			t.Errorf("screen = %d, want the search modal still", m.screen)
		}
	})
}

// TestOpenHere checks that open . opens the page of the repository of the
// current directory, and that . says where the repository is when it is on
// another host, for open and goto.
func TestOpenHere(t *testing.T) {
	t.Parallel()
	m, _ := newGotoApp(t, newGotoRepos(), WithHere(testRepo))
	got := runCommand(t, m, "open .")
	if !slices.Contains(got, tea.Msg(ui.OpenMsg{URL: "https://github.com/eggzec/gh-tui"})) {
		t.Errorf("open . sent %v, want the page of %s", got, testRepo)
	}

	for _, line := range []string{"open .", "goto ."} {
		m, _ = newGotoApp(t, newGotoRepos(), WithHereElsewhere("ghe.example.com"))
		runCommand(t, m, line)
		if !hasToast(m, "The repository here is on ghe.example.com, not github.com.") {
			t.Errorf("%s: toasts %s; want the host of the repository here", line, toasted(m))
		}
	}

	m, _ = newGotoApp(t, newGotoRepos())
	runCommand(t, m, "open .")
	if !hasToast(m, "No repository in the current directory.") {
		t.Errorf("open . toasts %s without a repository here", toasted(m))
	}
}
