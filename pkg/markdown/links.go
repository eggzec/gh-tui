package markdown

import (
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// The links of a render pass through the lines, while they are tidied and
// made safe, as characters of a private-use plane, since both drop every
// hyperlink: linkOpen+i opens the i-th link of the render, linkClose ends
// it, and goneMark stands where the address glamour prints after a link's
// text was. They are one cell wide, no control and no markdown, and
// relink turns each pair into a hyperlink once the line is safe.
const (
	goneMark  = '\U000F1FFE'
	linkClose = '\U000F1FFF'
	linkOpen  = '\U000F2000'
	maxLinks  = 0x1000
)

// isLinkRune reports whether r is one of the characters above.
func isLinkRune(r rune) bool { return r >= goneMark && r < linkOpen+maxLinks }

// scrubLinks returns s without the characters of links, which become
// U+FFFD, so no text can pass for a link's.
func scrubLinks(s string) string {
	if !strings.ContainsFunc(s, isLinkRune) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isLinkRune(r) {
			return unicode.ReplacementChar
		}
		return r
	}, s)
}

// span is a hyperlink in the output of glamour: out[start:end] is its
// escape sequences and what they enclose, inner.
type span struct {
	start, end int
	url, inner string
}

// osc8 returns the length of the hyperlink sequence at the start of s,
// ESC ] 8 ; params ; uri and its terminator, and its uri.
func osc8(s string) (n int, uri string, ok bool) {
	if !strings.HasPrefix(s, "\x1b]8;") {
		return 0, "", false
	}
	for j := 4; j < len(s); j++ {
		switch s[j] {
		case 0x07:
			n = j + 1
		case esc:
			if j+1 >= len(s) || s[j+1] != '\\' {
				return 0, "", false
			}
			n = j + 2
		default:
			continue
		}
		_, uri, ok = strings.Cut(s[4:j], ";")
		return n, uri, ok
	}
	return 0, "", false
}

// findLinks returns the hyperlinks of out, in order. Glamour writes each as an
// opening sequence, its text and a closing one.
func findLinks(out string) []span {
	var all []span
	for i := 0; i < len(out); {
		j := strings.Index(out[i:], "\x1b]8;")
		if j < 0 {
			break
		}
		start := i + j
		n, uri, ok := osc8(out[start:])
		if !ok {
			i = start + 1
			continue
		}
		if uri == "" {
			i = start + n
			continue
		}
		from := start + n
		for k := from; ; {
			m := strings.Index(out[k:], "\x1b]8;")
			if m < 0 {
				return all
			}
			cn, curi, cok := osc8(out[k+m:])
			if !cok {
				k += m + 1
				continue
			}
			if curi != "" {
				// Not nested: the link's end is missing.
				k += m + cn
				continue
			}
			end := k + m
			all = append(all, span{start: start, end: end + cn, url: uri, inner: out[from:end]})
			i = end + cn
			break
		}
	}
	return all
}

// bare returns s as what it shows with no space, quote token or style.
func (r *Renderer) bare(s string) string {
	s = strings.ReplaceAll(xansi.Strip(s), r.glyphs.quoteToken(), "")
	return strings.Map(func(c rune) rune {
		if unicode.IsSpace(c) {
			return -1
		}
		return c
	}, s)
}

// target returns the address a link of the source to addr goes to: addr,
// or what the resolver makes of it, if it is relative.
func (r *Renderer) target(addr string) string {
	if r.resolve == nil {
		return addr
	}
	if u, err := url.Parse(addr); err != nil || u.Scheme != "" || u.Host != "" {
		return addr
	}
	return r.resolve(addr)
}

// linkable reports whether termtext.Link would link to addr.
func linkable(addr string) bool {
	return termtext.Link(addr, "x") != "x"
}

// swapLinks returns out, what glamour rendered, with each link that may be
// one as characters of links, and the address it printed after its text
// gone, since the text is the link. Those that can't be, and a render with
// links off, stay as they were, with their address shown. It sets r.urls
// to the addresses of the links, by their number.
func (r *Renderer) swapLinks(out string) string {
	r.urls = r.urls[:0]
	out = scrubLinks(out)
	if !termtext.LinksOn() {
		return out
	}
	all := findLinks(out)
	if len(all) == 0 {
		return out
	}
	var b strings.Builder
	pos := 0
	for k := 0; k < len(all); k++ {
		sp := all[k]
		addr := r.target(sp.url)
		if len(r.urls) >= maxLinks || !linkable(addr) {
			continue
		}
		b.WriteString(out[pos:sp.start])
		pos = sp.end
		b.WriteString(r.stand(addr, sp.inner))
		if n := r.printed(out, all[k:]); n > 0 {
			prev := sp
			for _, next := range all[k+1 : k+1+n] {
				gap := out[prev.end:next.start]
				if prev == sp && !strings.Contains(gap, "\n") {
					gap = strings.Replace(gap, " ", "", 1)
				}
				b.WriteString(gap)
				b.WriteRune(goneMark)
				prev = next
			}
			pos = prev.end
			k += n
		}
	}
	b.WriteString(out[pos:])
	return b.String()
}

// printed returns how many of the spans after the first of spans are the
// address glamour printed after its text: the same address, shown as it is,
// with only space between. A long address wrapped over lines is one span
// per line. It returns 0 if they aren't.
func (r *Renderer) printed(out string, spans []span) int {
	sp := spans[0]
	want := r.bare(sp.url)
	var got strings.Builder
	for i := 1; i < len(spans); i++ {
		next := spans[i]
		if next.url != sp.url || r.bare(out[spans[i-1].end:next.start]) != "" {
			return 0
		}
		got.WriteString(r.bare(next.inner))
		if got.String() == want {
			return i
		}
		if !strings.HasPrefix(want, got.String()) {
			return 0
		}
	}
	return 0
}

// stand returns inner, the text of a link to addr, within the characters
// of a link of its own on each of its lines.
func (r *Renderer) stand(addr, inner string) string {
	i := len(r.urls)
	r.urls = append(r.urls, addr)
	open := string(linkOpen + rune(i))
	lines := strings.Split(inner, "\n")
	for j, l := range lines {
		if r.bare(l) != "" {
			lines[j] = open + l + string(linkClose)
		}
	}
	return strings.Join(lines, "\n")
}

// dropGone returns lines without those left with nothing by the address
// that was on them, and the mark of the addresses gone from the rest.
func (r *Renderer) dropGone(lines []string) []string {
	kept := lines[:0]
	for _, l := range lines {
		if i := strings.IndexRune(l, goneMark); i >= 0 {
			if r.bare(l[:i]) == "" {
				// The address began the line, so the space that
				// followed it would indent the rest.
				last := strings.LastIndex(l, string(goneMark)) + utf8.RuneLen(goneMark)
				l = l[:last] + dropSpace(l[last:])
			}
			l = strings.ReplaceAll(l, string(goneMark), "")
			if r.bare(l) == "" {
				continue
			}
		}
		kept = append(kept, l)
	}
	return kept
}

// dropSpace returns s without its first space, if only escape sequences
// come before it.
func dropSpace(s string) string {
	for i := 0; i < len(s); {
		switch {
		case s[i] == ' ':
			return s[:i] + s[i+1:]
		case s[i] == esc && i+1 < len(s) && s[i+1] == '[':
			j := i + 2
			for j < len(s) && (s[j] < '@' || s[j] > '~') {
				j++
			}
			i = j + 1
		default:
			return s
		}
	}
	return s
}

// underlined returns the text of a link drawn underlined, as the address
// it stands for is: the underline is set again after each reset in it, and
// ends with it.
func underlined(inner string) string {
	const on = "\x1b[4m"
	inner = strings.ReplaceAll(inner, "\x1b[m", "\x1b[m"+on)
	inner = strings.ReplaceAll(inner, "\x1b[0m", "\x1b[0m"+on)
	return on + strings.TrimSuffix(inner, on) + "\x1b[24m"
}

// relink returns line, made safe, with the characters of each link of the
// render as a hyperlink.
func (r *Renderer) relink(line string) string {
	if !strings.ContainsFunc(line, isLinkRune) {
		return line
	}
	var b strings.Builder
	for line != "" {
		i := strings.IndexFunc(line, func(c rune) bool { return c >= linkOpen && c < linkOpen+maxLinks })
		if i < 0 {
			break
		}
		b.WriteString(line[:i])
		c, _ := utf8.DecodeRuneInString(line[i:])
		line = line[i+4:]
		inner := line
		rest := ""
		if j := strings.IndexRune(line, linkClose); j >= 0 {
			inner, rest = line[:j], line[j+4:]
		}
		line = rest
		if n := int(c - linkOpen); n < len(r.urls) {
			b.WriteString(termtext.Link(r.urls[n], underlined(inner)))
		} else {
			b.WriteString(inner)
		}
	}
	b.WriteString(line)
	return b.String()
}
