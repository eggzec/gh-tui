package logview

import (
	"slices"
	"strings"
	"testing"
)

// shown returns the rows shown, each as its text indented by its depth.
func shownRows(m Model) []string {
	out := make([]string, len(m.vis))
	for i, r := range m.vis {
		out[i] = strings.Repeat(".", m.rows[r].depth) + m.rows[r].text
	}
	return out
}

// isOpen reports whether the fold whose header reads title is expanded.
func isOpen(tb testing.TB, m Model, title string) bool {
	tb.Helper()
	for _, f := range m.folds {
		if m.rows[f.head].text == title {
			return f.open
		}
	}
	tb.Fatalf("no fold %q", title)
	return false
}

func TestFoldGroups(t *testing.T) {
	lines := []Line{
		{Text: "before"},
		{Kind: Group, Text: "outer"},
		{Text: "a"},
		{Kind: Group, Text: "inner"},
		{Text: "b"},
		{Kind: EndGroup},
		{Kind: EndGroup},
		// An end without a group is dropped.
		{Kind: EndGroup},
		{Text: "after"},
	}
	m := view(t, lines, WithSize(40, 10))
	if got, want := shownRows(m), []string{"before", "outer", "after"}; !slices.Equal(got, want) {
		t.Errorf("groups start collapsed: %q, want %q", got, want)
	}
	// Fold all folds steps only, so it leaves groups as they are.
	m, _ = keys(t, m, "*")
	if got, want := shownRows(m), []string{"before", "outer", "after"}; !slices.Equal(got, want) {
		t.Errorf("fold all changed groups: %q, want %q", got, want)
	}
	m.ExpandAll()
	if got, want := shownRows(m), []string{"before", "outer", ".a", ".inner", "..b", "after"}; !slices.Equal(got, want) {
		t.Errorf("after expand all: %q, want %q", got, want)
	}
	if m.Lines() != len(lines) {
		t.Errorf("Lines() = %d, want %d", m.Lines(), len(lines))
	}
	m.CollapseAll()
	if got, want := shownRows(m), []string{"before", "outer", "after"}; !slices.Equal(got, want) {
		t.Errorf("after collapse all: %q, want %q", got, want)
	}
}

func TestFoldSections(t *testing.T) {
	lines := []Line{
		{Text: "x"},
		{Kind: Group, Text: "g"},
		{Text: "y"},
		{Text: "z"},
		// Section two has no group open, and a group never ends a section.
		{Kind: EndGroup},
		{Text: "w"},
	}
	// Out of order and overlapping: two starts where one would go on.
	sections := []Section{
		{Title: "two", Start: 2, End: 5},
		{Title: "one\n\x1b]0;pwned\x07\x1b[1mfirst", Start: 0, End: 4},
	}
	m := New(WithSize(40, 10))
	m.SetLines(lines, sections)
	m.ExpandAll()
	want := []string{"one first", ".x", ".g", "two", ".y", ".z", "w"}
	if got := shownRows(m); !slices.Equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

func TestFoldDefaults(t *testing.T) {
	m := open(t, WithSize(80, 24))
	for _, f := range m.folds {
		if want := f.sec >= 0; f.open != want {
			t.Errorf("fold %q open = %v, want %v", m.rows[f.head].text, f.open, want)
		}
	}
	if m.cur != 0 || cursorText(m) != "Set up job" {
		t.Errorf("cursor on %q, want the first section", cursorText(m))
	}
}

func TestFoldKeys(t *testing.T) {
	tests := []struct {
		name       string
		keys       []string
		wantCursor string
		open       map[string]bool
	}{
		{name: "enter collapses a section", keys: []string{"enter"}, wantCursor: "Set up job",
			open: map[string]bool{"Set up job": false}},
		{name: "enter expands a group", keys: []string{"j", "j", "enter"}, wantCursor: "Runner Image Provisioner",
			open: map[string]bool{"Runner Image Provisioner": true}},
		{name: "toggle inside a group collapses it", keys: []string{"j", "j", "enter", "j", "j", "enter"},
			wantCursor: "Runner Image Provisioner", open: map[string]bool{"Runner Image Provisioner": false}},
		{name: "plus expands", keys: []string{"j", "j", "+", "+"}, wantCursor: "Runner Image Provisioner",
			open: map[string]bool{"Runner Image Provisioner": true}},
		{name: "plus on a line does nothing", keys: []string{"j", "+"}, wantCursor: "Current runner version: '2.337.0'",
			open: map[string]bool{"Set up job": true}},
		{name: "minus collapses", keys: []string{"j", "j", "+", "-"}, wantCursor: "Runner Image Provisioner",
			open: map[string]bool{"Runner Image Provisioner": false, "Set up job": true}},
		{name: "minus on a line collapses what holds it", keys: []string{"j", "-"}, wantCursor: "Set up job",
			open: map[string]bool{"Set up job": false}},
		{name: "minus on a collapsed group collapses its section", keys: []string{"j", "j", "-"},
			wantCursor: "Set up job", open: map[string]bool{"Set up job": false}},
		{name: "fold all keeps the cursor on what holds it and groups as they are",
			keys: []string{"j", "j", "+", "j", "*"}, wantCursor: "Set up job",
			open: map[string]bool{"Set up job": false, "Install Go": false, "Runner Image Provisioner": true}},
		{name: "fold all again expands the steps", keys: []string{"j", "j", "+", "j", "*", "*"},
			wantCursor: "Set up job",
			open: map[string]bool{"Set up job": true, "Install Go": true, "Runner Image Provisioner": true,
				"run golangci-lint": false}},
		{name: "one step open folds all", keys: []string{"*", "enter", "*"}, wantCursor: "Set up job",
			open: map[string]bool{"Set up job": false, "Install Go": false}},
		{name: "equals is not bound", keys: []string{"="}, wantCursor: "Set up job",
			open: map[string]bool{"Set up job": true, "Install Go": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, WithSize(80, 24))
			m, _ = keys(t, m, tt.keys...)
			if got := cursorText(m); got != tt.wantCursor {
				t.Errorf("cursor on %q, want %q", got, tt.wantCursor)
			}
			for title, want := range tt.open {
				if got := isOpen(t, m, title); got != want {
					t.Errorf("%q open = %v, want %v", title, got, want)
				}
			}
		})
	}
}

func TestFoldAll(t *testing.T) {
	m := open(t, WithSize(80, 24))
	m.ExpandAll()
	if len(m.vis) != len(m.rows) {
		t.Errorf("expand all shows %d rows, want all %d", len(m.vis), len(m.rows))
	}
	m.CollapseAll()
	if len(m.vis) != len(lintSteps) {
		t.Errorf("collapse all shows %d rows, want the %d steps", len(m.vis), len(lintSteps))
	}
}

// A copy of a model keeps its folds when the other toggles.
func TestFoldCopies(t *testing.T) {
	m := open(t, WithSize(80, 24))
	before := shownRows(m)
	c, _ := keys(t, m, "enter")
	c, _ = keys(t, c, "*")
	if got := shownRows(m); !slices.Equal(got, before) {
		t.Error("toggling a copy changed the original")
	}
	if !isOpen(t, m, "Set up job") || isOpen(t, m, "Runner Image") {
		t.Error("toggling a copy changed the original's folds")
	}
	if slices.Equal(shownRows(c), before) {
		t.Error("the copy didn't change")
	}
}

func TestFocusFailed(t *testing.T) {
	m := open(t, WithSize(80, 24), WithFocusFailed(true))
	for _, f := range m.folds {
		if f.sec < 0 {
			continue
		}
		if want := m.secs[f.sec].failed; f.open != want {
			t.Errorf("section %q open = %v, want %v", m.rows[f.head].text, f.open, want)
		}
	}
	want := `D:\a\bubbletea\bubbletea\charmbracelet\bubbletea\clipboard_backend.go:95:6: type clipboardCommand is unused (unused)`
	if got := cursorText(m); got != want {
		t.Errorf("cursor on %q, want the first error", got)
	}
	if !isOpen(t, m, "run golangci-lint") {
		t.Error("the group of the error is collapsed")
	}
	if !m.covers(m.cur) {
		t.Error("the error is out of view")
	}
	if !strings.Contains(plain(m), "error 1/5") {
		t.Errorf("status doesn't count the error:\n%s", plain(m))
	}
}

func TestFocusFailedWithoutSections(t *testing.T) {
	lines := []Line{
		{Text: "ok"},
		{Kind: Group, Text: "build"},
		{Kind: Error, Text: "boom"},
		{Kind: EndGroup},
	}
	m := view(t, lines, WithSize(40, 5))
	if !m.FocusFailed() || cursorText(m) != "boom" {
		t.Errorf("FocusFailed moved to %q, want the error", cursorText(m))
	}
	m = view(t, plainLines("fine", "all good"), WithSize(40, 5))
	if m.FocusFailed() || m.cur != 0 {
		t.Error("FocusFailed found something in a log without errors")
	}
}

// A failed step without error lines opens on its title.
func TestFocusFailedWithoutErrors(t *testing.T) {
	m := New(WithSize(40, 5), WithFocusFailed(true))
	m.SetLines(plainLines("a", "b", "c"), []Section{
		{Title: "one", Start: 0, End: 2},
		{Title: "two", Start: 2, End: 3, Failed: true},
	})
	if cursorText(m) != "two" || isOpen(t, m, "one") || !isOpen(t, m, "two") {
		t.Errorf("cursor on %q with %q shown, want on the failed step", cursorText(m), shownRows(m))
	}
}

// A log that Prepare read and SetLog showed shows as SetLines shows it,
// and Prepare leaves the model that it read on as it was.
func TestPrepareThenSetLog(t *testing.T) {
	lines, secs := synthetic(3000)
	want := New(WithSize(80, 20), WithFocusFailed(true))
	want.SetLines(lines, secs)

	m := New(WithSize(80, 20), WithFocusFailed(true))
	m.SetLines(lines[:10], nil)
	before := m.View()
	l := m.Prepare(lines, secs)
	if m.View() != before || m.Lines() != 10 {
		t.Fatalf("Prepare changed the model: %d lines", m.Lines())
	}
	m.SetLog(l)
	if got := m.View(); got != want.View() {
		t.Errorf("SetLog shows:\n%s\nwant as SetLines:\n%s", got, want.View())
	}
	if m.Lines() != want.Lines() || m.Errors() != want.Errors() || m.Warnings() != want.Warnings() {
		t.Errorf("SetLog: %d lines, %d errors, %d warnings; want %d, %d, %d",
			m.Lines(), m.Errors(), m.Warnings(), want.Lines(), want.Errors(), want.Warnings())
	}
}
