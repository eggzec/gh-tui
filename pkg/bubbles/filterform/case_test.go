package filterform

import (
	"slices"
	"testing"
)

// A value typed in another case ticks the option it matches.
func TestMultiTypedInAnotherCase(t *testing.T) {
	spec := Spec{Fields: []Field{{
		Key: "l", Kind: Multi, Qualifier: "label",
		Options: []Item{{Value: "bug"}, {Value: "docs"}},
	}}}
	m := newKeyed(t, spec)
	m.SetQuery("label:BUG,Docs")
	got := m.Values()["l"].List()
	if !slices.Equal(got, []string{"bug", "docs"}) {
		t.Fatalf("values = %q, want bug and docs", got)
	}
}
