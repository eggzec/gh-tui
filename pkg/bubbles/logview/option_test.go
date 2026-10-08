package logview

import (
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

func TestOptions(t *testing.T) {
	tests := []struct {
		name          string
		keys          []string
		wantWrap      bool
		wantNumbers   bool
		wantTimes     TimeMode
		wantNote      string
		wantCapturing bool
		wantClose     bool
	}{
		{name: "minus waits for an option", keys: []string{"-"}, wantNumbers: true, wantCapturing: true},
		{name: "S wraps long lines", keys: []string{"-", "S"}, wantWrap: true, wantNumbers: true, wantNote: "Wrap long lines"},
		{name: "S again chops them", keys: []string{"-", "S", "-", "S"}, wantNumbers: true, wantNote: "Chop long lines"},
		{name: "N hides the line numbers", keys: []string{"-", "N"}, wantNote: "Hide line numbers"},
		{name: "N again shows them", keys: []string{"-", "N", "-", "N"}, wantNumbers: true, wantNote: "Show line numbers"},
		{name: "T turns the times on", keys: []string{"-", "T"}, wantNumbers: true, wantTimes: TimeRelative,
			wantNote: "Show times since the section started"},
		{name: "T again shows the times of day", keys: []string{"-", "T", "-", "T"}, wantNumbers: true,
			wantTimes: TimeAbsolute, wantNote: "Show times of day"},
		{name: "T a third time hides them", keys: []string{"-", "T", "-", "T", "-", "T"}, wantNumbers: true,
			wantNote: "Hide times"},
		{name: "an unknown option says so", keys: []string{"-", "x"}, wantNumbers: true, wantNote: "No such option: -x"},
		{name: "s, t and # are no keys", keys: []string{"s", "t", "#"}, wantNumbers: true},
		{name: "plus and equals are no keys", keys: []string{"+", "="}, wantNumbers: true},
		{name: "esc cancels", keys: []string{"-", "esc"}, wantNumbers: true},
		{name: "then closes", keys: []string{"-", "esc", "esc"}, wantNumbers: true, wantClose: true},
		{name: "q is no option", keys: []string{"-", "q"}, wantNumbers: true, wantNote: "No such option: -q"},
		{name: "the note goes at the next key", keys: []string{"-", "S", "j"}, wantWrap: true, wantNumbers: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, WithSize(80, 12))
			m, msg := keys(t, m, tt.keys...)
			if m.Wrap() != tt.wantWrap || m.LineNumbers() != tt.wantNumbers || m.TimeMode() != tt.wantTimes {
				t.Errorf("wrap %v, line numbers %v, times %v; want %v, %v, %v",
					m.Wrap(), m.LineNumbers(), m.TimeMode(), tt.wantWrap, tt.wantNumbers, tt.wantTimes)
			}
			if m.flash != tt.wantNote {
				t.Errorf("note %q, want %q", m.flash, tt.wantNote)
			}
			rows := strings.Split(plain(m), "\n")
			status := rows[len(rows)-1]
			if tt.wantNote != "" && !strings.HasPrefix(status, tt.wantNote) {
				t.Errorf("status %q doesn't start with the note", status)
			}
			if m.Capturing() != tt.wantCapturing || m.ChoosingOption() != tt.wantCapturing || m.Searching() {
				t.Errorf("capturing %v, choosing %v, searching %v; want %v, %v, false",
					m.Capturing(), m.ChoosingOption(), m.Searching(), tt.wantCapturing, tt.wantCapturing)
			}
			if tt.wantCapturing && !strings.HasPrefix(status, "-") {
				t.Errorf("status %q doesn't show the -", status)
			}
			if _, ok := msg.(CloseMsg); ok != tt.wantClose {
				t.Errorf("sent %#v, want close %v", msg, tt.wantClose)
			}
		})
	}
}

// The keys that used to change the view's options on their own do nothing.
func TestOldOptionKeysDoNothing(t *testing.T) {
	m := open(t, WithSize(80, 24))
	rows := shownRows(m)
	m, _ = keys(t, m, "s", "t", "#", "+", "=")
	if m.Wrap() || m.TimeMode() != TimeHidden || !m.LineNumbers() || !slices.Equal(shownRows(m), rows) {
		t.Errorf("wrap %v, times %v, line numbers %v after the old keys; want the view as it was",
			m.Wrap(), m.TimeMode(), m.LineNumbers())
	}
	// A lone minus only waits for an option, and folds nothing.
	m, _ = keys(t, m, "-")
	if !slices.Equal(shownRows(m), rows) || !m.ChoosingOption() {
		t.Error("- folded something, or didn't wait for an option")
	}
}

// Follow still works, and the search prompt types - and *.
func TestOptionAndTyping(t *testing.T) {
	m := open(t, WithSize(80, 24))
	was := m.Follow()
	m, _ = keys(t, m, "F")
	if m.Follow() == was {
		t.Error("F no longer follows")
	}
	m = typeText(t, m, "/")
	m = typeText(t, m, "-*")
	if got := m.input.Value(); got != "-*" {
		t.Errorf("the prompt holds %q, want -*", got)
	}
	if m.ChoosingOption() {
		t.Error("- in the prompt waits for an option")
	}
}

// A log view takes the keys that name an option from its option key map.
func TestOptionKeysAreRebound(t *testing.T) {
	rebound := func(action string) []string {
		if action == "log_option.chop" {
			return []string{"W"}
		}
		return lookup.Of(action)
	}
	m := open(t, WithSize(80, 12), WithKeyMap(NewKeyMap(keymap.Func(rebound))))
	m, _ = keys(t, m, "-", "S")
	if m.Wrap() || !strings.HasPrefix(m.flash, noteNoOption) {
		t.Errorf("-S: wrap %v, note %q, want S to name no option", m.Wrap(), m.flash)
	}
	m, _ = keys(t, m, "-", "W")
	if !m.Wrap() {
		t.Error("-W didn't wrap the lines")
	}
}

// With the option key unbound, or its option keys, nothing changes.
func TestNoOptionKeys(t *testing.T) {
	bare := func(action string) []string {
		if strings.HasPrefix(action, "log_option.") {
			return nil
		}
		return lookup.Of(action)
	}
	m := open(t, WithSize(80, 12), WithKeyMap(NewKeyMap(keymap.Func(bare))))
	m, _ = keys(t, m, "-", "S", "esc")
	if m.Wrap() || m.Capturing() {
		t.Errorf("wrap %v, capturing %v, want the option key to take one key and change nothing", m.Wrap(), m.Capturing())
	}
	if got := NewKeyMap(keymap.Func(bare)).Option.Help().Desc; got != "option" {
		t.Errorf("option help is %q, want %q", got, "option")
	}
	none := func(action string) []string {
		if action == "option" {
			return nil
		}
		return lookup.Of(action)
	}
	m = open(t, WithSize(80, 12), WithKeyMap(NewKeyMap(keymap.Func(none))))
	m, _ = keys(t, m, "-", "S")
	if m.Wrap() || m.Capturing() {
		t.Error("an unbound option key still takes keys")
	}
}

// The option key's help lists the keys that name an option, and option
// mode's help lists every option action with its key.
func TestOptionHelpFollowsKeys(t *testing.T) {
	if got := testKeys(t).Option.Help().Desc; got != "option: S N T" {
		t.Errorf("option help is %q, want the default keys", got)
	}
	rebound := func(action string) []string {
		switch action {
		case "log_option.chop":
			return []string{"W"}
		case "log_option.timestamps":
			return nil
		}
		return lookup.Of(action)
	}
	m := open(t, WithSize(80, 12), WithKeyMap(NewKeyMap(keymap.Func(rebound))))
	if got := m.keys.Option.Help().Desc; got != "option: W N" {
		t.Errorf("option help is %q, want W N", got)
	}
	m, _ = keys(t, m, "-")
	want := map[string]string{"chop or wrap long lines": "W", "line numbers": "N", "timestamps": "", "cancel": "esc"}
	got := map[string]string{}
	if first := m.ShortHelp()[0]; first.Help().Desc != "cancel" {
		t.Errorf("option mode's help starts with %q, want cancel first", first.Help().Desc)
	}
	for _, b := range m.ShortHelp() {
		if b.Enabled() {
			got[b.Help().Desc] = b.Help().Key
		} else {
			got[b.Help().Desc] = ""
		}
	}
	for desc, k := range want {
		if g, ok := got[desc]; !ok || g != k {
			t.Errorf("option mode help lists %q with key %q (listed %v), want %q", desc, g, ok, k)
		}
	}
	// Only the keys of the option are enabled in the full help.
	for _, g := range m.FullHelp() {
		for _, b := range g {
			if b.Enabled() && !slices.Contains([]string{"W", "N", "esc"}, b.Help().Key) {
				t.Errorf("full help lists %q while an option is awaited", b.Help().Key)
			}
		}
	}
}

// Toggle all, the log's star, does nothing in a log without folds.
func TestToggleAllWithoutFolds(t *testing.T) {
	m := view(t, plainLines("a", "b", "c"), WithSize(40, 6))
	rows := shownRows(m)
	m, _ = keys(t, m, "*", "*")
	if got := shownRows(m); !slices.Equal(got, rows) {
		t.Errorf("rows %q after * on a log without folds, want %q", got, rows)
	}
}

// The note on the last option goes when the view loses or takes the focus,
// so it never stays on the status line of a view nobody presses keys in.
func TestOptionNoteGoesWithFocus(t *testing.T) {
	m := open(t, WithSize(80, 12))
	m, _ = keys(t, m, "-", "S")
	if m.flash == "" {
		t.Fatal("-S left no note")
	}
	m.Blur()
	if m.flash != "" || strings.Contains(plain(m), noteWrap) {
		t.Errorf("note %q stays after blur", m.flash)
	}
	m.flash = noteChop
	m.Focus()
	if m.flash != "" {
		t.Errorf("note %q stays after focus", m.flash)
	}
}
