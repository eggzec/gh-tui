package cmdline

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestUpdate(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		initial string
		// typed is typed first, then keys are pressed.
		typed string
		keys  []tea.Msg
		// want is the message the last key sends, or nil for none.
		want        tea.Msg
		wantValue   string
		wantFocused bool
	}{
		{
			name:  "typing",
			typed: "goto cli/cli",
			want:  nil, wantValue: "goto cli/cli", wantFocused: true,
		},
		{
			name:      "typing after the initial text",
			initial:   "goto ",
			typed:     "cli/cli",
			wantValue: "goto cli/cli", wantFocused: true,
		},
		{
			name:      "editing keys belong to the input",
			typed:     "goto cli",
			keys:      []tea.Msg{home, runeKey("x"), left, tea.KeyPressMsg{Code: tea.KeyDelete}},
			wantValue: "goto cli", wantFocused: true,
		},
		{
			name:      "q and x are text",
			typed:     "qx",
			wantValue: "qx", wantFocused: true,
		},
		{
			name:  "submit",
			typed: "goto cli/cli",
			keys:  []tea.Msg{enter},
			want:  SubmitMsg{Line: "goto cli/cli"}, wantValue: "goto cli/cli",
		},
		{
			name:  "submit trims the line",
			typed: "  goto cli/cli  ",
			keys:  []tea.Msg{enter},
			want:  SubmitMsg{Line: "goto cli/cli"}, wantValue: "  goto cli/cli  ",
		},
		{
			name:  "submitting a blank line cancels",
			typed: "   ",
			keys:  []tea.Msg{enter},
			want:  CancelMsg{}, wantValue: "   ",
		},
		{
			name:  "esc cancels",
			typed: "goto",
			keys:  []tea.Msg{esc},
			want:  CancelMsg{}, wantValue: "goto",
		},
		{
			name:  "ctrl+c cancels",
			typed: "goto",
			keys:  []tea.Msg{ctrlC},
			want:  CancelMsg{}, wantValue: "goto",
		},
		{
			name:  "backspace edits a line with text",
			typed: "go",
			keys:  []tea.Msg{bksp},
			want:  nil, wantValue: "g", wantFocused: true,
		},
		{
			name:  "backspace on an empty line cancels",
			typed: "g",
			keys:  []tea.Msg{bksp, bksp},
			want:  CancelMsg{},
		},
		{
			name: "results pass by untouched",
			keys: []tea.Msg{SubmitMsg{ID: -1, Line: "x"}, CancelMsg{ID: -1}},
			want: nil, wantFocused: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := opened(t, tt.initial, append(tt.opts, WithSize(80, MaxHeight))...)
			m = typeText(t, m, tt.typed)
			m, got := press(t, m, tt.keys...)
			switch want := tt.want.(type) {
			case SubmitMsg:
				want.ID = m.ID()
				tt.want = want
			case CancelMsg:
				want.ID = m.ID()
				tt.want = want
			}
			if got != tt.want {
				t.Errorf("sent %#v, want %#v", got, tt.want)
			}
			if v := m.Value(); v != tt.wantValue {
				t.Errorf("Value() = %q, want %q", v, tt.wantValue)
			}
			if m.Focused() != tt.wantFocused {
				t.Errorf("Focused() = %v, want %v", m.Focused(), tt.wantFocused)
			}
		})
	}
}

func TestUpdateBlurred(t *testing.T) {
	m := New(WithSize(80, MaxHeight))
	for _, msg := range []tea.Msg{runeKey("a"), enter, esc, bksp} {
		var cmd tea.Cmd
		if m, cmd = m.Update(msg); cmd != nil {
			t.Errorf("a blurred command line answered %v", msg)
		}
	}
	if m.Value() != "" {
		t.Errorf("a blurred command line took text: %q", m.Value())
	}
}

// Two command lines never react to each other's messages.
func TestUpdateScoped(t *testing.T) {
	a, b := opened(t, "", WithSize(80, MaxHeight)), opened(t, "", WithSize(80, MaxHeight))
	if a.ID() == b.ID() {
		t.Fatal("two command lines share an ID")
	}
	b = typeText(t, b, "quit")
	_, msg := press(t, b, enter)
	sub, ok := msg.(SubmitMsg)
	if !ok || sub.ID != b.ID() {
		t.Fatalf("b sent %#v, want a SubmitMsg with its ID", msg)
	}
	a2, cmd := a.Update(sub)
	if cmd != nil || a2.Value() != "" || !a2.Focused() {
		t.Errorf("a reacted to b's submit: value %q, focused %v", a2.Value(), a2.Focused())
	}
}

// Open starts a new command over whatever was left from the last one.
func TestOpenResets(t *testing.T) {
	m := opened(t, "", WithSize(80, MaxHeight))
	m = typeText(t, m, "old")
	m, _ = press(t, m, esc)
	m.Open("new ")
	if !m.Focused() || m.Value() != "new " {
		t.Fatalf("after Open: focused %v, value %q", m.Focused(), m.Value())
	}
	m = typeText(t, m, "x")
	if m.Value() != "new x" {
		t.Errorf("typing after Open gave %q, want %q", m.Value(), "new x")
	}
}
