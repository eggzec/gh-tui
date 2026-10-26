package panes

import (
	"fmt"
	"strings"
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
	build := func(name func(Def) string, sep string, digits bool) string {
		parts := make([]string, 0, len(defs))
		for i, d := range defs {
			p := Pane(i + 1)
			var t string
			switch {
			case digits && p == cur:
				t = fmt.Sprintf("[%d]", p)
			case digits:
				t = fmt.Sprintf("%d", p)
			default:
				t = fmt.Sprintf("%d %s", p, name(d))
			}
			if p == cur {
				parts = append(parts, s.StripCurrent.Render(t))
			} else {
				parts = append(parts, s.StripOther.Render(t))
			}
		}
		return strings.Join(parts, s.StripSep.Render(sep))
	}
	full := func(d Def) string { return d.Name }
	short := func(d Def) string { return d.Short }
	return []string{
		build(full, s.Separator, false),
		build(short, s.Separator, false),
		build(short, " ", false),
		build(nil, " ", true),
	}
}
