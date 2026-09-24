package tui

import (
	"bytes"
	"context"
	"errors"
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
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
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
	m.toast.SetErrorDuration(0)
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
	if !strings.Contains(s, "press / to search") {
		t.Errorf("header doesn't say how to pick a repository:\n%s", s)
	}
}

func TestPanesAreSizedToTheirFrames(t *testing.T) {
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
	for width, want := range map[int]int{70: 28, 80: 32, 100: 40, 200: 60, 20: 24} {
		if got := filesWidth(width); got != want {
			t.Errorf("filesWidth(%d) = %d, want %d", width, got, want)
		}
	}
}

func TestRepoScreenIsFramed(t *testing.T) {
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

func TestFocusMovesBetweenPanes(t *testing.T) {
	m, fakes := newTestApp(t)
	steps := []struct {
		key  string
		want string
	}{
		{"tab", "Pull requests"},
		{"tab", "Issues"},
		{"tab", "Files"},
		{"shift+tab", "Issues"},
		{"]", "Files"},
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

func TestNotificationsKeyTogglesScreens(t *testing.T) {
	m, fakes := newTestApp(t)
	run(m, m.key(press("2")))
	run(m, m.key(press("n")))
	if m.screen != notifScreen || !slices.Equal(focusedTitles(fakes), []string{"Notifications"}) {
		t.Fatalf("n didn't show the notifications: focused %v", focusedTitles(fakes))
	}
	if s := onScreen(m); !strings.Contains(s, "Notifications content") || !strings.Contains(s, "n back") {
		t.Errorf("notifications screen or its help is missing:\n%s", s)
	}
	// Tab has no panes to move between here.
	run(m, m.key(press("tab")))
	if m.screen != notifScreen {
		t.Error("tab left the notifications")
	}
	run(m, m.key(press("n")))
	if m.screen != repoScreen || !slices.Equal(focusedTitles(fakes), []string{"Pull requests"}) {
		t.Errorf("n didn't go back to the pane that had focus: %v", focusedTitles(fakes))
	}
	run(m, m.key(press("n")))
	run(m, m.key(press("3")))
	if m.screen != repoScreen || !fakes[2].focused {
		t.Error("a pane key on the notifications didn't show that pane")
	}
}

func TestSectionsStartWhenShown(t *testing.T) {
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
	run(m, m.key(press("n")))
	run(m, m.key(press("n")))
	run(m, m.key(press("n")))
	for _, f := range fakes {
		if f.inits != 1 {
			t.Errorf("%s started %d times, want once", f.title, f.inits)
		}
	}
}

func TestInitRunsWhatTheFirstRepoAsksFor(t *testing.T) {
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
	m, fakes := newTestApp(t)
	run(m, m.Init())
	for _, f := range fakes {
		if f.inits != 1 {
			t.Errorf("%s started %d times, want once, for the notifications badge too", f.title, f.inits)
		}
	}
}

func TestKeysGoToTheFocusedPaneOnly(t *testing.T) {
	m, fakes := newTestApp(t)
	run(m, m.key(press("x")))
	if !fakes[0].got(isKey("x")) || fakes[1].got(isKey("x")) || fakes[3].got(isKey("x")) {
		t.Error("x should reach only the focused pane")
	}
}

func TestCapturingSectionTakesEveryKey(t *testing.T) {
	m, fakes := newTestApp(t)
	fakes[0].capturing = true
	keys := []string{"q", "?", "]", "2", "n", "/"}
	for _, k := range keys {
		if cmd := m.key(press(k)); cmd != nil {
			t.Errorf("%s returned a command while the section captures keys", k)
		}
	}
	if m.focus != 0 || m.screen != repoScreen || m.help.ShowAll {
		t.Errorf("app keys acted while captured: focus %d, screen %d, full help %v", m.focus, m.screen, m.help.ShowAll)
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
	m, fakes := newApp(t, core.RepoRef{})
	run(m, func() tea.Msg { return ui.SyncMsg{Key: "k"} })
	for _, f := range fakes {
		if !f.got(func(msg tea.Msg) bool { _, ok := msg.(ui.SyncMsg); return ok }) {
			t.Errorf("%s missed SyncMsg, although it hasn't started", f.title)
		}
	}
}

func TestRepoMsgShowsTheRepo(t *testing.T) {
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	var asked []core.RepoRef
	info := func(_ context.Context, ref core.RepoRef) (core.Repo, error) {
		asked = append(asked, ref)
		return core.Repo{DefaultBranch: "main"}, nil
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
	if s := onScreen(m); !strings.Contains(s, "charmbracelet/bubbletea ─ main") {
		t.Errorf("header lacks the repository and its branch:\n%s", s)
	}
}

func TestRepoInfoForAnotherRepoIsIgnored(t *testing.T) {
	m, _ := newTestApp(t)
	m.Update(repoInfoMsg{repo: core.Repo{Ref: core.RepoRef{Owner: "a", Name: "b"}, DefaultBranch: "trunk"}})
	if m.branch != "" {
		t.Error("the branch of another repository reached the header")
	}
}

func TestRepoWatcherSeesRepoBeforeSections(t *testing.T) {
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

func TestDoneWithErrorShowsToast(t *testing.T) {
	m, fakes := newTestApp(t)
	m.Update(ui.DoneMsg{What: "merge #42", Err: errors.New("conflict")})
	if got := onScreen(m); !strings.Contains(got, "Couldn't merge #42:") || !strings.Contains(got, "conflict") {
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
	if !strings.Contains(onScreen(m), "Starred") {
		t.Error("view has no toast")
	}
}

func TestWarningAtStart(t *testing.T) {
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
	if !strings.Contains(onScreen(m), "Couldn't open the browser") {
		t.Error("a failed open showed no toast")
	}
}

func TestHelpShowsPaneAndAppKeys(t *testing.T) {
	m, fakes := newTestApp(t)
	s := onScreen(m)
	for _, want := range []string{"x close", "/ search", "n notifications", "? help", "q quit"} {
		if !strings.Contains(s, want) {
			t.Errorf("help lacks %q:\n%s", want, s)
		}
	}
	short := fakes[0].height
	run(m, m.key(press("?")))
	if fakes[0].height >= short {
		t.Errorf("height with full help = %d, want less than %d", fakes[0].height, short)
	}
	if s = onScreen(m); !strings.Contains(s, "next pane") || !strings.Contains(s, "1/2/3 focus pane") {
		t.Errorf("full help lacks the pane keys:\n%s", s)
	}
}

func TestMissingSectionsAreLeftOut(t *testing.T) {
	pulls := &fakeSection{title: "Pull requests"}
	m := New(t.Context(), config.Default(), Layout{Pulls: pulls}, WithRepo(testRepo))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if pulls.width != 78 || !pulls.focused {
		t.Errorf("a lone pane should fill the screen and have focus: %dx%d", pulls.width, pulls.height)
	}
	if s := onScreen(m); !strings.Contains(s, "[1] Pull requests") {
		t.Errorf("screen lacks the lone pane:\n%s", s)
	}
	run(m, m.key(press("n")))
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
	_, fakes := newTestApp(t)
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3]}
	app := New(t.Context(), config.Default(), layout, WithRepo(testRepo))
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(80, 24))
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Files content")) && bytes.Contains(out, []byte("Issues content"))
	}, teatest.WithDuration(time.Second))
	tm.Send(press("n"))
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Notifications content"))
	}, teatest.WithDuration(time.Second))
	tm.Send(press("q"))
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(time.Second)).(*Model)
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

func BenchmarkUpdate(b *testing.B) {
	m, files := benchApp(b)
	msg := press("j")
	b.ReportAllocs()
	for b.Loop() {
		m.Update(msg)
		files.msgs = files.msgs[:0]
	}
}
