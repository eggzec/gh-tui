package pager

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// hostSearch opens the prompt of a pager that holds rows, types query into
// it and submits it, as a host that takes its own keys does.
func hostSearch(t *testing.T, m *Model, query string) {
	t.Helper()
	m.OpenSearch()
	if !m.Prompting() {
		t.Fatal("OpenSearch left the prompt closed")
	}
	for _, r := range query {
		m.UpdatePrompt(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m.SubmitPrompt()
}

func TestHostSearchReadsBack(t *testing.T) {
	m := fresh(t, WithSize(40, 8), WithRows(hostRows{"alpha", "beta", "Alpha beta", "gamma"}))
	if m.MatchStatus() != "" {
		t.Errorf("status %q before a search, want none", m.MatchStatus())
	}
	hostSearch(t, &m, "alpha")
	if m.Prompting() {
		t.Error("the prompt is still open after submit")
	}
	if got := m.Matches(); got != 2 {
		t.Fatalf("%d matches, want 2", got)
	}
	// A submitted search leaves the jump to the host.
	if _, ok := m.Current(); ok {
		t.Error("the search jumped to a match on its own")
	}
	m.JumpFrom(0)
	line, ok := m.Current()
	if !ok || line != 0 {
		t.Errorf("current = %d, %v; want line 0", line, ok)
	}
	if got := m.MatchStatus(); got != "match 1/2" {
		t.Errorf("status %q, want match 1/2", got)
	}
	m.StepMatch(1)
	line, _ = m.Current()
	ranges, cur := m.Ranges(line)
	if line != 2 || len(ranges) != 1 || cur != 0 {
		t.Errorf("after a step: line %d, ranges %v, cur %d; want line 2, one range, cur 0", line, ranges, cur)
	}
	// Another row has its ranges too, none of them current.
	ranges, cur = m.Ranges(0)
	if len(ranges) != 1 || cur != -1 {
		t.Errorf("row 0: ranges %v, cur %d; want one range, cur -1", ranges, cur)
	}
	if r, _ := m.Ranges(99); r != nil {
		t.Errorf("a row out of range has ranges %v", r)
	}
}

func TestHostSearchInvertAndNotes(t *testing.T) {
	m := fresh(t, WithSize(40, 8), WithRows(hostRows{"alpha", "beta", "gamma"}))
	hostSearch(t, &m, "!alpha")
	if got := m.Matches(); got != 2 {
		t.Errorf("%d matches of !alpha, want 2", got)
	}
	if r, _ := m.Ranges(1); r != nil {
		t.Errorf("an inverted search has ranges %v", r)
	}

	hostSearch(t, &m, "zzz")
	if m.Note() != noteNotFound {
		t.Errorf("note %q, want %q", m.Note(), noteNotFound)
	}
	m.ClearNote()
	if m.Note() != "" {
		t.Errorf("note %q after ClearNote", m.Note())
	}

	hostSearch(t, &m, "(")
	if n := m.Note(); n == "" || n == noteNotFound {
		t.Errorf("note %q for an invalid pattern", n)
	}
}

func TestHostPromptCancelAndEmptyBackspace(t *testing.T) {
	m := fresh(t, WithSize(40, 8), WithRows(hostRows{"alpha"}))
	m.OpenSearch()
	m.UpdatePrompt(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if got := m.PromptView(20); len([]rune(got)) == 0 {
		t.Error("empty prompt view")
	}
	m.CancelPrompt()
	if m.Prompting() || m.Query() != "" {
		t.Errorf("after cancel: prompting %v, query %q", m.Prompting(), m.Query())
	}
}
