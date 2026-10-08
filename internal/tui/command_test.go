package tui

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	ctrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
)

// drive is run that also returns every message it applied, in order.
func drive(m *Model, cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if cmds, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range cmds {
			out = append(out, drive(m, c)...)
		}
		return out
	}
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeFor[tea.Cmd]() {
		var out []tea.Msg
		for i := range v.Len() {
			out = append(out, drive(m, v.Index(i).Interface().(tea.Cmd))...)
		}
		return out
	}
	if _, ok := msg.(spinner.TickMsg); ok || msg == nil {
		// A spinner ticks for as long as a goto waits.
		return nil
	}
	_, next := m.Update(msg)
	return append([]tea.Msg{msg}, drive(m, next)...)
}

// typeKeys presses a key for each character of s.
func typeKeys(m *Model, s string) {
	for _, r := range s {
		drive(m, m.key(press(string(r))))
	}
}

// command opens the command line, types line and runs it, and returns
// the messages that followed.
func runCommand(t *testing.T, m *Model, line string) []tea.Msg {
	t.Helper()
	drive(m, m.key(press(":")))
	if !m.line.Focused() {
		t.Fatalf(": didn't open the command line")
	}
	typeKeys(m, line)
	return drive(m, m.key(enter))
}

// toasted returns the text of the toasts shown, with the edges of the
// toasts and the wrapping of their text removed.
func toasted(m *Model) string {
	words := strings.Fields(ansi.Strip(m.toast.View()))
	words = slices.DeleteFunc(words, func(w string) bool { return w == "▌" })
	return strings.Join(words, " ")
}

// hasToast reports whether a toast shows text, which the toast may have
// wrapped anywhere, even inside a word.
func hasToast(m *Model, text string) bool {
	squeeze := func(s string) string { return strings.Join(strings.Fields(s), "") }
	return strings.Contains(squeeze(toasted(m)), squeeze(text))
}

// drawn reports whether the program's output drew text. The renderer
// redraws only the cells that changed and moves the cursor past the rest,
// so text may reach the output in pieces: drawn compares without styles,
// cursor moves or spaces, and text must be new to the screen everywhere
// but in its spaces.
func drawn(out []byte, text string) bool {
	squeeze := func(s string) string { return strings.Join(strings.Fields(s), "") }
	return strings.Contains(squeeze(ansi.Strip(string(out))), squeeze(text))
}

func quit(msgs []tea.Msg) bool {
	for _, msg := range msgs {
		if _, ok := msg.(tea.QuitMsg); ok {
			return true
		}
	}
	return false
}

// appKind names the apps of the tests: one of fake sections opened on
// testRepo, one with a dashboard opened on it, and one with a search page
// opened on testRepo.
type appKind int

const (
	repoApp appKind = iota
	dashApp
	searchApp
)

func newKindApp(t *testing.T, kind appKind) (*Model, []*fakeSection) {
	t.Helper()
	switch kind {
	case dashApp:
		return newDashApp(t, core.RepoRef{})
	case searchApp:
		return newSearchApp(t, testRepo)
	case repoApp:
	}
	return newTestApp(t)
}

func TestCommandKeyOpensTheLine(t *testing.T) {
	tests := []struct {
		name string
		app  appKind
		// prep puts the app in the state to test.
		prep func(m *Model, fakes []*fakeSection)
		open bool
	}{
		{name: "repo screen", open: true},
		{name: "notifications", prep: func(m *Model, _ []*fakeSection) {
			m.showScreen(notifScreen, 0)
		}, open: true},
		{name: "dashboard", app: dashApp, open: true},
		{name: "repo screen from the dashboard", app: dashApp, prep: func(m *Model, _ []*fakeSection) {
			m.showScreen(repoScreen, 0)
		}, open: true},
		{name: "search page", app: searchApp, prep: func(m *Model, _ []*fakeSection) {
			m.showScreen(searchScreen, 0)
		}, open: true},
		{name: "search box focused", app: searchApp, prep: func(m *Model, fakes []*fakeSection) {
			m.showScreen(searchScreen, 0)
			fakes[5].capturing = true
		}},
		{name: "section capturing", prep: func(_ *Model, fakes []*fakeSection) {
			fakes[0].capturing = true
		}},
		{name: "modal open", prep: func(m *Model, _ []*fakeSection) {
			m.openModal(&fakeModal{title: "Preview"})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, fakes := newKindApp(t, tt.app)
			if tt.prep != nil {
				tt.prep(m, fakes)
			}
			p, mod := m.focused(), m.modal
			drive(m, m.key(press(":")))
			if m.line.Focused() != tt.open {
				t.Fatalf("line open = %v, want %v", m.line.Focused(), tt.open)
			}
			if tt.open {
				if s := onScreen(m); !strings.Contains(s, ":"+linePlaceholder) || strings.Contains(s, "help") {
					t.Errorf("the line should replace the help:\n%s", s)
				}
				return
			}
			// The key goes where it went before, and types a colon there.
			got := false
			if f, ok := mod.(*fakeModal); ok {
				got = strings.Contains(strings.Join(f.keys(), ""), ":")
			} else if f, ok := p.section.(*fakeSection); ok {
				got = f.got(isKey(":"))
			}
			if !got {
				t.Error(": should reach the section or modal that takes keys")
			}
		})
	}
}

func TestLineClosesWithoutQuitting(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{press("esc"), press("ctrl+h")} {
		t.Run(k.String(), func(t *testing.T) {
			m, fakes := newTestApp(t)
			drive(m, m.key(press(":")))
			msgs := drive(m, m.key(k))
			if m.line.Focused() {
				t.Fatal("the line is still open")
			}
			if quit(msgs) {
				t.Fatal("closing the line quit the app")
			}
			if fakes[0].got(isKey(k.String())) {
				t.Error("the key reached the section")
			}
			if s := onScreen(m); !strings.Contains(s, "? help") {
				t.Errorf("the help should be back:\n%s", s)
			}
		})
	}
}

func TestLineTakesEveryKey(t *testing.T) {
	m, fakes := newTestApp(t)
	drive(m, m.key(press(":")))
	typeKeys(m, "qn2?/")
	if m.screen != repoScreen || m.focus != 0 || m.helpOpen() {
		t.Errorf("app keys acted while the line was open: screen %d, focus %d", m.screen, m.focus)
	}
	if fakes[0].got(func(msg tea.Msg) bool { _, ok := msg.(tea.KeyPressMsg); return ok }) {
		t.Error("a section got a key while the line was open")
	}
	if got := m.line.Value(); got != "qn2?/" {
		t.Errorf("line = %q, want what was typed", got)
	}
	m.Update(tea.PasteMsg{Content: "pasted"})
	if got := m.line.Value(); got != "qn2?/pasted" {
		t.Errorf("line = %q, want the paste after what was typed", got)
	}
}

func TestCommands(t *testing.T) {
	tests := []struct {
		line  string
		quits bool
		toast string
	}{
		{line: "q", quits: true},
		{line: "  q  ", quits: true},
		{line: "q now", toast: "The q command takes no argument."},
		{line: "quit", toast: "Unknown command: quit."},
		{line: "Q", toast: "Unknown command: Q."},
		{line: "owner/name", toast: "Unknown command: owner/name."},
		{line: "#12", toast: "Unknown command: #12."},
		{line: "bubbletea", toast: "Unknown command: bubbletea."},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			m, _ := newTestApp(t)
			msgs := runCommand(t, m, tt.line)
			if quit(msgs) != tt.quits {
				t.Errorf("quit = %v, want %v", !tt.quits, tt.quits)
			}
			if m.line.Focused() {
				t.Error("the line should close once it runs")
			}
			if tt.toast != "" && !hasToast(m, tt.toast) {
				t.Errorf("toasts lack %q: %s", tt.toast, toasted(m))
			}
		})
	}
}

func TestCommandKeyIsConfigurable(t *testing.T) {
	cfg := config.Default()
	cfg.Keys.Set(config.ActionCommand, []string{";"})
	cfg.Keys.Set("command_line.cancel", []string{"ctrl+g"})
	fakes := []*fakeSection{{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"}}
	m := New(t.Context(), cfg, Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3]}, WithRepo(testRepo))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	drive(m, m.key(press(":")))
	if m.line.Focused() {
		t.Fatal(": opened the line after it was bound to ;")
	}
	drive(m, m.key(press(";")))
	if !m.line.Focused() {
		t.Fatal("; didn't open the line")
	}
	// Letters type, and the key set for cancel cancels.
	typeKeys(m, "q")
	if !m.line.Focused() || m.line.Value() != "q" {
		t.Fatalf("q should type in the line, got %q", m.line.Value())
	}
	drive(m, m.key(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl}))
	if m.line.Focused() {
		t.Error("ctrl+g, bound to cancel, didn't cancel the line")
	}
	if s := onScreen(m); !strings.Contains(s, "; command") {
		t.Errorf("the help should name the key bound:\n%s", s)
	}
}

func TestLineKeys(t *testing.T) {
	tests := []struct {
		name         string
		set          map[string][]string
		submit, stop []string
	}{
		{name: "default", submit: []string{"enter"}, stop: []string{"esc"}},
		{
			name:   "the keys of select and dismiss are left to the rest of the app",
			set:    map[string][]string{config.ActionSelect: {"enter", "ctrl+j"}, config.ActionDismiss: {"ctrl+g", "f1"}},
			submit: []string{"enter"}, stop: []string{"esc"},
		},
		{
			name:   "its own keys",
			set:    map[string][]string{"command_line.run": {"ctrl+j"}, "command_line.cancel": {"ctrl+g", "f1"}},
			submit: []string{"ctrl+j"}, stop: []string{"ctrl+g", "f1"},
		},
		{
			name:   "unbound cancel leaves no key",
			set:    map[string][]string{"command_line.cancel": {}},
			submit: []string{"enter"}, stop: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := config.Default().Keys
			for action, ks := range tt.set {
				keys.Set(action, ks)
			}
			k := lineKeys(keys)
			if got := k.Submit.Keys(); !slices.Equal(got, tt.submit) {
				t.Errorf("submit keys = %q, want %q", got, tt.submit)
			}
			if got := k.Cancel.Keys(); !slices.Equal(got, tt.stop) {
				t.Errorf("cancel keys = %q, want %q", got, tt.stop)
			}
		})
	}
}

// TestLineCompletesWithItsOwnKey checks that a key set for the command
// line's complete action completes, and that tab, no longer bound, doesn't.
func TestLineCompletesWithItsOwnKey(t *testing.T) {
	cfg := config.Default()
	cfg.Keys.Set("command_line.complete", []string{"ctrl+i"})
	fakes := []*fakeSection{{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"}}
	m := New(t.Context(), cfg, Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3]}, WithRepo(testRepo))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	drive(m, m.key(press(":")))
	typeKeys(m, "go")
	drive(m, m.key(tea.KeyPressMsg{Code: tea.KeyTab}))
	if got := m.line.Value(); got != "go" {
		t.Fatalf("tab completed to %q though complete is bound to ctrl+i", got)
	}
	drive(m, m.key(tea.KeyPressMsg{Code: 'i', Mod: tea.ModCtrl}))
	if got := m.line.Value(); got == "go" {
		t.Error("ctrl+i didn't complete the line")
	}
}

func TestHelpNamesTheCommandKey(t *testing.T) {
	m, fakes := newTestApp(t)
	// At 80 columns the short help still has room for it.
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if s := onScreen(m); !strings.Contains(s, ": command") {
		t.Errorf("short help lacks the command key:\n%s", s)
	}
	if rows := helpRows(t, m); !slices.Contains(rows, ": command") {
		t.Errorf("the help lacks the command key: %q", rows)
	}
	fakes[0].capturing = true
	if s := onScreen(m); strings.Contains(s, ": command") {
		t.Errorf("the help of a section that captures keys names the command key, which it types:\n%s", s)
	}
	fakes[0].capturing = false
	m.openModal(&fakeModal{title: "Preview"})
	if s := onScreen(m); strings.Contains(s, ": command") {
		t.Errorf("the bar over a modal names the command key, which it doesn't take:\n%s", s)
	}
	if rows := helpRows(t, m); slices.Contains(rows, ": command") {
		t.Errorf("the help of a modal names the command key, which it doesn't take: %q", rows)
	}
}

func TestLineView(t *testing.T) {
	cases := []struct {
		name  string
		typed string
	}{
		{name: "empty"},
		{name: "plain", typed: "goto zz"},
		{name: "commands", typed: "g"},
		{name: "repos", typed: "goto "},
		{name: "numbers", typed: "goto #"},
		{name: "settings", typed: "set ui"},
		{name: "values", typed: "set ui.icons="},
	}
	for _, width := range []int{80, 120} {
		for _, c := range cases {
			t.Run(strconv.Itoa(width)+"/"+c.name, func(t *testing.T) {
				m, _ := newTestApp(t, WithRecall(newFakeRecall()))
				m.Update(tea.WindowSizeMsg{Width: width, Height: 12})
				drive(m, m.key(press(":")))
				typeKeys(m, c.typed)
				golden.RequireEqual(t, m.View().Content)
			})
		}
	}
}

func TestProgramQuitsFromTheLine(t *testing.T) {
	_, fakes := newTestApp(t)
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3]}
	app := New(t.Context(), config.Default(), layout, WithRepo(testRepo))
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(80, 24))
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return drawn(out, "Files content")
	}, teatest.WithDuration(5*time.Second))
	// The program takes the keys in order, so the final model tells what
	// they did.
	tm.Send(press(":"))
	tm.Send(press("q"))
	tm.Send(enter)
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*Model)
	if !ok || final.line.Focused() {
		t.Error(":q should quit with the line closed")
	}
}

func BenchmarkViewLine(b *testing.B) {
	m, _ := benchApp(b)
	m.key(press(":"))
	for _, r := range "goto" {
		m.key(press(string(r)))
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdateLine(b *testing.B) {
	m, _ := benchApp(b)
	m.key(press(":"))
	msgs := []tea.KeyPressMsg{press("g"), {Code: tea.KeyBackspace}}
	b.ReportAllocs()
	for b.Loop() {
		for _, msg := range msgs {
			m.Update(msg)
		}
	}
}

// BenchmarkTypeInLine types a command that opens and closes the list of
// candidates, drawing a frame after each key, as the user sees it.
func BenchmarkTypeInLine(b *testing.B) {
	m, _ := benchApp(b)
	m.key(press(":"))
	keys := []tea.KeyPressMsg{press("g"), press("o"), {Code: tea.KeyBackspace}, {Code: tea.KeyBackspace}}
	b.ReportAllocs()
	for b.Loop() {
		for _, k := range keys {
			m.Update(k)
			_ = m.View()
		}
	}
}

func TestKeyPress(t *testing.T) {
	for _, name := range []string{"r", "?", "R", "+", ":", "é", "ctrl+r", "alt+x", "ctrl+alt+x", "shift+tab", "enter", "esc", "space", "pgdown", "f5", "ctrl+space"} {
		msg, ok := keyPress(name)
		if !ok || msg.String() != name {
			t.Errorf("keyPress(%q) = %q, %v, want the key", name, msg.String(), ok)
		}
	}
	for _, name := range []string{"", "rr", "ctrl+", "nosuch+x", "\xff"} {
		if msg, ok := keyPress(name); ok {
			t.Errorf("keyPress(%q) = %q, want none", name, msg.String())
		}
	}
}

// TestPressingCommands checks that a command that does what a key does
// presses that key, as the config binds it, where the key would go.
func TestPressingCommands(t *testing.T) {
	t.Run("refresh", func(t *testing.T) {
		m, fakes := newTestApp(t)
		drive(m, m.key(press("2")))
		runCommand(t, m, "refresh")
		if !fakes[1].got(isKey("r")) || fakes[0].got(isKey("r")) {
			t.Error("refresh didn't press r in the focused pane alone")
		}
	})
	t.Run("refresh bound elsewhere", func(t *testing.T) {
		cfg := config.Default()
		cfg.Keys.Set(config.ActionRefresh, []string{"nosuch+key", "ctrl+r"})
		fakes := []*fakeSection{{title: "Files"}}
		m := New(t.Context(), cfg, Layout{Files: fakes[0]}, WithRepo(testRepo))
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		runCommand(t, m, "refresh")
		if !fakes[0].got(isKey("ctrl+r")) {
			t.Error("refresh didn't press the key the config binds")
		}
	})
	t.Run("help", func(t *testing.T) {
		m, fakes := newTestApp(t)
		runCommand(t, m, "help")
		if !m.helpOpen() || m.keyhelp.Title() != "Help · Files" {
			t.Fatalf("help didn't open the help of the focused pane: open %v, title %q", m.helpOpen(), m.keyhelp.Title())
		}
		if fakes[0].got(isKey("?")) {
			t.Error("the key reached the section")
		}
	})
	t.Run("no argument", func(t *testing.T) {
		m, fakes := newTestApp(t)
		runCommand(t, m, "refresh now")
		if !hasToast(m, "The refresh command takes no argument.") || fakes[0].got(isKey("r")) {
			t.Errorf("refresh with an argument ran: %s", toasted(m))
		}
	})
}

// Backspace on an empty command line does nothing, so a held key can't close
// the line and then go back a screen; ctrl+h still closes it.
func TestBackspaceKeepsAnEmptyCommandLineOpen(t *testing.T) {
	m, _ := newDashApp(t, core.RepoRef{})
	run(m, m.key(press("I")))
	run(m, m.key(press(":")))
	run(m, m.key(press("backspace")))
	run(m, m.key(press("backspace")))
	if !m.line.Focused() || m.screen != notifScreen {
		t.Fatalf("line open = %v on screen %d, want it open on the notifications", m.line.Focused(), m.screen)
	}
	run(m, m.key(press("ctrl+h")))
	if m.line.Focused() {
		t.Error("ctrl+h left the empty line open")
	}
}
