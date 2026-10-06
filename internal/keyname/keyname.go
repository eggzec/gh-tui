// Package keyname reads the names of keys as the config and bubbletea
// write them, such as "r", "?", "ctrl+r", "shift+tab" or "space". The
// config checks the names it is given with it, and the tui presses keys
// by their names.
package keyname

import (
	"strings"
	"sync"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// keyMods are the modifiers that prefix a key's name, such as "ctrl+".
var keyMods = map[string]tea.KeyMod{
	"ctrl": tea.ModCtrl, "alt": tea.ModAlt, "shift": tea.ModShift,
	"meta": tea.ModMeta, "hyper": tea.ModHyper, "super": tea.ModSuper,
}

// keyCodes are the keys named by a word, such as "enter" or "f13", as
// bubbletea names them. A word that names two keys, such as "up" for the
// arrow and the keypad's, names the first. The keypad's digits name
// none: "1" is the key that types 1. Nor does "select", the Select key of
// old terminals, which almost no keyboard has, so that the name reads
// only as the config's select action.
var keyCodes = sync.OnceValue(func() map[string]rune {
	out := map[string]rune{
		"enter": tea.KeyEnter, "tab": tea.KeyTab, "backspace": tea.KeyBackspace,
		"esc": tea.KeyEscape, "space": tea.KeySpace,
	}
	for code := tea.KeyUp; code <= tea.KeyIsoLevel5Shift; code++ {
		name := tea.Key{Code: code}.String()
		if _, ok := out[name]; ok || utf8.RuneCountInString(name) < 2 || code == tea.KeySelect {
			continue
		}
		out[name] = code
	}
	return out
})

// Press returns a press of the key named name, and false for a name that
// names no key, so that no press could ever match it.
func Press(name string) (tea.KeyPressMsg, bool) {
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
	if code, ok := keyCodes()[rest]; ok {
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

// Valid reports whether name names a key that a press can match.
func Valid(name string) bool {
	_, ok := Press(name)
	return ok
}
