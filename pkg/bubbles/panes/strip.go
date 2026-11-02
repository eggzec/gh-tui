package panes

import (
	"fmt"

	"github.com/eggzec/gh-tui/pkg/bubbles/tabrow"
)

// StripForms are the forms of a pane strip for the panes defs with the pane
// cur current, from the longest to the shortest: full names, short names,
// short names without separators, then digits with the current pane in
// brackets. The current pane is accented, the others dim. The caller draws
// the strip as one row where it likes, with the first form that fits; the
// bubble draws none itself, because where a strip belongs depends on the
// modal around the panes.
func (s Styles) StripForms(defs []Def, cur Pane) []string {
	defs = defs[:min(len(defs), maxPanes)]
	items := make([]tabrow.Item, len(defs))
	for i, d := range defs {
		items[i] = tabrow.Item{
			Label: fmt.Sprintf("%d %s", i+1, d.Name),
			Short: fmt.Sprintf("%d %s", i+1, d.Short),
		}
	}
	row := tabrow.New(
		tabrow.WithStyles(tabrow.Styles{Current: s.StripCurrent, Other: s.StripOther, Marker: s.StripSep}),
		tabrow.WithSeparator(s.Separator),
		tabrow.WithStyledBlanks(),
	)
	row.SetItems(items, int(cur)-1)
	return row.Forms()
}
