package actions

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// A run links its workflow, number and title to its page, and a job its
// name, whatever their width and whether they are selected.
func TestRowLinks(t *testing.T) {
	m, _ := newModal(t, newFake(), 120, 30)
	for _, selected := range []bool{false, true} {
		uitest.CheckLinks(t, func(host, title string, width int) (string, string) {
			r := core.Run{ID: 1, Name: "CI", Number: 7, DisplayTitle: title, Status: core.RunCompleted,
				Conclusion: core.ConclusionSuccess, URL: ui.WebURL(host, "o/r/actions/runs/1")}
			return m.renderRun(r, selected, width), r.URL
		}, 120, 40, 12, 3)
		uitest.CheckLinks(t, func(host, title string, width int) (string, string) {
			j := core.Job{ID: 2, Name: title, Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
				URL: ui.WebURL(host, "o/r/actions/runs/1/job/2")}
			return m.jobRow(j, selected, true, width, testNow), j.URL
		}, 120, 40, 12, 3)
	}
}
