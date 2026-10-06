package keyname

import "testing"

// TestPress checks that a press of a key's name reads as that name, so
// that the bindings that hold the name match it.
func TestPress(t *testing.T) {
	for _, name := range []string{
		"r", "R", "?", "+", "1", "ctrl+r", "ctrl+shift+r", "ctrl+alt+a", "ctrl++", "alt+enter", "shift+tab", "ctrl+shift+up",
		"enter", "esc", "space", "tab", "backspace", "up", "pgdown", "f1", "f13", "f63", "delete", "é",
	} {
		msg, ok := Press(name)
		if !ok || msg.String() != name {
			t.Errorf("Press(%q) = %q, %v, want the key", name, msg.String(), ok)
		}
		if !Valid(name) {
			t.Errorf("Valid(%q) = false", name)
		}
	}
}

// TestPressRefuses checks the names that no press reads as.
func TestPressRefuses(t *testing.T) {
	for _, name := range []string{
		"", " ", "rr", "ctrl", "ctrl+", "ctlr+r", "escape", "Enter", "shift+a", "f0", "f64", "\xff",
		"ctrl+R", "alt+A", "alt+ctrl+a", "ctrl+ctrl+a",
	} {
		if msg, ok := Press(name); ok {
			t.Errorf("Press(%q) = %q, want none", name, msg.String())
		}
		if Valid(name) {
			t.Errorf("Valid(%q) = true", name)
		}
	}
}

// TestSelectIsNoKey checks that "select" names no key, but the config's
// action.
func TestSelectIsNoKey(t *testing.T) {
	if Valid("select") {
		t.Error(`Valid("select") = true, want false`)
	}
}
