package tui

import (
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	searchpage "github.com/eggzec/gh-tui/internal/tui/search"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, newKeyMap(config.Default().Keys))
}

// winner names the binding that k reaches in the app's layers, by its
// layer.
func winner(m *Model, k string) string {
	b, src, ok := uitest.Winner(m.keyLayers(), k)
	if !ok {
		return "nothing"
	}
	return src + ": " + b.Help().Desc
}

// The layers take a key in the order the app routes it: a claimed key to
// the section, the app's own before the section's, a modal's and a
// capturing section's before everything but the quit key.
func TestKeyLayersOrder(t *testing.T) {
	m, pulls, _ := newFilterApp(t)
	if got := winner(m, "]"); got != "Pull requests: claimed" {
		t.Errorf("] reaches %q, want the section that claims it", got)
	}
	run(m, m.key(press("]")))
	if m.focus != 1 || !pulls.got(isKey("]")) {
		t.Error("] didn't reach the section that claims it")
	}
	if got := winner(m, "["); got != "app: previous pane" {
		t.Errorf("[ reaches %q, want the app", got)
	}

	m, fakes := newTestApp(t)
	fakes[0].keyMap = backKeys{}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	if got := winner(m, "esc"); got != "Files: back" {
		t.Errorf("esc reaches %q, want the section's back", got)
	}
	run(m, m.key(press("z")))
	if got := winner(m, "esc"); got != "app: unzoom" {
		t.Errorf("esc reaches %q while zoomed, want the unzoom", got)
	}
	run(m, m.key(press("esc")))
	if m.zoom || fakes[0].got(isKey("esc")) {
		t.Error("esc while zoomed didn't unzoom, or reached the section")
	}

	fakes[0].capturing = true
	if got := winner(m, "q"); got != "nothing" {
		t.Errorf("q reaches %q in a capturing section, want it typed", got)
	}
	if got := winner(m, "ctrl+c"); got != "app: quit" {
		t.Errorf("ctrl+c reaches %q in a capturing section, want the quit", got)
	}
	fakes[0].capturing = false

	// Away from the repository screen there are no panes to cycle.
	run(m, m.key(press("n")))
	if m.screen != notifScreen {
		t.Fatalf("n showed screen %d, want the notifications", m.screen)
	}
	if got := winner(m, "tab"); got == "app: next pane" {
		t.Error("the notifications offer the next pane")
	}
	run(m, m.key(press("n")))

	mod := &fakeModal{title: "Preview"}
	run(m, ui.OpenModal(mod))
	if got := winner(m, "x"); got != "Preview: close" {
		t.Errorf("x reaches %q with a modal open, want the modal", got)
	}
	if got := winner(m, "?"); got != "app: help" {
		t.Errorf("? reaches %q with a modal open, want the help", got)
	}
	mod.typing = true
	if got := winner(m, "?"); got != "nothing" {
		t.Errorf("? reaches %q with a modal that types, want it typed", got)
	}
}

// The open command line takes every key, ctrl+c too, so its keys are the
// only ones that reach anything, and it types the rest.
func TestKeyLayersOfTheCommandLine(t *testing.T) {
	m, _ := newTestApp(t)
	run(m, m.key(press(":")))
	layers := m.keyLayers()
	if len(layers) != 1 || layers[0].Source != "command line" || !layers[0].Typing {
		t.Fatalf("the open command line has the layers %+v, want its own alone", layers)
	}
	for k, want := range map[string]string{"esc": "command line: cancel", "ctrl+c": "command line: cancel", "q": "nothing"} {
		if got := winner(m, k); got != want {
			t.Errorf("%s reaches %q with the command line open, want %q", k, got, want)
		}
	}
}

// focusOf names where the focus is: the focused pane of the repository
// screen, the notifications, or the part of the search page that has it,
// by its last layer of keys, and the query it searches for.
func focusOf(m *Model) string {
	switch m.screen {
	case repoScreen:
		return "repo: " + m.focused().section.Title()
	case notifScreen:
		return "notifications"
	case searchScreen:
		layers := m.keyLayers()
		part := map[string]string{"query": "query", "search": "kinds", "results": "results"}[layers[len(layers)-1].Source]
		return fmt.Sprintf("search: %s %q", part, m.srch.section.(*searchpage.Section).Query())
	case dashScreen:
	}
	return fmt.Sprintf("screen %d", m.screen)
}

// Tab, shift+tab, ] and [ cycle the panes of the repository screen, which
// the app does, and reach the section on the notifications and search
// screens, which does what it defines for them: the search page moves
// between its query, kinds and results, and its query types ] and [. The
// notifications have none of them. The help credits each key to what it
// reaches.
func TestNextAndPrevKeysOnEachScreen(t *testing.T) {
	for _, tt := range []struct {
		name string
		repo bool
		// keys reach the screen, where from is the focus.
		keys []string
		from string
		key  string
		// winner is what the help credits key to, and want the focus
		// after it.
		winner, want string
	}{
		{"repo: tab", true, nil, "repo: Files", "tab", "app: next pane", "repo: Pull requests"},
		{"repo: shift+tab", true, nil, "repo: Files", "shift+tab", "app: previous pane", "repo: Issues"},
		{"repo: ]", true, nil, "repo: Files", "]", "app: next pane", "repo: Pull requests"},
		{"repo: [", true, nil, "repo: Files", "[", "app: previous pane", "repo: Issues"},

		{"notifications: tab", false, []string{"n"}, "notifications", "tab", "nothing", "notifications"},
		{"notifications: shift+tab", false, []string{"n"}, "notifications", "shift+tab", "nothing", "notifications"},
		{"notifications: ]", false, []string{"n"}, "notifications", "]", "nothing", "notifications"},
		{"notifications: [", false, []string{"n"}, "notifications", "[", "nothing", "notifications"},

		{"search query: tab", false, []string{"/", "k", "e", "y"}, `search: query "key"`, "tab", "query: next", `search: kinds "key"`},
		{"search query: shift+tab", false, []string{"/", "k", "e", "y"}, `search: query "key"`, "shift+tab", "query: previous", `search: results "key"`},
		{"search query: ]", false, []string{"/", "k", "e", "y"}, `search: query "key"`, "]", "nothing", `search: query "key]"`},
		{"search query: [", false, []string{"/", "k", "e", "y"}, `search: query "key"`, "[", "nothing", `search: query "key["`},

		{"search kinds: tab", false, []string{"/", "k", "e", "y", "up"}, `search: kinds "key"`, "tab", "search: next", `search: results "key"`},
		{"search kinds: shift+tab", false, []string{"/", "k", "e", "y", "up"}, `search: kinds "key"`, "shift+tab", "search: previous", `search: query "key"`},
		{"search kinds: ]", false, []string{"/", "k", "e", "y", "up"}, `search: kinds "key"`, "]", "search: next", `search: results "key"`},
		{"search kinds: [", false, []string{"/", "k", "e", "y", "up"}, `search: kinds "key"`, "[", "search: previous", `search: query "key"`},

		{"search results: tab", false, []string{"/", "k", "e", "y", "enter"}, `search: results "key"`, "tab", "search: next", `search: query "key"`},
		{"search results: shift+tab", false, []string{"/", "k", "e", "y", "enter"}, `search: results "key"`, "shift+tab", "search: previous", `search: kinds "key"`},
		{"search results: ]", false, []string{"/", "k", "e", "y", "enter"}, `search: results "key"`, "]", "search: next", `search: query "key"`},
		{"search results: [", false, []string{"/", "k", "e", "y", "enter"}, `search: results "key"`, "[", "search: previous", `search: kinds "key"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Let the clock that regexp2 starts for the highlighting
				// run out, so the bubble ends with nothing left running.
				defer time.Sleep(time.Hour)
				m := newKeysApp(t, tt.repo)
				for _, k := range tt.keys {
					msg, _ := keyPress(k)
					driveKeys(t, m, m.key(msg))
				}
				if got := focusOf(m); got != tt.from {
					t.Fatalf("the keys reach %s, want %s", got, tt.from)
				}
				if got := winner(m, tt.key); got != tt.winner {
					t.Errorf("the help credits %s to %q, want %q", tt.key, got, tt.winner)
				}
				before := onScreen(m)
				msg, _ := keyPress(tt.key)
				driveKeys(t, m, m.key(msg))
				if got := focusOf(m); got != tt.want {
					t.Errorf("%s moved the focus to %s, want %s", tt.key, got, tt.want)
				}
				if tt.from == "notifications" && onScreen(m) != before {
					t.Errorf("%s changed the notifications, which have no use for it", tt.key)
				}
			})
		})
	}
}
