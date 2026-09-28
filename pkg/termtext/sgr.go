package termtext

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Style is the style of text from byte Pos of it on, as the SGR sequences
// before it left it: Seq is one SGR sequence that sets that style from
// the default, or "" for the default. Seq holds only what colors and
// weighs text, never blink or conceal, and it is short whatever the text
// held, so a view can write it at the start of every row it draws.
type Style struct {
	Pos int
	Seq string
}

// Styler folds the SGR sequences of text into its [Style]s. The zero
// value starts from the default style.
type Styler struct {
	state sgrState
	list  []Style
	// seqs holds the sequence of each state met, so a state that comes
	// back costs no new string.
	seqs map[sgrState]string
}

// Grow makes room for n more styles.
func (s *Styler) Grow(n int) {
	s.list = slices.Grow(s.list, n)
}

// Add folds seq, an SGR sequence as [Escape] finds one, into the style
// of the text from byte pos on. A style is added only where the style
// changes, and one at most for each pos.
func (s *Styler) Add(pos int, seq string) {
	s.state.apply(seq[2 : len(seq)-1])
	next := s.seq()
	n := len(s.list)
	if n > 0 && s.list[n-1].Pos == pos {
		s.list = s.list[:n-1]
		n--
	}
	prev := ""
	if n > 0 {
		prev = s.list[n-1].Seq
	}
	if next != prev {
		s.list = append(s.list, Style{Pos: pos, Seq: next})
	}
}

// Styles returns the styles added, in the order of their positions.
func (s *Styler) Styles() []Style { return s.list }

// seq returns the sequence that sets the current style.
func (s *Styler) seq() string {
	if q, ok := s.seqs[s.state]; ok {
		return q
	}
	if s.seqs == nil {
		s.seqs = make(map[sgrState]string)
	}
	q := s.state.seq()
	s.seqs[s.state] = q
	return q
}

// Attributes of an SGR style, with the parameters that set them.
const (
	attrBold uint16 = 1 << iota
	attrDim
	attrItalic
	attrUnderline
	attrDoubleUnderline
	attrInverse
	attrStrike
	attrOverline
)

var attrParams = []struct {
	attr  uint16
	param string
}{
	{attrBold, "1"}, {attrDim, "2"}, {attrItalic, "3"}, {attrUnderline, "4"},
	{attrDoubleUnderline, "21"}, {attrInverse, "7"}, {attrStrike, "9"}, {attrOverline, "53"},
}

// sgrState is the style SGR sequences set, as far as it colors and weighs
// text: its attributes, and the parameters of its foreground, background
// and underline colors, such as "31" or "38;5;208", or "" for the default.
type sgrState struct {
	attrs      uint16
	fg, bg, ul string
}

// apply folds the parameters of an SGR sequence into s. Those that don't
// color or weigh text are dropped, blink and conceal too, since text that
// hides or flashes can make a file read other than it is.
func (s *sgrState) apply(params string) {
	// The parameters, and where each starts in params, split without an
	// allocation.
	var (
		buf    [maxSGRParams]string
		starts [maxSGRParams]int
	)
	ps := buf[:0]
	for off := 0; ; {
		if len(ps) == maxSGRParams {
			// Escape doesn't let such a sequence through.
			return
		}
		p, _, more := strings.Cut(params[off:], ";")
		starts[len(ps)] = off
		ps = append(ps, p)
		if !more {
			break
		}
		off += len(p) + 1
	}
	for i := 0; i < len(ps); i++ {
		head, sub, colon := strings.Cut(ps[i], ":")
		switch v := strings.TrimLeft(head, "0"); v {
		case "":
			*s = sgrState{}
		case "1":
			s.attrs |= attrBold
		case "2":
			s.attrs |= attrDim
		case "3":
			s.attrs |= attrItalic
		case "4":
			// 4:0 turns underline off; the other styles of 4:n are all
			// underline here.
			if colon && strings.TrimLeft(sub, "0") == "" {
				s.attrs &^= attrUnderline | attrDoubleUnderline
			} else {
				s.attrs |= attrUnderline
			}
		case "7":
			s.attrs |= attrInverse
		case "9":
			s.attrs |= attrStrike
		case "21":
			s.attrs |= attrDoubleUnderline
		case "22":
			s.attrs &^= attrBold | attrDim
		case "23":
			s.attrs &^= attrItalic
		case "24":
			s.attrs &^= attrUnderline | attrDoubleUnderline
		case "27":
			s.attrs &^= attrInverse
		case "29":
			s.attrs &^= attrStrike
		case "53":
			s.attrs |= attrOverline
		case "55":
			s.attrs &^= attrOverline
		case "39":
			s.fg = ""
		case "49":
			s.bg = ""
		case "59":
			s.ul = ""
		case "38", "48", "58":
			var args []string
			if colon {
				args = strings.Split(sub, ":")
			} else {
				n := colorArgs(ps[i+1:])
				if n < 0 {
					// The rest can't be read apart from the color.
					return
				}
				args = ps[i+1 : i+1+n]
				i += n
			}
			c, ok := color(v, args)
			if !ok {
				// A color no terminal reads is dropped, whatever it was
				// before.
				continue
			}
			switch v {
			case "38":
				s.fg = c
			case "48":
				s.bg = c
			default:
				s.ul = c
			}
		default:
			switch len(v) {
			case 2:
				if v[1] >= '0' && v[1] <= '7' {
					switch v[0] {
					case '3', '9':
						s.fg = v
					case '4':
						s.bg = v
					}
				}
			case 3:
				if v[:2] == "10" && v[2] <= '7' {
					s.bg = v
				}
			}
		}
	}
}

// color returns the parameters that set the color of kind, 38, 48 or 58,
// that args, what follows kind in the sequence, name, in their shortest
// form: 5;n for one of 256 colors and 2;r;g;b for a true color, in
// decimal, so no color keeps the padding or the colon form it came in and
// every style stays short. The colon form of a true color may name a
// color space before r, which is dropped. It reports false for args that
// name no color.
func color(kind string, args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	var nums []string
	switch strings.TrimLeft(args[0], "0") {
	case "5":
		nums = args[1:]
		if len(nums) != 1 {
			return "", false
		}
	case "2":
		nums = args[1:]
		if len(nums) == 4 {
			// The color space of the colon form.
			nums = nums[1:]
		}
		if len(nums) != 3 {
			return "", false
		}
	default:
		return "", false
	}
	var b strings.Builder
	b.WriteString(kind + ";" + strings.TrimLeft(args[0], "0"))
	for _, n := range nums {
		v, ok := byteValue(n)
		if !ok {
			return "", false
		}
		b.WriteString(";" + v)
	}
	return b.String(), true
}

// byteValue returns n, a decimal of 0 to 255, without its leading zeros,
// with "" for 0, as the colon form may leave it.
func byteValue(n string) (string, bool) {
	v := strings.TrimLeft(n, "0")
	if v == "" {
		return "0", true
	}
	if len(v) > 3 || strings.Trim(v, "0123456789") != "" || len(v) == 3 && v > "255" {
		return "", false
	}
	return v, true
}

// colorArgs returns how many of args, the parameters after 38, 48 or 58,
// are the color's: 2 for 5;n and 4 for 2;r;g;b, or -1 if they are neither.
func colorArgs(args []string) int {
	if len(args) == 0 {
		return -1
	}
	switch strings.TrimLeft(args[0], "0") {
	case "5":
		if len(args) >= 2 {
			return 2
		}
	case "2":
		if len(args) >= 4 {
			return 4
		}
	}
	return -1
}

// seq returns one SGR sequence that sets s from the default, or "" for the
// default.
func (s sgrState) seq() string {
	var ps []string
	for _, a := range attrParams {
		if s.attrs&a.attr != 0 {
			ps = append(ps, a.param)
		}
	}
	for _, c := range []string{s.fg, s.bg, s.ul} {
		if c != "" {
			ps = append(ps, c)
		}
	}
	if len(ps) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(ps, ";") + "m"
}

// maxSGR and maxSGRParams are how long the parameters of an SGR sequence
// and how many they are at most. Longer ones are dropped: no style needs
// them, and each would cost every row it colors.
const (
	maxSGR       = 64
	maxSGRParams = 32
)

// Escape returns the length of the escape sequence at the start of s,
// which starts with ESC, and whether it is an SGR sequence to keep: one
// of at most maxSGR bytes of parameters and maxSGRParams of them. A sequence cut
// short ends where it stops being valid, so what follows is text; a string
// sequence, such as a title (OSC), ends at a newline too, so one never
// ended can't hide the lines after it.
func Escape(s string) (n int, sgr bool) {
	if len(s) < 2 {
		return len(s), false
	}
	switch s[1] {
	case '[':
		j := 2
		for j < len(s) && s[j] >= 0x30 && s[j] <= 0x3f {
			j++
		}
		params := s[2:j]
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
			j++
		}
		if j < len(s) && s[j] >= 0x40 && s[j] <= 0x7e {
			return j + 1, s[j] == 'm' && j == 2+len(params) && sgrParams(params)
		}
		return j, false
	case ']', 'P', 'X', '^', '_':
		// OSC, DCS, SOS, PM and APC hold a string up to BEL or ST.
		for j := 2; j < len(s); j++ {
			switch s[j] {
			case ansi.BEL:
				return j + 1, false
			case '\n':
				return j, false
			case ansi.ESC:
				if j+1 < len(s) && s[j+1] == '\\' {
					return j + 2, false
				}
				return j, false
			}
		}
		return len(s), false
	}
	j := 1
	for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
		j++
	}
	if j < len(s) && s[j] >= 0x30 && s[j] <= 0x7e {
		return j + 1, false
	}
	return j, false
}

// sgrParams reports whether p holds only the digits and separators of SGR
// parameters, and no private markers, and no more of them than to keep.
func sgrParams(p string) bool {
	if len(p) > maxSGR || strings.Count(p, ";") >= maxSGRParams {
		return false
	}
	for i := range len(p) {
		if c := p[i]; (c < '0' || c > '9') && c != ';' && c != ':' {
			return false
		}
	}
	return true
}

// HasSGR reports whether s holds an SGR sequence, such as the colors of
// a program's output kept in a file.
func HasSGR(s string) bool {
	for {
		i := strings.IndexByte(s, ansi.ESC)
		if i < 0 {
			return false
		}
		n, sgr := Escape(s[i:])
		if sgr {
			return true
		}
		s = s[i+max(n, 1):]
	}
}
