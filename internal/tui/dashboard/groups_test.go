package dashboard

import (
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

func TestViewGrouped(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), nil, 100, 30)
	run(t, s, s.repos.setGroups([]orderpick.Choice{{Value: "owner"}}))
	golden.RequireEqual(t, s.View())
}

func hasNotify(msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(ui.NotifyMsg); ok {
			return true
		}
	}
	return false
}
