package notifications

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestSelected(t *testing.T) {
	if _, ok := newSection(t, newFake(), 80, 20).Selected(); ok {
		t.Error("an empty inbox selected something")
	}
	svc := newFake(thread("5", "charmbracelet/lipgloss", core.SubjectCommit, "Fix width", "author", true, 0))
	s := newSection(t, svc, 80, 20)
	n, _ := s.feed.Selected()
	got, ok := s.Selected()
	if !ok || got != ui.NotificationSelection(n) || got.What != "commit" || got.SHA == "" {
		t.Errorf("Selected() = %+v, %v, want the commit of %+v", got, ok, n)
	}
}
