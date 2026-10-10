package tui

import (
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// tapAction presses the first key of action, and runs what follows.
func tapAction(t *testing.T, m *Model, action string) {
	t.Helper()
	msg, ok := keyPress(m.cfg.Keys.Of(action)[0])
	if !ok {
		t.Fatalf("can't press %s", action)
	}
	driveKeys(t, m, m.key(msg))
}

// runReferences opens the command line, and runs the references command.
func runReferences(t *testing.T, m *Model) {
	t.Helper()
	tapAction(t, m, config.ActionCommand)
	if !m.line.Focused() {
		t.Fatal("the command key didn't open the line")
	}
	for _, r := range "references" {
		driveKeys(t, m, m.key(press(string(r))))
	}
	driveKeys(t, m, m.key(enter))
}

// The links of a pull request, picked, replace its modal, and the back key
// returns to them as they were left; the next one returns to the
// conversation, and esc closes the modal.
func TestReferencesBackReturnsToTheLinks(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, true)
		driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 2} })
		pr := m.modal
		if pr == nil {
			t.Fatal("no pull request opened")
		}
		thread := pr.View()
		tapAction(t, m, "pull_modal.references")
		if got := layerNames(m.keyLayers()); got != "global, references" {
			t.Fatalf("the keys after # reach %q, want the links'", got)
		}
		driveKeys(t, m, m.key(press("*")))
		driveKeys(t, m, m.key(press("j")))
		driveKeys(t, m, m.key(press("j")))
		links := ansi.Strip(pr.View())
		if !strings.Contains(links, "Closes (1)") || !strings.Contains(links, "Mentioned in (3)") {
			t.Fatalf("the modal doesn't show the links:\n%s", links)
		}

		// Enter on the item opens it in place of the modal.
		driveKeys(t, m, m.key(enter))
		if m.modal == nil || m.modal == pr {
			t.Fatal("enter didn't open the item in place of the modal")
		}
		item := m.modal
		// Back returns to the links, with the cursor where it was.
		tapAction(t, m, config.ActionBack)
		if m.modal != pr {
			t.Fatalf("back returned to %v, want the pull request", m.topModal())
		}
		if got := ansi.Strip(pr.View()); got != links {
			t.Errorf("the links changed while the item was open:\n%s\n---\n%s", links, got)
		}
		if d, ok := item.(ui.Discarder); ok {
			d.Discard()
		}
		// Back again steps out of the links, to the tab they covered.
		tapAction(t, m, config.ActionBack)
		if m.modal != pr || layerNames(m.keyLayers()) != "global, pull_modal, pull_overview" {
			t.Fatalf("the second back left the keys at %q", layerNames(m.keyLayers()))
		}
		if got := pr.View(); got != thread {
			t.Errorf("the thread changed while the links showed:\n%s", got)
		}
		// Back with nothing behind it does nothing, and esc closes.
		tapAction(t, m, config.ActionBack)
		if m.modal != pr {
			t.Error("back at the top of the modal closed it")
		}
		tapAction(t, m, config.ActionDismiss)
		if m.modal != nil {
			t.Error("esc didn't close the modal")
		}
	})
}

// esc clears the filter of the links first, and then closes the modal; q
// closes it at once.
func TestReferencesEscThenClose(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, true)
		driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 1} })
		if m.modal == nil {
			t.Fatal("no issue opened")
		}
		tapAction(t, m, "issue_modal.references")
		tapAction(t, m, "references.quick_filter")
		for _, r := range "mention" {
			driveKeys(t, m, m.key(press(string(r))))
		}
		driveKeys(t, m, m.key(enter))
		if v := ansi.Strip(m.modal.View()); !strings.Contains(v, "& mention") {
			t.Fatalf("the filter doesn't show:\n%s", v)
		}
		tapAction(t, m, config.ActionDismiss)
		if m.modal == nil {
			t.Fatal("the first esc closed the modal; it clears the filter")
		}
		if v := ansi.Strip(m.modal.View()); strings.Contains(v, "& mention") {
			t.Errorf("the filter is still there:\n%s", v)
		}
		tapAction(t, m, config.ActionDismiss)
		if m.modal != nil {
			t.Error("the second esc didn't close the modal")
		}

		driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 1} })
		tapAction(t, m, "issue_modal.references")
		tapAction(t, m, config.ActionQuit)
		if m.modal != nil {
			t.Error("q over the links didn't close the modal")
		}
	})
}

// The filter types every key, backspace and q included, which the app
// doesn't take as intents while it is open.
func TestReferencesFilterTypesKeys(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, true)
		driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 2} })
		pr := m.modal
		tapAction(t, m, "pull_modal.references")
		tapAction(t, m, "references.quick_filter")
		for _, r := range "qx" {
			driveKeys(t, m, m.key(press(string(r))))
		}
		if m.modal != pr {
			t.Fatal("q closed the modal while the filter types")
		}
		tapAction(t, m, config.ActionBack)
		if m.modal != pr {
			t.Fatal("backspace stepped out of the links while the filter types")
		}
		if got := layerNames(m.keyLayers()); got != "always, search_prompt (types)" {
			t.Errorf("the keys are %q", got)
		}
	})
}

// The command shows the links of the modal open, with the key unbound
// too, and refuses elsewhere.
func TestReferencesCommand(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysAppWith(t, true, func(c *config.Config) {
			c.Keys.Set("pull_modal.references", []string{})
		})
		runReferences(t, m)
		if !hasToast(m, "references works in the PR and issue modals.") {
			t.Errorf("toast = %q, want the refusal", toasted(m))
		}
		driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 2} })
		pr := m.modal
		runReferences(t, m)
		if got := layerNames(m.keyLayers()); m.modal != pr || got != "global, references" {
			t.Fatalf("the command left the keys at %q", got)
		}
		// Again it changes nothing.
		runReferences(t, m)
		if got := layerNames(m.keyLayers()); got != "global, references" {
			t.Errorf("the command again left the keys at %q", got)
		}

		driveKeys(t, m, m.key(press("esc")))
		driveKeys(t, m, m.key(press("esc")))
		driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 1} })
		runReferences(t, m)
		if got := layerNames(m.keyLayers()); got != "global, references" {
			t.Errorf("over an issue the command left the keys at %q", got)
		}
	})
}

// Over a modal that doesn't name it, the command is refused as the others
// are.
func TestReferencesCommandOverOtherModals(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, true)
		m.openModal(&sourceModal{title: "README.md", renders: true})
		runReferences(t, m)
		if want := "Close the file first to use references."; !hasToast(m, want) {
			t.Errorf("toast = %q, want %q", toasted(m), want)
		}
	})
}

// The command is one the modals of a pull request and an issue name.
func TestReferencesCommandIsNamed(t *testing.T) {
	t.Parallel()
	c, ok := findCommand(ui.CommandReferences)
	if !ok || c.over != overNamed {
		t.Fatalf("the references command is %+v", c)
	}
}

// The colon is text where the keys type it: in the filter of the links, in
// a search or an option of a log, and it opens no line over a question.
func TestReferencesColonIsTypedWhereKeysType(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, true)
		driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 2} })
		tapAction(t, m, "pull_modal.references")
		tapAction(t, m, "references.quick_filter")
		tapAction(t, m, config.ActionCommand)
		if m.line.Focused() {
			t.Error(": opened the line over the filter of the links")
		}
		if got := m.modal.View(); !strings.Contains(ansi.Strip(got), "&:") {
			t.Errorf("the colon wasn't typed into the filter:\n%s", ansi.Strip(got))
		}
	})
	for _, steps := range [][]string{
		{"pull_check_log.find"},
		{"pull_check_log.option"},
		{"pull_check_log.rerun_failed"},
	} {
		synctest.Test(t, func(t *testing.T) {
			m := newKeysApp(t, true)
			driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 2, Checks: true} })
			tapAction(t, m, config.ActionSelect)
			for _, s := range steps {
				tapAction(t, m, s)
			}
			tapAction(t, m, config.ActionCommand)
			if m.line.Focused() {
				t.Errorf("%v: : opened the line", steps)
			}
		})
	}
}

// A pull request or issue modal that shows its links is still the modal of
// the pull request or issue: going back to it keeps ui.maximized, and the
// refusals name it.
func TestReferencesModalKeepsItsContext(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysAppWith(t, true, func(c *config.Config) { c.UI.Maximized = []string{"pull_modal"} })
		driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 2} })
		pr := m.modal
		if !m.maximized {
			t.Fatal("the pull request didn't open maximized")
		}
		tapAction(t, m, "pull_modal.references")
		if got := modalContext(pr); got != "pull_modal" {
			t.Errorf("modalContext of the links = %q, want pull_modal", got)
		}
		if got := m.modalName(pr); got != "the pull request" {
			t.Errorf("the links are named %q", got)
		}
		tapAction(t, m, config.ActionOwner)
		if want := "Close the pull request first to use the owner page."; !hasToast(m, want) {
			t.Errorf("toast = %q, want %q", toasted(m), want)
		}
		// Open an item in place of the modal, which isn't maximized, and
		// come back.
		driveKeys(t, m, m.key(enter))
		if m.modal == pr || m.maximized {
			t.Fatalf("the item opened as %v, maximized %v", m.topModal(), m.maximized)
		}
		tapAction(t, m, config.ActionBack)
		if m.modal != pr || !m.maximized {
			t.Errorf("back returned to %v, maximized %v; want the pull request, maximized", m.topModal(), m.maximized)
		}
		// The same for an issue.
		tapAction(t, m, config.ActionDismiss)
		driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 1} })
		tapAction(t, m, "issue_modal.references")
		if got := m.modalName(m.modal); got != "the issue" {
			t.Errorf("the links of an issue are named %q", got)
		}
	})
}
