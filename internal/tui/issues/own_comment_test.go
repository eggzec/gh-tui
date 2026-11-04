package issues

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// ownModal opens the modal on a comment, with the viewer being its author.
func ownModal(t *testing.T) (*fakeService, *host, *detailModal, *core.TimelineItem) {
	t.Helper()
	svc := newFakeService(sampleIssues(12))
	h, m := opened(t, svc, 30)
	// The comments are a fold; open it and step onto the first one.
	press(t, h, "2", "l", "j")
	s, ok := m.conv.Current()
	if !ok || s.Item == nil {
		t.Fatalf("the cursor is on %+v, want a comment", s)
	}
	m.viewer = s.Item.Actor.Login
	return svc, h, m, s.Item
}

func TestEditOwnComment(t *testing.T) {
	svc, h, m, it := ownModal(t)

	press(t, h, "e")
	if m.composing != composeEdit || !sameText(m.prompt.Value(), it.Body) {
		t.Fatalf("e opened composing %v with %q, want the comment's text %q to edit", m.composing, m.prompt.Value(), it.Body)
	}
	if m.HasDraft() {
		t.Error("an unchanged text is no draft")
	}
	typeText(t, h, " Edited.")
	if !m.HasDraft() {
		t.Error("changed text should count as unsent")
	}
	press(t, h, "ctrl+s")
	if got := question(h); got != "Save your changes to the comment?" {
		t.Fatalf("submit asked %q", got)
	}
	runHolding(t, h, h.Update(keyMsg("y")))
	got := svc.changeCalls()
	if len(got) != 1 || !strings.HasPrefix(got[0], "edit comment 999 "+it.ID+": ") || !strings.HasSuffix(got[0], " Edited.") {
		t.Fatalf("changes = %v, want the edit of %s", got, it.ID)
	}
	if m.composing != composeNone {
		t.Error("saving should close the prompt")
	}
}

func TestEditOnlyOwnComment(t *testing.T) {
	svc, h, m, _ := ownModal(t)
	m.viewer = "someone-else"
	press(t, h, "e")
	if m.composing != composeNone {
		t.Error("e opened the prompt on a comment that is not yours")
	}
	if len(svc.changeCalls()) != 0 {
		t.Errorf("changes = %v, want none", svc.changeCalls())
	}
}

func TestDeleteOwnComment(t *testing.T) {
	svc, h, _, it := ownModal(t)

	press(t, h, "D")
	if got := question(h); got != "Delete your comment?" {
		t.Fatalf("D asked %q", got)
	}
	runHolding(t, h, h.Update(keyMsg("y")))
	if got := svc.changeCalls(); len(got) != 1 || got[0] != "delete comment 999 "+it.ID {
		t.Fatalf("changes = %v, want the delete of %s", got, it.ID)
	}
}

func TestDeleteOnlyOwnComment(t *testing.T) {
	svc, h, m, _ := ownModal(t)
	m.viewer = "someone-else"
	press(t, h, "D")
	if m.ask != nil || len(svc.changeCalls()) != 0 {
		t.Error("D asked to delete a comment that is not yours")
	}
}
