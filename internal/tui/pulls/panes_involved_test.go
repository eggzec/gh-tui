package pulls

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

func involvedFake() *involvedService {
	return &involvedService{fakeService: newFakeService(), timeline: []core.TimelineItem{
		{Kind: core.TimelineComment, Actor: person("dina", "Dina"), At: stamp},
	}}
}

// TestInvolvedCursorRedraws checks that moving the cursor changes the drawn
// panes, which are cached until something they show changes.
func TestInvolvedCursorRedraws(t *testing.T) {
	t.Parallel()
	m := openedInvolved(t, involvedFake())
	before := m.View()
	m.Update(keyMsg("j"))
	if m.View() == before {
		t.Error("down left the frame as it was")
	}
}

// TestInvolvedEnter checks that enter on a person with comments focuses the
// conversation, on one with none says so, and on a header folds the group.
func TestInvolvedEnter(t *testing.T) {
	t.Parallel()
	t.Run("person with comments", func(t *testing.T) {
		t.Parallel()
		m := openedInvolved(t, involvedFake())
		if !moveTo(m, "dina") {
			t.Fatal("no row for dina")
		}
		if toasts := run(m, m.Update(keyMsg("enter"))); len(toasts) != 0 || m.focus != panes.Conversation {
			t.Errorf("focus %v, toasts %q: want the conversation and none", m.focus, toasts)
		}
	})
	t.Run("person without comments", func(t *testing.T) {
		t.Parallel()
		m := openedInvolved(t, involvedFake())
		if !moveTo(m, "rita") {
			t.Fatal("no row for rita")
		}
		toasts := run(m, m.Update(keyMsg("enter")))
		if !slices.Equal(toasts, []string{"no comments from Rita"}) || m.focus != panes.Involved {
			t.Errorf("focus %v, toasts %q", m.focus, toasts)
		}
	})
	t.Run("header", func(t *testing.T) {
		t.Parallel()
		m := openedInvolved(t, involvedFake())
		m.Update(keyMsg("j"))
		was := m.inv.tree.Folded("group:reviewers")
		run(m, m.Update(keyMsg("enter")))
		if m.inv.tree.Folded("group:reviewers") == was {
			t.Error("enter on the header didn't toggle the fold")
		}
	})
}

// TestInvolvedKeys checks that the help lists the tree's keys while the pane
// has the focus, and that a modal key still wins over the pane.
func TestInvolvedKeys(t *testing.T) {
	t.Parallel()
	m := openedInvolved(t, involvedFake())
	layers := m.KeyLayers()
	if got := layers[len(layers)-1]; got.Context != ctxInvolved || len(got.Bindings) == 0 {
		t.Errorf("last layer is %q with %d bindings", got.Context, len(got.Bindings))
	}
	m.Update(keyMsg("2"))
	if m.focus != panes.Conversation {
		t.Errorf("2 focused %v, want the conversation", m.focus)
	}
}
