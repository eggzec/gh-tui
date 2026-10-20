package search

import (
	"strings"
	"testing"
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
