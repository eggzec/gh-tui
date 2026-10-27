package prpanes

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestGlyphsFrom(t *testing.T) {
	t.Parallel()
	for _, set := range []string{config.IconsNerd, config.IconsUnicode, config.IconsASCII} {
		t.Run(set, func(t *testing.T) {
			t.Parallel()
			ic := ui.NewIcons(set)
			g := GlyphsFrom(ic)
			marks := []struct {
				name      string
				got, want string
			}{
				{"Separator", g.Separator, ic.Separator},
				{"Ellipsis", g.Ellipsis, ic.Ellipsis},
				{"Dot", g.Dot, ic.Dot},
				{"Dash", g.Dash, ic.Dash},
				{"Folded", g.Folded, ic.Folded},
			}
			for _, m := range marks {
				if m.got != m.want {
					t.Errorf("%s = %q, want the icon set's %q", m.name, m.got, m.want)
				}
				if m.got == "" {
					t.Errorf("%s is empty", m.name)
				}
				if set != config.IconsASCII {
					continue
				}
				for _, r := range m.got {
					if r > 0x7e {
						t.Errorf("%s = %q has non-ASCII rune %U", m.name, m.got, r)
					}
				}
				// The separator and the ellipsis are spelled out in ASCII
				// (" - ", "..."), so only the single marks must fit a cell.
				if m.name == "Separator" || m.name == "Ellipsis" {
					continue
				}
				if w := lipgloss.Width(m.got); w != 1 {
					t.Errorf("%s = %q is %d cells wide, want 1", m.name, m.got, w)
				}
			}
		})
	}
}
