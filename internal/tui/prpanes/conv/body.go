package conv

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

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
)

// details finds where a collapsible block starts, which a body is cut at.
var details = regexp.MustCompile(`(?i)<details`)

// prepare returns src cleaned, tab-free and cut at its first collapsible
// block.
func prepare(src string) string {
	src = termtext.Clean(src, tabStop)
	if loc := details.FindStringIndex(src); loc != nil {
		src = src[:loc[0]]
	}
	return strings.TrimSpace(src)
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
	lines := strings.Split(out, "\n")
	if n := len(lines) - maxBodyLines; n > 0 {
		lines = append(lines[:maxBodyLines:maxBodyLines],
			b.st.dim.Render(b.st.Glyphs.Cut+" "+strconv.Itoa(n)+" more lines"))
	}
	return lines
}

// message returns the lines of a row: prefix, then body, whose first line
// follows prefix on the head and the others hang under it. tail is what the
// head holds at its right end, which the text leaves room for. depth is the
// depth of the row and kids says whether it has children.
func (b *builder) message(prefix, body string, depth int, kids bool, tail string) []string {
	src := prepare(body)
	limit := b.width - 1 - guide*depth
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
	pad := hang
	if kids {
		col = max(hang, guide)
		pad = col - guide
	}
	room := markdown.Room(limit, col)
	lines := b.render(src, room)
	if len(lines) == 0 {
		return []string{prefix}
	}
	pads := strings.Repeat(" ", pad)
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
