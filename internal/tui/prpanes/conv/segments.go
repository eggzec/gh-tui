package conv

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/desc"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// untitled is the title of a details block that has no summary.
const untitled = "Details"

// segs is what follows the text that leads a body: its nodes, the index in
// the body's nodes of the first of them, and where the body's text sits.
type segs struct {
	nodes []desc.Node
	base  int
	col   column
}

// body returns the lines of the row of a comment, as [builder.message] does
// for the text that leads src, and what follows that text.
func (b *builder) body(prefix, src string, depth int, kids bool, tail string) ([]string, segs) {
	head, rest, base := split(src)
	return b.message(prefix, head, depth, kids, tail), segs{nodes: rest, base: base, col: b.column(prefix, depth, kids)}
}

// segments returns the rows of what follows the head of the body of the row
// owner, at depth, which is one deeper than the row so that a closed fold
// hides them with its children. They are inline rows, which hang off the row
// as part of its body: a details block is a closed fold, and an open one
// shows its content behind a bar and a corner that ends it.
func (b *builder) segments(owner string, s segs, depth int) []treeview.Node {
	rows, _ := b.nodes(owner, owner, s.nodes, s.base, s.col, depth, 0)
	return rows
}

// nodes returns the rows of nodes, which sit level blocks deep in the block
// whose IDs start with scope, and the number of lines they come to with every
// block open. first is the index of the first node among the nodes of the
// body, which names the text.
func (b *builder) nodes(owner, scope string, nodes []desc.Node, first int, col column, depth, level int) (rows []treeview.Node, total int) {
	for i, n := range nodes {
		if n.Block == nil {
			got, lines := b.segmentText(scope+"/t"+strconv.Itoa(first+i), n.Text, col, depth, level)
			rows = append(rows, got...)
			total += lines
			continue
		}
		id := owner + "/" + n.Block.ID
		kids, lines := b.nodes(owner, id, n.Block.Nodes, 0, col, depth+1, level+1)
		rows = append(rows, b.block(id, n.Block, lines, col, depth, level))
		rows = append(rows, kids...)
		rows = append(rows, treeview.Node{
			ID: id + "/end", Depth: depth + 1, Inline: true, Skip: true, Kind: KindSegment,
			Lines: []string{b.lead(col, level) + b.st.dim.Render(b.st.Glyphs.Corner)},
		})
		total += lines + 2
	}
	return rows, total
}

// lead returns what starts each line of a segment: the space of the body and
// a bar for each level.
func (b *builder) lead(col column, level int) string {
	s := strings.Repeat(" ", col.pad)
	if level > 0 {
		s += b.st.dim.Render(strings.Repeat(b.st.Glyphs.Bar, level))
	}
	return s
}

// block returns the row of the summary of a details block, which hides lines
// lines.
func (b *builder) block(id string, bl *desc.Block, lines int, col column, depth, level int) treeview.Node {
	title := termtext.OneLine(bl.Summary)
	if title == "" {
		title = untitled
	}
	title = termtext.Truncate(title, max(col.room-2*level, 1), b.st.Glyphs.Cut)
	return treeview.Node{
		ID: id, Depth: depth, Inline: true, Branch: true, Closed: true, Kind: KindSegment,
		Lines:  []string{b.lead(col, level) + b.st.bold.Render(title)},
		Marker: b.marker(lines, "line", "lines"),
	}
}

// segmentText returns the rows of the markdown src, which is level blocks
// deep, and how many lines they have with every diagram open. The text is one
// row, unless it holds diagrams: each of those is a closed fold, and the text
// between them is a row of its own.
func (b *builder) segmentText(id, src string, col column, depth, level int) (rows []treeview.Node, total int) {
	// The parser caps how deep blocks nest, but the bars of the deepest can
	// still take the whole width.
	room := col.room - 2*level
	if room < 1 {
		return nil, 0
	}
	src = strings.TrimSpace(src)
	lead := b.lead(col, level)
	if b.o.MD != nil {
		if _, heads := b.o.MD.RenderHeads(src, room); len(heads) > 0 {
			return b.diagrams(id, src, lead, room, depth)
		}
	}
	lines := b.render(src, room)
	if len(lines) == 0 {
		return nil, 0
	}
	for i, l := range lines {
		lines[i] = lead + l
	}
	return []treeview.Node{{ID: id, Depth: depth, Lines: lines, Inline: true, Kind: KindSegment}}, len(lines)
}

// diagrams returns the rows of the markdown src, which holds diagrams, in room
// cells, whose IDs start with id, with each line led by lead, and how many
// lines they have with every diagram open. Lines of code are cut to room, not
// wrapped, and no blank line stands next to a diagram.
func (b *builder) diagrams(id, src, lead string, room, depth int) (rows []treeview.Node, total int) {
	_, heads := b.o.MD.RenderHeads(src, room)
	// Show every diagram open: a closed fold hides its lines, so opening one
	// needs no new rows.
	open := make([]int, len(heads))
	for i, h := range heads {
		open[i] = h.Block
	}
	out, heads := b.o.MD.RenderHeads(src, room, open...)
	lines := strings.Split(out, "\n")
	blank := func(l string) bool { return strings.TrimSpace(ansi.Strip(l)) == "" }
	text := func(rowID string, from, to int) {
		for from < to && blank(lines[from]) {
			from++
		}
		for to > from && blank(lines[to-1]) {
			to--
		}
		part := b.cap(b.code(lines[from:to], room))
		if len(part) == 0 {
			return
		}
		out := make([]string, len(part))
		for i, l := range part {
			out[i] = lead + l
		}
		rows = append(rows, treeview.Node{ID: rowID, Depth: depth, Lines: out, Inline: true, Kind: KindSegment})
		total += len(out)
	}
	at, k := 0, 0
	for _, h := range heads {
		if h.Line < at || h.End > len(lines) || h.End <= h.Line {
			continue
		}
		text(id+".s"+strconv.Itoa(k), at, h.Line)
		code := make([]string, h.End-h.Line)
		for i, l := range lines[h.Line:h.End] {
			if ansi.StringWidth(l) > room {
				l = ansi.Truncate(l, room, b.st.Glyphs.Cut)
			}
			code[i] = lead + l
		}
		rows = append(rows, treeview.Node{
			ID: id + ".m" + strconv.Itoa(h.Block), Depth: depth, Lines: code,
			Inline: true, Branch: true, Closed: true, Kind: KindSegment,
		})
		total += len(code)
		at, k = h.End, h.Block+1
	}
	text(id+".s"+strconv.Itoa(k), at, len(lines))
	return rows, total
}

// segmentRe matches what an ID of a row of a segment adds to the ID of its
// owner: a block, the end of one, and the text at the top or inside one.
var segmentRe = regexp.MustCompile(`/(d\d+(\.\d+)*/(end|t\d+(\.[ms]\d+)?)|d\d+(\.\d+)*|t\d+(\.[ms]\d+)?)$`)

// Owner returns the ID of the row that holds the body a row of a segment
// belongs to, and id when it is not one. Only the IDs of the rows that hold
// bodies are told apart: comments, reviews, threads and their replies.
func Owner(id string) string {
	for _, p := range []string{"c:", "rc:", "r:", "t:", "p:"} {
		if strings.HasPrefix(id, p) {
			return segmentRe.ReplaceAllString(id, "")
		}
	}
	return id
}
