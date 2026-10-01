package actions

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

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
			return m.jobRow(j, title, m.rowGutter(selected, true), selected, width, testNow), j.URL
		}, 120, 40, 12, 3)
	}
}

// A run that ended as it was created never ran, so it shows no duration,
// and a skipped one says so, as its jobs and steps do; one that ran shows
// how long.
func TestRunThatNeverRanHasNoDuration(t *testing.T) {
	m, _ := newModal(t, newFake(), 120, 30)
	at := testNow.Add(-time.Hour)
	cancelled := core.Run{ID: 1, Name: "CI", Number: 7, DisplayTitle: "never ran", Status: core.RunCompleted,
		Conclusion: core.ConclusionCancelled, CreatedAt: at, UpdatedAt: at}
	if s := ansi.Strip(m.renderRun(cancelled, false, 80)); strings.Contains(s, "0s") {
		t.Errorf("a run that never ran reads\n%s", s)
	}
	skipped := cancelled
	skipped.Conclusion = core.ConclusionSkipped
	if s := ansi.Strip(m.renderRun(skipped, false, 80)); strings.Contains(s, "0s") || !strings.Contains(s, "skipped") {
		t.Errorf("a skipped run reads\n%s", s)
	}
	ran := skipped
	ran.Conclusion, ran.UpdatedAt = core.ConclusionSuccess, at.Add(65*time.Second)
	if s := ansi.Strip(m.renderRun(ran, false, 80)); !strings.Contains(s, "1m 5s") {
		t.Errorf("a run of 65s reads\n%s", s)
	}
}
