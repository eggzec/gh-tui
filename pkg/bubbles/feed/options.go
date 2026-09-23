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
	maxChunks  int
	prefetch   int
}

func defaultSettings() settings {
	return settings{
		ctx:        context.Background(),
		itemHeight: 1,
		emptyText:  "Nothing to show.",
		keyMap:     DefaultKeyMap(),
		styles:     DefaultStyles(true),
		maxChunks:  DefaultMaxChunks,
	}
}

// DefaultMaxChunks is the number of chunks a feed keeps loaded by default.
// At common page sizes of 30 to 100 items that is several screens around
// the window.
const DefaultMaxChunks = 8

// WithMaxChunks sets how many chunks the feed keeps loaded. It evicts the
// items of chunks farther from the window, and fetches them again when the
// window comes back. The chunks the window needs are always kept.
func WithMaxChunks(n int) Option {
	return func(s *settings) {
		s.maxChunks = max(n, 1)
	}
}

// WithPrefetch sets how many rows before the end of the loaded items the
// feed fetches the next chunk, and how many rows around the window it keeps
// loaded. The default, 0, uses the height of the window.
func WithPrefetch(rows int) Option {
	return func(s *settings) {
		s.prefetch = max(rows, 0)
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
