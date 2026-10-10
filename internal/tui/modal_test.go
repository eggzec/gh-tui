package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// fakeModal records what the app asks of it.
type fakeModal struct {
	title         string
	width, height int
	themed        bool
	msgs          []tea.Msg
	// typing is whether the modal types keys into an input.
	typing bool
}

func (f *fakeModal) Title() string { return f.title }
func (f *fakeModal) Update(msg tea.Msg) tea.Cmd {
	f.msgs = append(f.msgs, msg)
	return nil
}

func (f *fakeModal) View() string {
	rows := make([]string, f.height)
	rows[0] = f.title + " body"
	for i := range rows {
		rows[i] = lipgloss.PlaceHorizontal(f.width, lipgloss.Left, rows[i])
	}
	return strings.Join(rows, "\n")
}
func (f *fakeModal) SetSize(w, h int)  { f.width, f.height = w, h }
func (f *fakeModal) SetTheme(ui.Theme) { f.themed = true }
func (f *fakeModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{keyhelp.FromHelp(f.title, sectionKeys{}, f.typing)}
}

// Commands implements ui.Commanded: the fake takes none that acts on it.
func (f *fakeModal) Commands() []string { return nil }

func (f *fakeModal) keys() []string {
	var ks []string
	for _, msg := range f.msgs {
		if k, ok := msg.(tea.KeyPressMsg); ok {
			ks = append(ks, k.String())
		}
	}
	return ks
}

// TestModalTakesEveryKey checks that a modal takes every key but ctrl+c,
// which quits, and the help key, which opens the help over it.
func TestModalTakesEveryKey(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	mod := &fakeModal{title: "Preview"}
	run(m, ui.OpenModal(mod))
	if !mod.themed || mod.width == 0 || mod.height == 0 {
		t.Fatalf("modal opened without theme or size: themed %v, %dx%d", mod.themed, mod.width, mod.height)
	}

	for _, k := range []string{"q", "tab", "x"} {
		run(m, m.key(press(k)))
	}
	if got := mod.keys(); !slices.Equal(got, []string{"q", "tab", "x"}) {
		t.Errorf("modal got keys %v, want all three", got)
	}
	if fakes[0].got(func(msg tea.Msg) bool { _, ok := msg.(tea.KeyPressMsg); return ok }) {
		t.Error("a section got a key while a modal was open")
	}
	if m.focus != 0 || m.helpOpen() {
		t.Errorf("app keys acted under a modal: focus %d, help open %v", m.focus, m.helpOpen())
	}

	cmd := m.key(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c didn't quit under a modal")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c under a modal should quit")
	}
}

func TestModalGetsOtherMessages(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	mod := &fakeModal{title: "Preview"}
	run(m, ui.OpenModal(mod))
	run(m, func() tea.Msg { return ui.DoneMsg{What: "x"} })
	if !slices.ContainsFunc(mod.msgs, func(msg tea.Msg) bool { _, ok := msg.(ui.DoneMsg); return ok }) {
		t.Error("the modal missed a message sent to everyone")
	}
}

// hidingModal is a modal that stops what it does while another is open in
// its place.
type hidingModal struct {
	fakeModal
	hidden int
}

func (h *hidingModal) Hide() { h.hidden++ }

// settlingModal is a modal that waits out a resize, over which the
// command line opens.
type settlingModal struct {
	fakeModal
	settles int
}

// modalSettledMsg ends the wait of a settlingModal.
type modalSettledMsg struct{}

// KeyLayers are those of a modal over which the command key opens the
// line.
func (s *settlingModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{{Source: "preview", Context: "preview"}}
}

func (s *settlingModal) Settle() tea.Cmd {
	s.settles++
	return func() tea.Msg { return modalSettledMsg{} }
}

// A resize of the terminal has the open modal wait it out, and the
// message that ends the wait reaches it; a modal that waits out nothing
// is resized alike.
func TestModalSettlesAfterAResize(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	plain := &fakeModal{title: "Plain"}
	run(m, ui.OpenModal(plain))
	run(m, func() tea.Msg { return tea.WindowSizeMsg{Width: 90, Height: 30} })
	if w, h := m.modalSize(); plain.width != w || plain.height != h {
		t.Errorf("the modal is %dx%d after the resize, want %dx%d", plain.width, plain.height, w, h)
	}
	mod := &settlingModal{}
	mod.title = "README.md"
	run(m, ui.OpenModal(mod))
	run(m, func() tea.Msg { return tea.WindowSizeMsg{Width: 100, Height: 30} })
	if mod.settles != 1 {
		t.Errorf("the modal settled %d times after the resize, want once", mod.settles)
	}
	if !slices.ContainsFunc(mod.msgs, func(msg tea.Msg) bool { _, ok := msg.(modalSettledMsg); return ok }) {
		t.Error("the end of the wait didn't reach the modal")
	}
}

// A change of the footer resizes the open modal as a resize of the
// terminal does, and the modal waits it out alike: on a short terminal,
// the candidates of the command line take a row from it, which may
// narrow what it renders.
func TestModalSettlesWhenTheFooterChanges(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	mod := &settlingModal{}
	mod.title = "README.md"
	run(m, ui.OpenModal(mod))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 15})
	if mod.settles != 1 {
		t.Errorf("the modal settled %d times after the terminal's resize, want once", mod.settles)
	}
	h := mod.height
	m.Update(press(":"))
	m.Update(press("s"))
	if !m.line.Focused() {
		t.Fatal("the command line didn't open over the modal")
	}
	if mod.height == h {
		t.Fatalf("the command line left the modal %d rows high", h)
	}
	if mod.settles != 2 {
		t.Errorf("the modal settled %d times after the command line took a row, want twice", mod.settles)
	}
	m.Update(press("e"))
	if mod.settles != 2 {
		t.Errorf("the modal settled %d times after a key that resized nothing, want still twice", mod.settles)
	}
}

func TestOpeningAModalHidesTheOpenOne(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	first := &hidingModal{}
	first.title = "Actions"
	run(m, ui.OpenModal(first))
	run(m, ui.OpenModal(first))
	if first.hidden != 0 {
		t.Fatal("opening the open modal again hid it")
	}
	run(m, ui.OpenModal(&fakeModal{title: "Preview"}))
	if first.hidden != 1 {
		t.Errorf("the replaced modal was hidden %d times, want once", first.hidden)
	}
}

func TestOpeningAModalReplacesTheOpenOne(t *testing.T) {
	t.Parallel()
	m, fakes := newTestApp(t)
	first, second := &fakeModal{title: "Search"}, &fakeModal{title: "Preview"}
	run(m, ui.OpenModal(first))
	run(m, ui.OpenModal(second))
	if m.topModal() != second {
		t.Fatal("the second modal isn't open")
	}
	run(m, func() tea.Msg { return ui.DoneMsg{What: "x"} })
	run(m, m.key(press("x")))
	if len(first.msgs) != 0 {
		t.Errorf("the replaced modal still got %d messages", len(first.msgs))
	}
	if len(second.keys()) != 1 {
		t.Error("the open modal should get keys")
	}

	// Closing the replaced modal leaves the open one alone.
	run(m, ui.CloseModal(first))
	if m.topModal() != second {
		t.Fatal("closing a replaced modal closed the open one")
	}
	run(m, ui.CloseModal(second))
	if m.topModal() != nil {
		t.Fatal("the modal is still open")
	}
	// Closing again is harmless.
	run(m, ui.CloseModal(second))

	run(m, m.key(press("x")))
	if !fakes[0].got(func(msg tea.Msg) bool { k, ok := msg.(tea.KeyPressMsg); return ok && k.String() == "x" }) {
		t.Error("keys didn't go back to the section after the modal closed")
	}
}

func TestModalIsDrawnInAFrameOverTheScreen(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	mod := &fakeModal{title: "go.mod"}
	run(m, ui.OpenModal(mod))

	out := m.View().Content
	if w, h := lipgloss.Width(out), lipgloss.Height(out); w != 80 || h != 24 {
		t.Errorf("screen is %dx%d, want 80x24", w, h)
	}
	s := onScreen(m)
	for _, want := range []string{"╭─ go.mod ─", "go.mod body", "Pull requests"} {
		if !strings.Contains(s, want) {
			t.Errorf("screen lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(s, "quit") {
		t.Error("help shows the app's keys under a modal")
	}

	fw, fh := m.frameSize()
	if mod.width != fw-4 || mod.height != fh-2 {
		t.Errorf("modal size %dx%d, want the frame %dx%d less its edges", mod.width, mod.height, fw, fh)
	}
	run(m, func() tea.Msg { return tea.WindowSizeMsg{Width: 120, Height: 40} })
	if fw, fh = m.frameSize(); mod.width != fw-4 || mod.height != fh-2 {
		t.Errorf("modal not resized with the terminal: %dx%d", mod.width, mod.height)
	}
}

func BenchmarkViewWithModal(b *testing.B) {
	m, _ := benchApp(b)
	m.Update(ui.OpenModalMsg{Modal: &fakeModal{title: "Preview"}})
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}
