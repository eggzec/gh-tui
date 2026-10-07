package actions

import (
	"slices"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
)

// KeyMap holds the keys of the modal and of the bubbles it shows. The
// modal matches its own keys first, so the bubbles get only the keys it
// leaves them.
type KeyMap struct {
	// form holds the keys of the filter form, as the other filters have them.
	form filterform.KeyMap
	// Next and Prev move the focus through the panes, and Left and Right
	// to the pane beside the focused one.
	Next, Prev  key.Binding
	Left, Right key.Binding
	// NextTab and PrevTab switch between All, Failing, Running and Mine.
	NextTab, PrevTab key.Binding
	// Select drills into the pane after the focused one.
	Select key.Binding
	// Back closes the filter or the confirmation, or steps back a pane,
	// and closes the modal from the runs.
	Back key.Binding
	// Filter edits the filter of the runs, in a step of the modal.
	Filter key.Binding
	// Zoom shows the focused pane alone.
	Zoom key.Binding
	// Open opens the run or the job on GitHub.
	Open key.Binding
	// Refresh reads the runs again, or what failed to load.
	Refresh key.Binding
	// RerunFailed, Rerun, RerunJob and Cancel change the selected run,
	// once the user confirms. RerunJob has the key of the jobs, and
	// rerunLog and rerunNotes those of the log and the annotations.
	RerunFailed, Rerun, RerunJob, Cancel key.Binding
	rerunLog, rerunNotes                 key.Binding
	// Confirm answers the confirmation.
	Confirm ui.ConfirmKeys
	// Annotations moves the focus between the annotations of a failed job
	// and its log, with the key of the log, and notes with that of the
	// annotations.
	Annotations, notes key.Binding

	// List moves through the runs and the jobs, and Log through the log.
	List feed.KeyMap
	Log  logview.KeyMap
}

// The contexts of the keys of the modal: its own, and one for each pane.
const (
	ctxModal       = "actions"
	ctxRuns        = "actions_runs"
	ctxJobs        = "actions_jobs"
	ctxLog         = "actions_log"
	ctxAnnotations = "actions_annotations"
	ctxFilter      = "actions_filter"
)

func newKeyMap(keys config.Keymap) KeyMap {
	modal, jobs, log, notes := ui.In(keys, ctxModal), ui.In(keys, ctxJobs), ui.In(keys, ctxLog), ui.In(keys, ctxAnnotations)
	k := KeyMap{
		NextTab:     modal.Binding("global.next_tab", "tab"),
		PrevTab:     modal.Binding("global.prev_tab", "previous tab"),
		Left:        modal.Binding("pane_left", "pane left"),
		Right:       modal.Binding("pane_right", "pane right"),
		Select:      modal.Binding("global.select", "open"),
		Back:        modal.Binding("global.dismiss", "back"),
		Filter:      modal.Binding("filter", "filter"),
		Zoom:        modal.Binding("global.zoom", "zoom"),
		Open:        modal.Binding("global.open", "browser"),
		Refresh:     modal.Binding("global.refresh", "refresh"),
		RerunFailed: modal.Binding("rerun_failed", "rerun failed"),
		Rerun:       modal.Binding("rerun", "rerun all"),
		RerunJob:    jobs.Binding("rerun_job", "rerun job"),
		rerunLog:    log.Binding("rerun_job", "rerun job"),
		rerunNotes:  notes.Binding("rerun_job", "rerun job"),
		Cancel:      modal.Binding("cancel", "cancel run"),
		Annotations: log.Binding("annotations", "annotations"),
		notes:       notes.Binding("annotations", "annotations"),
		Confirm:     ui.DefaultConfirmKeys(),
		form:        ui.FilterFormKeys(keys),
	}
	// The modal's own keys come before those of the lists and the log,
	// such as f, which pages down there and filters here.
	k.Next = modal.Binding("global.next_pane", "pane")
	k.Prev = modal.Binding("global.prev_pane", "previous pane")
	fk := feed.DefaultKeyMap()
	fk.Retry = relabel(k.Refresh, "retry")
	fk.Retry.SetEnabled(false)
	k.List = fk

	// The log folds with enter, and closes with the back key, which
	// clears a search first.
	lk := logview.DefaultKeyMap()
	lk.Close = relabel(k.Back, "back")
	k.Log = lk
	return k
}

// own returns the keys of the modal itself, in the order it matches them.
func (k KeyMap) own() []key.Binding {
	return slices.Concat(k.screen(), k.pane(k.RerunJob))
}

// screen returns the keys that work in every pane, in the order the modal
// matches them.
func (k KeyMap) screen() []key.Binding {
	return []key.Binding{
		k.NextTab, k.PrevTab, k.Next, k.Prev, k.Right, k.Left, k.Filter, k.Zoom, k.Open,
		k.RerunFailed, k.Rerun, k.Cancel, k.Refresh, k.Back,
	}
}

// pane returns the keys of the focused pane that the modal matches, the
// re-run of a job being rerun.
func (k KeyMap) pane(rerun key.Binding) []key.Binding {
	return []key.Binding{rerun, k.Select}
}

// rerunJob returns the key that re-runs a job, of the jobs, the log or the
// annotations, whichever has the focus.
func (k KeyMap) rerunJob(m *Modal) key.Binding {
	switch {
	case m.focus == logPane && m.log.OnAnnotations():
		return k.rerunNotes
	case m.focus == logPane:
		return k.rerunLog
	}
	return k.RerunJob
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Next, k.Zoom, k.Cancel, k.RerunFailed, k.Rerun, k.RerunJob, k.Filter, k.Open}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return slices.Concat([][]key.Binding{k.own(), {k.Annotations, k.Confirm.Yes, k.Confirm.No}}, k.List.FullHelp(), k.Log.FullHelp())
}

// job returns the keys of the log pane's job view: the moves of the
// lists through its annotations, and the select key to open one.
func (k KeyMap) job() jobview.KeyMap {
	return jobview.KeyMap{
		Log: k.Log, Annotations: k.Annotations, NotesAnnotations: k.notes,
		LogContext: ctxLog, NotesContext: ctxAnnotations,
		Up: k.List.Up, Down: k.List.Down, Select: relabel(k.Select, "open file"), Open: k.Open,
	}
}

// relabel returns b described as desc.
func relabel(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}
