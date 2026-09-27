package keyhelp

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestFilter(t *testing.T) {
	tests := []struct {
		name  string
		typed string
		want  []string
	}{
		{name: "empty lists all", want: []string{
			"help", "quit", "command", "refresh",
			"down", "up", "open", "reload", "merge", "check out", "cancel", "close",
		}},
		{name: "fuzzy over descriptions", typed: "chk", want: []string{"check out"}},
		{name: "by source", typed: "app", want: []string{"help", "quit", "command", "refresh"}},
		{name: "by key name", typed: "enter", want: []string{"open"}},
		// Fuzzy matching finds ctrl+c too; capture finds a key exactly.
		{name: "fuzzy over keys", typed: "ctrl+r", want: []string{"quit", "refresh", "reload"}},
		{name: "no match", typed: "zzz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := typeText(t, open(t), tt.typed)
			if got := descs(m); !slices.Equal(got, tt.want) {
				t.Errorf("shown %q, want %q", got, tt.want)
			}
		})
	}
}

// tab captures keys, each key replacing the last, until tab again; esc
// then clears the key, and closes the help next.
func TestCapture(t *testing.T) {
	m := open(t)
	m, _ = press(t, m, tab)
	if !m.Capturing() {
		t.Fatal("tab didn't start capturing")
	}
	m, sent := press(t, m, esc)
	if len(sent) != 0 || m.Key() != "esc" || !slices.Equal(descs(m), []string{"cancel", "close"}) {
		t.Fatalf("captured esc: key %q, shown %q, sent %v", m.Key(), descs(m), sent)
	}
	m, _ = press(t, m, ctrlR)
	if m.Key() != "ctrl+r" || !slices.Equal(descs(m), []string{"refresh", "reload"}) {
		t.Fatalf("captured ctrl+r: key %q, shown %q", m.Key(), descs(m))
	}
	m, _ = press(t, m, tab)
	if m.Capturing() || m.Key() != "ctrl+r" {
		t.Fatalf("tab again: capturing %v, key %q; want the key kept", m.Capturing(), m.Key())
	}
	m, sent = press(t, m, esc)
	if len(sent) != 0 || m.Key() != "" || len(m.Shown()) != len(m.Rows()) {
		t.Fatalf("esc: sent %v, key %q, %d of %d shown", sent, m.Key(), len(m.Shown()), len(m.Rows()))
	}
	_, sent = press(t, m, esc)
	if !slices.Equal(sent, []tea.Msg{CloseMsg{ID: m.ID()}}) {
		t.Errorf("second esc sent %v, want a CloseMsg", sent)
	}
}

// ? closes the help while the query is empty, and is typed otherwise.
func TestQuestionMark(t *testing.T) {
	m := open(t)
	if _, sent := press(t, m, question); len(sent) != 1 {
		t.Errorf("? on an empty query sent %v, want a CloseMsg", sent)
	}
	m = typeText(t, m, "he")
	m, sent := press(t, m, question)
	if len(sent) != 0 || m.Query() != "he?" {
		t.Errorf("? on a query: sent %v, query %q", sent, m.Query())
	}
	m, _ = press(t, m, esc)
	if m.Query() != "" {
		t.Errorf("esc left the query %q", m.Query())
	}
}

func TestScroll(t *testing.T) {
	m := open(t, WithSize(60, 6))
	top := m.View()
	steps := []struct {
		key  tea.KeyPressMsg
		want int
	}{
		{down, 1}, {down, 2}, {up, 1}, {pgDown, 5}, {pgUp, 1}, {end, -1}, {home, 0},
	}
	for _, s := range steps {
		m, _ = press(t, m, s.key)
		want := s.want
		if want < 0 {
			want = m.vp.TotalLineCount() - m.vp.Height()
		}
		if got := m.vp.YOffset(); got != want {
			t.Errorf("after %s the offset is %d, want %d", s.key, got, want)
		}
	}
	if m.View() != top {
		t.Error("home didn't bring back the first view")
	}
}

func TestBlurredIgnoresKeys(t *testing.T) {
	m := New(WithLayers(layers()), WithSize(60, 10))
	m, sent := press(t, m, esc, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if len(sent) != 0 || m.Query() != "" {
		t.Errorf("a blurred help took keys: sent %v, query %q", sent, m.Query())
	}
}

// Messages carry the ID of the help that sent them.
func TestIDs(t *testing.T) {
	a, b := New(), New()
	if a.ID() == b.ID() {
		t.Error("two helps share an ID")
	}
}

// The help's own keys follow its state: Close is typed into a query,
// and only Capture acts while a key is captured.
func TestHelpState(t *testing.T) {
	enabled := func(m Model) []string {
		var out []string
		for _, g := range m.FullHelp() {
			for _, b := range g {
				if b.Enabled() {
					out = append(out, b.Help().Desc)
				}
			}
		}
		return out
	}
	m := open(t)
	if got := enabled(m); !slices.Contains(got, "close") || len(got) != 9 {
		t.Errorf("open: enabled %q, want all nine", got)
	}
	m = typeText(t, m, "q")
	if got := enabled(m); slices.Contains(got, "close") {
		t.Errorf("with a query: enabled %q, want close typed", got)
	}
	m, _ = press(t, m, tab)
	if got := enabled(m); !slices.Equal(got, []string{"find a key"}) {
		t.Errorf("capturing: enabled %q, want only tab", got)
	}
	if len(m.ShortHelp()) != 3 {
		t.Errorf("ShortHelp has %d bindings, want 3", len(m.ShortHelp()))
	}
}

func TestSetters(t *testing.T) {
	m := open(t)
	m.SetTitle("Help · Issues")
	m.SetQuery("merge")
	if m.Title() != "Help · Issues" || !slices.Equal(descs(m), []string{"merge"}) {
		t.Errorf("title %q, shown %q", m.Title(), descs(m))
	}
	m.SetLayers(layers()[:1])
	if len(m.Rows()) != 4 || len(m.Shown()) != 0 {
		t.Errorf("after SetLayers: %d rows, %d shown", len(m.Rows()), len(m.Shown()))
	}
	m.Reset()
	if m.Query() != "" || len(m.Shown()) != 4 {
		t.Errorf("after Reset: query %q, %d shown", m.Query(), len(m.Shown()))
	}
	k := m.KeyMap()
	k.Close.SetEnabled(false)
	m.SetKeyMap(k)
	if m.KeyMap().Close.Enabled() {
		t.Error("SetKeyMap didn't take")
	}
	s := DefaultStyles(false)
	m.SetStyles(s)
	if m.Styles().Title.GetForeground() != s.Title.GetForeground() {
		t.Error("SetStyles didn't take")
	}
	m.Blur()
	if m.Focused() || m.Init() != nil || m.Width() != 80 || m.Height() != 24 {
		t.Error("Blur, Init or the size is off")
	}
}
