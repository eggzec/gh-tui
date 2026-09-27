package finder

import "context"

// Option configures a finder in [New].
type Option func(*settings)

// Icons returns the icon drawn before the path of it, such as the type of
// a file, styled as it should be drawn. Return "" for no icon. A row asks
// once and is kept until the matches, the size or the styles change, so
// the icon must depend only on it.
type Icons func(it Item) string

// Links returns the address the path of it links to, such as its page on
// the web, or "" for none. A terminal that knows links (OSC 8) opens it on
// a click; only a plain https address links. Like [Icons], it is asked
// once per row kept.
type Links func(it Item) string

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
	icons         Icons
	links         Links
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

// WithIcons draws an icon before every path. Finders have no icons by
// default, since icon fonts are not installed everywhere.
func WithIcons(icons Icons) Option {
	return func(s *settings) { s.icons = icons }
}

// WithLinks links the path of every row to the address links returns.
func WithLinks(links Links) Option {
	return func(s *settings) { s.links = links }
}
