package desc

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

const (
	// tabStop is the stop that tabs in a body expand to.
	tabStop = 4
	// guide is the cells of one level of the guides of a tree.
	guide = 3
	// untitled is the title of a block that has no summary.
	untitled = "Details"
)

// Styles are what the rows are drawn with. Build them once, with
// [NewStyles], and again when the theme or the icons change.
type Styles struct {
	// Fold stands for what a closed block hides, and Cut ends a text that was
	// cut.
	Fold, Cut string

	bold lipgloss.Style
}

// NewStyles returns the styles for theme t and the icon set named set, whose
// icons are ic.
func NewStyles(t ui.Theme, ic ui.Icons, set string) Styles {
	s := Styles{Fold: "⋯", Cut: ic.Ellipsis, bold: t.Text.Bold(true)}
	if set == config.IconsASCII {
		s.Fold = "..."
	}
	return s
}

// Tree returns base with the guides the description draws: a dim bar beside
// the content of an open block, and a corner ending it. The bar is the left
// border of the icons ic.
func Tree(base foldtree.Styles, ic ui.Icons) foldtree.Styles {
	base.Branch = ic.Border.Left + "  "
	base.Line = ic.Border.Left + "  "
	base.Last = "╰─ "
	return base
}

// Options say how rows are drawn.
type Options struct {
	// Width is the cells of the tree the rows are drawn in.
	Width int
	// Styles draw the rows.
	Styles Styles
	// MD renders the text, shared by every caller. Without it the text shows
	// as plain wrapped lines.
	MD *markdown.Renderer
	// Open names the folds that start open by their row IDs: "d0" for a
	// details block, and for a diagram the ID of the text it is in with ".m"
	// and its index, such as "/0.m0". Every other fold starts closed.
	Open map[string]bool
}

// Rows returns the rows of body: one for each rendered line, a fold for each
// details block and for each diagram, whose lines the fold holds, and, after
// the content of an open block, an empty row that ends it. A block's content
// is one level deeper than its summary.
func Rows(body string, o Options) []foldtree.Row {
	if o.Width <= 1 {
		return nil
	}
	b := builder{o: o}
	rows, _ := b.nodes(Parse(termtext.Clean(body, tabStop)), "", 0)
	return rows
}

type builder struct {
	o Options
}

// nodes returns the rows of nodes in the block id at depth, and the number of
// lines they come to with every block open, not counting the ends of blocks.
func (b *builder) nodes(nodes []Node, id string, depth int) (rows []foldtree.Row, total int) {
	for i, n := range nodes {
		if n.Block == nil {
			got, lines := b.text(n.Text, fmt.Sprintf("%s/%d", id, i), depth)
			rows = append(rows, got...)
			total += lines
			continue
		}
		kids, lines := b.nodes(n.Block.Nodes, n.Block.ID, depth+1)
		rows = append(rows, b.head(n.Block, lines, depth))
		rows = append(rows, kids...)
		rows = append(rows, foldtree.Row{
			ID: n.Block.ID + "/end", Depth: depth + 1, Lines: []string{""}, Skip: true,
		})
		total += lines + 1
	}
	return rows, total
}

// head returns the row of the summary of block, which hides lines lines.
func (b *builder) head(block *Block, lines, depth int) foldtree.Row {
	title := block.Summary
	if title == "" {
		title = untitled
	}
	noun := "lines"
	if lines == 1 {
		noun = "line"
	}
	return foldtree.Row{
		ID:     block.ID,
		Depth:  depth,
		Lines:  []string{b.o.Styles.bold.Render(title)},
		Marker: fmt.Sprintf("%s %d %s", b.o.Styles.Fold, lines, noun),
		Fold:   true,
		Closed: !b.o.Open[block.ID],
	}
}

// text returns the rows of the markdown src at depth, whose IDs start with
// prefix, and how many lines they come to with every diagram open.
func (b *builder) text(src, prefix string, depth int) (rows []foldtree.Row, total int) {
	room := markdown.Room(b.o.Width-1, guide*depth)
	if room == 0 {
		return nil, 0
	}
	var (
		out   string
		heads []markdown.Head
	)
	if b.o.MD == nil {
		out = ansi.Wrap(strings.Trim(src, "\r\n"), room, "")
	} else {
		var open []int
		out, heads = b.o.MD.RenderHeads(src, room)
		// Show every diagram open: a closed fold hides its lines, so
		// opening one needs no new rows.
		for _, h := range heads {
			open = append(open, h.Block)
		}
		if len(open) > 0 {
			out, heads = b.o.MD.RenderHeads(src, room, open...)
		}
	}
	if out == "" {
		return nil, 0
	}
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		if ansi.StringWidth(l) > room {
			lines[i] = ansi.Truncate(l, room, b.o.Styles.Cut)
		}
	}
	line := func(from, to int) {
		for ; from < to; from++ {
			rows = append(rows, foldtree.Row{
				ID: fmt.Sprintf("%s.%d", prefix, from), Depth: depth, Lines: lines[from : from+1 : from+1],
			})
		}
	}
	// No blank line stands next to a diagram.
	blank := func(i int) bool { return strings.TrimSpace(ansi.Strip(lines[i])) == "" }
	at := 0
	for _, h := range heads {
		if h.Line < at || h.End > len(lines) || h.End <= h.Line {
			continue
		}
		to := h.Line
		for to > at && blank(to-1) {
			to--
		}
		line(at, to)
		id := fmt.Sprintf("%s.m%d", prefix, h.Block)
		rows = append(rows, foldtree.Row{
			ID: id, Depth: depth, Lines: lines[h.Line:h.End:h.End], Fold: true, Closed: !b.o.Open[id],
		})
		at = h.End
		for at < len(lines) && blank(at) {
			at++
		}
	}
	line(at, len(lines))
	for _, r := range rows {
		total += len(r.Lines)
	}
	return rows, total
}
