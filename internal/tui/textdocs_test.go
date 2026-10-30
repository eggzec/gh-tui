package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

func openTexts(t *testing.T, docs []pager.Doc) (*Model, *textModal) {
	t.Helper()
	m, _ := newTestApp(t)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	drive(m, m.openTexts("Two", docs, false))
	tm, ok := m.modals.Top().(*textModal)
	if !ok {
		t.Fatalf("open modal is %T, want the text modal", m.modals.Top())
	}
	return m, tm
}

func TestColonNStepsTextDocuments(t *testing.T) {
	t.Parallel()
	m, tm := openTexts(t, []pager.Doc{{Name: "one.txt", Text: "first text\n"}, {Name: "two.txt", Text: "second text\n"}})
	if got := tm.Commands(); len(got) != 2 {
		t.Fatalf("commands %v, want n and p", got)
	}
	runCommand(t, m, "n")
	if got := ansi.Strip(tm.View()); !strings.Contains(got, "second text") || !strings.Contains(got, "2/2") {
		t.Errorf("after :n the view is\n%s", got)
	}
	runCommand(t, m, "p")
	if got := ansi.Strip(tm.View()); !strings.Contains(got, "first text") || !strings.Contains(got, "1/2") {
		t.Errorf("after :p the view is\n%s", got)
	}
}

func TestColonNRefusedOverOneText(t *testing.T) {
	t.Parallel()
	m, tm := openTexts(t, []pager.Doc{{Name: "one.txt", Text: "only text\n"}})
	if tm.Commands() != nil {
		t.Errorf("commands %v, want none", tm.Commands())
	}
	runCommand(t, m, "n")
	if got := toasted(m); !strings.Contains(got, "n") {
		t.Errorf("n over one document gave no toast: %s", got)
	}
}
