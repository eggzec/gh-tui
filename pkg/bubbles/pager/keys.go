package pager

import (
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// KeyMap holds the key bindings of a pager. It implements help.KeyMap.
type KeyMap struct {
	Up           key.Binding `keymap:"up" help:"up"`
	Down         key.Binding `keymap:"down" help:"down"`
	PageUp       key.Binding `keymap:"page_up" help:"page up"`
	PageDown     key.Binding `keymap:"page_down" help:"page down"`
	HalfPageUp   key.Binding `keymap:"half_page_up" help:"½ page up"`
	HalfPageDown key.Binding `keymap:"half_page_down" help:"½ page down"`
	Home         key.Binding `keymap:"top" help:"top"`
	End          key.Binding `keymap:"bottom" help:"bottom"`
	// Left and Right scroll sideways while lines are not wrapped.
	Left  key.Binding `keymap:"left" help:"left"`
	Right key.Binding `keymap:"right" help:"right"`

	// Option waits for the name of an option to toggle, as less's - does:
	// S chops or wraps long lines, N shows or hides the line numbers, s
	// squeezes runs of blank lines into one, i ignores case in searches
	// unless the pattern has a capital, or matches it, and I ignores case
	// always, or matches it. Esc then cancels it. NewKeyMap words the help
	// from the keys of Options; the tag has its words with the default
	// ones.
	Option  key.Binding  `keymap:"option" help:"option: S N s i I"`
	Options OptionKeyMap `keymap:"pager_option"`

	// Search opens the search prompt, Confirm searches for the pattern
	// typed, a regexp, or for the lines it doesn't match after a "!", and
	// Cancel closes the prompt. Outside the prompt, Cancel clears the
	// search. The pager enables Confirm only while the prompt is open, and
	// Cancel only while it is or a search or filter is shown, so esc
	// closes the pager otherwise.
	Search  key.Binding `keymap:"find" help:"search"`
	Confirm key.Binding `keymap:"search_prompt.run" help:"search"`
	Cancel  key.Binding `keymap:"search_prompt.cancel" help:"cancel"`
	// CancelEmpty closes the prompt too, but only on an empty line, where
	// the prompt takes it before it would erase a character.
	CancelEmpty key.Binding `keymap:"search_prompt.cancel_empty" help:"cancel"`
	// Filter opens the filter prompt, where Confirm shows only the lines
	// the pattern typed matches, or doesn't match after a "!", and an
	// empty line shows them all again. Outside the prompt, Cancel stops
	// a filter still running, or clears the one shown, once no search is
	// shown.
	Filter key.Binding `keymap:"quick_filter" help:"filter"`
	// Next and Prev move between matches. The pager enables them only
	// while there are matches, so help shows them only when they work.
	Next key.Binding `keymap:"next_match" help:"next match"`
	Prev key.Binding `keymap:"prev_match" help:"prev match"`

	// Edit opens the content in an external editor, at the line at the
	// top of the window, and suspends the program until it exits: the
	// editor set with WithEditor, else $VISUAL, else $EDITOR. The pager
	// enables it only while it shows content.
	Edit key.Binding `keymap:"edit" help:"edit"`

	// Quit and Dismiss both ask the parent to close the pager with a
	// [CloseMsg]: quit as the app's quit key, and dismiss as its key for
	// stepping back out of what is open. While a search is shown, a key
	// bound to Cancel clears it first.
	Quit    key.Binding `keymap:"global.quit" help:"close"`
	Dismiss key.Binding `keymap:"global.dismiss" help:"close"`
}

// OptionKeyMap holds the keys that name an option, after the pager's
// Option key. The pager enables them only while it waits for one.
type OptionKeyMap struct {
	// Chop chops or wraps long lines, and LineNumbers shows or hides the
	// line numbers.
	Chop        key.Binding `keymap:"chop" help:"chop or wrap long lines"`
	LineNumbers key.Binding `keymap:"line_numbers" help:"line numbers"`
	// Squeeze squeezes runs of blank lines into one.
	Squeeze key.Binding `keymap:"squeeze" help:"squeeze blank lines"`
	// SmartCase ignores case in searches unless the pattern has a
	// capital, or matches it, and IgnoreCase ignores it always, or
	// matches it.
	SmartCase  key.Binding `keymap:"smart_case" help:"smart case"`
	IgnoreCase key.Binding `keymap:"ignore_case" help:"ignore case"`
	// Cancel chooses no option.
	Cancel key.Binding `keymap:"cancel" help:"cancel"`
}

// NewKeyMap returns the key bindings that look gives, where an action is
// named as the pager's own, such as "page_down", or as a context's, such
// as "global.quit". A pager without a key map has no key bound.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	k.Confirm.SetEnabled(false)
	k.Cancel.SetEnabled(false)
	k.CancelEmpty.SetEnabled(false)
	k.Next.SetEnabled(false)
	k.Prev.SetEnabled(false)
	k.Options.setEnabled(false)
	if keys := k.Options.keysHelp(); keys != "" {
		k.Option.SetHelp(k.Option.Help().Key, "option: "+keys)
	} else {
		k.Option.SetHelp(k.Option.Help().Key, "option")
	}
	return k
}

// keysHelp lists the keys that name an option, as help words them: the
// ones that have a key.
func (o OptionKeyMap) keysHelp() string {
	var keys []string
	for _, b := range []key.Binding{o.Chop, o.LineNumbers, o.Squeeze, o.SmartCase, o.IgnoreCase} {
		if len(b.Keys()) > 0 {
			keys = append(keys, b.Help().Key)
		}
	}
	return strings.Join(keys, " ")
}

// setEnabled enables or disables every option key that has a key.
func (o *OptionKeyMap) setEnabled(on bool) {
	for _, b := range []*key.Binding{&o.Chop, &o.LineNumbers, &o.Squeeze, &o.SmartCase, &o.IgnoreCase, &o.Cancel} {
		b.SetEnabled(on)
	}
}

// Close returns the binding that stands for Quit and Dismiss in help, which
// lists them as one row.
func (k KeyMap) Close() key.Binding { return keymap.Join(k.Quit, k.Dismiss) }

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Search, k.Next, k.Prev, k.Close()}
}

// FullHelp returns the bindings for the full help view, every binding of
// the key map once. The model's own full help, which is what help reads,
// lists Quit and Dismiss as one row, [KeyMap.Close].
func (k KeyMap) FullHelp() [][]key.Binding { return k.fullHelp(k.Quit, k.Dismiss) }

// fullHelp returns the full help with closing as the bindings that close
// the pager, listed together.
func (k KeyMap) fullHelp(closing ...key.Binding) [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown},
		{k.Home, k.End, k.Left, k.Right, k.Option},
		{k.Options.Chop, k.Options.LineNumbers, k.Options.Squeeze, k.Options.SmartCase, k.Options.IgnoreCase, k.Options.Cancel},
		append([]key.Binding{k.Search, k.Filter, k.Confirm, k.Cancel, k.CancelEmpty, k.Next, k.Prev, k.Edit}, closing...),
	}
}
