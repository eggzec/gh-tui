package issues

import (
	"testing"

	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// A conversation rebuilt by a poll while the composer is open is drawn with
// the key focus once the composer closes, since the focus follows the pane.
func TestConversationFocusSurvivesRebuildUnderComposer(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(12))
	h, m := opened(t, svc, 30)
	press(t, h, "2")
	if m.nav.Focus() != paneConversation {
		t.Fatalf("focus = %v, want the conversation", m.nav.Focus())
	}
	press(t, h, "c")
	if m.composerOpen() == nil {
		t.Fatal("c should open the composer")
	}
	run(t, h, h.Update(ui.SyncMsg{Key: issuesvc.SyncKey(testRepo)}))
	press(t, h, "esc")
	if m.composerOpen() != nil {
		t.Fatal("esc should close the composer")
	}
	m.View()
	// The rebuilt conversation starts with its first run folded: l unfolds it
	// and j moves into it, so a blurred conversation stays on the run.
	before, _, _ := m.conv.Selected()
	press(t, h, "l")
	press(t, h, "j")
	after, _, _ := m.conv.Selected()
	if before == after {
		t.Errorf("the conversation ignored l and j (selected %q), it is blurred under the focus", before)
	}
}
