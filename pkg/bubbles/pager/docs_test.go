package pager

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// settle runs cmd and hands what it sends to m, and what that sends, as a
// program would. It must not be given a spinner's tick.
func settle(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = settle(m, c)
		}
		return m
	}
	if msg == nil {
		return m
	}
	m, next := m.Update(msg)
	return settle(m, next)
}

// docsPager returns a focused pager of 40 by 8 cells that holds docs.
func docsPager(tb testing.TB, docs ...Doc) Model {
	tb.Helper()
	m := fresh(tb, WithSize(40, 8))
	m.Focus()
	m = settle(m, m.SetDocs(docs))
	return m
}

func step(tb testing.TB, m Model, delta int) Model {
	tb.Helper()
	cmd, ok := m.Step(delta)
	if !ok {
		tb.Fatalf("Step(%d) from document %d didn't move", delta, m.Doc())
	}
	return settle(m, cmd)
}

func TestDocsKeepTheirOwnState(t *testing.T) {
	m := docsPager(t, Doc{Name: "a.txt", Text: numbered(100)}, Doc{Name: "b.txt", Text: numbered(60)})
	m, _ = keys(t, m, "j", "j", "j", "j", "j")
	m = typeText(t, m, "/")
	m = typeText(t, m, "xxx 4")
	m, _ = keys(t, m, "enter")
	m.mark = 7
	top, query, matches := m.top, m.Query(), m.Matches()
	if top == 0 || query == "" || matches == 0 {
		t.Fatalf("setup: top %d, query %q, %d matches", top, query, matches)
	}

	m = step(t, m, 1)
	if m.Name() != "b.txt" || m.top != 0 || m.Query() != "" || m.mark != -1 {
		t.Fatalf("document b shows %q at %d with query %q, mark %d", m.Name(), m.top, m.Query(), m.mark)
	}
	m, _ = keys(t, m, "&")
	m = typeText(t, m, "xx")
	m, _ = keys(t, m, "enter")
	filter, shown := m.Filter(), m.Shown()
	if filter != "xx" || shown >= m.Lines() {
		t.Fatalf("filter %q shows %d of %d", filter, shown, m.Lines())
	}

	m = step(t, m, -1)
	if m.Name() != "a.txt" || m.top != top || m.Query() != query || m.Matches() != matches || m.mark != 7 || m.Filter() != "" {
		t.Errorf("document a back at %d with query %q (%d matches), mark %d, filter %q; want %d, %q (%d), 7, none",
			m.top, m.Query(), m.Matches(), m.mark, m.Filter(), top, query, matches)
	}
	m = step(t, m, 1)
	if m.Filter() != filter || m.Shown() != shown || m.Query() != "" {
		t.Errorf("document b back with filter %q showing %d, query %q; want %q, %d", m.Filter(), m.Shown(), m.Query(), filter, shown)
	}
}

func TestDocsSetContentDropsTheList(t *testing.T) {
	m := docsPager(t, Doc{Name: "a", Text: "one"}, Doc{Name: "b", Text: "two"})
	m = step(t, m, 1)
	m.SetContent("c", "three")
	if m.Docs() != 1 || m.Doc() != 0 || m.Name() != "c" {
		t.Errorf("after SetContent: %d documents, at %d, showing %q", m.Docs(), m.Doc(), m.Name())
	}
	if _, ok := m.Step(1); ok {
		t.Error("Step moved in a single document")
	}
}

func TestDocsStepStopsAtTheEnds(t *testing.T) {
	m := docsPager(t, Doc{Name: "a", Text: "1"}, Doc{Name: "b", Text: "2"}, Doc{Name: "c", Text: "3"})
	if _, ok := m.Step(-1); ok {
		t.Error("Step(-1) moved from the first")
	}
	m = step(t, m, 5)
	if m.Doc() != 2 || m.Name() != "c" {
		t.Errorf("Step(5) shows %d, %q; want the last", m.Doc(), m.Name())
	}
	if _, ok := m.Step(1); ok {
		t.Error("Step(1) moved from the last")
	}
	m = step(t, m, -1)
	if m.Doc() != 1 {
		t.Errorf("Step(-1) shows %d, want 1", m.Doc())
	}
}

func TestDocsCount(t *testing.T) {
	m := fresh(t)
	if m.Docs() != 1 || m.Doc() != 0 {
		t.Errorf("empty pager: %d documents at %d", m.Docs(), m.Doc())
	}
	m.SetContent("a", "x")
	if i := m.Add(Doc{Name: "b", Text: "y"}); i != 1 || m.Docs() != 2 || m.Doc() != 0 || m.Name() != "a" {
		t.Errorf("Add gave %d: %d documents at %d showing %q", i, m.Docs(), m.Doc(), m.Name())
	}
	m.SetDocs([]Doc{{Name: "p"}, {Name: "q"}, {Name: "r"}})
	if m.Docs() != 3 || m.Doc() != 0 || m.Name() != "p" {
		t.Errorf("SetDocs: %d documents at %d showing %q", m.Docs(), m.Doc(), m.Name())
	}
	m.SetDocs(nil)
	if m.Docs() != 1 {
		t.Errorf("SetDocs(nil): %d documents", m.Docs())
	}
}

// Work started before a step sends results that match no document, and the
// document it left restarts it when it is shown again.
func TestDocsStepDropsAndRestartsWork(t *testing.T) {
	big := numbered(30000)
	if len(big) < syncLimit {
		t.Fatalf("test text is %d bytes, under %d", len(big), syncLimit)
	}
	for _, tc := range []struct {
		name  string
		start func(m *Model) tea.Cmd
		done  func(m Model) bool
	}{
		{"search", func(m *Model) tea.Cmd { return m.SetSearch("xxx 2") }, func(m Model) bool { return m.Matches() > 0 && !m.search.running }},
		{"filter", func(m *Model) tea.Cmd {
			return m.project(projection{filter: filter{query: "xx", re: regexp.MustCompile("xx")}})
		}, func(m Model) bool { return !m.Projecting() && m.Shown() < m.Lines() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := docsPager(t, Doc{Name: "a.txt", Text: big}, Doc{Name: "b.txt", Text: numbered(20)})
			stale := tc.start(&m)
			if stale == nil {
				t.Fatal("work ran at once")
			}
			cmd, _ := m.Step(1)
			m = settle(m, cmd)
			// What the work found arrives after the step.
			m = settle(m, stale)
			if m.Matches() != 0 || m.Query() != "" || m.Filter() != "" || m.Shown() != m.Lines() {
				t.Errorf("document b took the result: %d matches, query %q, filter %q, shows %d of %d",
					m.Matches(), m.Query(), m.Filter(), m.Shown(), m.Lines())
			}
			cmd, _ = m.Step(-1)
			if cmd == nil {
				t.Fatal("returning restarted nothing")
			}
			if m = settle(m, cmd); !tc.done(m) {
				t.Errorf("work didn't finish after returning: %d matches, shows %d of %d", m.Matches(), m.Shown(), m.Lines())
			}
		})
	}
}

func TestDocsStepDropsAndRestartsHighlight(t *testing.T) {
	m := fresh(t, WithSize(40, 8))
	m.Focus()
	stale := m.SetDocs([]Doc{{Name: "a.go", Text: goSource}, {Name: "b.txt", Text: "plain"}})
	if stale == nil {
		t.Fatal("no highlight started")
	}
	m = step(t, m, 1)
	m = settle(m, stale)
	if m.spans != nil {
		t.Error("document b took the highlight of document a")
	}
	cmd, _ := m.Step(-1)
	if cmd == nil {
		t.Fatal("returning didn't highlight again")
	}
	if m = settle(m, cmd); m.spans == nil {
		t.Error("document a isn't highlighted after returning")
	}
}

func TestDocsCopiesDoNotShareSaved(t *testing.T) {
	m := docsPager(t, Doc{Name: "a", Text: numbered(30)}, Doc{Name: "b", Text: "2"})
	other := m
	m = step(t, m, 1)
	if !other.docs[0].pending {
		t.Error("a step of one copy wrote the saved documents of the other")
	}
	if other.Doc() != 0 || other.Name() != "a" {
		t.Errorf("the other copy moved to %d, %q", other.Doc(), other.Name())
	}
	other.Add(Doc{Name: "c"})
	if m.Docs() != 2 {
		t.Errorf("Add to one copy gave the other %d documents", m.Docs())
	}
}

func TestDocsSetDoc(t *testing.T) {
	m := docsPager(t, Doc{Name: "a", Text: "one"}, Doc{Name: "b", Text: "two"})
	m.SetDoc(1, Doc{Name: "c", Text: "x\ny\nz"})
	if m.Name() != "a" || m.Docs() != 2 {
		t.Fatalf("SetDoc of another document changed what is shown: %q, %d", m.Name(), m.Docs())
	}
	m = step(t, m, 1)
	if m.Name() != "c" || m.Lines() != 3 {
		t.Errorf("document 1 shows %q with %d lines, want c with 3", m.Name(), m.Lines())
	}

	// A document that waits for its text, filled in once it arrived.
	m = fresh(t, WithSize(40, 8))
	m.SetLoading("a")
	m.Add(Doc{Name: "b", Text: "two"})
	if m.state != stateLoading || m.Docs() != 2 {
		t.Fatalf("setup: state %v, %d documents", m.state, m.Docs())
	}
	m.SetDoc(0, Doc{Name: "a", Text: "arrived"})
	if m.state != stateReady || m.Docs() != 2 || m.Lines() != 1 {
		t.Errorf("after SetDoc: state %v, %d documents, %d lines", m.state, m.Docs(), m.Lines())
	}
}

func TestDocsRenderedFitsAgainAfterAStep(t *testing.T) {
	r := &wrapWords{src: words(400)}
	m := rendered(t, r)
	m.Add(Doc{Name: "b", Text: "two"})
	m.top = m.Lines() / 3
	word := firstWord(m)
	n, _ := strconv.Atoi(strings.TrimPrefix(word, "word"))

	m = step(t, m, 1)
	m.SetSize(20, 6)
	calls := len(r.widths)
	m = step(t, m, -1)
	if len(r.widths) != calls+1 || r.widths[calls] != m.textWidth() {
		t.Errorf("rendered at %v after the step, want once more at %d", r.widths[calls:], m.textWidth())
	}
	got, _ := strconv.Atoi(strings.TrimPrefix(firstWord(m), "word"))
	if abs(got-n) > 8 {
		t.Errorf("the top is %s at the new width, want about %s", firstWord(m), word)
	}
}
