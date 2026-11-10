package confirm

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func testKeys(tb testing.TB) KeyMap {
	tb.Helper()
	return NewKeyMap(keytest.Table(map[string][]string{
		"yes":     {"y"},
		"no":      {"n", "esc"},
		"switch":  {"tab", "left", "right", "h", "l"},
		"default": {"enter"},
	}))
}

func TestKeyMap(t *testing.T) {
	t.Parallel()
	keytest.Complete(t, testKeys(t))
	keytest.Tagged(t, testKeys(t))
	keytest.HelpTags(t, testKeys(t))
	keytest.NoConflicts(t, testKeys(t))
}

func TestAnswer(t *testing.T) {
	t.Parallel()
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	for _, tt := range []struct {
		name     string
		msg      tea.KeyPressMsg
		armed    bool
		def      bool
		wantDone bool
		wantRun  bool
	}{
		{"yes", tea.KeyPressMsg{Code: 'y', Text: "y"}, false, false, true, true},
		{"no", tea.KeyPressMsg{Code: 'n', Text: "n"}, false, true, true, false},
		{"esc is no", tea.KeyPressMsg{Code: tea.KeyEscape}, false, true, true, false},
		{"default yes armed", enter, true, true, true, true},
		{"default no armed", enter, true, false, true, false},
		{"default not armed", enter, false, true, false, false},
		{"default repeat", tea.KeyPressMsg{Code: tea.KeyEnter, IsRepeat: true}, true, true, false, false},
		{"other key", tea.KeyPressMsg{Code: 'x', Text: "x"}, true, true, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ran := false
			a := Ask{Question: "Merge?", Default: tt.def, Run: func() tea.Cmd {
				ran = true
				return nil
			}}
			cmd, done := testKeys(t).Answer(&a, tt.msg, tt.armed)
			if cmd != nil {
				cmd()
			}
			if done != tt.wantDone || ran != tt.wantRun {
				t.Errorf("done = %v, ran = %v, want %v, %v", done, ran, tt.wantDone, tt.wantRun)
			}
		})
	}
}

// The switch keys move the choice, and enter gives the choice that is
// picked, not the default.
func TestSwitch(t *testing.T) {
	t.Parallel()
	k := testKeys(t)
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyTab}, {Code: tea.KeyLeft}, {Code: tea.KeyRight}, {Code: 'h', Text: "h"}, {Code: 'l', Text: "l"}} {
		for _, def := range []bool{false, true} {
			t.Run(key.String()+strconv.FormatBool(def), func(t *testing.T) {
				t.Parallel()
				ran := false
				a := Ask{Question: "Close?", Default: def, Run: func() tea.Cmd { ran = true; return nil }}
				if cmd, done := k.Answer(&a, key, true); cmd != nil || done {
					t.Fatalf("a switch key answered: %v, %v", cmd != nil, done)
				}
				if a.Yes() == def {
					t.Errorf("choice is %v after a switch, want it flipped", a.Yes())
				}
				if _, done := k.Answer(&a, enter, true); !done || ran != !def {
					t.Errorf("enter after a switch: done %v, ran %v; want run %v", done, ran, !def)
				}
			})
		}
	}
}

func TestBoxGolden(t *testing.T) {
	t.Parallel()
	long := "Merge pull request #12 into a branch with a very long name that wraps over lines?"
	for _, w := range []int{50, 80, 120} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			t.Parallel()
			st, k := DefaultStyles(true), testKeys(t)
			for name, a := range map[string]Ask{
				"yes":    {Question: long, Default: true},
				"no":     {Question: "Close issue #12?"},
				"danger": {Question: "Delete your comment?", Danger: true},
			} {
				t.Run(name, func(t *testing.T) {
					golden.RequireEqual(t, []byte(strings.Join(a.Box(st, k, w), "\n")))
				})
			}
		})
	}
}

func TestBoxShape(t *testing.T) {
	t.Parallel()
	k := testKeys(t)
	a := Ask{Question: "Reopen PR #5?"}
	got := ansi.Strip(strings.Join(a.Box(Styles{}, k, 50), "\n"))
	if n := strings.Count(got, "\n") + 1; n != 4 {
		t.Fatalf("box = \n%s\nwant 4 lines, got %d", got, n)
	}
	if !strings.Contains(got, "[ No ]") || !strings.Contains(got, "y/n · tab: switch · ↵: confirm") {
		t.Errorf("box = \n%s", got)
	}
	for l := range strings.SplitSeq(got, "\n") {
		if ansi.StringWidth(l) != 50 {
			t.Errorf("line %q is %d cells, want 50", l, ansi.StringWidth(l))
		}
	}
	// A narrow box drops the hint but keeps the border whole.
	for _, l := range a.Box(Styles{}, k, 20) {
		if ansi.StringWidth(l) != 20 {
			t.Errorf("narrow line %q is %d cells, want 20", l, ansi.StringWidth(l))
		}
	}
}

// A question may name what GitHub calls something, such as a title.
func TestHostileQuestions(t *testing.T) {
	t.Parallel()
	a := Ask{Question: "Close " + termtexttest.Hostile + "?"}
	st, k := DefaultStyles(true), testKeys(t)
	for _, w := range []int{20, 80, 300} {
		termtexttest.AssertClean(t, strings.Join(a.Box(st, k, w), "\n"), w)
	}
}

func TestQuestionWraps(t *testing.T) {
	t.Parallel()
	k := testKeys(t)
	long := Ask{Question: "Merge #1234 into release/v2.0-beta with a merge commit and then a few words more to make it long?"}
	if h := long.Height(40); h != 5 {
		t.Errorf("height = %d, want 5 for a question of two lines", h)
	}
	if h := (Ask{Question: "Close?"}).Height(40); h != 4 {
		t.Errorf("height = %d, want 4 for a question of one line", h)
	}
	if got := len(long.Box(Styles{}, k, 40)); got != 5 {
		t.Errorf("box has %d lines, want 5", got)
	}
}

// Enter gives the choice once it is armed and not as a repeated press; an
// unbound default key gives nothing and shows nothing.
func TestDefaultKey(t *testing.T) {
	t.Parallel()
	k := testKeys(t)
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	for _, tt := range []struct {
		name       string
		def, armed bool
		msg        tea.KeyPressMsg
		done, run  bool
	}{
		{"no", false, true, enter, true, false},
		{"yes", true, true, enter, true, true},
		{"repeat", true, true, tea.KeyPressMsg{Code: tea.KeyEnter, IsRepeat: true}, false, false},
		{"unarmed", true, false, enter, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ran := false
			a := Ask{Question: "Star eggzec/x?", Default: tt.def, Run: func() tea.Cmd {
				ran = true
				return func() tea.Msg { return nil }
			}}
			cmd, done := k.Answer(&a, tt.msg, tt.armed)
			if done != tt.done || ran != tt.run || (cmd != nil) != tt.run {
				t.Errorf("done %v, ran %v, cmd %v; want done %v and run %v", done, ran, cmd != nil, tt.done, tt.run)
			}
		})
	}
	unbound := NewKeyMap(keytest.Table(map[string][]string{"yes": {"y"}, "no": {"n", "esc"}}))
	a := Ask{Question: "Star eggzec/x?", Default: true}
	if cmd, done := unbound.Answer(&a, enter, true); cmd != nil || done {
		t.Errorf("an unbound default key answered: %v, %v", cmd != nil, done)
	}
	if h := ansi.Strip(a.hint(Styles{}, unbound)); h != "y/n" {
		t.Errorf("hint %q, want only the answers", h)
	}
}

// The keys that answer follow the table: yes set to Y answers only with Y,
// and no set to esc alone leaves n to do nothing.
func TestKeysFollowTheTable(t *testing.T) {
	t.Parallel()
	k := NewKeyMap(keytest.Table(map[string][]string{"yes": {"Y"}, "no": {"esc"}, "default": {"enter"}}))
	for _, tt := range []struct {
		name      string
		msg       tea.KeyPressMsg
		done, run bool
	}{
		{"Y", tea.KeyPressMsg{Code: 'Y', Text: "Y"}, true, true},
		{"y", tea.KeyPressMsg{Code: 'y', Text: "y"}, false, false},
		{"esc", tea.KeyPressMsg{Code: tea.KeyEscape}, true, false},
		{"n", tea.KeyPressMsg{Code: 'n', Text: "n"}, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ran := false
			a := Ask{Question: "Close issue #12?", Run: func() tea.Cmd {
				ran = true
				return func() tea.Msg { return nil }
			}}
			cmd, done := k.Answer(&a, tt.msg, true)
			if done != tt.done || ran != tt.run || (cmd != nil) != tt.run {
				t.Errorf("done %v, ran %v; want done %v and run %v", done, ran, tt.done, tt.run)
			}
		})
	}
	// An unbound answer is listed without a key.
	unbound := NewKeyMap(keytest.Table(map[string][]string{"yes": {"y"}}))
	if unbound.No.Enabled() || unbound.No.Help().Desc != "no" {
		t.Errorf("unbound no = enabled %v, desc %q; want disabled and described", unbound.No.Enabled(), unbound.No.Help().Desc)
	}
}

func BenchmarkAnswer(b *testing.B) {
	k := testKeys(b)
	a := Ask{Question: "Merge pull request #12?", Default: true, Run: func() tea.Cmd { return nil }}
	msg := tea.KeyPressMsg{Code: 'y', Text: "y"}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = k.Answer(&a, msg, true)
	}
}

func BenchmarkView(b *testing.B) {
	m := New(Ask{Question: "Merge pull request #12?", Default: true}, WithKeyMap(testKeys(b)))
	m.SetSize(80, 5)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}
