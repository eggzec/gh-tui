package termtext

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// terminal applies the SGR sequences of out to s, as a terminal would.
func terminal(s sgrState, out string) sgrState {
	for out != "" {
		i := strings.IndexByte(out, '\x1b')
		if i < 0 {
			break
		}
		n, _ := Escape(out[i:])
		seq := out[i : i+n]
		s.apply(seq[2 : len(seq)-1])
		out = out[i+n:]
	}
	return s
}

// randomStyle returns a style of some attributes and colors.
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
	return sgrState{attrs: uint16(r.IntN(1 << len(attrParams))), fg: pick("38"), bg: pick("48"), ul: pick("58")}
}

// TestPen changes the style of a terminal from any style to any other, over
// a base style, and leaves it in the style that the base and the next
// style, written in full after a reset, would.
func TestPen(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, base := range []string{"", "\x1b[38;2;200;206;219m", "\x1b[1;38;2;239;125;125m", "\x1b[3m\x1b[48;5;4m"} {
		pen := NewPen(base)
		var b strings.Builder
		pen.Start(&b)
		term := terminal(sgrState{}, b.String())
		for i := range 2000 {
			next := randomStyle(r)
			seq := next.seq()
			if i%100 == 99 {
				// Something else styled the terminal.
				pen.Lost()
				term = randomStyle(r)
			}
			b.Reset()
			pen.Write(&b, seq)
			want := terminal(terminal(sgrState{}, base), seq)
			if term = terminal(term, b.String()); term != want {
				t.Fatalf("base %q, to %q: wrote %q, left %+v, want %+v", base, seq, b.String(), term, want)
			}
		}
	}
}
