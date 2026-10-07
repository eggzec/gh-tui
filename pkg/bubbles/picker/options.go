package picker

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
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
	errorText     func(error) (text, hint string)
	headers       bool
	width, height int
	keys          KeyMap
	styles        Styles
	focused       bool
	modes         bool
	marked        bool
	markOn        string
	markOff       string
	typed         func(text string) (Item, bool)
	noFilterLine  bool
}

func defaultSettings() settings {
	return settings{
		parent:    context.Background(),
		debounce:  DefaultDebounce,
		emptyText: "No results. Try other words.",
		headers:   true,
		styles:    DefaultStyles(true),
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

// WithPlaceholder sets the text shown while the input is empty. Without
// it, the picker says "Search" and the ellipsis of its styles.
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

// WithErrorText sets how a failed search reads. say returns the words for
// err and a hint, such as "r to retry", or "" for none; the hint is styled
// as one and kept whole when the row is cut. An empty text shows no error.
// By default the row says "Couldn't search:" and the first line of the
// error.
func WithErrorText(say func(error) (text, hint string)) Option {
	return func(s *settings) {
		s.errorText = say
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

// WithKeyMap sets the key bindings, which NewKeyMap makes. Without them
// every binding is disabled, and no key acts.
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

// WithModes gives the picker a normal mode, which it starts in and returns
// to on Focus: the input is blurred, the keys of KeyMap.Normal move, and
// letters don't type. Normal.Insert and Normal.Append focus the input, and
// the cancel key blurs it again, keeping the query, before it cancels the
// picker. Without it, the picker always types.
func WithModes(on bool) Option {
	return func(s *settings) {
		s.modes = on
	}
}

// WithMarks draws on or off, then a space, before each title: on for items
// whose Value is among those given to SetMarked, off for the rest. The
// narrower mark is padded to the width of the other, so a blank one keeps
// the titles aligned.
func WithMarks(on, off string) Option {
	return func(s *settings) {
		on, off = clean(on), clean(off)
		// Both marks take the width of the wider, so titles stay aligned.
		w := max(ansi.StringWidth(on), ansi.StringWidth(off))
		s.marked = true
		s.markOn = on + strings.Repeat(" ", w-ansi.StringWidth(on))
		s.markOff = off + strings.Repeat(" ", w-ansi.StringWidth(off))
	}
}

// WithTyped lets the user choose what they typed. While the query isn't
// empty, in either mode, it lists the item that typed returns, if it says
// ok, as the last row, under a header of its own and not counted among the
// results. It isn't listed while a search runs or after one failed, nor
// when the title or the value of a listed result equals the text, ignoring
// case and surrounding spaces. While a search runs, enter chooses that item
// rather than a result of an earlier query.
func WithTyped(typed func(text string) (Item, bool)) Option {
	return func(s *settings) {
		s.typed = typed
	}
}

// WithFilterLine sets whether the picker draws the input and the line of
// scopes and result count. Without them the list takes the whole height and
// the picker never types, so Normal.Insert and Normal.Append are disabled.
// The default is true.
func WithFilterLine(show bool) Option {
	return func(s *settings) {
		s.noFilterLine = !show
	}
}
