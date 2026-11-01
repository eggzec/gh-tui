package pulls

import (
	"fmt"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
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
		if toasts := run(m, m.Update(keyMsg("enter"))); len(toasts) != 0 || m.nav.Focus() != paneConversation {
			t.Errorf("focus %v, toasts %q: want the conversation and none", m.nav.Focus(), toasts)
		}
	})
	t.Run("person without comments", func(t *testing.T) {
		t.Parallel()
		m := openedInvolved(t, involvedFake())
		if !moveTo(m, "rita") {
			t.Fatal("no row for rita")
		}
		toasts := run(m, m.Update(keyMsg("enter")))
		if !slices.Equal(toasts, []string{"no comments from Rita"}) || m.nav.Focus() != paneInvolved {
			t.Errorf("focus %v, toasts %q", m.nav.Focus(), toasts)
		}
	})
	t.Run("header", func(t *testing.T) {
		t.Parallel()
		m := openedInvolved(t, involvedFake())
		m.Update(keyMsg("j"))
		was := m.inv.tree.Folded("group:reviewers")
		run(m, m.Update(keyMsg("enter")))
		if m.inv.tree.Folded("group:reviewers") != was {
			t.Error("enter on the header changed the fold")
		}
	})
}

// TestInvolvedRerequest checks that R on a reviewer who reviewed asks first and
// then sends the request, on one whose review is pending only says so, on a
// refused request says so, and on any other row does nothing.
func TestInvolvedRerequest(t *testing.T) {
	t.Parallel()
	t.Run("reviewed asks, then sends", func(t *testing.T) {
		t.Parallel()
		svc := involvedFake()
		m := openedInvolved(t, svc)
		if !moveTo(m, "dina") {
			t.Fatal("no row for dina")
		}
		// The command that comes back arms the question; it sends nothing.
		m.Update(keyMsg("R"))
		if len(svc.asked) != 0 || m.top() == nil {
			t.Fatalf("R sent %d requests and left overlay %v; want a question and no request", len(svc.asked), m.top())
		}
		run(m, m.Update(keyMsg("y")))
		if len(svc.asked) != 1 || svc.asked[0][0].Login != "dina" || svc.asked[0][0].Name != "Dina" {
			t.Errorf("requests %+v, want one for dina", svc.asked)
		}
	})
	t.Run("declined sends nothing", func(t *testing.T) {
		t.Parallel()
		svc := involvedFake()
		m := openedInvolved(t, svc)
		moveTo(m, "dina")
		m.Update(keyMsg("R"))
		m.Update(keyMsg("n"))
		if len(svc.asked) != 0 {
			t.Errorf("requests %+v after no", svc.asked)
		}
	})
	t.Run("pending says so", func(t *testing.T) {
		t.Parallel()
		svc := involvedFake()
		m := openedInvolved(t, svc)
		if !moveTo(m, "rita") {
			t.Fatal("no row for rita")
		}
		toasts := run(m, m.Update(keyMsg("R")))
		if !slices.Equal(toasts, []string{"review is already requested from Rita"}) || m.top() != nil || len(svc.asked) != 0 {
			t.Errorf("toasts %q, overlay %v, requests %+v", toasts, m.top(), svc.asked)
		}
	})
	t.Run("refused says so", func(t *testing.T) {
		t.Parallel()
		svc := involvedFake()
		svc.refuse = fmt.Errorf("request review: %w", pulls.ErrAlreadyRequested)
		m := openedInvolved(t, svc)
		moveTo(m, "dina")
		m.Update(keyMsg("R"))
		toasts := run(m, m.Update(keyMsg("y")))
		if !slices.Equal(toasts, []string{"review is already requested from Dina"}) {
			t.Errorf("toasts %q", toasts)
		}
	})
	t.Run("other rows do nothing", func(t *testing.T) {
		t.Parallel()
		svc := involvedFake()
		m := openedInvolved(t, svc)
		if toasts := run(m, m.Update(keyMsg("R"))); len(toasts) != 0 || m.top() != nil {
			t.Errorf("R on a header: toasts %q, overlay %v", toasts, m.top())
		}
		if !moveTo(m, "bob") {
			t.Fatal("no row for bob")
		}
		if toasts := run(m, m.Update(keyMsg("R"))); len(toasts) != 0 || m.top() != nil || len(svc.asked) != 0 {
			t.Errorf("R on an assignee: toasts %q, overlay %v, requests %+v", toasts, m.top(), svc.asked)
		}
	})
}

// TestInvolvedRerequestKeyListed checks that the help lists the key only while
// the involved pane has the focus.
func TestInvolvedRerequestKeyListed(t *testing.T) {
	t.Parallel()
	lists := func(m *panesModal) bool {
		for _, l := range m.KeyLayers() {
			for _, b := range l.Bindings {
				if b.Help().Desc == "re-request" {
					return true
				}
			}
		}
		return false
	}
	m := openedInvolved(t, involvedFake())
	if !lists(m) {
		t.Error("the help lacks re-request while the pane has the focus")
	}
	m.Update(keyMsg("2"))
	if lists(m) {
		t.Error("the help lists re-request while the conversation has the focus")
	}
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
	if m.nav.Focus() != paneConversation {
		t.Errorf("2 focused %v, want the conversation", m.nav.Focus())
	}
}
