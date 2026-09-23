package tabs

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

var sections = []string{"Pull requests", "Issues", "Notifications", "Repositories"}

var (
	keyTab      = tea.KeyPressMsg{Code: tea.KeyTab}
	keyShiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
)

func digit(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func TestUpdate(t *testing.T) {
	other := New(WithTabs(sections...))
	tests := []struct {
		name    string
		active  int
		blurred bool
		msg     tea.Msg
		want    int
		changed bool
	}{
		{name: "next", active: 0, msg: keyTab, want: 1, changed: true},
		{name: "next wraps", active: 3, msg: keyTab, want: 0, changed: true},
		{name: "prev", active: 2, msg: keyShiftTab, want: 1, changed: true},
		{name: "prev wraps", active: 0, msg: keyShiftTab, want: 3, changed: true},
		{name: "jump", active: 0, msg: digit('3'), want: 2, changed: true},
		{name: "jump to last", active: 0, msg: digit('4'), want: 3, changed: true},
		{name: "jump to same tab", active: 1, msg: digit('2'), want: 1},
		{name: "jump past last", active: 1, msg: digit('9'), want: 1},
		{name: "other key", active: 1, msg: digit('x'), want: 1},
		{name: "blurred", active: 1, blurred: true, msg: keyTab, want: 1},
		{name: "other instance", active: 1, msg: ChangeMsg{ID: other.ID(), Index: 3}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(WithTabs(sections...), WithActive(tt.active), WithFocused(!tt.blurred))
			m, cmd := m.Update(tt.msg)
			if got := m.Active(); got != tt.want {
				t.Errorf("Active() = %d, want %d", got, tt.want)
			}
			if !tt.changed {
				if cmd != nil {
					t.Errorf("got a command, want none")
				}
				return
			}
			if cmd == nil {
				t.Fatal("got no command, want a ChangeMsg")
			}
			want := ChangeMsg{ID: m.ID(), Index: tt.want}
			if got := cmd(); got != want {
				t.Errorf("msg = %#v, want %#v", got, want)
			}
		})
	}
}

func TestUpdateWithoutTabs(t *testing.T) {
	m := New(WithFocused(true))
	m, cmd := m.Update(keyTab)
	if cmd != nil || m.Active() != 0 {
		t.Errorf("Active() = %d, cmd = %v; want 0 and no command", m.Active(), cmd)
	}
}

func TestSetActive(t *testing.T) {
	tests := []struct {
		name    string
		index   int
		want    int
		changed bool
	}{
		{name: "other tab", index: 2, want: 2, changed: true},
		{name: "same tab", index: 1, want: 1},
		{name: "negative", index: -1, want: 1},
		{name: "past last", index: 4, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(WithTabs(sections...), WithActive(1))
			cmd := m.SetActive(tt.index)
			if m.Active() != tt.want {
				t.Errorf("Active() = %d, want %d", m.Active(), tt.want)
			}
			if (cmd != nil) != tt.changed {
				t.Fatalf("command = %v, want changed = %v", cmd != nil, tt.changed)
			}
			if cmd != nil {
				if got, want := cmd(), (ChangeMsg{ID: m.ID(), Index: tt.want}); got != want {
					t.Errorf("msg = %#v, want %#v", got, want)
				}
			}
		})
	}
}

func TestSetTabsClampsActive(t *testing.T) {
	m := New(WithTabs(sections...), WithActive(3))
	m.SetTabs("Pull requests", "Issues")
	if m.Active() != 1 {
		t.Errorf("Active() = %d, want 1", m.Active())
	}
}

func TestIDsAreUnique(t *testing.T) {
	a, b := New(), New()
	if a.ID() == b.ID() {
		t.Errorf("both instances have ID %d", a.ID())
	}
}

func TestFocus(t *testing.T) {
	m := New()
	if m.Focused() {
		t.Fatal("new model is focused")
	}
	m.Focus()
	if !m.Focused() {
		t.Fatal("Focus did not focus")
	}
	m.Blur()
	if m.Focused() {
		t.Fatal("Blur did not blur")
	}
}

func TestSetKeyMap(t *testing.T) {
	k := DefaultKeyMap()
	k.Next.SetKeys("l")
	m := New(WithTabs(sections...), WithFocused(true))
	m.SetKeyMap(k)
	m, _ = m.Update(digit('l'))
	if m.Active() != 1 {
		t.Errorf("Active() = %d, want 1", m.Active())
	}
}

func BenchmarkUpdate(b *testing.B) {
	m := New(WithTabs(sections...), WithWidth(80), WithFocused(true))
	b.ReportAllocs()
	for b.Loop() {
		m, _ = m.Update(keyTab)
	}
}
