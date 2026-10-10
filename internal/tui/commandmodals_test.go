package tui

import (
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// The command key opens the command line over each modal that doesn't type
// the key as text, and esc closes the line again, leaving the modal.
func TestCommandKeyOpensOverEveryModal(t *testing.T) {
	t.Parallel()
	for _, mod := range modalsOver {
		t.Run(mod.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, mod.steps...)
				open := m.modal
				tap(t, m, ":")
				if !m.line.Focused() {
					t.Fatalf(": didn't open the command line over %s", mod.title)
				}
				if m.modal != open {
					t.Error("the command key changed the modal")
				}
				tap(t, m, "esc")
				if m.line.Focused() || m.modal != open {
					t.Errorf("esc left the line open %v, modal kept %v; want it closed over the same modal", m.line.Focused(), m.modal == open)
				}
			})
		})
	}
}

// Esc over a modal dismisses an error toast first, then cancels the line,
// and only then reaches the modal.
func TestEscLadderOverTheCommandLine(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_2", "global.select")
		open := m.modal
		tap(t, m, ":")
		driveKeys(t, m, m.toast.Push(toast.Error, "It failed."))
		tap(t, m, "esc")
		if m.toast.Has(toast.Error) || !m.line.Focused() {
			t.Errorf("first esc: error toast %v, line open %v; want the toast gone and the line open", m.toast.Has(toast.Error), m.line.Focused())
		}
		tap(t, m, "esc")
		if m.line.Focused() || m.modal != open {
			t.Errorf("second esc: line open %v, modal kept %v; want the line closed and the modal open", m.line.Focused(), m.modal == open)
		}
		tap(t, m, "esc")
		if m.modal != nil {
			t.Error("third esc left the modal open")
		}
	})
}

// The finder types the command key into its query, and a question that a
// modal asks takes it as an answer.
func TestCommandKeyIsTypedWhereTheModalTypes(t *testing.T) {
	t.Parallel()
	m := newKeysApp(t, true)
	pressKeys(t, m, "ctrl+p")
	mod := m.modal
	if mod == nil {
		t.Fatal("ctrl+p opened no finder")
	}
	pressKeys(t, m, ":")
	if m.line.Focused() {
		t.Error(": opened the line over the finder, which types it")
	}
	if m.modal != mod {
		t.Error("the finder closed")
	}
}

// An allowed command runs over each modal, and a refused one says to close
// the modal first, by its name, and leaves it open. An action that the
// modal doesn't have says where it works.
func TestCommandsOverEveryModal(t *testing.T) {
	t.Parallel()
	for _, mod := range modalsOver {
		t.Run(mod.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, mod.steps...)
				open := m.modal
				runCommand(t, m, "set files.markdown=raw")
				if m.cfg.Files.Markdown != "raw" {
					t.Errorf("set over %s didn't run: files.markdown is %q", mod.title, m.cfg.Files.Markdown)
				}
				for _, line := range []string{"goto cli/cli", "search", "find_file", "auth"} {
					runCommand(t, m, line)
					name, _, _ := strings.Cut(line, " ")
					want := "Close " + mod.title + " first to use " + name + "."
					if !hasToast(m, want) {
						t.Errorf("%s: toasts %s, want %q", line, toasted(m), want)
					}
					if m.modal != open {
						t.Fatalf("%s changed the modal", line)
					}
				}
				runCommand(t, m, "history")
				if want := "Close " + mod.title + " first to use history."; !hasToast(m, want) {
					t.Errorf("history over %s: toasts %s, want %q", mod.title, toasted(m), want)
				}
				if m.modal != open {
					t.Fatal("history changed the modal")
				}
			})
		})
	}
}

// The names of the commands that complete over a modal are those that run
// over it: the commands of the app alone, and those that it names.
func TestCompletionOverModalsListsWhatRuns(t *testing.T) {
	t.Parallel()
	m := newKeysApp(t, true)
	all := func(prefix string) []string { return texts(m.complete(prefix, len(prefix))) }
	for _, tt := range []struct {
		prefix string
		want   []string
	}{
		{"s", []string{"search ", "set "}},
		{"co", []string{"collapse", "config ", "copy "}},
		{"r", []string{"reset_base", "raw ", "references", "refresh", "repo"}},
		{"g", []string{"goto "}},
	} {
		if got := all(tt.prefix); !slices.Equal(got, tt.want) {
			t.Errorf("without a modal %q completes %q, want %q", tt.prefix, got, tt.want)
		}
	}
	for _, tt := range []struct {
		name   string
		steps  []string
		prefix string
		want   []string
	}{
		{"history", []string{"repo.history"}, "s", []string{"set "}},
		{"history", []string{"repo.history"}, "co", []string{"config ", "copy "}},
		{"history", []string{"repo.history"}, "r", []string{"reset_base", "refresh"}},
		{"actions", []string{"repo.actions"}, "g", nil},
		{"pull request", []string{"global.pane_2", "global.select"}, "co", []string{"config ", "copy "}},
		{"pull request", []string{"global.pane_2", "global.select"}, "r", []string{"reopen", "references", "refresh"}},
		{"issue", []string{"global.pane_3", "global.select"}, "r", []string{"reopen", "references", "refresh"}},
		{"issue", []string{"global.pane_3", "global.select"}, "co", []string{"comment", "config ", "copy "}},
		{"preview", []string{"files.down", "global.select"}, "r", []string{"raw ", "refresh"}},
		{"preview", []string{"files.down", "global.select"}, "co", []string{"config ", "copy "}},
		{"preview", []string{"files.down", "global.select"}, "o", []string{"option", "open "}},
	} {
		synctest.Test(t, func(t *testing.T) {
			m := keysAfter(t, true, tt.steps...)
			if got := texts(m.complete(tt.prefix, len(tt.prefix))); !slices.Equal(got, tt.want) {
				t.Errorf("over %s %q completes %q, want %q", tt.name, tt.prefix, got, tt.want)
			}
		})
	}
}

// Every command that a modal names is one that runs over the modals that
// name it, and not over the others.
func TestDeclaredCommandsExist(t *testing.T) {
	t.Parallel()
	for _, mod := range modalsOver {
		synctest.Test(t, func(t *testing.T) {
			m := keysAfter(t, true, mod.steps...)
			for _, name := range m.modal.Commands() {
				c, ok := findCommand(name)
				if !ok || c.over != overNamed {
					t.Errorf("%s names %q, which is %v and not a command that modals name", mod.name, name, ok)
				}
			}
		})
	}
	for _, c := range commands {
		if c.over == overNamed && c.name != ui.CommandCopy && c.name != ui.CommandRaw && c.name != ui.CommandReferences {
			t.Errorf("%s runs over the modals that name it, and none does", c.name)
		}
	}
}

// Over a modal, the config shows in the pager, and backspace returns to the
// modal as it was.
func TestConfigOverAModalReturnsToIt(t *testing.T) {
	t.Parallel()
	for _, mod := range modalsOver {
		t.Run(mod.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, mod.steps...)
				open := m.modal
				runCommand(t, m, "config")
				if m.modal == open || m.modalName(m.modal) != "the pager" {
					t.Fatalf("config over %s shows %v, want the pager", mod.title, m.modal)
				}
				tap(t, m, "backspace")
				if m.modal != open {
					t.Errorf("backspace left the pager over %s, want it back", mod.title)
				}
			})
		})
	}
}

// Over a modal, copy takes what the modal shows, not what the list or
// the tree behind it has selected: each modal is opened on something other
// than what the screen's cursor is on.
func TestCopyOverAModal(t *testing.T) {
	t.Parallel()
	other := testRepo
	for _, tt := range []struct {
		name  string
		steps []string
		msg   tea.Msg
		lines map[string]string
	}{
		// The list's cursor is on pull request 2, the issue list's on 1.
		{name: "pull request", msg: ui.OpenPullMsg{Repo: other, Number: 9}, lines: map[string]string{"copy ref": "Copied eggzec/gh-tui#9."}},
		{name: "issue", msg: ui.OpenIssueMsg{Repo: other, Number: 8}, lines: map[string]string{"copy ref": "Copied eggzec/gh-tui#8."}},
		{name: "release", msg: ui.OpenReleaseMsg{Repo: other, ID: keyRelease.ID, URL: keyRelease.URL}, lines: map[string]string{
			"copy ref": "Copied v1.0.0.", "copy url": "Copied " + keyRelease.URL + ".",
		}},
		{name: "file", msg: ui.OpenFileMsg{Repo: other, Path: "README.md", SHA: "b1"}, lines: map[string]string{"copy path": "Copied README.md."}},
		{name: "commit", msg: ui.OpenCommitMsg{Repo: other, SHA: keyCommit.SHA}, lines: map[string]string{
			"copy sha": "Copied abc123.", "copy url": "Copied " + keyCommit.URL + ".",
		}},
		{name: "run", steps: []string{"repo.actions"}, lines: map[string]string{
			"copy sha": "Copied abc123.", "copy url": "Copied " + keyRun.URL + ".",
		}},
		{name: "job", steps: []string{"repo.actions", "global.next_pane"}, lines: map[string]string{"copy url": "Copied " + keyJob.URL + "."}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, tt.steps...)
				if tt.msg != nil {
					driveKeys(t, m, func() tea.Msg { return tt.msg })
				}
				open := m.modal
				if open == nil {
					t.Fatal("no modal is open")
				}
				for line, want := range tt.lines {
					runCommand(t, m, line)
					if !hasToast(m, want) {
						t.Errorf("%s: toasts %s, want %q", line, toasted(m), want)
					}
					if m.modal != open {
						t.Fatalf("%s changed the modal", line)
					}
				}
			})
		})
	}
}

// A release that isn't read yet has no tag, so there is no reference of it
// to copy, and the repository is not one.
func TestCopyRefOfAReleaseNotRead(t *testing.T) {
	t.Parallel()
	sel := ui.Selection{What: "release", Repo: testRepo}
	if got := ref(sel); got != "" {
		t.Errorf("ref of a release without a tag = %q, want none", got)
	}
}
