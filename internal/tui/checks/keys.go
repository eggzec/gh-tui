package checks

import (
	"slices"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
)

// KeyMap holds the keys of the step. The step handles its own keys first,
// so the bubbles' keys leave out any it takes.
type KeyMap struct {
	// Up, Down, PageUp, PageDown, Home and End move through the checks.
	Up, Down, PageUp, PageDown, Home, End key.Binding
	// Select opens the check under the cursor: its job, or what its app
	// reported.
	Select key.Binding
	// Back steps back to the checks, and from them closes the step.
	Back key.Binding
	// Open opens the check on GitHub, or where its app points.
	Open key.Binding
	// Refresh reads the checks again, or what failed to load.
	Refresh key.Binding
	// RerunFailed re-runs the failed jobs of the run of the check, once the
	// user confirms it.
	RerunFailed key.Binding
	// Confirm answers the confirmation.
	Confirm ui.ConfirmKeys
	// Annotations moves the focus between the annotations of a failed job
	// and its log.
	Annotations key.Binding

	// Log moves through the log of a job, and Detail through what an app
	// reported.
	Log    logview.KeyMap
	Detail viewport.KeyMap
}

func newKeyMap(keys map[string][]string) KeyMap {
	fk := feed.DefaultKeyMap()
	k := KeyMap{
		Up: fk.Up, Down: fk.Down, PageUp: fk.PageUp, PageDown: fk.PageDown, Home: fk.Home, End: fk.End,
		Select:      ui.Binding(keys, config.ActionSelect, "open"),
		Back:        ui.Binding(keys, config.ActionBack, "back"),
		Open:        ui.Binding(keys, config.ActionOpen, "browser"),
		Refresh:     ui.Binding(keys, config.ActionRefresh, "refresh"),
		RerunFailed: ui.Binding(keys, config.ActionRerunFailed, "rerun failed"),
		Annotations: ui.Binding(keys, config.ActionAnnotations, "annotations"),
		Confirm:     ui.DefaultConfirmKeys(),
	}
	// ctrl+r, the second key of refresh, re-runs the failed jobs here.
	// PR5: this decides where ctrl+r goes, since the step matches refresh
	// first; match the re-run first instead of dropping the key.
	k.Refresh = ui.FreeKeys(k.Refresh, k.RerunFailed)
	own := []key.Binding{k.Select, k.Back, k.Open, k.Refresh, k.RerunFailed, k.Annotations}
	// PR5: the step matches its own keys first, so dropping them here and
	// below only keeps the collisions out of help.
	for _, b := range []*key.Binding{&k.Up, &k.Down, &k.PageUp, &k.PageDown, &k.Home, &k.End} {
		*b = ui.FreeKeys(*b, own...)
	}

	// The log folds with enter and closes with the back key, which clears
	// its search first, so both stay the log's.
	lk := logview.DefaultKeyMap()
	logOwn := []key.Binding{k.Open, k.Refresh, k.RerunFailed, k.Annotations}
	for _, b := range []*key.Binding{
		&lk.Up, &lk.Down, &lk.PageUp, &lk.PageDown, &lk.HalfPageUp, &lk.HalfPageDown, &lk.Home, &lk.End,
		&lk.Left, &lk.Right, &lk.Toggle, &lk.Expand, &lk.Collapse, &lk.FoldAll,
		&lk.NextError, &lk.PrevError, &lk.NextWarning, &lk.PrevWarning, &lk.Wrap, &lk.Times, &lk.LineNumbers,
		&lk.Follow, &lk.Search, &lk.Next, &lk.Prev,
	} {
		*b = ui.FreeKeys(*b, logOwn...)
	}
	lk.Close = k.Back
	lk.Close.SetHelp(k.Back.Help().Key, "back")
	k.Log = lk

	dk := viewport.DefaultKeyMap()
	for _, b := range []*key.Binding{&dk.PageDown, &dk.PageUp, &dk.HalfPageUp, &dk.HalfPageDown, &dk.Up, &dk.Down, &dk.Left, &dk.Right} {
		*b = ui.FreeKeys(*b, own...)
	}
	k.Detail = dk
	return k
}

// job returns the keys of the job view: the moves of the checks through
// its annotations, and the select key to open one.
func (k KeyMap) job() jobview.KeyMap {
	sel := k.Select
	sel.SetHelp(sel.Help().Key, "open file")
	return jobview.KeyMap{Log: k.Log, Annotations: k.Annotations, Up: k.Up, Down: k.Down, Select: sel, Open: k.Open}
}

// relabel returns b described as desc.
func relabel(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}

// own returns the keys of the step itself, in the order it matches them.
func (k KeyMap) own() []key.Binding {
	return []key.Binding{
		k.Back, k.Refresh, k.Open, k.RerunFailed,
		k.Select, k.Up, k.Down, k.PageUp, k.PageDown, k.Home, k.End, k.Annotations,
	}
}

// detail returns the keys that move through what an app reported.
func (k KeyMap) detail() []key.Binding {
	d := k.Detail
	return []key.Binding{d.Up, d.Down, d.PageUp, d.PageDown, d.HalfPageUp, d.HalfPageDown, d.Left, d.Right}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Select, k.Back, k.RerunFailed, k.Open}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return slices.Concat([][]key.Binding{k.own(), {k.Confirm.Yes, k.Confirm.No}, k.detail()}, k.Log.FullHelp())
}

// KeyLayers implements ui.Keyed: the answer to the question while it is
// open, or else the keys of the step, and then those of the job or of
// what an app reported, whichever shows.
func (s *Step) KeyLayers() []keyhelp.Layer {
	k := s.keys
	if s.ask != nil {
		return []keyhelp.Layer{k.Confirm.Layer()}
	}
	if s.mode == jobMode && s.view.Capturing() {
		return s.view.KeyLayers()
	}
	k = k.state(s)
	own := keyhelp.Layer{Source: "checks", Bindings: k.own(), Short: k.ShortHelp()}
	switch s.mode {
	case jobMode:
		return append([]keyhelp.Layer{own}, s.view.KeyLayers()...)
	case detailMode:
		return []keyhelp.Layer{own, {Source: "detail", Bindings: k.detail(), Short: k.detail()[:2]}}
	case listMode:
	}
	return []keyhelp.Layer{own}
}

// state returns k as the step takes it in its mode, named for what the
// keys do there: the list's keys only on the list, and the annotations
// key, which the job view handles, never.
func (k KeyMap) state(s *Step) KeyMap {
	k.RerunFailed = s.rerunKey()
	k.Annotations.SetEnabled(false)
	if s.mode != listMode {
		k.Back = relabel(k.Back, "checks")
		// The back key clears the search of the log first.
		k.Back.SetEnabled(k.Back.Enabled() && (s.mode != jobMode || s.view.Query() == ""))
		for _, b := range []*key.Binding{&k.Select, &k.Up, &k.Down, &k.PageUp, &k.PageDown, &k.Home, &k.End} {
			b.SetEnabled(false)
		}
		return k
	}
	k.Back = relabel(k.Back, "detail")
	k.Select = relabel(k.Select, "open")
	if r, ok := s.selected(); ok && r.job() {
		k.Select = relabel(k.Select, "log")
	}
	return k
}

// rerunKey is the re-run key, enabled while the check shown, or the one
// under the cursor, is of a run of GitHub Actions, and the viewer may
// re-run it.
func (s *Step) rerunKey() key.Binding {
	b := s.keys.RerunFailed
	r, ok := s.current()
	b.SetEnabled(b.Enabled() && ok && r.job())
	return s.gate().Gated(b, ui.ActRerun, nil)
}

// gate decides what the viewer may do in the repository.
func (s *Step) gate() ui.Gate {
	g := ui.Gate{Repo: s.q.Repo, Caps: s.opts.caps}
	if s.opts.voice != nil {
		g.Token = s.opts.voice.Token
	}
	return g
}
