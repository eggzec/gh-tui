package finder

import "context"

// Option configures a finder in [New].
type Option func(*settings)

type settings struct {
	parent        context.Context
	width, height int
	focused       bool
	keys          KeyMap
	styles        Styles
	placeholder   string
	one, many     string
	syncLimit     int
	recentPaths   []string
}

// DefaultSyncLimit is how many paths the finder matches in Update by
// default. Matching more runs in a command, since it may take longer than
// a frame.
const DefaultSyncLimit = 5000

func defaultSettings() settings {
	return settings{
		parent:      context.Background(),
		keys:        DefaultKeyMap(),
		styles:      DefaultStyles(true),
		placeholder: "Type to find a file",
		one:         "file",
		many:        "files",
		syncLimit:   DefaultSyncLimit,
	}
}

// WithContext sets the context that bounds the load and the matches, which
// Close cancels. It is context.Background by default.
func WithContext(ctx context.Context) Option {
	return func(s *settings) {
		if ctx != nil {
			s.parent = ctx
		}
	}
}

// WithSize sets the width and height.
func WithSize(width, height int) Option {
	return func(s *settings) { s.width, s.height = max(width, 0), max(height, 0) }
}

// WithKeyMap sets the key bindings.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) { s.keys = k }
}

// WithStyles sets the styles.
func WithStyles(st Styles) Option {
	return func(s *settings) { s.styles = st }
}

// WithPlaceholder sets the text shown while the query is empty.
func WithPlaceholder(text string) Option {
	return func(s *settings) { s.placeholder = text }
}

// WithNoun sets what the status line calls a path, one and many of them,
// such as "file" and "files", the default.
func WithNoun(one, many string) Option {
	return func(s *settings) { s.one, s.many = one, many }
}

// WithRecent ranks the paths opened recently above others that match as
// well, and first while the query is empty, the most recent first. At most
// 16 count.
func WithRecent(paths []string) Option {
	return func(s *settings) {
		n := min(len(paths), maxRecent)
		s.recentPaths = paths[:n:n]
	}
}

// WithSyncLimit sets how many paths the finder matches in Update: a query
// with more candidates is matched in a command, and its result shown when
// it arrives. The default is DefaultSyncLimit; 0 matches every query but
// the empty one in a command.
func WithSyncLimit(n int) Option {
	return func(s *settings) { s.syncLimit = max(n, 0) }
}
