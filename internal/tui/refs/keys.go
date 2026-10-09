package refs

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

// Context is the context of the keys of the step. It is a layer of its own,
// not a pane of the modal it shows in, so that the keys of the pull request
// or issue, which would act on it behind the list, are off while it shows.
const Context = "references"

// KeyMap holds the keys of the step and of the tree it shows.
type KeyMap struct {
	// Tree moves through the links and folds their groups, and its open
	// key opens the item under the cursor.
	Tree tree.KeyMap
	// Filter opens the prompt of the quick filter.
	Filter key.Binding
	// Open opens the item under the cursor on GitHub, Refresh reads the
	// links again, Back steps back to the conversation, and Dismiss clears
	// the filter and then closes the modal. They are intents of the keys
	// that work everywhere, which the step implements.
	Open, Refresh, Back, Dismiss key.Binding
	// prompt are the keys of the filter's prompt.
	prompt promptKeys
}

func newKeyMap(keys config.Keymap) KeyMap {
	in, search := ui.In(keys, Context), ui.In(keys, "search_prompt")
	return KeyMap{
		Tree:    tree.NewKeyMap(ui.Lookup(keys, Context)),
		Filter:  in.Binding("quick_filter", "filter"),
		Open:    in.Binding("global.open", "open in browser"),
		Refresh: in.Binding("global.refresh", "refresh"),
		Back:    in.Binding("global.back", "back"),
		Dismiss: in.Binding("global.dismiss", "close"),
		prompt: promptKeys{
			run:         search.Binding("run", "filter"),
			cancel:      search.Binding("cancel", "clear filter"),
			cancelEmpty: search.Binding("cancel_empty", "close when empty"),
		},
	}
}

// promptKeys are the keys of the prompt of the filter, which takes every
// key: enter keeps the filter, esc clears it, and backspace on an empty line
// closes the prompt.
type promptKeys struct {
	run, cancel, cancelEmpty key.Binding
}

// ShortHelp implements help.KeyMap.
func (p promptKeys) ShortHelp() []key.Binding {
	return []key.Binding{p.run, p.cancel}
}

// FullHelp implements help.KeyMap.
func (p promptKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{p.run, p.cancel, p.cancelEmpty}}
}

// KeyLayers returns the keys that work in the step, in the order they are
// matched: the prompt's alone while it types, and otherwise those of the
// step and of the tree. The key that clears the filter says so while there
// is one.
func (s *Step) KeyLayers() []keyhelp.Layer {
	if s.prompt.Focused() {
		return []keyhelp.Layer{ui.ContextHelp("search_prompt", s.keys.prompt, true)}
	}
	k := s.keys
	if s.filter != "" {
		k.Dismiss.SetHelp(k.Dismiss.Help().Key, "clear filter")
	}
	t := k.Tree
	bindings := []key.Binding{
		k.Filter, k.Open, k.Refresh, t.Up, t.Down, t.PageUp, t.PageDown, t.HalfPageUp, t.HalfPageDown, t.Home, t.End,
		t.Expand, t.Collapse, t.ToggleAll, t.Open, k.Back, k.Dismiss,
	}
	short := []key.Binding{t.Open, t.Expand, t.Collapse, t.ToggleAll, k.Filter, k.Back, k.Dismiss}
	return []keyhelp.Layer{ui.ContextLayer(Context, bindings, short)}
}
