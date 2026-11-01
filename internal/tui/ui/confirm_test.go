package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// press returns the key press of k, as a terminal sends it.
func press(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	}
	r := []rune(k)
	return tea.KeyPressMsg{Code: r[0], Text: k}
}

// ran counts the runs of a Confirm.
type ran struct{ n int }

func (r *ran) confirm(q string) confirm.Ask {
	return confirm.Ask{Question: q, Run: func() tea.Cmd {
		r.n++
		return func() tea.Msg { return "sent" }
	}}
}

// A confirmation open when the icons are set draws with the new set, which
// the theme set after styles.
func TestConfirmModalSetIcons(t *testing.T) {
	var r ran
	m := NewConfirmModal(r.confirm("Close issue #12?"), confirm.NewKeyMap(Lookup(config.Default().Keys, "confirm")), NewIcons(config.IconsNerd))
	cfg, err := config.Default().Set("ui.icons", config.IconsASCII)
	if err != nil {
		t.Fatal(err)
	}
	if cmd := m.Update(SettingsMsg{Config: cfg}); cmd != nil {
		t.Errorf("the settings sent %v", cmd)
	}
	if got, want := m.icons.Separator, NewIcons(config.IconsASCII).Separator; got != want {
		t.Errorf("separator = %q, want %q", got, want)
	}
	if r.n != 0 || m.answered {
		t.Error("the settings answered the question")
	}
}

func TestRecheck(t *testing.T) {
	refusal := Notify(toast.Info, "You can't close issues in eggzec/x (read access).")
	tests := []struct {
		name string
		// question is what now asks by the time of the yes, "" for a
		// change that no longer applies.
		question string
		refused  bool
		// run is whether the change runs, and want what the yes shows
		// instead.
		run  bool
		want tea.Msg
	}{
		{name: "the same change runs", question: "Close issue #12?", run: true, want: "sent"},
		{name: "another change sends nothing", question: "Close issue #13?",
			want: NotifyMsg{Level: toast.Info, Text: "#12 changed meanwhile, so nothing was sent."}},
		{name: "a change that no longer applies sends nothing",
			want: NotifyMsg{Level: toast.Info, Text: "#12 changed meanwhile, so nothing was sent."}},
		{name: "a refusal says why", refused: true,
			want: NotifyMsg{Level: toast.Info, Text: "You can't close issues in eggzec/x (read access)."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r ran
			c := Recheck("Close issue #12?", "#12", func() (confirm.Ask, bool, tea.Cmd) {
				if tt.refused {
					return confirm.Ask{}, false, refusal
				}
				return r.confirm(tt.question), tt.question != "", nil
			})
			if c.Question != "Close issue #12?" {
				t.Fatalf("asks %q", c.Question)
			}
			msgs := runAll(c.Run())
			if (r.n == 1) != tt.run || r.n > 1 || !slices.Equal(msgs, []tea.Msg{tt.want}) {
				t.Errorf("ran %d times and showed %v, want run %v and %v", r.n, msgs, tt.run, tt.want)
			}
		})
	}
}

func TestOverLastLines(t *testing.T) {
	for _, tt := range []struct {
		view  string
		lines []string
		want  string
	}{
		{"a\nb\nc", []string{"Q"}, "a\nb\nQ"},
		{"a\nb\nc", []string{"Q", "R"}, "a\nQ\nR"},
		{"a", []string{"Q"}, "Q"},
		{"", []string{"Q"}, "Q"},
		{"a\n", []string{"Q"}, "a\nQ"},
	} {
		if got := OverLastLines(tt.view, tt.lines); got != tt.want {
			t.Errorf("OverLastLines(%q, %q) = %q, want %q", tt.view, tt.lines, got, tt.want)
		}
	}
}

func TestConfirmModal(t *testing.T) {
	tests := []struct {
		keys []string
		// closed is whether the modal asked to close, and run whether the
		// change ran.
		closed, run bool
	}{
		{keys: []string{"y"}, closed: true, run: true},
		{keys: []string{"enter"}},
		{keys: []string{"n"}, closed: true},
		{keys: []string{"esc"}, closed: true},
		{keys: []string{"x", "q", ":"}},
		{keys: []string{"x", "y"}, closed: true, run: true},
		// A second yes before the modal closed, such as a repeated key or
		// a paste, runs nothing more.
		{keys: []string{"y", "y"}, closed: true, run: true},
		{keys: []string{"y", "enter"}, closed: true, run: true},
		{keys: []string{"n", "y"}, closed: true},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.keys, " "), func(t *testing.T) {
			var r ran
			m := NewConfirmModal(r.confirm("Close issue #12?"), confirm.NewKeyMap(Lookup(config.Default().Keys, "confirm")), NewIcons(config.IconsUnicode))
			var msgs []tea.Msg
			for _, k := range tt.keys {
				msgs = append(msgs, runAll(m.Update(press(k)))...)
			}
			closed := slices.Contains(msgs, tea.Msg(LeaveModalMsg{Modal: m}))
			if closed != tt.closed || (r.n == 1) != tt.run || r.n > 1 {
				t.Errorf("%v: closed %v and ran %d times, want closed %v and run %v", tt.keys, closed, r.n, tt.closed, tt.run)
			}
			if tt.run && !slices.Contains(msgs, tea.Msg("sent")) {
				t.Errorf("%v: the change wasn't sent: %v", tt.keys, msgs)
			}
		})
	}
}

func TestConfirmModalFits(t *testing.T) {
	m := NewConfirmModal(confirm.Ask{Question: "Close issue #12?"}, confirm.NewKeyMap(Lookup(config.Default().Keys, "confirm")), NewIcons(config.IconsUnicode))
	if w, h := m.Fit(100, 20); w != 29 || h != 1 {
		t.Errorf("Fit = %d×%d, want the question and its keys on one line", w, h)
	}
	if w, _ := m.Fit(10, 20); w != 10 {
		t.Errorf("Fit in 10 columns = %d wide", w)
	}
	m.SetSize(29, 1)
	if got, want := m.View(), "Close issue #12?  y/n · ↵: no"; got != want {
		t.Errorf("View = %q, want %q", got, want)
	}
	if m.Title() != "Confirm" || m.Question() != "Close issue #12?" {
		t.Errorf("titled %q asking %q", m.Title(), m.Question())
	}
}

// A modal answers the default only once its arming message came back.
func TestConfirmModalArms(t *testing.T) {
	var r ran
	c := r.confirm("Star eggzec/x?")
	c.Default = true
	m := NewConfirmModal(c, confirm.NewKeyMap(Lookup(config.Default().Keys, "confirm")), NewIcons(config.IconsUnicode))
	if msgs := runAll(m.Update(press("enter"))); len(msgs) != 0 || r.n != 0 {
		t.Errorf("enter before arming gave %v, ran %d times", msgs, r.n)
	}
	m.Update(Arm()())
	if msgs := runAll(m.Update(press("enter"))); r.n != 1 || !slices.Contains(msgs, tea.Msg("sent")) {
		t.Errorf("enter once armed gave %v, ran %d times", msgs, r.n)
	}
}

// runAll runs cmd and the commands of the batches it returns, and returns
// their messages.
func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	b, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range b {
		out = append(out, runAll(c)...)
	}
	return out
}

// TestConfirmKeysFollowTheConfig checks that the keys that answer are those
// of the confirm context: yes set to Y answers only with Y, and no set to
// esc alone leaves n to do nothing.
func TestConfirmKeysFollowTheConfig(t *testing.T) {
	keys := config.Default().Keys
	keys.Set("confirm.yes", []string{"Y"})
	keys.Set("confirm.no", []string{"esc"})
	k := confirm.NewKeyMap(Lookup(keys, "confirm"))
	for _, tt := range []struct {
		key       string
		done, run bool
	}{
		{"Y", true, true},
		{"y", false, false},
		{"esc", true, false},
		{"n", false, false},
	} {
		t.Run(tt.key, func(t *testing.T) {
			var r ran
			cmd, done := k.Answer(r.confirm("Close issue #12?"), press(tt.key), true)
			if done != tt.done || (r.n == 1) != tt.run || (cmd != nil) != tt.run {
				t.Errorf("%s: done %v, ran %d times; want done %v and run %v", tt.key, done, r.n, tt.done, tt.run)
			}
		})
	}
	// An unbound answer is listed without a key.
	keys.Set("confirm.no", nil)
	if no := confirm.NewKeyMap(Lookup(keys, "confirm")).No; no.Enabled() || no.Help().Desc != "no" {
		t.Errorf("unbound no = enabled %v, desc %q; want disabled and described", no.Enabled(), no.Help().Desc)
	}
}
