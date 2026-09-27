package markdown

import "strings"

const esc = 0x1b

// tidy returns a line glamour rendered with what draws nothing left out:
// the spaces that pad it to the width, the style sequences that close a
// style only for the next to open it again, which glamour writes around
// every word, and those that only color spaces. A space shows its style
// only with a background, reverse video or a line through or under it.
// It drops the escape sequences that aren't styles too, such as
// hyperlinks, which the app doesn't use. What is left draws the same, in
// far fewer bytes, which every frame copies.
func tidy(line string) string {
	if strings.IndexByte(line, esc) < 0 {
		return strings.TrimRight(line, " ")
	}
	var b strings.Builder
	b.Grow(len(line))
	var (
		// style is the style that the text read next is drawn in: the
		// sequences read since the last reset. written is the style in
		// effect where b ends.
		style, written string
		shows          bool // written shows on spaces
		keep           int  // b up to the last cell worth drawing
		open           bool // written is not the default at keep
	)
	for i := 0; i < len(line); {
		if line[i] == esc {
			n, sgr := escape(line[i:])
			if sgr {
				seq := line[i : i+n]
				if resets(seq) {
					style = ""
				}
				if !isReset(seq) {
					style += seq
				}
			}
			i += n
			continue
		}
		j := strings.IndexByte(line[i:], esc)
		if j < 0 {
			j = len(line)
		} else {
			j += i
		}
		text := line[i:j]
		i = j
		if strings.Trim(text, " ") == "" && !showsSpace(style) {
			// Spaces look the same in any style that doesn't show on them.
			if shows {
				b.WriteString("\x1b[m")
				written, shows = "", false
			}
			b.WriteString(text)
			continue
		}
		if style != written {
			switch {
			case written != "" && strings.HasPrefix(style, written):
				b.WriteString(style[len(written):])
			case written != "":
				b.WriteString("\x1b[m")
				b.WriteString(style)
			default:
				b.WriteString(style)
			}
			written, shows = style, showsSpace(style)
		}
		start := b.Len()
		b.WriteString(text)
		if shows {
			keep, open = b.Len(), written != ""
		} else if t := strings.TrimRight(text, " "); t != "" {
			keep, open = start+len(t), written != ""
		}
	}
	out := b.String()[:keep]
	if open {
		out += "\x1b[m"
	}
	return out
}

// escape returns the length of the escape sequence at the start of s, and
// whether it sets a style.
func escape(s string) (n int, sgr bool) {
	if len(s) < 2 {
		return len(s), false
	}
	switch s[1] {
	case '[':
		j := 2
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x3f {
			j++
		}
		if j < len(s) && s[j] >= 0x40 && s[j] <= 0x7e {
			// Private forms such as ESC [ > 4 ; 2 m aren't styles.
			return j + 1, s[j] == 'm' && strings.Trim(s[2:j], "0123456789;:") == ""
		}
		return j, false
	case ']':
		for j := 2; j < len(s); j++ {
			switch s[j] {
			case 0x07:
				return j + 1, false
			case esc:
				if j+1 < len(s) && s[j+1] == '\\' {
					return j + 2, false
				}
				return j, false
			}
		}
		return len(s), false
	}
	return 2, false
}

// params returns the parameters of the style sequence seq.
func params(seq string) []string {
	return strings.FieldsFunc(seq[2:len(seq)-1], func(r rune) bool { return r == ';' })
}

// isReset reports whether seq only resets the style.
func isReset(seq string) bool {
	return seq == "\x1b[m" || seq == "\x1b[0m"
}

// resets reports whether seq starts with a reset, so the style before it
// no longer applies.
func resets(seq string) bool {
	p := params(seq)
	return len(p) == 0 || strings.TrimLeft(p[0], "0") == ""
}

// showsSpace reports whether style, a run of style sequences, leaves on
// what shows on a space: a background, reverse video, or a line under,
// through or over the text.
func showsSpace(style string) bool {
	var bg, reverse, under, through, over bool
	for style != "" {
		n, _ := escape(style)
		p := params(style[:n])
		style = style[n:]
		if len(p) == 0 {
			bg, reverse, under, through, over = false, false, false, false, false
		}
		for i := 0; i < len(p); i++ {
			switch c := strings.TrimLeft(p[i], "0"); c {
			case "":
				bg, reverse, under, through, over = false, false, false, false, false
			case "4", "21":
				under = true
			case "24":
				under = false
			case "7":
				reverse = true
			case "27":
				reverse = false
			case "9":
				through = true
			case "29":
				through = false
			case "53":
				over = true
			case "55":
				over = false
			case "49":
				bg = false
			case "38", "58":
				i += colorArgs(p[i+1:])
			case "48":
				bg = true
				i += colorArgs(p[i+1:])
			default:
				// 40 to 47 and 100 to 107 set a background color.
				if len(c) == 2 && c[0] == '4' && c[1] <= '7' || len(c) == 3 && c[:2] == "10" && c[2] <= '7' {
					bg = true
				}
			}
		}
	}
	return bg || reverse || under || through || over
}

// colorArgs returns how many of the parameters p after 38, 48 or 58 belong
// to the color.
func colorArgs(p []string) int {
	if len(p) == 0 {
		return 0
	}
	switch p[0] {
	case "5":
		return min(2, len(p))
	case "2":
		return min(4, len(p))
	}
	return 0
}
