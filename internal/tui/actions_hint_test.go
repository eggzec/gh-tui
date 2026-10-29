package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// On a failed job, the status line offers to re-run the job at the widths
// people use, with the rate limits and the connection on its right: the
// moves of the list give way to it when the bar is short.
func TestStatusBarOfAJobRowOffersRerunJob(t *testing.T) {
	t.Parallel()
	for _, w := range []int{120, 80} {
		c := keyContext{name: "actions: jobs", repo: true, steps: []string{"repo.actions", "actions_runs.expand", "actions_runs.down"}, context: "actions_runs", want: "global, actions, actions_runs"}
		m, _ := c.reach(t)
		m.login = "someone"
		now := time.Now()
		m.rate = core.RateStatus{At: now, Answered: now, Quotas: []core.Quota{
			{Resource: "core", Limit: 5000, Remaining: 4800},
		}}
		m.Update(tea.WindowSizeMsg{Width: w, Height: 40})
		m.drawStatus()
		m.View()
		if bar := m.bar(); !strings.Contains(bar, "rerun job") {
			t.Errorf("width %d: the status line on a failed job is %q, want J rerun job", w, bar)
		}
	}
}
