package tui

import (
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

// listFindCase is a list whose default keys open a find and a quick filter.
type listFindCase struct {
	// name is the name of the context in keyContexts that reaches the list.
	name string
	// zoom zooms the list first, with the key of the zoom.
	zoom bool
	// text is a part of the text of the row the list holds.
	text string
}

var listFindCases = []listFindCase{
	{name: "dashboard: repositories", text: "gh"},
	{name: "dashboard: zoomed", text: "gh"},
	{name: "pull requests", text: "e"},
	{name: "issues", text: "e"},
	{name: "issues", zoom: true, text: "e"},
	{name: "notifications", text: "e"},
	{name: "search: results", text: "e"},
	{name: "owner: repositories", text: "gh"},
	{name: "actions: runs", text: "e"},
}

func (c listFindCase) context(t *testing.T) keyContext {
	t.Helper()
	for _, kc := range keyContexts() {
		if kc.name == c.name {
			if c.zoom {
				kc.steps = append(slices.Clone(kc.steps), "global.zoom")
			}
			return kc
		}
	}
	t.Fatalf("no key context named %q", c.name)
	return keyContext{}
}

// Every list binds / to find, n and N to step between the finds, and & to
// a quick filter by default, whose prompts type every key and whose esc
// clears what they showed, with the list zoomed too.
func TestListsFindAndQuickFilterByDefault(t *testing.T) {
	for _, c := range listFindCases {
		name := c.name
		if c.zoom {
			name += " zoomed"
		}
		t.Run(name, func(t *testing.T) {
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
				prompting := func() bool { return strings.Contains(layerNames(m.keyLayers()), "search_prompt (types)") }
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
				zoomed, onScreen := m.zoomed(), m.screen

				// The prompt types what the app binds, and esc closes it.
				for _, open := range []string{"/", "&"} {
					press(open)
					if !prompting() {
						t.Fatalf("after %s, the keys are %q, want the search prompt's", open, layerNames(m.keyLayers()))
					}
					press("q", ":", "?")
					if m.screen != onScreen || m.helpOpen() || m.line.Focused() {
						t.Fatalf("after %s, a typed key acted", open)
					}
					press("esc")
					if prompting() || m.zoomed() != zoomed {
						t.Fatalf("after %s and esc, the prompt is open %v, zoomed %v", open, prompting(), m.zoomed())
					}
				}

				// A find shows its match, n and N move between matches, and
				// esc clears it.
				press("/")
				press(strings.Split(c.text, "")...)
				press("enter")
				if want := "/" + c.text + "  1/1"; !strings.Contains(screen(), want) {
					t.Fatalf("after the find of %q the screen lacks %q:\n%s", c.text, want, screen())
				}
				if !enabled("next match") || !enabled("prev match") {
					t.Errorf("after a find, next and prev match are not enabled in %q", layerNames(m.keyLayers()))
				}
				press("n", "N")
				if prompting() || !strings.Contains(screen(), "/"+c.text+"  1/1") {
					t.Errorf("n and N lost the find:\n%s", screen())
				}
				press("esc")
				if strings.Contains(screen(), "/"+c.text+"  1/1") || m.zoomed() != zoomed {
					t.Errorf("esc left the find or changed the zoom (zoomed %v):\n%s", m.zoomed(), screen())
				}

				// A quick filter shows its chip, and esc clears it before it
				// does anything else.
				press("&")
				press(strings.Split(c.text, "")...)
				press("enter")
				chip := "&" + c.text
				if !strings.Contains(screen(), chip+"   1 of 1") {
					t.Fatalf("after the filter of %q the screen lacks its chip:\n%s", c.text, screen())
				}
				press("esc")
				if strings.Contains(screen(), chip+"  ") {
					t.Errorf("esc left the filter:\n%s", screen())
				}
				if m.zoomed() != zoomed || m.screen != onScreen {
					t.Errorf("the esc that cleared the filter changed the zoom (%v) or the screen", m.zoomed())
				}
			})
		})
	}
}

// With two rows that hold the text, a find says 1/2, n moves to 2/2, and N
// back to 1/2.
func TestFindStepsBetweenTwoMatches(t *testing.T) {
	second := keyIssue
	second.ID, second.Number, second.Title = "I_3", 3, "Keys again"
	keyIssueRows = []core.Issue{keyIssue, second}
	t.Cleanup(func() { keyIssueRows = []core.Issue{keyIssue} })
	synctest.Test(t, func(t *testing.T) {
		c := listFindCase{name: "issues"}.context(t)
		m, _ := c.reach(t)
		press := func(names ...string) {
			t.Helper()
			for _, n := range names {
				msg, _ := keyPress(n)
				driveKeys(t, m, m.key(msg))
			}
		}
		screen := func() string { return ansi.Strip(m.View().Content) }
		press("/", "k", "e", "y", "s", "enter")
		for _, step := range []struct{ key, want string }{{"", "/keys  1/2"}, {"n", "/keys  2/2"}, {"N", "/keys  1/2"}} {
			if step.key != "" {
				press(step.key)
			}
			if !strings.Contains(screen(), step.want) {
				t.Fatalf("after %q the screen lacks %q:\n%s", step.key, step.want, screen())
			}
		}
	})
}
