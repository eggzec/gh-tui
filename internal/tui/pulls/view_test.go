package pulls

import (
	"strconv"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	tests := []struct {
		name string
		view func(t *testing.T) string
	}{
		{"list at 80 columns", list(80)},
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
		{"modal at 80 columns", func(t *testing.T) string {
			t.Helper()
			s := started(t, newFakeService(), 80, 30)
			press(t, s, "enter")
			return s.modal().View()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			golden.RequireEqual(t, tt.view(t))
		})
	}
}

func TestDetailHeader(t *testing.T) {
	for _, width := range []int{80, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			s := started(t, newFakeService(), width, 30)
			press(t, s, "enter")
			golden.RequireEqual(t, s.modal().detailHeader(width))
		})
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
