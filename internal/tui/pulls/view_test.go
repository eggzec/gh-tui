package pulls

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestView(t *testing.T) {
	tests := []struct {
		name string
		view func(t *testing.T) string
	}{
		{"list at 40 columns", list(40)},
		{"list at 60 columns", list(60)},
		{"filtered at 40 columns", filtered(40)},
		{"filtered at 60 columns", filtered(60)},
		{"filtered at 100 columns", filtered(100)},
		{"filter modal at 80 columns", filterModal(60, 18)},
		{"filter modal at 140 columns", filterModal(108, 30)},
		{"list at 80 columns", list(80)},
		{"list at 100 columns", list(100)},
		{"list at 120 columns", list(120)},
		{"no repository", func(t *testing.T) string {
			t.Helper()
			s := newTest(t, newFakeService(), 80, 7)
			s.Init()
			return s.View()
		}},
		{"empty filter", func(t *testing.T) string {
			t.Helper()
			svc := newFakeService()
			svc.pulls = nil
			return started(t, svc, 80, 4).View()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			golden.RequireEqual(t, tt.view(t))
		})
	}
}

// The header shows the total GitHub told with the loaded count in brackets,
// matched over total under a filter, and drops the count where the tabs
// leave it no room.
func TestHeaderCount(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		filter string
		want   string // "" when no count shows
	}{
		{"wide", 120, "", "250 ["},
		{"wide filtered", 120, "is:merged author:hubot", "/250"},
		{"narrow", 30, "", ""},
		{"narrow filtered", 30, "is:merged author:hubot", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := started(t, newFakeService(), tt.width, 6)
			s.total.Store(250)
			if tt.filter != "" {
				apply(t, s, tt.filter)
				s.total.Store(250)
			}
			s.renderHeader()
			got := ansi.Strip(s.header)
			if tt.want == "" {
				if strings.Contains(got, "250") {
					t.Errorf("header %q shows the count, want none", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("header %q lacks %q", got, tt.want)
			}
		})
	}
}

func TestDetailHeader(t *testing.T) {
	for _, width := range []int{80, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			s := started(t, newFakeService(), width, 30)
			press(t, s, "enter")
			s.modal().SetSize(width, 30)
			golden.RequireEqual(t, s.modal().header())
		})
	}
}

// filtered returns the view of the list at width on the merged tab,
// filtered by author.
func filtered(width int) func(t *testing.T) string {
	return func(t *testing.T) string {
		t.Helper()
		s := started(t, newFakeService(), width, 6)
		apply(t, s, "is:merged author:hubot")
		return s.View()
	}
}

// filterModal returns the view of the filter modal of the list filtered by
// author and label, in the room the app gives it inside a frame of at most
// width by height.
func filterModal(width, height int) func(t *testing.T) string {
	return func(t *testing.T) string {
		t.Helper()
		s := started(t, newFakeService(), 80, 20, WithFacets(fakeFacets{}))
		apply(t, s, "is:open author:@me label:cache -is:draft sort:created-desc crash")
		f, _ := s.Filter()
		m := ui.NewFilterModal(t.Context(), s.Title(), s.Section, f)
		m.SetTheme(s.theme)
		m.SetSize(m.Fit(width, height))
		return m.View()
	}
}

// list returns the view of the list at width with the second row selected.
func list(width int) func(t *testing.T) string {
	return func(t *testing.T) string {
		t.Helper()
		s := started(t, newFakeService(), width, 10)
		press(t, s, "down")
		return s.View()
	}
}

func TestConfirmView(t *testing.T) {
	for _, width := range []int{80, 120} {
		w := strconv.Itoa(width)
		t.Run("merge dialog at "+w+" columns", func(t *testing.T) {
			s := started(t, newFakeService(), width, 30)
			press(t, s, "M")
			d := dialogOf(s)
			if d == nil {
				t.Fatal("merge opened no dialog")
			}
			// Sized as the app sizes it, inside the frame over the screen.
			d.SetSize(d.Fit(width-2*max(width/10, 2)-4, 30))
			golden.RequireEqual(t, d.View())
		})
		t.Run("close from the list at "+w+" columns", func(t *testing.T) {
			s := started(t, newFakeService(), width, 30)
			press(t, s, "X")
			m, ok := s.modals[len(s.modals)-1].(*ui.ConfirmModal)
			if !ok {
				t.Fatal("close opened no question")
			}
			// Sized as the app sizes it, inside the frame over the screen.
			m.SetSize(m.Fit(width-2*max(width/10, 2)-4, 30))
			golden.RequireEqual(t, m.View())
		})
	}
}
