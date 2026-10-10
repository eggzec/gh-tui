package keymap_test

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

func TestActionPress(t *testing.T) {
	merge := keymap.Record(key.NewBinding(key.WithKeys("M")), "pulls.merge")
	unbound := keymap.Record(keymap.Unbound(key.NewBinding()), "pulls.close")
	held := keymap.Record(key.NewBinding(key.WithKeys("X")), "pulls.reopen")
	held.SetEnabled(false)
	other := key.NewBinding(key.WithKeys("o"))

	tests := []struct {
		name string
		msg  tea.KeyPressMsg
		in   []key.Binding
		want bool
	}{
		{"its key", tea.KeyPressMsg{Code: 'M', Text: "M"}, []key.Binding{merge}, true},
		{"another key", tea.KeyPressMsg{Code: 'x', Text: "x"}, []key.Binding{merge}, false},
		{"the action of a bound binding", keymap.ActionPress("pulls.merge"), []key.Binding{other, merge}, true},
		{"the action of an unbound binding", keymap.ActionPress("pulls.close"), []key.Binding{unbound}, true},
		{"the action of a held binding with keys", keymap.ActionPress("pulls.reopen"), []key.Binding{held}, false},
		{"another action", keymap.ActionPress("pulls.merge"), []key.Binding{unbound, held}, false},
		{"a binding of no action", keymap.ActionPress("pulls.merge"), []key.Binding{other}, false},
	}
	for _, tt := range tests {
		if got := keymap.Matches(tt.msg, tt.in...); got != tt.want {
			t.Errorf("%s: Matches = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPressed(t *testing.T) {
	if a, ok := keymap.Pressed(keymap.ActionPress("pulls.merge")); !ok || a != "pulls.merge" {
		t.Errorf("Pressed = %q, %v, want pulls.merge", a, ok)
	}
	if a, ok := keymap.Pressed(tea.KeyPressMsg{Code: 'a', Text: "a"}); ok {
		t.Errorf("Pressed(a) = %q, want a key", a)
	}
	if _, ok := keymap.Pressed("pulls.merge"); ok {
		t.Error("Pressed(a string) = ok")
	}
}
