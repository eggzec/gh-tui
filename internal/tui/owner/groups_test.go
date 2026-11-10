package owner

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// G opens the choice of levels on the repositories and the stars, choosing
// one groups the list, alt+g lists it flat and sets the off chip apart, and
// alt+g with no levels says so. The other tabs have no grouping.
func TestGroupKeys(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), "octocat", 100, 30)

	if msgs := press(t, s, "alt+g"); !toasted(msgs) {
		t.Error("alt+g with no levels said nothing")
	}
	if _, ok := s.Update(keyPress("g"))().(ui.OpenModalMsg); !ok {
		t.Fatal("g did not open the group modal on the repositories")
	}

	run(t, s, s.setGroups([]orderpick.Choice{{Value: "owner"}}))
	if s.grouping.Off || s.grouping.Groups() == nil {
		t.Fatalf("choosing a level left off %v, groups %v", s.grouping.Off, s.grouping.Groups())
	}

	press(t, s, "alt+g")
	if !s.grouping.Off || s.grouping.Groups() != nil {
		t.Errorf("alt+g left off %v, groups %v; want it off", s.grouping.Off, s.grouping.Groups())
	}
	if chip := ansi.Strip(s.grouping.Chip(s.icons.Separator)); chip == "" || !strings.Contains(ansi.Strip(s.View()), chip) {
		t.Errorf("the off chip %q is not on the screen", chip)
	}
	press(t, s, "alt+g")
	if s.grouping.Off {
		t.Error("alt+g again left the grouping off")
	}

	press(t, s, "]")
	if s.page.tab != starsTab {
		t.Fatalf("] reached tab %v, want the stars", s.page.tab)
	}
	if _, ok := s.Update(keyPress("g"))().(ui.OpenModalMsg); !ok {
		t.Error("g did not open the group modal on the stars")
	}

	press(t, s, "]")
	if s.page.tab == reposTab || s.page.tab == starsTab {
		t.Fatalf("] reached tab %v, want one past the stars", s.page.tab)
	}
	if cmd := s.Update(keyPress("g")); cmd != nil {
		if _, ok := cmd().(ui.OpenModalMsg); ok {
			t.Error("g opened the group modal on a tab that has no grouping")
		}
	}
}

// The group modal applies its choice from inside the modal, so the section
// sees only the OrderedMsg after it: that alone must draw the groups, and one
// alt+g must switch them off in the view.
func TestGroupsShowAfterTheModalApplies(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), "octocat", 100, 30)
	flat := ansi.Strip(s.View())

	s.setGroups([]orderpick.Choice{{Value: "owner"}})
	s.Update(ui.OrderedMsg{})
	grouped := ansi.Strip(s.View())
	if grouped == flat {
		t.Fatal("the view is still the flat list after the modal applied")
	}

	press(t, s, "alt+g")
	if got := ansi.Strip(s.View()); got == grouped {
		t.Error("alt+g left the grouped view on the screen")
	}
}

func TestViewGrouped(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), "octocat", 100, 30)
	run(t, s, s.setGroups([]orderpick.Choice{{Value: "owner"}}))
	run(t, s, s.Update(ui.OrderedMsg{}))
	golden.RequireEqual(t, s.View())
}

func TestViewGroupedNarrow(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), "octocat", 80, 24)
	run(t, s, s.setGroups([]orderpick.Choice{{Value: "owner"}}))
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
