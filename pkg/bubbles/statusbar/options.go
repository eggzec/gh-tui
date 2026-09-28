package statusbar

// Option configures a [Model] in [New].
type Option func(*Model)

// WithWidth sets the width of the bar.
func WithWidth(width int) Option {
	return func(m *Model) { m.SetWidth(width) }
}

// WithItems sets the items on the left and on the right of the bar.
func WithItems(left, right []Item) Option {
	return func(m *Model) { m.SetItems(left, right) }
}

// WithStyles sets the styles.
func WithStyles(s Styles) Option {
	return func(m *Model) { m.SetStyles(s) }
}
