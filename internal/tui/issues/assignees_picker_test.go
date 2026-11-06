package issues

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// onAssignees moves the cursor of the Involved pane from the author to the
// first assignee: the author, the group, then mona. The tree takes keys once
// a frame has drawn it focused, as the app does after every message.
func onAssignees(t *testing.T, h *host) {
	t.Helper()
	press(t, h, "3")
	h.modal().View()
	press(t, h, "down", "down")
	if id, _ := h.modal().side.inv.SelectedID(); id != "assignee:mona" {
		t.Fatalf("the cursor is on %q, want the first assignee", id)
	}
}

// The picker ticks the assignees of the issue. Applying a change asks, and
// the answer sends exactly the people added and removed.
func TestAssigneesPicker(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 30, WithFacets(&fakeFacets{}))
	press(t, h, "down", "enter", "*")
	m := h.modal()
	onAssignees(t, h)
	press(t, h, "e")
	o, ok := m.over.(*assigneesOverlay)
	if !ok {
		t.Fatalf("e opened %T, want the assignees picker", m.over)
	}
	if !o.On["mona"] || !o.Was["mona"] {
		t.Fatalf("the current assignee is not ticked: on %v, was %v", o.On, o.Was)
	}
	// mona is first, ticked: untick it, then tick octocat, which the search of
	// the people of the repository found.
	press(t, h, "space", "down", "space", "enter")
	if got, want := question(h), "Assign @octocat and unassign @mona on #999?"; got != want {
		t.Fatalf("applying asked %q, want %q", got, want)
	}
	if len(svc.changeCalls()) != 0 {
		t.Fatalf("sent %v before the answer", svc.changeCalls())
	}
	for _, d := range runHolding(t, h, h.Update(keyMsg("y"))) {
		run(t, h, h.Update(d))
	}
	if got := svc.changeCalls(); !slices.Equal(got, []string{"assign 999 +octocat -mona"}) {
		t.Errorf("changes = %v", got)
	}
}

// Applying with nothing changed asks nothing and sends nothing.
func TestAssigneesPickerUnchanged(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(12))
	h, m := opened(t, svc, 30)
	onAssignees(t, h)
	press(t, h, "e")
	if m.over == nil {
		t.Fatal("e opened no picker")
	}
	press(t, h, "enter")
	if question(h) != "" || len(svc.changeCalls()) != 0 || m.over != nil {
		t.Errorf("asked %q, sent %v, picker open %v", question(h), svc.changeCalls(), m.over != nil)
	}
}

// e outside the Assignees group opens nothing.
func TestAssigneesPickerElsewhere(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(12))
	h, m := opened(t, svc, 30)
	// The cursor starts on the author.
	press(t, h, "3")
	m.View()
	press(t, h, "e")
	if m.over != nil {
		t.Errorf("e on the author opened %T", m.over)
	}
}

// A viewer without triage access gets the refusal, and no picker.
func TestAssigneesPickerRefused(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 30, WithViewer(me))
	run(t, h, h.Update(ui.CapsMsg{Repo: testRepo, Caps: readCaps}))
	press(t, h, "down", "enter", "*")
	m := h.modal()
	onAssignees(t, h)
	msgs := press(t, h, "e")
	if m.over != nil {
		t.Error("the picker opened for a viewer who cannot assign")
	}
	if want := info("Assigning needs triage access to eggzec/gh-tui."); !slices.Contains(msgs, want) {
		t.Errorf("fed %v, want the refusal", msgs)
	}
}
