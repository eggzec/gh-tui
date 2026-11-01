package confirm

import (
	"slices"
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
			cmd, done := testKeys(t).Answer(a, tt.msg, tt.armed)
			if cmd != nil {
				cmd()
			}
			if done != tt.wantDone || ran != tt.wantRun {
				t.Errorf("done = %v, ran = %v, want %v, %v", done, ran, tt.wantDone, tt.wantRun)
			}
		})
	}
}

func TestView(t *testing.T) {
	t.Parallel()
	a := Ask{Question: "Merge pull request #12 into a branch with a very long name that wraps over lines?", Default: true}
	for _, w := range []int{80, 120} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			t.Parallel()
			st, k := DefaultStyles(true), testKeys(t)
			t.Run("Line", func(t *testing.T) { golden.RequireEqual(t, a.Line(st, k, w)) })
			t.Run("Lines", func(t *testing.T) { golden.RequireEqual(t, []byte(strings.Join(a.Lines(st, k, w, MaxLines), "\n"))) })
		})
	}
}

func TestLine(t *testing.T) {
	t.Parallel()
	a := Ask{Question: "Reopen PR #5?"}
	st, k := Styles{}, testKeys(t)
	if got, want := a.Line(st, k, 32), "Reopen PR #5?        y/n · ↵: no"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	// A narrow line cuts the question and keeps the keys.
	if got, want := a.Line(st, k, 20), "Reopen … y/n · ↵: no"; got != want {
		t.Errorf("narrow line = %q, want %q", got, want)
	}
}

// A question may name what GitHub calls something, such as a title.
func TestHostileQuestions(t *testing.T) {
	t.Parallel()
	a := Ask{Question: "Close " + termtexttest.Hostile + "?"}
	st, k := DefaultStyles(true), testKeys(t)
	for _, w := range []int{20, 80, 300} {
		termtexttest.AssertClean(t, a.Line(st, k, w), w)
		termtexttest.AssertClean(t, strings.Join(a.Lines(st, k, w, MaxLines), "\n"), w)
	}
}

func TestLines(t *testing.T) {
	t.Parallel()
	st, k := Styles{}, testKeys(t)
	long := Ask{Question: "Merge #1234 into release/v2.0-beta with a merge commit?"}
	for _, tt := range []struct {
		name string
		a    Ask
		w, n int
		want []string
	}{
		{"fits on one line", Ask{Question: "Close PR #5?"}, 28, 2, []string{"Close PR #5?     y/n · ↵: no"}},
		{"wraps to two", long, 48, 2, []string{
			"Merge #1234 into release/v2.0-beta              ",
			"with a merge commit?                 y/n · ↵: no",
		}},
		{"cuts past the last line", long, 32, 2, []string{
			"Merge #1234 into                ",
			"release/v2.0-beta w… y/n · ↵: no",
		}},
		// A branch name wraps whole, and is cut if it is longer than a line.
		{"keeps words whole", Ask{Question: "Merge #1 into release/v2.0-beta-rc?"}, 38, 2, []string{
			"Merge #1 into                         ",
			"release/v2.0-beta-rc?      y/n · ↵: no",
		}},
		{"cuts a word longer than a line", Ask{Question: "Merge #1 into release/v2.0-beta-candidate-long?"}, 38, 2, []string{
			"Merge #1 into                         ",
			"release/v2.0-beta-candida… y/n · ↵: no",
		}},
		{"one line cuts", long, 32, 1, []string{"Merge #1234 into re… y/n · ↵: no"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.a.Lines(st, k, tt.w, tt.n); !slices.Equal(got, tt.want) {
				t.Errorf("Lines = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAnswersYesOrNoAlone(t *testing.T) {
	t.Parallel()
	k := testKeys(t)
	a := Ask{Question: "Close issue #12?", Run: func() tea.Cmd { return nil }}
	if cmd, done := k.Answer(a, tea.KeyPressMsg{Code: tea.KeyTab}, true); cmd != nil || done {
		t.Errorf("tab answered the question: %v, %v", cmd != nil, done)
	}
	if got := ansi.Strip(a.Line(Styles{}, k, 40)); !strings.HasSuffix(got, "y/n · ↵: no") {
		t.Errorf("the question reads %q, want it to end in the answers and the default", got)
	}
}

// Enter gives the default of the question, once it is armed and not as a
// repeated press; an unbound default key gives nothing and shows nothing.
func TestDefaultKey(t *testing.T) {
	t.Parallel()
	k := testKeys(t)
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	for _, tt := range []struct {
		name         string
		def, armed   bool
		msg          tea.KeyPressMsg
		done, run    bool
		wantHintTail string
	}{
		{"no", false, true, enter, true, false, "↵: no"},
		{"yes", true, true, enter, true, true, "↵: yes"},
		{"repeat", true, true, tea.KeyPressMsg{Code: tea.KeyEnter, IsRepeat: true}, false, false, "↵: yes"},
		{"unarmed", true, false, enter, false, false, "↵: yes"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ran := false
			a := Ask{Question: "Star eggzec/x?", Default: tt.def, Run: func() tea.Cmd {
				ran = true
				return func() tea.Msg { return nil }
			}}
			cmd, done := k.Answer(a, tt.msg, tt.armed)
			if done != tt.done || ran != tt.run || (cmd != nil) != tt.run {
				t.Errorf("done %v, ran %v, cmd %v; want done %v and run %v", done, ran, cmd != nil, tt.done, tt.run)
			}
			if got := ansi.Strip(a.Line(Styles{}, k, 60)); !strings.HasSuffix(got, tt.wantHintTail) {
				t.Errorf("line %q, want it to end in %q", got, tt.wantHintTail)
			}
		})
	}
	unbound := NewKeyMap(keytest.Table(map[string][]string{"yes": {"y"}, "no": {"n", "esc"}}))
	a := Ask{Question: "Star eggzec/x?", Default: true}
	if cmd, done := unbound.Answer(a, enter, true); cmd != nil || done {
		t.Errorf("an unbound default key answered: %v, %v", cmd != nil, done)
	}
	if got := ansi.Strip(a.Line(Styles{}, unbound, 40)); !strings.HasSuffix(got, "  y/n") {
		t.Errorf("line %q, want only the answers", got)
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
			cmd, done := k.Answer(a, tt.msg, true)
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
		_, _ = k.Answer(a, msg, true)
	}
}

func BenchmarkView(b *testing.B) {
	m := New(Ask{Question: "Merge pull request #12?", Default: true}, WithKeyMap(testKeys(b)))
	m.SetSize(80, 2)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}
