package treeview

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

// WithRows sets the rows of the tree.
func WithRows(rows []Node) Option {
	return func(m *Model) { m.SetRows(rows) }
}
