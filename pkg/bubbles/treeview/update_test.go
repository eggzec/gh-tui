package treeview

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestKeys(t *testing.T) {
	closedFiles := []Node{fold("Files", 0), leaf("README.md", 1), kind(leaf("x", 1), "file")}
	acts := open(fold("Files", 0))
	acts.Action = true

	tests := []struct {
		name     string
		rows     []Node
		keys     []string
		sel      string
		visible  []string
		messages []tea.Msg
	}{
		{
			name: "l opens a closed fold and stays on it",
			rows: closedFiles, keys: []string{"l"},
			sel: "Files", visible: []string{"Files", "README.md", "x"},
		},
		{
			name: "l on an open fold goes to its first child",
			rows: closedFiles, keys: []string{"l", "l"},
			sel: "README.md", visible: []string{"Files", "README.md", "x"},
		},
		{
			name: "l on a leaf does nothing",
			rows: closedFiles, keys: []string{"l", "j", "l"},
			sel: "README.md", visible: []string{"Files", "README.md", "x"},
		},
		{
			name: "l skips a child the cursor may not rest on",
			rows: []Node{open(fold("a", 0)), {ID: "rule", Depth: 1, Lines: []string{"--"}, Skip: true}, leaf("b", 1)},
			keys: []string{"l"}, sel: "b", visible: []string{"a", "rule", "b"},
		},
		{
			name: "h closes an open fold and stays on it",
			rows: files(), keys: []string{"h"},
			sel: "Files", visible: []string{"Files", "Notes", "todo"},
		},
		{
			name: "h on a leaf goes to its parent",
			rows: files(), keys: []string{"G", "h"},
			sel: "Notes", visible: visible(newTree(files())),
		},
		{
			name: "h on a closed fold goes to its parent",
			rows: files(), keys: []string{"j", "j", "h", "h"},
			sel: "cmd", visible: []string{"Files", "cmd", "app", "README.md", "Notes", "todo"},
		},
		{
			name: "h on a top-level row that is not open does nothing",
			rows: closedFiles, keys: []string{"h"},
			sel: "Files", visible: []string{"Files"},
		},
		{
			name: "h on a top-level leaf does nothing",
			rows: []Node{leaf("a", 0), leaf("b", 0)}, keys: []string{"j", "h"},
			sel: "b", visible: []string{"a", "b"},
		},
		{
			name: "enter on a closed fold reports it and never opens it",
			rows: closedFiles, keys: []string{"enter"},
			sel: "Files", visible: []string{"Files"},
			messages: []tea.Msg{OpenMsg{ID: "Files"}},
		},
		{
			name: "enter on an open fold reports it and never closes it",
			rows: files(), keys: []string{"enter"},
			sel: "Files", visible: visible(newTree(files())),
			messages: []tea.Msg{OpenMsg{ID: "Files"}},
		},
		{
			name: "enter on a fold with an action reports it and keeps the fold",
			rows: []Node{acts, leaf("c", 1)}, keys: []string{"enter"},
			sel: "Files", visible: []string{"Files", "c"},
			messages: []tea.Msg{OpenMsg{ID: "Files"}},
		},
		{
			name: "enter on a leaf reports it",
			rows: closedFiles, keys: []string{"l", "j", "enter"},
			sel: "README.md", visible: []string{"Files", "README.md", "x"},
			messages: []tea.Msg{OpenMsg{ID: "README.md"}},
		},
		{
			name: "star opens every fold when one is closed",
			rows: []Node{open(fold("a", 0)), leaf("a1", 1), fold("b", 0), leaf("b1", 1), fold("b2", 1), leaf("b21", 2)},
			keys: []string{"*"}, sel: "a",
			visible: []string{"a", "a1", "b", "b1", "b2", "b21"},
		},
		{
			name: "star closes every fold when all show open",
			rows: files(), keys: []string{"*"},
			sel: "Files", visible: []string{"Files", "Notes"},
		},
		{
			name: "star closing moves the cursor to the fold that hides it",
			rows: files(), keys: []string{"G", "*"},
			sel: "Notes", visible: []string{"Files", "Notes"},
		},
		{
			name: "star does nothing without folds",
			rows: []Node{leaf("a", 0), leaf("b", 0)}, keys: []string{"*"},
			sel: "a", visible: []string{"a", "b"},
		},
		{
			name: "space reports the row and moves to the next of its kind",
			rows: files(), keys: []string{"j", "j", "j", "space"},
			sel: "main.go", visible: visible(newTree(files())),
			messages: []tea.Msg{MarkMsg{ID: "flags.go", Kind: "file"}},
		},
		{
			name: "space reveals the folder of the next row of its kind",
			rows: []Node{kind(leaf("a", 0), "file"), fold("dir", 0), kind(leaf("b", 1), "file")},
			keys: []string{"space"}, sel: "b", visible: []string{"a", "dir", "b"},
			messages: []tea.Msg{MarkMsg{ID: "a", Kind: "file"}},
		},
		{
			name: "space skips rows of another kind",
			rows: []Node{kind(leaf("a", 0), "file"), kind(leaf("c", 0), "commit"), kind(leaf("b", 0), "file")},
			keys: []string{"space"}, sel: "b", visible: []string{"a", "c", "b"},
			messages: []tea.Msg{MarkMsg{ID: "a", Kind: "file"}},
		},
		{
			name: "space on the last of its kind stays",
			rows: []Node{kind(leaf("a", 0), "file"), leaf("z", 0)},
			keys: []string{"space"}, sel: "a", visible: []string{"a", "z"},
			messages: []tea.Msg{MarkMsg{ID: "a", Kind: "file"}},
		},
		{
			name: "space on a row with no kind says so",
			rows: files(), keys: []string{"space"},
			sel: "Files", visible: visible(newTree(files())),
			messages: []tea.Msg{NoMarkMsg{ID: "Files"}},
		},
		{
			name: "j and k skip rows the cursor may not rest on",
			rows: []Node{leaf("a", 0), {ID: "rule", Lines: []string{"--"}, Skip: true}, leaf("b", 0)},
			keys: []string{"j", "j", "k", "k", "k"}, sel: "a", visible: []string{"a", "rule", "b"},
		},
		{
			name: "G and g go to the ends over rows the cursor may not rest on",
			rows: []Node{{ID: "t", Lines: []string{"--"}, Skip: true}, leaf("a", 0), leaf("b", 0), {ID: "e", Lines: []string{"--"}, Skip: true}},
			keys: []string{"G", "G", "g", "G"}, sel: "b", visible: []string{"t", "a", "b", "e"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTree(tt.rows)
			m, msgs := keys(t, m, tt.keys...)
			if got := selected(m); got != tt.sel {
				t.Errorf("cursor on %q, want %q", got, tt.sel)
			}
			if tt.visible != nil {
				if got := visible(m); !slices.Equal(got, tt.visible) {
					t.Errorf("visible = %v, want %v", got, tt.visible)
				}
			}
			if len(msgs) != len(tt.messages) {
				t.Fatalf("messages = %v, want %v", msgs, tt.messages)
			}
			for i, want := range tt.messages {
				if got := withoutTree(msgs[i]); !reflect.DeepEqual(got, want) {
					t.Errorf("message %d = %#v, want %#v", i, got, want)
				}
			}
		})
	}
}

// withoutTree clears the tree of a message, which a test cannot know.
func withoutTree(msg tea.Msg) tea.Msg {
	switch msg := msg.(type) {
	case OpenMsg:
		msg.Tree, msg.Node = 0, Node{}
		return msg
	case MarkMsg:
		msg.Tree = 0
		return msg
	case NoMarkMsg:
		msg.Tree = 0
		return msg
	}
	return msg
}

// A message names the tree that sent it, so two trees never read each
// other's.
func TestMessagesNameTheirTree(t *testing.T) {
	a, b := newTree(files()), newTree(files())
	_, am := keys(t, a, "space")
	_, bm := keys(t, b, "space")
	if got := am[0].(NoMarkMsg).Tree; got != a.ID() {
		t.Errorf("message names tree %d, want %d", got, a.ID())
	}
	if a.ID() == b.ID() || bm[0].(NoMarkMsg).Tree != b.ID() {
		t.Errorf("trees share an identity: %d and %d", a.ID(), b.ID())
	}
}

func TestBlurredIgnoresKeys(t *testing.T) {
	m := newTree(files())
	m.Blur()
	m, msgs := keys(t, m, "j", "enter", "space", "*")
	if selected(m) != "Files" || len(msgs) != 0 || len(m.vis) != len(files()) {
		t.Errorf("a blurred tree reacted: on %q, %v", selected(m), msgs)
	}
}

func TestNoKeyMapBindsNothing(t *testing.T) {
	m := New(Source{}, WithFocused(true), WithRows(files()))
	m, msgs := keys(t, m, "j", "l", "h", "enter", "space", "*", "G")
	if selected(m) != "Files" || len(msgs) != 0 || m.Len() != len(files()) {
		t.Errorf("a tree with no keys reacted: on %q, %v", selected(m), msgs)
	}
	for _, g := range m.KeyMap().FullHelp() {
		for _, b := range g {
			if b.Enabled() || b.Help().Desc == "" {
				t.Errorf("%q: enabled %v, keys %v", b.Help().Desc, b.Enabled(), b.Keys())
			}
		}
	}
}

func TestEmptyTreeTakesKeys(t *testing.T) {
	m := newTree(nil)
	m, msgs := keys(t, m, "j", "k", "l", "h", "enter", "space", "*", "G", "pgdown")
	if _, ok := m.Selected(); ok || len(msgs) != 0 {
		t.Errorf("an empty tree selected a row or sent %v", msgs)
	}
}

func TestSetRowsKeepsTheCursorOnItsRow(t *testing.T) {
	rows := []Node{leaf("a", 0), leaf("b", 0), leaf("c", 0)}
	tests := []struct {
		name string
		next []Node
		want string
	}{
		{"row inserted above", []Node{leaf("new", 0), leaf("a", 0), leaf("b", 0), leaf("c", 0)}, "b"},
		{"rows reordered", []Node{leaf("c", 0), leaf("a", 0), leaf("b", 0)}, "b"},
		{"row removed goes to the one now in its place", []Node{leaf("a", 0), leaf("c", 0)}, "c"},
		{"last row removed goes to the new last", []Node{leaf("a", 0)}, "a"},
		{"all rows replaced", []Node{leaf("x", 0), leaf("y", 0)}, "y"},
		{"no rows", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := keys(t, newTree(rows), "j")
			m.SetRows(tt.next)
			if got := selected(m); got != tt.want {
				t.Errorf("cursor on %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetRowsMovesTheCursorToTheFoldThatHidesItsRow(t *testing.T) {
	m, _ := keys(t, newTree(files()), "j", "j", "j")
	if selected(m) != "flags.go" {
		t.Fatalf("cursor on %q", selected(m))
	}
	m.SetRows(files())
	if selected(m) != "flags.go" {
		t.Errorf("cursor on %q after a refresh, want flags.go", selected(m))
	}
	// The next data puts the row under a new fold that starts closed.
	next := files()
	next = slices.Insert(next, 3, fold("wrap", 3))
	next[4].Depth, next[5].Depth = 4, 4
	m.SetRows(next)
	if selected(m) != "wrap" {
		t.Errorf("cursor on %q, want the fold that hides flags.go", selected(m))
	}
}

func TestSetRowsKeepsTheFoldsTheUserLeft(t *testing.T) {
	m, _ := keys(t, newTree(files()), "h") // closes Files
	m.SetRows(files())                     // the data says it is open
	if !m.Folded("Files") || m.Len() != 3 {
		t.Errorf("Files reopened by a refresh: folded %v, %d rows", m.Folded("Files"), m.Len())
	}
	rows := files()
	rows = append(rows, fold("Later", 0), leaf("later1", 1))
	m.SetRows(rows)
	if !m.Folded("Later") {
		t.Error("a new fold did not start in the state its row gave")
	}
	m.SetRows(files()) // Later is gone, then back
	m.SetRows(rows)
	if !m.Folded("Later") {
		t.Error("a fold that came back did not start in the state its row gave")
	}
}

func TestSetRowsDoesNotAliasTheCallersRows(t *testing.T) {
	rows := files()
	m := newTree(rows)
	rows[0].ID = "changed"
	if n, _ := m.Selected(); n.ID != "Files" {
		t.Errorf("the tree reads the caller's rows: cursor on %q", n.ID)
	}
}

func TestModelsDoNotShareFolds(t *testing.T) {
	a := newTree(files())
	b := a
	b, _ = keys(t, b, "h")
	if a.Folded("Files") || !b.Folded("Files") {
		t.Errorf("a copy changed its original's folds: %v %v", a.Folded("Files"), b.Folded("Files"))
	}
}

func TestReveal(t *testing.T) {
	m := newTree([]Node{fold("a", 0), fold("b", 1), leaf("c", 2), leaf("d", 0)}, WithSize(20, 2))
	if ok, _ := m.Reveal("c"); !ok {
		t.Fatal("Reveal(c) = false")
	}
	if selected(m) != "c" || m.Folded("a") || m.Folded("b") {
		t.Errorf("cursor on %q, a folded %v, b folded %v", selected(m), m.Folded("a"), m.Folded("b"))
	}
	if ok, _ := m.Reveal("nope"); ok {
		t.Error("Reveal of a row that is not there = true")
	}
}

func lineRows(n int) []Node {
	rows := make([]Node, n)
	for i := range rows {
		rows[i] = leaf(fmt.Sprintf("r%02d", i), 0)
	}
	return rows
}

func TestScrollWindow(t *testing.T) {
	tests := []struct {
		name string
		rows []Node
		keys []string
		sel  string
		off  int
	}{
		{"stays while the cursor is in view", lineRows(20), []string{"j", "j", "j", "j"}, "r04", 0},
		{"scrolls one line past the end", lineRows(20), []string{"j", "j", "j", "j", "j"}, "r05", 1},
		{"page down moves a window", lineRows(20), []string{"pgdown"}, "r05", 1},
		{"half page moves half a window", lineRows(20), []string{"ctrl+d"}, "r02", 0},
		{"page up goes back", lineRows(20), []string{"pgdown", "pgdown", "pgup"}, "r05", 5},
		{"end shows the last line", lineRows(20), []string{"G"}, "r19", 15},
		{"home goes back to the top", lineRows(20), []string{"G", "g"}, "r00", 0},
		{"paging never moves past the last row", lineRows(7), []string{"pgdown", "pgdown", "pgdown"}, "r06", 2},
		{"paging up never moves past the first row", lineRows(7), []string{"pgup"}, "r00", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTree(tt.rows, WithSize(10, 5))
			m, _ = keys(t, m, tt.keys...)
			if got := selected(m); got != tt.sel || m.off != tt.off {
				t.Errorf("cursor on %q, window at %d; want %q, %d", got, m.off, tt.sel, tt.off)
			}
		})
	}
}

func TestScrollKeepsTallRowsInView(t *testing.T) {
	body := Node{ID: "body", Lines: []string{"title", "l1", "l2", "l3", "l4", "l5", "l6"}, Branch: true}
	rows := []Node{leaf("a", 0), leaf("b", 0), open(body), leaf("z", 0)}
	m := newTree(rows, WithSize(10, 4))
	m, _ = keys(t, m, "j", "j")
	if m.off != 2 {
		t.Errorf("window at %d on a row taller than the window, want its first line (2)", m.off)
	}
	m, _ = keys(t, m, "j", "j", "j")
	if selected(m) != "body" || m.off != 9-4 {
		t.Errorf("cursor on %q, window at %d, want body with its end in view at 5", selected(m), m.off)
	}
	m, _ = keys(t, m, "j")
	if selected(m) != "z" || m.off != 9-4+1 {
		t.Errorf("cursor on %q, window at %d, want z at 6", selected(m), m.off)
	}
	m, _ = keys(t, m, "k", "h")
	if m.off != 0 || m.Len() != 4 {
		t.Errorf("window at %d after closing the row, want 0 now the text fits", m.off)
	}
}

func TestResizeKeepsTheCursorInView(t *testing.T) {
	m, _ := keys(t, newTree(lineRows(20), WithSize(10, 10)), "G")
	m.SetSize(10, 3)
	if m.off != 17 {
		t.Errorf("window at %d after the window shrank, want 17", m.off)
	}
	m.SetSize(10, 30)
	if m.off != 0 {
		t.Errorf("window at %d after the window grew past the text, want 0", m.off)
	}
}
