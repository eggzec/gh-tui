package search

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func newTestModal(t *testing.T) *Modal {
	t.Helper()
	return NewModal(newSection(t, newFake(), 80, 24))
}

func TestModalTitle(t *testing.T) {
	t.Parallel()
	if got := newTestModal(t).Title(); got != ui.SearchTitle {
		t.Errorf("Title = %q, want %q", got, ui.SearchTitle)
	}
}

func TestModalOverRewritesBack(t *testing.T) {
	t.Parallel()
	m := newTestModal(t)
	other := newTestModal(t)

	open := func(back ui.Modal) tea.Cmd {
		return func() tea.Msg { return ui.OpenModalMsg{Modal: other, Over: back} }
	}

	got := m.over(open(nil))().(ui.OpenModalMsg)
	if got.Over != ui.Modal(m) {
		t.Errorf("Back = %v, want the modal", got.Over)
	}

	got = m.over(open(other))().(ui.OpenModalMsg)
	if got.Over != ui.Modal(other) {
		t.Errorf("a set Back was replaced by %v", got.Over)
	}

	batch := m.over(func() tea.Msg { return tea.BatchMsg{open(nil)} })().(tea.BatchMsg)
	got = batch[0]().(ui.OpenModalMsg)
	if got.Over != ui.Modal(m) {
		t.Errorf("Back in a batch = %v, want the modal", got.Over)
	}
}

func TestModalHideKeepsRunning(t *testing.T) {
	t.Parallel()
	m := newTestModal(t)
	m.Hide()
	if !m.s.focused {
		t.Error("Hide blurred the section")
	}
}

func TestModalDiscardBlurs(t *testing.T) {
	t.Parallel()
	m := newTestModal(t)
	m.Discard()
	if m.s.focused {
		t.Error("Discard left the section focused")
	}
}

func TestModalDismissLeaves(t *testing.T) {
	t.Parallel()
	for _, action := range []string{ui.ActDismiss, ui.ActBack, ui.ActQuit} {
		m := newTestModal(t)
		cmd, ok := m.Act(action)
		if !ok || cmd == nil {
			t.Fatalf("%s: Act = %v, %v", action, cmd, ok)
		}
		msg, isLeave := cmd().(ui.LeaveModalMsg)
		if !isLeave || msg.Modal != ui.Modal(m) {
			t.Errorf("%s: got %v, want a LeaveModalMsg for the modal", action, msg)
		}
		if m.s.focused {
			t.Errorf("%s: section still focused", action)
		}
	}
}
