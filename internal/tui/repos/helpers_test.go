package repos

import (
	"reflect"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var testNow = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

func ref(s string) core.RepoRef {
	r, err := core.ParseRepoRef(s)
	if err != nil {
		panic(err)
	}
	return r
}

// sampleRepos covers every column: tags, long names and descriptions,
// missing languages and counts from zero to millions.
func sampleRepos() []core.Repo {
	ago := func(d time.Duration) time.Time { return testNow.Add(-d) }
	return []core.Repo{
		{Ref: ref("eggzec/gh-tui"), Description: "A GitHub client for the terminal, built on the Charm stack", Language: "Go", Stars: 1234, Starred: true, UpdatedAt: ago(3 * time.Hour)},
		{Ref: ref("charmbracelet/bubbletea"), Description: "A powerful little TUI framework 🏗", Language: "Go", Stars: 38_912, Starred: true, UpdatedAt: ago(26 * time.Hour)},
		{Ref: ref("eggzec/dotfiles"), Description: "My dotfiles", Language: "Shell", Stars: 3, Private: true, UpdatedAt: ago(9 * 24 * time.Hour)},
		{Ref: ref("eggzec/cli"), Description: "GitHub’s official command line tool", Language: "Go", Fork: true, UpdatedAt: ago(40 * 24 * time.Hour)},
		{Ref: ref("eggzec/old-experiments"), Description: "Things I tried in 2019.\nMostly abandoned.", Language: "TypeScript", Stars: 12, Archived: true, Private: true, UpdatedAt: ago(800 * 24 * time.Hour)},
		{Ref: ref("torvalds/linux"), Description: "Linux kernel source tree", Language: "C", Stars: 1_234_567, UpdatedAt: ago(5 * time.Minute)},
		{Ref: ref("a-very-long-organization-name/with-a-long-repository-name"), Language: "Jupyter Notebook", Stars: 999, UpdatedAt: ago(10 * time.Second)},
		{Ref: ref("eggzec/notes"), UpdatedAt: ago(2 * 24 * time.Hour)},
	}
}

func testTheme() ui.Theme {
	p, err := config.Default().Palette(true)
	if err != nil {
		panic(err)
	}
	return ui.NewTheme(p, true)
}

// newSection returns a focused section of width by height over svc, with
// its first page loaded.
func newSection(tb testing.TB, svc Service, width, height int, opts ...Option) *Section {
	tb.Helper()
	opts = append([]Option{WithNow(func() time.Time { return testNow })}, opts...)
	s := New(tb.Context(), svc, config.Default().Keys, opts...)
	s.SetTheme(testTheme())
	s.SetSize(width, height)
	s.Focus()
	run(s, s.Init())
	return s
}

// run executes cmd the way the program would, feeding every message back
// to s as the app broadcasts it, and returns the app messages in order.
func run(s *Section, cmd tea.Cmd) []tea.Msg {
	var out []tea.Msg
	var exec func(tea.Cmd)
	exec = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		msg := cmd()
		switch msg := msg.(type) {
		case nil, spinner.TickMsg:
			return
		case tea.BatchMsg:
			for _, c := range msg {
				exec(c)
			}
			return
		case ui.RepoMsg, ui.ShowMsg, ui.OpenMsg, ui.DoneMsg, ui.NotifyMsg:
			out = append(out, msg)
		}
		if cmds, ok := sequence(msg); ok {
			for _, c := range cmds {
				exec(c)
			}
			return
		}
		exec(s.Update(msg))
	}
	exec(cmd)
	return out
}

// sequence unpacks the message of tea.Sequence, whose type is unexported.
func sequence(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return nil, false
	}
	cmds := make([]tea.Cmd, v.Len())
	for i := range cmds {
		cmds[i], _ = reflect.TypeAssert[tea.Cmd](v.Index(i))
	}
	return cmds, true
}

// keys presses each key and runs what it returns.
func keys(s *Section, ks ...string) []tea.Msg {
	out := make([]tea.Msg, 0, len(ks))
	for _, k := range ks {
		out = append(out, run(s, s.Update(press(k)))...)
	}
	return out
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}
