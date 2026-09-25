package actions

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
)

// KeyMap holds the keys of the modal. The modal handles its own keys
// first, so the bubbles' keys leave out any the modal takes.
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
	// Yes and No answer the confirmation.
	Yes, No key.Binding
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
		Yes:         key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y", "yes")),
		No:          key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n", "no")),
	}
	// The tabs take their keys from the panes' keys, which the app shares
	// with other screens.
	tabs := []key.Binding{k.NextTab, k.PrevTab}
	k.Next = without(ui.Binding(keys, config.ActionNextTab, "pane"), tabs)
	k.Prev = without(ui.Binding(keys, config.ActionPrevTab, "previous pane"), tabs)
	// Refresh reads the runs again, so ctrl+r, its second key, is free for
	// the re-run of the failed jobs.
	k.Refresh = without(k.Refresh, []key.Binding{k.RerunFailed})

	own := []key.Binding{
		k.Next, k.Prev, k.Left, k.Right, k.NextTab, k.PrevTab, k.Select, k.Back, k.Filter, k.Zoom, k.Open,
		k.Refresh, k.RerunFailed, k.Rerun, k.RerunJob, k.Cancel,
	}
	fk := feed.DefaultKeyMap()
	for _, b := range []*key.Binding{&fk.Up, &fk.Down, &fk.PageUp, &fk.PageDown, &fk.Home, &fk.End} {
		*b = without(*b, own)
	}
	fk.Retry = relabel(k.Refresh, "retry")
	fk.Retry.SetEnabled(false)
	k.List = fk

	// The log folds with enter, and closes with the back key, which
	// clears a search first.
	lk := logview.DefaultKeyMap()
	logOwn := slices.DeleteFunc(slices.Clone(own), func(b key.Binding) bool {
		return slices.Equal(b.Keys(), k.Select.Keys()) || slices.Equal(b.Keys(), k.Back.Keys())
	})
	for _, b := range []*key.Binding{
		&lk.Up, &lk.Down, &lk.PageUp, &lk.PageDown, &lk.HalfPageUp, &lk.HalfPageDown, &lk.Home, &lk.End,
		&lk.Left, &lk.Right, &lk.Toggle, &lk.Expand, &lk.Collapse, &lk.ExpandAll, &lk.CollapseAll,
		&lk.NextError, &lk.PrevError, &lk.NextWarning, &lk.PrevWarning, &lk.Wrap, &lk.Times, &lk.LineNumbers,
		&lk.Follow, &lk.Search, &lk.Next, &lk.Prev,
	} {
		enabled := b.Enabled()
		*b = without(*b, logOwn)
		// The view enables the moves between errors, warnings and matches
		// only while there are some.
		b.SetEnabled(enabled && len(b.Keys()) > 0)
	}
	lk.Close = relabel(k.Back, "back")
	k.Log = lk
	return k
}

// job returns the keys of the log pane's job view: the moves of the
// lists through its annotations, and the select key to open one.
func (k KeyMap) job() jobview.KeyMap {
	return jobview.KeyMap{
		Log: k.Log, Annotations: k.Annotations, Up: k.List.Up, Down: k.List.Down,
		Select: relabel(k.Select, "open file"), Open: k.Open,
	}
}

var keyLabels = strings.NewReplacer("pgdown", "pgdn", "down", "↓", "up", "↑", "left", "←", "right", "→")

// without returns b without the keys that taken use.
func without(b key.Binding, taken []key.Binding) key.Binding {
	keys := slices.DeleteFunc(slices.Clone(b.Keys()), func(k string) bool {
		return slices.ContainsFunc(taken, func(t key.Binding) bool {
			return slices.Contains(t.Keys(), k)
		})
	})
	switch len(keys) {
	case len(b.Keys()):
		return b
	case 0:
		return key.NewBinding(key.WithDisabled())
	}
	labels := make([]string, len(keys))
	for i, k := range keys {
		labels[i] = keyLabels.Replace(k)
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(labels, "/"), b.Help().Desc))
}

// relabel returns b described as desc.
func relabel(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}
