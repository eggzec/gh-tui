package actions

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// Update handles the modal's keys and its reads, and passes everything else
// to the feed, the log and the form. Messages of other modals, such as the
// one this replaced, are ignored.
func (m *Modal) Update(msg tea.Msg) tea.Cmd {
	return m.update(msg)
}

func (m *Modal) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.press(msg)
	case restMsg:
		if msg.id != m.id || msg.seq != m.seq {
			return nil
		}
		return m.readJobs()
	case ui.AheadMsg:
		return tea.Batch(m.aheadJobs.Rested(msg), m.aheadLogs.Rested(msg))
	case jobsMsg:
		if msg.id != m.id {
			return nil
		}
		return tea.Batch(m.receiveJobs(msg), m.startTick())
	case runMsg:
		if msg.id != m.id {
			return nil
		}
		return m.receiveRun(msg)
	case workflowsMsg:
		if msg.id != m.id {
			return nil
		}
		return m.receiveWorkflows(msg)
	case tickMsg:
		if msg.id != m.id {
			return nil
		}
		return m.ticked()
	case ui.SyncMsg:
		return m.synced(msg)
	case ui.OnlineMsg:
		// A rate limit is the token's, and has lifted unless one holds.
		if !msg.Limited {
			m.aheadJobs.Resume()
			m.aheadLogs.Resume()
		}
		return m.online()
	case ui.CapsMsg:
		if msg.Repo.Same(m.repo) {
			m.caps = msg.Caps
		}
		return nil
	case ui.ReopenedMsg:
		if msg.Modal != m {
			return nil
		}
		return m.reopened()
	case ui.DoneMsg:
		if msg.From != Title {
			return nil
		}
		return m.done(msg)
	case spinner.TickMsg:
		if msg.ID == m.spin.ID() {
			return m.spun(msg)
		}
	}
	// Pages and ticks of the bubbles inside.
	cmds := []tea.Cmd{m.updateRuns(msg), m.updateLog(msg)}
	if m.filterStep != nil {
		cmds = append(cmds, m.updateFilter(msg))
	}
	return tea.Batch(cmds...)
}

// reopened starts again what stopped while a preview opened from the modal
// hid it, whose messages went to the preview: the timers and the spinner,
// the reads that were in flight, whose answers were lost, and the run
// shown, read from the cache in case a poll moved it.
func (m *Modal) reopened() tea.Cmd {
	m.ticking, m.spinning = false, false
	var lost []tea.Cmd
	if m.jobs.loading && m.hasRun {
		lost = append(lost, m.readJobs())
	}
	if m.workflows.loading {
		lost = append(lost, m.readWorkflows(false))
	}
	lost = append(lost, m.log.ReadLost())
	cmd := tea.Batch(append(lost, m.fromCache())...)
	if m.loading() {
		cmd = tea.Batch(cmd, m.startSpinner())
	}
	return tea.Batch(cmd, m.startTick())
}

// spun spins the spinner while something loads.
func (m *Modal) spun(msg spinner.TickMsg) tea.Cmd {
	if !m.loading() {
		m.spinning = false
		return nil
	}
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return cmd
}

func (m *Modal) updateLog(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.log, cmd = m.log.Update(msg)
	return cmd
}

// press handles a key: the confirmation takes the answer, the filter and a
// search of the log take every key, the modal's own keys come next, and
// then those of the focused pane.
func (m *Modal) press(msg tea.KeyPressMsg) tea.Cmd {
	m.notice = ""
	if m.ask != nil {
		return m.answer(msg)
	}
	if m.filterStep != nil {
		if m.filterStep.form == nil && keymap.Matches(msg, m.keys.Dismiss) {
			return m.dismiss()
		}
		return m.updateFilter(msg)
	}
	inLog := m.focus == logPane
	if inLog && m.log.Capturing() {
		return m.updateLog(msg)
	}
	if m.focus == runsPane && m.runs.Takes(msg) {
		// The prompt of the runs' find or filter takes the keys before
		// the modal's own.
		return m.updateRuns(msg)
	}
	k := m.keys
	switch {
	// The tabs share ] and [ with the panes, and take them first.
	case keymap.Matches(msg, k.NextTab):
		return m.switchTab(1)
	case keymap.Matches(msg, k.PrevTab):
		return m.switchTab(-1)
	case keymap.Matches(msg, k.Next):
		return m.moveFocus(1)
	case keymap.Matches(msg, k.Prev):
		return m.moveFocus(-1)
	case k.focusOf(msg) >= 0:
		return m.focusPane(k.focusOf(msg))
	case m.focus == runsPane && keymap.Matches(msg, k.Filter):
		return m.openFilter()
	case m.focus == runsPane && keymap.Matches(msg, k.ClearFilter):
		return m.setFilter(cleared(m.filter))
	case keymap.Matches(msg, k.Zoom):
		m.zoom = !m.zoom
		m.layout()
		return nil
	case keymap.Matches(msg, k.Open):
		return m.open()
	case keymap.Matches(msg, k.RerunFailed):
		return m.asks(m.rerunFailed)
	case keymap.Matches(msg, k.Rerun):
		return m.asks(m.rerunAll)
	case m.focus != runsPane && keymap.Matches(msg, k.rerunJob(m)):
		return m.asks(m.rerunJob)
	case keymap.Matches(msg, k.Cancel):
		return m.asks(m.cancelRun)
	case keymap.Matches(msg, k.Refresh):
		return m.refresh()
	case keymap.Matches(msg, k.Dismiss):
		return m.dismiss()
	case keymap.Matches(msg, k.Back):
		return m.back()
	case keymap.Matches(msg, k.Select) && m.focus != logPane:
		return m.drill()
	}
	switch m.focus {
	case runsPane:
		return m.updateRuns(msg)
	case jobsPane:
		return m.pressJobs(msg)
	default:
		return m.updateLog(msg)
	}
}

// moveFocus moves the focus d panes on, round the ends.
func (m *Modal) moveFocus(d int) tea.Cmd {
	return m.focusPane(pane((int(m.focus) + d + numPanes) % numPanes))
}

// focusPane focuses p, and reads what it shows at once rather than when
// the cursor rests.
func (m *Modal) focusPane(p pane) tea.Cmd {
	m.setFocus(p)
	if p != logPane {
		return nil
	}
	return m.log.ReadNow()
}

// setFocus focuses pane p, and blurs the others.
func (m *Modal) setFocus(p pane) {
	m.focus = p
	if p == runsPane {
		m.runs.Focus()
	} else {
		m.runs.Blur()
	}
	if p == logPane {
		m.log.Focus()
	} else {
		m.log.Blur()
	}
	m.layout()
}

// drill focuses the pane after the focused one, or folds the group of
// jobs under the cursor, or opens it.
func (m *Modal) drill() tea.Cmd {
	switch m.focus {
	case runsPane:
		if !m.hasRun {
			return nil
		}
		return m.focusPane(jobsPane)
	case jobsPane:
		if m.jobs.onGroup() {
			m.jobs.toggle()
			m.scrollJobs()
			return m.readLogsAround()
		}
		if _, ok := m.jobs.selected(); !ok {
			return nil
		}
		return m.focusPane(logPane)
	case logPane:
	}
	return nil
}

// back steps back one pane: the log to the jobs, and the jobs to the runs.
// It does nothing on the runs.
func (m *Modal) back() tea.Cmd {
	if m.focus == runsPane {
		return nil
	}
	m.setFocus(m.focus - 1)
	return nil
}

// dismiss peels one layer at a time: the filter before its form shows, the
// search of the log, the find or the quick filter of the runs, and when
// none is left, the modal.
func (m *Modal) dismiss() tea.Cmd {
	if m.filterStep != nil {
		m.filterStep = nil
		return nil
	}
	switch {
	case m.focus == logPane && m.log.ClearSearch():
		return nil
	case m.focus == runsPane:
		if cmd, ok := m.runs.ClearTransient(); ok {
			return cmd
		}
	}
	return m.close()
}

// refresh reads again what the focused pane shows, or what failed to
// load.
func (m *Modal) refresh() tea.Cmd {
	// A refresh tries again the reads ahead that failed lately.
	m.aheadJobs.Resume()
	m.aheadLogs.Resume()
	switch m.focus {
	case jobsPane:
		return m.rereadJobs()
	case logPane:
		return m.log.Retry()
	case runsPane:
	}
	return m.runs.Reload()
}

// rereadJobs reads the jobs of the run shown again.
func (m *Modal) rereadJobs() tea.Cmd {
	if !m.hasRun {
		return nil
	}
	m.seq++
	m.jobs.loading = true
	return tea.Batch(m.readJobs(), m.startSpinner())
}

// online reads again, now that GitHub answers again, what failed for want
// of an answer from it: the runs, the jobs of the run shown and the log
// of the job. Runs, jobs, annotations and workflows served from what an
// earlier read kept, while GitHub couldn't be reached or rate limited the
// read, are read again too.
func (m *Modal) online() tea.Cmd {
	cmds := []tea.Cmd{ui.RetryUnreached(&m.runs), ui.RetryUnreached(&m.log), m.rereadWorkflows()}
	if j := &m.jobs; !j.loading && (ui.Unreached(j.err) || j.kept) {
		j.kept = false
		cmds = append(cmds, m.rereadJobs())
	}
	return tea.Batch(cmds...)
}

// open opens the run, or the job of the jobs and the log panes, on GitHub.
// A group has no page, so the run opens from it.
func (m *Modal) open() tea.Cmd {
	if m.focus == logPane || m.focus == jobsPane && !m.jobs.onGroup() {
		if j, ok := m.jobs.selected(); ok && j.URL != "" {
			return ui.Open(j.URL)
		}
	}
	if m.hasRun && m.run.URL != "" {
		return ui.Open(m.run.URL)
	}
	return nil
}
