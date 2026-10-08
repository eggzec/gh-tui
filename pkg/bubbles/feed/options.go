package feed

import (
	"context"

	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
)

// Option configures a feed in [New].
//
// Option is deliberately not generic: it sets the settings every feed shares,
// whatever its item type, so call sites never spell out type arguments.
type Option func(*settings)

// settings holds the configuration that does not depend on the item type.
type settings struct {
	parent     context.Context
	width      int
	height     int
	itemHeight int
	emptyText  string
	errorText  func(error) (text, hint string)
	keyMap     KeyMap
	styles     Styles
	// promptKeys are the keys of the prompt of a find or filter.
	promptKeys cmdline.KeyMap
	focused    bool
	maxChunks  int
	prefetch   int
	// key is a func(T) string, checked against the item type in New.
	key any
}

func defaultSettings() settings {
	return settings{
		parent:     context.Background(),
		itemHeight: 1,
		emptyText:  "Nothing to show.",
		keyMap:     NewKeyMap(unbound),
		promptKeys: cmdline.NewKeyMap(unbound),
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

// WithPromptKeys sets the keys of the prompt that [KeyMap.Find] and
// [KeyMap.QuickFilter] open: Submit runs what was typed, Cancel closes the
// prompt, and CancelEmpty closes it on an empty line. While no prompt is
// open, Cancel clears the find shown, and then the filter. Without them,
// the prompt can't be closed by a key but a blur.
func WithPromptKeys(k cmdline.KeyMap) Option {
	return func(s *settings) {
		s.promptKeys = k
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

// WithKey sets how to identify an item, so that Reload keeps the selection
// on the same item even when it moved. Without a key, Reload keeps the index.
//
// WithKey is the one option that depends on the item type. It infers T from
// key, so call sites stay free of type arguments, and New ignores a key whose
// type does not match the feed's items. Use [Model.SetKey] to have the
// compiler check the type.
func WithKey[T any](key func(T) string) Option {
	return func(s *settings) {
		s.key = key
	}
}

// WithContext sets the parent context of every fetch. Cancel it to stop the
// feed's work in flight, for example when the user navigates away.
func WithContext(ctx context.Context) Option {
	return func(s *settings) {
		if ctx != nil {
			s.parent = ctx
		}
	}
}
