package conv

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/desc"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

const (
	// tabStop is the stop that tabs in a body expand to.
	tabStop = 4
	// maxBodyLines is the most lines of a body a row holds, which guards
	// against very long ones.
	maxBodyLines = 200
	// guide is the cells of one level of the guides of a tree.
	guide = 3
	// arrow is the cells of the fold arrow that the tree puts before a row
	// with lines under its head.
	arrow = 2
)

// split cleans src and cuts it into the text that leads the body, trimmed,
// and the nodes after it: the details blocks and the text between them. The
// leading text ends where its first diagram starts, which opens the nodes.
// base is the index in the body's nodes of the first of those.
func split(src string) (head string, rest []desc.Node, base int) {
	nodes := desc.Parse(termtext.Clean(src, tabStop))
	if len(nodes) == 0 || nodes[0].Block != nil {
		return "", nodes, 0
	}
	text := nodes[0].Text
	if i := diagramAt(text); i >= 0 {
		return strings.TrimSpace(text[:i]), append([]desc.Node{{Text: text[i:]}}, nodes[1:]...), 0
	}
	return strings.TrimSpace(text), nodes[1:], 1
}

// diagramAt returns where the first diagram of the markdown src starts, or -1.
// It is a mermaid block whose fence has no indent, which the renderer draws
// on its own, as a head with the block's code under it. An indented one may
// sit in a list or hold other text, and stays in the text before it.
func diagramAt(src string) int {
	var char byte // the fence char of the code block the line is in, or 0
	var size int  // the length of that fence
	for pos := 0; pos < len(src); {
		line, _, _ := strings.Cut(src[pos:], "\n")
		start := pos
		pos += len(line) + 1
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		if indent > 3 || trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
			continue
		}
		run := len(trimmed) - len(strings.TrimLeft(trimmed, trimmed[:1]))
		info := strings.TrimSpace(trimmed[run:])
		switch {
		case run < 3:
		case char != 0:
			if trimmed[0] == char && run >= size && info == "" {
				char = 0
			}
		case trimmed[0] == '`' && strings.Contains(info, "`"):
		default:
			if lang := strings.Fields(strings.ToLower(info + " ")); indent == 0 && len(lang) > 0 && lang[0] == "mermaid" {
				return start
			}
			char, size = trimmed[0], run
		}
	}
	return -1
}

// render returns the lines of the markdown src, a prepared body, at most room
// cells wide, or the first maxBodyLines of them and a note of the rest.
func (b *builder) render(src string, room int) []string {
	if src == "" {
		return nil
	}
	var out string
	if b.o.MD != nil {
		out = b.o.MD.Render(src, room)
	} else {
		out = ansi.Wrap(src, room, "")
	}
	if out == "" {
		return nil
	}
	return b.cap(b.code(strings.Split(out, "\n"), room))
}

// cap returns the first maxBodyLines of lines and a note of the rest.
func (b *builder) cap(lines []string) []string {
	if n := len(lines) - maxBodyLines; n > 0 {
		lines = append(lines[:maxBodyLines:maxBodyLines],
			b.st.dim.Render(b.st.Glyphs.Cut+" "+strconv.Itoa(n)+" more lines"))
	}
	return lines
}

// code returns lines cut to room cells, which the lines of code are not
// wrapped to, and without the blank lines next to a block of code.
func (b *builder) code(lines []string, room int) []string {
	isCode := func(l string) bool { return b.o.MD != nil && b.o.MD.IsCode(l) }
	blank := func(l string) bool { return strings.TrimSpace(ansi.Strip(l)) == "" }
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if blank(l) {
			// A run of blank lines goes when code lies at either end of it.
			j := i
			for j < len(lines) && blank(lines[j]) {
				j++
			}
			if (len(out) > 0 && isCode(out[len(out)-1])) || (j < len(lines) && isCode(lines[j])) {
				i = j - 1
				continue
			}
		}
		if room > 0 && ansi.StringWidth(l) > room {
			l = ansi.Truncate(l, room, b.st.Glyphs.Cut)
		}
		out = append(out, l)
	}
	return out
}

// column is where the text of a row sits.
type column struct {
	// limit is the cells the row has, pw those of its prefix, and lead says
	// whether the text starts beside the prefix.
	limit, pw int
	lead      bool
	// pad is the cells that the lines under the head start after, and room
	// the cells the text of those lines has.
	pad, room int
}

// column returns where the text of a row with prefix sits, at depth and with
// or without children.
func (b *builder) column(prefix string, depth int, kids bool) column {
	limit := b.width - 1 - guide*depth - arrow
	pw := ansi.StringWidth(prefix)
	// A prefix that takes much of the row leaves the text no room beside it:
	// the text goes under it instead.
	lead := pw+2 <= limit/2
	hang := 2
	if lead {
		hang = pw + 2
	}
	// The lines under the head follow the guide that carries on to the
	// children, which takes guide cells of the hang, and the text of a head
	// without a prefix beside it sits just past that guide.
	col := hang
	pad := hang + arrow
	if kids {
		col = max(hang, guide)
		pad = col - guide + arrow
	}
	return column{limit: limit, pw: pw, lead: lead, pad: pad, room: markdown.Room(limit, col)}
}

// message returns the lines of a row: prefix, then src, the text that leads
// its body, whose first line follows prefix on the head and the others hang
// under it. tail is what the head holds at its right end, which the text
// leaves room for. depth is the depth of the row and kids says whether it has
// children.
func (b *builder) message(prefix, src string, depth int, kids bool, tail string) []string {
	c := b.column(prefix, depth, kids)
	limit, pw, lead, room := c.limit, c.pw, c.lead, c.room
	lines := b.render(src, room)
	if len(lines) == 0 {
		return []string{prefix}
	}
	pads := strings.Repeat(" ", c.pad)
	if !lead {
		out := []string{prefix}
		for _, l := range lines {
			out = append(out, pads+l)
		}
		return out
	}
	if tail != "" {
		tw := ansi.StringWidth(tail) + 2
		if pw+2+ansi.StringWidth(lines[0])+tw > limit {
			lines = b.render(src, markdown.Room(room, tw))
		}
	}
	out := make([]string, 0, len(lines))
	out = append(out, prefix+"  "+lines[0])
	for _, l := range lines[1:] {
		out = append(out, pads+l)
	}
	return out
}

// quiet returns lines of a message made with prefix, with their text dim: the
// colours of the markdown go, its links stay, and prefix is left as it is.
func (b *builder) quiet(lines []string, prefix string) []string {
	head := prefix + "  "
	for i, l := range lines {
		if i == 0 {
			rest, ok := strings.CutPrefix(l, head)
			if !ok {
				continue
			}
			lines[i] = head + b.st.dim.Render(uncolor(rest))
			continue
		}
		lines[i] = b.st.dim.Render(uncolor(l))
	}
	return lines
}

// uncolor returns s without its colours and styles, and with its links.
func uncolor(s string) string {
	var sb strings.Builder
	var state byte
	for s != "" {
		seq, _, n, next := ansi.DecodeSequence(s, state, nil)
		state = next
		if !ansi.HasCsiPrefix(seq) || !strings.HasSuffix(seq, "m") {
			sb.WriteString(seq)
		}
		s = s[n:]
	}
	return sb.String()
}
