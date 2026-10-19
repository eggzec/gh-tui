package pulls

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The diff of a file, with a range selected and a draft written under it, at
// 80 columns in both themes.
func TestDraftDiffView(t *testing.T) {
	t.Parallel()
	for name, dark := range map[string]bool{"dark": true, "light": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, m, o := diffPanes(t, "a.go")
			p, _ := config.Default().Palette(dark)
			m.SetTheme(ui.NewTheme(p, dark))
			m.SetSize(80, 24)
			writeDraft(t, h, m, o, "rename it")
			// A second range, still selected, shows the selection styles.
			press(t, h, "V")
			press(t, h, "j")
			golden.RequireEqual(t, h.modals[len(h.modals)-1].View())
		})
	}
}
