package actions

import (
	"slices"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// Update handles the modal's keys and its reads, and passes everything else
// to the feed, the log and the form. Messages of other modals, such as the
// one this replaced, are ignored.
func (m *Modal) Update(msg tea.Msg) tea.Cmd {
	return m.over(m.update(msg))
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
			if msg.Over == nil {
				msg.Over = m
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
	if ui.IsArm(msg) {
		m.armed = m.ask != nil
		return nil
	}
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
		return m.readJobs(m.run)
	case ui.AheadMsg:
		return tea.Batch(m.aheadJobs.Rested(msg), m.aheadLogs.Rested(msg))
	case jobsMsg:
		if msg.id != m.id {
			return nil
		}
		cmd := m.receiveJobs(msg)
		// An open run whose jobs arrived may be one to keep up to date.
		m.follow()
		return tea.Batch(cmd, m.startTick())
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
			return m.spin.Update(msg, m.loading())
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
	m.ticking = false
	m.spin.Stop()
	var lost []tea.Cmd
	if m.cur().loading && m.hasRun {
		lost = append(lost, m.readJobs(m.run))
	}
	if m.workflows.loading {
		lost = append(lost, m.readWorkflows(false))
	}
	// The overlay took the runs list's sync messages, so it reloads it.
	lost = append(lost, m.runs.Reload(), m.readJobsAround(), m.readLogsAround(), m.fromCache())
	cmd := tea.Batch(lost...)
	if m.loading() {
		cmd = tea.Batch(cmd, m.spin.Start())
	}
	return tea.Batch(cmd, m.startTick())
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
		return m.viewWorkflow()
	case keymap.Matches(msg, k.RerunFailed):
		return m.asks(m.rerunFailed)
	case keymap.Matches(msg, k.Rerun):
		return m.asks(m.rerunAll)
	case m.inRuns() && m.cursorOnJobs() && keymap.Matches(msg, k.RerunJob):
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
	if m.notes {
		return m.pressNotes(msg)
	}
	return m.updateRuns(msg)
}

// inRuns reports that the runs have the focus.
func (m *Modal) inRuns() bool { return !m.notes && m.focus == runsPane }

// moveFocus moves the focus d stops on, round the ends: the runs and the
// annotations.
func (m *Modal) moveFocus(d int) tea.Cmd {
	const stops = numPanes
	at := int(m.focus)
	if m.notes {
		at = int(notesPane)
	}
	at = (at + d + stops) % stops
	if pane(at) == notesPane {
		m.focusNotes()
		return nil
	}
	return m.focusPane(pane(at))
}

// focusBox focuses the box with the number n of the digit keys: the runs,
// or the annotations.
func (m *Modal) focusBox(n pane) tea.Cmd {
	if n == notesPane {
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

// onJobRow reports whether the cursor is on a row of a job, not a group.
func (m *Modal) onJobRow() bool {
	return m.inRuns() && !m.onGroupRow() && m.cursorOnJobs()
}

// cursorOnJobs reports whether the cursor is on a row under a run.
func (m *Modal) cursorOnJobs() bool {
	_, _, ok := m.runs.SelectedChild()
	return ok
}

// drill folds or opens a run or a group of jobs under the cursor, and opens
// the log of a job over the modal. On the line that says there are more, and on
// the note of loading, an error or no jobs, it does nothing.
func (m *Modal) drill() tea.Cmd {
	if m.notes {
		return m.openNote()
	}
	if m.onGroupRow() {
		m.toggleGroup()
		return m.readLogsAround()
	}
	if m.onJobRow() {
		if j, ok := m.cursorJob(); ok {
			return m.openLog(j, jobview.Hints{})
		}
		return nil
	}
	cmd, _ := m.runs.Activate()
	return cmd
}

// back returns from the annotations to the runs, and from a job row to the
// run it is of, as the collapse key does. It does nothing on a run.
func (m *Modal) back() tea.Cmd {
	// Backspace cancels the wait for the workflows as esc does.
	m.workflows.wanted = false
	if m.notes {
		m.setFocus(m.focus)
		return nil
	}
	if m.inRuns() && m.cursorOnJobs() {
		// Always a step up, even from a group, which h would fold first.
		return m.feedRuns(keymap.ActionPress(ctxRuns + ".collapse"))
	}
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
	if m.cursorOnJobs() {
		return tea.Batch(m.rereadJobs(), m.rereadNotes())
	}
	// The note of a run whose jobs failed to load sits under the run, and the
	// refresh key it names must retry it.
	if j := m.cur(); m.runs.Unfolded() && !j.loaded && j.err != nil {
		return m.rereadJobs()
	}
	return m.runs.Reload()
}

// rereadJobs reads the jobs of the run shown again, and those of the other
// open runs whose jobs are loaded.
func (m *Modal) rereadJobs() tea.Cmd {
	if !m.hasRun {
		return nil
	}
	m.seq++
	m.cur().loading = true
	cmds := []tea.Cmd{m.readJobs(m.run), m.spin.Start()}
	open := m.openLoaded()
	for i := range open {
		cmds = append(cmds, m.rereadRun(open[i]))
	}
	return tea.Batch(cmds...)
}

// openLoaded returns the open runs, other than the one shown, whose jobs are
// loaded, as the cache has them. A finished run whose jobs were read whole
// has nothing new to read, so it is left out unless its jobs are kept or
// failed to load.
func (m *Modal) openLoaded() []core.Run {
	var out []core.Run
	open := m.runs.UnfoldedItems()
	for i := range open {
		r := m.current(open[i])
		j := m.jobsOf[r.ID]
		if j == nil || !j.loaded || (m.hasRun && r.ID == m.run.ID) {
			continue
		}
		stale := !r.Done() || j.kept || j.err != nil ||
			slices.ContainsFunc(j.items, func(x core.Job) bool { return !x.Done() })
		if stale {
			out = append(out, r)
		}
	}
	return out
}

// rereadRun reads the jobs of an open run that isn't shown again.
func (m *Modal) rereadRun(r core.Run) tea.Cmd {
	m.jobsOf[r.ID].loading = true
	return tea.Batch(m.readJobs(r), m.spin.Start())
}

// online reads again, now that GitHub answers again, what failed for want
// of an answer from it: the runs, the jobs of the run shown and the log
// of the job. Runs, jobs, annotations and workflows served from what an
// earlier read kept, while GitHub couldn't be reached or rate limited the
// read, are read again too.
func (m *Modal) online() tea.Cmd {
	cmds := []tea.Cmd{ui.RetryUnreached(&m.runs), m.rereadWorkflows(), m.rereadNotes()}
	if j := m.cur(); !j.loading && (ui.Unreached(j.err) || j.kept) {
		j.kept = false
		cmds = append(cmds, m.rereadJobs())
	}
	runs := m.openLoaded()
	for i := range runs {
		if j := m.jobsOf[runs[i].ID]; !j.loading && (ui.Unreached(j.err) || j.kept) {
			j.kept = false
			cmds = append(cmds, m.rereadRun(runs[i]))
		}
	}
	return tea.Batch(cmds...)
}

// open opens the run, or the job of the jobs, on GitHub. A group has no
// page, so the run opens from it.
func (m *Modal) open() tea.Cmd {
	if m.onJobRow() {
		if j, ok := m.cursorJob(); ok && j.URL != "" {
			return ui.Open(j.URL)
		}
	}
	if m.hasRun && m.run.URL != "" {
		return ui.Open(m.run.URL)
	}
	return nil
}
