package toast

import "time"

// Option configures a [Model] in [New].
type Option func(*Model)

// WithDuration sets how long info, success and warning toasts stay. Zero or
// less keeps them until they are dismissed.
func WithDuration(d time.Duration) Option {
	return func(m *Model) { m.duration = d }
}

// WithErrorDuration sets how long error toasts stay. It defaults to longer
// than the other levels, since errors matter more and take longer to read.
func WithErrorDuration(d time.Duration) Option {
	return func(m *Model) { m.errorDuration = d }
}

// WithMax sets the most toasts shown at once. Values below one are treated
// as one.
func WithMax(n int) Option {
	return func(m *Model) { m.max = max(n, 1) }
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
