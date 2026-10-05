package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// sourceModal is a modal that shows a file rendered or as its source, as
// the file preview does, and takes commands unless it types into an
// input.
type sourceModal struct {
	fakeModal
	raw, renders bool
	set          []bool
}

func (s *sourceModal) TakesCommands() bool { return !s.typing }
func (s *sourceModal) Raw() (raw, ok bool) { return s.raw, s.renders }
func (s *sourceModal) SetRaw(raw bool) tea.Cmd {
	s.set = append(s.set, raw)
	s.raw = raw
	return nil
}

func TestRawCommand(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		raw     bool
		renders bool
		toast   string
		set     []bool
	}{
		{name: "on", line: "raw on", renders: true, set: []bool{true}},
		{name: "off", line: "raw off", raw: true, renders: true, set: []bool{false}},
		{name: "alone, rendered", line: "raw", renders: true, toast: "raw is off: the file shows rendered."},
		{name: "alone, raw", line: "raw", raw: true, renders: true, toast: "raw is on: the file shows as its source."},
		{name: "other word", line: "raw yes", renders: true, toast: "Write it as raw on or raw off."},
		{name: "a file that doesn't render", line: "raw on", toast: "The open file has no rendered view."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestApp(t)
			mod := &sourceModal{title: "README.md", raw: tt.raw, renders: tt.renders}
			m.openModal(mod)
			runCommand(t, m, tt.line)
			if !slices.Equal(mod.set, tt.set) {
				t.Errorf("SetRaw calls = %v, want %v", mod.set, tt.set)
			}
			if tt.toast != "" && !hasToast(m, tt.toast) {
				t.Errorf("toast = %q, want %q", toasted(m), tt.toast)
			}
			if !m.isOpen(mod) {
				t.Error("the command closed the modal")
			}
		})
	}
}

func TestRawCommandWithoutAFile(t *testing.T) {
	m, _ := newTestApp(t)
	runCommand(t, m, "raw on")
	if want := "The open file has no rendered view."; !hasToast(m, want) {
		t.Errorf("toast = %q, want %q", toasted(m), want)
	}
}

// The command key opens the line over a modal that takes commands, and
// only while it doesn't type into an input; the help names it then.
func TestCommandKeyOverAModalThatTakesCommands(t *testing.T) {
	m, _ := newTestApp(t)
	mod := &sourceModal{title: "README.md", renders: true}
	m.openModal(mod)
	if rows := helpRows(t, m); !slices.Contains(rows, ": command") {
		t.Errorf("the help of a modal that takes commands lacks the command key: %q", rows)
	}
	drive(m, m.key(press(":")))
	if !m.line.Focused() {
		t.Fatal(": didn't open the line over the modal")
	}
	if strings.Contains(strings.Join(mod.keys(), ""), ":") {
		t.Error("the modal got the command key too")
	}
	drive(m, m.key(press("esc")))
	mod.typing = true
	drive(m, m.key(press(":")))
	if m.line.Focused() {
		t.Fatal(": opened the line over a modal that types it")
	}
	if !strings.Contains(strings.Join(mod.keys(), ""), ":") {
		t.Error("the typing modal didn't get the colon")
	}
	for _, l := range m.layersNow() {
		for _, b := range l.Bindings {
			if slices.Contains(b.Keys(), ":") && b.Enabled() && b.Help().Desc == "command" {
				t.Errorf("the keys of a typing modal hold the command key, in %s", l.Source)
			}
		}
	}
}

// files.markdown sets how the next markdown file shows, for the session.
func TestSetMarkdown(t *testing.T) {
	m, _ := newTestApp(t)
	runCommand(t, m, "set files.markdown=raw")
	if m.cfg.Files.Markdown != "raw" || !hasToast(m, "files.markdown is raw for this session.") {
		t.Errorf("files.markdown = %q, toast %q", m.cfg.Files.Markdown, toasted(m))
	}
	runCommand(t, m, "set files.markdown=html")
	if m.cfg.Files.Markdown != "raw" || !hasToast(m, "rendered or raw") {
		t.Errorf("files.markdown = %q after a bad value, toast %q", m.cfg.Files.Markdown, toasted(m))
	}
}

// pressKeys presses each key in the app of the real sections.
func pressKeys(t *testing.T, m *Model, keys ...string) {
	t.Helper()
	for _, k := range keys {
		msg, ok := keyPress(k)
		if !ok {
			t.Fatalf("can't press %q", k)
		}
		driveKeys(t, m, m.key(msg))
	}
}

// The command key opens the line over the file preview, except while its
// pager types, and the finder and the other modals type it.
func TestCommandKeyOverTheFilePreview(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		open bool
	}{
		{name: "preview", keys: []string{"down", "enter"}, open: true},
		{name: "preview search", keys: []string{"down", "enter", "/"}},
		{name: "preview option", keys: []string{"down", "enter", "-"}},
		{name: "preview count", keys: []string{"down", "enter", "5"}},
		{name: "finder", keys: []string{"t"}},
		{name: "history", keys: []string{"B"}},
		{name: "issue", keys: []string{"3", "enter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newKeysApp(t, true)
			pressKeys(t, m, tt.keys...)
			mod := m.topModal()
			if mod == nil {
				t.Fatal("no modal open")
			}
			pressKeys(t, m, ":")
			if m.line.Focused() != tt.open {
				t.Fatalf("line open = %v, want %v", m.line.Focused(), tt.open)
			}
			if m.topModal() != mod {
				t.Error("the command key closed the modal")
			}
		})
	}
}

// Over the file preview, raw and the commands that don't touch what is
// behind it run; the rest say to close the file first.
func TestCommandsOverTheFilePreview(t *testing.T) {
	m := newKeysApp(t, true)
	pressKeys(t, m, "down", "enter")
	mod := m.topModal()
	s, ok := mod.(ui.Sourced)
	if !ok {
		t.Fatalf("the preview %T shows no source", mod)
	}
	run := func(line string) {
		t.Helper()
		pressKeys(t, m, ":")
		typeKeys(m, line)
		driveKeys(t, m, m.key(enter))
	}
	run("raw on")
	if raw, _ := s.Raw(); !raw {
		t.Error("raw on didn't show the source")
	}
	run("set files.markdown=raw")
	if m.cfg.Files.Markdown != "raw" {
		t.Errorf("set over the preview didn't run: %q", m.cfg.Files.Markdown)
	}
	for _, name := range []string{"goto", "search", "filter", "sort", "config", "auth", "refresh", "copy"} {
		line := name
		if name == "goto" || name == "copy" {
			line += " x"
		}
		run(line)
		if m.topModal() != mod {
			t.Fatalf("%s replaced the preview with %v", name, m.topModal())
		}
		if want := "Close the file first to use " + name + "."; !hasToast(m, want) {
			t.Errorf("%s: toast = %q, want %q", name, toasted(m), want)
		}
	}
	if got := texts(m.complete("r", 1)); !slices.Equal(got, []string{"raw "}) {
		t.Errorf("over the preview r completes %q, want only raw", got)
	}
}

// On a short terminal, the frame of a modal leaves the command line and
// its candidates below it.
func TestFrameLeavesTheCommandLine(t *testing.T) {
	m := newKeysApp(t, true)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 16})
	pressKeys(t, m, "down", "enter", ":")
	typeKeys(m, "r")
	if m.footerHeight() < 2 {
		t.Fatalf("the footer is %d rows, want the line and its candidates", m.footerHeight())
	}
	_, h := m.frameSize()
	if bottom := (m.height-h)/2 + h; bottom > m.height-m.footerHeight() {
		t.Errorf("the frame ends at row %d, over the footer of %d rows of %d", bottom, m.footerHeight(), m.height)
	}
	if s := onScreen(m); !strings.Contains(s, "raw") {
		t.Errorf("the candidates don't show:\n%s", s)
	}
}
