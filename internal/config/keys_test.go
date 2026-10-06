package config

import (
	"strings"
	"testing"
)

// TestValidateKeyNames checks that the keys of an action are names a key
// press has, that a misspelt one is refused with the action and the name,
// and that no keys at all unbind the action.
func TestValidateKeyNames(t *testing.T) {
	for _, tt := range []struct {
		name string
		keys []string
		want string
	}{
		{"letter", []string{"r"}, ""},
		{"capital", []string{"R"}, ""},
		{"symbol", []string{"?", "/", "+", "-", ":", "@", "[", "]"}, ""},
		{"modifiers", []string{"ctrl+r", "alt+x", "shift+tab", "ctrl+shift+up", "ctrl+shift+r", "ctrl+alt+a", "ctrl++"}, ""},
		{"named", []string{"enter", "esc", "space", "tab", "backspace", "pgup", "pgdown", "home", "end", "delete", "insert"}, ""},
		{"function keys", []string{"f1", "f12", "f13", "f24"}, ""},
		{"unbound", []string{}, ""},
		{"unbound as nil", nil, ""},
		{"misspelt modifier", []string{"r", "ctlr+r"}, `keys.refresh: unknown key "ctlr+r"`},
		{"misspelt name", []string{"escape"}, `keys.refresh: unknown key "escape"`},
		{"word", []string{"refresh"}, `keys.refresh: unknown key "refresh"`},
		{"modifier alone", []string{"ctrl"}, `keys.refresh: unknown key "ctrl"`},
		{"modifier without key", []string{"ctrl+"}, `keys.refresh: unknown key "ctrl+"`},
		{"shifted letter", []string{"shift+a"}, `keys.refresh: unknown key "shift+a"`},
		{"upper case name", []string{"Enter"}, `keys.refresh: unknown key "Enter"`},
		{"two letters", []string{"rr"}, `keys.refresh: unknown key "rr"`},
		{"space typed", []string{" "}, `keys.refresh: unknown key " "`},
		{"empty", []string{""}, "keys.refresh: empty key"},
		{"capital with ctrl", []string{"ctrl+R"}, `keys.refresh: no terminal reports "ctrl+R": write a capital with ctrl or alt as its small letter and shift, "ctrl+shift+r"`},
		{"capital with alt", []string{"alt+A"}, `keys.refresh: no terminal reports "alt+A": write a capital with ctrl or alt as its small letter and shift, "alt+shift+a"`},
		{"capital with shift", []string{"shift+R"}, `keys.refresh: unknown key "shift+R"`},
		{"modifiers out of order", []string{"alt+ctrl+a"}, `keys.refresh: the modifiers of "alt+ctrl+a" go in the order ctrl, alt, shift, meta, hyper, super: "ctrl+alt+a"`},
		{"shift before ctrl", []string{"shift+ctrl+r"}, `keys.refresh: the modifiers of "shift+ctrl+r" go in the order ctrl, alt, shift, meta, hyper, super: "ctrl+shift+r"`},
		{"modifier twice", []string{"ctrl+ctrl+a"}, `keys.refresh: unknown key "ctrl+ctrl+a"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKeys(ActionRefresh, tt.keys)
			switch {
			case tt.want == "" && err != nil:
				t.Errorf("validateKeys(%q) = %v, want nil", tt.keys, err)
			case tt.want != "" && (err == nil || !strings.HasPrefix(err.Error(), tt.want)):
				t.Errorf("validateKeys(%q) = %v, want %q", tt.keys, err, tt.want)
			}
		})
	}
}

// TestDefaultKeyNames checks that every key default.yaml binds is a name
// the validator takes.
func TestDefaultKeyNames(t *testing.T) {
	for action, keys := range Default().Keys {
		if err := validateKeys(action, keys); err != nil {
			t.Error(err)
		}
	}
}

// TestUnbindInFile checks that keys.X: [] in the file loads, unbinding X
// and leaving the other actions their defaults, and that a misspelt key
// in the file fails the load.
func TestUnbindInFile(t *testing.T) {
	cfg, _, err := loadBase(writeConfig(t, "keys:\n  star: []\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Keys[ActionStar]; len(got) != 0 {
		t.Errorf("star = %q, want no keys", got)
	}
	if got := cfg.Keys[ActionHelp]; len(got) == 0 {
		t.Error("help lost its default keys")
	}

	_, _, err = loadBase(writeConfig(t, "keys:\n  star: [ctlr+s]\n"))
	if err == nil || !strings.Contains(err.Error(), `keys.star: unknown key "ctlr+s"`) {
		t.Errorf("Load = %v, want the misspelt key named", err)
	}
}
