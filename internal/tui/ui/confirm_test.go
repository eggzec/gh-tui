package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
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

func (r *ran) confirm(q string) Confirm {
	return Confirm{Question: q, Run: func() tea.Cmd {
		r.n++
		return func() tea.Msg { return "sent" }
	}}
}

func TestConfirmAnswer(t *testing.T) {
	tests := []struct {
		key       string
		done, run bool
	}{
		{"y", true, true},
		{"enter", false, false},
		{"n", true, false},
		{"esc", true, false},
		{"q", false, false},
		{"m", false, false},
		{"j", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			var r ran
			cmd, done := NewConfirmKeys(config.Default().Keys).Answer(r.confirm("Close issue #12?"), press(tt.key))
			if done != tt.done || (r.n == 1) != tt.run || (cmd != nil) != tt.run {
				t.Errorf("%s: done %v, ran %d times, cmd %v; want done %v and run %v", tt.key, done, r.n, cmd != nil, tt.done, tt.run)
			}
		})
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
			c := Recheck("Close issue #12?", "#12", func() (Confirm, bool, tea.Cmd) {
				if tt.refused {
					return Confirm{}, false, refusal
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

func TestConfirmLine(t *testing.T) {
	c := Confirm{Question: "Reopen PR #5?"}
	st, k := ConfirmStyles{}, NewConfirmKeys(config.Default().Keys)
	if got, want := c.Line(st, k, 24), "Reopen PR #5?        y/n"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	// A narrow line cuts the question and keeps the keys.
	if got, want := c.Line(st, k, 12), "Reopen … y/n"; got != want {
		t.Errorf("narrow line = %q, want %q", got, want)
	}
}

// A question may name what GitHub calls something, such as a title.
func TestConfirmCleansHostileQuestions(t *testing.T) {
	c := Confirm{Question: "Close " + termtexttest.Hostile + "?"}
	st, k := Theme{}.Confirm(NewIcons(config.IconsUnicode)), NewConfirmKeys(config.Default().Keys)
	for _, w := range []int{20, 80, 300} {
		termtexttest.AssertClean(t, c.Line(st, k, w), w)
		termtexttest.AssertClean(t, strings.Join(c.Lines(st, k, w, ConfirmLines), "\n"), w)
	}
}

func TestConfirmLines(t *testing.T) {
	st, k := ConfirmStyles{}, NewConfirmKeys(config.Default().Keys)
	long := Confirm{Question: "Merge #1234 into release/v2.0-beta with a merge commit?"}
	tests := []struct {
		name string
		c    Confirm
		w, n int
		want []string
	}{
		{"fits on one line", Confirm{Question: "Close PR #5?"}, 20, 2, []string{"Close PR #5?     y/n"}},
		{"wraps to two", long, 40, 2, []string{
			"Merge #1234 into release/v2.0-beta      ",
			"with a merge commit?                 y/n",
		}},
		{"cuts past the last line", long, 24, 2, []string{
			"Merge #1234 into        ",
			"release/v2.0-beta w… y/n",
		}},
		// A branch name wraps whole, and is cut if it is longer than a line.
		{"keeps words whole", Confirm{Question: "Merge #1 into release/v2.0-beta-rc?"}, 30, 2, []string{
			"Merge #1 into                 ",
			"release/v2.0-beta-rc?      y/n",
		}},
		{"cuts a word longer than a line", Confirm{Question: "Merge #1 into release/v2.0-beta-candidate-long?"}, 30, 2, []string{
			"Merge #1 into                 ",
			"release/v2.0-beta-candida… y/n",
		}},
		{"one line cuts", long, 24, 1, []string{"Merge #1234 into re… y/n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.c.Lines(st, k, tt.w, tt.n)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Lines = %q, want %q", got, tt.want)
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
			m := NewConfirmModal(r.confirm("Close issue #12?"), NewConfirmKeys(config.Default().Keys), NewIcons(config.IconsUnicode))
			var msgs []tea.Msg
			for _, k := range tt.keys {
				msgs = append(msgs, runAll(m.Update(press(k)))...)
			}
			closed := slices.Contains(msgs, tea.Msg(CloseModalMsg{Modal: m}))
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
	m := NewConfirmModal(Confirm{Question: "Close issue #12?"}, NewConfirmKeys(config.Default().Keys), NewIcons(config.IconsUnicode))
	if w, h := m.Fit(100, 20); w != len("Close issue #12?")+2+len("y/n") || h != 1 {
		t.Errorf("Fit = %d×%d, want the question and its keys on one line", w, h)
	}
	if w, _ := m.Fit(10, 20); w != 10 {
		t.Errorf("Fit in 10 columns = %d wide", w)
	}
	m.SetSize(21, 1)
	if got, want := m.View(), "Close issue #12?  y/n"; got != want {
		t.Errorf("View = %q, want %q", got, want)
	}
	if m.Title() != "Confirm" || m.Question() != "Close issue #12?" {
		t.Errorf("titled %q asking %q", m.Title(), m.Question())
	}
}

// choosing is a question of two choices, which the key for the method
// steps between.
func choosing(r *ran, i int) Confirm {
	names := []string{"squash", "rebase"}
	c := r.confirm("Merge #1 as " + names[i] + "?")
	c.Cycle = func() Confirm { return choosing(r, (i+1)%len(names)) }
	return c
}

func TestConfirmChoices(t *testing.T) {
	keys := NewConfirmKeys(config.Default().Keys)
	var r ran
	q := choosing(&r, 0)

	next, ok := keys.Step(q, press("tab"))
	if !ok || next.Question != "Merge #1 as rebase?" {
		t.Errorf("tab gives %q, %v; want the next choice", next.Question, ok)
	}
	if _, ok := keys.Step(q, press("y")); ok {
		t.Error("y stepped through the choices")
	}
	// A question with no choices ignores the key, and shows no hint.
	plain := r.confirm("Close issue #12?")
	if _, ok := keys.Step(plain, press("tab")); ok {
		t.Error("tab stepped through a question that has no choices")
	}
	if cmd, done := keys.Answer(plain, press("tab")); cmd != nil || done {
		t.Errorf("tab answered a question that has no choices: %v, %v", cmd != nil, done)
	}
	st := ConfirmStyles{}
	if got := ansi.Strip(plain.Line(st, keys, 40)); !strings.HasSuffix(got, "  y/n") || strings.Contains(got, "tab") {
		t.Errorf("a question with no choices reads %q, want only y/n", got)
	}
	if got := ansi.Strip(q.Line(st, keys, 60)); !strings.HasSuffix(got, "tab method · y/n") {
		t.Errorf("a question with choices reads %q, want the key for the method", got)
	}

	// Help lists the key for the method for a question that has choices
	// alone.
	enabled := func(layers ...keyhelp.Layer) []string {
		var out []string
		for _, l := range layers {
			for _, b := range l.Bindings {
				if b.Enabled() {
					out = append(out, b.Help().Desc)
				}
			}
		}
		return out
	}
	if got := enabled(keys.LayerFor(q)); !slices.Equal(got, []string{"method", "yes", "no"}) {
		t.Errorf("help of a question with choices = %v", got)
	}
	if got := enabled(keys.LayerFor(plain)); !slices.Equal(got, []string{"yes", "no"}) {
		t.Errorf("help of a question with no choices = %v", got)
	}
	if got := enabled(keys.Layer()); slices.Contains(got, "method") {
		t.Errorf("help of a question = %v, want no method", got)
	}

	// In a modal of its own, the question changes in place, and the answer
	// is that of the choice shown.
	m := NewConfirmModal(q, keys, NewIcons(config.IconsUnicode))
	runAll(m.Update(press("tab")))
	if m.Question() != "Merge #1 as rebase?" || r.n != 0 {
		t.Errorf("after tab asks %q, ran %d times; want the next choice, not run", m.Question(), r.n)
	}
	if got := enabled(m.KeyLayers()...); !slices.Contains(got, "method") {
		t.Errorf("help of the modal = %v, want the method", got)
	}
	runAll(m.Update(press("y")))
	if r.n != 1 {
		t.Errorf("ran %d times, want once", r.n)
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
	k := NewConfirmKeys(keys)
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
			cmd, done := k.Answer(r.confirm("Close issue #12?"), press(tt.key))
			if done != tt.done || (r.n == 1) != tt.run || (cmd != nil) != tt.run {
				t.Errorf("%s: done %v, ran %d times; want done %v and run %v", tt.key, done, r.n, tt.done, tt.run)
			}
		})
	}
	// An unbound answer is listed without a key.
	keys.Set("confirm.no", nil)
	if no := NewConfirmKeys(keys).No; no.Enabled() || no.Help().Desc != "no" {
		t.Errorf("unbound no = enabled %v, desc %q; want disabled and described", no.Enabled(), no.Help().Desc)
	}
}
