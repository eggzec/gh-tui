package pager

// Option configures a pager in [New].
type Option func(*settings)

type settings struct {
	width, height  int
	keys           KeyMap
	styles         Styles
	tabWidth       int
	lineNumbers    bool
	wrap           bool
	highlightLimit int
}

// DefaultTabWidth is the number of columns between tab stops by default.
const DefaultTabWidth = 4

// DefaultHighlightLimit is the size in bytes above which content is shown
// as plain text by default. Highlighting runs in the background, but it
// takes seconds for a few megabytes.
const DefaultHighlightLimit = 1 << 20

func defaultSettings() settings {
	return settings{
		keys:           DefaultKeyMap(),
		styles:         DefaultStyles(true),
		tabWidth:       DefaultTabWidth,
		lineNumbers:    true,
		highlightLimit: DefaultHighlightLimit,
	}
}

// WithSize sets the width and height, including the status line.
func WithSize(width, height int) Option {
	return func(s *settings) {
		s.width, s.height = width, height
	}
}

// WithKeyMap sets the key bindings.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) {
		s.keys = k
	}
}

// WithStyles sets the styles.
func WithStyles(st Styles) Option {
	return func(s *settings) {
		s.styles = st
	}
}

// WithTabWidth sets the number of columns between tab stops. It applies to
// content set afterwards.
func WithTabWidth(n int) Option {
	return func(s *settings) {
		s.tabWidth = max(n, 1)
	}
}

// WithLineNumbers sets whether the line numbers are shown. They are by
// default.
func WithLineNumbers(show bool) Option {
	return func(s *settings) {
		s.lineNumbers = show
	}
}

// WithWrap sets whether long lines are soft-wrapped instead of scrolled
// sideways. They are scrolled by default.
func WithWrap(wrap bool) Option {
	return func(s *settings) {
		s.wrap = wrap
	}
}

// WithHighlightLimit sets the size in bytes above which content is shown
// as plain text. Zero or less turns highlighting off.
func WithHighlightLimit(bytes int) Option {
	return func(s *settings) {
		s.highlightLimit = bytes
	}
}
