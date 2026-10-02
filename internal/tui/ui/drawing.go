package ui

import (
	"cmp"
	"strings"

	"charm.land/bubbles/v2/spinner"

	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
)

// setDrawing sets the glyphs that draw rather than mark: dots, arrows,
// lines and frames. The Nerd Font set draws them as the Unicode set does.
func (ic *Icons) setDrawing(set string) {
	if set == config.IconsASCII {
		ic.Dot, ic.Ring, ic.Crumb, ic.Before, ic.Cell = "*", "o", ">", "<", "#"
		ic.Arrow, ic.Up, ic.Down = "->", "^", "v"
		ic.Times, ic.Minus = "x", "-"
		ic.Border = lipgloss.ASCIIBorder()
		ic.Edge, ic.InputEdge, ic.Remove, ic.Warning, ic.Below = "|", "|", "x", "!", "->"
		ic.Comment, ic.Recent = "c", "~"
		ic.OpenQuote, ic.CloseQuote, ic.Dash = `"`, `"`, "-"
		ic.Spinner = spinner.Line
		ic.keys = asciiKeys
		return
	}
	ic.Dot, ic.Ring, ic.Crumb, ic.Before, ic.Cell = "●", "○", "›", "‹", "■"
	ic.Arrow, ic.Up, ic.Down = "→", "↑", "↓"
	ic.Times, ic.Minus = "×", "−"
	ic.Border = lipgloss.RoundedBorder()
	ic.Edge, ic.InputEdge, ic.Remove, ic.Warning, ic.Below = "▌", "┃", "✕", "⚠", "↳"
	ic.Comment, ic.Recent = "◦", "↺"
	ic.OpenQuote, ic.CloseQuote, ic.Dash = "“", "”", "—"
}

// SpinnerOr returns the spinner of the icon set, or def where the set
// leaves each view its own. The set's frames repeat to as many as def has,
// at def's pace, and take a space after them as those of def do, so that a
// view switching between the two mid-spin always has a frame to draw and
// keeps the text after it in place.
func (ic Icons) SpinnerOr(def spinner.Spinner) spinner.Spinner {
	n := len(ic.Spinner.Frames)
	if n == 0 || len(def.Frames) == 0 {
		return def
	}
	pad := ""
	if strings.HasSuffix(def.Frames[0], " ") {
		pad = " "
	}
	s := spinner.Spinner{Frames: make([]string, len(def.Frames)), FPS: def.FPS}
	for i := range s.Frames {
		s.Frames[i] = ic.Spinner.Frames[i%n] + pad
	}
	return s
}

// OrUnicode returns ic with the separator, ellipsis and dash of the
// Unicode set where it has none, as zero icons don't, so words joined
// with them read well whoever builds them.
func (ic Icons) OrUnicode() Icons {
	ic.Separator = cmp.Or(ic.Separator, " · ")
	ic.Ellipsis = cmp.Or(ic.Ellipsis, "…")
	ic.Dash = cmp.Or(ic.Dash, "—")
	return ic
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
