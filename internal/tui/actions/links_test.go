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
			m.cur().items = []core.Job{j}
			var line jobLine
			line.job, line.label = 0, title
			m.cur().lines = []jobLine{line}
			m.inner = width
			// A name that wraps links on each of its lines; the first is
			// the row to check here.
			return m.lineRows(0, selected, true, width, testNow)[0], j.URL
		}, 120, 40, 12, 3)
	}
}

// The run's row keeps to its name and title; how long it took shows in its
// details, and a run whose jobs never started has no duration.
func TestRunDurationShowsInTheDetails(t *testing.T) {
	m, _ := newModal(t, newFake(), 120, 30)
	at := testNow.Add(-time.Hour)
	r := core.Run{ID: 1, Name: "CI", Number: 7, DisplayTitle: "a run", Status: core.RunCompleted,
		Conclusion: core.ConclusionSuccess, CreatedAt: at, UpdatedAt: at.Add(65 * time.Second)}
	if s := ansi.Strip(m.renderRun(r, false, 80)); strings.Contains(s, "1m 5s") {
		t.Errorf("the run's row tells its duration\n%s", s)
	}
	m.cur().items = []core.Job{{StartedAt: at, CompletedAt: at.Add(65 * time.Second)}}
	if got := m.jobsSpan(testNow); got != "1m 5s" {
		t.Errorf("a run of 65s reads %q", got)
	}
	m.cur().items = []core.Job{{}}
	if got := m.jobsSpan(testNow); got != "-" {
		t.Errorf("a run that never ran reads %q, want -", got)
	}
}
