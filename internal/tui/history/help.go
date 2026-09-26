package history

import "charm.land/bubbles/v2/key"

// helpKeys lists the keys of the focused pane, named for what they do
// there.
type helpKeys struct {
	m *Modal
}

// ShortHelp returns the bindings for the short help view.
func (h helpKeys) ShortHelp() []key.Binding {
	m, k := h.m, h.m.keys
	switch {
	case m.focus == branchPane && m.branches.filter != nil:
		return m.branches.filter.ShortHelp()
	case m.focus == commitPane && m.commit.patch:
		if m.commit.pager.Capturing() {
			return m.commit.pager.ShortHelp()
		}
		pk := m.commit.pager.KeyMap()
		return []key.Binding{pk.Search, pk.Next, pk.Prev, k.Open, k.Next, h.zoom(), h.back()}
	}
	return append(h.actions(), k.Next, h.zoom(), h.back())
}

// FullHelp returns the bindings for the full help view.
func (h helpKeys) FullHelp() [][]key.Binding {
	m, k := h.m, h.m.keys
	switch {
	case m.focus == branchPane && m.branches.filter != nil:
		return m.branches.filter.FullHelp()
	case m.focus == commitPane && m.commit.patch:
		return append(m.commit.pager.FullHelp(), []key.Binding{k.Open, k.ResetBase, k.Next, k.Prev, h.zoom(), h.back()})
	}
	moves := k.List
	if m.focus == graphPane {
		moves = k.Graph
	}
	actions := h.actions()
	if m.base.Ref != "" {
		actions = append(actions, k.ResetBase)
	}
	return [][]key.Binding{
		{moves.Up, moves.Down, moves.PageUp, moves.PageDown, moves.Home, moves.End},
		append(actions, k.Retry),
		{k.Next, k.Prev, h.zoom(), h.back()},
	}
}

// actions are what the focused pane does with its keys.
func (h helpKeys) actions() []key.Binding {
	m, k := h.m, h.m.keys
	switch m.focus {
	case branchPane:
		return []key.Binding{named(k.Select, "graph"), k.Filter, k.UseAsBase, k.Open}
	case graphPane:
		return []key.Binding{named(k.Select, "diff"), k.UseAsBase, k.Open}
	case commitPane:
	}
	return []key.Binding{named(k.Select, "patch"), k.UseAsBase, k.Open}
}

// zoom is the zoom key, left out where the modal shows one pane anyway.
func (h helpKeys) zoom() key.Binding {
	z := h.m.keys.Zoom
	z.SetEnabled(z.Enabled() && !h.m.narrow())
	return z
}

// back is the back key, which shows every pane again while one is
// zoomed, closes a patch, and closes the modal from the branches.
func (h helpKeys) back() key.Binding {
	m := h.m
	switch {
	case m.zoomed():
		return named(m.keys.Back, "unzoom")
	case m.focus == commitPane && m.commit.patch:
		return named(m.keys.Back, "files")
	case m.focus == branchPane:
		return named(m.keys.Back, "close")
	}
	return m.keys.Back
}

// named returns b described as desc.
func named(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}
