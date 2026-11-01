package treeview

import "context"

// Option configures a tree in [New].
type Option func(*Model)

// WithSize sets the width and height of the tree in cells.
func WithSize(width, height int) Option {
	return func(m *Model) { m.width, m.height = max(width, 0), max(height, 0) }
}

// WithKeyMap sets the key bindings of the tree.
func WithKeyMap(k KeyMap) Option {
	return func(m *Model) { m.keyMap = k }
}

// WithStyles sets the styles of the tree.
func WithStyles(s Styles) Option {
	return func(m *Model) { m.setStyles(s) }
}

// WithFocused sets whether the tree starts focused.
func WithFocused(focused bool) Option {
	return func(m *Model) { m.focused = focused }
}

// WithContext sets the context the loads of the tree run under; cancelling it
// cancels them. The default is [context.Background].
func WithContext(ctx context.Context) Option {
	return func(m *Model) { m.ctx = ctx }
}

// WithIcons draws the text returned for a row before its text, and a space.
func WithIcons(icon func(Node) string) Option {
	return func(m *Model) { m.icon = icon }
}

// WithEmpty sets the text shown when there are no rows.
func WithEmpty(text string) Option {
	return func(m *Model) {
		s := m.styles
		s.EmptyText = text
		m.setStyles(s)
	}
}

// WithErrorText sets how a failed load is worded. The default is the error's
// own text.
func WithErrorText(text func(error) string) Option {
	return func(m *Model) { m.errText = text }
}

// WithMaxLoads sets how many loads expand-all runs at once. The default is
// [DefaultMaxLoads].
func WithMaxLoads(n int) Option {
	return func(m *Model) { m.maxLoads = max(n, 1) }
}

// WithExpandAllLimits sets how many rows and how many levels expand-all may
// open before it stops and reports [Model.Capped]. The defaults are
// [DefaultExpandAllNodes] and [DefaultExpandAllDepth].
func WithExpandAllLimits(nodes, depth int) Option {
	return func(m *Model) { m.maxNodes, m.maxDepth = max(nodes, 1), max(depth, 1) }
}

// WithRows sets the rows of a tree that has no source of its own, the static
// path: pair it with a New(Source{}), and change the rows later with SetRows.
// A tree built from [Rows] or [Lazy] gets its top level from the source, which
// replaces rows set this way.
func WithRows(rows []Node) Option {
	return func(m *Model) { m.SetRows(rows) }
}
