package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
)

// setDrawing sets the glyphs that draw rather than mark: dots, arrows,
// lines and frames. The Nerd Font set draws them as the Unicode set does.
func (ic *Icons) setDrawing(set string) {
	if set == config.IconsASCII {
		ic.Dot, ic.Ring, ic.Crumb, ic.Cell = "*", "o", ">", "#"
		ic.Arrow, ic.Up, ic.Down = "->", "^", "v"
		ic.Times, ic.Minus = "x", "-"
		ic.Border = lipgloss.ASCIIBorder()
		ic.keys = asciiKeys
		return
	}
	ic.Dot, ic.Ring, ic.Crumb, ic.Cell = "●", "○", "›", "■"
	ic.Arrow, ic.Up, ic.Down = "→", "↑", "↓"
	ic.Times, ic.Minus = "×", "−"
	ic.Border = lipgloss.RoundedBorder()
}

// asciiKeys names in words the keys that help draws as arrows, pairs of
// them first, and the half of "½ page down".
var asciiKeys = strings.NewReplacer(
	"↑↓", "up/down", "←→", "left/right",
	"↑", "up", "↓", "down", "←", "left", "→", "right", "↵", "enter", "½", "half",
)

// Key returns help, the name of a key or keys as help shows it, such as
// "↑/k", or what the key does, in the words of the icon set: "up/k" in the
// ASCII set.
func (ic Icons) Key(help string) string {
	if ic.keys == nil {
		return help
	}
	return ic.keys.Replace(help)
}
