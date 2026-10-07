package keymap_test

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

func TestJoin(t *testing.T) {
	quit := key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "close"))
	dismiss := key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close"))
	off := key.NewBinding(key.WithDisabled())
	unbound := key.NewBinding(key.WithHelp("", "back"), key.WithDisabled())
	held := key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "close"))
	held.SetEnabled(false)

	tests := []struct {
		name    string
		in      []key.Binding
		keys    []string
		help    key.Help
		enabled bool
	}{
		{"both", []key.Binding{quit, dismiss}, []string{"q", "esc"}, key.Help{Key: "q/esc", Desc: "close"}, true},
		{"one switched off", []key.Binding{off, dismiss}, []string{"esc"}, key.Help{Key: "esc", Desc: "close"}, true},
		{"the keys of a disabled one don't work", []key.Binding{held, dismiss}, []string{"esc"}, key.Help{Key: "esc", Desc: "close"}, true},
		{"none enabled keeps the words", []key.Binding{off, unbound}, nil, key.Help{Key: "", Desc: "back"}, false},
		{"all held keep their keys", []key.Binding{held}, []string{"x"}, key.Help{Key: "x", Desc: "close"}, false},
		{"same key once", []key.Binding{quit, quit}, []string{"q"}, key.Help{Key: "q", Desc: "close"}, true},
		{"none", nil, nil, key.Help{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := keymap.Join(tt.in...)
			if !slices.Equal(got.Keys(), tt.keys) || got.Help() != tt.help || got.Enabled() != tt.enabled {
				t.Errorf("Join = keys %v, help %+v, enabled %v; want %v, %+v, %v",
					got.Keys(), got.Help(), got.Enabled(), tt.keys, tt.help, tt.enabled)
			}
		})
	}
}
