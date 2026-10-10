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
	cfg, _, err := loadBase(writeConfig(t, "keys:\n  pulls:\n    merge: [ctrl+g]\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Keys.Of("pulls.merge"); !slices.Equal(got, []string{"ctrl+g"}) {
		t.Errorf("pulls.merge = %q, want [ctrl+g]", got)
	}
	if got := cfg.Keys.Of("pulls.close"); !slices.Equal(got, []string{"X"}) {
		t.Errorf("pulls.close = %q, want the default [X]", got)
	}

	for _, tt := range []struct{ name, file, want string }{
		{"action without a context", "keys:\n  quit: [x]\n", "line 2: keys.quit: unknown context"},
		{"unknown context", "keys:\n  pull:\n    merge: [M]\n", "line 3: keys.pull: unknown context"},
		{"context without actions", "keys:\n  pulls: [M]\n", "line 2: keys.pulls: want the actions of the context and their keys, such as keys.pulls.bottom"},
		{"action of another context", "keys:\n  issues:\n    merge: [M]\n", "line 3: keys.issues.merge: unknown action"},
		{"global action in a context", "keys:\n  pulls:\n    quit: [Q]\n", "line 3: keys.pulls.quit: quit is a global action, which no context may redefine: set keys.global.quit"},
		{"global key in a context", "keys:\n  pulls:\n    merge: [r]\n", "line 3: keys.pulls.merge: r is already keys.global.refresh"},
		{"context key made global", "keys:\n  global:\n    zoom: [M]\n", "line 3: keys.global.zoom: M is also keys.pulls.merge, keys.pull_modal.merge: unbind or rebind them there"},
		{"context key made global once", "keys:\n  global:\n    zoom: [B]\n", "line 3: keys.global.zoom: B is also keys.repo.history: unbind or rebind them there"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := loadBase(writeConfig(t, tt.file))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load = %v, want %q", err, tt.want)
			}
		})
	}

	// Two contexts other than the global one may share a key.
	if got, want := Default().Keys.Of("pulls.close"), Default().Keys.Of("issues.close"); !slices.Equal(got, want) {
		t.Errorf("pulls.close = %q and issues.close = %q, want the same key in both", got, want)
	}
}

// TestKeyValidation checks each rule of the keys' validation against the
// message it gives, with its line where the file sets what is wrong.
func TestKeyValidation(t *testing.T) {
	for _, tt := range []struct{ name, file, want string }{
		{"old flat key", "keys:\n  merge: [m]\n", "line 2: keys.merge: unknown context"},
		{"unknown context", "keys:\n  pull:\n    merge: [M]\n", "line 3: keys.pull: unknown context"},
		{"context without actions", "keys:\n  pulls: [m]\n", "line 2: keys.pulls: want the actions of the context and their keys, such as keys.pulls.bottom"},
		{"unknown action", "keys:\n  pulls:\n    mege: [M]\n", "line 3: keys.pulls.mege: unknown action"},
		{"removed action", "keys:\n  notifications:\n    read_all: [M]\n", "line 3: keys.notifications.read_all: removed, mark all notifications read with the :read all command, which asks first and needs no key"},
		{"global action elsewhere", "keys:\n  pulls:\n    refresh: [R]\n", "line 3: keys.pulls.refresh: refresh is a global action, which no context may redefine: set keys.global.refresh"},
		{"misspelt key", "keys:\n  pulls:\n    merge: [ctlr+m]\n", `line 3: keys.pulls.merge: unknown key "ctlr+m"`},
		{"ctrl+c in a context", "keys:\n  pulls:\n    merge: [ctrl+c]\n", "line 3: keys.pulls.merge: ctrl+c always quits and can't be bound"},
		{"ctrl+c in global", "keys:\n  global:\n    quit: [q, ctrl+c]\n", "line 3: keys.global.quit: ctrl+c always quits and can't be bound"},
		{"one key, two actions", "keys:\n  pulls:\n    sort: [f]\n", "keys.pulls: f is both filter and sort"},
		{"pane against global", "keys:\n  actions_jobs:\n    rerun_job: [r]\n", "line 3: keys.actions_jobs.rerun_job: r is already keys.global.refresh"},
		{"screen against global", "keys:\n  actions:\n    rerun_failed: [r]\n", "line 3: keys.actions.rerun_failed: r is already keys.global.refresh"},
		{"screen key against its panes", "keys:\n  actions:\n    cancel: [J]\n", "line 3: keys.actions.cancel: J is also keys.actions_jobs.rerun_job, keys.actions_log.rerun_job, keys.actions_annotations.rerun_job: unbind or rebind them there"},
		{"screen against global", "keys:\n  repo:\n    history: [o]\n", "line 3: keys.repo.history: o is already keys.global.open"},
		{"pane against its screen", "keys:\n  actions_log:\n    annotations: [X]\n", "line 3: keys.actions_log.annotations: X is already keys.actions.cancel, which works in every pane of the Actions modal"},
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
		"pulls.close":  {"X"},
		"pulls.filter": {"f"},
		"issues.close": {"X"},
	} {
		if got := cfg.Keys.Of(action); !slices.Equal(got, want) && (len(got) != 0 || len(want) != 0) {
			t.Errorf("%s = %q, want %q", action, got, want)
		}
	}
	if !slices.Contains(cfg.Keys.Actions(), "pulls.sort") {
		t.Error("an unbound action is no longer listed, which help needs to show it without a key")
	}
}

// TestCapturingContextsSkipTheChain checks that a capturing context may bind
// a key of the global context, and not one key on two of its actions.
func TestCapturingContextsSkipTheChain(t *testing.T) {
	k := Default().Keys
	k["confirm"] = map[string][]string{"yes": {"r"}, "no": {"ctrl+p"}}
	if err := k.validate(); err != nil {
		t.Errorf("validate() = %v, want a capturing context free to bind a key of global", err)
	}
	k["confirm"] = map[string][]string{"yes": {"enter"}, "no": {"enter"}}
	if err := k.validate(); err == nil || !strings.Contains(err.Error(), "keys.confirm: enter is both no and yes") {
		t.Errorf("validate() = %v, want one key on two actions refused", err)
	}
}

// TestTypingContextsRefusePrintableKeys checks that a context that types
// refuses a key it would type, naming the setting, unless its action is
// one that may be bound to such a key, and that a key with ctrl or alt, or
// a context that types nothing, is free to bind one.
func TestTypingContextsRefusePrintableKeys(t *testing.T) {
	for _, tt := range []struct{ name, file, want string }{
		{"letter in the finder", "keys:\n  finder:\n    reveal: [o]\n", "line 3: keys.finder.reveal: o would be typed into the finder; use a key that types nothing, such as one with ctrl or alt"},
		{"capital in the command line", "keys:\n  command_line:\n    complete: [T]\n", "line 3: keys.command_line.complete: T would be typed into the command line; use a key that types nothing, such as one with ctrl or alt"},
		{"symbol in the query of the search page", "keys:\n  search_query:\n    submit: [\"?\"]\n", "line 3: keys.search_query.submit: ? would be typed into the query; use a key that types nothing, such as one with ctrl or alt"},
		{"space in the picker", "keys:\n  picker:\n    choose: [space]\n", "line 3: keys.picker.choose: space would be typed into the picker; use a key that types nothing, such as one with ctrl or alt"},
		{"one of several keys", "keys:\n  prompt:\n    cancel: [esc, q]\n", "line 3: keys.prompt.cancel: q would be typed into the prompt; use a key that types nothing, such as one with ctrl or alt"},
		{"help's close, which may", "keys:\n  help:\n    close: [\"?\"]\n", ""},
		{"help's close, another letter", "keys:\n  help:\n    close: [x]\n", ""},
		{"help's cancel, which may not", "keys:\n  help:\n    cancel: [x]\n", "line 3: keys.help.cancel: x would be typed into the help; use a key that types nothing, such as one with ctrl or alt"},
		{"picker's toggle, which may", "keys:\n  picker:\n    toggle: [space]\n", ""},
		{"a ctrl key", "keys:\n  finder:\n    reveal: [ctrl+o]\n", ""},
		{"an alt key", "keys:\n  finder:\n    reveal: [alt+o]\n", ""},
		{"a context that types nothing", "keys:\n  confirm:\n    \"yes\": [Y]\n", ""},
		{"normal mode of a picker", "keys:\n  picker_normal:\n    insert: [I]\n", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := loadBase(writeConfig(t, tt.file))
			switch {
			case tt.want == "" && err != nil:
				t.Errorf("Load = %v, want none", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Errorf("Load = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestKeysOfWidgetsAreChecked checks that the keys of the filter form and
// the widgets that take every key are known, so that a key of the global
// context that one of them uses is refused, and a user's key for one of
// their actions is checked as the keys of every other context are.
func TestKeysOfWidgetsAreChecked(t *testing.T) {
	for _, tt := range []struct{ name, file, want string }{
		{"global key against the filter form", "keys:\n  global:\n    zoom: [delete]\n", "line 3: keys.global.zoom: delete is also keys.filter.clear, keys.actions_filter.clear: unbind or rebind them there"},
		{"form key against global", "keys:\n  filter:\n    toggle: [r]\n", "line 3: keys.filter.toggle: r is already keys.global.refresh"},
		{"one key on two actions of the form", "keys:\n  filter:\n    insert: [a]\n    append: [a]\n", "keys.filter: a is both append and insert"},
		{"unknown action of a widget", "keys:\n  finder:\n    browse: [ctrl+o]\n", "line 3: keys.finder.browse: unknown action"},
		{"ctrl+c in the command line", "keys:\n  command_line:\n    cancel: [ctrl+c]\n", "line 3: keys.command_line.cancel: ctrl+c always quits and can't be bound"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := loadBase(writeConfig(t, tt.file))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestKeysNeedLists checks that an action set to something that isn't a list
// of keys, such as a bare key, is refused naming the setting and its line,
// and that a list in a list is too.
func TestKeysNeedLists(t *testing.T) {
	for _, tt := range []struct{ name, file, want string }{
		{"a bare key", "keys:\n  pulls:\n    merge: m\n", "line 3: keys.pulls.merge: want a list of keys, such as [m]"},
		{"a bare key that needs quotes", "keys:\n  pulls:\n    merge: \"[\"\n", "line 3: keys.pulls.merge: want a list of keys, such as [x]"},
		{"a mapping", "keys:\n  pulls:\n    merge: {a: b}\n", "line 3: keys.pulls.merge: want a list of keys, such as [x]"},
		{"a list in a list", "keys:\n  pulls:\n    merge: [[m]]\n", "line 3: keys.pulls.merge: want a list of key names"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := loadBase(writeConfig(t, tt.file))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load = %v, want %q", err, tt.want)
			}
		})
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
	for _, pane := range []string{"pull_check_list", "pull_check_log", "pull_check_annotations", "pull_check_detail"} {
		if got, want := Chain(pane), []string{"global", "pull_modal", pane}; !slices.Equal(got, want) {
			t.Errorf("Chain(%s) = %q, want %q", pane, got, want)
		}
	}
	if _, ok := LookupContext("pull_checks"); ok {
		t.Error("pull_checks is a context, want it gone")
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

// TestChecksKeysArePanesOfThePullModal checks that the panes of the Checks
// step are panes of the modal of the pull request, so that none of their
// keys may be one of the modal's, and that the re-run key is set in each.
func TestChecksKeysArePanesOfThePullModal(t *testing.T) {
	cfg, _, err := loadBase(writeConfig(t, "keys:\n  pull_check_log:\n    annotations: [m]\n  pull_check_list:\n    rerun_failed: []\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Keys.Of("pull_check_log.annotations"); !slices.Equal(got, []string{"m"}) {
		t.Errorf("annotations = %q, want [m]", got)
	}
	if got := cfg.Keys.Of("pull_check_list.rerun_failed"); len(got) != 0 {
		t.Errorf("list rerun_failed = %q, want it unbound", got)
	}
	if got := cfg.Keys.Of("pull_check_detail.rerun_failed"); got != nil {
		t.Errorf("detail rerun_failed = %q, want none: what an app reported is never re-run", got)
	}
	for _, pane := range []string{"pull_check_log", "pull_check_annotations"} {
		if got := cfg.Keys.Of(pane + ".rerun_failed"); !slices.Equal(got, []string{"R"}) {
			t.Errorf("%s.rerun_failed = %q, want the default [R]", pane, got)
		}
	}
	for _, tt := range []struct{ file, want string }{
		{"keys:\n  pull_check_log:\n    rerun_failed: [M]\n", "line 3: keys.pull_check_log.rerun_failed: M is already keys.pull_modal.merge, which works in every pane of the Pull request modal"},
		{"keys:\n  pull_check_log:\n    prev_warning: [W]\n", "line 3: keys.pull_check_log.prev_warning: W is already keys.pull_modal.draft, which works in every pane of the Pull request modal"},
		{"keys:\n  pull_check_list:\n    rerun_failed: [C]\n", "line 3: keys.pull_check_list.rerun_failed: C is already keys.pull_modal.checks, which works in every pane of the Pull request modal"},
	} {
		if _, _, err := loadBase(writeConfig(t, tt.file)); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Load(%q) = %v, want %q", tt.file, err, tt.want)
		}
	}
}

// TestEmptyKeysDontPanic checks that an empty value under keys is read:
// an empty context changes nothing, an empty action is an error, and an
// empty unknown context is an unknown context.
func TestEmptyKeysDontPanic(t *testing.T) {
	for _, tt := range []struct{ name, file, want string }{
		{"empty context", "keys:\n  pulls:\n", ""},
		{"empty action", "keys:\n  pulls:\n    merge:\n", "line 3: keys.pulls.merge: want a list of keys, such as [m], or [] to unbind"},
		{"unknown empty context", "keys:\n  nope:\n", "line 2: keys.nope: unknown context"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _, err := loadBase(writeConfig(t, tt.file))
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Load = %v, want none", err)
				}
				if got := cfg.Keys.Of("pulls.merge"); !slices.Equal(got, []string{"M"}) {
					t.Errorf("pulls.merge = %q, want the default", got)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load = %v, want %q", err, tt.want)
			}
		})
	}
}

// The log of the checks has no key for the previous warning by default, as
// the pull request modal uses W to draft; the Actions log, which is not in
// that modal, keeps it.
func TestCheckLogWarningHasNoDefaultKey(t *testing.T) {
	keys := Default().Keys
	for action, want := range map[string][]string{
		"pull_modal.draft": {"W"}, "pull_check_log.prev_warning": {}, "actions_log.prev_warning": {"W"},
	} {
		if got := keys.Of(action); !slices.Equal(got, want) && (len(got) != 0 || len(want) != 0) {
			t.Errorf("%s = %q, want %q", action, got, want)
		}
	}
}
