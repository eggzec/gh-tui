package checks

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		keys          []string
	}{
		{"190 columns", wideW, wideH, nil},
		{"190 columns job", wideW, wideH, []string{"enter"}},
		{"190 columns running", wideW, wideH, []string{"down", "down", "down", "enter"}},
		{"190 columns detail", wideW, wideH, []string{"down", "down", "enter"}},
		{"190 columns confirm", wideW, wideH, []string{"enter", "R"}},
		{"80 columns", narrowW, narrowH, nil},
		{"80 columns job", narrowW, narrowH, []string{"enter"}},
		{"80 columns annotations", narrowW, narrowH, []string{"enter", "A"}},
		{"80 columns detail", narrowW, narrowH, []string{"down", "down", "enter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, h := newStep(t, newFake(), tt.width, tt.height)
			h.keys(tt.keys...)
			v := s.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

// With the ASCII icons the checks draw ASCII alone: the list and its
// crumbs, a job and its annotations. A check's detail is left out, since
// it shows the markdown the check wrote.
func TestViewASCII(t *testing.T) {
	for _, keys := range [][]string{nil, {"enter"}, {"enter", "A"}} {
		s, h := newStep(t, newFake(), narrowW, narrowH, WithIcons(ui.NewIcons(config.IconsASCII)))
		h.keys(keys...)
		if v := ansi.Strip(s.View()); strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("after %v: view isn't ASCII:\n%s", keys, v)
		}
	}
}

// The checks and the job that failed to load say why, in both themes.
func TestViewFailed(t *testing.T) {
	for _, tt := range []struct {
		name string
		keys []string
		dark bool
	}{
		{"checks light", nil, false},
		{"checks dark", nil, true},
		{"job light", []string{"enter"}, false},
		{"job dark", []string{"enter"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			if tt.keys == nil {
				f.checksErr = fmt.Errorf("pull checks: %w", core.ErrOffline)
			}
			f.jobsErr = fmt.Errorf("list jobs: %w", core.ErrForbidden)
			s, h := newStep(t, f, narrowW, narrowH)
			p, err := config.Default().Palette(tt.dark)
			if err != nil {
				t.Fatal(err)
			}
			s.SetTheme(ui.NewTheme(p, tt.dark))
			h.keys(tt.keys...)
			v := s.View()
			assertFits(t, v, narrowW, narrowH)
			golden.RequireEqual(t, v)
		})
	}
}

func TestViewFitsAnySize(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {10, 3}, {30, 5}, {250, 60}} {
		for _, keys := range [][]string{nil, {"enter"}, {"enter", "A"}, {"down", "down", "enter"}, {"enter", "R"}} {
			s, h := newStep(t, newFake(), wideW, wideH)
			h.keys(keys...)
			s.SetSize(size[0], size[1])
			assertFits(t, s.View(), size[0], size[1])
		}
	}
}
