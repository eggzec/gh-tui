package tui

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
)

// fakeRecall knows repos, and the numbers of testRepo and bubbletea.
type fakeRecall struct {
	repos   []core.RepoRef
	numbers map[core.RepoRef][]Numbered
	owners  []string
	// calls counts the reads, which must stay few per edit.
	calls int
}

func (f *fakeRecall) Repos() []core.RepoRef {
	f.calls++
	return f.repos
}

func (f *fakeRecall) Numbers(repo core.RepoRef) []Numbered {
	f.calls++
	return f.numbers[repo]
}

func (f *fakeRecall) Owners() []string {
	f.calls++
	return f.owners
}

func newFakeRecall() *fakeRecall {
	refs := func(names ...string) []core.RepoRef {
		out := make([]core.RepoRef, 0, len(names))
		for _, n := range names {
			r, _ := core.ParseRepoRef(n)
			out = append(out, r)
		}
		return out
	}
	return &fakeRecall{
		repos: refs(
			"cli/cli", "charmbracelet/bubbles", "charmbracelet/bubbletea", "charmbracelet/lipgloss",
			"charmbracelet/glamour", "eggzec/gh-tui", "eggzec/dotfiles", "golang/go", "golang/tools",
			"charmbracelet/huh", "charmbracelet/x", "rust-lang/go-rust",
		),
		numbers: map[core.RepoRef][]Numbered{
			testRepo: {
				{Number: 131, Title: "Command mode"}, {Number: 130, Title: "Target parser"},
				{Number: 124, Title: "Kind of a number"}, {Number: 13, Title: "Old issue"}, {Number: 7, Title: "First"},
			},
			bubbletea: {{Number: 1813, Title: "v2: key presses"}, {Number: 1698, Title: "Paste in Windows"}},
		},
	}
}

func texts(cs []cmdline.Candidate) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Text)
	}
	return out
}

func TestComplete(t *testing.T) {
	tests := []struct {
		name string
		// line has | at the cursor, or the cursor at its end.
		line string
		// recent are the repositories selected, the latest last; the app
		// opens on testRepo.
		recent []core.RepoRef
		// history are the lines of the command line, oldest first.
		history []string
		want    []string
		// span is the span of the first candidate.
		span [2]int
	}{
		{name: "empty", line: ""},
		{name: "command", line: "g", want: []string{"goto "}, span: [2]int{0, 1}},
		{name: "every command", line: "  ", want: nil},
		{name: "q", line: "q", want: []string{"q"}, span: [2]int{0, 1}},
		{name: "help", line: "he", want: []string{"help"}},
		{name: "refresh", line: "re", want: []string{"refresh"}},
		{name: "raw", line: "ra", want: []string{"raw "}},
		{name: "raw on", line: "raw o", want: []string{"on", "off"}},
		{name: "after refresh", line: "refresh c"},
		{name: "search", line: "sea", want: []string{"search "}},
		{name: "after search", line: "search c"},
		{name: "filter", line: "f", want: []string{"filter"}},
		{name: "search, set and sort", line: "s", want: []string{"search ", "set ", "sort"}},
		{name: "open", line: "op", want: []string{"open "}},
		{name: "open completes as goto", line: "open CLI/", want: []string{"cli/cli"}},
		{name: "open completes numbers", line: "open #13", want: []string{"#131", "#130", "#13"}},
		{name: "command before an argument", line: "go| x", want: []string{"goto"}, span: [2]int{0, 2}},
		{name: "unknown", line: "x"},
		{name: "after an unknown command", line: "nosuch c"},
		{name: "after q", line: "q c"},
		{
			name: "repos, recent first", line: "goto ",
			want: []string{"eggzec/gh-tui", "cli/cli", "charmbracelet/bubbles", "charmbracelet/bubbletea", "charmbracelet/lipgloss", "charmbracelet/glamour", "eggzec/dotfiles", "golang/go"},
			span: [2]int{5, 5},
		},
		{
			name: "prefix of the owner", line: "goto charm",
			want: []string{"charmbracelet/bubbles", "charmbracelet/bubbletea", "charmbracelet/lipgloss", "charmbracelet/glamour", "charmbracelet/huh", "charmbracelet/x"},
			span: [2]int{5, 10},
		},
		{
			name: "prefix of the name, then inside", line: "goto bubble",
			want: []string{"charmbracelet/bubbles", "charmbracelet/bubbletea"},
		},
		{
			name: "the full name, then the name", line: "goto go",
			want: []string{"golang/go", "golang/tools", "rust-lang/go-rust"},
		},
		{
			name: "the name, then inside", line: "goto gl",
			want: []string{"charmbracelet/glamour", "charmbracelet/lipgloss"},
		},
		{name: "any case", line: "goto CLI/", want: []string{"cli/cli"}},
		{name: "recent first", line: "goto golang/", recent: []core.RepoRef{{Owner: "golang", Name: "tools"}}, want: []string{"golang/tools", "golang/go"}},
		{
			name: "recent, then the history", line: "goto ",
			history: []string{"goto golang/tools", "goto #3", "q", "goto https://github.com/rust-lang/go-rust/pull/1", "goto eggzec/gh-tui"},
			want:    []string{"eggzec/gh-tui", "rust-lang/go-rust", "golang/tools", "cli/cli", "charmbracelet/bubbles", "charmbracelet/bubbletea", "charmbracelet/lipgloss", "charmbracelet/glamour"},
		},
		{name: "the history alone", line: "goto zz", history: []string{"goto zz/top", "goto bad name"}, want: []string{"zz/top"}},
		{name: "nothing matches", line: "goto zz"},
		{name: "a link", line: "goto https://github.com/c"},
		{name: "a second word", line: "goto cli/cli c"},
		{
			name: "word under the cursor", line: "goto cha|rm",
			want: []string{"charmbracelet/bubbles", "charmbracelet/bubbletea", "charmbracelet/lipgloss", "charmbracelet/glamour", "charmbracelet/huh", "charmbracelet/x"},
			span: [2]int{5, 10},
		},
		{
			name: "numbers of the repository", line: "goto #",
			want: []string{"#131", "#130", "#124", "#13", "#7"}, span: [2]int{5, 6},
		},
		{name: "numbers by prefix", line: "goto #13", want: []string{"#131", "#130", "#13"}},
		{name: "numbers of another", line: "goto charmbracelet/bubbletea#1", want: []string{"charmbracelet/bubbletea#1813", "charmbracelet/bubbletea#1698"}, span: [2]int{5, 30}},
		{name: "numbers of one unknown", line: "goto a/b#1"},
		{name: "not a number", line: "goto #x"},
		{name: "not a repository", line: "goto a#1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestApp(t, WithRecall(newFakeRecall()))
			for _, r := range tt.recent {
				m.remember(r)
			}
			m.line.SetHistory(tt.history)
			line, cursor := strings.ReplaceAll(tt.line, "|", ""), strings.Index(tt.line, "|")
			if cursor < 0 {
				cursor = len(line)
			}
			got := m.complete(line, cursor)
			want := tt.want
			if len(want) > maxCandidates {
				want = want[:maxCandidates]
			}
			if !slices.Equal(texts(got), want) {
				t.Fatalf("complete(%q) = %q, want %q", tt.line, texts(got), want)
			}
			if len(got) > 0 && tt.span != [2]int{} && (got[0].Start != tt.span[0] || got[0].End != tt.span[1]) {
				t.Errorf("span = %d..%d, want %d..%d", got[0].Start, got[0].End, tt.span[0], tt.span[1])
			}
		})
	}
}

func TestCompleteShowsTitlesAndDetails(t *testing.T) {
	m, _ := newTestApp(t, WithRecall(newFakeRecall()))
	got := m.complete("goto #7", 7)
	if len(got) != 1 || got[0].Label != "#7" || got[0].Detail != "First" {
		t.Errorf("complete = %+v, want #7 with its title", got)
	}
	m.recall.(*fakeRecall).numbers[testRepo][0].Title = "A title much too long to show whole in the row"
	got = m.complete("goto #131", 9)
	if len(got) != 1 || got[0].Detail != "A title much too long to sh…" {
		t.Errorf("complete = %+v, want the title cut short", got)
	}
	got = m.complete("g", 1)
	if len(got) != 1 || got[0].Detail == "" {
		t.Errorf("complete = %+v, want goto with what it does", got)
	}
}

func TestTabCompletesTheLine(t *testing.T) {
	recall := newFakeRecall()
	m, _ := newTestApp(t, WithRecall(recall))
	drive(m, m.key(press(":")))
	h := m.contentHeight()
	typeKeys(m, "g")
	if m.contentHeight() != h-1 {
		t.Errorf("content height = %d with the candidates, want %d", m.contentHeight(), h-1)
	}
	drive(m, m.key(press("tab")))
	typeKeys(m, "bubblet")
	drive(m, m.key(press("tab")))
	if got := m.line.Value(); got != "goto charmbracelet/bubbletea" {
		t.Errorf("line = %q, want it completed", got)
	}
	recall.calls = 0
	typeKeys(m, "#")
	if recall.calls > 2 {
		t.Errorf("an edit read the recall %d times", recall.calls)
	}
	drive(m, m.key(press("tab")))
	if got := m.line.Value(); got != "goto charmbracelet/bubbletea#1813" {
		t.Errorf("line = %q, want the number completed", got)
	}
	drive(m, m.key(press("esc")))
	if m.contentHeight() != h+0 || m.line.Focused() {
		t.Errorf("content height = %d once the line closed, want %d", m.contentHeight(), h)
	}
}

func TestRecentRepos(t *testing.T) {
	m, _ := newTestApp(t)
	for i := range maxRecent + 3 {
		drive(m, func() tea.Msg { return testRepoMsg(i) })
	}
	drive(m, func() tea.Msg { return testRepoMsg(4) })
	if len(m.recent) != maxRecent || m.recent[0] != testRepoMsg(4).Repo || m.recent[1] != testRepoMsg(maxRecent+2).Repo {
		t.Errorf("recent = %v, want the last %d selected, latest first", m.recent, maxRecent)
	}
}

func BenchmarkComplete(b *testing.B) {
	m, _ := benchApp(b)
	m.recall = newFakeRecall()
	b.ReportAllocs()
	for b.Loop() {
		_ = m.complete("goto charm", 10)
		_ = m.complete("goto #1", 7)
	}
}

// testRepoMsg selects repository i of those the tests make up.
func testRepoMsg(i int) ui.RepoMsg {
	return ui.RepoMsg{Repo: core.RepoRef{Owner: "owner", Name: "repo" + strconv.Itoa(i)}}
}

func TestCompleteArgumentsOverAModal(t *testing.T) {
	m, _ := newTestApp(t, WithRecall(newFakeRecall()))
	if got := m.complete("goto #7", 7); len(got) == 0 {
		t.Fatal("goto completes nothing without a modal")
	}
	m.openModal(&fakeModal{title: "Preview"})
	if got := m.complete("goto #7", 7); len(got) != 0 {
		t.Errorf("complete(goto) over a modal = %+v, want nothing, since goto is refused", got)
	}
	if got := m.complete("raw ", 4); len(got) == 0 {
		t.Error("raw completes nothing over a modal, which it runs over")
	}
	// What is typed narrows the suggestions as it does without a modal.
	got := m.complete("raw of", 6)
	if len(got) != 1 || got[0].Text != "off" {
		t.Errorf("complete(raw of) over a modal = %+v, want only off", got)
	}
}
