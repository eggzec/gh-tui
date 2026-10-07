package thread

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/markdown"
)

const diagramBody = "Intro.\n\n```mermaid\nsequenceDiagram\n    A->>B: hello\n    B-->>A: hi\n```\n\nOutro."

// withDiagrams returns a thread whose body and comments are markdown,
// rendered with the thread's Markdown as a caller's renderer does.
func withDiagrams(tb testing.TB, body string, comments []comment, width, height int) Model[comment] {
	tb.Helper()
	src := &source{fail: map[string]int{}, chunks: [][]comment{comments}}
	var md func(string, int) string
	render := func(c comment, width int) string {
		return "  @" + c.author + "\n" + markdown.Indent(md(c.body, markdown.Room(width, 4)), "  ")
	}
	m := New(src.fetch, render, WithKeyMap(testKeys()), WithSize(width, height), WithFocused(true))
	md = func(s string, w int) string { return m.Markdown(s, w) }
	return drain(tb, m, m.SetDocument("Title", body))
}

// screen returns the view without styles.
func screen(m Model[comment]) []string {
	return strings.Split(ansi.Strip(m.View()), "\n")
}

// row returns the first line on screen that holds s, or -1.
func row(m Model[comment], s string) int {
	for i, l := range screen(m) {
		if strings.Contains(l, s) {
			return i
		}
	}
	return -1
}

func enter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }

// Enter opens the diagram on screen to show its code under its head, which
// stays where it was, and closes it again; the head shows the pointer.
func TestDiagramToggles(t *testing.T) {
	m := withDiagrams(t, diagramBody, nil, 80, 20)
	head := row(m, "◆ sequence diagram · 3 lines · View diagram ↗")
	if head < 0 {
		t.Fatalf("no head:\n%s", strings.Join(screen(m), "\n"))
	}
	if l := screen(m)[head]; !strings.HasPrefix(l, "›") {
		t.Errorf("the head has no pointer: %q", l)
	}
	if row(m, "A->>B: hello") >= 0 {
		t.Fatal("the diagram shows its code before it is opened")
	}
	if !m.OnDiagram() {
		t.Error("OnDiagram() is false with a head on screen")
	}

	m, _ = m.Update(enter())
	if got := row(m, "◆ sequence diagram"); got != head {
		t.Errorf("the head moved from row %d to %d", head, got)
	}
	code := row(m, "A->>B: hello")
	if code <= head || row(m, "Outro.") <= code {
		t.Errorf("the code isn't between the head and what follows:\n%s", strings.Join(screen(m), "\n"))
	}

	m, _ = m.Update(enter())
	if row(m, "A->>B: hello") >= 0 || row(m, "◆ sequence diagram") != head {
		t.Errorf("the diagram didn't close:\n%s", strings.Join(screen(m), "\n"))
	}
}

// A diagram in a comment opens like one in the body, and the one on
// screen is the one enter and Diagram act on.
func TestDiagramInComment(t *testing.T) {
	flow := "```mermaid\ngraph TD\n  A-->B\n```"
	m := withDiagrams(t, "Body.", []comment{
		{author: "alice", body: "First."},
		{author: "coderabbit", body: "Walkthrough.\n\n" + flow},
		{author: "bob", body: strings.Repeat("More.\n\n", 40)},
	}, 60, 30)
	head := row(m, "◆ flowchart · 2 lines · View diagram ↗")
	if head < 0 {
		t.Fatalf("no head:\n%s", strings.Join(screen(m), "\n"))
	}
	if h := m.target(); h == nil || h.chunk != 0 || h.item != 1 {
		t.Errorf("the target is %+v, want the second comment", h)
	}
	m, _ = m.Update(enter())
	if row(m, "A-->B") <= head {
		t.Errorf("the comment's diagram didn't open:\n%s", strings.Join(screen(m), "\n"))
	}
	// Scrolling the head off screen, the open code still counts.
	m = press(t, m, slices.Repeat([]string{"j"}, head+1)...)
	if !m.OnDiagram() || row(m, "A-->B") < 0 {
		t.Errorf("the open code on screen has no diagram:\n%s", strings.Join(screen(m), "\n"))
	}
	m, _ = m.Update(enter())
	if row(m, "A-->B") >= 0 || row(m, "◆ flowchart") != 0 {
		t.Errorf("closing from below didn't bring the head to the top:\n%s", strings.Join(screen(m), "\n"))
	}
}

// Without a diagram on screen, enter does nothing and the help doesn't
// offer it.
func TestNoDiagram(t *testing.T) {
	m := withDiagrams(t, "Just text.\n\n```go\nx := 1\n```", nil, 60, 10)
	before := m.View()
	if m.OnDiagram() {
		t.Error("OnDiagram() reports a diagram")
	}
	m, _ = m.Update(enter())
	if m.View() != before {
		t.Error("enter changed the view")
	}
	for _, b := range m.ShortHelp() {
		if b.Enabled() && key.Matches(enter(), b) {
			t.Errorf("the help offers %q", b.Help().Desc)
		}
	}

	m = withDiagrams(t, diagramBody, nil, 60, 10)
	offered := false
	for _, b := range m.ShortHelp() {
		offered = offered || b.Enabled() && key.Matches(enter(), b)
	}
	if !offered {
		t.Error("the help doesn't offer enter on a diagram")
	}
}

// What the reader opened stays open when the thread renders again, as on a
// resize, and a new document starts closed.
func TestDiagramStaysOpen(t *testing.T) {
	m := withDiagrams(t, diagramBody, nil, 80, 20)
	m, _ = m.Update(enter())
	m.SetSize(60, 20)
	if row(m, "A->>B: hello") < 0 {
		t.Errorf("the diagram closed on a resize:\n%s", strings.Join(screen(m), "\n"))
	}
	m = drain(t, m, m.Reset())
	m = drain(t, m, m.SetDocument("Title", diagramBody))
	if row(m, "A->>B: hello") >= 0 {
		t.Error("the diagram is open in a new document")
	}
}
