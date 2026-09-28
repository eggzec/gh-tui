package actions

import (
	"slices"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
)

// KeyMap holds the keys of the modal and of the bubbles it shows. The
// modal matches its own keys first, so the bubbles get only the keys it
// leaves them.
type KeyMap struct {
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
	// once the user confirms.
	RerunFailed, Rerun, RerunJob, Cancel key.Binding
	// Confirm answers the confirmation.
	Confirm ui.ConfirmKeys
	// Annotations moves the focus between the annotations of a failed job
	// and its log.
	Annotations key.Binding

	// List moves through the runs and the jobs, and Log through the log.
	List feed.KeyMap
	Log  logview.KeyMap
}

func newKeyMap(keys map[string][]string) KeyMap {
	k := KeyMap{
		NextTab:     ui.Binding(keys, config.ActionNextFilter, "tab"),
		PrevTab:     ui.Binding(keys, config.ActionPrevFilter, "previous tab"),
		Left:        ui.Binding(keys, config.ActionPaneLeft, "pane left"),
		Right:       ui.Binding(keys, config.ActionPaneRight, "pane right"),
		Select:      ui.Binding(keys, config.ActionSelect, "open"),
		Back:        ui.Binding(keys, config.ActionBack, "back"),
		Filter:      ui.Binding(keys, config.ActionFilter, "filter"),
		Zoom:        ui.Binding(keys, config.ActionZoom, "zoom"),
		Open:        ui.Binding(keys, config.ActionOpen, "browser"),
		Refresh:     ui.Binding(keys, config.ActionRefresh, "refresh"),
		RerunFailed: ui.Binding(keys, config.ActionRerunFailed, "rerun failed"),
		Rerun:       ui.Binding(keys, config.ActionRerun, "rerun all"),
		RerunJob:    ui.Binding(keys, config.ActionRerunJob, "rerun job"),
		Cancel:      ui.Binding(keys, config.ActionCancelRun, "cancel run"),
		Annotations: ui.Binding(keys, config.ActionAnnotations, "annotations"),
		Confirm:     ui.DefaultConfirmKeys(),
	}
	// The tabs take ] and [ from the panes, whose keys the app shares
	// with other screens: the modal matches the tabs first. ctrl+r, the
	// second key of refresh, re-runs the failed jobs, which the modal
	// matches before refresh. Its own keys come before those of the lists
	// and the log, such as f, which pages down there and filters here.
	k.Next = ui.Binding(keys, config.ActionNextTab, "pane")
	k.Prev = ui.Binding(keys, config.ActionPrevTab, "previous pane")
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
	return []key.Binding{
		k.NextTab, k.PrevTab, k.Next, k.Prev, k.Right, k.Left, k.Filter, k.Zoom, k.Open,
		k.RerunFailed, k.Rerun, k.RerunJob, k.Cancel, k.Refresh, k.Back, k.Select, k.Annotations,
	}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Next, k.Zoom, k.Cancel, k.RerunFailed, k.Rerun, k.RerunJob, k.Filter, k.Open}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return slices.Concat([][]key.Binding{k.own(), {k.Confirm.Yes, k.Confirm.No}}, k.List.FullHelp(), k.Log.FullHelp())
}

// job returns the keys of the log pane's job view: the moves of the
// lists through its annotations, and the select key to open one.
func (k KeyMap) job() jobview.KeyMap {
	return jobview.KeyMap{
		Log: k.Log, Annotations: k.Annotations, Up: k.List.Up, Down: k.List.Down,
		Select: relabel(k.Select, "open file"), Open: k.Open,
	}
}

// relabel returns b described as desc.
func relabel(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}
