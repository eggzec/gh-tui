package actions

import (
	tea "charm.land/bubbletea/v2"
)

// showJob reads ahead the logs around the job under the cursor of the jobs,
// and keeps the log of the job itself, so that opening it finds it read, or
// being read.
func (m *Modal) showJob() tea.Cmd {
	defer m.layout()
	around := m.readLogsAround()
	j, ok := m.jobs.selected()
	if !ok {
		m.aheadLogs.Unkeep()
		return around
	}
	m.aheadLogs.Opened(j.ID)
	// The log overlay reads the job's log itself, and joins its read ahead,
	// which is left to go on.
	m.aheadLogs.Keep(j.ID)
	return around
}
