package orderpick

import "slices"

// Option configures a picker built with New.
type Option func(*settings)

// settings holds what the options set.
type settings struct {
	items     []Item
	start     []Choice
	single    bool
	keys      KeyMap
	styles    Styles
	heading   string
	emptyText string
	width     int
	height    int
	// markOn and markOff go before the titles of the left pane, where nothing
	// is ticked since a ticked item moves to the right pane.
	markOn  string
	markOff string
}

func defaultSettings() settings {
	return settings{
		styles:    DefaultStyles(true),
		heading:   "Order",
		emptyText: "Nothing chosen",
		markOn:    "☑",
		markOff:   "☐",
	}
}

// WithMarks sets the marks drawn before the titles of the left pane.
func WithMarks(on, off string) Option {
	return func(s *settings) { s.markOn, s.markOff = on, off }
}

// WithItems sets the items that can be chosen, in the order the left pane
// lists them. The picker keeps a copy.
func WithItems(items []Item) Option {
	return func(s *settings) { s.items = slices.Clone(items) }
}

// WithSingle limits the right pane to one item: choosing another replaces it,
// and the old one goes back to the left pane.
func WithSingle(on bool) Option {
	return func(s *settings) { s.single = on }
}

// WithChosen starts with choices in the right pane, in order, each in the
// direction it names. A choice whose value no item has is left out, and a
// picker made WithSingle keeps only the first. Values are compared with ==,
// so they must be comparable.
func WithChosen(choices ...Choice) Option {
	return func(s *settings) { s.start = slices.Clone(choices) }
}

// WithKeyMap sets the key bindings. Without it every binding is disabled.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) { s.keys = k }
}

// WithStyles sets the styles.
func WithStyles(st Styles) Option {
	return func(s *settings) { s.styles = st }
}

// WithHeading sets the title of the right pane.
func WithHeading(text string) Option {
	return func(s *settings) { s.heading = text }
}

// WithEmptyText sets the text the right pane shows while nothing is chosen.
func WithEmptyText(text string) Option {
	return func(s *settings) { s.emptyText = text }
}

// WithSize sets the width and height, which SetSize changes later.
func WithSize(width, height int) Option {
	return func(s *settings) { s.width, s.height = width, height }
}
