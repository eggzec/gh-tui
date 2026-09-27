package checks

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// A check links its name to where it points, whatever its width and
// whether the cursor is on it, and so does a commit status.
func TestRowLinks(t *testing.T) {
	s, _ := newStep(t, newFake(), 100, 20)
	for _, cursor := range []bool{false, true} {
		uitest.CheckLinks(t, func(host, title string, width int) (string, string) {
			c := &core.Check{Name: title, Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
				DetailsURL: ui.WebURL(host, "o/r/actions/runs/1/job/2")}
			return s.renderRow(row{check: c}, cursor, width, testNow), c.DetailsURL
		}, 100, 40, 12, 3)
		uitest.CheckLinks(t, func(host, title string, width int) (string, string) {
			st := &core.StatusContext{Context: title, State: "success", TargetURL: ui.WebURL(host, "o/r/status")}
			return s.renderRow(row{status: st}, cursor, width, testNow), st.TargetURL
		}, 100, 40, 12, 3)
	}
}
