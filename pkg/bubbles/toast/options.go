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

// WithKeyMap sets the key bindings.
func WithKeyMap(k KeyMap) Option {
	return func(m *Model) { m.keys = k }
}

// WithStyles sets the styles.
func WithStyles(s Styles) Option {
	return func(m *Model) { m.SetStyles(s) }
}
