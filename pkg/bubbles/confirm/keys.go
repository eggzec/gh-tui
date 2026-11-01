package confirm

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// KeyMap holds the keys that answer an [Ask]. They only mean something
// while a question is open.
type KeyMap struct {
	Yes key.Binding `keymap:"yes" help:"yes"`
	No  key.Binding `keymap:"no" help:"no"`
	// Default gives the answer that the question shows as its default.
	Default key.Binding `keymap:"default" help:"answer the default"`
}

// NewKeyMap returns the keys that answer a question, as look gives them for
// the actions yes, no and default: by default y for yes, n or esc for no,
// and enter for the answer the question shows as its default, which is no
// unless the change is safe and easy to undo. A press that arrives before
// the question is armed, or a repeated one, never gives that answer.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	// The answers are named by their first key, as the line that asks has
	// little room.
	for _, b := range []*key.Binding{&k.Yes, &k.No, &k.Default} {
		if ks := b.Keys(); len(ks) > 0 {
			b.SetHelp(keymap.Label(ks[0]), b.Help().Desc)
		}
	}
	return k
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding { return []key.Binding{k.Yes, k.No, k.Default} }

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// Answer takes msg as the answer to a: yes runs a and returns what sends
// the change, and no drops it. The default key gives the default of a, but
// only once the question is armed and not for a repeated press. Either way
// done is set, and the question closes. Any other key does nothing and
// leaves the question open.
func (k KeyMap) Answer(a Ask, msg tea.KeyPressMsg, armed bool) (cmd tea.Cmd, done bool) {
	switch {
	case key.Matches(msg, k.Yes):
		return a.Run(), true
	case key.Matches(msg, k.No):
		return nil, true
	case key.Matches(msg, k.Default) && armed && !msg.IsRepeat:
		if a.Default {
			return a.Run(), true
		}
		return nil, true
	}
	return nil, false
}

// answers names the keys that answer, such as "y/n".
func (k KeyMap) answers() string {
	var keys []string
	for _, b := range []key.Binding{k.Yes, k.No} {
		if h := b.Help().Key; h != "" {
			keys = append(keys, h)
		}
	}
	return strings.Join(keys, "/")
}
