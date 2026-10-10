package tui

import (
	"strings"
	"testing"
	"testing/synctest"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// markCases are the lists whose rows the user can mark, with a part of the
// text of the row each holds.
var markCases = []listFindCase{
	{name: "pull requests", text: "e"},
	{name: "issues", text: "e"},
	{name: "notifications", text: "e"},
}

// Space marks the row under the cursor, in the lists of pull requests,
// issues and notifications, and unmarks it; the marked rows show their mark
// and the last line counts them. Esc clears the find, then the filter, then
// the marks, and help lists the mark key, and the esc that clears the marks
// while some are set.
func TestListsMarkRows(t *testing.T) {
	t.Parallel()
	for _, c := range markCases {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m, _ := c.context(t).reach(t)
				press := func(names ...string) {
					t.Helper()
					for _, n := range names {
						msg, ok := keyPress(n)
						if !ok {
							t.Fatalf("can't press %q", n)
						}
						driveKeys(t, m, m.key(msg))
					}
				}
				screen := func() string { return ansi.Strip(m.View().Content) }
				enabled := func(desc string) bool {
					for _, l := range m.keyLayers() {
						for _, b := range l.Bindings {
							if b.Help().Desc == desc && b.Enabled() {
								return true
							}
						}
					}
					return false
				}
				glyph := ui.NewIcons(m.cfg.UI.Icons).Marked
				marked := func() bool {
					return strings.Contains(screen(), " "+glyph+" ") || strings.Contains(screen(), "▌"+glyph+" ")
				}

				if !enabled("mark") {
					t.Fatalf("help lacks the mark key in %q", layerNames(m.keyLayers()))
				}
				if enabled("clear marks") || marked() || strings.Contains(screen(), "marked") {
					t.Fatalf("a row is marked at the start:\n%s", screen())
				}

				press("space")
				if !strings.Contains(screen(), "1 marked") || !strings.Contains(screen(), "▌"+glyph+" ") {
					t.Fatalf("after space the screen lacks the mark and its count:\n%s", screen())
				}
				if !enabled("clear marks") {
					t.Errorf("help lacks esc, clear marks, with a row marked: %q", layerNames(m.keyLayers()))
				}
				press("space")
				if marked() || strings.Contains(screen(), "marked") || enabled("clear marks") {
					t.Fatalf("a second space left the row marked:\n%s", screen())
				}

				// Esc clears the find, then the filter, then the marks.
				press("space")
				press("/")
				press(strings.Split(c.text, "")...)
				press("enter")
				press("&")
				press(strings.Split(c.text, "")...)
				press("enter")
				if s := screen(); !strings.Contains(s, "1 marked") || !strings.Contains(s, "&"+c.text) || !strings.Contains(s, "/"+c.text) {
					t.Fatalf("the find, the filter and the mark don't all show:\n%s", s)
				}
				press("esc")
				if s := screen(); strings.Contains(s, "/"+c.text+"  ") || !strings.Contains(s, "&"+c.text) || !strings.Contains(s, "1 marked") {
					t.Fatalf("the first esc should clear the find only:\n%s", s)
				}
				press("esc")
				if s := screen(); strings.Contains(s, "&"+c.text) || !strings.Contains(s, "1 marked") {
					t.Fatalf("the second esc should clear the filter only:\n%s", s)
				}
				press("esc")
				if s := screen(); strings.Contains(s, "marked") || marked() {
					t.Fatalf("the third esc should clear the marks:\n%s", s)
				}
			})
		})
	}
}

// With ui.icons set to ascii the mark is drawn in ASCII too.
func TestListMarkASCII(t *testing.T) {
	t.Parallel()
	for _, c := range markCases {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m, _ := c.context(t).reach(t)
				runCommand(t, m, "set ui.icons="+config.IconsASCII)
				msg, _ := keyPress("space")
				driveKeys(t, m, m.key(msg))
				ic := ui.NewIcons(config.IconsASCII)
				if want := ic.Cursor + ic.Marked + " "; !strings.Contains(ansi.Strip(m.View().Content), want) {
					t.Errorf("the screen lacks the ASCII mark %q:\n%s", want, ansi.Strip(m.View().Content))
				}
			})
		})
	}
}
