package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// fakeSection records what the app asks of it.
type fakeSection struct {
	title         string
	badge         string
	inits         int
	focused       bool
	width, height int
	themed        bool
	msgs          []tea.Msg
	capturing     bool
	// keyMap, if set, is the help of the section.
	keyMap help.KeyMap
	// reply, if set, answers each message.
	reply func(tea.Msg) tea.Cmd
}

func (s *fakeSection) Title() string { return s.title }
func (s *fakeSection) Badge() string { return s.badge }
func (s *fakeSection) Init() tea.Cmd { s.inits++; return nil }
func (s *fakeSection) Update(msg tea.Msg) tea.Cmd {
	s.msgs = append(s.msgs, msg)
	if s.reply != nil {
		return s.reply(msg)
	}
	return nil
}
func (s *fakeSection) View() string      { return s.title + " content" }
func (s *fakeSection) SetSize(w, h int)  { s.width, s.height = w, h }
func (s *fakeSection) SetTheme(ui.Theme) { s.themed = true }
func (s *fakeSection) Focus()            { s.focused = true }
func (s *fakeSection) Blur()             { s.focused = false }
func (s *fakeSection) KeyLayers() []keyhelp.Layer {
	var km help.KeyMap = sectionKeys{}
	if s.keyMap != nil {
		km = s.keyMap
	}
	return []keyhelp.Layer{keyhelp.FromHelp(s.title, km, s.capturing)}
}
func (s *fakeSection) Capturing() bool { return s.capturing }
func (s *fakeSection) got(match func(tea.Msg) bool) bool {
	return slices.ContainsFunc(s.msgs, match)
}

type sectionKeys struct{}

func (sectionKeys) ShortHelp() []key.Binding {
	return []key.Binding{key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "close"))}
}
func (k sectionKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

var testRepo = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}

// newApp returns an app of fake sections, files, pull requests, issues and
// notifications in that order, on an 80x24 terminal. A zero repo starts
// it on the notifications.
func newApp(t *testing.T, repo core.RepoRef, opts ...Option) (*Model, []*fakeSection) {
	t.Helper()
	fakes := []*fakeSection{{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"}}
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3]}
	if repo != (core.RepoRef{}) {
		opts = append([]Option{WithRepo(repo)}, opts...)
	}
	m := New(t.Context(), config.Default(), layout, opts...)
	// Toasts that never expire keep run from waiting on their timers.
	m.toast.SetDuration(0)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m, fakes
}

// newTestApp returns an app opened on testRepo.
func newTestApp(t *testing.T, opts ...Option) (*Model, []*fakeSection) {
	t.Helper()
	return newApp(t, testRepo, opts...)
}

// onScreen is the text on screen with styles removed and whitespace
// collapsed, so text that a toast wraps still matches.
func onScreen(m *Model) string {
	return strings.Join(strings.Fields(ansi.Strip(m.View().Content)), " ")
}

// run applies the command's message, and those of batched and sequenced
// commands, to m, in order.
func run(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if cmds, ok := msg.(tea.BatchMsg); ok {
		for _, c := range cmds {
			run(m, c)
		}
		return
	}
	// tea.Sequence's message type is unexported, but it is a list of
	// commands too.
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeFor[tea.Cmd]() {
		for i := range v.Len() {
			run(m, v.Index(i).Interface().(tea.Cmd))
		}
		return
	}
	if msg == nil {
		return
	}
	_, next := m.Update(msg)
	run(m, next)
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

func isKey(k string) func(tea.Msg) bool {
	return func(msg tea.Msg) bool { p, ok := msg.(tea.KeyPressMsg); return ok && p.String() == k }
}

func focusedTitles(fakes []*fakeSection) []string {
	var out []string
	for _, f := range fakes {
		if f.focused {
			out = append(out, f.title)
		}
	}
	return out
}

func TestOpensOnRepoWithFilesFocused(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	for _, f := range fakes {
		if !f.themed {
			t.Errorf("%s has no theme", f.title)
		}
		if !f.got(func(msg tea.Msg) bool { r, ok := msg.(ui.RepoMsg); return ok && r.Repo == testRepo }) {
			t.Errorf("%s wasn't told the repository", f.title)
		}
	}
	if m.screen != repoScreen {
		t.Error("the app didn't open on the repository screen")
	}
	if got := focusedTitles(fakes); !slices.Equal(got, []string{"Files"}) {
		t.Errorf("focused = %v, want only Files", got)
	}
	if s := onScreen(m); !strings.Contains(s, "eggzec/gh-tui") {
		t.Errorf("header lacks the repository:\n%s", s)
	}
}

func TestOpensOnNotificationsWithoutRepo(t *testing.T) {
	t.Parallel()
	m, fakes := newApp(t, core.RepoRef{})
	if m.screen != notifScreen {
		t.Fatal("the app didn't open on the notifications")
	}
	if got := focusedTitles(fakes); !slices.Equal(got, []string{"Notifications"}) {
		t.Errorf("focused = %v, want only Notifications", got)
	}
	s := onScreen(m)
	if !strings.Contains(s, "Notifications content") || strings.Contains(s, "Files content") {
		t.Errorf("screen isn't the notifications:\n%s", s)
	}
	if !strings.Contains(s, "─ Notifications ─") || strings.Contains(s, "to search") {
		t.Errorf("header doesn't name the notifications:\n%s", s)
	}
}

func TestPanesAreSizedToTheirFrames(t *testing.T) {
	t.Parallel()
	_, fakes := newTestApp(t)
	// The header and the help line take a row each; the files take two
	// fifths of the width, and the other two panes share the height.
	want := map[string][2]int{
		"Files":         {32 - 2, 22 - 2},
		"Pull requests": {48 - 2, 11 - 2},
		"Issues":        {48 - 2, 11 - 2},
		"Notifications": {80 - 2, 22 - 2},
	}
	for _, f := range fakes {
		if w := want[f.title]; f.width != w[0] || f.height != w[1] {
			t.Errorf("%s size = %dx%d, want %dx%d", f.title, f.width, f.height, w[0], w[1])
		}
	}
}

func TestFilesWidthIsBounded(t *testing.T) {
	t.Parallel()
	for width, want := range map[int]int{70: 28, 80: 32, 100: 40, 200: 60, 20: 24} {
		if got := filesWidth(width); got != want {
			t.Errorf("filesWidth(%d) = %d, want %d", width, got, want)
		}
	}
}

func TestRepoScreenIsFramed(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	out := m.View().Content
	lines := strings.Split(out, "\n")
	if len(lines) != 24 {
		t.Fatalf("screen has %d lines, want 24", len(lines))
	}
	for i, l := range lines[:23] {
		if w := ansi.StringWidth(l); w != 80 {
			t.Errorf("line %d is %d wide, want 80: %q", i, w, ansi.Strip(l))
		}
	}
	plain := ansi.Strip(out)
	for _, want := range []string{
		"─ eggzec/gh-tui ─",
		"╭─[1] Files ─", "╭─[2] Pull requests ─", "╭─[3] Issues ─",
		"│Files content", "│Pull requests content", "│Issues content",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("screen lacks %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "Notifications content") {
		t.Error("the repository screen shows the notifications")
	}
}

func TestFocusedFrameTakesTheAccent(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	focused, blurred := m.st.focusEdge.Render("╭─"), m.st.edge.Render("╭─")
	if focused == blurred {
		t.Fatal("the focused and blurred edges look the same")
	}
	if !strings.HasPrefix(m.panes[0].top, focused) || !strings.HasPrefix(m.panes[1].top, blurred) {
		t.Error("the focused pane's edge isn't the accent, or another pane's is")
	}
	run(m, m.key(press("tab")))
	if !strings.HasPrefix(m.panes[1].top, focused) || !strings.HasPrefix(m.panes[0].top, blurred) {
		t.Error("the accent didn't follow the focus")
	}
}

func TestNarrowShowsTheFocusedPaneAlone(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	for _, f := range fakes[:3] {
		if f.width != 58 || f.height != 20 {
			t.Errorf("%s size = %dx%d, want the full 58x20", f.title, f.width, f.height)
		}
	}
	s := onScreen(m)
	if !strings.Contains(s, "[1] Files") || strings.Contains(s, "Pull requests content") {
		t.Errorf("narrow screen should show only the files:\n%s", s)
	}
	run(m, m.key(press("tab")))
	if s = onScreen(m); !strings.Contains(s, "Pull requests content") || strings.Contains(s, "Files content") {
		t.Errorf("tab should show the pull requests alone:\n%s", s)
	}
}

func TestZoom(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	tests := []struct {
		name string
		// key is pressed after the terminal is resized to width, if set.
		key           string
		width, height int
		zoom          bool
		// shown are the panes on screen, the first of them focused.
		shown []string
		// toSection is set when the key reaches the focused section.
		toSection bool
	}{
		{name: "z zooms", key: "z", zoom: true, shown: []string{"Files"}},
		{name: "tab keeps the zoom", key: "tab", zoom: true, shown: []string{"Pull requests"}},
		{name: "a pane key keeps the zoom", key: "3", zoom: true, shown: []string{"Issues"}},
		{name: "shift+tab keeps the zoom", key: "shift+tab", zoom: true, shown: []string{"Pull requests"}},
		{name: "a resize keeps the zoom", width: 160, height: 40, zoom: true, shown: []string{"Pull requests"}},
		{name: "narrow and zoomed", width: 60, height: 24, zoom: true, shown: []string{"Pull requests"}},
		{name: "esc at 60 columns goes to the pane", key: "esc", zoom: true, shown: []string{"Pull requests"}, toSection: true},
		{name: "wide again", width: 120, height: 36, zoom: true, shown: []string{"Pull requests"}},
		{name: "esc keeps the zoom", key: "esc", zoom: true, shown: []string{"Pull requests"}, toSection: true},
		{name: "z unzooms", key: "z", shown: []string{"Pull requests", "Files", "Issues"}},
		{name: "z zooms again", key: "z", zoom: true, shown: []string{"Pull requests"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.width > 0 {
				m.Update(tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			}
			for _, f := range fakes {
				f.msgs = nil
			}
			if tt.key != "" {
				run(m, m.key(press(tt.key)))
			}
			if m.zoom != tt.zoom {
				t.Errorf("zoom = %v, want %v", m.zoom, tt.zoom)
			}
			if got := focusedTitles(fakes); !slices.Equal(got, tt.shown[:1]) {
				t.Errorf("focused = %v, want %s", got, tt.shown[0])
			}
			s := onScreen(m)
			for _, f := range fakes[:3] {
				if on := strings.Contains(s, f.title+" content"); on != slices.Contains(tt.shown, f.title) {
					t.Errorf("%s on screen = %v, want %v:\n%s", f.title, on, !on, s)
				}
			}
			if f := fakes[slices.IndexFunc(fakes, func(f *fakeSection) bool { return f.focused })]; tt.zoom && f.width != m.width-2 {
				t.Errorf("the zoomed pane is %d wide, want the %d inside the frame", f.width, m.width-2)
			}
			if got := slices.ContainsFunc(fakes, func(f *fakeSection) bool { return f.got(isKey(tt.key)) }); tt.key != "" && got != tt.toSection {
				t.Errorf("%s reached a section = %v, want %v", tt.key, got, tt.toSection)
			}
		})
	}
}

func TestZoomNeedsTheRepoScreen(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	run(m, m.key(press("I")))
	run(m, m.key(press("z")))
	if m.zoom || !fakes[3].got(isKey("z")) {
		t.Error("z on the notifications should go to them, not zoom the repository screen")
	}
	pulls := &fakeSection{title: "Pull requests"}
	lone := New(t.Context(), config.Default(), Layout{Pulls: pulls}, WithRepo(testRepo))
	lone.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	run(lone, lone.key(press("z")))
	if lone.zoom || !pulls.got(isKey("z")) {
		t.Error("z with one pane should go to it")
	}
}

func TestZoomOnlyWhereItShows(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	m.Update(tea.WindowSizeMsg{Width: 64, Height: 24})
	run(m, m.key(press("z")))
	run(m, m.key(press("esc")))
	if m.zoom || !fakes[0].got(isKey("z")) || !fakes[0].got(isKey("esc")) {
		t.Errorf("zoom %v; at 64 columns z and esc should go to the pane", m.zoom)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	if s := onScreen(m); !strings.Contains(s, "Pull requests content") || !strings.Contains(s, "Files content") {
		t.Errorf("widening should show every pane:\n%s", s)
	}
}

func TestZoomLeavesKeysToCapture(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	run(m, m.key(press("z")))
	fakes[0].capturing = true
	run(m, m.key(press("z")))
	run(m, m.key(press("esc")))
	if !m.zoom || !fakes[0].got(isKey("z")) || !fakes[0].got(isKey("esc")) {
		t.Error("a capturing section should get z and esc, and the zoom stay")
	}
	fakes[0].capturing = false
	mod := &fakeModal{title: "Preview"}
	run(m, ui.OpenModal(mod))
	run(m, m.key(press("esc")))
	if !m.zoom || !slices.Equal(mod.keys(), []string{"esc"}) {
		t.Errorf("an open modal should get esc, and the zoom stay: zoom %v, modal got %v", m.zoom, mod.keys())
	}
}

// backKeys is the help of a section whose back key is esc.
type backKeys struct{}

func (backKeys) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "close")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}
func (k backKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// TestZoomHelp checks that the bar and the help name esc for what it does,
// the section's back, and the zoom key for what it does, which is the way
// out of a zoom while one shows, and only where the zoom works.
func TestZoomHelp(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	fakes[0].keyMap = backKeys{}
	check := func(when string, bar, help []string, not ...string) {
		t.Helper()
		s := onScreen(m)
		for _, want := range bar {
			if !strings.Contains(s, want) {
				t.Errorf("%s: the bar lacks %q:\n%s", when, want, s)
			}
		}
		rows := helpRows(t, m)
		for _, want := range help {
			if !slices.Contains(rows, want) {
				t.Errorf("%s: the help lacks %q: %q", when, want, rows)
			}
		}
		for _, no := range not {
			if strings.Contains(s, no) || slices.Contains(rows, no) {
				t.Errorf("%s: %q shows:\n%s\n%q", when, no, s, rows)
			}
		}
	}
	check("unzoomed", []string{"esc back"}, []string{"z zoom", "esc back"}, "z unzoom")
	run(m, m.key(press("z")))
	check("zoomed", []string{"z unzoom", "esc back", "x close"}, []string{"z unzoom", "esc back"}, "esc unzoom", "z zoom")
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	check("at 60 columns", []string{"esc back"}, []string{"esc back"}, "z unzoom", "z zoom")
}

// helpRows returns the rows that reach something in the help opened on
// the app now, each as its keys and description, and closes it again.
func helpRows(t *testing.T, m *Model) []string {
	t.Helper()
	run(m, m.key(press("?")))
	if !m.helpOpen() {
		t.Fatal("? didn't open the help")
	}
	var out []string
	for _, r := range m.keyhelp.Shown() {
		if r.Status == keyhelp.Active {
			out = append(out, strings.Join(r.Binding.Keys(), " ")+" "+r.Binding.Help().Desc)
		}
	}
	run(m, m.key(press("?")))
	if m.helpOpen() {
		t.Fatal("? didn't close the help")
	}
	return out
}

func TestZoomView(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 14})
	run(m, m.key(press("2")))
	run(m, m.key(press("z")))
	golden.RequireEqual(t, m.View().Content)
}

func TestProgramZooms(t *testing.T) {
	t.Parallel()
	_, fakes := newTestApp(t)
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3]}
	app := New(t.Context(), config.Default(), layout, WithRepo(testRepo))
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(120, 36))
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return strings.Contains(ansi.Strip(string(out)), "Issues content")
	}, teatest.WithDuration(5*time.Second))
	// The program takes messages in order, so the final model shows what
	// the keys and the resize left.
	tm.Send(press("z"))
	tm.Send(press("tab"))
	tm.Send(tea.WindowSizeMsg{Width: 160, Height: 40})
	tm.Send(press("q"))
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*Model)
	if !ok || !final.zoom || final.focus != 1 || fakes[1].width != 158 {
		t.Error("the final model should show the pull requests zoomed, over the width it was resized to")
	}
	if s := onScreen(final); !strings.Contains(s, "z unzoom") || strings.Contains(s, "Files content") {
		t.Errorf("the final screen should show the pull requests alone, and how to unzoom:\n%s", s)
	}
}

func TestFocusMovesBetweenPanes(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	steps := []struct {
		key  string
		want string
	}{
		{"tab", "Pull requests"},
		{"tab", "Issues"},
		{"tab", "Files"},
		{"shift+tab", "Issues"},
		// ] and [ are for the tabs of a pane, which the app leaves to it.
		{"]", "Issues"},
		{"[", "Issues"},
		{"2", "Pull requests"},
		{"1", "Files"},
		{"3", "Issues"},
	}
	for _, st := range steps {
		run(m, m.key(press(st.key)))
		if got := focusedTitles(fakes); !slices.Equal(got, []string{st.want}) {
			t.Errorf("after %s focused = %v, want %s", st.key, got, st.want)
		}
	}
	for _, f := range fakes {
		if f.got(isKey("tab")) || f.got(isKey("2")) {
			t.Errorf("%s got a key the app handles", f.title)
		}
	}
}

// I shows the notifications, and pressing it again stays there; backspace
// goes back to the pane that had the focus.
func TestNotificationsKeyShowsTheNotifications(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	run(m, m.key(press("2")))
	run(m, m.key(press("I")))
	if m.screen != notifScreen || !slices.Equal(focusedTitles(fakes), []string{"Notifications"}) {
		t.Fatalf("I didn't show the notifications: focused %v", focusedTitles(fakes))
	}
	if s := onScreen(m); !strings.Contains(s, "Notifications content") || !strings.Contains(s, "backspace back") {
		t.Errorf("notifications screen or its help is missing:\n%s", s)
	}
	// Tab has no panes to move between here.
	run(m, m.key(press("tab")))
	if m.screen != notifScreen {
		t.Error("tab left the notifications")
	}
	run(m, m.key(press("I")))
	if m.screen != notifScreen || len(m.back) != 1 {
		t.Errorf("I again left the notifications: screen %d, %d places to go back to, want 1", m.screen, len(m.back))
	}
	run(m, m.key(press("backspace")))
	if m.screen != repoScreen || !slices.Equal(focusedTitles(fakes), []string{"Pull requests"}) {
		t.Errorf("backspace didn't go back to the pane that had focus: %v", focusedTitles(fakes))
	}
	run(m, m.key(press("backspace")))
	if m.screen != repoScreen {
		t.Error("backspace at the bottom left the screen")
	}
	run(m, m.key(press("I")))
	run(m, m.key(press("3")))
	if m.screen != notifScreen || fakes[2].focused {
		t.Error("a pane key on the notifications switched screens")
	}
}

func TestSectionsStartWhenShown(t *testing.T) {
	t.Parallel()
	m, fakes := newApp(t, core.RepoRef{})
	run(m, m.Init())
	for _, f := range fakes {
		want := 0
		if f.title == "Notifications" {
			want = 1
		}
		if f.inits != want {
			t.Errorf("%s started %d times, want %d", f.title, f.inits, want)
		}
	}
	run(m, m.show("Files"))
	run(m, m.key(press("I")))
	run(m, m.key(press("backspace")))
	for _, f := range fakes {
		if f.inits != 1 {
			t.Errorf("%s started %d times, want once", f.title, f.inits)
		}
	}
}

func TestInitRunsWhatTheFirstRepoAsksFor(t *testing.T) {
	t.Parallel()
	type loaded struct{}
	files := &fakeSection{title: "Files", reply: func(msg tea.Msg) tea.Cmd {
		if _, ok := msg.(ui.RepoMsg); ok {
			return func() tea.Msg { return loaded{} }
		}
		return nil
	}}
	m := New(t.Context(), config.Default(), Layout{Files: files}, WithRepo(testRepo))
	run(m, m.Init())
	if !files.got(func(msg tea.Msg) bool { _, ok := msg.(loaded); return ok }) {
		t.Error("the command the section returned for the first repository never ran")
	}
}

func TestRepoScreenStartsEveryPane(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	run(m, m.Init())
	for _, f := range fakes {
		if f.inits != 1 {
			t.Errorf("%s started %d times, want once, for the notifications badge too", f.title, f.inits)
		}
	}
}

func TestKeysGoToTheFocusedPaneOnly(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	run(m, m.key(press("x")))
	if !fakes[0].got(isKey("x")) || fakes[1].got(isKey("x")) || fakes[3].got(isKey("x")) {
		t.Error("x should reach only the focused pane")
	}
}

func TestCapturingSectionTakesEveryKey(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	fakes[0].capturing = true
	keys := []string{"q", "?", "]", "2", "I", "S"}
	for _, k := range keys {
		if cmd := m.key(press(k)); cmd != nil {
			t.Errorf("%s returned a command while the section captures keys", k)
		}
	}
	if m.focus != 0 || m.screen != repoScreen || m.helpOpen() {
		t.Errorf("app keys acted while captured: focus %d, screen %d, help open %v", m.focus, m.screen, m.helpOpen())
	}
	var got []string
	for _, msg := range fakes[0].msgs {
		if k, ok := msg.(tea.KeyPressMsg); ok {
			got = append(got, k.String())
		}
	}
	if !slices.Equal(got, keys) {
		t.Errorf("section got keys %v, want %v", got, keys)
	}

	if cmd := m.key(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); cmd == nil {
		t.Error("ctrl+c didn't quit while the section captures keys")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c while capturing should quit")
	}

	fakes[0].capturing = false
	if cmd := m.key(press("q")); cmd == nil {
		t.Error("q didn't quit once the section stopped capturing")
	}
}

func TestAppMessagesReachEverySection(t *testing.T) {
	t.Parallel()
	m, fakes := newApp(t, core.RepoRef{})
	run(m, func() tea.Msg { return ui.SyncMsg{Key: "k"} })
	for _, f := range fakes {
		if !f.got(func(msg tea.Msg) bool { _, ok := msg.(ui.SyncMsg); return ok }) {
			t.Errorf("%s missed SyncMsg, although it hasn't started", f.title)
		}
	}
}

func TestRepoMsgShowsTheRepo(t *testing.T) {
	t.Parallel()
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	var asked []core.RepoRef
	info := func(_ context.Context, ref core.RepoRef) (core.Repo, error) {
		asked = append(asked, ref)
		return core.Repo{DefaultBranch: "main", Caps: core.RepoCaps{Known: true, Permission: core.PermissionRead}}, nil
	}
	m, fakes := newApp(t, core.RepoRef{}, WithRepoInfo(info))
	run(m, func() tea.Msg { return ui.RepoMsg{Repo: other} })
	if m.screen != repoScreen || !slices.Equal(focusedTitles(fakes), []string{"Files"}) {
		t.Errorf("RepoMsg didn't show the files: screen %d, focused %v", m.screen, focusedTitles(fakes))
	}
	for _, f := range fakes {
		if !f.got(func(msg tea.Msg) bool { r, ok := msg.(ui.RepoMsg); return ok && r.Repo == other }) {
			t.Errorf("%s missed RepoMsg", f.title)
		}
	}
	if !slices.Equal(asked, []core.RepoRef{other}) {
		t.Errorf("read %v, want the new repository", asked)
	}
	caps := ui.CapsMsg{Repo: other, Caps: core.RepoCaps{Known: true, Permission: core.PermissionRead}}
	for _, f := range fakes {
		if !f.got(func(msg tea.Msg) bool { return msg == tea.Msg(caps) }) {
			t.Errorf("%s missed the caps of the repository", f.title)
		}
	}
	if s := onScreen(m); !strings.Contains(s, "charmbracelet/bubbletea ─ main") {
		t.Errorf("header lacks the repository and its branch:\n%s", s)
	}
}

func TestRepoInfoForAnotherRepoIsIgnored(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	m.Update(repoInfoMsg{repo: core.Repo{Ref: core.RepoRef{Owner: "a", Name: "b"}, DefaultBranch: "trunk"}})
	if m.branch != "" {
		t.Error("the branch of another repository reached the header")
	}
	for _, f := range fakes {
		if f.got(func(msg tea.Msg) bool { _, ok := msg.(ui.CapsMsg); return ok }) {
			t.Errorf("%s got the caps of another repository", f.title)
		}
	}
}

func TestRepoWatcherSeesRepoBeforeSections(t *testing.T) {
	t.Parallel()
	want := core.RepoRef{Owner: "cli", Name: "cli"}
	var got []core.RepoRef
	var fakes []*fakeSection
	isRepo := func(msg tea.Msg) bool { r, ok := msg.(ui.RepoMsg); return ok && r.Repo == want }
	m, fakes := newTestApp(t, WithRepoWatcher(func(repo core.RepoRef) {
		got = append(got, repo)
		for _, f := range fakes {
			if f.got(isRepo) {
				t.Errorf("%s saw the repository before the watcher", f.title)
			}
		}
	}))
	run(m, func() tea.Msg { return ui.RepoMsg{Repo: want} })
	if !slices.Equal(got, []core.RepoRef{testRepo, want}) {
		t.Errorf("watcher got %v, want the first repository, then %v", got, want)
	}
	for _, f := range fakes {
		if !f.got(isRepo) {
			t.Errorf("%s missed RepoMsg", f.title)
		}
	}
}

// ghError stands for an error of the github package, whose text names the
// request and its status, with GitHub's own reason.
type ghError struct {
	is     error
	reason string
}

func (e *ghError) Error() string {
	return "github: PUT /repos/eggzec/gh-tui/pulls/42/merge: 405 " + e.reason
}

func (e *ghError) Unwrap() error  { return e.is }
func (e *ghError) Reason() string { return e.reason }

// failure is an error of a kind, and the cause a toast gives for it.
type failure struct {
	name  string
	err   error
	cause string
}

// testLog is the log file that failures point to, as the toasts show it.
const testLog = "~/.local/state/gh-tui/gh-tui.log"

// failures are errors of every kind that GitHub or the app may fail
// with, each with a chain the user must never see. An empty cause means
// no toast.
func failures() []failure {
	reset := time.Now().Add(time.Hour)
	offline := &url.Error{Op: "Put", URL: "https://api.github.com/repos/eggzec/gh-tui", Err: errors.New("dial tcp: no route to host")}
	return []failure{
		{name: "offline", err: fmt.Errorf("github: %w: %w", core.ErrOffline, offline), cause: "can't reach GitHub."},
		{name: "unavailable", err: &ghError{is: core.ErrUnavailable}, cause: "GitHub isn't responding."},
		{name: "forbidden", err: &ghError{is: core.ErrForbidden, reason: "Resource not accessible"}, cause: "you don't have access to this."},
		{name: "not found", err: &ghError{is: core.ErrNotFound, reason: "Not Found"}, cause: "this doesn't exist or is private."},
		{name: "rejected", err: &ghError{is: core.ErrConflict, reason: "Pull Request is not mergeable"}, cause: "Pull Request is not mergeable."},
		{name: "rate limited", err: fmt.Errorf("github: 403: %w", &core.RateLimitError{Reset: reset}), cause: "rate limited until " + ui.Clock(reset, time.Now()) + "."},
		{name: "rate limited with no reset", err: fmt.Errorf("github: 403: %w", &core.RateLimitError{}), cause: "rate limited by GitHub."},
		{name: "auth", err: &ghError{is: core.ErrUnauthorized, reason: "Bad credentials"}, cause: "GitHub rejected the token. Run gh auth login, then restart gh-tui."},
		{name: "internal", err: errors.New("github: decode 200: unexpected EOF"), cause: "something went wrong, see " + testLog + "."},
		{name: "canceled", err: fmt.Errorf("github: PUT /repos: %w", context.Canceled)},
	}
}

// logVoice returns the voice of the default keys, pointing to testLog.
func logVoice(t *testing.T) ui.Voice {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory:", err)
	}
	return ui.NewVoice(config.Default().Keys, filepath.Join(home, ".local", "state", "gh-tui", "gh-tui.log"))
}

// leaks are what a toast must never show of an error: the package,
// requests, paths, status codes and the home directory.
var leaks = []string{"github:", "PUT", "GET", "/repos", "405", "403", "200", "api.github.com", "dial tcp", "EOF", "canceled"}

// checkClean fails t if text shows anything of leaks, or the home
// directory.
func checkClean(t *testing.T, text string) {
	t.Helper()
	for _, l := range leaks {
		if strings.Contains(text, l) {
			t.Errorf("toast shows %q: %s", l, text)
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.Contains(text, home) {
		t.Errorf("toast shows the home directory: %s", text)
	}
}

func TestDoneWithErrorShowsToast(t *testing.T) {
	t.Parallel()
	for _, f := range failures() {
		t.Run(f.name, func(t *testing.T) {
			m, fakes := newTestApp(t, WithVoice(logVoice(t)))
			m.Update(ui.DoneMsg{What: "merge #42", Err: f.err})
			switch got := toasted(m); {
			case f.cause == "" && got != "":
				t.Errorf("toast %q, want none", got)
			case f.cause != "" && !hasToast(m, "Couldn't merge #42: "+f.cause):
				t.Errorf("toast %q, want %q", got, "Couldn't merge #42: "+f.cause)
			}
			checkClean(t, toasted(m))
			if !fakes[2].got(func(msg tea.Msg) bool { _, ok := msg.(ui.DoneMsg); return ok }) {
				t.Error("DoneMsg wasn't passed on to the sections")
			}
		})
	}
}

func TestFailShowsToast(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"not found names the subject", &ghError{is: core.ErrNotFound, reason: "Not Found"}, "Couldn't load #42: eggzec/gh-tui#42 doesn't exist or is private."},
		{"forbidden names the repository", &ghError{is: core.ErrForbidden, reason: "Resource not accessible"}, "Couldn't load #42: you don't have access to eggzec/gh-tui."},
		{"offline", fmt.Errorf("github: %w: dial tcp", core.ErrOffline), "Couldn't load #42: can't reach GitHub."},
		{"canceled", fmt.Errorf("github: GET /repos: %w", context.Canceled), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestApp(t, WithVoice(logVoice(t)))
			m.Update(ui.FailMsg{What: "load #42", Err: core.About("eggzec/gh-tui#42", tt.err)})
			switch got := toasted(m); {
			case tt.want == "" && got != "":
				t.Errorf("toast %q, want none", got)
			case tt.want != "" && !hasToast(m, tt.want):
				t.Errorf("toast %q, want %q", got, tt.want)
			}
			checkClean(t, toasted(m))
		})
	}
}

func TestDoneWithErrorWithoutLog(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	m.Update(ui.DoneMsg{What: "merge #42", Err: errors.New("github: decode 200: unexpected EOF")})
	if want := "Couldn't merge #42: something went wrong."; !hasToast(m, want) {
		t.Errorf("toast %q, want %q", toasted(m), want)
	}
}

func TestDoneWithoutErrorIsQuiet(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	m.Update(ui.DoneMsg{What: "merge #42"})
	if !m.toast.Empty() {
		t.Error("a successful change showed a toast")
	}
}

func TestNotifyShowsToast(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	m.Update(ui.NotifyMsg{Level: toast.Success, Text: "Starred"})
	if !strings.Contains(onScreen(m), "Starred") {
		t.Error("view has no toast")
	}
}

func TestWarningAtStart(t *testing.T) {
	t.Parallel()
	m, _ := newApp(t, core.RepoRef{}, WithWarning("No disk cache"))
	if strings.Contains(onScreen(m), "No disk cache") {
		t.Error("the warning showed before the app started")
	}
	run(m, m.Init())
	if !strings.Contains(onScreen(m), "No disk cache") {
		t.Error("view has no warning after Init")
	}
}

func TestShowFocusesTheSection(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	run(m, func() tea.Msg { return ui.ShowMsg{Title: "Notifications"} })
	if m.screen != notifScreen || !fakes[3].focused {
		t.Error("ShowMsg didn't show the notifications")
	}
	run(m, func() tea.Msg { return ui.ShowMsg{Title: "Issues"} })
	if m.screen != repoScreen || !slices.Equal(focusedTitles(fakes), []string{"Issues"}) {
		t.Errorf("ShowMsg didn't focus the issues: %v", focusedTitles(fakes))
	}
}

func TestBadgeIsInTheHeader(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	fakes[3].badge = "3"
	m.Update(ui.SyncMsg{Key: "notifications"})
	if s := onScreen(m); !strings.Contains(s, "● 3 notifications") {
		t.Errorf("header lacks the badge:\n%s", s)
	}
	fakes[3].badge = "1"
	m.Update(ui.SyncMsg{Key: "notifications"})
	if s := onScreen(m); !strings.Contains(s, "● 1 notification ") {
		t.Errorf("header lacks the singular badge:\n%s", s)
	}
}

func TestHeaderFitsAnyWidth(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	fakes[3].badge = "30+"
	m.Update(ui.SyncMsg{})
	for _, w := range []int{1, 5, 12, 20, 30, 80, 200} {
		m.Update(tea.WindowSizeMsg{Width: w, Height: 10})
		if got := ansi.StringWidth(m.header); got != w {
			t.Errorf("header at %d columns is %d wide: %q", w, got, ansi.Strip(m.header))
		}
	}
}

func TestSyncListensAgain(t *testing.T) {
	t.Parallel()
	events := make(chan ui.SyncMsg, 2)
	events <- ui.SyncMsg{Key: "a"}
	events <- ui.SyncMsg{Key: "b"}
	close(events)
	next := func(context.Context) (ui.SyncMsg, bool) {
		msg, ok := <-events
		return msg, ok
	}
	m, fakes := newTestApp(t, WithSync(next))
	run(m, m.listen())
	for _, k := range []string{"a", "b"} {
		if !fakes[1].got(func(msg tea.Msg) bool { s, ok := msg.(ui.SyncMsg); return ok && s.Key == k }) {
			t.Errorf("sync event %q didn't reach the sections", k)
		}
	}
}

func TestFocusIsReported(t *testing.T) {
	t.Parallel()
	var got []bool
	m, _ := newTestApp(t, WithActivity(func(active bool) { got = append(got, active) }))
	m.Update(tea.BlurMsg{})
	m.Update(tea.FocusMsg{})
	if len(got) != 2 || got[0] || !got[1] {
		t.Errorf("reported %v, want [false true]", got)
	}
}

func TestOpenReportsFailure(t *testing.T) {
	// Not parallel: it sets environment variables, which the whole process shares.
	for _, tt := range []struct{ name, ghBrowser, want string }{
		{"no GH_BROWSER", "", "Couldn't open the browser: set one with gh config set browser <command>."},
		{"GH_BROWSER", "nosuchbrowser", "Couldn't open the browser: GH_BROWSER names a command that didn't open it."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GH_BROWSER", tt.ghBrowser)
			failed := &exec.Error{Name: "/usr/bin/xdg-open", Err: exec.ErrNotFound}
			m, _ := newTestApp(t, WithBrowser(func(string) (*exec.Cmd, error) { return nil, failed }))
			run(m, m.openURL("https://github.com"))
			if !hasToast(m, tt.want) {
				t.Errorf("toast %q, want %q", toasted(m), tt.want)
			}
			if got := toasted(m); strings.Contains(got, "xdg-open") || strings.Contains(got, "/usr") {
				t.Errorf("toast shows the error: %s", got)
			}
		})
	}
}

// TestOpenRedraws checks that the screen is drawn again after a browser
// starts detached, and that a browser that runs in the terminal is given
// it.
func TestOpenRedraws(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t, WithBrowser(func(string) (*exec.Cmd, error) { return nil, nil }))
	if msg := m.openURL("https://github.com")(); msg != tea.ClearScreen() {
		t.Errorf("message %T after a detached browser, want a redraw", msg)
	}
	w3m := exec.Command("w3m", "https://github.com")
	m, _ = newTestApp(t, WithBrowser(func(string) (*exec.Cmd, error) { return w3m, nil }))
	msg := m.openURL("https://github.com")()
	if want := tea.ExecProcess(w3m, nil)(); reflect.TypeOf(msg) != reflect.TypeOf(want) {
		t.Errorf("message %T for a text browser, want %T, which hands it the terminal", msg, want)
	}
}

// TestHelpShowsPaneAndAppKeys checks that the bar hints at the keys of the
// focused pane and of the app, and that the help lists them all without
// taking room from the panes.
func TestHelpShowsPaneAndAppKeys(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	s := onScreen(m)
	for _, want := range []string{"x close", "S search", "I notifications", "? help", "q quit"} {
		if !strings.Contains(s, want) {
			t.Errorf("the bar lacks %q:\n%s", want, s)
		}
	}
	height := fakes[0].height
	rows := helpRows(t, m)
	for _, want := range []string{"tab next pane", "1 2 3 focus pane", "x close", "? help"} {
		if !slices.Contains(rows, want) {
			t.Errorf("the help lacks %q: %q", want, rows)
		}
	}
	if fakes[0].height != height {
		t.Errorf("the help resized the pane from %d to %d rows", height, fakes[0].height)
	}
}

func TestMissingSectionsAreLeftOut(t *testing.T) {
	t.Parallel()
	pulls := &fakeSection{title: "Pull requests"}
	m := New(t.Context(), config.Default(), Layout{Pulls: pulls}, WithRepo(testRepo))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if pulls.width != 78 || !pulls.focused {
		t.Errorf("a lone pane should fill the screen and have focus: %dx%d", pulls.width, pulls.height)
	}
	if s := onScreen(m); !strings.Contains(s, "[1] Pull requests") {
		t.Errorf("screen lacks the lone pane:\n%s", s)
	}
	run(m, m.key(press("I")))
	run(m, m.key(press("tab")))
	run(m, m.key(press("3")))
	if m.screen != repoScreen || !pulls.focused {
		t.Error("keys for missing sections moved the focus")
	}
	empty := New(t.Context(), config.Default(), Layout{})
	empty.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	run(empty, empty.Init())
	run(empty, empty.key(press("x")))
	if h := lipgloss.Height(empty.View().Content); h != 24 {
		t.Errorf("empty app is %d lines high, want 24", h)
	}
}

func TestProgramRendersAndQuits(t *testing.T) {
	t.Parallel()
	_, fakes := newTestApp(t)
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3]}
	app := New(t.Context(), config.Default(), layout, WithRepo(testRepo))
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(80, 24))
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Files content")) && bytes.Contains(out, []byte("Issues content"))
	}, teatest.WithDuration(5*time.Second))
	tm.Send(press("I"))
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Notifications content"))
	}, teatest.WithDuration(5*time.Second))
	tm.Send(press("q"))
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*Model)
	if !ok || final.screen != notifScreen {
		t.Error("the final model isn't on the notifications")
	}
}

func benchApp(b *testing.B) (*Model, *fakeSection) {
	b.Helper()
	files := &fakeSection{title: "Files"}
	layout := Layout{
		Files: files, Pulls: &fakeSection{title: "Pull requests"},
		Issues: &fakeSection{title: "Issues"}, Notifications: &fakeSection{title: "Notifications", badge: "3"},
	}
	m := New(b.Context(), config.Default(), layout, WithRepo(testRepo))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m, files
}

func BenchmarkView(b *testing.B) {
	m, _ := benchApp(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkViewZoomed(b *testing.B) {
	m, _ := benchApp(b)
	m.key(press("z"))
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	m, files := benchApp(b)
	msg := press("j")
	b.ReportAllocs()
	for b.Loop() {
		m.Update(msg)
		files.msgs = files.msgs[:0]
	}
}

// rateCounter tells rate limits that count how often they were read.
type rateCounter struct{ reads int }

func (r *rateCounter) RateStatus() core.RateStatus {
	r.reads++
	return core.RateStatus{Quotas: []core.Quota{{Resource: "core", Remaining: r.reads}}}
}

func TestSyncRateLimitRereadsRates(t *testing.T) {
	t.Parallel()
	rates := &rateCounter{}
	m, _ := newTestApp(t, WithRateStatus(rates))
	if rates.reads != 1 || m.rate.Quotas[0].Remaining != 1 {
		t.Fatalf("at start: read %d times, kept %+v; want read once", rates.reads, m.rate)
	}
	m.Update(ui.SyncMsg{Key: "notifications"})
	if rates.reads != 1 {
		t.Errorf("another key read the rate limits again")
	}
	m.Update(ui.SyncMsg{Key: core.SyncRateLimit})
	if rates.reads != 2 || m.rate.Quotas[0].Remaining != 2 {
		t.Errorf("after %s: read %d times, kept %+v; want the second read", core.SyncRateLimit, rates.reads, m.rate)
	}
}

// Toasts are marked with the glyphs of the icon set, and a switch of
// ui.icons marks the next ones with the new set's.
func TestToastMarksFollowIcons(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, set := range []string{config.IconsASCII, config.IconsUnicode, config.IconsNerd} {
		runCommand(t, m, "set ui.icons="+set)
		ic := ui.NewIcons(set)
		for _, tt := range []struct {
			level toast.Level
			mark  string
		}{{toast.Info, ic.Info}, {toast.Success, ic.Yes}, {toast.Error, ic.Error}} {
			m.toast.Clear()
			m.toast.Push(tt.level, "The repository is starred")
			want := tt.mark + " The repository is starred"
			if got := toasted(m); !strings.Contains(got, want) {
				t.Errorf("%s: toast %q, want %q", set, got, want)
			}
		}
	}
}

// The toasts leave the border of the pane under them, however narrow the
// screen.
func TestToastsKeepThePaneBorder(t *testing.T) {
	t.Parallel()
	for _, width := range []int{80, 24, 20} {
		m, _ := newTestApp(t)
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		m.toast.Push(toast.Error, "Couldn't merge #5: the base branch was modified; review and try the merge again.")
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		if last := lines[len(lines)-2]; !strings.HasSuffix(last, "╯") || strings.Contains(last, "▌") {
			t.Errorf("at %d columns the bottom border is %q, want it whole", width, last)
		}
		drawn := false
		for _, l := range lines[:len(lines)-2] {
			if strings.Contains(l, ui.NewIcons(config.Default().UI.Icons).Error) {
				drawn = true
				if !strings.HasPrefix(l, "│") || !strings.HasSuffix(l, "│") {
					t.Errorf("at %d columns toast line %q covers a border", width, l)
				}
			}
		}
		if !drawn {
			t.Errorf("at %d columns no toast drawn:\n%s", width, strings.Join(lines, "\n"))
		}
	}
}
