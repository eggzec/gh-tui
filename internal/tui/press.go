package tui

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// keyCodes are the keys that the config names by a word, such as "enter".
var keyCodes = map[string]rune{
	"enter": tea.KeyEnter, "tab": tea.KeyTab, "backspace": tea.KeyBackspace, "esc": tea.KeyEscape,
	"space": tea.KeySpace, "up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
	"insert": tea.KeyInsert, "delete": tea.KeyDelete,
	"f1": tea.KeyF1, "f2": tea.KeyF2, "f3": tea.KeyF3, "f4": tea.KeyF4, "f5": tea.KeyF5, "f6": tea.KeyF6,
	"f7": tea.KeyF7, "f8": tea.KeyF8, "f9": tea.KeyF9, "f10": tea.KeyF10, "f11": tea.KeyF11, "f12": tea.KeyF12,
}

// keyMods are the modifiers that prefix a key's name, such as "ctrl+".
var keyMods = map[string]tea.KeyMod{
	"ctrl": tea.ModCtrl, "alt": tea.ModAlt, "shift": tea.ModShift,
	"meta": tea.ModMeta, "hyper": tea.ModHyper, "super": tea.ModSuper,
}

// keyPress returns a press of the key named name, as the config and
// bubbletea name keys, such as "r", "?", "ctrl+r" or "shift+tab", and
// false for a name it can't press.
func keyPress(name string) (tea.KeyPressMsg, bool) {
	var k tea.Key
	rest := name
	for {
		mod, after, ok := strings.Cut(rest, "+")
		m, known := keyMods[mod]
		if !ok || !known || after == "" {
			break
		}
		k.Mod |= m
		rest = after
	}
	if code, ok := keyCodes[rest]; ok {
		k.Code = code
	} else {
		r, n := utf8.DecodeRuneInString(rest)
		if r == utf8.RuneError || n != len(rest) {
			return tea.KeyPressMsg{}, false
		}
		k.Code = r
		if k.Mod&^tea.ModShift == 0 {
			k.Text = rest
		}
	}
	msg := tea.KeyPressMsg(k)
	// Keys are matched by their names, so a press that reads otherwise
	// wouldn't be the key.
	return msg, msg.String() == name
}

// press presses the first key bound to action, as the user would, so that
// a command does just what its key does, where the key goes: the same
// gates, questions and help apply.
func (m *Model) press(action string) tea.Cmd {
	for _, name := range m.cfg.Keys[action] {
		if msg, ok := keyPress(name); ok {
			return m.key(msg)
		}
	}
	return m.toast.Push(toast.Error, "No key is bound to "+action+" in the config.")
}
