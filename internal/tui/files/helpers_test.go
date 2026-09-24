package files

import (
	"errors"
	"reflect"
	"slices"
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
// of bubbletea. Of the files of gh-tui, .gitignore fails to load, go.mod is
// binary and README with spaces.md is too large.
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
	f.addBlob(file("main.go", 0), mainGo)
	f.addBlob(file("AGENTS.md", 0), "# AGENTS.md\n\nGuidance for anyone.\n")
	f.addBlob(link, "AGENTS.md")
	f.addBlob(file("go.mod", 0), "\x00\x01binary")
	f.blobErrs[file("README with spaces.md", 0).SHA] = &core.TooLargeError{Size: 3_000, Limit: 1_000}
	f.blobErrs[file(".gitignore", 0).SHA] = errors.New("boom")
	f.addTree(ghTUI, internalSHA, dir("core", "d1"), dir("tui", "d2"))
	f.addTree(other, "", file("tea.go", 10_000), file("go.mod", 300))
	return f
}

const mainGo = "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"

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
		case ui.BaseMsg:
			// The app shows the base, then passes it on.
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

// host stands in for the app around the section: it opens and closes the
// modals the section asks for, sends keys to the top modal, and every
// other message to the section and the open modals.
type host struct {
	s             *Section
	width, height int
	modals        []ui.Modal
	// got holds the messages for the app, in order.
	got []tea.Msg
}

func newHost(s *Section) *host {
	return &host{s: s, width: 60, height: 10}
}

// top returns the modal opened last, or nil.
func (h *host) top() ui.Modal {
	if len(h.modals) == 0 {
		return nil
	}
	return h.modals[len(h.modals)-1]
}

func (h *host) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	switch msg := msg.(type) {
	case nil, spinner.TickMsg:
		return
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
		return
	case ui.OpenModalMsg:
		h.got = append(h.got, msg)
		msg.Modal.SetTheme(testTheme())
		msg.Modal.SetSize(h.width, h.height)
		h.modals = append(h.modals, msg.Modal)
		return
	case ui.CloseModalMsg:
		h.got = append(h.got, msg)
		h.modals = slices.DeleteFunc(h.modals, func(m ui.Modal) bool { return m == msg.Modal })
		return
	case ui.OpenMsg, ui.NotifyMsg:
		h.got = append(h.got, msg)
		return
	case tea.KeyPressMsg:
		h.press(msg)
		return
	}
	if cmds, ok := sequence(msg); ok {
		for _, c := range cmds {
			h.run(c)
		}
		return
	}
	h.run(h.s.Update(msg))
	for _, m := range slices.Clone(h.modals) {
		h.run(m.Update(msg))
	}
}

func (h *host) press(k tea.KeyPressMsg) {
	if m := h.top(); m != nil {
		h.run(m.Update(k))
		return
	}
	h.run(h.s.Update(k))
}

// keys presses each key and runs what it returns.
func (h *host) keys(ks ...string) {
	for _, k := range ks {
		h.press(press(k))
	}
}

// modal returns the text of the top modal, or "" when none is open.
func (h *host) modal() string {
	if m := h.top(); m != nil {
		return ansi.Strip(m.View())
	}
	return ""
}
