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
	cmd := m.update(msg)
	m.fit()
	return m.over(cmd)
}

// over makes the modals that cmd opens, the filter, sort and group, open over
// the modal, so that closing them returns here. Hide stops only what the
// modal follows, and reopened starts it again on the way back.
func (m *Modal) over(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case ui.OpenModalMsg:
			if msg.Back == nil {
				msg.Back = m
			}
			return msg
		case tea.BatchMsg:
			for i, c := range msg {
				msg[i] = m.over(c)
			}
			return msg
		default:
			return msg
		}
	}
}

func (m *Modal) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ui.SettingsMsg:
		msg.ReloadIcons(&m.opts.icons, m.opts.voice)
		return nil
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
	case notesMsg:
		if msg.id != m.id {
			return nil
		}
		m.receiveNotes(msg)
		return nil
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
	return m.updateRuns(msg)
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
	// The overlay took the runs list's sync messages, so it reloads it.
	lost = append(lost, m.runs.Reload(), m.readJobsAround(), m.readLogsAround(), m.fromCache())
	cmd := tea.Batch(lost...)
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

// press handles a key: the confirmation takes the answer, the filter takes
// every key, the modal's own keys come next, and
// then those of the focused pane.
func (m *Modal) press(msg tea.KeyPressMsg) tea.Cmd {
	m.notice = ""
	if m.ask != nil {
		return m.answer(msg)
	}
	if m.inRuns() && m.runs.Takes(msg) {
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
		return m.focusBox(k.focusOf(msg))
	case keymap.Matches(msg, k.Zoom):
		m.zoom = !m.zoom && m.canZoom()
		m.layout()
		return nil
	case m.inRuns() && keymap.Matches(msg, k.Filter):
		return m.openFilter()
	case m.inRuns() && keymap.Matches(msg, k.ClearFilter):
		m.toggle.On()
		return m.setFilter(cleared(m.filter))
	case m.inRuns() && keymap.Matches(msg, k.ToggleFilter):
		next, ok := m.toggle.Switch(m.filter, cleared(m.filter))
		if !ok {
			return ui.NothingToSwitch()
		}
		return m.setFilter(next)
	case m.inRuns() && keymap.Matches(msg, k.GroupBy):
		return ui.OpenModal(m.groupModal())
	case m.inRuns() && keymap.Matches(msg, k.ToggleGroups):
		cmd, ok := m.toggleGroups()
		if !ok {
			return ui.NothingToSwitch()
		}
		return cmd
	case m.inRuns() && keymap.Matches(msg, k.Sort):
		return ui.OpenModal(m.sortModal())
	case m.inRuns() && keymap.Matches(msg, k.ToggleSort):
		cmd, ok := m.toggleSort()
		if !ok {
			return ui.NothingToSwitch()
		}
		return cmd
	case keymap.Matches(msg, k.Open):
		return m.open()
	case keymap.Matches(msg, k.ViewFile):
		return m.viewWorkflow(m)
	case keymap.Matches(msg, k.RerunFailed):
		return m.asks(m.rerunFailed)
	case keymap.Matches(msg, k.Rerun):
		return m.asks(m.rerunAll)
	case m.inJobs() && keymap.Matches(msg, k.RerunJob):
		return m.asks(m.rerunJob)
	case keymap.Matches(msg, k.Cancel):
		return m.asks(m.cancelRun)
	case keymap.Matches(msg, k.Refresh):
		return m.refresh()
	case keymap.Matches(msg, k.Dismiss):
		return m.dismiss()
	case keymap.Matches(msg, k.Back):
		return m.back()
	case keymap.Matches(msg, k.Select):
		return m.drill()
	}
	switch {
	case m.notes:
		return m.pressNotes(msg)
	case m.focus == runsPane:
		if keymap.Matches(msg, m.keys.Jobs.Down) && m.rowsOf(jobsPane) > 0 && len(m.jobs.lines) > 0 {
			// Down from a run with jobs under it goes into them.
			m.setFocus(jobsPane)
			m.scrollJobs()
			return m.showJob()
		}
		return m.updateRuns(msg)
	}
	return m.pressJobs(msg)
}

// inRuns and inJobs report that the runs, or the jobs, have the focus.
func (m *Modal) inRuns() bool { return !m.notes && m.focus == runsPane }
func (m *Modal) inJobs() bool { return !m.notes && m.focus == jobsPane }

// moveFocus moves the focus d stops on, round the ends: the runs, the jobs,
// and the annotations.
func (m *Modal) moveFocus(d int) tea.Cmd {
	const stops = numPanes + 1
	at := int(m.focus)
	if m.notes {
		at = numPanes
	}
	at = (at + d + stops) % stops
	if pane(at) == jobsPane && m.rowsOf(jobsPane) <= 0 {
		// The jobs have no room, or are folded: step over them.
		at = (at + d + stops) % stops
	}
	if at == numPanes {
		m.focusNotes()
		return nil
	}
	return m.focusPane(pane(at))
}

// focusBox focuses the box with the number n of the digit keys: the runs,
// or the annotations.
func (m *Modal) focusBox(n pane) tea.Cmd {
	if n == 1 {
		m.focusNotes()
		return nil
	}
	return m.focusPane(runsPane)
}

// focusNotes focuses the annotations box.
func (m *Modal) focusNotes() {
	m.notes = true
	m.runs.Blur()
	m.layout()
}

// focusPane focuses p.
func (m *Modal) focusPane(p pane) tea.Cmd {
	m.setFocus(p)
	return nil
}

// setFocus focuses pane p, and blurs the others.
func (m *Modal) setFocus(p pane) {
	m.focus, m.notes = p, false
	if p == runsPane {
		m.runs.Focus()
	} else {
		m.runs.Blur()
	}
	m.layout()
}

// drill focuses the jobs from the runs, and from the jobs folds the group
// under the cursor, or opens the log of the job over the modal.
func (m *Modal) drill() tea.Cmd {
	if m.notes {
		return m.openNote()
	}
	switch m.focus {
	case runsPane:
		if cmd, more := m.runs.Activate(); more {
			return cmd
		}
		if _, ok := m.runs.Selected(); !ok || !m.hasRun {
			return nil
		}
		// A run folds its jobs away, or shows them again.
		if m.shut == nil {
			m.shut = map[int64]bool{}
		}
		m.shut[m.run.ID] = !m.shut[m.run.ID]
		return nil
	case jobsPane:
		if m.jobs.onGroup() {
			m.jobs.toggle()
			m.scrollJobs()
			return m.readLogsAround()
		}
		j, ok := m.jobs.selected()
		if !ok {
			return nil
		}
		return m.openLog(j)
	}
	return nil
}

// back steps back from the jobs to the runs. It does nothing on the runs.
func (m *Modal) back() tea.Cmd {
	// Backspace cancels the wait for the workflows as esc does.
	m.workflows.wanted = false
	if m.notes {
		m.setFocus(m.focus)
		return nil
	}
	if m.focus == runsPane {
		return nil
	}
	m.setFocus(m.focus - 1)
	return nil
}

// dismiss peels one layer at a time: the wait for the filter, the find or the quick filter of the
// runs, and when none is left, the modal, which returns to the modal it was
// opened over if there is one.
func (m *Modal) dismiss() tea.Cmd {
	// The wait for the workflows is a layer too: workflows arriving later
	// open no filter nobody asks for.
	if m.workflows.wanted {
		m.workflows.wanted = false
		return nil
	}
	if m.inRuns() {
		if cmd, ok := m.runs.ClearTransient(); ok {
			return cmd
		}
	}
	return m.leave()
}

// refresh reads again what the focused pane shows, or what failed to
// load.
func (m *Modal) refresh() tea.Cmd {
	// A refresh tries again the reads ahead that failed lately.
	m.aheadJobs.Resume()
	m.aheadLogs.Resume()
	if m.notes {
		return m.rereadNotes()
	}
	if m.focus == jobsPane {
		return tea.Batch(m.rereadJobs(), m.rereadNotes())
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
	cmds := []tea.Cmd{ui.RetryUnreached(&m.runs), m.rereadWorkflows(), m.rereadNotes()}
	if j := &m.jobs; !j.loading && (ui.Unreached(j.err) || j.kept) {
		j.kept = false
		cmds = append(cmds, m.rereadJobs())
	}
	return tea.Batch(cmds...)
}

// open opens the run, or the job of the jobs, on GitHub. A group has no
// page, so the run opens from it.
func (m *Modal) open() tea.Cmd {
	if m.inJobs() && !m.jobs.onGroup() {
		if j, ok := m.jobs.selected(); ok && j.URL != "" {
			return ui.Open(j.URL)
		}
	}
	if m.hasRun && m.run.URL != "" {
		return ui.Open(m.run.URL)
	}
	return nil
}
