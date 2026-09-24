package files

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		section       func(t *testing.T, width, height int) *Section
	}{
		{"no repo at 30 columns", 30, 8, func(t *testing.T, w, h int) *Section {
			t.Helper()
			return newSection(t, sampleFake(), w, h)
		}},
		{"no repo at 80 columns", 80, 6, func(t *testing.T, w, h int) *Section {
			t.Helper()
			return newSection(t, sampleFake(), w, h)
		}},
		{"loaded", 30, 10, func(t *testing.T, w, h int) *Section {
			t.Helper()
			return loaded(t, sampleFake(), w, h)
		}},
		{"nested", 30, 10, func(t *testing.T, w, h int) *Section {
			t.Helper()
			s := loaded(t, sampleFake(), w, h)
			keys(s, "+", "down", "+", "down")
			return s
		}},
		{"blurred", 30, 4, func(t *testing.T, w, h int) *Section {
			t.Helper()
			s := loaded(t, sampleFake(), w, h)
			s.Blur()
			return s
		}},
		{"empty repo", 30, 3, func(t *testing.T, w, h int) *Section {
			t.Helper()
			f := newFake()
			f.addTree(ghTUI, "")
			return loaded(t, f, w, h)
		}},
		{"error", 40, 3, func(t *testing.T, w, h int) *Section {
			t.Helper()
			f := newFake()
			return loaded(t, f, w, h)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.section(t, tt.width, tt.height).View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

func TestViewPreview(t *testing.T) {
	tests := []struct {
		name string
		row  int
	}{
		{"text", rowAgents},
		{"too large", rowReadme},
		{"symlink", rowLink},
		{"error", rowGitignore},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := openRow(t, sampleFake(), tt.row)
			v := h.top().View()
			assertFits(t, v, h.width, h.height)
			golden.RequireEqual(t, v)
		})
	}
}
