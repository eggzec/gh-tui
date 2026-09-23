package feed

import "context"

// Option configures a feed in [New].
//
// Option is deliberately not generic: it sets the settings every feed shares,
// whatever its item type, so call sites never spell out type arguments.
type Option func(*settings)

// settings holds the configuration that does not depend on the item type.
type settings struct {
	ctx        context.Context
	width      int
	height     int
	itemHeight int
	emptyText  string
	keyMap     KeyMap
	styles     Styles
	focused    bool
}

func defaultSettings() settings {
	return settings{
		ctx:        context.Background(),
		itemHeight: 1,
		emptyText:  "Nothing to show.",
		keyMap:     DefaultKeyMap(),
		styles:     DefaultStyles(true),
	}
}

// WithSize sets the width and height of the feed in cells.
func WithSize(width, height int) Option {
	return func(s *settings) {
		s.width, s.height = max(width, 0), max(height, 0)
	}
}

// WithItemHeight sets the number of lines every row takes. The default is 1.
func WithItemHeight(n int) Option {
	return func(s *settings) {
		s.itemHeight = max(n, 1)
	}
}

// WithEmptyText sets the text shown when the feed has no items. Tell the
// user what they can do about it, for example how to change the filter.
func WithEmptyText(text string) Option {
	return func(s *settings) {
		s.emptyText = text
	}
}

// WithKeyMap sets the key bindings.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) {
		s.keyMap = k
	}
}

// WithStyles sets the styles.
func WithStyles(st Styles) Option {
	return func(s *settings) {
		s.styles = st
	}
}

// WithFocused sets whether the feed starts focused and reacts to keys.
func WithFocused(focused bool) Option {
	return func(s *settings) {
		s.focused = focused
	}
}

// WithContext sets the parent context of every fetch. Cancel it to stop the
// feed's work in flight, for example when the user navigates away.
func WithContext(ctx context.Context) Option {
	return func(s *settings) {
		if ctx != nil {
			s.ctx = ctx
		}
	}
}
