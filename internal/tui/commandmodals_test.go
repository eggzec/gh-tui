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

// Esc over a modal dismisses an error toast first, then cancels the line,
// and only then reaches the modal.
func TestEscLadderOverTheCommandLine(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_2", "global.select")
		open := m.modals.Top()
		tap(t, m, ":")
		driveKeys(t, m, m.toast.Push(toast.Error, "It failed."))
		tap(t, m, "esc")
		if m.toast.Has(toast.Error) || !m.line.Focused() {
			t.Errorf("first esc: error toast %v, line open %v; want the toast gone and the line open", m.toast.Has(toast.Error), m.line.Focused())
		}
		tap(t, m, "esc")
		if m.line.Focused() || m.modals.Top() != open {
			t.Errorf("second esc: line open %v, modal kept %v; want the line closed and the modal open", m.line.Focused(), m.modals.Top() == open)
		}
		tap(t, m, "esc")
		if m.modals.Top() != nil {
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
	mod := m.modals.Top()
	if mod == nil {
		t.Fatal("ctrl+p opened no finder")
	}
	pressKeys(t, m, ":")
	if m.line.Focused() {
		t.Error(": opened the line over the finder, which types it")
	}
	if m.modals.Top() != mod {
		t.Error("the finder closed")
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
		{"s", []string{"star", "search ", "set "}},
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
		{"history", []string{"repo.history"}, "s", []string{"search ", "set "}},
		{"history", []string{"repo.history"}, "co", []string{"config ", "copy "}},
		{"history", []string{"repo.history"}, "r", []string{"reset_base", "refresh"}},
		{"actions", []string{"repo.actions"}, "g", []string{"group_by", "group_toggle"}},
		{"pull request", []string{"global.pane_2", "global.select"}, "co", []string{"collapse", "comment", "config ", "copy "}},
		{"pull request", []string{"global.pane_2", "global.select"}, "r", []string{"resolve", "review", "refresh"}},
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
			for _, name := range m.modals.Top().Commands() {
				c, ok := findCommand(name)
				if !ok || c.over != overNamed {
					t.Errorf("%s names %q, which is %v and not a command that modals name", mod.name, name, ok)
				}
			}
		})
	}
	for _, c := range commands {
		if c.over == overNamed && c.name != ui.CommandCopy && c.name != ui.CommandRaw && c.name != ui.CommandReferences &&
			c.name != ui.CommandNextFile && c.name != ui.CommandPrevFile {
			t.Errorf("%s runs over the modals that name it, and none does", c.name)
		}
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
				open := m.modals.Top()
				if open == nil {
					t.Fatal("no modal is open")
				}
				for line, want := range tt.lines {
					runCommand(t, m, line)
					if !hasToast(m, want) {
						t.Errorf("%s: toasts %s, want %q", line, toasted(m), want)
					}
					if m.modals.Top() != open {
						t.Fatalf("%s changed the modal", line)
					}
				}
			})
		})
	}
}

// Esc closes the diff of a pull request at once, over a search it shows,
// through the app's dismiss key, and leaves the pull request open.
func TestEscClosesTheDiffOverASearch(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_2", "global.select", "global.pane_6")
		for _, key := range []string{"j", "j", "j"} {
			m.View()
			pressKeys(t, m, key)
		}
		files := m.View().Content
		pressKeys(t, m, "enter")
		diff := m.View().Content
		if diff == files {
			t.Fatal("enter did not open the diff")
		}
		pressKeys(t, m, "/")
		typeKeys(m, "e")
		driveKeys(t, m, m.key(enter))
		pull := m.modals.Top()
		if pull == nil {
			t.Fatal("no pull request open")
		}
		searched := m.View().Content
		pressKeys(t, m, "esc")
		if m.modals.Top() != pull {
			t.Errorf("esc left %T on top, want the pull request", m.modals.Top())
		}
		// "hunks only" is in the diff's title and nowhere in the pull request.
		if !strings.Contains(searched, "hunks only") {
			t.Fatal("the diff's title is not on screen over the search")
		}
		if got := m.View().Content; strings.Contains(got, "hunks only") {
			t.Error("esc left the diff open over a search")
		}
	})
}

// n and p step through the files of an open diff, and are refused over the
// pull request itself, which has no files to step through.
func TestColonNRunsOnlyInTheDiff(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_2", "global.select", "global.pane_6")
		runCommand(t, m, "n")
		if want := "Close the pull request first to use n."; !hasToast(m, want) {
			t.Errorf("n over the pull request: toasts %s, want %q", toasted(m), want)
		}
		// The rows are laid out when drawn, so each move follows a draw. The
		// third j passes the first folder of the tree, to its first file.
		for _, key := range []string{"j", "j", "j", "enter"} {
			m.View()
			pressKeys(t, m, key)
		}
		diff := m.View().Content
		runCommand(t, m, "n")
		next := m.View().Content
		if next == diff {
			t.Error("n left the diff on the same file")
		}
		runCommand(t, m, "p")
		if got := m.View().Content; got != diff {
			t.Errorf("p after n did not come back to the first file")
		}
		if got := texts(m.complete("n", 1)); !slices.Contains(got, "n") {
			t.Errorf("n over the diff completes %q, want it named", got)
		}
	})
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
