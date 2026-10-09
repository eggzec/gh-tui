package checks

import (
	"slices"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
)

// The contexts of the keys of the step, which are panes of the modal of the
// pull request: the list of checks, the log of a job and its annotations,
// and the detail of what an app reported.
const (
	ctxList        = "pull_check_list"
	ctxLog         = "pull_check_log"
	ctxAnnotations = "pull_check_annotations"
	ctxDetail      = "pull_check_detail"
)

// KeyMap holds the keys of the step and of the bubbles it shows. The step
// matches its own keys first, so the bubbles get only the keys it leaves
// them.
type KeyMap struct {
	// Up, Down, PageUp, PageDown, HalfPageUp, HalfPageDown, Home and End
	// move through the checks.
	Up, Down, PageUp, PageDown, HalfPageUp, HalfPageDown, Home, End key.Binding
	// Top and Bottom go to the top and bottom of what an app reported.
	// They have their own keys, those of its context.
	Top, Bottom key.Binding
	// Select opens the check under the cursor: its job, or what its app
	// reported.
	Select key.Binding
	// Back steps back from a job or a detail to the checks, and does
	// nothing on them. Dismiss clears the search of a log, and then asks
	// the modal to close.
	Back, Dismiss key.Binding
	// Open opens the check on GitHub, or where its app points.
	Open key.Binding
	// Refresh reads the checks again, or what failed to load.
	Refresh key.Binding
	// RerunList, RerunLog and RerunNotes re-run the failed
	// jobs of the run of the check, once the user confirms it. Each pane
	// has its own key for it, which the user may unbind in one.
	RerunList, RerunLog, RerunNotes key.Binding
	// Confirm answers the confirmation.
	Confirm ui.ConfirmKeys
	// Annotations moves the focus between the annotations of a failed job
	// and its log, with the key of the log and notes, that of the
	// annotations.
	Annotations, notes key.Binding
	// notesUp and notesDown move through the annotations of a job.
	notesUp, notesDown key.Binding

	// Log moves through the log of a job, and Detail through what an app
	// reported.
	Log    logview.KeyMap
	Detail viewport.KeyMap
}

func newKeyMap(keys config.Keymap) KeyMap {
	list, log, notes := ui.In(keys, ctxList), ui.In(keys, ctxLog), ui.In(keys, ctxAnnotations)
	fk := ui.NewMoveKeys(list)
	var dk detailKeys
	keymap.Fill(&dk, ui.In(keys, ctxDetail))
	k := KeyMap{
		Up: fk.Up, Down: fk.Down, PageUp: fk.PageUp, PageDown: fk.PageDown,
		HalfPageUp: fk.HalfPageUp, HalfPageDown: fk.HalfPageDown, Home: fk.Home, End: fk.End,
		Top:         dk.Top,
		Bottom:      dk.Bottom,
		Select:      list.Binding("global.select", "open"),
		Back:        list.Binding("global.back", "checks"),
		Dismiss:     list.Binding("global.dismiss", "close"),
		Open:        list.Binding("global.open", "browser"),
		Refresh:     list.Binding("global.refresh", "refresh"),
		RerunList:   list.Binding("rerun_failed", "rerun failed"),
		RerunLog:    log.Binding("rerun_failed", "rerun failed"),
		RerunNotes:  notes.Binding("rerun_failed", "rerun failed"),
		Annotations: log.Binding("annotations", "annotations"),
		notes:       notes.Binding("annotations", "annotations"),
		notesUp:     notes.Binding("up", "up"),
		notesDown:   notes.Binding("down", "down"),
		Confirm:     ui.NewConfirmKeys(keys),
	}
	// The step matches the re-run before refresh, and its own keys
	// before those of the log and of what an app reported. The step takes
	// the dismiss key before the log does, so that it closes the modal
	// rather than the log.
	lk := logview.NewKeyMap(log)
	lk.Quit, lk.Dismiss = key.NewBinding(key.WithDisabled()), k.Dismiss
	k.Log = lk

	k.Detail = dk.viewport()
	return k
}

// detailKeys are the keys that scroll what an app reported, from the
// context of its detail. They are filled there, and the viewport takes
// the ones it has.
type detailKeys struct {
	Up           key.Binding `keymap:"up" help:"up"`
	Down         key.Binding `keymap:"down" help:"down"`
	PageUp       key.Binding `keymap:"page_up" help:"page up"`
	PageDown     key.Binding `keymap:"page_down" help:"page down"`
	HalfPageUp   key.Binding `keymap:"half_page_up" help:"½ page up"`
	HalfPageDown key.Binding `keymap:"half_page_down" help:"½ page down"`
	Top          key.Binding `keymap:"top" help:"top"`
	Bottom       key.Binding `keymap:"bottom" help:"bottom"`
	Left         key.Binding `keymap:"left" help:"move left"`
	Right        key.Binding `keymap:"right" help:"move right"`
}

// viewport returns the keys that the viewport of the detail handles.
func (d detailKeys) viewport() viewport.KeyMap {
	return viewport.KeyMap{
		Up: d.Up, Down: d.Down, Left: d.Left, Right: d.Right,
		PageUp: d.PageUp, PageDown: d.PageDown, HalfPageUp: d.HalfPageUp, HalfPageDown: d.HalfPageDown,
	}
}

// job returns the keys of the job view: the moves of the checks through
// its annotations, and the select key to open one.
func (k KeyMap) job() jobview.KeyMap {
	sel := k.Select
	sel.SetHelp(sel.Help().Key, "open file")
	return jobview.KeyMap{
		Log: k.Log, Annotations: k.Annotations, NotesAnnotations: k.notes, LogContext: ctxLog, NotesContext: ctxAnnotations,
		Up: k.notesUp, Down: k.notesDown, Select: sel, Open: k.Open,
	}
}

// relabel returns b described as desc.
func relabel(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}

// shared returns the keys that work in each pane of the step, in the order
// it matches them, with rerun the re-run key of the pane that shows, and
// pane those of the list of checks.
func (k KeyMap) shared(rerun key.Binding) []key.Binding {
	if rerun.Help().Desc == "" {
		return []key.Binding{k.Back, k.Dismiss, k.Refresh, k.Open}
	}
	return []key.Binding{k.Back, k.Dismiss, rerun, k.Refresh, k.Open}
}

func (k KeyMap) pane() []key.Binding {
	return []key.Binding{k.Select, k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown, k.Home, k.End}
}

// detail returns the keys that move through what an app reported.
func (k KeyMap) detail() []key.Binding {
	d := k.Detail
	return []key.Binding{d.Up, d.Down, d.PageUp, d.PageDown, d.HalfPageUp, d.HalfPageDown,
		k.Top, k.Bottom, d.Left, d.Right}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Select, k.Back, k.Dismiss, k.RerunList, k.Open}
}

// FullHelp implements help.KeyMap: every key of the step and the bubbles
// it shows, once each.
func (k KeyMap) FullHelp() [][]key.Binding {
	return slices.Concat([][]key.Binding{
		k.shared(k.RerunList), k.pane(), {k.Annotations, k.RerunLog, k.RerunNotes},
		{k.Confirm.Method, k.Confirm.Yes, k.Confirm.No}, k.detail(),
	}, k.Log.FullHelp())
}

// TakesKeys reports whether the step takes every key while it shows: a
// question waits for its answer, or the log types a search or the name of
// an option. The modal the step is in leaves its own keys to the step then.
func (s *Step) TakesKeys() bool {
	return s.ask != nil || s.mode == jobMode && s.view.Capturing()
}

// KeyLayers implements ui.Keyed: the answer to the question while it is
// open, a search of the log while it takes every key, or else the keys of
// the pane that shows, the list of checks, the job or what an app
// reported, as the keys of the context of that. The keys of the modal the
// step is in come before them, which that modal lists.
func (s *Step) KeyLayers() []keyhelp.Layer {
	k := s.keys
	if s.ask != nil {
		return []keyhelp.Layer{k.Confirm.Layer()}
	}
	if s.mode == jobMode && s.view.Capturing() {
		return s.view.KeyLayers()
	}
	k = k.state(s)
	shared := k.shared(k.paneRerun(s))
	short := []key.Binding{k.Back, k.Dismiss, k.paneRerun(s), k.Open}
	switch s.mode {
	case jobMode:
		l := s.view.Layer()
		l.Bindings = slices.Concat(shared, l.Bindings)
		l.Short = slices.Concat(l.Short, short)
		return []keyhelp.Layer{l}
	case detailMode:
		det := keyhelp.Layer{Bindings: k.detail(), Short: k.detail()[:2]}
		return []keyhelp.Layer{ui.MergeLayers(ctxDetail, keyhelp.Layer{Bindings: shared, Short: short}, det)}
	case listMode:
	}
	list := keyhelp.Layer{Bindings: k.pane(), Short: []key.Binding{k.Up, k.Down, k.Select}}
	return []keyhelp.Layer{ui.MergeLayers(ctxList, keyhelp.Layer{Bindings: shared, Short: short}, list)}
}

// paneRerun returns the re-run key of the pane that shows.
func (k KeyMap) paneRerun(s *Step) key.Binding {
	switch {
	case s.mode == detailMode:
		// What an app reported is never a job to re-run.
		return key.Binding{}
	case s.mode == jobMode && s.view.OnAnnotations():
		return k.RerunNotes
	case s.mode == jobMode:
		return k.RerunLog
	}
	return k.RerunList
}

// state returns k as the step takes it in its mode, named for what the
// keys do there: the list's keys only on the list, and the annotations
// key, which the job view handles, never.
func (k KeyMap) state(s *Step) KeyMap {
	rerun := s.rerunKey()
	k.RerunList, k.RerunLog, k.RerunNotes = rerun, rerun, rerun
	// A key that re-run shares with refresh says why it can't re-run
	// rather than refresh.
	k.Refresh = ui.Yield(k.Refresh, rerun)
	// The dismiss key clears the search of the log first.
	if s.mode == jobMode && s.view.Query() != "" {
		k.Dismiss = relabel(k.Dismiss, "clear search")
	}
	// The back key steps out of a job or a detail, and does nothing on the
	// list.
	k.Back.SetEnabled(k.Back.Enabled() && s.mode != listMode)
	if s.mode != listMode {
		for _, b := range []*key.Binding{&k.Select, &k.Up, &k.Down, &k.PageUp, &k.PageDown, &k.HalfPageUp, &k.HalfPageDown} {
			b.SetEnabled(false)
		}
		// Home and End move through the checks only; Top and Bottom, which
		// are listed with the keys of what an app reported, go to its top
		// and bottom, and the log has keys of its own.
		k.Home.SetEnabled(false)
		k.End.SetEnabled(false)
		k.Top.SetEnabled(k.Top.Enabled() && s.mode == detailMode)
		k.Bottom.SetEnabled(k.Bottom.Enabled() && s.mode == detailMode)
		return k
	}
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
	b := s.keys.paneRerun(s)
	if s.mode == detailMode {
		return b
	}
	r, ok := s.current()
	b.SetEnabled(b.Enabled() && ok && r.job())
	return s.gate().Gated(b, ui.ActRerun, nil)
}

// gate decides what the viewer may do in the repository.
func (s *Step) gate() ui.Gate {
	g := ui.Gate{Repo: s.q.Repo, Caps: s.opts.caps, Icons: s.opts.icons}
	if s.opts.voice != nil {
		g.Token = s.opts.voice.Token
	}
	return g
}
