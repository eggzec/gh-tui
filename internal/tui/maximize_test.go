package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/access"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// ctxModal is a modal whose keys are those of the context ctx, as the
// real ones are, and which wants no more room than fit, if that is set.
type ctxModal struct {
	fakeModal
	ctx string
	fit [2]int
	// settles counts the waits out of a resize that the modal ended.
	settles int
}

// Settle implements ui.Settler.
func (c *ctxModal) Settle() tea.Cmd {
	c.settles++
	return nil
}

// TakesCommands implements ui.Commanded: the command line opens over the
// modal unless it types.
func (c *ctxModal) TakesCommands() bool { return !c.typing }

func newCtxModal(ctx string) *ctxModal {
	c := &ctxModal{ctx: ctx}
	c.title = ctx
	return c
}

func (c *ctxModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{{Source: c.ctx, Context: c.ctx, Typing: c.typing}}
}

// Fit implements ui.Fitter when fit is set.
func (c *ctxModal) Fit(maxWidth, maxHeight int) (width, height int) {
	if c.fit == [2]int{} {
		return maxWidth, maxHeight
	}
	return min(maxWidth, c.fit[0]), min(maxHeight, c.fit[1])
}

func send(m *Model, k string) { m.Update(press(k)) }

func resize(m *Model, w, h int) { m.Update(tea.WindowSizeMsg{Width: w, Height: h}) }

// wantInner checks that the modal has the size inside a frame of w by h.
func wantInner(t *testing.T, mod *ctxModal, w, h int, what string) {
	t.Helper()
	if mod.width != w-4 || mod.height != h-2 {
		t.Errorf("%s: the modal is %dx%d, want %dx%d inside a %dx%d frame", what, mod.width, mod.height, w-4, h-2, w, h)
	}
}

// Z fills the screen but for the footer, and Z again restores the size.
func TestMaximizeToggles(t *testing.T) {
	m, _ := newTestApp(t)
	mod := newCtxModal("history")
	run(m, ui.OpenModal(mod))
	fw, fh := m.frameSize()
	if fw >= m.width || fh >= m.height-1 {
		t.Fatalf("the modal opens at %dx%d, want less than the screen", fw, fh)
	}
	wantInner(t, mod, fw, fh, "before")

	send(m, "Z")
	if !m.maximized {
		t.Fatal("Z didn't maximize the modal")
	}
	wantInner(t, mod, m.width, m.height-1, "maximized")
	if got := mod.keys(); len(got) != 0 {
		t.Errorf("the modal got %v, want Z taken by the app", got)
	}
	out := ansi.Strip(m.View().Content)
	lines := strings.Split(out, "\n")
	if len(lines) != m.height || lipgloss.Width(out) != m.width {
		t.Fatalf("the screen is %dx%d, want %dx%d", lipgloss.Width(out), len(lines), m.width, m.height)
	}
	if !strings.HasPrefix(lines[0], "╭─ history ─") || !strings.HasSuffix(lines[0], "╮") {
		t.Errorf("the frame doesn't start at the top left and span the width: %q", lines[0])
	}
	if strings.HasPrefix(lines[m.height-1], "╰") || strings.HasPrefix(lines[m.height-2], "╰") == false {
		t.Errorf("the frame should end above the footer:\n%s", out)
	}

	send(m, "Z")
	if m.maximized {
		t.Fatal("Z again didn't restore the modal")
	}
	wantInner(t, mod, fw, fh, "restored")
}

// A maximized modal ignores the size it fits in, and the size it fits in
// returns once it is restored.
func TestMaximizeIgnoresFit(t *testing.T) {
	m, _ := newTestApp(t)
	mod := newCtxModal("filter")
	mod.fit = [2]int{30, 8}
	run(m, ui.OpenModal(mod))
	wantInner(t, mod, 34, 10, "fit")

	send(m, "Z")
	wantInner(t, mod, m.width, m.height-1, "maximized")
	// What the modal asks for changing doesn't shrink it.
	mod.fit = [2]int{20, 5}
	send(m, "x")
	wantInner(t, mod, m.width, m.height-1, "maximized after the fit changed")

	send(m, "Z")
	wantInner(t, mod, 24, 7, "restored")
}

// A resize of the terminal keeps the modal maximized, and restored.
func TestMaximizeSurvivesAResize(t *testing.T) {
	m, _ := newTestApp(t)
	mod := newCtxModal("preview")
	run(m, ui.OpenModal(mod))
	send(m, "Z")
	for _, size := range [][2]int{{120, 40}, {60, 20}} {
		resize(m, size[0], size[1])
		if !m.maximized || mod.width != size[0]-4 || mod.height != size[1]-1-2 {
			t.Errorf("after a resize to %dx%d the modal is %dx%d, maximized %v", size[0], size[1], mod.width, mod.height, m.maximized)
		}
	}
	if mod.settles == 0 {
		t.Error("the modal didn't wait out the change of its size")
	}
	resize(m, 100, 30)
	send(m, "Z")
	fw, fh := m.frameSize()
	if m.maximized || fw >= 100 || mod.width != fw-4 || mod.height != fh-2 {
		t.Errorf("restored and resized, the modal is %dx%d in a %dx%d frame, maximized %v", mod.width, mod.height, fw, fh, m.maximized)
	}
}

// ui.maximized lists the modals that open maximized, and opening another
// modal starts it from its own default.
func TestMaximizeDefaults(t *testing.T) {
	m, _ := newTestApp(t)
	m.cfg.UI.Maximized = []string{"history", "text"}
	m.applySettings()

	hist := newCtxModal("history")
	run(m, ui.OpenModal(hist))
	if !m.maximized {
		t.Fatal("history opened small, though ui.maximized lists it")
	}
	wantInner(t, hist, m.width, m.height-1, "history")

	other := newCtxModal("preview")
	run(m, ui.OpenModal(other))
	if m.maximized {
		t.Error("the preview opened maximized, though ui.maximized doesn't list it")
	}
	// What the user did to the one before isn't kept.
	send(m, "Z")
	run(m, ui.OpenModal(hist))
	if !m.maximized {
		t.Error("history lost its default after another modal was maximized")
	}
	send(m, "Z")
	run(m, ui.OpenModal(other))
	if m.maximized {
		t.Error("the preview kept the state of the history")
	}

	// A modal that is no key context has no default.
	run(m, ui.OpenModal(&fakeModal{title: "x"}))
	if m.maximized {
		t.Error("a modal without a context opened maximized")
	}
}

// A step that shows inside a modal, such as the checks of a pull request,
// is that modal, so the pull request is maximized however it was opened.
func TestMaximizeDefaultsOfSteps(t *testing.T) {
	for ctx, listed := range map[string]string{"pull_checks": "pull_modal", "actions_filter": "actions", "pull_modal": "pull_modal"} {
		m, _ := newTestApp(t)
		m.cfg.UI.Maximized = []string{listed}
		m.applySettings()
		run(m, ui.OpenModal(newCtxModal(ctx)))
		if !m.maximized {
			t.Errorf("a modal opened on %s didn't open maximized, though ui.maximized lists %s", ctx, listed)
		}
		m.cfg.UI.Maximized = []string{"history"}
		m.applySettings()
		run(m, ui.OpenModal(newCtxModal(ctx)))
		if m.maximized {
			t.Errorf("a modal opened on %s opened maximized, though ui.maximized doesn't list %s", ctx, listed)
		}
	}
}

// :set ui.maximized applies at the next open, not to the open modal.
func TestMaximizeSet(t *testing.T) {
	var told []config.Config
	m, _ := newSetApp(t, userConfig(), &told)
	mod := newCtxModal("history")
	run(m, ui.OpenModal(mod))
	runCommand(t, m, "set ui.maximized=[history]")
	if len(told) != 1 || len(m.cfg.UI.Maximized) != 1 {
		t.Fatalf("the setting wasn't applied: %v", m.cfg.UI.Maximized)
	}
	if m.maximized {
		t.Error("the open modal changed with the setting")
	}
	run(m, ui.OpenModal(newCtxModal("history")))
	if !m.maximized {
		t.Error("the next modal didn't open maximized")
	}
}

// While an input has the keys, Z is typed there and never maximizes.
func TestMaximizeIsTypedInInputs(t *testing.T) {
	t.Run("a modal's input", func(t *testing.T) {
		m, _ := newTestApp(t)
		mod := newCtxModal("filter")
		mod.typing = true
		run(m, ui.OpenModal(mod))
		send(m, "Z")
		if m.maximized || len(mod.keys()) != 1 || mod.keys()[0] != "Z" {
			t.Errorf("maximized %v, the modal got %v; want Z typed", m.maximized, mod.keys())
		}
	})
	t.Run("a context that captures", func(t *testing.T) {
		m, _ := newTestApp(t)
		mod := newCtxModal("search_prompt")
		run(m, ui.OpenModal(mod))
		send(m, "Z")
		if m.maximized || len(mod.keys()) != 1 {
			t.Errorf("maximized %v, the modal got %v; want Z left to it", m.maximized, mod.keys())
		}
	})
	t.Run("the search prompt of a pager", func(t *testing.T) {
		m, _ := newTestApp(t)
		run(m, m.openText("go.mod", "go.mod", "alpha\nZulu\n", false))
		send(m, "/")
		send(m, "Z")
		if m.maximized {
			t.Error("Z maximized the modal while the search prompt was open")
		}
		if s := onScreen(m); !strings.Contains(s, "/Z") {
			t.Errorf("the search prompt didn't take the Z:\n%s", s)
		}
	})
	t.Run("the command line", func(t *testing.T) {
		m, _ := newTestApp(t)
		run(m, ui.OpenModal(newCtxModal("preview")))
		send(m, ":")
		if !m.line.Focused() {
			t.Fatal("the command line didn't open over the modal")
		}
		send(m, "Z")
		if m.maximized || m.line.Value() != "Z" {
			t.Errorf("maximized %v, the line holds %q; want Z typed", m.maximized, m.line.Value())
		}
	})
	t.Run("the help", func(t *testing.T) {
		m, _ := newTestApp(t)
		run(m, ui.OpenModal(newCtxModal("history")))
		send(m, "?")
		if !m.helpOpen() {
			t.Fatal("the help didn't open")
		}
		send(m, "Z")
		if m.maximized {
			t.Error("Z maximized the modal under the help")
		}
	})
	t.Run("a question", func(t *testing.T) {
		m, _ := newTestApp(t)
		run(m, ui.OpenModal(ui.NewConfirmModal(ui.Confirm{Question: "Close it?"}, ui.NewConfirmKeys(config.Default().Keys), ui.NewIcons(config.IconsASCII))))
		send(m, "Z")
		if m.maximized {
			t.Error("Z maximized the question")
		}
	})
}

// The token modal maximizes, except while it asks its question, which
// takes every key.
func TestMaximizeTokenModal(t *testing.T) {
	m, _ := newTestApp(t)
	mod := newAuthModal(config.Default().Keys, "octocat@github.com", core.Access{}, access.Plan{}, uitest.Token(&uitest.Checker{}), nil)
	mod.checking = false
	run(m, ui.OpenModal(mod))
	send(m, "Z")
	if !m.maximized {
		t.Error("Z didn't maximize the token modal")
	}
	send(m, "Z")
	if m.maximized {
		t.Error("Z again didn't restore the token modal")
	}

	asking := newAuthModal(config.Default().Keys, "octocat@github.com", core.Access{}, access.Plan{Cmd: []string{"gh", "auth", "refresh"}}, uitest.Token(&uitest.Checker{}), nil)
	asking.checking = false
	run(m, ui.OpenModal(asking))
	send(m, "Z")
	if m.maximized {
		t.Error("Z maximized the token modal while it asks")
	}
}

// Without a modal, Z is none of the app's.
func TestMaximizeNeedsAModal(t *testing.T) {
	m, fakes := newTestApp(t)
	send(m, "Z")
	if m.maximized {
		t.Error("Z maximized with no modal open")
	}
	if !fakes[0].got(isKey("Z")) {
		t.Error("Z didn't reach the focused section")
	}
}

// Help lists the maximize key enabled while a modal is open, and
// disabled otherwise.
func TestMaximizeInHelp(t *testing.T) {
	find := func(m *Model) (key.Binding, bool) {
		for _, l := range m.layersNow() {
			for _, b := range l.Bindings {
				if b.Help().Desc == "maximize" {
					return b, true
				}
			}
		}
		return key.Binding{}, false
	}
	m, _ := newTestApp(t)
	if b, ok := find(m); ok && b.Enabled() {
		t.Error("help lists maximize enabled with no modal open")
	}
	run(m, ui.OpenModal(newCtxModal("history")))
	b, ok := find(m)
	if !ok || !b.Enabled() || b.Help().Key != "Z" {
		t.Errorf("help lacks maximize on Z while a modal is open: %v, %v", b, ok)
	}
	m.modal.(*ctxModal).typing = true
	if b, ok := find(m); ok && b.Enabled() {
		t.Error("help lists maximize enabled while the modal types")
	}
}

// Unbinding maximize leaves Z to the modal.
func TestMaximizeUnbound(t *testing.T) {
	m, _ := newTestApp(t)
	m.keys.Maximize.SetEnabled(false)
	mod := newCtxModal("history")
	run(m, ui.OpenModal(mod))
	send(m, "Z")
	if m.maximized || len(mod.keys()) != 1 {
		t.Errorf("maximized %v, the modal got %v; want Z left to it", m.maximized, mod.keys())
	}
}

// The scroll position of a modal's text survives the toggle, both ways.
func TestMaximizeKeepsTheScroll(t *testing.T) {
	m, _ := newTestApp(t)
	var text strings.Builder
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&text, "line %03d\n", i)
	}
	run(m, m.openText("go.mod", "go.mod", text.String(), false))
	for range 12 {
		send(m, "j")
	}
	top := func() string {
		for l := range strings.SplitSeq(ansi.Strip(m.View().Content), "\n") {
			if i := strings.Index(l, "line "); i >= 0 {
				return l[i : i+8]
			}
		}
		return ""
	}
	before := top()
	if before == "line 001" || before == "" {
		t.Fatalf("the text didn't scroll: top is %q", before)
	}
	for _, want := range []string{"maximized", "restored"} {
		send(m, "Z")
		if got := top(); got != before {
			t.Errorf("%s: the top line is %q, was %q", want, got, before)
		}
	}
}

// The view of a maximized modal.
func TestMaximizeView(t *testing.T) {
	m, _ := newTestApp(t)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	run(m, m.openText("go.mod", "go.mod", "module example.com/x\n\ngo 1.26\n", false))
	send(m, "Z")
	golden.RequireEqual(t, ansi.Strip(m.View().Content))
}
