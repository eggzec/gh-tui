package tui

import (
	"testing"
	"testing/synctest"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
)

// The frame of the panes modal of a pull request at 80, 120 and 160 columns,
// where all six panes sit side by side, and with plain text where icons are.
func TestPullModalFrame(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		icons         string
		width, height int
	}{
		{"80", config.IconsUnicode, 80, 24},
		{"120", config.IconsUnicode, 120, 36},
		{"160", config.IconsUnicode, 160, 40},
		{"ascii", config.IconsASCII, 80, 24},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := newKeysAppWith(t, true, func(c *config.Config) { c.UI.Icons = tt.icons })
				resize(m, tt.width, tt.height)
				c := keyContext{name: "pull request tabs"}
				for _, k := range []string{"global.pane_2", "global.select"} {
					for _, name := range c.press(t, m.cfg.Keys, k) {
						msg, _ := keyPress(name)
						driveKeys(t, m, m.key(msg))
					}
				}
				golden.RequireEqual(t, ansi.Strip(m.View().Content))
			})
		})
	}
}
