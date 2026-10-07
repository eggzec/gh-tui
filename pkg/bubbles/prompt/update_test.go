package prompt

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestUpdate(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		// typed is typed first, then keys are pressed.
		typed string
		keys  []tea.Msg
		// want is the message the last key sends, or nil for none.
		want      tea.Msg
		wantValue string
	}{
		{
			name:  "submit multi-line",
			typed: "Looks good",
			keys:  []tea.Msg{enter, runeKey("!"), ctrlS},
			want:  SubmitMsg{Value: "Looks good\n!"}, wantValue: "Looks good\n!",
		},
		{
			name:  "submit single-line",
			opts:  []Option{WithMode(SingleLine)},
			typed: "bug, ui",
			keys:  []tea.Msg{ctrlS},
			want:  SubmitMsg{Value: "bug, ui"}, wantValue: "bug, ui",
		},
		{
			name: "submit empty",
			keys: []tea.Msg{ctrlS},
			want: SubmitMsg{},
		},
		{
			name:  "cancel",
			typed: "never mind",
			keys:  []tea.Msg{esc},
			want:  CancelMsg{}, wantValue: "never mind",
		},
		{
			name:  "enter submits a single line",
			opts:  []Option{WithMode(SingleLine)},
			typed: "bug",
			keys:  []tea.Msg{enter},
			want:  SubmitMsg{Value: "bug"}, wantValue: "bug",
		},
		{
			name:  "enter starts a new line in multi-line",
			typed: "a",
			keys:  []tea.Msg{enter, runeKey("b")},
			want:  nil, wantValue: "a\nb",
		},
		{
			name:  "typing appends to the initial value",
			opts:  []Option{WithMode(SingleLine), WithValue("bug")},
			typed: ", ui",
			keys:  []tea.Msg{ctrlS},
			want:  SubmitMsg{Value: "bug, ui"}, wantValue: "bug, ui",
		},
		{
			name:      "typing appends to a multi-line initial value",
			opts:      []Option{WithValue("Hello")},
			typed:     " there",
			wantValue: "Hello there",
		},
		{
			name:      "backspace edits",
			typed:     "typo",
			keys:      []tea.Msg{bksp, bksp},
			wantValue: "ty",
		},
		{
			name:      "q and x are text",
			typed:     "qx",
			wantValue: "qx",
		},
		{
			name:      "char limit",
			opts:      []Option{WithCharLimit(3)},
			typed:     "abcdef",
			wantValue: "abc",
		},
		{
			name: "remapped submit",
			opts: []Option{WithKeyMap(func() KeyMap {
				k := testKeys(t)
				k.Submit.SetKeys("ctrl+d")
				return k
			}())},
			typed: "hi",
			keys:  []tea.Msg{ctrlS, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}},
			want:  SubmitMsg{Value: "hi"}, wantValue: "hi",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := focused(t, append([]Option{WithSize(40, 6)}, tt.opts...)...)
			m = typeText(t, m, tt.typed)
			m, got := send(t, m, tt.keys...)
			switch want := tt.want.(type) {
			case SubmitMsg:
				want.ID = m.ID()
				if got != want {
					t.Errorf("sent %#v, want %#v", got, want)
				}
			case CancelMsg:
				want.ID = m.ID()
				if got != want {
					t.Errorf("sent %#v, want %#v", got, want)
				}
			case nil:
				if _, ok := got.(SubmitMsg); ok {
					t.Errorf("sent %#v, want no submit", got)
				}
				if _, ok := got.(CancelMsg); ok {
					t.Errorf("sent %#v, want no cancel", got)
				}
			}
			if v := m.Value(); v != tt.wantValue {
				t.Errorf("Value() = %q, want %q", v, tt.wantValue)
			}
		})
	}
}

func TestStartsBlurredAndIgnoresKeys(t *testing.T) {
	m := newKeyed(t, WithSize(40, 6))
	if m.Focused() {
		t.Fatal("a new prompt is focused")
	}
	m = typeText(t, m, "hi")
	if _, msg := send(t, m, ctrlS); msg != nil || m.Value() != "" {
		t.Errorf("blurred prompt took keys: value %q, sent %#v", m.Value(), msg)
	}
	m.Focus()
	m = typeText(t, m, "hi")
	m.Blur()
	m = typeText(t, m, "!")
	if m.Value() != "hi" {
		t.Errorf("Value() = %q after blur, want hi", m.Value())
	}
}

func TestMessagesAreScoped(t *testing.T) {
	a, b := focused(t), focused(t)
	if a.ID() == b.ID() {
		t.Fatal("two prompts share an ID")
	}
	_, msg := send(t, a, ctrlS)
	if s, ok := msg.(SubmitMsg); !ok || s.ID != a.ID() {
		t.Errorf("submit = %#v, want the ID of its prompt", msg)
	}
}

func TestPaste(t *testing.T) {
	for _, mode := range []Mode{MultiLine, SingleLine} {
		m := focused(t, WithMode(mode), WithSize(40, 6))
		m, _ = m.Update(tea.PasteMsg{Content: "pasted"})
		if m.Value() != "pasted" {
			t.Errorf("mode %d: Value() = %q after paste", mode, m.Value())
		}
	}
}

func TestAccessors(t *testing.T) {
	m := newKeyed(t, WithMode(SingleLine), WithTitle("Labels"), WithSize(30, 3))
	if m.Mode() != SingleLine || m.Title() != "Labels" || m.Width() != 30 || m.Height() != 3 {
		t.Errorf("accessors = %v %q %d×%d", m.Mode(), m.Title(), m.Width(), m.Height())
	}
	m.SetTitle("Edit labels")
	m.SetValue("bug")
	m.SetSize(-1, -1)
	if m.Title() != "Edit labels" || m.Value() != "bug" || m.Width() != 0 || m.Height() != 0 || m.View() != "" {
		t.Errorf("after set: %q %q %d×%d %q", m.Title(), m.Value(), m.Width(), m.Height(), m.View())
	}
	if newKeyed(t, WithMode(Mode(7))).Mode() != MultiLine {
		t.Error("an unknown mode should fall back to multi-line")
	}
	if len(m.ShortHelp()) != 2 || len(m.FullHelp()) != 1 || m.Init() != nil {
		t.Error("help should list submit and cancel, and Init nothing")
	}
	if got := m.ShortHelp()[0].Help().Key; got != "↵" {
		t.Errorf("single-line help submits with %q, want ↵", got)
	}
	if got := newKeyed(t).ShortHelp()[0].Help().Key; got != "^s" {
		t.Errorf("multi-line help submits with %q, want ^s", got)
	}
	k := m.KeyMap()
	k.Cancel.SetEnabled(false)
	m.SetKeyMap(k)
	if m.KeyMap().Cancel.Enabled() {
		t.Error("SetKeyMap didn't take")
	}
	s := DefaultStyles(false)
	m.SetStyles(s)
	if m.Styles().Title.GetForeground() != s.Title.GetForeground() {
		t.Error("SetStyles didn't take")
	}
}
