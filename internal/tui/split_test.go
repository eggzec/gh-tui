package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// splitFake is a section with a preview box, which records its size.
type splitFake struct {
	fakeSection
	sizes [][2]int
	keys  []string // keys PreviewKey got
	cmd   tea.Cmd  // handed on once by PreviewCmd
}

type previewCmdMsg struct{}

func (s *splitFake) SetPreviewSize(w, h int) { s.sizes = append(s.sizes, [2]int{w, h}) }
func (s *splitFake) PreviewTitle() string    { return "Preview" }
func (s *splitFake) PreviewView() string     { return "preview content" }
func (s *splitFake) PreviewKey(msg tea.KeyPressMsg) tea.Cmd {
	s.keys = append(s.keys, msg.String())
	return nil
}
func (s *splitFake) SetPreviewFocus(bool) {}
func (s *splitFake) PreviewCmd() tea.Cmd {
	cmd := s.cmd
	s.cmd = nil
	return cmd
}

var _ ui.Splitter = (*splitFake)(nil)

func newSplitApp(t *testing.T, w, h int) (*Model, *splitFake) {
	t.Helper()
	files := &splitFake{}
	files.title = "Files"
	others := []*fakeSection{{title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"}}
	layout := Layout{Files: files, Pulls: others[0], Issues: others[1], Notifications: others[2]}
	m := New(t.Context(), config.Default(), layout, WithRepo(testRepo))
	rankFakes(m)
	m.toast.SetDuration(0)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m, files
}

func TestSplitZoomed(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	run(m, m.key(press("z")))
	if got, want := files.sizes[len(files.sizes)-1], [2]int{120 - 46 - 2, m.contentHeight() - 2}; got != want {
		t.Fatalf("preview size = %v, want %v", got, want)
	}
	if files.width != 44 {
		t.Errorf("list width = %d, want 44 inside the frame", files.width)
	}
	if s := onScreen(m); !strings.Contains(s, "preview content") || !strings.Contains(s, "Preview") {
		t.Errorf("screen has no preview box:\n%s", s)
	}
	golden.RequireEqual(t, m.View().Content)
}

func TestSplitZoomedHintBar(t *testing.T) {
	t.Parallel()
	m, _ := newSplitApp(t, 120, 40)
	run(m, m.key(press("z")))
	if s := onScreen(m); !strings.Contains(s, "switch split") {
		t.Errorf("hint bar has no switch split hint:\n%s", s)
	}
}

// The command the preview queues is returned by Update, once.
func TestSplitPreviewCmd(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	run(m, m.key(press("z")))
	files.cmd = func() tea.Msg { return previewCmdMsg{} }
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd == nil {
		t.Fatal("Update returned no command")
	}
	var found func(tea.Msg) bool
	found = func(msg tea.Msg) bool {
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, c := range b {
				if c != nil && found(c()) {
					return true
				}
			}
			return false
		}
		_, ok := msg.(previewCmdMsg)
		return ok
	}
	if !found(cmd()) {
		t.Error("the preview's command is not in what Update returned")
	}
	if _, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40}); cmd != nil && found(cmd()) {
		t.Error("the preview's command was handed on twice")
	}
}

// Help offers tab only where it works: as "switch split" beside a preview,
// not on a narrow screen that has none, and not zoomed over a pane without one.
func TestSplitTabHelp(t *testing.T) {
	t.Parallel()
	m, _ := newSplitApp(t, 120, 40)
	if got := winner(m, "tab"); got != "global: next pane" {
		t.Errorf("unzoomed tab reaches %q, want next pane", got)
	}
	run(m, m.key(press("z")))
	if got := winner(m, "tab"); got != "global: switch split" {
		t.Errorf("zoomed tab reaches %q, want switch split", got)
	}

	m, _ = newSplitApp(t, 80, 24)
	run(m, m.key(press("z")))
	if got := winner(m, "tab"); got != "nothing" {
		t.Errorf("narrow tab reaches %q, want nothing", got)
	}
	if got := winner(m, "shift+tab"); got != "nothing" {
		t.Errorf("narrow shift+tab reaches %q, want nothing", got)
	}

	m, _, _ = newFilterApp(t)
	run(m, m.key(press("z")))
	if m.ctl.Zoom.User() && m.splitShown() {
		t.Fatal("the pane has a split, so the case is not under test")
	}
	if m.ctl.Zoom.User() {
		if got := winner(m, "tab"); got != "nothing" {
			t.Errorf("zoomed tab without a preview reaches %q, want nothing", got)
		}
	}
}

func TestSplitNarrow(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 80, 24)
	run(m, m.key(press("z")))
	if s := onScreen(m); strings.Contains(s, "preview content") {
		t.Errorf("a narrow screen has a preview box:\n%s", s)
	}
	for _, size := range files.sizes {
		if size != [2]int{} {
			t.Errorf("SetPreviewSize%v on a narrow screen", size)
		}
	}
}

// Zoomed beside its preview, tab switches between the list and the preview
// and leaves the pane alone; the preview takes the keys until tab again.
func TestSplitTabSwitchesFocus(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	run(m, m.key(press("z")))
	pane := m.focus
	run(m, m.key(press("tab")))
	if !m.previewFocused() || m.focus != pane {
		t.Fatalf("after tab: prevFocus = %v, focus = %d, want true and %d", m.previewFocused(), m.focus, pane)
	}
	golden.RequireEqual(t, m.View().Content)
	run(m, m.key(press("j")))
	if len(files.keys) != 1 || files.keys[0] != "j" {
		t.Errorf("preview keys = %v, want [j]", files.keys)
	}
	run(m, m.key(press("shift+tab")))
	if m.previewFocused() || m.focus != pane {
		t.Errorf("after shift+tab: prevFocus = %v, focus = %d", m.previewFocused(), m.focus)
	}
	run(m, m.key(press("tab")))
	run(m, m.key(press("z")))
	if m.previewFocused() {
		t.Error("the preview keeps the focus after unzoom")
	}
}

// Not zoomed, tab cycles the panes. On a narrow screen, which is zoomed with
// no room for a preview, it does nothing, and the digits switch pane.
func TestSplitTabCyclesPanes(t *testing.T) {
	t.Parallel()
	m, _ := newSplitApp(t, 120, 40)
	pane := m.focus
	run(m, m.key(press("tab")))
	if m.focus == pane || m.previewFocused() {
		t.Errorf("unzoomed: tab left focus = %d (was %d), preview focused = %v", m.focus, pane, m.previewFocused())
	}
	m, _ = newSplitApp(t, 80, 24)
	pane = m.focus
	run(m, m.key(press("tab")))
	run(m, m.key(press("shift+tab")))
	if m.focus != pane || m.previewFocused() {
		t.Errorf("narrow: tab left focus = %d (was %d), preview focused = %v", m.focus, pane, m.previewFocused())
	}
	run(m, m.key(press("2")))
	if m.focus != 1 {
		t.Errorf("narrow: 2 left focus = %d, want 1", m.focus)
	}
	// Wide but short is narrow too: no preview shows, so Tab has none to flip to.
	m, _ = newSplitApp(t, 160, 24)
	pane = m.focus
	run(m, m.key(press("tab")))
	if m.focus != pane || m.previewFocused() {
		t.Errorf("wide and short: tab left focus = %d (was %d), preview focused = %v", m.focus, pane, m.previewFocused())
	}
}

// Backspace drops the user's zoom before it goes back.
func TestSplitBackDropsZoom(t *testing.T) {
	t.Parallel()
	m, _ := newSplitApp(t, 120, 40)
	run(m, m.key(press("z")))
	if !m.zoomed() {
		t.Fatal("z didn't zoom")
	}
	run(m, m.key(press("backspace")))
	if m.zoomed() {
		t.Error("backspace left the zoom on")
	}
}

// A digit zoomed beside a preview focuses its pane's list, and Tab does nothing
// on a pane with no preview.
func TestSplitDigitsSwitchPane(t *testing.T) {
	t.Parallel()
	m, _ := newSplitApp(t, 120, 40)
	run(m, m.key(press("z")))
	run(m, m.key(press("tab")))
	if !m.previewFocused() {
		t.Fatal("tab didn't focus the preview")
	}
	run(m, m.key(press("1")))
	if m.previewFocused() || m.focus != 0 {
		t.Errorf("1 left preview focused = %v, focus = %d; want the list of pane 0", m.previewFocused(), m.focus)
	}
	run(m, m.key(press("2")))
	run(m, m.key(press("tab")))
	if m.focus != 1 || m.previewFocused() || !m.zoomed() {
		t.Errorf("tab on a pane with no preview: focus = %d, preview focused = %v, zoomed = %v", m.focus, m.previewFocused(), m.zoomed())
	}
}

func TestSplitUnzoomed(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	if len(files.sizes) != 0 {
		t.Errorf("SetPreviewSize called %v while unzoomed", files.sizes)
	}
	run(m, m.key(press("z")))
	run(m, m.key(press("z")))
	if last := files.sizes[len(files.sizes)-1]; last != [2]int{} {
		t.Errorf("preview size after unzoom = %v, want 0x0", last)
	}
}
