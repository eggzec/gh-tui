package termtext

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// refTerm is a terminal's style as a terminal keeps it, written apart
// from sgrState so the pen is checked against something it doesn't share:
// one underline setting, which 4, 4:n and 21 each overwrite; bold and dim
// as two flags that 22 clears together; the other attributes as flags;
// and the colors as the parameters that set them.
type refTerm struct {
	bold, dim, italic, blink, inverse, conceal, strike, overline bool
	// underline is 0 for none, else the n of 4:n, 21 being 2.
	underline  int
	fg, bg, ul string
}

// write applies the SGR sequences of out to t, and fails on any other
// escape sequence, which the pen never writes.
func (t *refTerm) write(tb testing.TB, out string) {
	tb.Helper()
	for out != "" {
		i := strings.IndexByte(out, '\x1b')
		if i < 0 {
			return
		}
		rest := out[i:]
		end := strings.IndexByte(rest, 'm')
		if !strings.HasPrefix(rest, "\x1b[") || end < 0 {
			tb.Fatalf("not an SGR sequence: %q", rest)
		}
		t.sgr(tb, strings.Split(rest[2:end], ";"))
		out = rest[end+1:]
	}
}

func (t *refTerm) sgr(tb testing.TB, ps []string) {
	tb.Helper()
	for i := 0; i < len(ps); i++ {
		p := ps[i]
		switch p {
		case "", "0":
			*t = refTerm{}
		case "1":
			t.bold = true
		case "2":
			t.dim = true
		case "3":
			t.italic = true
		case "4", "4:1":
			t.underline = 1
		case "4:0", "24":
			t.underline = 0
		case "4:2", "21":
			t.underline = 2
		case "4:3", "4:4", "4:5":
			t.underline = int(p[2] - '0')
		case "5":
			t.blink = true
		case "7":
			t.inverse = true
		case "8":
			t.conceal = true
		case "9":
			t.strike = true
		case "22":
			t.bold, t.dim = false, false
		case "23":
			t.italic = false
		case "25":
			t.blink = false
		case "27":
			t.inverse = false
		case "28":
			t.conceal = false
		case "29":
			t.strike = false
		case "53":
			t.overline = true
		case "55":
			t.overline = false
		case "39":
			t.fg = ""
		case "49":
			t.bg = ""
		case "59":
			t.ul = ""
		case "38", "48", "58":
			n := 3
			if i+1 < len(ps) && ps[i+1] == "2" {
				n = 5
			}
			if i+n > len(ps) {
				tb.Fatalf("color cut short: %q", ps)
			}
			c := strings.Join(ps[i:i+n], ";")
			switch p {
			case "38":
				t.fg = c
			case "48":
				t.bg = c
			default:
				t.ul = c
			}
			i += n - 1
		default:
			switch {
			case len(p) == 2 && (p[0] == '3' || p[0] == '9') && p[1] <= '7':
				t.fg = p
			case len(p) == 2 && p[0] == '4' && p[1] <= '7', len(p) == 3 && p[:2] == "10":
				t.bg = p
			default:
				tb.Fatalf("parameter %q the reference doesn't know", p)
			}
		}
	}
}

// randomStyle returns a style of some attributes, underline and colors.
func randomStyle(r *rand.Rand) sgrState {
	colors := []string{"", "31", "38;5;208", "38;2;1;2;3", "38;2;255;255;255"}
	pick := func(kind string) string {
		c := colors[r.IntN(len(colors))]
		switch {
		case c == "" || kind == "38":
			return c
		case len(c) == 2:
			return "4" + c[1:]
		default:
			return kind + c[2:]
		}
	}
	return sgrState{
		attrs: uint8(r.IntN(1 << len(attrParams))),
		under: uint8(r.IntN(len(underParams))),
		fg:    pick("38"), bg: pick("48"), ul: pick("58"),
	}
}

// TestPen changes the style of a terminal from any style to any other, over
// a base style, and leaves it in the style that a reset, the base and the
// next style written in full would, by a reference terminal.
func TestPen(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, base := range []string{
		"", "\x1b[38;2;200;206;219m", "\x1b[1;38;2;239;125;125m", "\x1b[3m\x1b[48;5;4m",
		// What a style never holds: blink, conceal and a curly underline
		// that a style's own underline replaces.
		"\x1b[5;31m", "\x1b[8m", "\x1b[4:3;58;5;1m", "\x1b[21m",
	} {
		pen := NewPen(base)
		var b strings.Builder
		pen.Start(&b)
		var term refTerm
		term.write(t, b.String())
		for i := range 3000 {
			next := randomStyle(r)
			seq := next.seq()
			if i%100 == 99 {
				// Something else styled the terminal.
				pen.Lost()
				term.write(t, "\x1b[5;8m"+randomStyle(r).seq())
			}
			b.Reset()
			pen.Write(&b, seq)
			var want refTerm
			want.write(t, base+seq)
			term.write(t, b.String())
			if term != want {
				t.Fatalf("base %q, to %q: wrote %q, left %+v, want %+v", base, seq, b.String(), term, want)
			}
		}
	}
}

// Going from a double underline to a style that also asks for a single
// one leaves the one a terminal ends up with when written in full: the
// last, double.
func TestPenUnderlineIsOneSetting(t *testing.T) {
	pen := NewPen("")
	var b strings.Builder
	pen.Start(&b)
	pen.Write(&b, "\x1b[21m")
	b.Reset()
	pen.Write(&b, "\x1b[4;21m")
	if b.String() != "" {
		t.Errorf("wrote %q for the same double underline, want nothing", b.String())
	}
	b.Reset()
	pen.Write(&b, "\x1b[21;4m")
	if b.String() != "\x1b[4m" {
		t.Errorf("wrote %q to go to a single underline, want %q", b.String(), "\x1b[4m")
	}
}

// A base style the pen can't hold whole is never dropped by a reset that
// writes only the next style.
func TestPenKeepsBaseItCantHold(t *testing.T) {
	for _, base := range []string{"\x1b[5m", "\x1b[8;1m", "\x1b[4:9m"} {
		pen := NewPen(base)
		var b strings.Builder
		pen.Start(&b)
		pen.Lost()
		b.Reset()
		pen.Write(&b, "\x1b[1;3;7;9;53;38;2;1;2;3;48;2;4;5;6m")
		if !strings.HasPrefix(b.String(), "\x1b[m"+base) {
			t.Errorf("base %q: wrote %q, which drops the base", base, b.String())
		}
	}
}
