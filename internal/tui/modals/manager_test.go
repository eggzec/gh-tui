package modals_test

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/modals"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// fake is a modal that records what the manager calls on it. fakeA and fakeB
// are two types of it.
type fake struct {
	title    string
	hides    int
	discards int
	themes   int
	sizes    [][2]int
	narrow   []bool
	fit      [2]int
}

func (f *fake) Title() string                    { return f.title }
func (f *fake) Update(tea.Msg) tea.Cmd           { return nil }
func (f *fake) View() string                     { return "" }
func (f *fake) SetSize(w, h int)                 { f.sizes = append(f.sizes, [2]int{w, h}) }
func (f *fake) SetTheme(ui.Theme)                { f.themes++ }
func (f *fake) KeyLayers() []keyhelp.Layer       { return nil }
func (f *fake) Commands() []string               { return nil }
func (f *fake) Hide()                            { f.hides++ }
func (f *fake) Discard()                         { f.discards++ }
func (f *fake) SetNarrow(narrow bool)            { f.narrow = append(f.narrow, narrow) }
func (f *fake) Fit(w, h int) (width, height int) { return min(w, f.fit[0]), min(h, f.fit[1]) }
func (f *fake) lastSize() [2]int                 { return f.sizes[len(f.sizes)-1] }

type fakeA struct{ fake }

type fakeB struct{ fake }

func newA(title string) *fakeA { return &fakeA{fake{title: title, fit: [2]int{1000, 1000}}} }
func newB(title string) *fakeB { return &fakeB{fake{title: title, fit: [2]int{1000, 1000}}} }

func titles(s *modals.Manager) []string {
	var out []string
	for m := range s.Held() {
		out = append(out, m.Title())
	}
	return out
}

// msgs flattens a command's messages.
func msgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch m := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range m {
			out = append(out, msgs(c)...)
		}
		return out
	default:
		return []tea.Msg{m}
	}
}

func reopened(cmd tea.Cmd) []ui.Modal {
	var out []ui.Modal
	for _, m := range msgs(cmd) {
		if r, ok := m.(ui.ReopenedMsg); ok {
			out = append(out, r.Modal)
		}
	}
	return out
}

type spec struct {
	kind  string
	title string
	prio  modals.Priority
}

func (sp spec) modal() ui.Modal {
	if sp.kind == "b" {
		return newB(sp.title)
	}
	return newA(sp.title)
}

func build(t *testing.T, held ...spec) *modals.Manager {
	t.Helper()
	s := modals.New(10)
	for _, h := range held {
		if ch := s.Add(h.modal(), h.prio); !ch.Opened {
			t.Fatalf("setup open of %v refused: %v", h, ch.Refused)
		}
	}
	return s
}

func TestAddRules(t *testing.T) {
	t.Parallel()
	const anywhere = modals.PriorityAnywhere
	tests := []struct {
		name    string
		held    []spec
		req     spec
		refused modals.Refusal
		top     string
		after   []string // titles, top first
	}{
		{"higher over", []spec{{"a", "x", 20}}, spec{"b", "y", 30}, modals.NotRefused, "y", []string{"y", "x"}},
		{"equal replaces", []spec{{"a", "x", 20}}, spec{"b", "y", 20}, modals.NotRefused, "y", []string{"y", "x"}},
		{"same type another title replaces", []spec{{"a", "x", 20}}, spec{"a", "y", 20}, modals.NotRefused, "y", []string{"y", "x"}},
		{"lower refused", []spec{{"a", "x", 30}}, spec{"b", "y", 20}, modals.Lower, "x", []string{"x"}},
		{"same refused even when higher", []spec{{"a", "x", 20}}, spec{"a", "x", 30}, modals.Same, "x", []string{"x"}},
		{"same below the top is not same", []spec{{"a", "x", 20}, {"b", "y", 30}}, spec{"a", "x", 40}, modals.NotRefused, "x", []string{"x", "y", "x"}},
		{"negative over 60", []spec{{"a", "x", 60}}, spec{"b", "s", anywhere}, modals.NotRefused, "s", []string{"s", "x"}},
		{"negative over negative of another type", []spec{{"a", "s", anywhere}}, spec{"b", "n", anywhere}, modals.NotRefused, "n", []string{"n", "s"}},
		{"20 over a negative", []spec{{"a", "s", anywhere}}, spec{"b", "p", 20}, modals.NotRefused, "p", []string{"p", "s"}},
		{"older negative of the same type goes", []spec{{"a", "s", anywhere}, {"b", "p", 20}}, spec{"a", "t", anywhere}, modals.NotRefused, "t", []string{"t", "p"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := build(t, tt.held...)
			before := s.Top()
			req := tt.req.modal()
			ch := s.Add(req, tt.req.prio)
			if ch.Refused != tt.refused || ch.Opened != (tt.refused == modals.NotRefused) {
				t.Fatalf("change = %+v, want refusal %v", ch, tt.refused)
			}
			if got := s.Top().Title(); got != tt.top {
				t.Errorf("top = %q, want %q", got, tt.top)
			}
			if got := titles(s); !slices.Equal(got, tt.after) {
				t.Errorf("held = %v, want %v", got, tt.after)
			}
			if tt.refused != modals.NotRefused {
				if ch.Blocker != before {
					t.Errorf("blocker = %v, want the top", ch.Blocker)
				}
				if f := fakeOf(req); f.discards+f.hides+f.themes+len(f.sizes) != 0 {
					t.Errorf("a refused modal was called: %+v", f)
				}
			}
		})
	}
}

func fakeOf(m ui.Modal) *fake {
	switch v := m.(type) {
	case *fakeA:
		return &v.fake
	case *fakeB:
		return &v.fake
	}
	panic("unknown fake")
}

func TestStale(t *testing.T) {
	t.Parallel()
	x, y := newA("x"), newB("y")
	s := modals.New(10)
	s.Add(x, modals.PriorityContent)
	s.Add(y, modals.PriorityDetail)

	z := newA("z")
	if ch := s.Add(z, modals.PriorityShape, modals.WithOver(x)); ch.Refused != modals.Stale || ch.Opened || s.Top() != ui.Modal(y) {
		t.Errorf("stale over = %+v, top %v", ch, s.Top())
	}
	// Stale is checked before Same.
	if ch := s.Add(newB("y"), modals.PriorityShape, modals.WithOver(x)); ch.Refused != modals.Stale {
		t.Errorf("stale before same: %v", ch.Refused)
	}
	// WithOver is no bypass of the rank rule.
	if ch := s.Add(newA("w"), modals.PriorityContent, modals.WithOver(y)); ch.Refused != modals.Lower {
		t.Errorf("over a higher top = %v, want Lower", ch.Refused)
	}
	if ch := s.Add(newA("v"), modals.PriorityShape, modals.WithOver(y)); !ch.Opened {
		t.Errorf("over the top refused: %v", ch.Refused)
	}
	if f := fakeOf(z); f.discards != 0 {
		t.Error("a refused open discarded the new modal")
	}
}

func TestEqualReplaceBackReturns(t *testing.T) {
	t.Parallel()
	x, y := newA("x"), newB("y")
	s := modals.New(10)
	s.Add(x, modals.PriorityContent)
	s.Add(y, modals.PriorityContent)
	if !s.CanBack() {
		t.Fatal("cannot go back to the replaced modal")
	}
	ch := s.Back(y)
	if !ch.Closed || s.Top() != ui.Modal(x) || y.discards != 1 {
		t.Errorf("back: %+v top %v discards %d", ch, s.Top(), y.discards)
	}
	if got := reopened(ch.Cmd); len(got) != 1 || got[0] != ui.Modal(x) {
		t.Errorf("reopened = %v, want x once", got)
	}
}

func TestNegativeStacksAndPopsAlone(t *testing.T) {
	t.Parallel()
	s1, n, p := newA("s"), newB("n"), newA("p")
	s := modals.New(10)
	s.Add(p, modals.PriorityContent)
	s.Add(s1, modals.PriorityAnywhere)
	s.Add(n, modals.PriorityAnywhere)

	ch := s.Close(n)
	if s.Top() != ui.Modal(s1) || !ch.Closed || n.discards != 1 || s1.discards != 0 {
		t.Fatalf("esc on a negative: top %v, %+v", s.Top(), ch)
	}
	if got := titles(s); !slices.Equal(got, []string{"s", "p"}) {
		t.Errorf("held = %v", got)
	}
	s.Close(s1)
	if s.Top() != ui.Modal(p) {
		t.Errorf("top = %v, want p", s.Top())
	}
}

func TestRunStopsAtNegative(t *testing.T) {
	t.Parallel()
	neg, c1, c2 := newA("s"), newB("c1"), newA("c2")
	s := modals.New(10)
	s.Add(neg, modals.PriorityAnywhere)
	s.Add(c1, modals.PriorityContent)
	s.Add(c2, modals.PriorityContent)
	ch := s.Close(c2)
	if s.Top() != ui.Modal(neg) || c1.discards != 1 || c2.discards != 1 || neg.discards != 0 {
		t.Errorf("top %v, discards %d %d %d", s.Top(), c1.discards, c2.discards, neg.discards)
	}
	if got := reopened(ch.Cmd); len(got) != 1 || got[0] != ui.Modal(neg) {
		t.Errorf("reopened = %v", got)
	}
}

func TestEscRunAndBackspace(t *testing.T) {
	t.Parallel()
	under, a, b := newA("u"), newB("a"), newA("b")
	s := modals.New(10)
	s.Add(under, modals.PriorityList)
	s.Add(a, modals.PriorityContent)
	s.Add(b, modals.PriorityContent)

	ch := s.Close(b)
	if s.Top() != ui.Modal(under) || a.discards != 1 || b.discards != 1 {
		t.Errorf("esc run: top %v", s.Top())
	}
	if got := reopened(ch.Cmd); len(got) != 1 || got[0] != ui.Modal(under) {
		t.Errorf("reopened = %v", got)
	}

	a2, b2 := newB("a2"), newA("b2")
	s.Add(a2, modals.PriorityContent)
	s.Add(b2, modals.PriorityContent)
	s.Back(b2)
	if s.Top() != ui.Modal(a2) || a2.discards != 0 || b2.discards != 1 {
		t.Errorf("back popped more than one: top %v", s.Top())
	}
}

func TestGuards(t *testing.T) {
	t.Parallel()
	var asked []string
	guard := func(name string, ans modals.Closing) modals.Guard {
		return func(r modals.Reason) modals.Closing {
			asked = append(asked, fmt.Sprintf("%s:%d", name, r))
			return ans
		}
	}
	mark := func(name string) tea.Cmd { return func() tea.Msg { return name } }

	t.Run("own close skips own guard, run asks the others with dismiss", func(t *testing.T) {
		asked = nil
		s := modals.New(10)
		s.Add(newA("a"), modals.PriorityContent, modals.WithGuard(guard("a", modals.Close())))
		b := newB("b")
		s.Add(b, modals.PriorityContent, modals.WithGuard(guard("b", modals.Close())))
		ch := s.Close(b)
		if !slices.Equal(asked, []string{"a:0"}) || ch.Refused != modals.NotRefused || s.Top() != nil {
			t.Errorf("asked %v, change %+v", asked, ch)
		}
	})
	t.Run("keep stops the run", func(t *testing.T) {
		asked = nil
		s := modals.New(10)
		keeper, mid, own := newA("k"), newB("m"), newA("o")
		keep := modals.Closing{Keep: true, Toast: "unsaved", Cmd: mark("keep")}
		s.Add(keeper, modals.PriorityContent, modals.WithGuard(guard("k", keep)))
		s.Add(mid, modals.PriorityContent, modals.WithGuard(guard("m", modals.CloseAfter(mark("save")))))
		s.Add(own, modals.PriorityContent)
		ch := s.Close(own)
		if ch.Refused != modals.Kept || ch.Toast != "unsaved" || ch.Blocker != ui.Modal(keeper) || !ch.Closed || s.Top() != ui.Modal(keeper) {
			t.Fatalf("change %+v top %v", ch, s.Top())
		}
		if mid.discards != 1 || own.discards != 1 || keeper.discards != 0 {
			t.Error("wrong modals discarded")
		}
		got := msgs(ch.Cmd)
		if len(got) != 3 || got[0] != "save" || got[1] != "keep" {
			t.Errorf("cmd msgs = %v", got)
		}
		if r := reopened(ch.Cmd); len(r) != 1 || r[0] != ui.Modal(keeper) {
			t.Errorf("reopened = %v", r)
		}
	})
	t.Run("keep with a confirm-opening cmd", func(t *testing.T) {
		s := modals.New(10)
		k, own := newA("k"), newB("o")
		confirm := ui.OpenModal(newA("confirm"))
		s.Add(k, modals.PriorityContent, modals.WithGuard(guard("k", modals.Closing{Keep: true, Cmd: confirm})))
		s.Add(own, modals.PriorityContent)
		ch := s.Close(own)
		var opens int
		for _, m := range msgs(ch.Cmd) {
			if _, ok := m.(ui.OpenModalMsg); ok {
				opens++
			}
		}
		if opens != 1 || ch.Refused != modals.Kept {
			t.Errorf("opens %d, refused %v", opens, ch.Refused)
		}
	})
	t.Run("quit refused", func(t *testing.T) {
		asked = nil
		s := modals.New(10)
		a, b := newA("a"), newB("b")
		s.Add(a, modals.PriorityContent, modals.WithGuard(guard("a", modals.Refuse("draft"))))
		s.Add(b, modals.PriorityShape, modals.WithGuard(guard("b", modals.CloseAfter(mark("saved")))))
		ch := s.Quit()
		if ch.Refused != modals.Kept || ch.Toast != "draft" || ch.Blocker != ui.Modal(a) || ch.Closed {
			t.Errorf("quit %+v", ch)
		}
		if !slices.Equal(asked, []string{"b:1", "a:1"}) || s.Top() != ui.Modal(b) || len(titles(s)) != 2 {
			t.Errorf("asked %v", asked)
		}
		if got := msgs(ch.Cmd); len(got) != 1 || got[0] != "saved" {
			t.Errorf("cmd msgs = %v", got)
		}
	})
	t.Run("quit allowed", func(t *testing.T) {
		s := modals.New(10)
		s.Add(newA("a"), modals.PriorityContent, modals.WithGuard(guard("a", modals.Close())))
		if ch := s.Quit(); ch.Refused != modals.NotRefused || s.Top() == nil {
			t.Errorf("quit %+v", ch)
		}
	})
	t.Run("eviction asks dismiss and ignores keep", func(t *testing.T) {
		asked = nil
		s := modals.New(10)
		first := newA("0")
		s.Add(first, modals.PriorityContent, modals.WithGuard(guard("0", modals.Closing{Keep: true, Cmd: mark("evicted")})))
		for i := 1; i <= 10; i++ {
			if ch := s.Add(newA(strconv.Itoa(i)), modals.PriorityContent); ch.Opened && i < 10 && first.discards != 0 {
				t.Fatalf("evicted early at %d", i)
			}
		}
		if first.discards != 0 || len(asked) != 0 {
			t.Fatalf("evicted at the cap: asked %v", asked)
		}
		ch := s.Add(newA("11"), modals.PriorityContent)
		if !slices.Equal(asked, []string{"0:0"}) || first.discards != 1 || len(titles(s)) != 11 || !ch.Opened || ch.Refused != modals.NotRefused {
			t.Errorf("asked %v, discards %d, held %d, change %+v", asked, first.discards, len(titles(s)), ch)
		}
		if got := msgs(ch.Cmd); len(got) != 1 || got[0] != "evicted" {
			t.Errorf("cmd msgs = %v", got)
		}
	})
}

func TestHideDiscardAndNonTop(t *testing.T) {
	t.Parallel()
	x, y := newA("x"), newB("y")
	s := modals.New(10)
	s.Add(x, modals.PriorityContent)
	s.Add(y, modals.PriorityShape)
	if x.hides != 1 || y.hides != 0 {
		t.Errorf("hides x %d y %d", x.hides, y.hides)
	}
	for _, ch := range []modals.Change{s.Close(x), s.Back(x)} {
		if ch.Opened || ch.Closed || ch.Refused != modals.NotRefused || ch.Cmd != nil || ch.Blocker != nil {
			t.Errorf("close of a non-top = %+v", ch)
		}
	}
	if x.discards+y.discards != 0 || len(titles(s)) != 2 {
		t.Error("a non-top close changed the trail")
	}
	s.Close(y)
	if y.discards != 1 || x.discards != 0 {
		t.Errorf("discards x %d y %d", x.discards, y.discards)
	}
}

func TestOptionalInterfacesAreOptional(t *testing.T) {
	t.Parallel()
	// A modal without Hider or Discarder is neither hidden nor discarded.
	s := modals.New(10)
	type bare struct{ ui.Modal }
	b := &bare{newA("b")}
	s.Add(b, modals.PriorityContent)
	s.Add(newB("o"), modals.PriorityShape)
	s.Close(s.Top())
	s.Close(b)
	if s.Top() != nil {
		t.Error("trail not empty")
	}
}

func TestMaximizedKeptPerEntry(t *testing.T) {
	t.Parallel()
	x, y := newA("x"), newB("y")
	s := modals.New(10)
	s.Resize(modals.Room{Width: 100, Height: 40, Footer: 1})
	s.Add(x, modals.PriorityContent, modals.WithMaximized(true))
	if !s.Maximized() {
		t.Fatal("not maximized at the start")
	}
	if w, h := s.FrameSize(); w != 100 || h != 39 {
		t.Errorf("maximized frame = %d x %d", w, h)
	}
	s.Add(y, modals.PriorityShape)
	if s.Maximized() {
		t.Error("the new top inherited maximize")
	}
	if !s.ToggleMaximized() || !s.Maximized() {
		t.Error("toggle did not maximize y")
	}
	s.Close(y)
	if !s.Maximized() || x.lastSize() != [2]int{96, 37} {
		t.Errorf("x after return: maximized %v, size %v", s.Maximized(), x.lastSize())
	}
}

func TestResizeRefitAndNarrow(t *testing.T) {
	t.Parallel()
	x := newA("x")
	x.fit = [2]int{20, 5}
	s := modals.New(10)
	s.Add(x, modals.PriorityContent)
	if len(x.sizes) != 0 {
		t.Fatal("sized before there was a room")
	}
	if !s.Resize(modals.Room{Width: 100, Height: 40, Footer: 1, Narrow: true}) {
		t.Error("first resize reported no change")
	}
	if x.lastSize() != [2]int{20, 5} || !slices.Equal(x.narrow, []bool{true}) {
		t.Errorf("size %v narrow %v", x.lastSize(), x.narrow)
	}
	if w, h := s.FrameSize(); w != 24 || h != 7 {
		t.Errorf("frame = %d x %d", w, h)
	}
	if s.Resize(modals.Room{Width: 100, Height: 40, Footer: 1, Narrow: true}) {
		t.Error("same room reported a change")
	}
	if s.Refit() {
		t.Error("refit with the same ask reported a change")
	}
	x.fit = [2]int{30, 9}
	if !s.Refit() || x.lastSize() != [2]int{30, 9} {
		t.Errorf("refit: size %v", x.lastSize())
	}
	// Without a Fitter, Refit does nothing.
	s.Add(newB("y"), modals.PriorityShape)
	type bare struct{ ui.Modal }
	s.Add(&bare{newA("z")}, modals.PriorityForm)
	if s.Refit() {
		t.Error("refit of a modal that is no Fitter")
	}
}

func TestThemeAndReturn(t *testing.T) {
	t.Parallel()
	x, y := newA("x"), newB("y")
	s := modals.New(10)
	s.Add(x, modals.PriorityContent)
	s.SetTheme(ui.Theme{})
	if x.themes != 1 {
		t.Errorf("top themes = %d", x.themes)
	}
	s.Add(y, modals.PriorityShape)
	if y.themes != 1 || x.themes != 1 {
		t.Errorf("themes x %d y %d", x.themes, y.themes)
	}
	s.SetTheme(ui.Theme{})
	if x.themes != 1 || y.themes != 2 {
		t.Errorf("only the top is themed at once: x %d y %d", x.themes, y.themes)
	}
	s.Close(y)
	if x.themes != 2 {
		t.Errorf("theme on return: x %d", x.themes)
	}
}

func TestReopenedOnceOnReturn(t *testing.T) {
	t.Parallel()
	x, y := newA("x"), newB("y")
	s := modals.New(10)
	if got := reopened(s.Add(x, modals.PriorityContent).Cmd); len(got) != 0 {
		t.Errorf("an open sent reopened: %v", got)
	}
	s.Add(y, modals.PriorityShape)
	if got := reopened(s.Close(y).Cmd); len(got) != 1 || got[0] != ui.Modal(x) {
		t.Errorf("reopened = %v", got)
	}
	if ch := s.Close(x); len(reopened(ch.Cmd)) != 0 {
		t.Error("closing the last modal sent reopened")
	}
}

func TestAnywhereKeepsOneCopyAndDiscardsTheOlder(t *testing.T) {
	t.Parallel()
	old, p := newA("old"), newB("p")
	s := modals.New(10)
	s.Add(old, modals.PriorityAnywhere)
	s.Add(p, modals.PriorityContent)
	s.Add(newA("new"), modals.PriorityAnywhere)
	if old.discards != 1 {
		t.Errorf("older copy discards = %d, want 1", old.discards)
	}
	if got := titles(s); !slices.Equal(got, []string{"new", "p"}) {
		t.Errorf("held = %v", got)
	}
}

func TestSameAgainstAnywhereTop(t *testing.T) {
	t.Parallel()
	s := modals.New(10)
	top := newA("s")
	s.Add(top, modals.PriorityAnywhere)
	dup := newA("s")
	ch := s.Add(dup, modals.PriorityAnywhere)
	if ch.Refused != modals.Same || ch.Blocker != ui.Modal(top) || s.Top() != ui.Modal(top) || top.discards != 0 || dup.discards != 0 {
		t.Errorf("change %+v", ch)
	}
}

func TestCloseAnywhereLeavesEquals(t *testing.T) {
	t.Parallel()
	c1, c2, neg := newA("c1"), newB("c2"), newA("s")
	s := modals.New(10)
	s.Add(c1, modals.PriorityContent)
	s.Add(c2, modals.PriorityContent)
	s.Add(neg, modals.PriorityAnywhere)
	ch := s.Close(neg)
	if !ch.Closed || s.Top() != ui.Modal(c2) || c1.discards+c2.discards != 0 || neg.discards != 1 {
		t.Errorf("change %+v, held %v", ch, titles(s))
	}
}

func TestRankChecksHighestBeneathAnywhere(t *testing.T) {
	t.Parallel()
	s := modals.New(10)
	s.Add(newA("prompt"), 60)
	srch := newB("s")
	s.Add(srch, modals.PriorityAnywhere)
	l := newA("l")
	ch := s.Add(l, 10)
	if ch.Refused != modals.Lower || ch.Blocker != ui.Modal(srch) || s.Top() != ui.Modal(srch) {
		t.Errorf("change %+v", ch)
	}
	if f := fakeOf(l); f.discards+f.hides+f.themes != 0 {
		t.Errorf("a refused modal was called: %+v", f)
	}
	if ch := s.Add(newA("higher"), 70); !ch.Opened {
		t.Errorf("a higher modal was refused: %v", ch.Refused)
	}
}

func TestAnywhereHasNoGuard(t *testing.T) {
	t.Parallel()
	var asked int
	g := func(modals.Reason) modals.Closing { asked++; return modals.Refuse("no") }
	s := modals.New(0)
	s.Add(newA("p"), modals.PriorityContent)
	s.Add(newB("s"), modals.PriorityAnywhere, modals.WithGuard(g))
	if ch := s.Quit(); ch.Refused != modals.NotRefused || ch.Cmd != nil {
		t.Errorf("quit %+v", ch)
	}
	s.Add(newA("p2"), modals.PriorityContent)
	if asked != 0 {
		t.Errorf("an anywhere guard was asked %d times", asked)
	}
}

func TestAnywhereNotCountedForEviction(t *testing.T) {
	t.Parallel()
	t.Run("a burst keeps the content", func(t *testing.T) {
		s := modals.New(1)
		c1, c2, c3 := newA("c1"), newB("c2"), newA("c3")
		s.Add(c1, modals.PriorityContent)
		s.Add(newA("s"), modals.PriorityAnywhere)
		s.Add(newB("n"), modals.PriorityAnywhere)
		s.Add(c2, modals.PriorityContent)
		if c1.discards != 0 {
			t.Fatal("content evicted by anywhere modals")
		}
		s.Add(c3, modals.PriorityContent)
		if c1.discards != 1 || !slices.Equal(titles(s), []string{"c3", "c2", "n", "s"}) {
			t.Errorf("held %v, c1 discards %d", titles(s), c1.discards)
		}
	})
	t.Run("the oldest is anywhere", func(t *testing.T) {
		s := modals.New(1)
		neg, c1 := newA("s"), newB("c1")
		s.Add(neg, modals.PriorityAnywhere)
		s.Add(c1, modals.PriorityContent)
		s.Add(newA("c2"), modals.PriorityContent)
		s.Add(newB("c3"), modals.PriorityContent)
		if neg.discards != 0 || c1.discards != 1 || !slices.Equal(titles(s), []string{"c3", "c2", "s"}) {
			t.Errorf("held %v, discards neg %d c1 %d", titles(s), neg.discards, c1.discards)
		}
	})
}

func TestBackAsksGuard(t *testing.T) {
	t.Parallel()
	mark := func(name string) tea.Cmd { return func() tea.Msg { return name } }
	t.Run("keep", func(t *testing.T) {
		var asked []modals.Reason
		under, top := newA("u"), newB("t")
		s := modals.New(10)
		s.Add(under, modals.PriorityContent)
		s.Add(top, modals.PriorityContent, modals.WithGuard(func(r modals.Reason) modals.Closing {
			asked = append(asked, r)
			return modals.Closing{Keep: true, Toast: "draft", Cmd: mark("kept")}
		}))
		ch := s.Back(top)
		if ch.Refused != modals.Kept || ch.Toast != "draft" || ch.Blocker != ui.Modal(top) || ch.Closed {
			t.Errorf("change %+v", ch)
		}
		if s.Top() != ui.Modal(top) || top.discards != 0 || !slices.Equal(asked, []modals.Reason{modals.Dismiss}) {
			t.Errorf("top %v, asked %v", s.Top(), asked)
		}
		if got := msgs(ch.Cmd); len(got) != 1 || got[0] != "kept" {
			t.Errorf("cmd msgs = %v", got)
		}
	})
	t.Run("close", func(t *testing.T) {
		under, top := newA("u"), newB("t")
		s := modals.New(10)
		s.Add(under, modals.PriorityContent)
		s.Add(top, modals.PriorityContent, modals.WithGuard(func(modals.Reason) modals.Closing {
			return modals.CloseAfter(mark("saved"))
		}))
		ch := s.Back(top)
		if !ch.Closed || ch.Refused != modals.NotRefused || s.Top() != ui.Modal(under) || top.discards != 1 {
			t.Errorf("change %+v", ch)
		}
		if got := msgs(ch.Cmd); len(got) != 2 || got[0] != "saved" {
			t.Errorf("cmd msgs = %v", got)
		}
	})
}

func TestQuitWithoutGuards(t *testing.T) {
	t.Parallel()
	s := build(t, spec{"a", "x", 20})
	if ch := s.Quit(); ch.Cmd != nil || ch.Refused != modals.NotRefused {
		t.Errorf("quit %+v", ch)
	}
}
