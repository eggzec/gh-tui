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
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// The contexts of the keys of the step: the list of checks, the log of a
// job and its annotations, and the detail of what an app reported.
const (
	ctxList        = "pull_checks"
	ctxLog         = "pull_check_log"
	ctxAnnotations = "pull_check_annotations"
	ctxDetail      = "pull_check_detail"
)

// KeyMap holds the keys of the step and of the bubbles it shows. The step
// matches its own keys first, so the bubbles get only the keys it leaves
// them.
type KeyMap struct {
	// Up, Down, PageUp, PageDown, HalfPageUp, HalfPageDown, Home and End
	// move through the checks. Home and End also go to the top and bottom
	// of what an app reported.
	Up, Down, PageUp, PageDown, HalfPageUp, HalfPageDown, Home, End key.Binding
	// Top and Bottom go to the top and bottom of what an app reported:
	// Home and End, named for what they do there.
	Top, Bottom key.Binding
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
	// user confirms it, with the key of the list, and rerunLog,
	// rerunNotes and rerunDetail with those of what the step shows.
	RerunFailed                       key.Binding
	rerunLog, rerunNotes, rerunDetail key.Binding
	// Confirm answers the confirmation.
	Confirm ui.ConfirmKeys
	// Annotations moves the focus between the annotations of a failed job
	// and its log, with the key of the log and notes, that of the
	// annotations.
	Annotations, notes key.Binding

	// Log moves through the log of a job, and Detail through what an app
	// reported.
	Log    logview.KeyMap
	Detail viewport.KeyMap
}

func newKeyMap(keys config.Keymap) KeyMap {
	fk := feed.DefaultKeyMap()
	list, log, notes, detail := ui.In(keys, ctxList), ui.In(keys, ctxLog), ui.In(keys, ctxAnnotations), ui.In(keys, ctxDetail)
	k := KeyMap{
		Up: fk.Up, Down: fk.Down, PageUp: fk.PageUp, PageDown: fk.PageDown,
		HalfPageUp: fk.HalfPageUp, HalfPageDown: fk.HalfPageDown, Home: fk.Home, End: fk.End,
		Top:         relabel(fk.Home, "top"),
		Bottom:      relabel(fk.End, "bottom"),
		Select:      list.Binding("global.select", "open"),
		Back:        list.Binding("global.dismiss", "back"),
		Open:        list.Binding("global.open", "browser"),
		Refresh:     list.Binding("global.refresh", "refresh"),
		RerunFailed: list.Binding("rerun_failed", "rerun failed"),
		rerunLog:    log.Binding("rerun_failed", "rerun failed"),
		rerunNotes:  notes.Binding("rerun_failed", "rerun failed"),
		rerunDetail: detail.Binding("rerun_failed", "rerun failed"),
		Annotations: log.Binding("annotations", "annotations"),
		notes:       notes.Binding("annotations", "annotations"),
		Confirm:     ui.DefaultConfirmKeys(),
	}
	// The step matches the re-run before refresh, and its own keys
	// before those of the log and of what an app reported.
	lk := logview.DefaultKeyMap()
	lk.Close = k.Back
	lk.Close.SetHelp(k.Back.Help().Key, "back")
	k.Log = lk

	k.Detail = detailKeyMap()
	return k
}

// detailKeyMap returns the keys that scroll what an app reported, which
// page as the pager does.
func detailKeyMap() viewport.KeyMap {
	pk := pager.DefaultKeyMap()
	d := viewport.DefaultKeyMap()
	d.PageUp, d.PageDown, d.HalfPageUp, d.HalfPageDown = pk.PageUp, pk.PageDown, pk.HalfPageUp, pk.HalfPageDown
	return d
}

// Capturing reports whether the step takes every key, while a search of the
// log is open.
func (s *Step) Capturing() bool { return s.mode == jobMode && s.view.Capturing() }

// rerun returns the re-run key of what the step shows: the key of the
// list, or of the log, the annotations or the detail.
func (k KeyMap) rerun(m mode, onNotes bool) key.Binding {
	switch {
	case m == jobMode && onNotes:
		return k.rerunNotes
	case m == jobMode:
		return k.rerunLog
	case m == detailMode:
		return k.rerunDetail
	}
	return k.RerunFailed
}

// job returns the keys of the job view: the moves of the checks through
// its annotations, and the select key to open one.
func (k KeyMap) job() jobview.KeyMap {
	sel := k.Select
	sel.SetHelp(sel.Help().Key, "open file")
	return jobview.KeyMap{
		Log: k.Log, Annotations: k.Annotations, NotesAnnotations: k.notes, LogContext: ctxLog, NotesContext: ctxAnnotations,
		Up: k.Up, Down: k.Down, Select: sel, Open: k.Open,
	}
}

// relabel returns b described as desc.
func relabel(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}

// own returns the keys of the step itself, in the order it matches them,
// with the re-run key of what it shows.
func (k KeyMap) own(rerun key.Binding) []key.Binding {
	return []key.Binding{
		k.Back, rerun, k.Refresh, k.Open,
		k.Select, k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown, k.Home, k.End,
	}
}

// shortHelp returns the keys of the step worth a hint.
func (k KeyMap) shortHelp(rerun key.Binding) []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Select, k.Back, rerun, k.Open}
}

// detail returns the keys that move through what an app reported.
func (k KeyMap) detail() []key.Binding {
	d := k.Detail
	return []key.Binding{d.Up, d.Down, d.PageUp, d.PageDown, d.HalfPageUp, d.HalfPageDown,
		k.Top, k.Bottom, d.Left, d.Right}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding { return k.shortHelp(k.RerunFailed) }

// FullHelp implements help.KeyMap: every key of the step and the bubbles
// it shows, once each.
func (k KeyMap) FullHelp() [][]key.Binding {
	return slices.Concat([][]key.Binding{
		k.own(k.RerunFailed), {k.Annotations},
		{k.Confirm.Yes, k.Confirm.No}, k.detail(),
	}, k.Log.FullHelp())
}

// KeyLayers implements ui.Keyed: the answer to the question while it is
// open, a search of the log while it takes every key, or else the keys of
// the step with those of the checks, of the job or of what an app
// reported, whichever shows, as the keys of the context of that.
func (s *Step) KeyLayers() []keyhelp.Layer {
	k := s.keys
	if s.ask != nil {
		return []keyhelp.Layer{k.Confirm.Layer()}
	}
	if s.mode == jobMode && s.view.Capturing() {
		return s.view.KeyLayers()
	}
	k = k.state(s)
	rerun := k.RerunFailed
	own := keyhelp.Layer{Bindings: k.own(rerun), Short: k.shortHelp(rerun)}
	switch s.mode {
	case jobMode:
		l := s.view.Layer()
		return []keyhelp.Layer{ui.MergeLayers(l.Context, own, l)}
	case detailMode:
		det := keyhelp.Layer{Bindings: k.detail(), Short: k.detail()[:2]}
		return []keyhelp.Layer{ui.MergeLayers(ctxDetail, own, det)}
	case listMode:
	}
	return []keyhelp.Layer{ui.MergeLayers(ctxList, own)}
}

// state returns k as the step takes it in its mode, named for what the
// keys do there: the list's keys only on the list, and the annotations
// key, which the job view handles, never.
func (k KeyMap) state(s *Step) KeyMap {
	k.RerunFailed = s.rerunKey()
	// A key that re-run shares with refresh says why it can't re-run
	// rather than refresh.
	k.Refresh = ui.Yield(k.Refresh, k.RerunFailed)
	if s.mode != listMode {
		k.Back = relabel(k.Back, "checks")
		// The back key clears the search of the log first.
		k.Back.SetEnabled(k.Back.Enabled() && (s.mode != jobMode || s.view.Query() == ""))
		for _, b := range []*key.Binding{&k.Select, &k.Up, &k.Down, &k.PageUp, &k.PageDown, &k.HalfPageUp, &k.HalfPageDown} {
			b.SetEnabled(false)
		}
		// Home and End go to the top and bottom of what an app reported;
		// they are listed with its keys, and the log has keys of its own.
		k.Home.SetEnabled(false)
		k.End.SetEnabled(false)
		k.Top.SetEnabled(k.Top.Enabled() && s.mode == detailMode)
		k.Bottom.SetEnabled(k.Bottom.Enabled() && s.mode == detailMode)
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
	b := s.keys.rerun(s.mode, s.view.OnAnnotations())
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
