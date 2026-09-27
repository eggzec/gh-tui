package actions

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// helpKeys lists the keys of the focused pane, named for what they do
// there, and those of the filter or the confirmation while one is open.
type helpKeys struct {
	m *Modal
}

// ShortHelp returns the bindings for the short help view.
func (h helpKeys) ShortHelp() []key.Binding {
	m, k := h.m, h.m.keys
	switch {
	case m.ask != nil:
		return k.Confirm.ShortHelp()
	case m.filterStep != nil:
		if f := m.filterStep.form; f != nil {
			return f.ShortHelp()
		}
		return []key.Binding{relabel(k.Back, "back")}
	case m.focus == logPane && m.log.Capturing():
		return m.log.ShortHelp()
	}
	var short []key.Binding
	if m.focus == logPane && (m.log.State() == jobview.Ready || m.log.OnAnnotations()) {
		short = m.log.Keys()
	} else {
		short = []key.Binding{h.drill()}
	}
	short = append(short, k.Next, k.Zoom)
	return append(short, h.changes()...)
}

// FullHelp returns the bindings for the full help view.
func (h helpKeys) FullHelp() [][]key.Binding {
	m, k := h.m, h.m.keys
	switch {
	case m.ask != nil:
		return k.Confirm.FullHelp()
	case m.filterStep != nil:
		if f := m.filterStep.form; f != nil {
			return f.FullHelp()
		}
		return [][]key.Binding{{relabel(k.Back, "back")}}
	case m.focus == logPane && m.log.State() == jobview.Ready:
		return append(m.log.FullHelp(), h.changes(), h.moves())
	}
	l := k.List
	return [][]key.Binding{
		{l.Up, l.Down, l.PageUp, l.PageDown, l.Home, l.End},
		{h.drill(), k.NextTab, k.PrevTab, k.Filter, k.Open, k.Refresh},
		h.changes(),
		h.moves(),
	}
}

// drill is the select key, named for the pane it opens.
func (h helpKeys) drill() key.Binding {
	switch h.m.focus {
	case runsPane:
		return relabel(h.m.keys.Select, "jobs")
	case jobsPane:
		return relabel(h.m.keys.Select, "log")
	case logPane:
	}
	return key.NewBinding(key.WithDisabled())
}

// changes are the keys that change the run: a re-run of its failed jobs
// or of all of them once it completed, of the job under the cursor away
// from the runs, or a cancel while it runs.
func (h helpKeys) changes() []key.Binding {
	m, k := h.m, h.m.keys
	if !m.hasRun {
		return nil
	}
	g := m.gate()
	if !m.run.Done() {
		return []key.Binding{g.Gated(k.Cancel, ui.ActCancelRun, nil), k.Filter, k.Open}
	}
	out := []key.Binding{g.Gated(k.RerunFailed, ui.ActRerun, nil), g.Gated(k.Rerun, ui.ActRerun, nil)}
	if m.focus != runsPane {
		out = append(out, g.Gated(k.RerunJob, ui.ActRerun, nil))
	}
	return append(out, k.Filter, k.Open)
}

// moves are the keys that move between the panes and the tabs, and step
// back.
func (h helpKeys) moves() []key.Binding {
	k := h.m.keys
	back := k.Back
	if h.m.focus == runsPane && !h.m.zoom {
		back = relabel(back, "close")
	}
	return []key.Binding{k.Next, k.Prev, k.Left, k.Right, k.NextTab, k.PrevTab, k.Zoom, back}
}
