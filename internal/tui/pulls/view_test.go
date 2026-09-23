package pulls

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	tests := []struct {
		name    string
		section func(t *testing.T) *Section
	}{
		{"list at 80 columns", func(t *testing.T) *Section {
			t.Helper()
			return started(t, newFakeService(), 80, 10)
		}},
		{"list at 120 columns", func(t *testing.T) *Section {
			t.Helper()
			return started(t, newFakeService(), 120, 10)
		}},
		{"no repository", func(t *testing.T) *Section {
			t.Helper()
			s := newTest(t, newFakeService(), 80, 7)
			s.Init()
			return s
		}},
		{"empty filter", func(t *testing.T) *Section {
			t.Helper()
			svc := newFakeService()
			svc.pulls = nil
			return started(t, svc, 80, 4)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			golden.RequireEqual(t, tt.section(t).View())
		})
	}
}
