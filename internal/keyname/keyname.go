// Package keyname reads the names of keys as the config and bubbletea
// write them, such as "r", "?", "ctrl+r", "shift+tab" or "space". The
// config checks the names it is given with it, and the tui presses keys
// by their names.
package keyname

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// modifier is a modifier of a key, by the name it has in a key's name.
type modifier struct {
	name string
	mod  tea.KeyMod
}

// keyMods are the modifiers that prefix a key's name, such as "ctrl+",
// in the order bubbletea writes them.
var keyMods = []modifier{
	{"ctrl", tea.ModCtrl}, {"alt", tea.ModAlt}, {"shift", tea.ModShift},
	{"meta", tea.ModMeta}, {"hyper", tea.ModHyper}, {"super", tea.ModSuper},
}

// modOrder names the modifiers in the order a key's name writes them.
const modOrder = "ctrl, alt, shift, meta, hyper, super"

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

// Press returns a press of the key named name, and false for a name
// that Check refuses, so that no press could ever match it. A sequence
// of keys is no one press, so it gives false; see [ParseChord].
func Press(name string) (tea.KeyPressMsg, bool) {
	if Sequence(name) {
		return tea.KeyPressMsg{}, false
	}
	msg, _, ok := parse(name)
	return msg, ok && Check(name) == nil
}

// Chord is a sequence of two keys, pressed one after the other.
type Chord struct {
	First, Second tea.KeyPressMsg
}

// Sequence reports whether name is a sequence of keys, such as "g g", and
// not one key: its keys are joined by a space, which no key's name has.
func Sequence(name string) bool { return strings.Contains(name, " ") }

// Parts returns the names of the keys that name is: itself for one key, or
// each key of a sequence.
func Parts(name string) []string { return strings.Split(name, " ") }

// ParseChord returns the presses of the sequence name, such as "g g" or
// "ctrl+w x", or else why name is not one: a sequence is two keys that
// each are a valid key name, joined by one space.
func ParseChord(name string) (Chord, error) {
	parts := Parts(name)
	if len(parts) == 1 {
		return Chord{}, fmt.Errorf("%q is one key, not a sequence of two such as \"g g\"", name)
	}
	if slices.Contains(parts, "") {
		return Chord{}, fmt.Errorf("the sequence %q has an empty key: join its two keys with one space, such as \"g g\"", name)
	}
	if len(parts) != 2 {
		return Chord{}, fmt.Errorf("the sequence %q has %d keys: a sequence is two keys, such as \"g g\"", name, len(parts))
	}
	var c Chord
	for i, part := range parts {
		msg, ok := Press(part)
		if !ok {
			return Chord{}, fmt.Errorf("the sequence %q: %w", name, Check(part))
		}
		if i == 0 {
			c.First = msg
		} else {
			c.Second = msg
		}
	}
	return c, nil
}

// Valid reports whether name names a key that a press can match, or a
// sequence of two.
func Valid(name string) bool { return Check(name) == nil }

// Check returns nil if name names a key a press can match, or a sequence
// of two such keys (see [ParseChord]), or else why it doesn't: an unknown
// name, modifiers out of order, or a capital letter with ctrl or alt,
// which terminals report as the small letter with shift.
func Check(name string) error {
	if Sequence(name) && strings.TrimSpace(name) != "" {
		_, err := ParseChord(name)
		return err
	}
	msg, mods, ok := parse(name)
	if !ok {
		return unknown(name)
	}
	k := tea.Key(msg)
	if k.Mod&^tea.ModShift != 0 && unicode.IsUpper(k.Code) {
		lower := tea.Key{Code: unicode.ToLower(k.Code), Mod: k.Mod | tea.ModShift}
		return fmt.Errorf("no terminal reports %q: write a capital with ctrl or alt as its small letter and shift, %q", name, lower.String())
	}
	got := msg.String()
	switch {
	case got == name:
		return nil
	case !slices.IsSorted(mods) && Check(got) == nil:
		return fmt.Errorf("the modifiers of %q go in the order %s: %q", name, modOrder, got)
	}
	return unknown(name)
}

// unknown is the error of a name that names no key.
func unknown(name string) error {
	return fmt.Errorf("unknown key %q, want a name such as r, R, ctrl+r, shift+tab, enter or space", name)
}

// parse returns a press of the key named name, the places in keyMods of
// its modifiers in the order it names them, and false for a name that
// names no key. The press may read otherwise than name, such as with its
// modifiers in another order.
func parse(name string) (tea.KeyPressMsg, []int, bool) {
	var k tea.Key
	var mods []int
	rest := name
	for {
		mod, after, ok := strings.Cut(rest, "+")
		i := slices.IndexFunc(keyMods, func(m modifier) bool { return m.name == mod })
		if !ok || i < 0 || after == "" || k.Mod&keyMods[i].mod != 0 {
			break
		}
		k.Mod |= keyMods[i].mod
		mods = append(mods, i)
		rest = after
	}
	if code, ok := keyCodes()[rest]; ok {
		k.Code = code
	} else {
		r, n := utf8.DecodeRuneInString(rest)
		if r == utf8.RuneError || n != len(rest) {
			return tea.KeyPressMsg{}, nil, false
		}
		k.Code = r
		if k.Mod&^tea.ModShift == 0 {
			k.Text = rest
		}
	}
	return tea.KeyPressMsg(k), mods, true
}
