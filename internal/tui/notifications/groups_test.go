package notifications

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

func levels(names ...string) []orderpick.Choice {
	out := make([]orderpick.Choice, len(names))
	for i, n := range names {
		out[i] = orderpick.Choice{Value: n}
	}
	return out
}

// TestGroupKeyOpensTheModal checks that g asks the app for the group modal.
func TestGroupKeyOpensTheModal(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(inbox()...), 80, 12)
	for _, m := range press(t, s, "g") {
		if open, ok := m.(ui.OpenModalMsg); ok {
			if open.Modal.Title() != "Group · Notifications" {
				t.Errorf("title = %q", open.Modal.Title())
			}
			return
		}
	}
	t.Fatal("g did not open a modal")
}

// TestGroupApply checks that applying levels groups the list, and that none
// make it flat again.
func TestGroupApply(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(inbox()...), 80, 12)
	if s.feed.Grouped() {
		t.Fatal("the list starts grouped")
	}
	run(t, s, s.setGroups(levels("unread")))
	if !s.feed.Grouped() {
		t.Fatal("applying a level did not group the list")
	}
	if view := ansi.Strip(s.View()); !strings.Contains(view, "unread") {
		t.Errorf("no group named unread:\n%s", view)
	}
	run(t, s, s.setGroups(levels("unread", "type")))
	if !s.feed.Grouped() || len(s.levels) != 2 {
		t.Errorf("two levels: grouped %v, levels %v", s.feed.Grouped(), s.levels)
	}
	run(t, s, s.setGroups([]orderpick.Choice{{Value: "unread", Desc: true}, {Value: "repository"}}))
	if len(s.groupsOf(s.levels)) != 2 || !s.groupsOf(s.levels)[0].Desc {
		t.Error("the flipped order of the first level was lost")
	}
	run(t, s, s.setGroups(nil))
	if s.feed.Grouped() {
		t.Error("no levels left the list grouped")
	}
}

// TestGroupToggle checks that alt+g switches the grouping off and on, and
// refuses with nothing grouped.
func TestGroupToggle(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(inbox()...), 80, 12)
	if _, ok := s.toggleGroups(); ok {
		t.Fatal("toggled with nothing grouped")
	}
	press(t, s, "alt+g")
	if s.off || s.feed.Grouped() {
		t.Fatal("alt+g changed a list that is not grouped")
	}

	run(t, s, s.setGroups(levels("unread")))
	press(t, s, "alt+g")
	if s.feed.Grouped() || !s.off {
		t.Fatal("alt+g did not switch the grouping off")
	}
	if view := ansi.Strip(s.View()); !strings.Contains(view, "off") {
		t.Errorf("the header should say the grouping is off:\n%s", view)
	}
	press(t, s, "alt+g")
	if !s.feed.Grouped() || s.off {
		t.Error("alt+g did not restore the grouping")
	}
}

// TestHeaderSortAndGroupOff checks that the header words a sort and a grouping
// that are both off, the sort before the grouping.
func TestHeaderSortAndGroupOff(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(inbox()...), 140, 12)
	run(t, s, s.setSort([]orderpick.Choice{{Value: updatedField, Desc: true}}))
	run(t, s, s.setGroups(levels("unread")))
	press(t, s, "alt+s")
	press(t, s, "alt+g")
	if !s.sortOff || !s.off {
		t.Fatalf("sortOff %v, group off %v, want both", s.sortOff, s.off)
	}
	view := ansi.Strip(s.View())
	sortAt := strings.Index(view, "sort updated")
	groupAt := strings.Index(view, "by unread")
	if sortAt < 0 || groupAt < 0 || sortAt > groupAt {
		t.Errorf("want the sort chip before the group chip:\n%s", view)
	}
	if got := strings.Count(view, "off"); got < 2 {
		t.Errorf("%d off marks, want one for the sort and one for the group:\n%s", got, view)
	}
}

// TestHeaderNoSortOffWithoutSorts checks that a sort switched off with no
// sorts left does not word "sort off".
func TestHeaderNoSortOffWithoutSorts(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(inbox()...), 140, 12)
	s.sortOff = true
	s.renderHeader()
	if view := ansi.Strip(s.View()); strings.Contains(view, "sort off") {
		t.Errorf("header words a sort that is not there:\n%s", view)
	}
}

// TestGroupExpandKeys checks that h, l and * reach the grouped list.
func TestGroupExpandKeys(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(inbox()...), 80, 12)
	run(t, s, s.setGroups(levels("unread")))
	before := ansi.Strip(s.View())
	press(t, s, "*")
	if after := ansi.Strip(s.View()); after == before {
		t.Error("* did not change the grouped list")
	}
	press(t, s, "h")
	closed := ansi.Strip(s.View())
	press(t, s, "l")
	if open := ansi.Strip(s.View()); open == closed {
		t.Error("h and l did not close and open the group")
	}
}
