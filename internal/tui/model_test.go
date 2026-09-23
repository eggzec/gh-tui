package tui

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/tabs"
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
}

func (s *fakeSection) Title() string { return s.title }
func (s *fakeSection) Badge() string { return s.badge }
func (s *fakeSection) Init() tea.Cmd { s.inits++; return nil }
func (s *fakeSection) Update(msg tea.Msg) tea.Cmd {
	s.msgs = append(s.msgs, msg)
	return nil
}
func (s *fakeSection) View() string      { return s.title + " content" }
func (s *fakeSection) SetSize(w, h int)  { s.width, s.height = w, h }
func (s *fakeSection) SetTheme(ui.Theme) { s.themed = true }
func (s *fakeSection) Focus()            { s.focused = true }
func (s *fakeSection) Blur()             { s.focused = false }
func (s *fakeSection) Help() help.KeyMap { return sectionKeys{} }
func (s *fakeSection) Capturing() bool   { return s.capturing }
func (s *fakeSection) got(match func(tea.Msg) bool) bool {
	return slices.ContainsFunc(s.msgs, match)
}

type sectionKeys struct{}

func (sectionKeys) ShortHelp() []key.Binding {
	return []key.Binding{key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "close"))}
}
func (k sectionKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

func newTestApp(t *testing.T, opts ...Option) (*Model, []*fakeSection) {
	t.Helper()
	fakes := []*fakeSection{{title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"}}
	sections := make([]ui.Section, len(fakes))
	for i, f := range fakes {
		sections[i] = f
	}
	m := New(t.Context(), config.Default(), sections, opts...)
	// Toasts that never expire keep run from waiting on their timers.
	m.toast.SetDuration(0)
	m.toast.SetErrorDuration(0)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m, fakes
}

// screen is the text on screen with styles removed and whitespace
// collapsed, so text that a toast wraps still matches.
func screen(m *Model) string {
	return strings.Join(strings.Fields(ansi.Strip(m.View().Content)), " ")
}

// run applies the command's message, and those of batched commands, to m.
func run(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			run(m, c)
		}
	case nil:
	default:
		_, next := m.Update(msg)
		run(m, next)
	}
}

func press(k string) tea.KeyPressMsg {
	if k == "tab" {
		return tea.KeyPressMsg{Code: tea.KeyTab}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

func TestNewAppliesThemeAndFocusesFirst(t *testing.T) {
	_, fakes := newTestApp(t)
	for _, f := range fakes {
		if !f.themed {
			t.Errorf("%s has no theme", f.title)
		}
	}
	if !fakes[0].focused || fakes[1].focused {
		t.Error("only the first section should be focused")
	}
}

func TestSizeLeavesRoomForTabsAndHelp(t *testing.T) {
	_, fakes := newTestApp(t)
	for _, f := range fakes {
		if f.width != 80 || f.height != 24-2-1 {
			t.Errorf("%s size = %dx%d, want 80x21", f.title, f.width, f.height)
		}
	}
}

func TestSectionsStartLazily(t *testing.T) {
	m, fakes := newTestApp(t)
	run(m, m.Init())
	if fakes[0].inits != 1 || fakes[1].inits != 0 {
		t.Fatalf("inits = %d, %d; want only the first section started", fakes[0].inits, fakes[1].inits)
	}

	run(m, m.key(press("tab")))
	if fakes[1].inits != 1 || !fakes[1].focused || fakes[0].focused {
		t.Errorf("after tab: inits %d, focused %v/%v; want issues started and focused", fakes[1].inits, fakes[0].focused, fakes[1].focused)
	}

	run(m, m.key(press("1")))
	run(m, m.key(press("2")))
	if fakes[1].inits != 1 {
		t.Errorf("issues started %d times, want once", fakes[1].inits)
	}
}

func TestKeysGoToTheActiveSectionOnly(t *testing.T) {
	m, fakes := newTestApp(t)
	run(m, m.key(press("x")))
	isX := func(msg tea.Msg) bool { k, ok := msg.(tea.KeyPressMsg); return ok && k.String() == "x" }
	if !fakes[0].got(isX) || fakes[1].got(isX) {
		t.Error("x should reach only the active section")
	}
}

func TestCapturingSectionTakesEveryKey(t *testing.T) {
	m, fakes := newTestApp(t)
	fakes[0].capturing = true
	for _, k := range []string{"q", "?", "]", "2"} {
		if cmd := m.key(press(k)); cmd != nil {
			t.Errorf("%s returned a command while the section captures keys", k)
		}
	}
	if m.active != 0 || m.help.ShowAll {
		t.Errorf("app keys acted while captured: active %d, full help %v", m.active, m.help.ShowAll)
	}
	var got []string
	for _, msg := range fakes[0].msgs {
		if k, ok := msg.(tea.KeyPressMsg); ok {
			got = append(got, k.String())
		}
	}
	if !slices.Equal(got, []string{"q", "?", "]", "2"}) {
		t.Errorf("section got keys %v, want all four", got)
	}

	if cmd := m.key(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); cmd == nil {
		t.Error("ctrl+c didn't quit while the section captures keys")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c while capturing should quit")
	}
	if n := len(fakes[0].msgs); n != 4 {
		t.Errorf("section got %d keys, want ctrl+c kept from it", n)
	}

	fakes[0].capturing = false
	if cmd := m.key(press("q")); cmd == nil {
		t.Error("q didn't quit once the section stopped capturing")
	}
}

func TestAppMessagesReachEverySection(t *testing.T) {
	m, fakes := newTestApp(t)
	run(m, func() tea.Msg { return ui.RepoMsg{} })
	for _, f := range fakes {
		if !f.got(func(msg tea.Msg) bool { _, ok := msg.(ui.RepoMsg); return ok }) {
			t.Errorf("%s missed RepoMsg, although it hasn't started", f.title)
		}
	}
}

func TestRepoWatcherSeesRepoBeforeSections(t *testing.T) {
	want := core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	var got []core.RepoRef
	var fakes []*fakeSection
	isRepo := func(msg tea.Msg) bool { _, ok := msg.(ui.RepoMsg); return ok }
	m, fakes := newTestApp(t, WithRepoWatcher(func(repo core.RepoRef) {
		got = append(got, repo)
		for _, f := range fakes {
			if f.got(isRepo) {
				t.Errorf("%s saw the repository before the watcher", f.title)
			}
		}
	}))
	run(m, func() tea.Msg { return ui.RepoMsg{Repo: want} })
	if len(got) != 1 || got[0] != want {
		t.Errorf("watcher got %v, want [%v]", got, want)
	}
	for _, f := range fakes {
		if !f.got(isRepo) {
			t.Errorf("%s missed RepoMsg", f.title)
		}
	}
}

func TestDoneWithErrorShowsToast(t *testing.T) {
	m, fakes := newTestApp(t)
	m.Update(ui.DoneMsg{What: "merge #42", Err: errors.New("conflict")})
	if got := screen(m); !strings.Contains(got, "Couldn't merge #42:") || !strings.Contains(got, "conflict") {
		t.Errorf("view has no error toast:\n%s", got)
	}
	if !fakes[2].got(func(msg tea.Msg) bool { _, ok := msg.(ui.DoneMsg); return ok }) {
		t.Error("DoneMsg wasn't passed on to the sections")
	}
}

func TestDoneWithoutErrorIsQuiet(t *testing.T) {
	m, _ := newTestApp(t)
	m.Update(ui.DoneMsg{What: "merge #42"})
	if !m.toast.Empty() {
		t.Error("a successful change showed a toast")
	}
}

func TestNotifyShowsToast(t *testing.T) {
	m, _ := newTestApp(t)
	m.Update(ui.NotifyMsg{Level: toast.Success, Text: "Starred"})
	if !strings.Contains(screen(m), "Starred") {
		t.Error("view has no toast")
	}
}

func TestShowSwitchesSection(t *testing.T) {
	m, fakes := newTestApp(t)
	run(m, func() tea.Msg { return ui.ShowMsg{Title: "Notifications"} })
	if m.active != 2 || !fakes[2].focused {
		t.Errorf("active = %d, want Notifications", m.active)
	}
}

func TestForeignTabChangeIsIgnored(t *testing.T) {
	m, _ := newTestApp(t)
	m.Update(tabs.ChangeMsg{ID: m.tabs.ID() + 100, Index: 1})
	if m.active != 0 {
		t.Error("a tab change from another tab bar switched the section")
	}
}

func TestBadgesFollowSections(t *testing.T) {
	m, fakes := newTestApp(t)
	fakes[2].badge = "3"
	m.Update(ui.SyncMsg{Key: "notifications"})
	if got := m.tabs.Badge(2); got != "3" {
		t.Errorf("badge = %q, want 3", got)
	}
}

func TestSyncListensAgain(t *testing.T) {
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
	var got []bool
	m, _ := newTestApp(t, WithActivity(func(active bool) { got = append(got, active) }))
	m.Update(tea.BlurMsg{})
	m.Update(tea.FocusMsg{})
	if len(got) != 2 || got[0] || !got[1] {
		t.Errorf("reported %v, want [false true]", got)
	}
}

func TestOpenReportsFailure(t *testing.T) {
	m, _ := newTestApp(t, WithBrowser(func(string) error { return errors.New("no browser") }))
	run(m, m.openURL("https://github.com"))
	if !strings.Contains(screen(m), "Couldn't open the browser") {
		t.Error("a failed open showed no toast")
	}
}

func TestHelpToggleResizesSections(t *testing.T) {
	m, fakes := newTestApp(t)
	short := fakes[0].height
	run(m, m.key(press("?")))
	if fakes[0].height >= short {
		t.Errorf("height with full help = %d, want less than %d", fakes[0].height, short)
	}
}

func TestProgramRendersAndQuits(t *testing.T) {
	_, fakes := newTestApp(t)
	sections := []ui.Section{fakes[0], fakes[1]}
	tm := teatest.NewTestModel(t, New(t.Context(), config.Default(), sections), teatest.WithInitialTermSize(80, 24))
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Pull requests content")) && bytes.Contains(out, []byte("quit"))
	}, teatest.WithDuration(time.Second))
	tm.Send(press("q"))
	tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
}

func BenchmarkView(b *testing.B) {
	m := New(context.Background(), config.Default(), []ui.Section{&fakeSection{title: "Pull requests"}, &fakeSection{title: "Issues"}})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	f := &fakeSection{title: "Pull requests"}
	m := New(context.Background(), config.Default(), []ui.Section{f})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	msg := press("j")
	b.ReportAllocs()
	for b.Loop() {
		m.Update(msg)
		f.msgs = f.msgs[:0]
	}
}
