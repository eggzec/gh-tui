package tui

import (
	"strings"
	"testing"
	"testing/synctest"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
)

// A user can bind a list's find to another key than the default: the key
// opens a prompt that takes every key, q and : and digits too, enter finds
// and esc closes it.
func TestReboundFindKeyTypesInThePrompt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newKeysAppWith(t, true, func(cfg *config.Config) {
			cfg.Keys.Set("issues.find", []string{"ctrl+g"})
		})
		press := func(name string) {
			t.Helper()
			msg, ok := keyPress(name)
			if !ok {
				t.Fatalf("can't press %q", name)
			}
			driveKeys(t, m, m.key(msg))
		}
		// The third pane of the repository screen is the issues.
		for _, name := range m.cfg.Keys.Of("global.pane_3") {
			if _, ok := keyPress(name); ok {
				press(name)
				break
			}
		}
		screen := func() string { return ansi.Strip(m.View().Content) }

		press("ctrl+g")
		if got := layerNames(m.keyLayers()); !strings.Contains(got, "search_prompt (types)") {
			t.Fatalf("after /, the keys are %q, want the search prompt's", got)
		}
		// Keys the app binds are typed, and neither quit nor open anything.
		for _, name := range []string{"q", ":", "1", "?"} {
			press(name)
		}
		if m.screen != repoScreen || m.helpOpen() || m.line.Focused() {
			t.Fatalf("a typed key acted: screen %d, help open %v, command line %v", m.screen, m.helpOpen(), m.line.Focused())
		}
		if got := screen(); !strings.Contains(got, "/q:1?") {
			t.Errorf("the prompt doesn't show what was typed:\n%s", got)
		}
		press("enter")
		if got := layerNames(m.keyLayers()); strings.Contains(got, "search_prompt") {
			t.Errorf("after enter, the keys are %q, want the prompt closed", got)
		}
		if got := screen(); !strings.Contains(got, "Pattern not found") {
			t.Errorf("a find of text no issue holds doesn't say so:\n%s", got)
		}

		press("ctrl+g")
		for _, r := range "COLLIDE" {
			press(string(r))
		}
		press("enter")
		if got := screen(); !strings.Contains(got, "Pattern not found") {
			t.Errorf("a capital makes the find match case, but it found:\n%s", got)
		}
		press("ctrl+g")
		for _, r := range "collide" {
			press(string(r))
		}
		press("enter")
		if got := screen(); !strings.Contains(got, "/collide  1/1") {
			t.Errorf("the find of the issue's title doesn't show its match:\n%s", got)
		}

		press("ctrl+g")
		press("esc")
		if got := layerNames(m.keyLayers()); strings.Contains(got, "search_prompt") {
			t.Errorf("after esc, the keys are %q, want the prompt closed", got)
		}
		if m.screen != repoScreen {
			t.Errorf("esc left the repository screen")
		}
	})
}
