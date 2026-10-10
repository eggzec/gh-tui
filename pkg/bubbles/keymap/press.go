package keymap

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// actionMark starts the text of a press that stands for an action instead
// of a key. A terminal never reports it: it is a control character that no
// key types.
const actionMark = "\x00action:"

// ActionPress returns the press that stands for action, such as
// "pulls.merge", in the place of a key: [Matches] takes it for the key of
// that action, whether the config binds it to a key or not, so that a
// command does what the key does, where the key goes, with the same
// gates and questions.
func ActionPress(action string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyExtended, Text: actionMark + action}
}

// Pressed returns the action that msg stands for, if it is a press made
// by [ActionPress].
func Pressed(msg any) (action string, ok bool) {
	p, isPress := msg.(tea.KeyPressMsg)
	if !isPress {
		return "", false
	}
	return strings.CutPrefix(p.Text, actionMark)
}

// Matches reports whether k is the key of one of bs, as [key.Matches]
// does, or an [ActionPress] of an action that one of them was made for. A
// binding that has no key because the config unbinds it still takes the
// press of its action, even though it is disabled; one that has keys takes
// it only while it is enabled, as it takes its keys.
func Matches[K fmt.Stringer](k K, bs ...key.Binding) bool {
	action, ok := Pressed(k)
	if !ok {
		return key.Matches(k, bs...)
	}
	return slices.ContainsFunc(bs, func(b key.Binding) bool {
		return (b.Enabled() || len(b.Keys()) == 0) && slices.Contains(Actions(b), action)
	})
}
