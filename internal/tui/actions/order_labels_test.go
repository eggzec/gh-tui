package actions

import (
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// TestCreatedLabelsMatchTheOrder checks that the direction shown for the time
// of a run is the order the list takes.
func TestCreatedLabelsMatchTheOrder(t *testing.T) {
	t.Parallel()
	now := time.Now()
	runs := []core.Run{{ID: 1, CreatedAt: now.Add(-time.Hour)}, {ID: 2, CreatedAt: now}}
	var item orderpick.Item
	for _, it := range sortItems() {
		if it.Value == createdField {
			item = it
		}
	}
	for _, tc := range []struct {
		label string
		desc  bool
		first int64
	}{{"Newest first", false, 2}, {"Oldest first", true, 1}} {
		shown := item.AscLabel
		if tc.desc {
			shown = item.DescLabel
		}
		if shown != tc.label {
			t.Errorf("desc=%v shows %q, want %q", tc.desc, shown, tc.label)
		}
		got := slices.Clone(runs)
		slices.SortStableFunc(got, sortLess([]orderpick.Choice{{Value: createdField, Desc: tc.desc}}))
		if got[0].ID != tc.first {
			t.Errorf("%s: first run is %d, want %d", tc.label, got[0].ID, tc.first)
		}
	}
}
