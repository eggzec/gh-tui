package refs

import (
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The step in the frame of a terminal of 80 by 24 and of 120 by 40, in the
// states a user meets.
func TestView(t *testing.T) {
	sizes := []struct {
		name string
		w, h int
	}{{"80", narrowW, narrowH}, {"120", wideW, wideH}}
	states := []struct {
		name  string
		setup func(f *fake)
		steps func(s *Step, h *host)
	}{
		{"default", nil, nil},
		{"mentions", nil, func(_ *Step, h *host) { h.keys("G", "enter", "j") }},
		{"filtered", nil, func(_ *Step, h *host) { h.keys("&"); h.typed("tok") }},
		{"filtered kept", nil, func(_ *Step, h *host) { h.keys("&"); h.typed("bob"); h.keys("enter") }},
		{"no match", nil, func(_ *Step, h *host) { h.keys("&"); h.typed("zzz") }},
		{"empty", func(f *fake) { f.set(core.References{}) }, nil},
		{"offline", func(f *fake) { r := testRefs(); r.Offline = true; f.set(r) }, nil},
		{"unread", func(f *fake) {
			r := testRefs()
			r.Failed, r.FailedWhy = 3, "Rate limited"
			r.Unresolved, r.CommentsRead, r.CommentsTotal = 12, 100, 340
			f.set(r)
		}, nil},
		{"error", func(f *fake) { f.refsErr = &core.RateLimitError{Reset: testNow.Add(5 * time.Minute)} }, nil},
	}
	for _, sz := range sizes {
		for _, st := range states {
			t.Run(strings.ReplaceAll(st.name, " ", "_")+"_"+sz.name, func(t *testing.T) {
				f := newFake()
				if st.setup != nil {
					f.setup(st.setup)
				}
				s, h := newStep(t, f, sz.w, sz.h, WithVoice(ui.Voice{Loc: time.UTC}))
				if st.steps != nil {
					st.steps(s, h)
				}
				v := s.View()
				assertFits(t, v, sz.w, sz.h)
				golden.RequireEqual(t, v)
			})
		}
		// Before the first links arrive.
		t.Run("loading_"+sz.name, func(t *testing.T) {
			s := New(t.Context(), newFake(), self, true, config.Default().Keys, WithIcons(ui.NewIcons(config.IconsASCII)),
				WithItem(func() Item { return Item{Title: "Cache GraphQL reads on disk"} }))
			s.SetTheme(testTheme())
			s.SetSize(sz.w, sz.h)
			v := s.View()
			assertFits(t, v, sz.w, sz.h)
			golden.RequireEqual(t, v)
		})
	}
}

// setup is the fake's own, so a state of the view is its data alone.
func (f *fake) setup(edit func(*fake)) { edit(f) }

// With the ASCII icons the step draws ASCII alone, in every state, and
// nothing wraps.
func TestViewASCII(t *testing.T) {
	for _, steps := range [][]string{nil, {"G", "enter"}, {"&"}, {"*"}} {
		s, h := newStep(t, newFake(), narrowW, narrowH)
		h.keys(steps...)
		v := ansi.Strip(s.View())
		if strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("after %v: view isn't ASCII:\n%s", steps, v)
		}
	}
}

// The view is exactly its size, whatever the size, in every state.
func TestViewFitsAnySize(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {10, 3}, {30, 5}, {60, 18}, {250, 60}} {
		for _, steps := range [][]string{nil, {"G", "enter"}, {"&"}, {"&", "t", "enter"}, {"h", "h"}} {
			s, h := newStep(t, newFake(), wideW, wideH)
			h.keys(steps...)
			s.SetSize(size[0], size[1])
			assertFits(t, s.View(), size[0], size[1])
		}
	}
}

// The rows keep the room for the title, and give up the state first: a
// title that fits the row is whole, and the state shows when it fits too.
func TestViewKeepsTitles(t *testing.T) {
	s, _ := newStep(t, newFake(), narrowW, narrowH)
	has(t, s, "#204 Crash when the token lacks read:org")
	lacks(t, s, "closed as completed")
	s, _ = newStep(t, newFake(), wideW, wideH)
	has(t, s, "#204 Crash when the token lacks read:org", "closed as completed - closes")
}

// The text of the first line is cut to the width, with the filter at its
// right end while it is in force.
func TestViewFirstLine(t *testing.T) {
	s, h := newStep(t, newFake(), narrowW, narrowH, WithItem(func() Item {
		return Item{Title: "A title that is far longer than the sixty columns this step has to show it in"}
	}))
	if got := lines(s)[0]; !strings.HasPrefix(got, "#231 A title that is far longer") || !strings.Contains(got, "...") {
		t.Errorf("first line = %q", got)
	}
	h.keys("&")
	h.typed("tok")
	h.keys("enter")
	if got := lines(s)[0]; !strings.HasSuffix(got, "& tok") {
		t.Errorf("first line = %q, want the filter at its end", got)
	}
}
