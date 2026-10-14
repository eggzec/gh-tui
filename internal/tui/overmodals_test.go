package tui

import (
	"strings"
	"testing"
	"testing/synctest"
)

// TestOverEveryModal checks, over each of modalsOver, what the root rules
// do while a modal is open. Each modal is opened once, and the checks run
// in turn on that app: a subtest can't start inside the bubble of the
// fake clock, so each message names its check. The ones that leave the
// modal as it was come first, each starting with no toast so that it only
// sees its own; the one that closes it comes last.
func TestOverEveryModal(t *testing.T) {
	t.Parallel()
	for _, mod := range modalsOver {
		t.Run(mod.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, mod.steps...)
				if m.modal == nil {
					t.Fatal("the modal isn't open")
				}
				open := m.modal

				// A check could leave the modal on another step or focus
				// without replacing it, and every later check, the quit
				// above all, would then test that other view. So the
				// layers of keys it opened with are kept, and each check
				// must leave the modal on them.
				views := layerNames(m.keyLayers())
				same := func(after string) {
					t.Helper()
					if got := layerNames(m.keyLayers()); m.modal != open || got != views {
						t.Fatalf("%s: left %s with keys of %q, want %q", after, mod.title, got, views)
					}
				}

				// A key that shows another screen is refused, naming the
				// modal and what the key does.
				for _, tt := range []struct{ key, use string }{
					{"0", "the dashboard"}, {"I", "notifications"}, {"S", "search"}, {".", "the repository"},
				} {
					m.toast.Clear()
					screen := m.screen
					tap(t, m, tt.key)
					want := "Close " + mod.title + " first to use " + tt.use + "."
					if !hasToast(m, want) {
						t.Errorf("screen key %s: toasts: %s, want %q", tt.key, toasted(m), want)
					}
					if m.modal != open || m.screen != screen {
						t.Errorf("screen key %s: the key changed what shows: modal %v, screen %v", tt.key, m.modal != open, m.screen != screen)
					}
					same("screen key " + tt.key)
				}

				// The command key opens the command line over each modal
				// that doesn't type the key as text, and esc closes the line
				// again, leaving the modal.
				tap(t, m, ":")
				if !m.line.Focused() {
					t.Fatalf("command key: : didn't open the command line over %s", mod.title)
				}
				if m.modal != open {
					t.Error("command key: the key changed the modal")
				}
				tap(t, m, "esc")
				if m.line.Focused() || m.modal != open {
					t.Errorf("command key: esc left the line open %v, modal kept %v; want it closed over the same modal", m.line.Focused(), m.modal == open)
				}
				same("the esc that closes the command line")

				// An allowed command runs over each modal, and a refused
				// one says to close the modal first, by its name, and
				// leaves it open.
				m.toast.Clear()
				runCommand(t, m, "set files.markdown=raw")
				if m.cfg.Files.Markdown != "raw" {
					t.Errorf("commands: set over %s didn't run: files.markdown is %q", mod.title, m.cfg.Files.Markdown)
				}
				same("set")
				for _, line := range []string{"goto cli/cli", "search", "find_file", "auth", "history"} {
					m.toast.Clear()
					runCommand(t, m, line)
					name, _, _ := strings.Cut(line, " ")
					want := "Close " + mod.title + " first to use " + name + "."
					if !hasToast(m, want) {
						t.Errorf("commands: %s: toasts %s, want %q", line, toasted(m), want)
					}
					same(line)
				}

				// Over a modal, the config shows in the pager, and
				// backspace returns to the modal as it was.
				runCommand(t, m, "config")
				if m.modal == open || m.modalName(m.modal) != "the pager" {
					t.Fatalf("config: over %s shows %v, want the pager", mod.title, m.modal)
				}
				tap(t, m, "backspace")
				if m.modal != open {
					t.Fatalf("config: backspace left the pager over %s, want it back", mod.title)
				}
				same("the backspace from the config pager")

				// The quit key closes the modal in one press, from any of
				// its views.
				driveKeys(t, m, m.key(press("q")))
				if m.modal != nil {
					t.Errorf("quit: q left the modal open")
				}
			})
		})
	}
}
