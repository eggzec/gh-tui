package filterform

import (
	"slices"
	"strings"
	"testing"
)

// A value the checklist has that is no option survives opening it, ticking
// another row and applying.
func TestChecklistKeepsValuesThatAreNoOption(t *testing.T) {
	spec := Spec{Fields: []Field{{
		Key: "l", Kind: Multi, Qualifier: "label",
		Options: []Item{{Value: "bug"}, {Value: "docs"}},
	}}}
	m := open(t, spec)
	m.SetQuery("label:foo,bug")
	m, _ = press(t, m, space, keyJ, space, enter)
	got := m.Values()["l"].List()
	for _, want := range []string{"foo", "bug", "docs"} {
		if !slices.Contains(got, want) {
			t.Errorf("values = %q, want %q kept", got, want)
		}
	}
}

// Space in a list of one choice leaves its one mark.
func TestSpaceInAChoiceListKeepsOneMark(t *testing.T) {
	m := open(t, prSpec(nil), WithSize(80, 14))
	m, _ = press(t, m, down, down, space, keyJ, space)
	if v := view(m); strings.Count(v, "●") != 1 {
		t.Errorf("want exactly one mark after space in a choice list:\n%s", v)
	}
}
