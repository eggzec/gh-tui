package issues

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// openDev opens the first issue at w by 30 with the Development pane focused
// and the zoom as zoom says, where the width allows it.
func openDev(t *testing.T, w int, zoom bool) *detailModal {
	t.Helper()
	h := started(t, blockedSvc(), w, 30)
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	press(t, h, "4")
	if m.zoom.Zoomed() != zoom {
		press(t, h, "z")
	}
	return m
}

func TestZoomedSplitPaneDrawnOnce(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		w      int
		zoom   bool
		zoomed bool
	}{
		{99, true, true},
		{100, true, true},
		{119, true, true},
		{120, true, true},
		{120, false, false},
	} {
		t.Run(fmt.Sprintf("%d zoom %v", tc.w, tc.zoom), func(t *testing.T) {
			t.Parallel()
			m := openDev(t, tc.w, tc.zoom)
			l := m.paneLayout()
			if l.Zoomed != tc.zoomed {
				t.Fatalf("zoomed = %v, want %v", l.Zoomed, tc.zoomed)
			}
			n := 0
			for _, b := range l.Boxes {
				if b.Pane == 4 && b.Role != panes.Preview {
					n++
				}
			}
			if n != 1 {
				t.Errorf("%d boxes draw the pane, want 1", n)
			}
			v := ansi.Strip(m.View())
			assertFits(t, m.View(), tc.w, 30)
			title := "Development"
			if !tc.zoomed {
				title = "[4] Development"
			}
			if got := strings.Count(v, "┤ "+title) + strings.Count(v, " "+title+" "); got < 1 {
				t.Errorf("the title %q is missing:\n%s", title, v)
			}
			strip := strings.Count(v, "1 Desc")
			want := 0
			if tc.zoomed {
				want = 1
			}
			if strip != want {
				t.Errorf("the hint strip shows %d times, want %d:\n%s", strip, want, v)
			}
			if !tc.zoomed {
				return
			}
			if got := strings.Count(v, "╭─ Development"); got != 1 {
				t.Errorf("%d boxes are titled Development, want 1:\n%s", got, v)
			}
		})
	}
}

func TestPreviewFocusDropsOnResize(t *testing.T) {
	t.Parallel()
	m := openDev(t, 110, true)
	step := func() { m.nav.Step(m.paneLayout(), 1) }
	step()
	if !m.nav.InPreview(m.paneLayout()) {
		t.Fatal("the step didn't reach the preview at 110")
	}
	m.previewOff = 5
	m.SetSize(90, 30)
	if m.previewOff != 0 {
		t.Error("the offset stayed after the preview went")
	}
	if m.nav.Sub(4) != panes.List {
		t.Error("the focus stayed in the preview after the width fell under 100")
	}
	m.SetSize(110, 30)
	if m.nav.InPreview(m.paneLayout()) {
		t.Error("the preview took the focus back on a wider window")
	}
}

// longPreview opens the zoomed Involved pane of an issue with 40 comments.
func longPreview(t *testing.T) (*host, *detailModal) {
	t.Helper()
	svc := blockedSvc()
	for i := range 40 {
		svc.addComments(999, core.Comment{ID: fmt.Sprintf("IC_%d", i), Author: core.User{Login: "ann"}, Body: fmt.Sprintf("Early %d.", i)})
	}
	h := started(t, svc, 110, 30)
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	press(t, h, "3")
	m.View()
	return h, m
}

func TestPreviewFocusIsMarked(t *testing.T) {
	t.Parallel()
	_, m := longPreview(t)
	before := m.View()
	m.nav.Step(m.paneLayout(), 1)
	if !m.nav.InPreview(m.paneLayout()) {
		t.Fatal("the step didn't reach the preview")
	}
	inPreview := m.View()
	if inPreview == before {
		t.Error("the preview with the focus is drawn as it is without")
	}
	m.nav.Step(m.paneLayout(), -1)
	if m.View() != before {
		t.Error("the focus mark stayed after the step back")
	}
}

func TestPreviewScrolls(t *testing.T) {
	t.Parallel()
	h, m := longPreview(t)
	press(t, h, "tab")
	if !m.nav.InPreview(m.paneLayout()) {
		t.Fatal("tab didn't reach the preview")
	}
	plain := func() string { return ansi.Strip(m.View()) }
	if !strings.Contains(plain(), "Early 0.") || strings.Contains(plain(), "Early 39.") {
		t.Fatalf("the preview doesn't start at the top:\n%s", plain())
	}
	cur := m.side.inv.View()
	press(t, h, "j")
	if m.previewOff != 1 {
		t.Errorf("j moved the preview to %d, want 1", m.previewOff)
	}
	press(t, h, "G")
	if !strings.Contains(plain(), "Early 39.") {
		t.Errorf("G doesn't show the last comment:\n%s", plain())
	}
	end := m.previewOff
	press(t, h, "j")
	if m.previewOff != end {
		t.Errorf("j past the end moved to %d, was %d", m.previewOff, end)
	}
	press(t, h, "home")
	if m.previewOff != 0 {
		t.Errorf("home left the preview at %d", m.previewOff)
	}
	press(t, h, "ctrl+d")
	if m.previewOff <= 1 {
		t.Errorf("ctrl+d moved only %d", m.previewOff)
	}
	if m.side.inv.View() != cur {
		t.Error("the moves reached the Involved list")
	}
	// The offset survives a step back and a narrower, then wider window.
	press(t, h, "G")
	end = m.previewOff
	m.SetSize(100, 30)
	m.SetSize(110, 30)
	assertFits(t, m.View(), 110, 30)
	if m.previewOff == 0 || !strings.Contains(plain(), "Early 39.") {
		t.Errorf("a resize lost the end of the preview (offset %d, was %d)", m.previewOff, end)
	}
	press(t, h, "shift+tab")
	if m.previewOff == 0 {
		t.Error("the step back reset the offset")
	}
	off := m.previewOff
	press(t, h, "j")
	if m.previewOff != off {
		t.Error("j scrolled the preview from the list")
	}
}

// enabledHelp lists the descriptions of the enabled bindings the help shows.
func enabledHelp(m *detailModal) map[string]bool {
	got := map[string]bool{}
	for _, l := range m.KeyLayers() {
		for _, b := range l.Bindings {
			if b.Enabled() {
				got[b.Help().Desc] = true
			}
		}
	}
	return got
}

func TestPreviewFocusHelpListsOnlyScrolling(t *testing.T) {
	t.Parallel()
	h, m := longPreview(t)
	if !enabledHelp(m)["expand"] {
		t.Fatalf("the help lacks expand on the list: %v", enabledHelp(m))
	}
	press(t, h, "tab")
	if !m.nav.InPreview(m.paneLayout()) {
		t.Fatal("tab didn't reach the preview")
	}
	got := enabledHelp(m)
	for _, d := range []string{m.keys.Assign.Help().Desc, "expand", "collapse", "all"} {
		if got[d] {
			t.Errorf("the help lists %q while the preview has the focus", d)
		}
	}
	for _, d := range []string{"up", "down", "first", "last", "close"} {
		if !got[d] {
			t.Errorf("the help lacks %q in the preview: %v", d, got)
		}
	}
}

func TestPreviewScrollsWithTheTopChord(t *testing.T) {
	t.Parallel()
	h, m := longPreview(t)
	press(t, h, "tab")
	press(t, h, "G")
	if m.previewOff == 0 {
		t.Fatal("G didn't scroll")
	}
	// The root turns the chord "g g" into the press of its action before the
	// modal sees it.
	acts := keymap.Actions(m.keys.panes.scroll.Home)
	if len(acts) == 0 {
		t.Fatal("the top move has no action")
	}
	run(t, h, h.Update(keymap.ActionPress(acts[0])))
	if m.previewOff != 0 {
		t.Errorf("the top chord left the preview at %d", m.previewOff)
	}
}

func TestInvolvedPreviewDrawsConversation(t *testing.T) {
	t.Parallel()
	svc := blockedSvc()
	svc.addComments(999, core.Comment{ID: "IC_p", Author: core.User{Login: "ann"}, Body: "Peeked at."})
	h := started(t, svc, 110, 30)
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	press(t, h, "3")
	l := m.paneLayout()
	b, ok := l.Box(paneInvolved, panes.Preview)
	if !ok {
		t.Fatal("the zoomed Involved pane has no preview")
	}
	body, ok := m.peekBody(b.Rect.Inner().W)
	if !ok || body == "" {
		t.Fatalf("the conversation isn't drawn: %q", body)
	}
	if again, _ := m.peekBody(b.Rect.Inner().W); again != body {
		t.Error("a second draw at the same width differs")
	}
	v := m.View()
	assertFits(t, v, 110, 30)
	if !strings.Contains(ansi.Strip(v), "Peeked at.") {
		t.Errorf("the preview lacks the comment:\n%s", ansi.Strip(v))
	}
}

type strayMsg struct{}

func TestPreviewKeepsDrawnOnStrayMessage(t *testing.T) {
	t.Parallel()
	svc := blockedSvc()
	svc.addComments(999, core.Comment{ID: "IC_p", Author: core.User{Login: "ann"}, Body: "Peeked at."})
	h := started(t, svc, 110, 30)
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	press(t, h, "3")
	// The first message settles the stamp of what the conversation draws.
	m.Update(strayMsg{})
	m.View()
	ver := m.peekVer
	m.Update(strayMsg{})
	m.Update(strayMsg{})
	m.View()
	if m.peekVer != ver {
		t.Errorf("a message the conversation ignores drew the preview again: %d, was %d", m.peekVer, ver)
	}
}

func TestPreviewDrawsChangePastViewport(t *testing.T) {
	t.Parallel()
	svc := blockedSvc()
	for i := range 40 {
		svc.addComments(999, core.Comment{ID: fmt.Sprintf("IC_%d", i), Author: core.User{Login: "ann"}, Body: fmt.Sprintf("Early %d.", i)})
	}
	h := started(t, svc, 110, 30)
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	press(t, h, "3")
	m.Update(strayMsg{})
	m.View()
	ver := m.peekVer
	// A comment below the fold: the viewport of the conversation keeps its rows.
	svc.addComments(999, core.Comment{ID: "IC_late", Author: core.User{Login: "bob"}, Body: "Added below the fold."})
	_, cmd := m.conv.Recache()
	// Feed each read's message through the modal.
	var feed func(c tea.Cmd)
	feed = func(c tea.Cmd) {
		if c == nil {
			return
		}
		msg := c()
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, x := range b {
				feed(x)
			}
			return
		}
		m.Update(msg)
	}
	feed(cmd)
	if m.peekVer == ver {
		t.Fatal("a comment past the viewport didn't draw the preview again")
	}
	l := m.paneLayout()
	b, ok := l.Box(paneInvolved, panes.Preview)
	if !ok {
		t.Fatal("the zoomed Involved pane has no preview")
	}
	body, _ := m.peekBody(b.Rect.Inner().W)
	if !strings.Contains(ansi.Strip(body), "Added below the fold.") {
		t.Errorf("the preview lacks the new comment:\n%s", ansi.Strip(body))
	}
}

func TestZoomedSplitPaneView(t *testing.T) {
	t.Parallel()
	for _, w := range []int{80, 110} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			t.Parallel()
			m := openDev(t, w, true)
			v := m.View()
			assertFits(t, v, w, 30)
			golden.RequireEqual(t, v)
		})
	}
}
