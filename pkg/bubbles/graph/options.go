package graph

import "context"

// Option configures a graph in [New].
type Option func(*settings)

type settings struct {
	parent    context.Context
	width     int
	height    int
	emptyText string
	errorText func(error) (text, hint string)
	keyMap    KeyMap
	styles    Styles
	focused   bool
	prefetch  int
	maxLanes  int
}

func defaultSettings() settings {
	return settings{
		parent:    context.Background(),
		emptyText: "No commits on this branch.",
		keyMap:    NewKeyMap(unbound),
		styles:    DefaultStyles(true),
		maxLanes:  DefaultMaxLanes,
	}
}

// DefaultMaxLanes is the number of lanes a graph draws by default. With two
// cells a lane, that leaves room for the title at 40 columns.
const DefaultMaxLanes = 8

// maxMaxLanes caps WithMaxLanes, far beyond what fits on a screen, so lane
// colors fit in a byte.
const maxMaxLanes = 64

// WithMaxLanes sets how many lanes the graph draws. The lanes past them fold
// into one … slot, which shows ● for a commit in one of them.
func WithMaxLanes(n int) Option {
	return func(s *settings) {
		s.maxLanes = min(max(n, 1), maxMaxLanes)
	}
}

// WithPrefetch sets how many rows before the last loaded commit the cursor
// has to be for the graph to fetch the next chunk. The default, 0, uses the
// height of the window.
func WithPrefetch(rows int) Option {
	return func(s *settings) {
		s.prefetch = max(rows, 0)
	}
}

// WithSize sets the width and height of the graph in cells.
func WithSize(width, height int) Option {
	return func(s *settings) {
		s.width, s.height = max(width, 0), max(height, 0)
	}
}

// WithEmptyText sets the text shown when the branch has no commits.
func WithEmptyText(text string) Option {
	return func(s *settings) {
		s.emptyText = text
	}
}

// WithErrorText sets how the error row reads a failed fetch. say returns
// the words for err and a hint, such as "r to retry", or "" for none; the
// hint is styled as one and kept whole when the row is cut. An empty text
// shows no error. By default the row says "Couldn't load:" and the first
// line of the error, and names the retry key.
func WithErrorText(say func(error) (text, hint string)) Option {
	return func(s *settings) {
		s.errorText = say
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

// WithFocused sets whether the graph starts focused and reacts to keys.
func WithFocused(focused bool) Option {
	return func(s *settings) {
		s.focused = focused
	}
}

// WithContext sets the parent context of every fetch. Cancel it to stop the
// graph's work in flight, for example when the user navigates away.
func WithContext(ctx context.Context) Option {
	return func(s *settings) {
		if ctx != nil {
			s.parent = ctx
		}
	}
}
