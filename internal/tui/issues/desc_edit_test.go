package issues

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/prpanes"
)

// descModal opens the modal on the Description pane, with the viewer being the
// author of the issue.
func descModal(t *testing.T) (*fakeService, *host, *detailModal) {
	t.Helper()
	svc := newFakeService(sampleIssues(12))
	h, m := opened(t, svc, 30)
	m.viewer = m.issue.Author.Login
	press(t, h, "1")
	return svc, h, m
}

func TestEditDescriptionSends(t *testing.T) {
	t.Parallel()
	svc, h, m := descModal(t)

	press(t, h, "E")
	if _, ok := m.over.(*prpanes.DescEdit); !ok {
		t.Fatalf("E opened %T, want the editor of the description", m.over)
	}
	if m.HasDraft() {
		t.Error("an unchanged editor is no draft")
	}
	typeText(t, h, " edited")
	if !m.HasDraft() {
		t.Error("a changed title should count as unsent")
	}
	press(t, h, "ctrl+s")
	got := svc.changeCalls()
	if len(got) != 1 || !strings.HasPrefix(got[0], "edit issue 999 ") || !strings.Contains(got[0], " edited") {
		t.Fatalf("changes = %v, want the edit of the issue", got)
	}
	if m.over != nil {
		t.Error("saving should close the editor")
	}
}

func TestEditDescriptionCancels(t *testing.T) {
	t.Parallel()
	svc, h, m := descModal(t)

	press(t, h, "E")
	typeText(t, h, "x")
	press(t, h, "esc")
	if m.over != nil {
		t.Error("esc should close the editor")
	}
	if got := svc.changeCalls(); len(got) != 0 {
		t.Errorf("changes = %v, want none", got)
	}
}

func TestEditDescriptionNeedsATitle(t *testing.T) {
	t.Parallel()
	svc, h, m := descModal(t)

	press(t, h, "E")
	for range 200 {
		press(t, h, "backspace")
	}
	press(t, h, "ctrl+s")
	if m.over == nil {
		t.Error("an empty title should keep the editor open")
	}
	if got := svc.changeCalls(); len(got) != 0 {
		t.Errorf("changes = %v, want none", got)
	}
}

func TestEditDescriptionRefusedForOthers(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(12))
	h, m := opened(t, svc, 30)
	m.viewer = "someone-else"
	m.issue.Caps.Known, m.issue.Caps.Update = true, false
	press(t, h, "1", "E")
	if m.over != nil {
		t.Errorf("E opened %T for a viewer who may not edit", m.over)
	}
}

func TestEditOnConversationStillEditsComment(t *testing.T) {
	t.Parallel()
	_, h, m, _ := ownModal(t)
	press(t, h, "E")
	if m.composerOpen() == nil {
		t.Errorf("E in the Conversation opened %v, want the comment composer", m.over)
	}
}
