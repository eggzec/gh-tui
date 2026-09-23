package tabs

import "slices"

// Option configures a [Model] in [New].
type Option func(*Model)

// WithTabs sets the tab titles.
func WithTabs(titles ...string) Option {
	return func(m *Model) { m.tabs = slices.Clone(titles) }
}

// WithActive sets the initially active tab.
func WithActive(i int) Option {
	return func(m *Model) { m.active = i }
}

// WithWidth sets the width of the bar.
func WithWidth(width int) Option {
	return func(m *Model) { m.width = width }
}

// WithFocused sets whether the bar starts focused.
func WithFocused(focused bool) Option {
	return func(m *Model) { m.focused = focused }
}

// WithKeyMap sets the key bindings.
func WithKeyMap(k KeyMap) Option {
	return func(m *Model) { m.keys = k }
}

// WithStyles sets the styles.
func WithStyles(s Styles) Option {
	return func(m *Model) { m.styles = s }
}
