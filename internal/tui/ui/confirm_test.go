package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// press returns the key press of k, as a terminal sends it.
func press(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
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
			cmd, done := DefaultConfirmKeys().Answer(r.confirm("Close issue #12?"), press(tt.key))
			if done != tt.done || (r.n == 1) != tt.run || (cmd != nil) != tt.run {
				t.Errorf("%s: done %v, ran %d times, cmd %v; want done %v and run %v", tt.key, done, r.n, cmd != nil, tt.done, tt.run)
			}
		})
	}
}

func TestConfirmLine(t *testing.T) {
	c := Confirm{Question: "Reopen PR #5?"}
	st, k := ConfirmStyles{}, DefaultConfirmKeys()
	if got, want := c.Line(st, k, 24), "Reopen PR #5?        y/n"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	// A narrow line cuts the question and keeps the keys.
	if got, want := c.Line(st, k, 12), "Reopen … y/n"; got != want {
		t.Errorf("narrow line = %q, want %q", got, want)
	}
}

func TestConfirmLines(t *testing.T) {
	st, k := ConfirmStyles{}, DefaultConfirmKeys()
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
			m := NewConfirmModal(r.confirm("Close issue #12?"))
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
	m := NewConfirmModal(Confirm{Question: "Close issue #12?"})
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
