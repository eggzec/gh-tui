package tui

import (
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// A target that isn't there is usually a typo: goto says so in a warning
// that expires, whatever the target is, and keeps the error toasts, which
// stay until dismissed, for the failures of asking GitHub.
func TestGotoNotFoundIsAWarning(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		line string
		err  error
		// err is the error of a GitHub that fails, and warns reports
		// whether goto's toast is a warning.
		warns bool
	}{
		{name: "repository", line: "goto nosuchowner/nosuchrepo", warns: true},
		{name: "pull request or issue", line: "goto charmbracelet/bubbletea#99999999", warns: true},
		{name: "user or organization", line: "goto @octocta", warns: true},
		{name: "repository offline", line: "goto charmbracelet/bubbletea", err: errOffline},
		{name: "number offline", line: "goto charmbracelet/bubbletea#1813", err: errOffline},
		{name: "repository forbidden", line: "goto charmbracelet/bubbletea", err: core.ErrForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repos := newGotoRepos()
			kinds := newGotoKinds()
			owners := newFakeOwners()
			if strings.Contains(tt.line, "#") {
				kinds.err = tt.err
			} else {
				repos.err = tt.err
			}
			m, _ := newGotoApp(t, repos, WithKinds(kinds), WithOwners(owners))
			runCommand(t, m, tt.line)
			if toasted(m) == "" {
				t.Fatal("goto told nothing")
			}
			if got := m.toast.Has(toast.Warning); got != tt.warns {
				t.Errorf("warning shows = %v, want %v: %s", got, tt.warns, toasted(m))
			}
			if got := m.toast.Has(toast.Error); got == tt.warns {
				t.Errorf("error shows = %v, want %v: %s", got, !tt.warns, toasted(m))
			}
		})
	}
}

// esc dismisses an error toast first, and cancels a goto that waits next,
// even over the command line and the help, which close only when there is
// neither.
func TestEscDismissesBeforeLineAndHelp(t *testing.T) {
	t.Parallel()
	failure := errors.New("boom")
	open := map[string]func(*testing.T, *Model){
		"command line": func(t *testing.T, m *Model) {
			t.Helper()
			drive(m, m.key(press(":")))
			typeKeys(m, "go")
			if !m.line.Focused() {
				t.Fatal(": didn't open the line")
			}
		},
		"help": func(t *testing.T, m *Model) {
			t.Helper()
			drive(m, m.key(press("?")))
			if !m.helpOpen() {
				t.Fatal("? didn't open the help")
			}
		},
	}
	isOpen := map[string]func(*Model) bool{
		"command line": func(m *Model) bool { return m.line.Focused() },
		"help":         func(m *Model) bool { return m.helpOpen() },
	}
	for name, opener := range open {
		t.Run(name+" over an error toast", func(t *testing.T) {
			m, _ := newGotoApp(t, newGotoRepos())
			m.toast.Push(toast.Error, "Couldn't do it: "+failure.Error()+".")
			opener(t, m)
			drive(m, m.key(press("esc")))
			if m.toast.Has(toast.Error) {
				t.Error("esc left the error toast")
			}
			if !isOpen[name](m) {
				t.Errorf("esc closed the %s while an error toast showed", name)
			}
			drive(m, m.key(press("esc")))
			if isOpen[name](m) {
				t.Errorf("a second esc left the %s open", name)
			}
		})
		t.Run(name+" over a goto", func(t *testing.T) {
			m, _ := newGotoApp(t, newGotoRepos())
			cmd := submitLine(t, m, "goto charmbracelet/bubbletea")
			if m.going == nil {
				t.Fatal("the goto doesn't wait")
			}
			opener(t, m)
			drive(m, m.key(press("esc")))
			if m.going != nil {
				t.Error("esc left the goto waiting")
			}
			if !isOpen[name](m) {
				t.Errorf("esc closed the %s before it canceled the goto", name)
			}
			drive(m, cmd)
			if m.repo == bubbletea {
				t.Error("the canceled goto opened its repository")
			}
			drive(m, m.key(press("esc")))
			if isOpen[name](m) {
				t.Errorf("a second esc left the %s open", name)
			}
		})
	}
}

// Where icons are plain ASCII, no section draws a separator, a mark or an
// ellipsis from the set of unicode ones.
func TestASCIIIconsOnTheRepositoryScreen(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysAppWith(t, true, func(c *config.Config) { c.UI.Icons = config.IconsASCII })
		resize(m, 80, 24)
		for i, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
			if strings.ContainsRune(line, '·') {
				t.Errorf("line %d draws a unicode separator: %q", i+1, line)
			}
		}
	})
}
