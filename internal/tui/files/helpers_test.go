package files

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var (
	ghTUI = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	other = core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
)

// SHAs of the sample directories.
const (
	cmdSHA      = "c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0"
	ghTUISHA    = "c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1"
	internalSHA = "d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0"
)

func dir(name, sha string) core.TreeEntry {
	return core.TreeEntry{Path: name, Name: name, Type: core.EntryTree, Mode: "040000", SHA: sha}
}

func file(name string, size int64) core.TreeEntry {
	return core.TreeEntry{Path: name, Name: name, Type: core.EntryBlob, Mode: "100644", SHA: "b-" + name, Size: size}
}

// sampleFake holds the trees of gh-tui, with a submodule and a symlink, and
// of bubbletea.
func sampleFake() *fake {
	f := newFake()
	link := file("CLAUDE.md", 12)
	link.Mode = core.ModeSymlink
	f.addTree(ghTUI, "",
		dir("cmd", cmdSHA),
		dir("internal", internalSHA),
		core.TreeEntry{Path: "vendor-lib", Name: "vendor-lib", Type: core.EntryCommit, Mode: "160000", SHA: "e0"},
		file(".gitignore", 20),
		file("AGENTS.md", 12_000),
		link,
		file("go.mod", 900),
		file("README with spaces.md", 3_000),
	)
	f.addTree(ghTUI, cmdSHA, dir("gh-tui", ghTUISHA))
	f.addTree(ghTUI, ghTUISHA, file("main.go", 2_000))
	f.addTree(ghTUI, internalSHA, dir("core", "d1"), dir("tui", "d2"))
	f.addTree(other, "", file("tea.go", 10_000), file("go.mod", 300))
	return f
}

func testTheme() ui.Theme {
	p, err := config.Default().Palette(true)
	if err != nil {
		panic(err)
	}
	return ui.NewTheme(p, true)
}

// newSection returns a focused section of width by height over svc.
func newSection(tb testing.TB, svc Service, width, height int, opts ...Option) *Section {
	tb.Helper()
	s := New(tb.Context(), svc, config.Default().Keys, opts...)
	s.SetTheme(testTheme())
	s.SetSize(width, height)
	s.Focus()
	run(s, s.Init())
	return s
}

// loaded returns a section showing the files of gh-tui.
func loaded(tb testing.TB, svc Service, width, height int) *Section {
	tb.Helper()
	return newSection(tb, svc, width, height, WithRepo(ghTUI))
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
		case ui.OpenMsg, ui.NotifyMsg, ui.OpenModalMsg, ui.CloseModalMsg:
			out = append(out, msg)
			return
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
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// screen returns the view without styles.
func screen(s *Section) string {
	return ansi.Strip(s.View())
}

// assertFits checks that v is exactly height lines of exactly width cells.
func assertFits(tb testing.TB, v string, width, height int) {
	tb.Helper()
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		tb.Fatalf("view has %d lines, want %d", len(lines), height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			tb.Errorf("line %d is %d cells wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}
