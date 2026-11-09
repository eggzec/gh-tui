package issues

import (
	"slices"
	"strconv"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Opening a picker before the issue is read says so, as the editor of the
// description does, and opens nothing.
func TestPickersWaitForTheIssue(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		open func(m *detailModal) tea.Cmd
	}{
		{"labels", (*detailModal).openLabels},
		{"assignees", (*detailModal).openAssignees},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, m := opened(t, newFakeService(sampleIssues(12)), 30)
			m.loaded = false
			msgs := run(t, h, tt.open(m))
			want := info("Still reading #" + strconv.Itoa(m.number) + ".")
			if !slices.Contains(msgs, want) {
				t.Errorf("fed %v, want %v", msgs, want)
			}
			if m.over != nil {
				t.Errorf("opened %T before the issue was read", m.over)
			}
		})
	}
}
