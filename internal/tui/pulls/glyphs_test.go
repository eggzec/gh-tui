package pulls

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestStateGlyphs(t *testing.T) {
	h := started(t, newFakeService(), 100, 10, WithIcons(ui.NewIcons(config.IconsASCII)))
	s := h.Section
	for _, tt := range []struct {
		state core.State
		draft bool
		row   string
		badge string
	}{
		{core.StateOpen, false, "O #1 ", " O Open "},
		{core.StateOpen, true, "D #1 ", " D Draft "},
		{core.StateMerged, false, "M #1 ", " M Merged "},
		{core.StateClosed, false, "X #1 ", " X Closed "},
		// A closed draft was declined all the same.
		{core.StateClosed, true, "X #1 ", " X Closed "},
	} {
		pr := core.PullRequest{Draft: tt.draft}
		pr.Number, pr.Title, pr.State = 1, "T", tt.state
		if row := ansi.Strip(s.renderRow(pr, false, 100)); !strings.HasPrefix(row, tt.row) {
			t.Errorf("%s draft %v: row = %q, want it to start with %q", tt.state, tt.draft, row, tt.row)
		}
		if badge := ansi.Strip(s.st.badge(pr)); badge != tt.badge {
			t.Errorf("%s draft %v: badge = %q, want %q", tt.state, tt.draft, badge, tt.badge)
		}
	}
}

// Numbers too long for their column take the extra cells from the title,
// so the row keeps its width and the space after the number. A row too
// narrow for the glyph and the number is cut.
func TestLongNumbers(t *testing.T) {
	h := started(t, newFakeService(), 100, 10, WithIcons(ui.NewIcons(config.IconsASCII)))
	s := h.Section
	for _, tt := range []struct {
		number int
		row    string
	}{
		{7, "O #7     "},
		{12345, "O #12345 "},
		{123456, "O #123456 "},
		{1234567890, "O #1234567890 "},
	} {
		pr := core.PullRequest{}
		pr.Number, pr.Title, pr.State = tt.number, "Title", core.StateOpen
		for _, width := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 20, 40, 80, 120} {
			row := ansi.Strip(s.renderRow(pr, false, width))
			if want := tt.row[:min(len(tt.row), width)]; !strings.HasPrefix(row, want) {
				t.Errorf("#%d at %d: row = %q, want it to start with %q", tt.number, width, row, want)
			}
			if w := ansi.StringWidth(row); w != width {
				t.Errorf("#%d at %d: row is %d cells wide", tt.number, width, w)
			}
		}
	}
}
