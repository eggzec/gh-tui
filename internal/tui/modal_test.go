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

func TestOpeningAModalHidesTheOpenOne(t *testing.T) {
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
