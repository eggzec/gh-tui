package keyhelp

import "slices"

// Option configures the help in [New].
type Option func(*settings)

type settings struct {
	layers        []Layer
	title         string
	placeholder   string
	emptyText     string
	width, height int
	keys          KeyMap
	styles        Styles
}

func defaultSettings() settings {
	return settings{
		title:       "Help",
		placeholder: "Filter keys, or tab and press one",
		emptyText:   "No keys match.",
		styles:      DefaultStyles(true),
	}
}

// WithLayers sets the layers to list, in the order they take keys. The
// help keeps a copy.
func WithLayers(layers []Layer) Option {
	return func(s *settings) { s.layers = slices.Clone(layers) }
}

// WithTitle sets the title, such as "Help · Pull requests". The default is
// "Help".
func WithTitle(title string) Option {
	return func(s *settings) { s.title = title }
}

// WithPlaceholder sets the text shown while the query is empty.
func WithPlaceholder(text string) Option {
	return func(s *settings) { s.placeholder = text }
}

// WithEmptyText sets the text shown when no binding matches.
func WithEmptyText(text string) Option {
	return func(s *settings) { s.emptyText = text }
}

// WithSize sets the width and height.
func WithSize(width, height int) Option {
	return func(s *settings) { s.width, s.height = max(width, 0), max(height, 0) }
}

// WithKeyMap sets the key bindings, which NewKeyMap makes. Without them
// every binding is disabled, and no key acts.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) { s.keys = k }
}

// WithStyles sets the styles.
func WithStyles(st Styles) Option {
	return func(s *settings) { s.styles = st }
}
