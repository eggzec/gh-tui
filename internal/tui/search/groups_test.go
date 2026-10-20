package search

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// g opens the choice of the levels to group the repositories by, and alt+g
// has nothing to switch while no level is chosen. Neither acts on the other
// kinds.
func TestGroupKeys(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 120, 30)
	typeText(t, s, "tea")
	press(t, s, "esc")
	press(t, s, "alt+g")
	if s.grouping.Levels != nil {
		t.Fatalf("alt+g chose levels %v, want none", s.grouping.Levels)
	}
	if !strings.Contains(s.View(), "Repositories") {
		t.Fatalf("the results lost their label:\n%s", s.View())
	}
	press(t, s, "g")
	if got := len(s.keys.state(s).GroupBy.Keys()); got == 0 {
		t.Fatal("group by has no key on repositories")
	}
	press(t, s, "esc", "]")
	if s.keys.state(s).GroupBy.Enabled() {
		t.Error("group by is enabled on issues")
	}
}

// The label of the results tells the grouping is off, set apart from the
// count.
func TestGroupOffLabel(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 120, 30)
	typeText(t, s, "tea")
	press(t, s, "esc")
	s.grouping.Choose([]orderpick.Choice{{Value: "owner"}})
	press(t, s, "alt+g")
	if !s.grouping.Off {
		t.Fatal("alt+g did not switch the grouping off")
	}
	chip := s.grouping.Chip(s.icons.Separator)
	if chip == "" {
		t.Fatal("no chip while off")
	}
	if got := s.resultsLabel(); !strings.Contains(got, s.icons.Separator+chip) {
		t.Errorf("label %q does not set the chip %q apart", got, chip)
	}
}
