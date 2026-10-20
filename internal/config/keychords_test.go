package config

import (
	"slices"
	"strings"
	"testing"
)

// TestKeySequences checks that a sequence of two keys is a valid key, that
// a single key may share its first key, and that what is not a sequence of
// two valid keys, or sits where keys are typed, is refused.
func TestKeySequences(t *testing.T) {
	for _, tt := range []struct{ name, file, want string }{
		{"two keys", "keys:\n  dashboard_repos:\n    bottom: [\"g g\"]\n    top: [home]\n", ""},
		{"a key and a sequence that starts with it", "keys:\n  dashboard_calendar:\n    top: [g, home]\n    bottom: [\"g g\"]\n", ""},
		{"keys with modifiers", "keys:\n  dashboard_repos:\n    bottom: [\"ctrl+w x\"]\n", ""},
		{"three keys", "keys:\n  dashboard_repos:\n    bottom: [\"g g g\"]\n", `line 3: keys.dashboard_repos.bottom: the sequence "g g g" has 3 keys: a sequence is two keys`},
		{"an empty key", "keys:\n  dashboard_repos:\n    bottom: [\"g  \"]\n", `keys.dashboard_repos.bottom: the sequence "g  " has an empty key`},
		{"a leading space", "keys:\n  dashboard_repos:\n    bottom: [\" g\"]\n", `the sequence " g" has an empty key`},
		{"an unknown key", "keys:\n  dashboard_repos:\n    bottom: [\"g nope\"]\n", `keys.dashboard_repos.bottom: the sequence "g nope": unknown key "nope"`},
		{"ctrl+c in a sequence", "keys:\n  dashboard_repos:\n    bottom: [\"g ctrl+c\"]\n", "keys.dashboard_repos.bottom: ctrl+c always quits and can't be bound"},
		{"in a typing context", "keys:\n  command_line:\n    cancel: [\"g g\"]\n", `keys.command_line.cancel: "g g" is a sequence of keys, which the command line can't take`},
		{"two equal sequences", "keys:\n  dashboard_repos:\n    top: [\"g g\"]\n    bottom: [\"g g\"]\n", "keys.dashboard_repos: g g is both bottom and top"},
		{"a sequence against global", "keys:\n  global:\n    zoom: [\"g g\"]\n  dashboard_repos:\n    bottom: [\"g g\"]\n", "keys.global.zoom: g g is also keys.dashboard_repos.bottom"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := loadBase(writeConfig(t, tt.file))
			switch {
			case tt.want == "" && err != nil:
				t.Errorf("Load = %v, want no error", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Errorf("Load = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestKeymapSequences checks that a context's sequences come back as
// presses, by action, and that single keys do not.
func TestKeymapSequences(t *testing.T) {
	k := Keymap{"dashboard_repos": {
		"top":    {"g", "home"},
		"bottom": {"g g", "ctrl+w x"},
	}}
	got := k.Sequences("dashboard_repos")
	if len(got) != 1 || len(got["dashboard_repos.bottom"]) != 2 {
		t.Fatalf("Sequences = %v, want two for dashboard_repos.bottom only", got)
	}
	c := got["dashboard_repos.bottom"][1]
	if c.First.String() != "ctrl+w" || c.Second.String() != "x" {
		t.Errorf("second sequence = %q %q, want ctrl+w x", c.First.String(), c.Second.String())
	}
	if !slices.Equal(k["dashboard_repos"]["top"], []string{"g", "home"}) {
		t.Error("Sequences changed the keymap")
	}
}
