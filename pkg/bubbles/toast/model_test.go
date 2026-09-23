package toast

import (
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
)

// entry is what a test expects of one visible toast.
type entry struct {
	level Level
	text  string
	count int
}

func entries(m Model) []entry {
	out := make([]entry, 0, len(m.toasts))
	for _, t := range m.toasts {
		out = append(out, entry{t.level, t.text, t.count})
	}
	return out
}

func requireEntries(t *testing.T, m Model, want []entry) {
	t.Helper()
	got := entries(m)
	if len(got) != len(want) {
		t.Fatalf("toasts = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("toasts = %v, want %v", got, want)
		}
	}
}

type push struct {
	level Level
	text  string
}

func TestPush(t *testing.T) {
	tests := []struct {
		name   string
		opts   []Option
		pushes []push
		want   []entry
	}{
		{
			name:   "one",
			pushes: []push{{Info, "Loaded"}},
			want:   []entry{{Info, "Loaded", 1}},
		},
		{
			name:   "newest at the bottom",
			pushes: []push{{Info, "a"}, {Success, "b"}},
			want:   []entry{{Info, "a", 1}, {Success, "b", 1}},
		},
		{
			name:   "repeat counts instead of stacking",
			pushes: []push{{Error, "boom"}, {Error, "boom"}, {Error, "boom"}},
			want:   []entry{{Error, "boom", 3}},
		},
		{
			name:   "repeat moves to the bottom",
			pushes: []push{{Info, "a"}, {Info, "b"}, {Info, "a"}},
			want:   []entry{{Info, "b", 1}, {Info, "a", 2}},
		},
		{
			name:   "same text at another level stacks",
			pushes: []push{{Info, "a"}, {Error, "a"}},
			want:   []entry{{Info, "a", 1}, {Error, "a", 1}},
		},
		{
			name:   "max drops the oldest",
			opts:   []Option{WithMax(2)},
			pushes: []push{{Info, "a"}, {Info, "b"}, {Info, "c"}},
			want:   []entry{{Info, "b", 1}, {Info, "c", 1}},
		},
		{
			name:   "repeat at max keeps the others",
			opts:   []Option{WithMax(2)},
			pushes: []push{{Info, "a"}, {Info, "b"}, {Info, "a"}},
			want:   []entry{{Info, "b", 1}, {Info, "a", 2}},
		},
		{
			name:   "max below one is one",
			opts:   []Option{WithMax(0)},
			pushes: []push{{Info, "a"}, {Info, "b"}},
			want:   []entry{{Info, "b", 1}},
		},
		{
			name:   "unknown level is info",
			pushes: []push{{Level(42), "a"}},
			want:   []entry{{Info, "a", 1}},
		},
		{
			name:   "text is put on one line without escapes",
			pushes: []push{{Info, "  \x1b[31mred\x1b[m\n\tline  "}, {Info, "red line"}},
			want:   []entry{{Info, "red line", 2}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(tt.opts...)
			for _, p := range tt.pushes {
				m.Push(p.level, p.text)
			}
			requireEntries(t, m, tt.want)
			if m.Empty() != (len(tt.want) == 0) || m.Len() != len(tt.want) {
				t.Errorf("Empty() = %v, Len() = %d with %d toasts", m.Empty(), m.Len(), len(tt.want))
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	other := New()
	tests := []struct {
		name   string
		pushes []push
		// msgs may refer to the model under test, which the test passes in.
		msgs func(m Model) []tea.Msg
		want []entry
	}{
		{
			name:   "expire removes the toast",
			pushes: []push{{Info, "a"}, {Info, "b"}},
			msgs:   func(m Model) []tea.Msg { return []tea.Msg{ExpireMsg{m.ID(), 1}} },
			want:   []entry{{Info, "b", 1}},
		},
		{
			name:   "expire of another instance is ignored",
			pushes: []push{{Info, "a"}},
			msgs:   func(Model) []tea.Msg { return []tea.Msg{ExpireMsg{other.ID(), 1}} },
			want:   []entry{{Info, "a", 1}},
		},
		{
			name:   "earlier timer of a refreshed toast is ignored",
			pushes: []push{{Info, "a"}, {Info, "a"}},
			msgs:   func(m Model) []tea.Msg { return []tea.Msg{ExpireMsg{m.ID(), 1}} },
			want:   []entry{{Info, "a", 2}},
		},
		{
			name:   "latest timer of a refreshed toast expires it",
			pushes: []push{{Info, "a"}, {Info, "a"}},
			msgs:   func(m Model) []tea.Msg { return []tea.Msg{ExpireMsg{m.ID(), 2}} },
			want:   []entry{},
		},
		{
			name:   "expire of a dismissed toast is ignored",
			pushes: []push{{Info, "a"}, {Info, "b"}},
			msgs: func(m Model) []tea.Msg {
				return []tea.Msg{tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl}, ExpireMsg{m.ID(), 2}}
			},
			want: []entry{{Info, "a", 1}},
		},
		{
			name:   "dismiss key removes the newest",
			pushes: []push{{Info, "a"}, {Info, "b"}},
			msgs:   func(Model) []tea.Msg { return []tea.Msg{tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl}} },
			want:   []entry{{Info, "a", 1}},
		},
		{
			name: "dismiss key without toasts does nothing",
			msgs: func(Model) []tea.Msg { return []tea.Msg{tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl}} },
			want: []entry{},
		},
		{
			name:   "other keys are ignored",
			pushes: []push{{Info, "a"}},
			msgs:   func(Model) []tea.Msg { return []tea.Msg{tea.KeyPressMsg{Code: 'x', Text: "x"}} },
			want:   []entry{{Info, "a", 1}},
		},
		{
			name:   "other messages are ignored",
			pushes: []push{{Info, "a"}},
			msgs:   func(Model) []tea.Msg { return []tea.Msg{tea.WindowSizeMsg{Width: 10, Height: 10}} },
			want:   []entry{{Info, "a", 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			for _, p := range tt.pushes {
				m.Push(p.level, p.text)
			}
			for _, msg := range tt.msgs(m) {
				var cmd tea.Cmd
				m, cmd = m.Update(msg)
				if cmd != nil {
					t.Errorf("Update(%T) returned a command", msg)
				}
			}
			requireEntries(t, m, tt.want)
		})
	}
}

func TestDismissAndClear(t *testing.T) {
	m := New()
	if cmd := m.Dismiss(); cmd != nil {
		t.Error("Dismiss on an empty stack returned a command")
	}
	m.Push(Info, "a")
	m.Push(Info, "b")
	m.Push(Info, "c")
	m.Dismiss()
	requireEntries(t, m, []entry{{Info, "a", 1}, {Info, "b", 1}})
	m.Clear()
	if !m.Empty() || m.View() != "" {
		t.Errorf("after Clear: Empty() = %v, View() = %q", m.Empty(), m.View())
	}
}

func TestSetMaxDropsTheOldest(t *testing.T) {
	m := New()
	m.Push(Info, "a")
	m.Push(Info, "b")
	m.Push(Info, "c")
	m.SetMax(1)
	if m.Max() != 1 {
		t.Errorf("Max() = %d, want 1", m.Max())
	}
	requireEntries(t, m, []entry{{Info, "c", 1}})
}

// Copies of a model share nothing that a push or an expiry changes.
func TestCopiesAreIndependent(t *testing.T) {
	m := New()
	m.Push(Info, "a")
	m.Push(Info, "b")
	before := m
	m.Push(Info, "a")
	m, _ = m.Update(ExpireMsg{m.ID(), 2})
	requireEntries(t, before, []entry{{Info, "a", 1}, {Info, "b", 1}})
	requireEntries(t, m, []entry{{Info, "a", 2}})
}

func TestDismissBindingFollowsToasts(t *testing.T) {
	m := New()
	if m.KeyMap().Dismiss.Enabled() {
		t.Error("dismiss enabled without toasts")
	}
	m.Push(Info, "a")
	if !m.KeyMap().Dismiss.Enabled() {
		t.Error("dismiss disabled with a toast")
	}
	k := DefaultKeyMap()
	k.Dismiss.SetKeys("esc")
	m.SetKeyMap(k)
	if !m.KeyMap().Dismiss.Enabled() {
		t.Error("SetKeyMap lost the enabled state")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.Empty() {
		t.Error("custom dismiss key did not dismiss")
	}
	if help := m.KeyMap().ShortHelp(); len(help) != 1 || len(m.KeyMap().FullHelp()) != 1 {
		t.Errorf("help = %v", help)
	}
}

func TestOptionsAndAccessors(t *testing.T) {
	s := DefaultStyles(false)
	m := New(
		WithDuration(time.Second),
		WithErrorDuration(time.Minute),
		WithSize(100, 20),
		WithStyles(s),
		WithKeyMap(DefaultKeyMap()),
	)
	if m.Duration() != time.Second || m.ErrorDuration() != time.Minute {
		t.Errorf("durations = %v, %v", m.Duration(), m.ErrorDuration())
	}
	if m.Width() != 100 || m.Height() != 20 {
		t.Errorf("size = %dx%d", m.Width(), m.Height())
	}
	m.SetWidth(90)
	m.SetHeight(10)
	if m.Width() != 90 || m.Height() != 10 {
		t.Errorf("size = %dx%d", m.Width(), m.Height())
	}
	m.SetDuration(2 * time.Second)
	m.SetErrorDuration(3 * time.Second)
	if m.Duration() != 2*time.Second || m.ErrorDuration() != 3*time.Second {
		t.Errorf("durations = %v, %v", m.Duration(), m.ErrorDuration())
	}
	if m.Styles().Info != s.Info {
		t.Error("Styles() did not return the styles set")
	}
	if m.Init() != nil {
		t.Error("Init returned a command")
	}
}

func TestIDsAreUnique(t *testing.T) {
	if a, b := New(), New(); a.ID() == b.ID() {
		t.Errorf("two models share ID %d", a.ID())
	}
}

func TestLevelString(t *testing.T) {
	for l, want := range map[Level]string{
		Info: "info", Success: "success", Warning: "warning", Error: "error", Level(9): "unknown",
	} {
		if got := l.String(); got != want {
			t.Errorf("Level(%d).String() = %q, want %q", l, got, want)
		}
	}
}

// The command from Push fires after the duration for its level, with the
// message that expires exactly that toast.
func TestPushExpiresAfterDuration(t *testing.T) {
	tests := []struct {
		level Level
		want  time.Duration
	}{
		{Info, 2 * time.Second},
		{Success, 2 * time.Second},
		{Warning, 2 * time.Second},
		{Error, 5 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := New(WithDuration(2*time.Second), WithErrorDuration(5*time.Second))
				start := time.Now()
				cmd := m.Push(tt.level, "done")
				msg := cmd()
				if got := time.Since(start); got != tt.want {
					t.Errorf("fired after %v, want %v", got, tt.want)
				}
				m, _ = m.Update(msg)
				if !m.Empty() {
					t.Errorf("message %#v did not expire the toast", msg)
				}
			})
		})
	}
}

// A refreshed toast lives for a full duration after the refresh, even though
// the first timer fires in between.
func TestRefreshOutlivesTheFirstTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := New(WithDuration(2 * time.Second))
		first := m.Push(Info, "again")
		time.Sleep(time.Second) // fake clock inside the bubble
		second := m.Push(Info, "again")

		m, _ = m.Update(first())
		if m.Empty() {
			t.Fatal("the first timer expired the refreshed toast")
		}
		m, _ = m.Update(second())
		if !m.Empty() {
			t.Error("the second timer did not expire the toast")
		}
	})
}

func TestZeroDurationStays(t *testing.T) {
	m := New(WithDuration(0), WithErrorDuration(-1))
	if m.Push(Info, "a") != nil || m.Push(Error, "b") != nil {
		t.Error("Push returned a timer for a toast that should stay")
	}
}
