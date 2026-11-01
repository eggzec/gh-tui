package actions

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// KeyMap holds the keys of the modal and of the bubbles it shows. The
// modal matches its own keys first, so the bubbles get only the keys it
// leaves them.
type KeyMap struct {
	// form holds the keys of the filter form, as the other filters have them.
	form filterform.KeyMap
	// Next and Prev move the focus through the panes, and Panes focus the
	// pane with that number.
	Next, Prev key.Binding
	Panes      [numPanes]key.Binding
	// Zoom shows the focused box alone, and the boxes again.
	Zoom key.Binding
	// NextTab and PrevTab switch between All, Failing, Running and Mine.
	NextTab, PrevTab key.Binding
	// Select drills into the pane after the focused one.
	Select key.Binding
	// Back steps back a pane, and does nothing on the runs. Dismiss clears
	// what is transient, and then closes the modal; it closes the filter
	// before the form shows, too.
	Back, Dismiss key.Binding
	// Filter edits the filter of the runs, in a step of the modal, and
	// ClearFilter clears it. ToggleFilter switches it off and on. All work
	// with the focus on the runs.
	Filter, ClearFilter, ToggleFilter key.Binding
	// GroupBy and Sort open the choice of the levels the runs are grouped
	// and sorted by, and ToggleGroups and ToggleSort switch them off and
	// on. All work with the focus on the runs.
	GroupBy, ToggleGroups, Sort, ToggleSort key.Binding
	// order are the keys of the picker of the levels.
	order orderpick.KeyMap
	// ViewFile previews the workflow file of the run, at the commit it ran on.
	ViewFile key.Binding
	// Refresh reads the runs again, or what failed to load.
	Refresh key.Binding
	// RerunFailed, Rerun, RerunJob and Cancel change the selected run,
	// once the user confirms.
	RerunFailed, Rerun, RerunJob, Cancel key.Binding
	// Confirm answers the confirmation.
	Confirm confirm.KeyMap

	// Jump holds the keys of Panes, which it stands for in help.
	Jump key.Binding

	// Runs and Jobs move through the runs and the jobs, and Log through the
	// log. notesUp and notesDown move through the annotations.
	Runs feed.KeyMap
	// search are the keys of the prompt of the runs' find and filter.
	search             cmdline.KeyMap
	Log                logview.KeyMap
	notesUp, notesDown key.Binding
}

// The contexts of the keys of the modal: its own, and one for each pane.
const (
	ctxModal = "actions"
	ctxRuns  = "actions_runs"
	ctxLog   = "actions_log"
)

func newKeyMap(keys config.Keymap) KeyMap {
	modal, runs, log := ui.In(keys, ctxModal), ui.In(keys, ctxRuns), ui.In(keys, ctxLog)
	k := KeyMap{
		NextTab:      modal.Binding("global.next_tab", "tab"),
		PrevTab:      modal.Binding("global.prev_tab", "previous tab"),
		Select:       modal.Binding("global.select", "open"),
		Back:         modal.Binding("global.back", "back"),
		Dismiss:      modal.Binding("global.dismiss", "close"),
		Filter:       runs.Binding("filter", "filter"),
		ClearFilter:  runs.Binding("clear_filter", "clear filter"),
		ToggleFilter: runs.Binding("filter_toggle", "filter on/off"),
		GroupBy:      runs.Binding("group_by", "group by"),
		ToggleGroups: runs.Binding("group_toggle", "group on/off"),
		Sort:         runs.Binding("sort", "sort"),
		ToggleSort:   runs.Binding("sort_toggle", "sort on/off"),
		order:        ui.OrderPickKeys(keys),
		ViewFile:     modal.Binding("view_workflow", "view workflow file"),
		Refresh:      modal.Binding("global.refresh", "refresh"),
		RerunFailed:  modal.Binding("rerun_failed", "rerun failed"),
		Rerun:        modal.Binding("rerun", "rerun all"),
		RerunJob:     modal.Binding("rerun_job", "rerun job"),
		Zoom:         modal.Binding("global.zoom", "zoom"),
		Cancel:       modal.Binding("cancel", "cancel run"),
		Confirm:      confirm.NewKeyMap(ui.Lookup(keys, "confirm")),
		form:         ui.FilterFormKeys(keys, "filter"),
		// The modal's own keys come before those of the lists and the log.
		Next: modal.Binding("global.next_pane", "pane"),
		Prev: modal.Binding("global.prev_pane", "previous pane"),
	}
	for i, a := range [numPanes]string{"global.pane_1", "global.pane_2"} {
		k.Panes[i] = modal.Binding(a, paneTitles[i])
	}
	k.Jump = ui.Jump(k.Panes[:]...)
	k.notesUp, k.notesDown = log.Binding("up", "up"), log.Binding("down", "down")
	k.Runs = feed.NewKeyMap(runs)
	k.search = ui.SearchPromptKeys(keys)

	// The log folds with enter. The modal takes the keys that close it, so
	// the log has none of its own.
	lk := logview.NewKeyMap(log)
	lk.Quit.SetEnabled(false)
	lk.Dismiss.SetEnabled(false)
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
		k.NextTab, k.PrevTab, k.Next, k.Prev, k.Jump, k.Zoom, k.ViewFile,
		k.RerunFailed, k.Rerun, k.Cancel, k.Refresh, k.Dismiss, k.Back,
	}
}

// pane returns the keys of the focused pane that the modal matches, the
// re-run of a job being rerun.
func (k KeyMap) pane(rerun key.Binding) []key.Binding {
	return []key.Binding{rerun, k.Select}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Next, k.Cancel, k.RerunFailed, k.Rerun, k.RerunJob}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return slices.Concat([][]key.Binding{k.own(), {k.Filter, k.ClearFilter, k.ToggleFilter, k.GroupBy, k.ToggleGroups, k.Sort, k.ToggleSort, k.Confirm.Yes, k.Confirm.No, k.Confirm.Default}}, k.Runs.FullHelp(), k.Log.FullHelp())
}

// relabel returns b described as desc.
func relabel(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}

// job returns the keys of the log overlay's job view.
func (k KeyMap) job() jobview.KeyMap {
	return jobview.KeyMap{Log: k.Log, LogContext: ctxLog}
}

// paneTitles names the panes in help.
var paneTitles = [numPanes]string{"runs", "annotations"}

// focusOf returns the pane that msg focuses, or -1.
func (k KeyMap) focusOf(msg tea.KeyPressMsg) pane {
	for i, b := range k.Panes {
		if keymap.Matches(msg, b) {
			return pane(i)
		}
	}
	return -1
}
