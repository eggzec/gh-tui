package tui

import (
	"fmt"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
)

// numberedText returns n lines that say their number.
func numberedText(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

// textLine returns the status of the text modal on view, such as
// "line 1/100".
func textLine(t *testing.T, m *Model) string {
	t.Helper()
	tm, ok := m.modal.(*textModal)
	if !ok {
		t.Fatalf("the modal is %T, want the text modal", m.modal)
	}
	for l := range strings.SplitSeq(ansi.Strip(tm.View()), "\n") {
		if _, rest, ok := strings.Cut(l, "line "); ok && strings.Contains(rest, "/") {
			f := strings.Fields(rest)
			return "line " + f[0]
		}
	}
	t.Fatalf("the text modal shows no status line:\n%s", tm.View())
	return ""
}

// TestReaderKeysFollowTheConfig checks that a pager of the app takes the
// keys its context sets: a rebound key acts and the old one doesn't, an
// unbound one is off, and the option keys come from their own context.
func TestReaderKeysFollowTheConfig(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cfg := config.Default()
		cfg.Keys.Set("text.page_down", []string{"x"})
		cfg.Keys.Set("pager_option.chop", []string{"W"})
		m := newUnboundApp(t, cfg, false)
		driveKeys(t, m, m.openText("go.mod", "go.mod", numberedText(100), true))

		press1 := func(k string) {
			t.Helper()
			msg, _ := keyPress(k)
			driveKeys(t, m, m.key(msg))
		}
		press1("space")
		if got := textLine(t, m); got != "line 1/100" {
			t.Errorf("space moved to %s, want it to do nothing now", got)
		}
		press1("x")
		if got := textLine(t, m); got == "line 1/100" {
			t.Error("x, the page down key, didn't page")
		}

		tm := m.modal.(*textModal)
		if !tm.pager.Wrap() {
			t.Fatal("the text doesn't wrap")
		}
		press1("-")
		press1("S")
		if !tm.pager.Wrap() {
			t.Error("-S chopped the lines, though S names no option here")
		}
		press1("-")
		press1("W")
		if tm.pager.Wrap() {
			t.Error("-W didn't chop the lines")
		}
	})
}

// TestReaderKeysCanBeUnbound checks that a reader's action with no key is
// off, in its keys and in its help, and that its key does nothing.
func TestReaderKeysCanBeUnbound(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cfg := config.Default()
		cfg.Keys.Set("actions_log.follow", []string{})
		m := newUnboundApp(t, cfg, true)
		for _, name := range []string{config.ActionActions, config.ActionNextPane, config.ActionNextPane} {
			msg, _ := keyPress(cfg.Keys.Of(name)[0])
			driveKeys(t, m, m.key(msg))
		}
		if got := layerNames(m.keyLayers()); got != "global, actions, actions_log" {
			t.Fatalf("the keys reach %q, want the log of the Actions modal", got)
		}
		var follow int
		for _, l := range m.keyLayers() {
			for _, b := range l.Bindings {
				if b.Help().Desc != "follow" {
					continue
				}
				follow++
				if b.Enabled() || len(b.Keys()) != 0 {
					t.Errorf("the follow binding has keys %v, enabled %v, want it off with none", b.Keys(), b.Enabled())
				}
			}
		}
		if follow != 1 {
			t.Errorf("help lists follow %d times, want it once, unbound", follow)
		}
	})
}

// TestReaderActionsAreKnown checks that only the actions a reader has can
// be set: a pager has no count.
func TestReaderActionsAreKnown(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Keys.Set("preview.count", []string{"x"})
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "keys.preview.count: unknown action") {
		t.Errorf("Validate() = %v, want keys.preview.count to be an unknown action", err)
	}
}
