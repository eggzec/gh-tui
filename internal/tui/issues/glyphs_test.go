package issues

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestStateGlyphs(t *testing.T) {
	s := New(t.Context(), newFakeService(nil), config.Default().Keys, WithIcons(ui.NewIcons(config.IconsASCII)))
	for _, tt := range []struct {
		state  core.State
		reason core.StateReason
		row    string
		badge  string
	}{
		{core.StateOpen, "", "#1 o T", "o Open"},
		{core.StateOpen, core.ReasonReopened, "#1 o T", "o Open"},
		{core.StateClosed, "", "#1 x T", "x Closed"},
		{core.StateClosed, core.ReasonCompleted, "#1 x T", "x Closed"},
		{core.StateClosed, core.ReasonNotPlanned, "#1 - T", "- Closed as not planned"},
		{core.StateClosed, core.ReasonDuplicate, "#1 - T", "- Closed as not planned"},
	} {
		it := core.Issue{Number: 1, Title: "T", State: tt.state, Reason: tt.reason}
		if row := strings.TrimSpace(ansi.Strip(s.renderRow(it, false, 80))); !strings.HasPrefix(row, tt.row) {
			t.Errorf("%s %s: row = %q, want it to start with %q", tt.state, tt.reason, row, tt.row)
		}
		if badge := ansi.Strip(s.rows.badges[ui.IssueState(it)]); badge != tt.badge {
			t.Errorf("%s %s: badge = %q, want %q", tt.state, tt.reason, badge, tt.badge)
		}
	}
}
