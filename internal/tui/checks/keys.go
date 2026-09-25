package checks

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
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
	// user confirms with Yes.
	RerunFailed key.Binding
	Yes, No     key.Binding
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
		Yes:         key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y", "yes")),
		No:          key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n", "no")),
	}
	// ctrl+r, the second key of refresh, re-runs the failed jobs here.
	k.Refresh = ui.FreeKeys(k.Refresh, k.RerunFailed)
	own := []key.Binding{k.Select, k.Back, k.Open, k.Refresh, k.RerunFailed, k.Annotations}
	for _, b := range []*key.Binding{&k.Up, &k.Down, &k.PageUp, &k.PageDown, &k.Home, &k.End} {
		*b = ui.FreeKeys(*b, own...)
	}

	// The log folds with enter and closes with the back key, which clears
	// its search first, so both stay the log's.
	lk := logview.DefaultKeyMap()
	logOwn := []key.Binding{k.Open, k.Refresh, k.RerunFailed, k.Annotations}
	for _, b := range []*key.Binding{
		&lk.Up, &lk.Down, &lk.PageUp, &lk.PageDown, &lk.HalfPageUp, &lk.HalfPageDown, &lk.Home, &lk.End,
		&lk.Left, &lk.Right, &lk.Toggle, &lk.Expand, &lk.Collapse, &lk.ExpandAll, &lk.CollapseAll,
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

// keyHelp is a help.KeyMap made of fixed lists.
type keyHelp struct {
	short []key.Binding
	full  [][]key.Binding
}

func (h keyHelp) ShortHelp() []key.Binding  { return h.short }
func (h keyHelp) FullHelp() [][]key.Binding { return h.full }

// Help lists the keys of what the step shows, named for what they do
// there.
func (s *Step) Help() help.KeyMap {
	k := s.keys
	if s.ask != nil {
		return keyHelp{short: []key.Binding{k.Yes, k.No}, full: [][]key.Binding{{k.Yes, k.No}}}
	}
	back := relabel(k.Back, "checks")
	rerun := s.rerunKey()
	switch s.mode {
	case jobMode:
		if s.view.Capturing() {
			return keyHelp{short: s.view.ShortHelp(), full: [][]key.Binding{s.view.ShortHelp()}}
		}
		short := append(s.view.Keys(), back, rerun, k.Open)
		return keyHelp{short: short, full: append(s.view.FullHelp(), []key.Binding{back, rerun, k.Open, k.Refresh})}
	case detailMode:
		d := k.Detail
		short := []key.Binding{d.Up, d.Down, back, k.Open}
		return keyHelp{short: short, full: [][]key.Binding{{d.Up, d.Down, d.PageUp, d.PageDown, d.HalfPageUp, d.HalfPageDown}, {back, k.Open}}}
	case listMode:
	}
	sel := relabel(k.Select, "open")
	if r, ok := s.selected(); ok && r.job() {
		sel = relabel(k.Select, "log")
	}
	leave := relabel(k.Back, "detail")
	return keyHelp{
		short: []key.Binding{k.Up, k.Down, sel, leave, rerun, k.Open},
		full: [][]key.Binding{
			{k.Up, k.Down, k.PageUp, k.PageDown, k.Home, k.End},
			{sel, leave, k.Refresh, rerun, k.Open},
		},
	}
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
	return ui.Gate{Repo: s.q.Repo, Caps: s.opts.caps}
}
