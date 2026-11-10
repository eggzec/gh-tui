package pulls

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// TestTreesFollowNavFocus holds that the people and merge trees draw their
// cursor only while their pane has the focus.
func TestTreesFollowNavFocus(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		pane panes.Pane
		view func(m *panesModal) string
	}{
		"involved": {paneInvolved, func(m *panesModal) string { return m.inv.tree.View() }},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := openedPanes(t, 160)
			m.nav.SetFocus(tc.pane)
			m.View()
			focused := tc.view(m)
			m.nav.SetFocus(paneConversation)
			m.View()
			blurred := tc.view(m)
			if focused == blurred {
				t.Fatalf("the tree draws the same with and without the focus:\n%s", focused)
			}
			m.nav.SetFocus(tc.pane)
			m.View()
			if got := tc.view(m); got != focused {
				t.Errorf("the tree lost its cursor on coming back:\n%s", got)
			}
		})
	}
}

// TestFrameUsesAccent holds that the focused frame takes the configured
// accent, not the default blue.
func TestFrameUsesAccent(t *testing.T) {
	t.Parallel()
	m := openedPanes(t, 160)
	th := m.theme
	th.Accent = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF00AA"))
	m.SetTheme(th)
	if got, want := m.frames.EdgeFocus.Render("x"), th.Accent.Render("x"); got != want {
		t.Errorf("focused edge = %q, want the accent %q", got, want)
	}
	if got, want := m.frames.StripCurrent.Render("x"), th.Accent.Bold(true).Render("x"); got != want {
		t.Errorf("strip current = %q, want %q", got, want)
	}
	if got := m.frames.TitleFocus.Render("x"); got != th.Accent.Bold(true).Render("x") {
		t.Errorf("focused title = %q, want the bold accent", got)
	}
}
