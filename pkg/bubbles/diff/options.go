package diff

import "context"

// Option configures a [Layout] in [NewLayout] and a [Model] in [New]. Each
// reads the options that concern it and ignores the rest, so one list of
// options, such as [WithCollapseOver], serves both.
type Option func(*settings)

// settings holds what the options set.
type settings struct {
	collapseOver int
	firstFiles   int

	parent  context.Context
	width   int
	height  int
	keyMap  KeyMap
	styles  Styles
	focused bool
	tabs    int

	highlightLimit int
}

// DefaultTabWidth is the width of a tab stop in the lines of a diff.
const DefaultTabWidth = 4

// DefaultHighlightLimit is the size in bytes of the text of a file above
// which the file is shown as plain text.
const DefaultHighlightLimit = 512 << 10

func defaultSettings() settings {
	return settings{
		parent: context.Background(),
		keyMap: NewKeyMap(unbound),
		styles: DefaultStyles(true),
		tabs:   DefaultTabWidth,

		highlightLimit: DefaultHighlightLimit,
	}
}

// WithCollapseOver makes a file whose patch has more than lines lines start
// collapsed to its header. Zero, the default, collapses nothing.
func WithCollapseOver(lines int) Option {
	return func(s *settings) { s.collapseOver = lines }
}

// WithFirstFilesNote ends the layout with a note row saying that it shows
// only the first n files, for a caller that was given no more.
func WithFirstFilesNote(n int) Option {
	return func(s *settings) { s.firstFiles = n }
}

// WithSize sets the width and height of the view in cells.
func WithSize(width, height int) Option {
	return func(s *settings) { s.width, s.height = max(width, 0), max(height, 0) }
}

// WithKeyMap sets the key bindings.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) { s.keyMap = k }
}

// WithStyles sets the styles.
func WithStyles(st Styles) Option {
	return func(s *settings) { s.styles = st }
}

// WithFocused sets whether the view starts focused and reacts to keys.
func WithFocused(focused bool) Option {
	return func(s *settings) { s.focused = focused }
}

// WithContext sets the parent context of every fetch. Cancel it to stop the
// work in flight, for example when the user navigates away.
func WithContext(ctx context.Context) Option {
	return func(s *settings) {
		if ctx != nil {
			s.parent = ctx
		}
	}
}

// WithTabWidth sets the width of a tab stop. The default is [DefaultTabWidth].
func WithTabWidth(n int) Option {
	return func(s *settings) { s.tabs = max(n, 1) }
}

// WithHighlightLimit sets the size in bytes of the text of a file above
// which it is shown as plain text. Zero or less turns highlighting off.
func WithHighlightLimit(bytes int) Option {
	return func(s *settings) { s.highlightLimit = bytes }
}
