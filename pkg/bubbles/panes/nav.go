package panes

// Nav tracks which pane has the focus and, for each pane, whether its list or
// its preview has it. It is a plain value: copying it copies the state. Pane
// numbers are 1-based, and the zero Nav has no focus until SetFocus is called.
type Nav struct {
	focus Pane
	sub   [maxPanes]Role
}

// Focus returns the pane that has the focus.
func (n *Nav) Focus() Pane { return n.focus }

// SetFocus gives the focus to p and gives the list of every pane the focus, as
// a change of pane does.
func (n *Nav) SetFocus(p Pane) {
	n.focus = p
	n.Reset()
}

// Sub returns the part of pane p that has its focus: List or Preview. It
// reports List for a pane out of range.
func (n *Nav) Sub(p Pane) Role {
	if p < 1 || int(p) > maxPanes {
		return List
	}
	return n.sub[p-1]
}

// Reset gives the list of every pane the focus.
func (n *Nav) Reset() {
	for i := range n.sub {
		n.sub[i] = List
	}
}

// InPreview reports whether the preview of the focused pane is shown in l and
// has the focus.
func (n *Nav) InPreview(l Layout) bool {
	if n.Sub(n.focus) != Preview {
		return false
	}
	_, ok := l.Box(n.focus, Preview)
	return ok
}

// CanStep reports whether Step does anything: it moves between the panes, or
// between the list and the preview of a zoomed pane that has one.
func (n *Nav) CanStep(l Layout) bool {
	if !l.Zoomed {
		return true
	}
	_, ok := l.Box(n.focus, Preview)
	return ok
}

// Step moves the focus by delta panes, wrapping, or between the list and the
// preview of a zoomed pane.
func (n *Nav) Step(l Layout, delta int) {
	if l.Zoomed {
		if !n.CanStep(l) || n.focus < 1 || int(n.focus) > maxPanes {
			return
		}
		i := n.focus - 1
		if n.sub[i] == Preview {
			n.sub[i] = List
		} else {
			n.sub[i] = Preview
		}
		return
	}
	count := len(l.Panes)
	if count == 0 {
		return
	}
	n.SetFocus(Pane((int(n.focus)-1+delta%count+count)%count + 1))
}
