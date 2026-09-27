package tree

import "context"

// Option configures a tree in [New].
type Option func(*settings)

// Icons returns the icon drawn before the name of n, for example a folder
// or a file type, styled as it should be drawn. Return "" for no icon.
//
// The tree asks once for each node and state as the node loads, and keeps
// the icon, so it must depend only on n and expanded. Leaves are asked
// only with expanded false. Call [Model.SetIcons] to draw other icons,
// for example after the theme changes.
type Icons func(n Node, expanded bool) string

type settings struct {
	parent      context.Context
	width       int
	height      int
	emptyText   string
	keyMap      KeyMap
	styles      Styles
	focused     bool
	scrollOff   int
	icons       Icons
	expandNodes int
	expandDepth int
	maxLoads    int
}

func defaultSettings() settings {
	return settings{
		parent:      context.Background(),
		emptyText:   "Nothing to show.",
		keyMap:      DefaultKeyMap(),
		styles:      DefaultStyles(true),
		scrollOff:   DefaultScrollOff,
		expandNodes: DefaultExpandAllNodes,
		expandDepth: DefaultExpandAllDepth,
		maxLoads:    DefaultMaxLoads,
	}
}

const (
	// DefaultScrollOff is the number of rows kept visible above and below
	// the cursor by default.
	DefaultScrollOff = 2
	// DefaultExpandAllNodes is the number of nodes an expand-all reveals at
	// most by default.
	DefaultExpandAllNodes = 1000
	// DefaultExpandAllDepth is the number of levels below the cursor an
	// expand-all opens at most by default.
	DefaultExpandAllDepth = 16
	// DefaultMaxLoads is the number of loads an expand-all keeps in flight
	// at most by default.
	DefaultMaxLoads = 4
)

// WithSize sets the width and height of the tree in cells.
func WithSize(width, height int) Option {
	return func(s *settings) {
		s.width, s.height = max(width, 0), max(height, 0)
	}
}

// WithEmptyText sets the text shown when the tree has no nodes. Tell the
// user what they can do about it.
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

// WithFocused sets whether the tree starts focused and reacts to keys.
func WithFocused(focused bool) Option {
	return func(s *settings) {
		s.focused = focused
	}
}

// WithScrollOff sets how many rows stay visible above and below the cursor
// when the tree scrolls. The default is [DefaultScrollOff].
func WithScrollOff(rows int) Option {
	return func(s *settings) {
		s.scrollOff = max(rows, 0)
	}
}

// WithIcons draws an icon before every name. Trees have no icons by default,
// since icon fonts are not installed everywhere.
func WithIcons(icons Icons) Option {
	return func(s *settings) {
		s.icons = icons
	}
}

// WithExpandAllLimits caps an expand-all, which loads every branch below the
// cursor and could otherwise send a request for each directory of a huge
// tree. It stops expanding once nodes children have been revealed, and
// never opens branches more than depth levels below the cursor. Levels
// load one after another, so the cap keeps the nodes nearest the cursor.
// The defaults are [DefaultExpandAllNodes] and [DefaultExpandAllDepth].
func WithExpandAllLimits(nodes, depth int) Option {
	return func(s *settings) {
		s.expandNodes, s.expandDepth = max(nodes, 1), max(depth, 1)
	}
}

// WithMaxLoads sets how many loads an expand-all keeps in flight at once.
// The default is [DefaultMaxLoads].
func WithMaxLoads(n int) Option {
	return func(s *settings) {
		s.maxLoads = max(n, 1)
	}
}

// WithContext sets the parent context of every load. Cancel it to stop the
// tree's work in flight, for example when the user navigates away.
func WithContext(ctx context.Context) Option {
	return func(s *settings) {
		if ctx != nil {
			s.parent = ctx
		}
	}
}
