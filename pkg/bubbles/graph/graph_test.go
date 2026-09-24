package graph

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// source serves commits in chunks of size, with the start index as cursor.
type source struct {
	mu      sync.Mutex
	commits []Commit
	size    int
	fail    map[string]error
	calls   []string
}

func newSource(commits []Commit, size int) *source {
	return &source{commits: commits, size: size, fail: map[string]error{}}
}

func (s *source) fetch(_ context.Context, cursor string) ([]Commit, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, cursor)
	if err := s.fail[cursor]; err != nil {
		return nil, "", err
	}
	start := 0
	if cursor != "" {
		start, _ = strconv.Atoi(cursor)
	}
	end := min(start+s.size, len(s.commits))
	next := ""
	if end < len(s.commits) {
		next = strconv.Itoa(end)
	}
	return slices.Clone(s.commits[start:end]), next, nil
}

func (s *source) setFail(cursor string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		delete(s.fail, cursor)
		return
	}
	s.fail[cursor] = err
}

func (s *source) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// sample is a short history with a merged feature branch and a hotfix,
// newest first.
func sample() []Commit {
	commit := func(id, title, author, age string, parents ...string) Commit {
		return Commit{ID: id, Parents: parents, Short: id[:7], Title: title, Detail: author, Right: age}
	}
	return []Commit{
		commit("7f3a9c1e", "Merge pull request #42 from feat/graph", "octocat", "2h", "c41d2b0a", "e9b8a7f6"),
		commit("e9b8a7f6", "feat: draw lanes", "hubot", "3h", "a1b2c3d4"),
		commit("c41d2b0a", "fix: typo in README", "monalisa", "5h", "9e8d7c6b"),
		commit("a1b2c3d4", "feat: lay out lanes", "hubot", "1d", "5a4b3c2d"),
		commit("9e8d7c6b", "Merge branch 'hotfix'", "octocat", "2d", "5a4b3c2d", "0f1e2d3c"),
		commit("0f1e2d3c", "hotfix: crash on empty repo", "monalisa", "2d", "5a4b3c2d"),
		commit("5a4b3c2d", "chore: bump dependencies", "dependabot", "4d", "1234abcd"),
		commit("1234abcd", "Initial commit", "octocat", "1w"),
	}
}

// history returns n commits, newest first, of a main line that merges a
// two-commit branch every four commits.
func history(n int) []Commit {
	commits := make([]Commit, 0, n)
	add := func(parents ...string) string {
		id := fmt.Sprintf("%07x0", len(commits)*2654435761%(1<<28))
		commits = append(commits, Commit{
			ID: id, Parents: parents, Short: id[:7],
			Title:  fmt.Sprintf("commit number %d", len(commits)),
			Detail: "octocat", Right: strconv.Itoa(len(commits)%24) + "h",
		})
		return id
	}
	head := add()
	for len(commits)+4 <= n {
		a := add(head)
		main := add(head)
		b := add(a)
		head = add(main, b)
	}
	for len(commits) < n {
		head = add(head)
	}
	slices.Reverse(commits)
	return commits
}

// run executes cmd and feeds every resulting message back into m, until no
// commands are left. Spinner ticks are dropped so tests never sleep, and
// the messages for the parent are returned.
func run(tb testing.TB, m Model, cmd tea.Cmd) (_ Model, out []tea.Msg) {
	tb.Helper()
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case spinner.TickMsg:
		case SelectMsg, ChosenMsg:
			out = append(out, msg)
		default:
			var next tea.Cmd
			m, next = m.Update(msg)
			queue = append(queue, next)
		}
	}
	return m, out
}

// load builds a focused graph over src and fetches what fills the window.
func load(tb testing.TB, src *source, opts ...Option) Model {
	tb.Helper()
	opts = append([]Option{WithSize(60, 5), WithFocused(true)}, opts...)
	m := New(src.fetch, opts...)
	m, _ = run(tb, m, m.Init())
	return m
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// keys presses each key, runs the resulting commands, and returns the
// messages for the parent.
func keys(tb testing.TB, m Model, ks ...string) (_ Model, out []tea.Msg) {
	tb.Helper()
	for _, k := range ks {
		var cmd tea.Cmd
		m, cmd = m.Update(press(k))
		var msgs []tea.Msg
		m, msgs = run(tb, m, cmd)
		out = append(out, msgs...)
	}
	return m, out
}

func selectedID(m Model) string {
	c, _ := m.Selected()
	return c.ID
}

// graphs returns the plain graph of every loaded row.
func graphs(m Model) []string {
	out := make([]string, 0, len(m.rows))
	for i := range m.rows {
		out = append(out, plain(m.rows[i].cells))
	}
	return out
}

func TestInitLoadsFirstChunk(t *testing.T) {
	src := newSource(sample(), 3)
	m := New(src.fetch, WithSize(60, 2), WithPrefetch(1))
	if m.Focused() {
		t.Fatal("a new graph should start blurred")
	}
	if text, _ := m.statusLine(); !m.hasStatus() || text == m.emptyLine {
		t.Fatal("a new graph should show the loading row")
	}
	m, out := run(t, m, m.Init())
	if m.Len() != 3 || m.Done() {
		t.Fatalf("Len() = %d, Done() = %v; want 3, false", m.Len(), m.Done())
	}
	want := SelectMsg{ID: m.ID(), Commit: sample()[0]}
	if len(out) != 1 || !sameCommit(out[0], want) {
		t.Fatalf("messages = %v, want the first commit selected", out)
	}
}

// sameCommit reports whether msg is want, comparing commits by ID.
func sameCommit(msg, want tea.Msg) bool {
	switch w := want.(type) {
	case SelectMsg:
		got, ok := msg.(SelectMsg)
		return ok && got.ID == w.ID && got.Commit.ID == w.Commit.ID
	case ChosenMsg:
		got, ok := msg.(ChosenMsg)
		return ok && got.ID == w.ID && got.Commit.ID == w.Commit.ID
	}
	return false
}

func TestKeys(t *testing.T) {
	commits := history(40)
	tests := []struct {
		name    string
		keys    []string
		wantSel int
		wantLen int
	}{
		{"down", []string{"down"}, 1, 10},
		{"j and k", []string{"j", "j", "k"}, 1, 10},
		{"up stops at the newest", []string{"up"}, 0, 10},
		{"page down", []string{"pgdown"}, 5, 20},
		{"page up", []string{"pgdown", "pgup"}, 0, 20},
		{"end goes to the last loaded and fetches more", []string{"G"}, 9, 20},
		{"end again goes on", []string{"G", "G"}, 19, 30},
		{"home", []string{"G", "g"}, 0, 20},
		{"end of history", []string{"G", "G", "G", "G", "G"}, 39, 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := load(t, newSource(commits, 10))
			m, _ = keys(t, m, tt.keys...)
			if m.Index() != tt.wantSel || m.Len() != tt.wantLen {
				t.Fatalf("Index() = %d, Len() = %d; want %d, %d", m.Index(), m.Len(), tt.wantSel, tt.wantLen)
			}
			if m.Index() < m.top || m.Index() >= m.top+m.Height() {
				t.Fatalf("cursor %d outside window [%d, %d)", m.Index(), m.top, m.top+m.Height())
			}
		})
	}
}

func TestBlurredIgnoresKeys(t *testing.T) {
	m := load(t, newSource(sample(), 10), WithFocused(false))
	m, out := keys(t, m, "j", "enter")
	if m.Index() != 0 || len(out) != 0 {
		t.Fatalf("Index() = %d, messages = %v; a blurred graph moved", m.Index(), out)
	}
	m.Focus()
	if m, _ = keys(t, m, "j"); m.Index() != 1 {
		t.Fatal("a focused graph should move")
	}
	m.Blur()
	if m.Focused() {
		t.Fatal("Blur did not blur")
	}
}

func TestSelectAndChoose(t *testing.T) {
	commits := sample()
	m := load(t, newSource(commits, 10))
	id := m.ID()

	m, out := keys(t, m, "j", "j", "k")
	want := []tea.Msg{
		SelectMsg{ID: id, Commit: commits[1]},
		SelectMsg{ID: id, Commit: commits[2]},
		SelectMsg{ID: id, Commit: commits[1]},
	}
	if len(out) != len(want) {
		t.Fatalf("messages = %v, want %v", out, want)
	}
	for i := range want {
		if !sameCommit(out[i], want[i]) {
			t.Fatalf("message %d = %v, want %v", i, out[i], want[i])
		}
	}

	// Keys that leave the cursor where it is select nothing.
	m, out = keys(t, m, "k", "k", "g")
	if len(out) != 1 || !sameCommit(out[0], SelectMsg{ID: id, Commit: commits[0]}) {
		t.Fatalf("messages = %v, want one SelectMsg for the first commit", out)
	}

	_, out = keys(t, m, "enter")
	if len(out) != 1 || !sameCommit(out[0], ChosenMsg{ID: id, Commit: commits[0]}) {
		t.Fatalf("messages = %v, want a ChosenMsg for the first commit", out)
	}
	if c := out[0].(ChosenMsg).Commit; c.Title != commits[0].Title || len(c.Parents) != 2 {
		t.Fatalf("ChosenMsg.Commit = %+v, want the whole commit", c)
	}
}

func TestLayoutAcrossChunks(t *testing.T) {
	commits := history(37)
	whole := load(t, newSource(commits, len(commits)), WithSize(60, 40))
	want := graphs(whole)
	if !slices.Contains(want, "●─╮") {
		t.Fatalf("history has no merge: %q", want)
	}
	for _, size := range []int{1, 2, 3, 5, 10} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			m := load(t, newSource(commits, size), WithSize(60, 40))
			if got := graphs(m); !slices.Equal(got, want) {
				t.Fatalf("graph in chunks of %d:\n%s\nwant:\n%s", size, strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

func TestForkResolvedInALaterChunk(t *testing.T) {
	// The merge's second parent comes in the second chunk, and its lane
	// stays open until then.
	src := newSource(sample(), 2)
	m := New(src.fetch, WithSize(60, 2), WithFocused(true))
	// Take the first chunk before the next one is fetched.
	m, cmd := m.Update(m.fetchCmd()())
	if got := graphs(m); !slices.Equal(got, []string{"●─╮", "│ ●"}) {
		t.Fatalf("first chunk = %q", got)
	}
	if len(m.layout.lanes) != 2 {
		t.Fatalf("open lanes = %q, want both", m.layout.lanes)
	}
	m, _ = run(t, m, cmd)
	m, _ = keys(t, m, "G", "G", "G", "G")
	want := []string{"●─╮", "│ ●", "● │", "│ ●", "●─┼─╮", "│ │ ●", "●─┴─╯", "●"}
	if got := graphs(m); !slices.Equal(got, want) {
		t.Fatalf("graph = %q, want %q", got, want)
	}
	if len(m.layout.lanes) != 0 {
		t.Fatalf("open lanes = %q, want none at the root", m.layout.lanes)
	}
}

func TestPrefetch(t *testing.T) {
	src := newSource(history(100), 20)
	m := load(t, src, WithSize(60, 5), WithPrefetch(3))
	m, _ = keys(t, m, "pgdown", "pgdown", "pgdown")
	if m.Len() != 20 {
		t.Fatalf("Len() = %d before the threshold, want 20", m.Len())
	}
	m, _ = keys(t, m, "j")
	if m.Index() != 16 || m.Len() != 40 {
		t.Fatalf("Index() = %d, Len() = %d; want 16, 40", m.Index(), m.Len())
	}
}

func TestDoneStopsFetching(t *testing.T) {
	src := newSource(sample(), 5)
	m := load(t, src)
	m, _ = keys(t, m, "G", "G", "j", "G")
	if !m.Done() || m.Len() != 8 {
		t.Fatalf("Done() = %v, Len() = %d; want true, 8", m.Done(), m.Len())
	}
	if got := src.callCount(); got != 2 {
		t.Fatalf("fetched %d times, want 2", got)
	}
}

func TestErrorAndRetry(t *testing.T) {
	boom := errors.New("boom\nsecond line")
	src := newSource(sample(), 5)
	src.setFail("5", boom)
	m := load(t, src)
	m, _ = keys(t, m, "G")
	if !errors.Is(m.Err(), boom) || m.Len() != 5 {
		t.Fatalf("Err() = %v, Len() = %d; want %v, 5", m.Err(), m.Len(), boom)
	}
	if !m.KeyMap().Retry.Enabled() {
		t.Fatal("retry should be enabled after an error")
	}
	text, hint := m.statusLine()
	if strings.Contains(text, "second line") || !strings.Contains(hint, "r to retry") {
		t.Fatalf("error row = %q %q", text, hint)
	}
	// Moving does not retry by itself.
	if m, _ = keys(t, m, "k", "j"); src.callCount() != 2 {
		t.Fatalf("fetched %d times, want 2", src.callCount())
	}

	src.setFail("5", nil)
	m, _ = keys(t, m, "r")
	if m.Err() != nil || m.Len() != 8 {
		t.Fatalf("after retry: Err() = %v, Len() = %d", m.Err(), m.Len())
	}
	if m.KeyMap().Retry.Enabled() {
		t.Fatal("retry should be disabled once the fetch succeeds")
	}
}

func TestEmpty(t *testing.T) {
	m := load(t, newSource(nil, 10), WithEmptyText("No commits yet."))
	if !m.Done() || m.Len() != 0 {
		t.Fatalf("Done() = %v, Len() = %d", m.Done(), m.Len())
	}
	if _, ok := m.Selected(); ok {
		t.Fatal("Selected() on an empty graph returned a commit")
	}
	if m, out := keys(t, m, "j", "enter", "G"); len(out) != 0 || m.Index() != 0 {
		t.Fatalf("keys on an empty graph sent %v", out)
	}
	if !strings.Contains(m.View(), "No commits yet.") {
		t.Fatalf("View() = %q, want the empty text", m.View())
	}
	m.SetEmptyText("Nothing here.")
	if !strings.Contains(m.View(), "Nothing here.") {
		t.Fatal("SetEmptyText did not change the view")
	}
}

func TestIgnoresRepeatedAndEmptyIDs(t *testing.T) {
	commits := sample()
	commits = slices.Insert(commits, 2, commits[0], Commit{Title: "no ID"})
	m := load(t, newSource(commits, 3), WithSize(60, 20))
	if m.Len() != len(sample()) {
		t.Fatalf("Len() = %d, want %d", m.Len(), len(sample()))
	}
}

func TestReset(t *testing.T) {
	a := newSource(history(40), 10)
	m := load(t, a)
	m, _ = keys(t, m, "j", "j")
	// A fetch of the old branch in flight is dropped after the Reset.
	_, stale := m.Update(press("G"))

	b := newSource(history(12), 3)
	cmd := m.Reset(b.fetch)
	if m.Len() != 0 || m.Index() != 0 || !m.hasStatus() {
		t.Fatalf("after Reset: Len() = %d, Index() = %d", m.Len(), m.Index())
	}
	m, out := run(t, m, cmd)
	m, _ = run(t, m, stale)
	if m.Len() != 9 || m.rows[0].commit.ID != history(12)[0].ID {
		t.Fatalf("Len() = %d, first = %q; want the new branch", m.Len(), selectedID(m))
	}
	if len(out) != 1 || !sameCommit(out[0], SelectMsg{ID: m.ID(), Commit: history(12)[0]}) {
		t.Fatalf("messages = %v, want the new first commit selected", out)
	}
}

func TestIgnoresOtherInstances(t *testing.T) {
	a := load(t, newSource(sample(), 3))
	b := New(newSource(history(10), 3).fetch)
	if a.ID() == b.ID() {
		t.Fatal("two graphs share an ID")
	}
	a2, cmd := a.Update(b.fetchCmd()())
	if cmd != nil || a2.Len() != a.Len() {
		t.Fatal("graph reacted to another graph's message")
	}
	if _, cmd := a.Update(spinner.TickMsg{ID: -1}); cmd != nil {
		t.Fatal("graph reacted to another spinner's tick")
	}
}

func TestSpinnerStopsWhenLoaded(t *testing.T) {
	m := New(newSource(sample(), 10).fetch, WithSize(60, 5))
	tick := m.spin.Tick()
	m, _ = run(t, m, m.fetchCmd())
	if _, cmd := m.Update(tick); cmd != nil {
		t.Fatal("spinner kept ticking after the load")
	}
}

func TestResizeFetchesWhatTheWindowShows(t *testing.T) {
	src := newSource(history(50), 10)
	m := load(t, src, WithSize(60, 5))
	if m.Len() != 10 {
		t.Fatalf("Len() = %d, want 10", m.Len())
	}
	m.SetSize(60, 25)
	_, cmd := m.Update(nil)
	m, _ = run(t, m, cmd)
	if m.Len() != 30 {
		t.Fatalf("Len() = %d after growing, want 30", m.Len())
	}
}

func TestSetStyles(t *testing.T) {
	m := load(t, newSource(sample(), 10), WithSize(60, 8))
	before := m.View()
	s := DefaultStyles(false)
	s.Lanes = nil
	m.SetStyles(s)
	if m.View() == before {
		t.Fatal("SetStyles did not change the view")
	}
	if ansi.Strip(m.View()) != ansi.Strip(before) {
		t.Fatal("SetStyles changed the text")
	}
	if len(m.Styles().Lanes) != 1 {
		t.Fatal("no lane styles should fall back to one unstyled lane")
	}
}
