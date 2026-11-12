package history

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

// The sizes inside the frame on terminals of 140 by 40 and 80 by 24.
const (
	wideW, wideH     = 108, 30
	narrowW, narrowH = 60, 18
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		keys          []string
	}{
		{"140 columns", wideW, wideH, nil},
		{"140 columns branches", wideW, wideH, []string{"backspace", "j"}},
		{"140 columns patch", wideW, wideH, []string{"enter", "enter"}},
		{"140 columns zoomed", wideW, wideH, []string{"z"}},
		{"140 columns zoomed commit", wideW, wideH, []string{"z", "enter"}},
		{"92 columns zoomed patch", 92, 28, []string{"z", "enter", "enter"}},
		{"80 columns graph", narrowW, narrowH, nil},
		{"80 columns branches", narrowW, narrowH, []string{"backspace", "j"}},
		{"80 columns commit", narrowW, narrowH, []string{"enter"}},
		{"80 columns patch", narrowW, narrowH, []string{"enter", "enter"}},
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

// With the ASCII icons every pane draws ASCII alone: the graph, the
// branches, the commit and its patch, wide and narrow.
func TestViewASCII(t *testing.T) {
	for _, tt := range []struct {
		width, height int
		keys          []string
	}{
		{wideW, wideH, nil},
		{wideW, wideH, []string{"backspace", "j"}},
		{wideW, wideH, []string{"enter", "enter"}},
		{narrowW, narrowH, []string{"backspace", "j"}},
		{narrowW, narrowH, []string{"enter"}},
		{narrowW, narrowH, []string{"enter", "enter"}},
	} {
		m, h := newModal(t, newFake(), tt.width, tt.height, WithIcons(ui.NewIcons(config.IconsASCII)))
		h.keys(tt.keys...)
		if v := ansi.Strip(m.View()); strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("%dx%d after %v: view isn't ASCII:\n%s", tt.width, tt.height, tt.keys, v)
		}
	}
}

// A :set of the icons redraws the rows of the graph already shown, not just
// the rows read after it: the verified mark of the newest commit changes.
func TestSettingsIconsRedrawRows(t *testing.T) {
	cfg := testConfig()
	cfg.Row = []string{config.FieldShortSHA, config.FieldSubject, config.FieldVerified, config.FieldAge}
	uni, asc := ui.NewIcons(config.IconsUnicode), ui.NewIcons(config.IconsASCII)
	m, _ := newModal(t, newFake(), wideW, wideH, WithConfig(cfg), WithIcons(uni))
	if uni.Yes == asc.Yes {
		t.Fatalf("both icon sets mark a verified signature with %q", uni.Yes)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, uni.Yes) {
		t.Fatalf("before :set the graph lacks %q:\n%s", uni.Yes, v)
	}

	c := config.Default()
	c.UI.Icons = config.IconsASCII
	m.Update(ui.SettingsMsg{Config: c})
	m.SetTheme(testTheme())

	v := ansi.Strip(m.View())
	if strings.Contains(v, uni.Yes) || !strings.Contains(v, asc.Yes) {
		t.Errorf("after :set the graph rows should show %q, not %q:\n%s", asc.Yes, uni.Yes, v)
	}
}

// The commit and the branches say why they failed, in both themes.
func TestViewFailed(t *testing.T) {
	for _, tt := range []struct {
		name string
		keys []string
		dark bool
	}{
		{"commit light", []string{"enter"}, false},
		{"commit dark", []string{"enter"}, true},
		{"branches light", []string{"backspace"}, false},
		{"branches dark", []string{"backspace"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.errs["branches"] = fmt.Errorf("list branches: %w", core.ErrOffline)
			f.errs["commit "+short(main0)] = fmt.Errorf("get commit: %w", core.ErrForbidden)
			m, h := newModal(t, f, narrowW, narrowH)
			p, err := config.Default().Palette(tt.dark)
			if err != nil {
				t.Fatal(err)
			}
			m.SetTheme(ui.NewTheme(p, tt.dark))
			h.keys(tt.keys...)
			v := m.View()
			assertFits(t, v, narrowW, narrowH)
			golden.RequireEqual(t, v)
		})
	}
}

func TestViewFitsAnySize(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {10, 3}, {30, 5}, {89, 12}, {90, 12}, {200, 50}} {
		for _, keys := range [][]string{nil, {"backspace"}, {"enter"}, {"enter", "enter"}} {
			m, h := newModal(t, newFake(), wideW, wideH)
			h.keys(keys...)
			m.SetSize(size[0], size[1])
			assertFits(t, m.View(), size[0], size[1])
		}
	}
}

// The title of the branches counts them the way the other lists do: 1.2k,
// and a "+" while more pages are left.
func TestBranchesTitleCount(t *testing.T) {
	t.Parallel()
	f := newFake()
	items := make([]core.Branch, 1200)
	for i := range items {
		items[i] = core.Branch{Name: fmt.Sprintf("b%d", i)}
	}
	f.branchPages = map[string]core.Page[core.Branch]{"": {Items: items, Next: "more"}}
	m, _ := newModal(t, f, wideW, wideH)
	if s := screen(m); !strings.Contains(s, "Branches") || !strings.Contains(s, "1.2k+") {
		t.Errorf("branches title lacks 1.2k+:\n%s", s)
	}
}

func TestNarrowShowsOnePane(t *testing.T) {
	m, h := newModal(t, newFake(), narrowW, narrowH)
	if s := screen(m); strings.Contains(s, "fix/tabs") || !strings.Contains(s, "main: change 1") {
		t.Errorf("narrow screen shows more than the graph:\n%s", s)
	}
	h.keys("enter")
	if s := screen(m); !strings.Contains(s, "commands.go") || strings.Contains(s, "main: change 1") {
		t.Errorf("narrow screen of the commit:\n%s", s)
	}
	// The patch isn't previewed below the files: there is no room.
	if m.pagerShown() {
		t.Error("the pager shows below the files")
	}
}
