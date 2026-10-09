package tui

import (
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// keysAfter returns the app on testRepo after steps, each an action of the
// config, with every other message that follows them run.
func keysAfter(t *testing.T, repo bool, steps ...string) *Model {
	t.Helper()
	m := newKeysApp(t, repo)
	c := keyContext{name: "root rules", repo: repo}
	for _, s := range steps {
		for _, name := range c.press(t, m.cfg.Keys, s) {
			msg, _ := keyPress(name)
			driveKeys(t, m, m.key(msg))
		}
	}
	return m
}

// tap presses the key and runs what follows.
func tap(t *testing.T, m *Model, k string) {
	t.Helper()
	msg, ok := keyPress(k)
	if !ok {
		t.Fatalf("can't press %q", k)
	}
	driveKeys(t, m, m.key(msg))
}

// modalsOver are the modals the root rules are checked over, each with
// the steps that open it and what the refusal calls it.
var modalsOver = []struct {
	name  string
	steps []string
	title string
}{
	{"pull request", []string{"global.pane_2", "global.select"}, "the pull request"},
	{"pull request checks", []string{"global.pane_2", "pulls.checks"}, "the pull request"},
	{"issue", []string{"global.pane_3", "global.select"}, "the issue"},
	{"history", []string{"repo.history"}, "History"},
	{"actions runs", []string{"repo.actions"}, "Actions"},
	{"actions log", []string{"repo.actions", "global.next_pane", "global.next_pane"}, "Actions"},
	{"preview", []string{"files.down", "global.select"}, "the file"},
	{"config pager", []string{"global.command", typed("config"), "command_line.run"}, "the pager"},
	{"filter", []string{"global.pane_2", "pulls.filter"}, "the filter"},
	{"token prompt", []string{"global.command", typed("auth"), "command_line.run"}, "the token prompt"},
}

// TestScreenKeysAreRefusedOverAModal checks that a key that shows another
// screen is refused while a modal is open, naming the modal and what the
// key does, and is disabled in the modal's help.
func TestScreenKeysAreRefusedOverAModal(t *testing.T) {
	for _, mod := range modalsOver {
		for _, tt := range []struct{ key, use string }{
			{"0", "the dashboard"}, {"I", "notifications"}, {"S", "search"}, {".", "the repository"},
		} {
			t.Run(mod.name+" "+tt.key, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					m := keysAfter(t, true, mod.steps...)
					open, screen := m.modal, m.screen
					tap(t, m, tt.key)
					want := "Close " + mod.title + " first to use " + tt.use + "."
					if !hasToast(m, want) {
						t.Errorf("toasts: %s, want %q", toasted(m), want)
					}
					if m.modal != open || m.screen != screen {
						t.Errorf("the key changed what shows: modal %v, screen %v", m.modal != open, m.screen != screen)
					}
				})
			})
		}
	}
}

// TestScreenKeysAreDisabledInModalHelp checks that the help over a modal
// lists the keys that show another screen as disabled, but for the owner
// key where the modal takes it for its author.
func TestScreenKeysAreDisabledInModalHelp(t *testing.T) {
	for _, tt := range []struct {
		modal      []string
		owner      bool
		ownerLabel string
	}{
		{[]string{"global.pane_2", "global.select"}, true, "author"},
		{[]string{"global.pane_3", "global.select"}, true, "author"},
		{[]string{"repo.actions"}, false, ""},
		{[]string{"repo.history"}, false, ""},
	} {
		synctest.Test(t, func(t *testing.T) {
			m := keysAfter(t, true, tt.modal...)
			on := map[string]bool{}
			for _, l := range m.layersNow() {
				for _, b := range l.Bindings {
					for _, k := range b.Keys() {
						on[k+" "+b.Help().Desc] = b.Enabled()
					}
				}
			}
			for _, k := range []string{"0 dashboard", "I notifications", "S search", ". this repo"} {
				if enabled, ok := on[k]; !ok || enabled {
					t.Errorf("%v: help row %q listed %v, enabled %v; want it listed disabled", tt.modal, k, ok, enabled)
				}
			}
			author, listed := on["@ author"]
			ownerPage, refused := on["@ owner page"]
			switch {
			case tt.owner && (!listed || !author || refused):
				t.Errorf("%v: the author key is %v, enabled %v, and the owner page listed %v; want the author enabled alone", tt.modal, listed, author, refused)
			case !tt.owner && (listed || !refused || ownerPage):
				t.Errorf("%v: the owner page key is listed %v, enabled %v, author listed %v; want it listed disabled", tt.modal, refused, ownerPage, listed)
			}
		})
	}
}

// TestOwnerKeyOverModals checks that the owner key shows the author of a
// pull request or an issue from its modal, and is refused over the others.
func TestOwnerKeyOverModals(t *testing.T) {
	for _, steps := range [][]string{{"global.pane_2", "global.select"}, {"global.pane_3", "global.select"}} {
		synctest.Test(t, func(t *testing.T) {
			m := keysAfter(t, true, steps...)
			tap(t, m, "@")
			if m.modal != nil || m.screen != ownerScreen {
				t.Errorf("%v: after @, modal open %v on screen %v; want the owner page of the author", steps, m.modal != nil, m.screen)
			}
			if m.ownerLogin != "octocat" {
				t.Errorf("%v: the page is of %q, want the author octocat", steps, m.ownerLogin)
			}
		})
	}
	for _, steps := range [][]string{{"repo.actions"}, {"repo.history"}} {
		synctest.Test(t, func(t *testing.T) {
			m := keysAfter(t, true, steps...)
			tap(t, m, "@")
			if m.modal == nil || m.screen != repoScreen {
				t.Errorf("%v: @ left the modal, which refuses it", steps)
			}
			want := "Close " + modalName(m) + " first to use the owner page."
			if !hasToast(m, want) {
				t.Errorf("%v: toasts %s, want %q", steps, toasted(m), want)
			}
		})
	}
}

func modalName(m *Model) string { return m.modalName(m.modal) }

// TestQuitClosesTheModal checks that the quit key closes the modal in one
// press, from any of its views, and quits when none is open.
func TestQuitClosesTheModal(t *testing.T) {
	for _, mod := range modalsOver {
		t.Run(mod.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, mod.steps...)
				if m.modal == nil {
					t.Fatal("the modal isn't open")
				}
				cmd := m.key(press("q"))
				driveKeys(t, m, cmd)
				if m.modal != nil {
					t.Errorf("q left the modal open")
				}
			})
		})
	}
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true)
		cmd := m.key(press("q"))
		if cmd == nil {
			t.Fatal("q quits nothing")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Error("q with no modal should quit")
		}
	})
}

// TestQuitIsTypedInAnInput checks that q types into a modal's input, and
// doesn't close it.
func TestQuitIsTypedInAnInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_3", "global.select", "issue_modal.comment")
		tap(t, m, "q")
		if m.modal == nil {
			t.Error("q closed the modal while its comment was typed")
		}
	})
}

// TestDismissLadder checks that the dismiss key goes one step at a time: an
// error toast first, even over a modal, and then the modal closes.
func TestDismissLadder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_2", "global.select")
		driveKeys(t, m, func() tea.Msg { return ui.NotifyMsg{Level: toast.Error, Text: "Could not merge."} })
		tap(t, m, "esc")
		if m.modal == nil {
			t.Error("esc closed the modal with an error toast on view")
		}
		if m.toast.Has(toast.Error) {
			t.Error("esc left the error toast")
		}
		tap(t, m, "esc")
		if m.modal != nil {
			t.Error("esc didn't close the modal once the toast was gone")
		}
	})
}

// TestDismissLeavesOtherToasts checks that the dismiss key takes an error
// toast only: the others go on their own.
func TestDismissLeavesOtherToasts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true)
		driveKeys(t, m, func() tea.Msg { return ui.NotifyMsg{Level: toast.Info, Text: "Copied."} })
		driveKeys(t, m, func() tea.Msg { return ui.NotifyMsg{Level: toast.Error, Text: "Could not merge."} })
		tap(t, m, "esc")
		if m.toast.Has(toast.Error) || !m.toast.Has(toast.Info) {
			t.Errorf("toasts after esc: %s, want the info toast alone", toasted(m))
		}
	})
}

// TestOnlyTheZoomKeyUnzooms checks that esc and backspace leave a zoomed
// pane zoomed: on the screens, where esc does nothing, and in a modal,
// where backspace steps back and esc closes the modal; the zoom key shows
// the panes again.
func TestOnlyTheZoomKeyUnzooms(t *testing.T) {
	t.Run("repository", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := keysAfter(t, true, "global.zoom")
			if !m.zoomed() {
				t.Fatal("the pane isn't zoomed")
			}
			tap(t, m, "esc")
			if !m.zoomed() {
				t.Error("esc unzoomed the repository screen")
			}
			tap(t, m, "z")
			if m.zoomed() {
				t.Error("z didn't unzoom")
			}
		})
	})
	t.Run("dashboard", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := keysAfter(t, false, "global.zoom")
			before := onScreen(m)
			tap(t, m, "esc")
			if after := onScreen(m); after != before {
				t.Errorf("esc changed the zoomed dashboard:\n%s\nwas:\n%s", after, before)
			}
		})
	})
	t.Run("owner", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := newKeysApp(t, false)
			driveKeys(t, m, func() tea.Msg { return ui.OwnerMsg{Login: "octocat"} })
			tap(t, m, "z")
			before := onScreen(m)
			tap(t, m, "esc")
			if after := onScreen(m); after != before {
				t.Errorf("esc changed the zoomed owner page:\n%s\nwas:\n%s", after, before)
			}
		})
	})
	t.Run("history", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := keysAfter(t, true, "repo.history", "global.zoom")
			if !strings.Contains(onScreen(m), "z unzoom") {
				t.Fatal("the history isn't zoomed")
			}
			tap(t, m, "backspace")
			// backspace steps back from the graph, which leaves the modal
			// open, and never shows the panes again: the zoom key does.
			if m.modal == nil {
				t.Fatal("backspace closed the modal, which should step back from the graph")
			}
			if !strings.Contains(onScreen(m), "z unzoom") {
				t.Error("backspace unzoomed the history")
			}
			// esc closes the modal rather than showing the panes again.
			tap(t, m, "esc")
			if m.modal != nil {
				t.Error("esc left the history open")
			}
		})
	})
}

// TestErrorToastsStayUntilDismissed checks that an error toast has no
// time of its own and so no timer, by default.
func TestErrorToastsStayUntilDismissed(t *testing.T) {
	m := New(t.Context(), config.Default(), Layout{Files: &fakeSection{title: "Files"}}, WithRepo(testRepo))
	if d := m.toast.ErrorDuration(); d != 0 {
		t.Errorf("error toasts stay %v, want until dismissed", d)
	}
	if cmd := m.toast.Push(toast.Error, "Could not merge."); cmd != nil {
		t.Error("an error toast has a timer")
	}
	if cmd := m.toast.Push(toast.Info, "Copied."); cmd == nil {
		t.Error("an info toast has no timer")
	}
}

// TestToastKeyIsGone checks that ctrl+x no longer dismisses a toast and
// that the config has no action for it.
func TestToastKeyIsGone(t *testing.T) {
	for _, a := range config.Default().Keys.Actions() {
		if strings.HasSuffix(a, "dismiss_toast") {
			t.Errorf("the config has the action %s", a)
		}
	}
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true)
		driveKeys(t, m, func() tea.Msg { return ui.NotifyMsg{Level: toast.Error, Text: "Could not merge."} })
		tap(t, m, "ctrl+x")
		if !m.toast.Has(toast.Error) {
			t.Error("ctrl+x dismissed the error toast")
		}
	})
}

// TestGuidanceIsAWarning checks that what only tells the user how to use
// something, such as a refusal over a modal or a mistyped command, is a
// warning that goes on its own, and that a failure stays an error.
func TestGuidanceIsAWarning(t *testing.T) {
	fresh := New(t.Context(), config.Default(), Layout{Files: &fakeSection{title: "Files"}}, WithRepo(testRepo))
	if cmd := fresh.toast.Push(toast.Warning, "Close the file first."); cmd == nil {
		t.Error("a warning has no timer")
	}
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_2", "global.select")
		tap(t, m, "I")
		if !m.toast.Has(toast.Warning) || m.toast.Has(toast.Error) {
			t.Errorf("the refusal over a modal is not a warning alone: %s", toasted(m))
		}
	})
	for _, line := range []string{"set", "nonsense", "set nothing=1", "copy", "raw maybe"} {
		m, _ := newTestApp(t)
		runCommand(t, m, line)
		if !m.toast.Has(toast.Warning) || m.toast.Has(toast.Error) {
			t.Errorf(":%s says its guidance as %q, want a warning alone", line, toasted(m))
		}
	}
}

// TestDismissOrder checks that esc dismisses an error toast first, then
// cancels a goto that waits, even while a query is typed.
func TestDismissOrder(t *testing.T) {
	repos := newGotoRepos()
	m, _ := newGotoApp(t, repos)
	submitLine(t, m, "goto charmbracelet/bubbletea")
	run(m, m.toast.Push(toast.Error, "Could not merge."))
	if m.going == nil || !m.toast.Has(toast.Error) {
		t.Fatal("the goto and the toast aren't both there")
	}
	run(m, m.key(press("esc")))
	if m.toast.Has(toast.Error) || m.going == nil {
		t.Errorf("first esc: error toast %v, goto waiting %v; want the toast gone and the goto waiting", m.toast.Has(toast.Error), m.going != nil)
	}
	run(m, m.key(press("esc")))
	if m.going != nil {
		t.Error("second esc didn't cancel the goto")
	}

	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, false, "global.search", "search.insert")
		driveKeys(t, m, func() tea.Msg { return ui.NotifyMsg{Level: toast.Error, Text: "Could not merge."} })
		tap(t, m, "esc")
		if m.toast.Has(toast.Error) {
			t.Error("esc left the error toast while a query was typed")
		}
		if got := focusOf(m); !strings.Contains(got, "typing") {
			t.Errorf("focus = %s, want the typed query kept", got)
		}
	})
}

// TestCtrlCQuitsFromTheCommandLine checks that ctrl+c quits from the open
// command line, and that esc cancels the line.
func TestCtrlCQuitsFromTheCommandLine(t *testing.T) {
	m, _ := newTestApp(t)
	run(m, m.key(press(":")))
	cmd := m.key(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c did nothing in the command line")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c in the command line should quit")
	}
	m, _ = newTestApp(t)
	run(m, m.key(press(":")))
	drive(m, m.key(press("esc")))
	if m.line.Focused() {
		t.Error("esc didn't cancel the command line")
	}
}

// TestQuitClosesTheRelease checks that q closes a release modal.
func TestQuitClosesTheRelease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, true)
		driveKeys(t, m, func() tea.Msg {
			return ui.OpenReleaseMsg{Repo: testRepo, ID: keyRelease.ID, URL: keyRelease.URL}
		})
		if m.modal == nil {
			t.Fatal("the release isn't open")
		}
		tap(t, m, "q")
		if m.modal != nil {
			t.Error("q left the release open")
		}
	})
}
