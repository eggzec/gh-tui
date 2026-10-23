package actions

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// The runs tell when they started as the date format says.
func TestDates(t *testing.T) {
	m, h := newModal(t, newFake(), 120, 30, WithDates(ui.NewDates(uitest.DateLayout)))
	runs := testRuns()
	// The run's start shows in its details, beside the tree.
	uitest.Dated(t, m.View(), 120, runs[0].CreatedAt)
	h.keys("enter", "j") // fold the first run, then the next run
	uitest.Dated(t, m.View(), 120, runs[1].CreatedAt)
}
