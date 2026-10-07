package tui

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// collideKeys is the help of a section whose find key is S, which the
// app's search takes first, and whose close key is x.
type collideKeys struct{}

func (collideKeys) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "close")),
		key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "find")),
	}
}
func (k collideKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// shownDescs returns the descriptions of the rows the help shows.
func shownDescs(m *Model) []string {
	shown := m.keyhelp.Shown()
	out := make([]string, 0, len(shown))
	for _, r := range shown {
		out = append(out, r.Binding.Help().Desc)
	}
	return out
}

// TestHelpOverModal checks that the help key opens the help over a modal,
// which keeps the modal open and gets no keys until the help closes.
func TestHelpOverModal(t *testing.T) {
	m, _ := newTestApp(t)
	mod := &fakeModal{title: "Preview \x1b[31mred"}
	run(m, ui.OpenModal(mod))
	run(m, m.key(press("?")))
	if !m.helpOpen() || m.topModal() != mod {
		t.Fatalf("? over a modal: help open %v, modal open %v", m.helpOpen(), m.topModal() == mod)
	}
	if got := m.keyhelp.Title(); got != "Help · Preview red" {
		t.Errorf("title = %q, want the modal's, clean", got)
	}
	if !slices.Contains(shownDescs(m), "close") || slices.Contains(shownDescs(m), "search") {
		t.Errorf("the help lists %q, want the modal's keys and not the app's", shownDescs(m))
	}
	for _, k := range []string{"x", "q"} {
		run(m, m.key(press(k)))
	}
	if len(mod.keys()) != 0 || m.keyhelp.Query() != "xq" {
		t.Errorf("the modal got %q and the query is %q, want the query to get the keys", mod.keys(), m.keyhelp.Query())
	}
	run(m, m.key(press("esc")))
	if !m.helpOpen() || m.keyhelp.Query() != "" {
		t.Errorf("esc should clear the query first: open %v, query %q", m.helpOpen(), m.keyhelp.Query())
	}
	run(m, m.key(press("esc")))
	if m.helpOpen() || m.topModal() != mod {
		t.Fatalf("esc on an empty query should close the help alone: help open %v", m.helpOpen())
	}
	run(m, m.key(press("x")))
	if !slices.Equal(mod.keys(), []string{"x"}) {
		t.Errorf("once the help closed the modal got %q, want x", mod.keys())
	}
}

// TestHelpNotWhileTyping checks that the help key types into an input
// rather than open the help, in a modal and in a capturing section.
func TestHelpNotWhileTyping(t *testing.T) {
	m, fakes := newTestApp(t)
	fakes[0].capturing = true
	run(m, m.key(press("?")))
	if m.helpOpen() || !fakes[0].got(isKey("?")) {
		t.Errorf("? in a section that types: help open %v, section got it %v", m.helpOpen(), fakes[0].got(isKey("?")))
	}
	if s := ansi.Strip(lastLine(m)); strings.Contains(s, "help") {
		t.Errorf("the bar offers the help while ? is typed: %q", s)
	}
	fakes[0].capturing = false

	mod := &fakeModal{title: "Find file", typing: true}
	run(m, ui.OpenModal(mod))
	run(m, m.key(press("?")))
	if m.helpOpen() || !slices.Equal(mod.keys(), []string{"?"}) {
		t.Errorf("? in a modal that types: help open %v, modal got %q", m.helpOpen(), mod.keys())
	}
}

// TestHelpKeyThatTypesNothing checks that a help key that types nothing,
// such as f1, still opens the help while ? is typed.
func TestHelpKeyThatTypesNothing(t *testing.T) {
	cfg := config.Default()
	cfg.Keys.Set(config.ActionHelp, []string{"?", "f1"})
	m := New(t.Context(), cfg, Layout{Files: &fakeSection{title: "Files"}}, WithRepo(testRepo))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mod := &fakeModal{title: "Find file", typing: true}
	run(m, ui.OpenModal(mod))
	if s := ansi.Strip(lastLine(m)); !strings.Contains(s, "f1 help") {
		t.Errorf("the bar should offer f1 alone: %q", s)
	}
	run(m, m.key(press("?")))
	if m.helpOpen() {
		t.Fatal("? opened the help in a modal that types")
	}
	run(m, m.key(tea.KeyPressMsg{Code: tea.KeyF1}))
	if !m.helpOpen() {
		t.Fatal("f1 didn't open the help")
	}
	run(m, m.key(tea.KeyPressMsg{Code: tea.KeyF1}))
	if m.helpOpen() {
		t.Error("f1 didn't close the help")
	}
}

// TestHelpFilter checks that the query lists only the rows it matches,
// fuzzily, in their order, and that disabled rows show.
func TestHelpFilter(t *testing.T) {
	m, _ := newTestApp(t)
	run(m, m.key(press("?")))
	all := len(m.keyhelp.Shown())
	var unzoom keyhelp.Status = -1
	for _, r := range m.keyhelp.Rows() {
		if r.Binding.Help().Desc == "unzoom" {
			unzoom = r.Status
		}
	}
	if unzoom != keyhelp.Disabled {
		t.Errorf("unzoom is %v, want listed as disabled", unzoom)
	}
	typeKeys(m, "zm")
	got := shownDescs(m)
	if !slices.Equal(got, []string{"zoom", "unzoom"}) {
		t.Errorf("zm shows %q of %d, want zoom and unzoom", got, all)
	}
	typeKeys(m, "xyz")
	if len(m.keyhelp.Shown()) != 0 || !strings.Contains(onScreen(m), "No keys match.") {
		t.Errorf("zmxyz shows %q, want nothing, and says so", shownDescs(m))
	}
}

// TestHelpCapture checks that tab captures keys to find what they do,
// even keys that would act, but ctrl+c, which still quits.
func TestHelpCapture(t *testing.T) {
	m, _ := newTestApp(t)
	run(m, m.key(press("?")))
	run(m, m.key(press("tab")))
	if !m.keyhelp.Capturing() {
		t.Fatal("tab didn't capture keys")
	}
	for _, k := range []string{"q", "esc", "?"} {
		msg := press(k)
		cmd := m.key(msg)
		if cmd != nil {
			if _, ok := cmd().(tea.QuitMsg); ok {
				t.Fatalf("%s quit while captured", k)
			}
		}
		run(m, cmd)
		if !m.helpOpen() || m.keyhelp.Key() != msg.String() {
			t.Fatalf("%s: help open %v, key %q", k, m.helpOpen(), m.keyhelp.Key())
		}
		for _, r := range m.keyhelp.Shown() {
			if !slices.Contains(r.Binding.Keys(), msg.String()) {
				t.Errorf("%s shows %q, which doesn't hold it", k, r.Binding.Keys())
			}
		}
	}
	if got := shownDescs(m); !slices.Equal(got, []string{"help"}) {
		t.Errorf("? shows %q, want the help", got)
	}
	run(m, m.key(press("tab")))
	if m.keyhelp.Capturing() || m.keyhelp.Key() != "?" {
		t.Errorf("tab again: capturing %v, key %q; want the key kept", m.keyhelp.Capturing(), m.keyhelp.Key())
	}
	run(m, m.key(press("tab")))
	if cmd := m.key(ctrlC); cmd == nil {
		t.Error("ctrl+c didn't quit while keys are captured")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c while keys are captured should quit")
	}
}

// TestHelpShowsCollisions checks that the help lists both bindings of a
// key that two hold, with the one that loses it marked, rather than one.
func TestHelpShowsCollisions(t *testing.T) {
	m, fakes := newTestApp(t)
	fakes[0].keyMap = collideKeys{}
	run(m, m.key(press("?")))
	var search, find *keyhelp.Row
	for _, r := range m.keyhelp.Rows() {
		switch r.Binding.Help().Desc {
		case "search":
			search = &r
		case "find":
			find = &r
		}
	}
	if search == nil || find == nil {
		t.Fatalf("the help lists search %v and find %v, want both", search, find)
	}
	if search.Status != keyhelp.Active || find.Status != keyhelp.Shadowed ||
		len(find.Lost) != 1 || find.Lost[0].By.Help().Desc != "search" {
		t.Errorf("search is %v and find %v losing %+v, want find shadowed by search", search.Status, find.Status, find.Lost)
	}
	typeKeys(m, "find")
	if !strings.Contains(onScreen(m), "⚠ S find") || !strings.Contains(onScreen(m), "↳ S: search · global") {
		t.Errorf("the collision isn't marked:\n%s", ansi.Strip(m.View().Content))
	}
}

// TestHelpFollowsResizeAndTheme checks that the open help takes a new size
// and theme.
func TestHelpFollowsResizeAndTheme(t *testing.T) {
	m, _ := newTestApp(t)
	run(m, m.key(press("?")))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if w, h := m.helpSize(); m.keyhelp.Width() != w || m.keyhelp.Height() != h || w <= 76 {
		t.Errorf("the help is %dx%d, want %dx%d", m.keyhelp.Width(), m.keyhelp.Height(), w, h)
	}
	before := m.keyhelp.Styles().Key
	m.Update(tea.BackgroundColorMsg{Color: lipgloss.White})
	if m.keyhelp.Styles().Key.GetForeground() == before.GetForeground() {
		t.Error("the help kept the dark theme's styles on a light terminal")
	}
}

// TestHelpView checks the help at 80 and 120 columns, in the light and the
// dark theme, over a section whose key collides with the app's.
func TestHelpView(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := "light"
		if dark {
			theme = "dark"
		}
		for _, size := range [][2]int{{80, 24}, {120, 40}} {
			t.Run(theme+"/"+strconv.Itoa(size[0]), func(t *testing.T) {
				m, fakes := newTestApp(t)
				fakes[0].keyMap = collideKeys{}
				m.applyTheme(dark)
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				run(m, m.key(press("?")))
				golden.RequireEqual(t, m.View().Content)
			})
		}
	}
}

// TestHelpViewFiltered checks the help with a query at 80 columns, and a
// key captured at 120.
func TestHelpViewFiltered(t *testing.T) {
	m, _ := newTestApp(t)
	run(m, m.key(press("?")))
	typeKeys(m, "pane")
	t.Run("query", func(t *testing.T) { golden.RequireEqual(t, m.View().Content) })
	run(m, m.key(press("esc")))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	run(m, m.key(press("tab")))
	run(m, m.key(press("q")))
	t.Run("key", func(t *testing.T) { golden.RequireEqual(t, m.View().Content) })
}

func BenchmarkViewWithHelp(b *testing.B) {
	m, _ := benchApp(b)
	run(m, m.key(press("?")))
	b.ReportAllocs()
	for b.Loop() {
		m.View()
	}
}

// BenchmarkTypeInHelp types a letter into the open help's filter and
// deletes it, drawing a frame after each key, as the user sees it.
func BenchmarkTypeInHelp(b *testing.B) {
	m, _ := benchApp(b)
	run(m, m.key(press("?")))
	keys := []tea.KeyPressMsg{press("a"), {Code: tea.KeyBackspace}}
	b.ReportAllocs()
	for b.Loop() {
		for _, k := range keys {
			m.Update(k)
			_ = m.View()
		}
	}
}

// quits reports whether cmd quits the program.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// TestCtrlCAlwaysQuits checks that ctrl+c quits whatever quit is bound to,
// since the config can't bind it, from a section that takes every key, the
// help and a modal, and that the help lists it with the quit key.
func TestCtrlCAlwaysQuits(t *testing.T) {
	cfg := config.Default()
	cfg.Keys.Set(config.ActionQuit, []string{"x"})
	files := &fakeSection{title: "Files"}
	m := New(t.Context(), cfg, Layout{Files: files}, WithRepo(testRepo))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	for _, capturing := range []bool{false, true} {
		files.capturing, files.msgs = capturing, nil
		if !quits(m.key(ctrlC)) || files.got(isKey("ctrl+c")) {
			t.Errorf("capturing %v: ctrl+c didn't quit, or reached the section", capturing)
		}
		if got := winner(m, "ctrl+c"); got != "global: quit" {
			t.Errorf("capturing %v: ctrl+c reaches %q, want the quit", capturing, got)
		}
	}
	files.capturing = false
	if !quits(m.key(press("x"))) {
		t.Error("the key bound to quit didn't quit")
	}

	run(m, m.key(press("?")))
	if !quits(m.key(ctrlC)) {
		t.Error("ctrl+c didn't quit from the help")
	}
	run(m, m.key(press("?")))

	run(m, ui.OpenModal(&fakeModal{title: "Preview"}))
	if !quits(m.key(ctrlC)) {
		t.Error("ctrl+c didn't quit from a modal")
	}
	if got := winner(m, "ctrl+c"); got != "always: quit" {
		t.Errorf("ctrl+c reaches %q in a modal, want the quit", got)
	}
}

// TestHelpClosesForNewModal checks that a modal that opens while the help
// is open, such as one a goto opens once GitHub answers, closes the help,
// which listed the keys of what had them before.
func TestHelpClosesForNewModal(t *testing.T) {
	m, _ := newTestApp(t)
	run(m, m.key(press("?")))
	mod := &fakeModal{title: "Preview"}
	run(m, ui.OpenModal(mod))
	if m.helpOpen() || m.topModal() != mod {
		t.Fatalf("help open %v, modal open %v; want the modal alone", m.helpOpen(), m.topModal() == mod)
	}
	run(m, m.key(press("x")))
	if !slices.Equal(mod.keys(), []string{"x"}) {
		t.Errorf("the modal got %q, want x", mod.keys())
	}
}

// pagerModal is a modal of a pager, as the preview of a file is, whose
// pager types the keys it waits for.
type pagerModal struct {
	fakeModal
	pager pager.Model
}

func (p *pagerModal) Update(msg tea.Msg) tea.Cmd {
	p.fakeModal.Update(msg)
	var cmd tea.Cmd
	p.pager, cmd = p.pager.Update(msg)
	return cmd
}

func (p *pagerModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{keyhelp.FromHelp("pager", p.pager, p.pager.Capturing())}
}

// TestHelpKeyReachesWaitingPager checks that ? goes to a pager that waits
// for the name of an option.
func TestHelpKeyReachesWaitingPager(t *testing.T) {
	for _, first := range []string{"-"} {
		m, _ := newTestApp(t)
		mod := &pagerModal{title: "README.md", pager: pager.New(pager.WithKeyMap(pager.NewKeyMap(ui.In(config.Default().Keys, "preview").Of)), pager.WithSize(60, 10))}
		mod.pager.Focus()
		run(m, mod.pager.SetContent("README.md", strings.Repeat("line\n", 50)))
		run(m, ui.OpenModal(mod))
		run(m, m.key(press(first)))
		if !mod.pager.Capturing() {
			t.Fatalf("%s: the pager doesn't wait for a key", first)
		}
		run(m, m.key(press("?")))
		if m.helpOpen() || !slices.Equal(mod.keys(), []string{first, "?"}) {
			t.Errorf("%s then ?: help open %v, pager got %q", first, m.helpOpen(), mod.keys())
		}
	}
}
