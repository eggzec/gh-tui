package termtext

import "strings"

// Pen writes the escape sequences that change the style of a terminal
// from one [Style] to the next, each over a view's base style, in the
// fewest bytes: what changes, or a reset and the whole of the next style,
// whichever is shorter. The terminal ends up in the same style either
// way, so a view looks the same as one that writes the base style and the
// next in full each time, but a frame stays small whatever its content:
// even when every cell changes every color and attribute.
//
// The base style may set what a [Style] never holds, such as blink or
// conceal. The pen then never resets to write the whole of the next style
// in one sequence, which would drop it, but resets only to write the base
// style's own sequences again.
//
// The zero value has no base style and doesn't know what the terminal is
// in.
type Pen struct {
	// on is the base style's own sequences, and base what they set.
	on   string
	base sgrState
	// full is set when base holds all that on sets.
	full bool
	cur  sgrState
	// known is set while the terminal is in cur.
	known bool
}

// NewPen returns a pen that writes styles over base, the SGR sequences of
// a view's own style, which each style's colors replace and its
// attributes add to. It doesn't know what the terminal is in until
// [Pen.Start].
func NewPen(base string) Pen {
	p := Pen{on: base, full: true}
	for s := base; s != ""; {
		i := strings.IndexByte(s, '\x1b')
		if i < 0 {
			break
		}
		n, sgr := Escape(s[i:])
		if sgr {
			seq := s[i : i+n]
			if !p.base.apply(seq[2 : len(seq)-1]) {
				p.full = false
			}
		} else {
			p.full = false
		}
		s = s[i+max(n, 1):]
	}
	return p
}

// Start writes the base style to b, where the terminal is in its default
// style, as at the start of a row after a reset.
func (p *Pen) Start(b *strings.Builder) {
	b.WriteString(p.on)
	p.cur, p.known = p.base, true
}

// Lost tells the pen that something else set the style of the terminal,
// such as the style of a match.
func (p *Pen) Lost() {
	p.known = false
}

// Write writes to b what sets the terminal to seq, the Seq of a [Style],
// over the base style.
func (p *Pen) Write(b *strings.Builder, seq string) {
	var s sgrState
	if len(seq) > 3 {
		s.apply(seq[2 : len(seq)-1])
	}
	next := p.base
	next.attrs |= s.attrs
	if s.under != underNone {
		next.under = s.under
	}
	for _, c := range []struct{ to, from *string }{{&next.fg, &s.fg}, {&next.bg, &s.bg}, {&next.ul, &s.ul}} {
		if *c.from != "" {
			*c.to = *c.from
		}
	}
	if p.known && next == p.cur {
		return
	}
	b.WriteString(p.change(next))
	p.cur, p.known = next, true
}

// change returns what sets the terminal from p.cur, if known, to next,
// the shortest of: what changes; a reset, the base style and what next
// changes of it; and, when base holds the whole base style, a reset that
// the whole of next follows in the same sequence.
func (p *Pen) change(next sgrState) string {
	best := "\x1b[m" + p.on + p.base.diff(next)
	if p.full {
		if seq := next.seq(); seq != "" && len(seq)+2 < len(best) {
			best = "\x1b[0;" + seq[2:]
		}
	}
	if p.known {
		if diff := p.cur.diff(next); len(diff) < len(best) {
			best = diff
		}
	}
	return best
}

// attrOffs are the parameters that turn attributes off, and the
// attributes each turns off.
var attrOffs = []struct {
	attrs uint8
	param string
}{
	{attrBold | attrDim, "22"}, {attrItalic, "23"},
	{attrInverse, "27"}, {attrStrike, "29"}, {attrOverline, "55"},
}

// diff returns one SGR sequence that sets the terminal from s to next.
func (s sgrState) diff(next sgrState) string {
	var ps []string
	have := s.attrs
	for _, off := range attrOffs {
		if have&off.attrs&^next.attrs != 0 {
			// It turns off the others it covers too, which the next
			// loop turns back on.
			ps = append(ps, off.param)
			have &^= off.attrs
		}
	}
	for _, a := range attrParams {
		if next.attrs&a.attr != 0 && have&a.attr == 0 {
			ps = append(ps, a.param)
		}
	}
	if s.under != next.under {
		ps = append(ps, underParams[next.under])
	}
	for _, c := range []struct{ from, to, off string }{{s.fg, next.fg, "39"}, {s.bg, next.bg, "49"}, {s.ul, next.ul, "59"}} {
		switch {
		case c.from == c.to:
		case c.to == "":
			ps = append(ps, c.off)
		default:
			ps = append(ps, c.to)
		}
	}
	if len(ps) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(ps, ";") + "m"
}
