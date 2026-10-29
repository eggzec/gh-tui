package pager

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// Append tells the pager that its [Rows] grew: rows from to Len()-1 are new.
// It is the only way to tell the pager that Len grew, and the host calls it
// in the same Update that appended, before the pager draws. The rows before
// from must not change.
//
// The pager takes in only the new rows: the filter and squeeze pick among
// them, the search adds their matches to those it has, and the window, the
// query, the filter and the match the search is at stay where they are,
// unless following keeps the window at the end ([Model.SetFollow]). It costs
// as much as the new rows, not as much as all of them.
//
// A from before the rows the pager has taken in means rows it has seen
// changed, and a Len that shrank means rows went: it then takes in all the
// rows again, which costs as much as SetRows, keeping the window, the
// search and the filter as a change of the filter does. A from past them is
// read as the first row the pager has not seen. Append does nothing for
// anything but host rows, such as content or rendered content. The returned
// command is nil unless the pager had to take in all the rows again and
// that can take long, and then runs the filter.
//
// Append writes past the end of the lists it keeps, in place, so it must be
// called on the pager that owns the rows: copies of the model that both
// append would write over each other, as they would the rows.
func (m *Model) Append(from int) tea.Cmd {
	if m.state != stateReady || m.render != nil {
		return nil
	}
	n, seen := m.rows.Len(), m.seen
	switch {
	case n == seen && from >= seen:
		return nil
	case from < seen || n < seen:
		return m.retake(n)
	}
	gutter := m.gutterFor(seen)
	for i := seen; i < n; i++ {
		m.size += len(m.rows.Text(i)) + 1
	}
	ctx := context.Background()
	if m.proj.none() {
		m.kept = n
	} else {
		m.vis, m.kept, _ = pickRange(ctx, m.rows, m.proj, m.pics, seen, n, m.vis, m.kept)
	}
	// A search that is still running adds the rows when it arrives, as a
	// filter that is still picking does.
	if s := &m.search; s.re != nil && !s.running {
		s.lines, s.ends, _ = findFrom(ctx, s.re, s.invert, m.rows, m.vis, seen, s.lines, s.ends)
		m.enableSearchKeys()
	}
	m.seen = n
	// A host's gutter has grown with its rows before Append is called, so
	// both widths read alike and only the matches kept can be dropped.
	if m.gutter != nil || m.gutterFor(n) != gutter {
		// More digits narrow the text, which the matches shown depend on.
		m.hits.valid = false
	}
	if m.follow {
		m.toEnd()
	}
	m.clamp()
	return nil
}

// retake takes in all n rows again, after rows the pager had seen changed.
func (m *Model) retake(n int) tea.Cmd {
	m.size, m.seen = 0, n
	for i := range n {
		m.size += len(m.rows.Text(i)) + 1
	}
	// The filter runs again over the rows, which also runs the search again.
	cmd := m.project(m.want)
	if m.follow {
		m.toEnd()
	}
	m.clamp()
	return cmd
}

// toEnd scrolls the window to the end, and puts the cursor, if there is
// one, on the last line shown.
func (m *Model) toEnd() {
	m.top, m.row = m.last()
	if m.cursoring {
		m.setCurPos(m.count() - 1)
	}
}

// SetFollow keeps the window at the end of the rows as they grow with
// [Model.Append], like tail -f, and scrolls to the end now. Scrolling away
// from the end with a key turns it off, and the parent turns it on again,
// such as on the key that goes to the end. It is kept for each document.
func (m *Model) SetFollow(on bool) {
	m.follow = on
	if on && m.state == stateReady {
		m.toEnd()
		m.clamp()
	}
}

// Following reports whether the window follows the end of the rows.
func (m Model) Following() bool { return m.follow }

// stopFollowing turns following off after a key left the window away from
// the end.
func (m *Model) stopFollowing() {
	if !m.follow {
		return
	}
	if top, row := m.last(); m.top != top || m.row != row || m.cursoring && !m.cursorAtEnd() {
		m.follow = false
	}
}
