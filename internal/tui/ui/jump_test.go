package ui

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
)

// TestJump checks how the help names the keys of the panes: a run of
// keys by its ends, others each, and none as a disabled binding that
// still says what it does.
func TestJump(t *testing.T) {
	keys := config.Keymap{"test": {"a": {"1"}, "b": {"2"}, "c": {"3"}, "d": {"4"}, "x": {"x"}, "f": {"ctrl+f"}}}
	panes := func(actions ...string) []key.Binding {
		out := make([]key.Binding, len(actions))
		for i, a := range actions {
			out[i] = Binding(keys, "test."+a, a)
		}
		return out
	}
	for _, tt := range []struct {
		name    string
		actions []string
		want    string
		keys    []string
	}{
		{"run", []string{"a", "b", "c", "d"}, "1-4", []string{"1", "2", "3", "4"}},
		{"gap", []string{"a", "none", "c", "d"}, "1/3/4", []string{"1", "3", "4"}},
		{"two", []string{"a", "b"}, "1/2", []string{"1", "2"}},
		{"not a run", []string{"a", "b", "x"}, "1/2/x", []string{"1", "2", "x"}},
		{"words", []string{"a", "b", "f"}, "1/2/^f", []string{"1", "2", "ctrl+f"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := Jump(panes(tt.actions...)...)
			if got.Help().Key != tt.want || !slices.Equal(got.Keys(), tt.keys) || !got.Enabled() {
				t.Errorf("Jump = %q %q, enabled %v, want %q %q", got.Help().Key, got.Keys(), got.Enabled(), tt.want, tt.keys)
			}
		})
	}
	none := Jump(panes("none", "other")...)
	if none.Enabled() || none.Help().Desc != "focus pane" {
		t.Errorf("Jump of unbound panes = enabled %v, %q, want disabled focus pane", none.Enabled(), none.Help().Desc)
	}
}

// TestUnboundBinding checks that an action without keys gives a disabled
// binding that still says what it does, for the help.
func TestUnboundBinding(t *testing.T) {
	b := Binding(config.Keymap{"test": {"a": {}}}, "test.a", "refresh")
	if b.Enabled() || len(b.Keys()) != 0 || b.Help().Key != "" || b.Help().Desc != "refresh" {
		t.Errorf("Binding = enabled %v, keys %q, help %+v", b.Enabled(), b.Keys(), b.Help())
	}
	if hint := OpenHint(Icons{}, b); hint != "" {
		t.Errorf("OpenHint = %q, want none", hint)
	}
	if y := Yield(b, key.NewBinding(key.WithKeys("r"), key.WithDisabled())); y.Enabled() || len(y.Keys()) != 0 {
		t.Errorf("Yield = %q, want it unbound", y.Keys())
	}
}
