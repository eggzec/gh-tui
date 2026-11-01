package treeview

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

// testKeys are the keys the tests of the tree bind, as an app would.
var testKeys = map[string][]string{
	"up":             {"up", "k"},
	"down":           {"down", "j"},
	"page_up":        {"ctrl+b", "pgup"},
	"page_down":      {"ctrl+f", "pgdown"},
	"half_page_up":   {"ctrl+u"},
	"half_page_down": {"ctrl+d"},
	"top":            {"g", "home"},
	"bottom":         {"G", "end"},
	"global.select":  {"enter"},
	"expand":         {"right", "l"},
	"collapse":       {"left", "h"},
	"toggle_all":     {"*"},
	"mark":           {"space"},
}

var testKeyMap = NewKeyMap(keytest.Table(testKeys))

// newTree makes a focused tree with the keys of a pane, as the app does.
func newTree(rows []Node, opts ...Option) Model {
	return New(Source{}, append([]Option{WithKeyMap(testKeyMap), WithFocused(true), WithRows(rows)}, opts...)...)
}

// press makes the key press of a key name the tests use.
func press(k string) tea.KeyPressMsg {
	switch k {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	if c, ok := strings.CutPrefix(k, "ctrl+"); ok {
		r, _ := utf8.DecodeRuneInString(c)
		return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// keys presses each key and returns the model and the messages the commands
// sent.
func keys(tb testing.TB, m Model, ks ...string) (next Model, msgs []tea.Msg) {
	tb.Helper()
	for _, k := range ks {
		var cmd tea.Cmd
		m, cmd = m.Update(press(k))
		if cmd != nil {
			msgs = append(msgs, cmd())
		}
	}
	return m, msgs
}

// visible returns the IDs of the rows that show.
func visible(m Model) []string {
	ids := make([]string, 0, m.Len())
	for i := range m.vis {
		ids = append(ids, m.row(i).ID)
	}
	return ids
}

func selected(m Model) string {
	n, _ := m.Selected()
	return n.ID
}

// leaf and fold make rows of the given depth, whose text is their ID.
func leaf(id string, depth int) Node {
	return Node{ID: id, Depth: depth, Lines: []string{id}}
}

func fold(id string, depth int) Node {
	return Node{ID: id, Depth: depth, Lines: []string{id}, Branch: true, Closed: true}
}

// open returns r as a fold that starts open.
func open(r Node) Node {
	r.Closed = false
	return r
}

// kind returns r with a kind.
func kind(r Node, k string) Node {
	r.Kind = k
	return r
}

// files is a small tree of folders and files in the shape the Files pane
// has, with everything open:
//
//	Files
//	├─ cmd
//	│  ╰─ app
//	│     ├─ flags.go
//	│     ╰─ main.go
//	╰─ README.md
//	Notes
//	╰─ todo
func files() []Node {
	return []Node{
		open(fold("Files", 0)),
		open(fold("cmd", 1)),
		open(fold("app", 2)),
		kind(leaf("flags.go", 3), "file"),
		kind(leaf("main.go", 3), "file"),
		kind(leaf("README.md", 1), "file"),
		open(fold("Notes", 0)),
		leaf("todo", 1),
	}
}

// plain strips the styles of a view.
func plain(v string) string { return ansi.Strip(v) }

// assertFits checks that v is exactly height lines of exactly width cells.
func assertFits(tb testing.TB, v string, width, height int) {
	tb.Helper()
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		tb.Fatalf("view has %d lines, want %d", len(lines), height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			tb.Fatalf("line %d is %d cells wide, want %d: %q", i, w, width, l)
		}
	}
}
