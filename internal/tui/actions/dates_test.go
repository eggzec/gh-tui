package actions

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// The runs tell when they started as the date format says.
func TestDates(t *testing.T) {
	m, h := newModal(t, newFake(), 120, 30, WithDates(ui.NewDates(uitest.DateLayout)))
	h.keys("z") // the runs alone, wide enough for the dates
	runs := testRuns()
	uitest.Dated(t, m.View(), 120, runs[0].CreatedAt, runs[1].CreatedAt)
}
