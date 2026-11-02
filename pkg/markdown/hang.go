package markdown

import (
	"cmp"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

var ordinalMarker = regexp.MustCompile(`^\d+\. `)

// hang returns lines, glamour's render at width, with the lines of each
// list item that wrapped moved under the item's text, as "* text" is
// continued by "  more". Glamour writes an item's marker once and wraps its
// paragraph on its own, so the rest of the item starts at the list's edge.
// The item's lines are wrapped again in the room the marker leaves, so none
// grows past width. Lines of code, pictures and heads, which are marks that
// splice replaces, and of tables, are left as they are.
func (r *Renderer) hang(lines []string, width int) []string {
	g := r.glyphs
	bullet := cmp.Or(g.Bullet, "•")
	tick := cmp.Or(g.Tick, "✓")
	column := cmp.Or(g.TableColumn, "│")
	// A line of a table or a mark stays as it is and ends the item.
	plain := func(body string) bool {
		s := xansi.Strip(body)
		return strings.Contains(s, r.nonce) || strings.Contains(s, column)
	}
	marker := func(body string) int {
		s := xansi.Strip(body)
		switch {
		case strings.HasPrefix(s, bullet+" "):
			return xansi.StringWidth(bullet + " ")
		case strings.HasPrefix(s, "[ ] "), strings.HasPrefix(s, "["+tick+"] "):
			// A task item has a box in place of the bullet.
			return xansi.StringWidth("[" + tick + "] ")
		case ordinalMarker.MatchString(s):
			return len(ordinalMarker.FindString(s))
		}
		return 0
	}
	type split struct{ head, body string }
	cut := func(line string) split {
		quote := g.quoteToken()
		end, i := 0, 0
	scan:
		for i < len(line) {
			switch {
			case line[i] == esc:
				n, _ := escape(line[i:])
				i += n
			case line[i] == ' ':
				i++
				end = i
			case strings.HasPrefix(line[i:], quote):
				i += len(quote)
				end = i
			default:
				break scan
			}
		}
		return split{head: line[:end], body: line[end:]}
	}
	blank := func(s string) bool { return strings.TrimSpace(xansi.Strip(s)) == "" }

	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		t := tidy(lines[i])
		first := cut(t)
		mw := marker(first.body)
		if mw == 0 || plain(first.body) {
			out = append(out, lines[i])
			continue
		}
		headText := xansi.Strip(first.head)
		group := []split{first}
		j := i + 1
		for ; j < len(lines); j++ {
			s := cut(tidy(lines[j]))
			if blank(s.body) || marker(s.body) > 0 || xansi.Strip(s.head) != headText {
				break
			}
			if plain(s.body) {
				break
			}
			group = append(group, s)
		}
		if len(group) == 1 {
			out = append(out, lines[i])
			continue
		}
		headW := xansi.StringWidth(first.head)
		room := max(width-headW-mw, 1)
		// Lines glamour wrapped belong to one paragraph; those that end
		// where the text did, the source's own breaks, stay apart.
		var units []string
		text := xansi.TruncateLeft(first.body, mw, "")
		for k := 1; k < len(group); k++ {
			prev := headW + xansi.StringWidth(group[k-1].body)
			word := xansi.Strip(group[k].body)
			if n := strings.IndexAny(word, " ,.;-+|"); n >= 0 {
				word = word[:n+1]
			}
			if prev+1+xansi.StringWidth(word) > width {
				text += " " + group[k].body
				continue
			}
			units = append(units, text)
			text = group[k].body
		}
		units = append(units, text)
		pad := strings.Repeat(" ", mw)
		lead := first.head + xansi.Truncate(first.body, mw, "")
		for _, u := range units {
			for l := range strings.SplitSeq(lipgloss.Wrap(u, room, ""), "\n") {
				out = append(out, lead+l)
				lead = first.head + pad
			}
		}
		i = j - 1
	}
	return out
}
