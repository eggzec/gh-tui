package dashboard

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// G opens the choice of levels, choosing one groups the repositories, alt+g
// lists them flat and sets the off chip apart, and alt+g with no levels says
// so.
func TestGroupKeys(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), nil, 100, 30)

	if msgs := press(t, s, "alt+g"); !hasNotify(msgs) {
		t.Error("alt+g with no levels said nothing")
	}

	if _, ok := s.Update(keyPress("g"))().(ui.OpenModalMsg); !ok {
		t.Fatal("g did not open the group modal")
	}

	run(t, s, s.repos.setGroups([]orderpick.Choice{{Value: "owner"}}))
	if s.repos.grouping.Off || s.repos.grouping.Groups() == nil {
		t.Fatalf("choosing a level left off %v, groups %v", s.repos.grouping.Off, s.repos.grouping.Groups())
	}

	press(t, s, "alt+g")
	if !s.repos.grouping.Off || s.repos.grouping.Groups() != nil {
		t.Errorf("alt+g left off %v, groups %v; want it off", s.repos.grouping.Off, s.repos.grouping.Groups())
	}
	if chip := ansi.Strip(s.repos.grouping.Chip(s.icons.Separator)); chip == "" || !strings.Contains(screen(s), chip) {
		t.Errorf("the off chip %q is not on the screen", chip)
	}

	press(t, s, "alt+g")
	if s.repos.grouping.Off {
		t.Error("alt+g again left the grouping off")
	}
}

// The group modal applies its choice from inside the modal, so the section
// sees only the OrderedMsg after it: that alone must draw the groups, and one
// alt+g must switch them off in the view.
func TestGroupsShowAfterTheModalApplies(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), nil, 100, 30)
	flat := screen(s)

	s.repos.setGroups([]orderpick.Choice{{Value: "owner"}})
	s.Update(ui.OrderedMsg{})
	grouped := screen(s)
	if grouped == flat {
		t.Fatal("the view is still the flat list after the modal applied")
	}

	press(t, s, "alt+g")
	if got := screen(s); got == grouped {
		t.Error("alt+g left the grouped view on the screen")
	}
}

func TestViewGrouped(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), nil, 100, 30)
	run(t, s, s.repos.setGroups([]orderpick.Choice{{Value: "owner"}}))
	run(t, s, s.Update(ui.OrderedMsg{}))
	golden.RequireEqual(t, s.View())
}

// At 80 columns the groups' indent must come out of the name and description,
// so the Updated header and every row's age stay inside the border.
func TestViewGroupedNarrow(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), nil, 80, 24)
	run(t, s, s.repos.setGroups([]orderpick.Choice{{Value: "owner"}}))
	run(t, s, s.Update(ui.OrderedMsg{}))
	view := ansi.Strip(s.View())

	if !strings.Contains(view, "Updated") {
		t.Errorf("the Updated header is cut off:\n%s", view)
	}
	rows := 0
	for line := range strings.SplitSeq(view, "\n") {
		if w := ansi.StringWidth(line); w > 80 {
			t.Errorf("line is %d cells wide, want at most 80: %q", w, line)
		}
		if !strings.Contains(line, "Repository number") {
			continue
		}
		rows++
		if !ageEnd.MatchString(line) {
			t.Errorf("row does not end with its age before the border: %q", line)
		}
	}
	if rows == 0 {
		t.Fatalf("no repository rows on the screen:\n%s", view)
	}
}

// ageEnd matches a row that ends with an age cell and then the right border.
var ageEnd = regexp.MustCompile(`(now|\d+[smhdwy])\s*│$`)

func hasNotify(msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(ui.NotifyMsg); ok {
			return true
		}
	}
	return false
}
