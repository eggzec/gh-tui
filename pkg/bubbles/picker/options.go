package picker

import (
	"context"
	"slices"
	"time"
)

// Option configures a picker in [New].
type Option func(*settings)

type settings struct {
	parent        context.Context
	items         []Item
	local         bool
	scopes        []string
	debounce      time.Duration
	placeholder   string
	emptyText     string
	headers       bool
	width, height int
	keys          KeyMap
	styles        Styles
	focused       bool
}

func defaultSettings() settings {
	return settings{
		parent:      context.Background(),
		debounce:    DefaultDebounce,
		placeholder: "Search…",
		emptyText:   "No results. Try other words.",
		headers:     true,
		keys:        DefaultKeyMap(),
		styles:      DefaultStyles(true),
	}
}

// WithItems sets a fixed list of items. Without a Search function the
// picker filters them with fuzzy matching as the user types; with one, it
// lists them while the query is empty. The picker keeps a copy.
func WithItems(items []Item) Option {
	return func(s *settings) {
		s.items, s.local = items, true
	}
}

// WithScopes sets the kinds the scope keys cycle through, after "All". The
// scope reaches the Search function in Query.Scope, and limits fixed items
// to those of its kind.
func WithScopes(kinds ...string) Option {
	return func(s *settings) {
		s.scopes = slices.Clone(kinds)
	}
}

// WithDebounce sets how long the picker waits after the last key before it
// searches. Zero searches on every key. The default is DefaultDebounce.
// Filtering fixed items never waits.
func WithDebounce(d time.Duration) Option {
	return func(s *settings) {
		s.debounce = max(d, 0)
	}
}

// WithPlaceholder sets the text shown while the input is empty.
func WithPlaceholder(text string) Option {
	return func(s *settings) {
		s.placeholder = text
	}
}

// WithEmptyText sets the text shown when nothing matches. Tell the user what
// they can do about it.
func WithEmptyText(text string) Option {
	return func(s *settings) {
		s.emptyText = text
	}
}

// WithGroupHeaders sets whether each kind of item gets a header row. The
// default is true.
func WithGroupHeaders(show bool) Option {
	return func(s *settings) {
		s.headers = show
	}
}

// WithSize sets the width and height of the picker, frame included.
func WithSize(width, height int) Option {
	return func(s *settings) {
		s.width, s.height = max(width, 0), max(height, 0)
	}
}

// WithKeyMap sets the key bindings.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) {
		s.keys = k
	}
}

// WithStyles sets the styles. The default is DefaultStyles(true).
func WithStyles(st Styles) Option {
	return func(s *settings) {
		s.styles = st
	}
}

// WithContext sets the parent context of every search. Cancel it to stop
// the picker's work in flight.
func WithContext(ctx context.Context) Option {
	return func(s *settings) {
		if ctx != nil {
			s.parent = ctx
		}
	}
}
