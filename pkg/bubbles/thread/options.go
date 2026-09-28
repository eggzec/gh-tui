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
	maxChunks     int
	emptyText     string
	errorText     func(error) (text, hint string)
}

// DefaultMaxChunks is how many comment chunks a thread keeps rendered unless
// [WithMaxChunks] says otherwise.
const DefaultMaxChunks = 8

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

// WithMaxChunks sets how many comment chunks stay in memory. Chunks far from
// the screen beyond that are dropped, keeping only their cursor and height,
// and are fetched again when the screen nears them. The chunks on or near the
// screen are always kept, so a small n never makes them flicker. Zero or less
// keeps every chunk.
func WithMaxChunks(n int) Option {
	return func(s *settings) {
		s.maxChunks = n
	}
}

// WithEmptyText sets what the thread says when it has no comments, such as
// "No assets." when the items are something else. The default is "No
// comments yet."
func WithEmptyText(text string) Option {
	return func(s *settings) {
		s.emptyText = text
	}
}

// WithErrorText sets how the thread reads a failed fetch of comments, in
// the status line or in place of a chunk that failed to load again. say
// returns the words for err and a hint, such as "r to retry", or "" for
// none; the hint is styled as one and kept whole when the line is cut. An
// empty text shows no error, only the retry key. By default the thread
// says "Couldn't load comments:" and the first line of the error, and
// names the retry key.
func WithErrorText(say func(error) (text, hint string)) Option {
	return func(s *settings) {
		s.errorText = say
	}
}
