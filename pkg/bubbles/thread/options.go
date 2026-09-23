package thread

import (
	"context"

	"charm.land/glamour/v2/ansi"
)

// Option configures a thread in [New].
type Option func(*settings)

type settings struct {
	width, height int
	focused       bool
	keys          KeyMap
	styles        Styles
	markdown      *ansi.StyleConfig
	ctx           context.Context
}

// WithSize sets the width and height of the thread.
func WithSize(width, height int) Option {
	return func(s *settings) {
		s.width, s.height = width, height
	}
}

// WithFocused sets whether the thread starts focused.
func WithFocused(focused bool) Option {
	return func(s *settings) {
		s.focused = focused
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

// WithMarkdownStyle sets the glamour style of the body, overriding
// Styles.Markdown, including after later calls to SetStyles. Tests use it to
// pin the style.
func WithMarkdownStyle(cfg ansi.StyleConfig) Option {
	return func(s *settings) {
		s.markdown = &cfg
	}
}

// WithContext sets the parent context of every fetch. Cancelling it cancels
// the fetches in flight.
func WithContext(ctx context.Context) Option {
	return func(s *settings) {
		s.ctx = ctx
	}
}
