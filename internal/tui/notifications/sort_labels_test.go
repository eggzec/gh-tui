package notifications

import (
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// TestUpdatedLabelsMatchTheOrder checks that the direction shown for the time
// of an update is the order the list takes.
func TestUpdatedLabelsMatchTheOrder(t *testing.T) {
	t.Parallel()
	now := time.Now()
	threads := []core.Notification{{ID: "old", UpdatedAt: now.Add(-time.Hour)}, {ID: "new", UpdatedAt: now}}
	var item orderpick.Item
	for _, it := range sortItems() {
		if it.Value == updatedField {
			item = it
		}
	}
	s := &Section{}
	for _, tc := range []struct {
		label string
		desc  bool
		first string
	}{{"Newest first", false, "new"}, {"Oldest first", true, "old"}} {
		shown := item.AscLabel
		if tc.desc {
			shown = item.DescLabel
		}
		if shown != tc.label {
			t.Errorf("desc=%v shows %q, want %q", tc.desc, shown, tc.label)
		}
		got := slices.Clone(threads)
		slices.SortStableFunc(got, s.sortLess([]orderpick.Choice{{Value: updatedField, Desc: tc.desc}}))
		if got[0].ID != tc.first {
			t.Errorf("%s: first thread is %s, want %s", tc.label, got[0].ID, tc.first)
		}
	}
}
