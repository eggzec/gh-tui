package config

import (
	"slices"
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
		{"misspelt modifier", []string{"r", "ctlr+r"}, `keys.global.refresh: unknown key "ctlr+r"`},
		{"misspelt name", []string{"escape"}, `keys.global.refresh: unknown key "escape"`},
		{"word", []string{"refresh"}, `keys.global.refresh: unknown key "refresh"`},
		{"modifier alone", []string{"ctrl"}, `keys.global.refresh: unknown key "ctrl"`},
		{"modifier without key", []string{"ctrl+"}, `keys.global.refresh: unknown key "ctrl+"`},
		{"shifted letter", []string{"shift+a"}, `keys.global.refresh: unknown key "shift+a"`},
		{"upper case name", []string{"Enter"}, `keys.global.refresh: unknown key "Enter"`},
		{"two letters", []string{"rr"}, `keys.global.refresh: unknown key "rr"`},
		{"space typed", []string{" "}, `keys.global.refresh: unknown key " "`},
		{"empty", []string{""}, "keys.global.refresh: empty key"},
		{"capital with ctrl", []string{"ctrl+R"}, `keys.global.refresh: no terminal reports "ctrl+R": write a capital with ctrl or alt as its small letter and shift, "ctrl+shift+r"`},
		{"capital with alt", []string{"alt+A"}, `keys.global.refresh: no terminal reports "alt+A": write a capital with ctrl or alt as its small letter and shift, "alt+shift+a"`},
		{"capital with shift", []string{"shift+R"}, `keys.global.refresh: unknown key "shift+R"`},
		{"modifiers out of order", []string{"alt+ctrl+a"}, `keys.global.refresh: the modifiers of "alt+ctrl+a" go in the order ctrl, alt, shift, meta, hyper, super: "ctrl+alt+a"`},
		{"shift before ctrl", []string{"shift+ctrl+r"}, `keys.global.refresh: the modifiers of "shift+ctrl+r" go in the order ctrl, alt, shift, meta, hyper, super: "ctrl+shift+r"`},
		{"modifier twice", []string{"ctrl+ctrl+a"}, `keys.global.refresh: unknown key "ctrl+ctrl+a"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKeys(ContextGlobal, "refresh", tt.keys)
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
	for ctx, actions := range Default().Keys {
		for name, keys := range actions {
			if err := validateKeys(ctx, name, keys); err != nil {
				t.Error(err)
			}
		}
	}
}

// TestUnbindInFile checks that keys.<context>.X: [] in the file loads, unbinding X
// and leaving the other actions their defaults, and that a misspelt key
// in the file fails the load.
func TestUnbindInFile(t *testing.T) {
	cfg, _, err := loadBase(writeConfig(t, "keys:\n  repo:\n    star: []\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Keys.Of(ActionStar); len(got) != 0 {
		t.Errorf("star = %q, want no keys", got)
	}
	if got := cfg.Keys.Of(ActionHelp); len(got) == 0 {
		t.Error("help lost its default keys")
	}

	_, _, err = loadBase(writeConfig(t, "keys:\n  repo:\n    star: [ctlr+s]\n"))
	if err == nil || !strings.Contains(err.Error(), `line 3: keys.repo.star: unknown key "ctlr+s"`) {
		t.Errorf("Load = %v, want the misspelt key named", err)
	}
}

// TestKeysByContext checks that the file sets keys under their context,
// each action replacing only its own keys, and refuses an action set
// outside a context, an unknown context, a global action set in another
// context, and a key a context shares with a global action, each with its
// line.
func TestKeysByContext(t *testing.T) {
	cfg, _, err := loadBase(writeConfig(t, "keys:\n  pulls:\n    merge: [M]\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Keys.Of("pulls.merge"); !slices.Equal(got, []string{"M"}) {
		t.Errorf("pulls.merge = %q, want [M]", got)
	}
	if got := cfg.Keys.Of("pulls.close"); !slices.Equal(got, []string{"x"}) {
		t.Errorf("pulls.close = %q, want the default [x]", got)
	}

	for _, tt := range []struct{ name, file, want string }{
		{"action without a context", "keys:\n  quit: [x]\n", "line 2: keys.quit: unknown context"},
		{"unknown context", "keys:\n  pull:\n    merge: [M]\n", "line 3: keys.pull: unknown context"},
		{"context without actions", "keys:\n  pulls: [M]\n", "line 2: keys.pulls: want the actions of the context and their keys, such as keys.pulls.checks"},
		{"action of another context", "keys:\n  issues:\n    merge: [M]\n", "line 3: keys.issues.merge: unknown action"},
		{"global action in a context", "keys:\n  pulls:\n    quit: [Q]\n", "line 3: keys.pulls.quit: quit is a global action, which no context may redefine: set keys.global.quit"},
		{"global key in a context", "keys:\n  pulls:\n    merge: [r]\n", "line 3: keys.pulls.merge: r is already keys.global.refresh"},
		{"context key made global", "keys:\n  global:\n    zoom: [m]\n", "line 3: keys.global.zoom: m is also keys.pulls.merge, keys.notifications.read, keys.pull_modal.merge: unbind or rebind them there"},
		{"context key made global once", "keys:\n  global:\n    zoom: [B]\n", "keys.repo.history: B is already keys.global.zoom"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := loadBase(writeConfig(t, tt.file))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load = %v, want %q", err, tt.want)
			}
		})
	}

	// Two contexts other than the global one may share a key.
	if got, want := Default().Keys.Of("pulls.merge"), Default().Keys.Of("notifications.read"); !slices.Equal(got, want) {
		t.Errorf("pulls.merge = %q and notifications.read = %q, want the same key in both", got, want)
	}
}

// TestKeyValidation checks each rule of the keys' validation against the
// message it gives, with its line where the file sets what is wrong.
func TestKeyValidation(t *testing.T) {
	for _, tt := range []struct{ name, file, want string }{
		{"old flat key", "keys:\n  merge: [m]\n", "line 2: keys.merge: unknown context"},
		{"unknown context", "keys:\n  pull:\n    merge: [M]\n", "line 3: keys.pull: unknown context"},
		{"context without actions", "keys:\n  pulls: [m]\n", "line 2: keys.pulls: want the actions of the context and their keys, such as keys.pulls.checks"},
		{"unknown action", "keys:\n  pulls:\n    mege: [M]\n", "line 3: keys.pulls.mege: unknown action"},
		{"global action elsewhere", "keys:\n  pulls:\n    refresh: [R]\n", "line 3: keys.pulls.refresh: refresh is a global action, which no context may redefine: set keys.global.refresh"},
		{"misspelt key", "keys:\n  pulls:\n    merge: [ctlr+m]\n", `line 3: keys.pulls.merge: unknown key "ctlr+m"`},
		{"ctrl+c in a context", "keys:\n  pulls:\n    merge: [ctrl+c]\n", "line 3: keys.pulls.merge: ctrl+c always quits and can't be bound"},
		{"ctrl+c in global", "keys:\n  global:\n    quit: [q, ctrl+c]\n", "line 3: keys.global.quit: ctrl+c always quits and can't be bound"},
		{"one key, two actions", "keys:\n  pulls:\n    sort: [f]\n", "keys.pulls: f is both filter and sort"},
		{"pane against global", "keys:\n  actions:\n    rerun_failed: [r]\n", "line 3: keys.actions.rerun_failed: r is already keys.global.refresh"},
		{"screen against global", "keys:\n  repo:\n    history: [o]\n", "line 3: keys.repo.history: o is already keys.global.open"},
		{"pane against its screen", "keys:\n  actions_log:\n    annotations: [x]\n", "line 3: keys.actions_log.annotations: x is already keys.actions.cancel, which works in every pane of the Actions modal"},
		{"pane of a screen against its screen", "keys:\n  pulls:\n    merge: [B]\n", "line 3: keys.pulls.merge: B is already keys.repo.history, which works in every pane of the Repository screen"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := loadBase(writeConfig(t, tt.file))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestKeyOverrides checks that a file replaces the keys of the actions it
// lists and no others, whatever else its context has, and that [] unbinds.
func TestKeyOverrides(t *testing.T) {
	cfg, _, err := loadBase(writeConfig(t, "keys:\n  pulls:\n    merge: [ctrl+g]\n    sort: []\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for action, want := range map[string][]string{
		"pulls.merge":  {"ctrl+g"},
		"pulls.sort":   {},
		"pulls.close":  {"x"},
		"pulls.filter": {"f"},
		"issues.close": {"x"},
	} {
		if got := cfg.Keys.Of(action); !slices.Equal(got, want) && (len(got) != 0 || len(want) != 0) {
			t.Errorf("%s = %q, want %q", action, got, want)
		}
	}
	if !slices.Contains(cfg.Keys.Actions(), "pulls.sort") {
		t.Error("an unbound action is no longer listed, which help needs to show it without a key")
	}
}

// TestCapturingContextsSkipTheChain checks, with a context of keys that
// takes every key while it is open, that it may bind keys of the global
// context and of other contexts, and is held only to one key to one
// action.
func TestCapturingContextsSkipTheChain(t *testing.T) {
	old := actions
	t.Cleanup(func() { actions = old })
	known := Default().Keys
	known["search_query"] = map[string][]string{"submit": {"enter"}, "cancel": {"esc"}}
	actions = func() Keymap { return known }

	k := Default().Keys
	k["search_query"] = map[string][]string{"submit": {"r"}, "cancel": {"ctrl+p"}}
	if err := k.validate(); err != nil {
		t.Errorf("validate() = %v, want a capturing context free to bind a key of global", err)
	}
	k["search_query"] = map[string][]string{"submit": {"enter"}, "cancel": {"enter"}}
	if err := k.validate(); err == nil || !strings.Contains(err.Error(), "keys.search_query: enter is both cancel and submit") {
		t.Errorf("validate() = %v, want one key on two actions refused", err)
	}
}

// TestDefaultContexts checks that the context table is whole: every pane
// has a screen or modal as its parent, and the chain of each context is
// global, its parent and itself.
func TestDefaultContexts(t *testing.T) {
	byName := map[string]Context{}
	for _, c := range Contexts() {
		if _, dup := byName[c.Name]; dup {
			t.Errorf("the context %s is listed twice", c.Name)
		}
		byName[c.Name] = c
	}
	for _, c := range Contexts() {
		switch c.Reach {
		case ReachPane:
			if p, ok := byName[c.Parent]; !ok || p.Reach != ReachScreen {
				t.Errorf("the pane %s has %q as its parent, want a screen or modal", c.Name, c.Parent)
			}
		case ReachGlobal, ReachScreen:
			if c.Parent != "" {
				t.Errorf("%s has the parent %s, want none", c.Name, c.Parent)
			}
		case ReachCapture:
		}
	}
	if got, want := Chain("actions_log"), []string{"global", "actions", "actions_log"}; !slices.Equal(got, want) {
		t.Errorf("Chain(actions_log) = %q, want %q", got, want)
	}
	if got, want := Chain("notifications"), []string{"global", "notifications"}; !slices.Equal(got, want) {
		t.Errorf("Chain(notifications) = %q, want %q", got, want)
	}
	if got, want := Chain("command_line"), []string{"command_line"}; !slices.Equal(got, want) {
		t.Errorf("Chain(command_line) = %q, want %q", got, want)
	}
	if got := Chain("nowhere"); got != nil {
		t.Errorf("Chain(nowhere) = %q, want none", got)
	}
}
