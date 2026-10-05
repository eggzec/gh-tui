package ownerui

import (
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Drawer draws the profile and the cards of a page.
type Drawer struct {
	Styles *Styles
	Icons  ui.Icons
	// Links keeps the links of the cards' names.
	Links *termtext.Links
	// URL is the web page of a repository, and Lang the paint of the glyph
	// of its language.
	URL  func(core.Repo) string
	Lang func(core.Repo) Paint
}

// Lines renders the rows of cards on view in w cells, each card drawn by
// draw in the width it gets.
func (c *Cards) Lines(w, h int, draw func(c Card, selected bool, w int) [CardHeight]string) []string {
	lines := make([]string, 0, h)
	for row := c.Top; row < c.Top+c.Rows && row*c.Cols < len(c.Items); row++ {
		if row > c.Top {
			lines = append(lines, "")
		}
		var rowLines [CardHeight]strings.Builder
		for col := range c.Cols {
			i := row*c.Cols + col
			if i >= len(c.Items) {
				break
			}
			card := draw(c.Items[i], i == c.Sel, c.CardWidth(w, col))
			for l := range CardHeight {
				if col > 0 {
					rowLines[l].WriteString(strings.Repeat(" ", CardGap))
				}
				rowLines[l].WriteString(card[l])
			}
		}
		for l := range rowLines {
			lines = append(lines, rowLines[l].String())
		}
	}
	return lines
}

// Card renders a pinned repository in w cells after gutter: its name, two
// lines of its description, and its language and stars.
func (d Drawer) Card(c Card, gutter string, w int) [CardHeight]string {
	st := d.Styles
	inner := max(w-2, 0)
	r := c.Repo
	var out [CardHeight]string
	out[0] = gutter + d.Links.Link(d.URL(r), st.Name.Render(Truncate(r.Ref.String(), inner, d.Icons.Ellipsis)))
	desc := Wrap(CleanLine(r.Description), inner, 2, d.Icons.Ellipsis)
	if len(desc) == 0 && c.Here && r.Description == "" {
		desc = []string{"The repository of this directory."}
	}
	for i, l := range desc {
		out[1+i] = gutter + st.Muted.Render(l)
	}
	for i := len(desc); i < 2; i++ {
		out[1+i] = gutter
	}
	facts := d.RepoFacts(r, inner)
	if c.Here {
		facts = st.Accent.Render(d.Icons.Here) + "  " + facts
	}
	out[3] = gutter + termtext.Truncate(facts, inner, d.Icons.Ellipsis)
	for i := range out {
		out[i] = Fit(out[i], w)
	}
	return out
}

// RepoFacts renders the language, stars and flags of r in at most w cells.
func (d Drawer) RepoFacts(r core.Repo, w int) string {
	st := d.Styles
	parts := make([]string, 0, 4)
	if r.Language != "" {
		parts = append(parts, d.Lang(r).Render(d.Icons.Language(r.Language))+" "+st.Text.Render(r.Language))
	}
	parts = append(parts, st.Muted.Render(d.Icons.Star+" "+Count(r.Stars)))
	if flags := d.Icons.Flags(r); len(flags) > 0 {
		parts = append(parts, st.Subtle.Render(strings.Join(flags, " ")))
	}
	return termtext.Truncate(strings.Join(parts, "  "), w, d.Icons.Ellipsis)
}
