package pulls

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

// involvedScreen is what the modal draws with the Involved pane zoomed, which
// shows its preview, without styles. The zoom is left as it was found.
func involvedScreen(m *panesModal) string {
	m.Update(keyMsg("z"))
	screen := ansi.Strip(m.View())
	m.Update(keyMsg("z"))
	return screen
}

// moveToTitle presses down until the preview of the row under the cursor has
// title.
func moveToTitle(m *panesModal, title string) bool {
	for range 30 {
		if _, got, ok := m.hoveredInvolved(); ok && got == title {
			return true
		}
		m.Update(keyMsg("j"))
	}
	return false
}

// TestInvolvedPreviewText checks that the preview of a header lists its people
// and the preview of a person says who they are on the pull request.
func TestInvolvedPreviewText(t *testing.T) {
	t.Parallel()
	t.Run("header", func(t *testing.T) {
		t.Parallel()
		m := openedInvolved(t, involvedFake())
		m.Update(keyMsg("j"))
		if !moveToTitle(m, "Reviewers") {
			t.Fatal("no row for the reviewers header")
		}
		got := involvedScreen(m)
		for _, want := range []string{"Rita", "Dina"} {
			if !strings.Contains(got, want) {
				t.Errorf("the preview of the header lacks %q:\n%s", want, got)
			}
		}
	})
	t.Run("person", func(t *testing.T) {
		t.Parallel()
		m := openedInvolved(t, involvedFake())
		if !moveTo(m, "dina") {
			t.Fatal("no row for dina")
		}
		if got := involvedScreen(m); !strings.Contains(got, "review: approved") {
			t.Errorf("the preview of a reviewer who approved lacks her review:\n%s", got)
		}
	})
}

// TestInvolvedPreviewFollowsCursor checks that moving the cursor to another
// person changes the preview.
func TestInvolvedPreviewFollowsCursor(t *testing.T) {
	t.Parallel()
	m := openedInvolved(t, involvedFake())
	if !moveTo(m, "rita") {
		t.Fatal("no row for rita")
	}
	if got := involvedScreen(m); !strings.Contains(got, "review: requested") {
		t.Fatalf("on rita the preview lacks her review:\n%s", got)
	}
	if !moveTo(m, "dina") {
		t.Fatal("no row for dina")
	}
	got := involvedScreen(m)
	if strings.Contains(got, "review: requested") || !strings.Contains(got, "review: approved") {
		t.Errorf("on dina the preview still shows the last row's text:\n%s", got)
	}
}

// TestInvolvedPreviewResetsOnData checks that a preview drawn is drawn again
// once the people change, though the row under the cursor is the same.
func TestInvolvedPreviewResetsOnData(t *testing.T) {
	t.Parallel()
	m := openedInvolved(t, involvedFake())
	if !moveToTitle(m, "Assignees") {
		t.Fatal("no row for the assignees header")
	}
	if got := involvedScreen(m); !strings.Contains(got, "Bob") {
		t.Fatalf("before the change the preview lacks Bob:\n%s", got)
	}
	if len(m.inv.drawn) == 0 {
		t.Fatal("nothing was kept of the preview drawn")
	}
	d := involvedDetail(m.detail)
	d.Assignees = []core.User{person("carl", "Carl")}
	m.Update(panesDetailMsg{id: m.id, detail: d})
	got := involvedScreen(m)
	if !strings.Contains(got, "Carl") || strings.Contains(got, "Bob") {
		t.Errorf("after the change the preview is the old one:\n%s", got)
	}
}
