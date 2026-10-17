package conv

import (
	"path"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// snippetLines is the most lines of code a snippet shows.
const snippetLines = 4

// hunkHead reads the start of the old and the new side of a hunk from its
// header, such as "@@ -12,5 +14,7 @@".
var hunkHead = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// snippetStyles are what a snippet is drawn with: the colors of the diff
// view, for a line to look as it does there.
type snippetStyles struct {
	added, deleted, context, number, accent lipgloss.Style
	// bar goes before each line, and mark before the lines of the commented
	// range.
	bar, mark string
	// icon returns the icon of the file at a path and the space after it, or
	// "" for a set without file icons.
	icon func(path string) string
}

func newSnippetStyles(t ui.Theme, ic ui.Icons, set string) snippetStyles {
	d := t.Diff(ic)
	s := snippetStyles{
		added: d.Added, deleted: d.Deleted, context: d.Context, number: d.LineNumber,
		accent: lipgloss.NewStyle().Foreground(t.Accent.GetForeground()),
		bar:    "▏ ", mark: "▌",
		icon: func(p string) string {
			fi := ic.Entry(core.TreeEntry{Path: p, Name: path.Base(p)}, false)
			if fi.Glyph == "" {
				return ""
			}
			return t.FileIcon(fi).Render(fi.Glyph) + " "
		},
	}
	if set == config.IconsASCII {
		s.bar, s.mark = "| ", ">"
	}
	return s
}

// diffLine is a line of a hunk: its kind, '+', '-' or ' ', the numbers it has
// in the old and the new file, 0 for the side it isn't on, and its text.
type diffLine struct {
	kind     byte
	old, new int
	text     string
}

// hunkTail returns the last n lines of hunk, numbered, or nil for a hunk that
// has no header or no lines.
func hunkTail(hunk string, n int) []diffLine {
	lines := strings.Split(hunk, "\n")
	m := hunkHead.FindStringSubmatch(lines[0])
	if m == nil {
		return nil
	}
	old, cur := atoi(m[1]), atoi(m[2])
	var out []diffLine
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, `\`) {
			continue
		}
		d := diffLine{kind: ' ', text: l}
		if l != "" {
			d.kind, d.text = l[0], l[1:]
		}
		switch d.kind {
		case '+':
			d.new, cur = cur, cur+1
		case '-':
			d.old, old = old, old+1
		default:
			d.kind = ' '
			d.old, d.new = old, cur
			old, cur = old+1, cur+1
		}
		out = append(out, d)
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// withSnippet returns lines, those of the first comment of thread t, with the
// code the thread is on after the first. hunk is that comment's diff hunk. The
// snippet takes the column of the lines of the comment, which prefix, depth
// and kids give.
func (b *builder) withSnippet(lines []string, t *core.Thread, hunk, prefix string, depth int, kids bool) []string {
	if t.Resolved || strings.TrimSpace(hunk) == "" {
		return lines
	}
	col := b.column(prefix, depth, kids)
	snip := b.snippet(t, termtext.Clean(hunk, tabStop), col.room)
	if len(snip) == 0 {
		return lines
	}
	pads := strings.Repeat(" ", col.pad)
	out := make([]string, 0, len(lines)+len(snip))
	out = append(out, lines[0])
	for _, l := range snip {
		out = append(out, pads+l)
	}
	return append(out, lines[1:]...)
}

// snippet returns the lines that show the code thread t is on, a header and
// the last lines of hunk, each at most room cells wide. A thread on a file, or
// whose hunk has no lines, gets the header only.
func (b *builder) snippet(t *core.Thread, hunk string, room int) []string {
	s := &b.st.snip
	if t.Path == "" || room < 4 {
		return nil
	}
	// The hunk is numbered as it was made, so the thread's lines are the ones
	// it was made on, not those the code has moved to since.
	line, start := t.OriginalLine, t.OriginalStartLine
	if line == 0 {
		line, start = t.Line, t.StartLine
	}
	var rows []diffLine
	if !t.File && line > 0 {
		rows = hunkTail(hunk, snippetLines)
	}
	if start == 0 || start > line {
		start = line
	}
	bar := b.st.dim.Render(s.bar)
	out := []string{b.header(t, bar, line, start, len(rows) > 0, room)}

	w := 0
	for _, r := range rows {
		w = max(w, len(strconv.Itoa(max(r.old, r.new))))
	}
	first, last := markedRows(rows, t, line, start)
	for i, r := range rows {
		gutter := s.number.Render(num(r.old, w) + " " + num(r.new, w) + " ")
		mark := " "
		if i >= first && i <= last {
			mark = s.accent.Render(s.mark)
		}
		st := s.context
		switch r.kind {
		case '+':
			st = s.added
		case '-':
			st = s.deleted
		}
		l := bar + gutter + mark + st.Render(string(r.kind)+r.text)
		out = append(out, ansi.Truncate(l, room, b.st.Glyphs.Cut))
	}
	return out
}

// markedRows returns the first and last of rows that thread t is on, from the
// row of its start, on its start side, to that of its end, on its side. A start
// above rows marks from the first row, and an end not in rows marks none.
func markedRows(rows []diffLine, t *core.Thread, line, start int) (first, last int) {
	at := func(r diffLine, side core.Side) int {
		if side == core.SideLeft {
			return r.old
		}
		return r.new
	}
	startSide := t.StartSide
	if startSide == "" {
		startSide = t.Side
	}
	last = -1
	for i, r := range rows {
		if at(r, t.Side) == line {
			last = i
		}
	}
	if last < 0 || start == line && startSide == t.Side {
		return last, last
	}
	for i, r := range rows[:last] {
		if at(r, startSide) == start {
			return i, last
		}
	}
	return 0, last
}

// num returns n right-aligned in w cells, or blank for 0.
func num(n, w int) string {
	if n == 0 {
		return strings.Repeat(" ", w)
	}
	s := strconv.Itoa(n)
	return strings.Repeat(" ", w-len(s)) + s
}

// header returns the line over a snippet: the file and, when lines is set, the
// lines of the thread, with the mark of an outdated thread. A path that
// doesn't fit is cut at its start.
func (b *builder) header(t *core.Thread, bar string, line, start int, lines bool, room int) string {
	s := &b.st.snip
	name := termtext.OneLine(t.Path)
	icon := s.icon(name)
	var where string
	if lines {
		where = ":" + strconv.Itoa(line)
		if start != line {
			where = ":" + strconv.Itoa(start) + "-" + strconv.Itoa(line)
		}
	}
	tail := ""
	if t.Outdated {
		tail = " " + b.st.outdated()
	}
	fit := room - ansi.StringWidth(s.bar) - ansi.StringWidth(icon) - len(where) - ansi.StringWidth(tail)
	cut := b.st.Glyphs.Cut
	if w := ansi.StringWidth(name); w > fit && fit > ansi.StringWidth(cut) {
		name = ansi.TruncateLeft(name, w-fit+ansi.StringWidth(cut), cut)
	}
	return ansi.Truncate(bar+icon+b.st.dim.Render(name+where)+tail, room, cut)
}
