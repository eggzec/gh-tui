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
