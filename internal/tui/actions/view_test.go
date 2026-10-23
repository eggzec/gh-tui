package actions

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

// With the ASCII icons every view of the modal is ASCII alone, as wide or
// narrow as the frame is: the runs, the jobs, the log, the filters and a
// confirmation.
func TestViewASCII(t *testing.T) {
	for _, tt := range []struct {
		width, height int
		keys          []string
	}{
		{wideW, wideH, nil}, {wideW, wideH, []string{"tab"}}, {wideW, wideH, []string{"j"}},
		{wideW, wideH, []string{"f"}}, {wideW, wideH, []string{"R"}}, {wideW, wideH, []string{"]"}},
		{narrowW, narrowH, []string{"j"}}, {narrowW, narrowH, []string{"j", "enter"}},
		{narrowW, narrowH, []string{"enter", "j", "j", "enter"}}, {narrowW, narrowH, []string{"f"}},
	} {
		m, h := newModal(t, newFake(), tt.width, tt.height, WithIcons(ui.NewIcons(config.IconsASCII)))
		h.keys(tt.keys...)
		// The guides of the outline are box drawing, whatever the icons.
		if v := ansi.Strip(m.View()); strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII && !strings.ContainsRune("│├└▌", r) }) {
			t.Errorf("%dx%d after %v: view isn't ASCII:\n%s", tt.width, tt.height, tt.keys, v)
		}
	}
}

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		keys          []string
	}{
		{"190 columns", wideW, wideH, nil},
		{"190 columns jobs", wideW, wideH, []string{"tab"}},
		{"190 columns running", wideW, wideH, []string{"j"}},
		{"190 columns zoom", wideW, wideH, []string{"tab", "tab", "z"}},
		{"190 columns filter", wideW, wideH, []string{"f"}},
		{"190 columns confirm", wideW, wideH, []string{"R"}},
		{"190 columns failing", wideW, wideH, []string{"]"}},
		{"120 columns runs", 120, 40, nil},
		{"120 columns jobs", 120, 40, []string{"tab"}},
		{"80 columns runs", narrowW, narrowH, nil},
		{"80 columns jobs", narrowW, narrowH, []string{"j"}},
		{"80 columns log", narrowW, narrowH, []string{"j", "enter"}},
		{"80 columns running", narrowW, narrowH, []string{"enter", "j", "j", "enter"}},
		{"80 columns filter", narrowW, narrowH, []string{"f"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, h := newModal(t, newFake(), tt.width, tt.height)
			h.keys(tt.keys...)
			v := m.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

// Jobs that failed to load say why, in both themes.
func TestViewJobsFailed(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%t", dark), func(t *testing.T) {
			f := newFake()
			f.jobsErr = fmt.Errorf("list jobs: %w", core.ErrOffline)
			m, h := newModal(t, f, narrowW, narrowH, WithIcons(ui.NewIcons(config.IconsUnicode)))
			p, err := config.Default().Palette(dark)
			if err != nil {
				t.Fatal(err)
			}
			m.SetTheme(ui.NewTheme(p, dark))
			h.keys("j")
			v := m.View()
			assertFits(t, v, narrowW, narrowH)
			golden.RequireEqual(t, v)
		})
	}
}

// Jobs that wait for an approval or for others to finish look apart from
// queued ones, in every icon set.
func TestViewWaiting(t *testing.T) {
	for _, set := range []string{config.IconsUnicode, config.IconsASCII, config.IconsNerd} {
		t.Run(set, func(t *testing.T) {
			f := newFake()
			waiting := func(id int64, name string, status core.RunStatus) core.Job {
				return core.Job{ID: id, RunID: runningRun, Attempt: 1, Name: name, Status: status}
			}
			f.jobs[runningRun] = append(f.jobs[runningRun],
				waiting(501, "deploy (production)", core.RunWaiting),
				waiting(502, "e2e", core.RunPending),
				waiting(503, "release", core.RunRequested),
				waiting(504, "docs", core.RunQueued))
			m, h := newModal(t, f, wideW, 10, WithIcons(ui.NewIcons(set)))
			h.keys("enter", "j")
			v := m.View()
			if s := screen(m); !strings.Contains(s, "deploy (production)") || !strings.Contains(s, "waiting") {
				t.Errorf("the waiting jobs:\n%s", s)
			}
			assertFits(t, v, wideW, 10)
			golden.RequireEqual(t, v)
		})
	}
}

// The jobs past what the service reads are left to GitHub, and only then
// does the pane say so.
func TestViewMoreJobs(t *testing.T) {
	for _, more := range []bool{false, true} {
		f := newFake()
		f.moreJobs = more
		m, h := newModal(t, f, wideW, wideH)
		h.keys("tab")
		s := paneText(m, jobsPane)
		if got := strings.Contains(s, "First 4 jobs · o shows all"); got != more {
			t.Errorf("more jobs %v: the pane says so %v:\n%s", more, got, s)
		}
	}
}

func TestViewFitsAnySize(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {10, 3}, {30, 5}, {109, 12}, {110, 12}, {250, 60}} {
		for _, keys := range [][]string{nil, {"tab"}, {"tab", "tab"}, {"j", "tab", "tab"}, {"z"}, {"f"}, {"R"}} {
			m, h := newModal(t, newFake(), wideW, wideH)
			h.keys(keys...)
			m.SetSize(size[0], size[1])
			assertFits(t, m.View(), size[0], size[1])
		}
	}
}

// At 80 columns the jobs of the open run show under it in the runs' tree.
func TestViewTwoPanesAt80(t *testing.T) {
	m, _ := newModal(t, newFake(), narrowW, narrowH)
	s := screen(m)
	if !strings.Contains(s, "macos-lat") || !strings.Contains(s, "fix: skip") {
		t.Errorf("the 80-column modal shows only one section:\n%s", s)
	}
	assertFits(t, m.View(), narrowW, narrowH)
}
