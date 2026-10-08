package tui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// selectSection is a fake section whose cursor is on sel, or on nothing
// unless ok.
type selectSection struct {
	*fakeSection
	sel ui.Selection
	ok  bool
}

func (s *selectSection) Selected() (ui.Selection, bool) { return s.sel, s.ok }

// newSelectApp returns an app whose files, focused, are on sel.
func newSelectApp(t *testing.T, sel ui.Selection, ok bool) *Model {
	t.Helper()
	fakes := []*fakeSection{{title: "Files"}, {title: "Pull requests"}}
	files := &selectSection{fakeSection: fakes[0], sel: sel, ok: ok}
	m := New(t.Context(), config.Default(), Layout{Files: files, Pulls: fakes[1]}, WithRepo(testRepo))
	m.toast.SetDuration(0)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	return m
}

// clipboard returns what msgs put on the clipboard, if anything.
func clipboard(msgs []tea.Msg) []string {
	// tea.SetClipboard's message type is unexported, and holds the text.
	kind := reflect.TypeOf(tea.SetClipboard("")())
	var out []string
	for _, msg := range msgs {
		if reflect.TypeOf(msg) == kind {
			out = append(out, reflect.ValueOf(msg).String())
		}
	}
	return out
}

func TestCopyCommand(t *testing.T) {
	pull := ui.Selection{What: "pull request", URL: "https://github.com/eggzec/gh-tui/pull/7", Repo: testRepo, Number: 7}
	file := ui.Selection{What: "file", URL: "https://github.com/eggzec/gh-tui/blob/0123456789abcdef0123456789abcdef01234567/cmd/gh-tui/main.go",
		Repo: testRepo, SHA: "0123456789abcdef0123456789abcdef01234567", Path: "cmd/gh-tui/main.go"}
	repo := ui.Selection{What: "repository", URL: "https://github.com/eggzec/gh-tui", Repo: testRepo}
	issue := ui.Selection{What: "issue", Repo: testRepo, Number: 3}
	tests := []struct {
		name string
		sel  ui.Selection
		none bool
		line string
		// want is what is copied, if anything, and toast the text of the
		// toast.
		want  string
		toast string
	}{
		{name: "url", sel: pull, line: "copy url", want: pull.URL, toast: "Copied https://github.com/eggzec/gh-tui/pull/7."},
		{name: "ref of a number", sel: pull, line: "copy ref", want: "eggzec/gh-tui#7", toast: "Copied eggzec/gh-tui#7."},
		{name: "ref of a repository", sel: repo, line: "copy ref", want: "eggzec/gh-tui"},
		{name: "sha", sel: file, line: "copy sha", want: file.SHA},
		{name: "path", sel: file, line: "copy path", want: file.Path, toast: "Copied cmd/gh-tui/main.go."},
		{name: "a long url", sel: file, line: "copy url", want: file.URL, toast: "Copied https://github.com/eggzec/gh-tui/blob/0123456789abcdef01234…."},
		{name: "no sha", sel: pull, line: "copy sha", toast: "A pull request has no commit SHA to copy."},
		{name: "no path", sel: repo, line: "copy path", toast: "A repository has no path to copy."},
		{name: "no link", sel: issue, line: "copy url", toast: "An issue has no link to copy."},
		{name: "nothing selected", none: true, line: "copy url", toast: "Nothing is selected to copy."},
		{name: "what", sel: pull, line: "copy", toast: "Copy what? Use copy url, ref, sha or path."},
		{name: "unknown", sel: pull, line: "copy title", toast: `Can't copy "title". Use copy url, ref, sha or path.`},
		{name: "too long", sel: ui.Selection{What: "file", Path: strings.Repeat("a/", maxCopy)}, line: "copy path", toast: "The path is too long to copy."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSelectApp(t, tt.sel, !tt.none)
			msgs := runCommand(t, m, tt.line)
			var want []string
			if tt.want != "" {
				want = []string{tt.want}
			}
			if got := clipboard(msgs); !slices.Equal(got, want) {
				t.Errorf("copied %q, want %q", got, want)
			}
			if tt.toast != "" && !hasToast(m, tt.toast) {
				t.Errorf("toasts lack %q: %s", tt.toast, toasted(m))
			}
		})
	}
}

// TestCopyFromTheFocusedPane checks that copy copies the selection of the
// focused pane, and says so of a pane that selects nothing.
func TestCopyFromTheFocusedPane(t *testing.T) {
	m := newSelectApp(t, ui.Selection{What: "repository", Repo: testRepo}, true)
	drive(m, m.key(press("2")))
	msgs := runCommand(t, m, "copy ref")
	if got := clipboard(msgs); len(got) != 0 || !hasToast(m, "Nothing is selected to copy.") {
		t.Errorf("copied %q from a pane with no selection: %s", got, toasted(m))
	}
	drive(m, m.key(press("1")))
	if got := clipboard(runCommand(t, m, "copy ref")); !slices.Equal(got, []string{"eggzec/gh-tui"}) {
		t.Errorf("copied %q, want the files' selection", got)
	}
}

func TestCompleteCopy(t *testing.T) {
	m, _ := newTestApp(t)
	for line, want := range map[string][]string{
		"copy ":      {"url", "ref", "sha", "path"},
		"copy  s":    {"sha"},
		"copy p":     {"path"},
		"copy x":     nil,
		"copy url x": nil,
		"cop":        {"copy "},
	} {
		if got := texts(m.complete(line, len(line))); !slices.Equal(got, want) {
			t.Errorf("complete(%q) = %q, want %q", line, got, want)
		}
	}
	if got := m.complete("copy s", 6); got[0].Detail != "its commit SHA" || got[0].Start != 5 || got[0].End != 6 {
		t.Errorf("candidate = %+v", got[0])
	}
}

func TestArticle(t *testing.T) {
	for noun, want := range map[string]string{"issue": "An issue", "pull request": "A pull request", "": "The selection"} {
		if got := article(noun); got != want {
			t.Errorf("article(%q) = %q, want %q", noun, got, want)
		}
	}
}
