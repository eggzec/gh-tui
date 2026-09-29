package toast

// Option configures a [Model] in [New].
type Option func(*Model)

// WithMax sets the most toasts shown at once. Values below one are treated
// as one.
func WithMax(n int) Option {
	return func(m *Model) { m.max = max(n, 1) }
}

// WithRoom sets the room of a level. A share is kept from 1 to 100, and
// lines to at least one.
func WithRoom(level Level, r Room) Option {
	return func(m *Model) {
		if level.valid() {
			m.rooms[level] = r.valid()
		}
	}
}

// WithSize sets the area the stack is placed in.
func WithSize(width, height int) Option {
	return func(m *Model) { m.width, m.height = width, height }
}

// WithInset keeps the stack right cells from the right edge and bottom
// lines from the bottom of the background it is drawn over, for example
// inside the border of a pane. The stack takes its share of the width set
// with WithSize, and is shifted left by the inset. However narrow the
// width, it keeps as far from the left edge too.
func WithInset(right, bottom int) Option {
	return func(m *Model) { m.inset = [2]int{max(right, 0), max(bottom, 0)} }
}

// WithKeyMap sets the key bindings.
func WithKeyMap(k KeyMap) Option {
	return func(m *Model) { m.keys = k }
}

// WithStyles sets the styles.
func WithStyles(s Styles) Option {
	return func(m *Model) { m.SetStyles(s) }
}
