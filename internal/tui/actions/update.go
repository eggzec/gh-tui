package actions

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
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
	case logview.CloseMsg:
		if msg.ID == m.log.LogID() {
			return m.back()
		}
		return nil
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
// and the run shown, read from the cache in case a poll moved it.
func (m *Modal) reopened() tea.Cmd {
	m.ticking, m.spinning = false, false
	cmd := m.fromCache()
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
		if m.filterStep.form == nil && key.Matches(msg, m.keys.Back) {
			m.filterStep = nil
			return nil
		}
		return m.updateFilter(msg)
	}
	inLog := m.focus == logPane
	if inLog && m.log.Capturing() {
		return m.updateLog(msg)
	}
	k := m.keys
	switch {
	// The tabs share ] and [ with the panes, and take them first.
	case key.Matches(msg, k.NextTab):
		return m.switchTab(1)
	case key.Matches(msg, k.PrevTab):
		return m.switchTab(-1)
	case key.Matches(msg, k.Next):
		return m.moveFocus(1, true)
	case key.Matches(msg, k.Prev):
		return m.moveFocus(-1, true)
	case key.Matches(msg, k.Right):
		return m.moveFocus(1, false)
	case key.Matches(msg, k.Left):
		return m.moveFocus(-1, false)
	case key.Matches(msg, k.Filter):
		return m.openFilter()
	case key.Matches(msg, k.Zoom):
		m.zoom = !m.zoom
		m.layout()
		return nil
	case key.Matches(msg, k.Open):
		return m.open()
	case key.Matches(msg, k.RerunFailed):
		return m.asks(m.rerunFailed)
	case key.Matches(msg, k.Rerun):
		return m.asks(m.rerunAll)
	case key.Matches(msg, k.RerunJob) && m.focus != runsPane:
		return m.asks(m.rerunJob)
	case key.Matches(msg, k.Cancel):
		return m.asks(m.cancelRun)
	case key.Matches(msg, k.Refresh):
		return m.refresh()
	case key.Matches(msg, k.Back):
		if inLog && m.log.Query() != "" {
			// The back key clears the search first.
			return m.updateLog(msg)
		}
		return m.back()
	case key.Matches(msg, k.Select) && m.focus != logPane:
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

// moveFocus moves the focus d panes on, round the ends if round is set.
func (m *Modal) moveFocus(d int, round bool) tea.Cmd {
	to := int(m.focus) + d
	switch {
	case round:
		to = (to + numPanes) % numPanes
	case to < 0 || to >= numPanes:
		return nil
	}
	return m.focusPane(pane(to))
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

// back steps back one pane, and closes the modal from the runs. A zoomed
// pane goes back to its place first.
func (m *Modal) back() tea.Cmd {
	switch {
	case m.zoom:
		m.zoom = false
		m.layout()
	case m.focus == runsPane:
		return m.close()
	default:
		m.setFocus(m.focus - 1)
	}
	return nil
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
