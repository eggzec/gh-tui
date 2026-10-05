package markdown

import (
	"regexp"
	"strings"
)

// Resolve returns src with the address of each link and image in it
// replaced by what fn returns for it, told whether it is an image's: the
// addresses of inline links and images, of link reference definitions,
// and the src and href of HTML tags, a tag over several lines too. What
// fn returns goes in as it is, so it must not hold spaces or parentheses
// that would end the address. Code is left as it is: fenced, in quotes
// too, indented, in backticks, or in pre or code tags, and so are HTML
// comments. It lets a caller that knows what the relative addresses of a
// document are relative to, such as a README of a repository, make them
// absolute before rendering.
func Resolve(src string, fn func(addr string, image bool) string) string {
	if !strings.ContainsAny(src, "]=") {
		return src
	}
	r := resolver{blank: true}
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !r.text(line) {
			out = append(out, line)
			continue
		}
		// A tag left open takes the lines up to its end, within reason.
		n := 1
		for n < maxTagLines && i+n < len(lines) && openTag(strings.Join(lines[i:i+n], "\n")) && strings.TrimSpace(lines[i+n]) != "" {
			n++
		}
		chunk := strings.Join(lines[i:i+n], "\n")
		if m := refDef.FindStringSubmatchIndex(chunk); n == 1 && m != nil {
			from, to := m[4], m[5]
			if from < 0 {
				from, to = m[6], m[7]
			}
			out = append(out, chunk[:from]+fn(chunk[from:to], false)+chunk[to:])
			continue
		}
		out = append(out, strings.Split(r.html.outside(chunk, func(s string) string {
			return outsideCode(s, func(s string) string { return resolveTags(resolveInline(s, fn), fn) })
		}), "\n")...)
		i += n - 1
	}
	return strings.Join(out, "\n")
}

// maxTagLines is how many lines an HTML tag may span for its addresses to
// be resolved.
const maxTagLines = 8

// resolver is where Resolve is in a document: in a fenced or an indented
// code block, or in the HTML that holds code or a comment.
type resolver struct {
	fence *fence
	html  htmlCode
	// blank reports that the line before was blank, code that an
	// indented block goes on, and list that the last line indented less
	// than code started a list item, whose indented lines are its text.
	blank, code, list bool
}

// text reports whether line is text whose addresses resolve, and keeps
// what it says of the lines after.
func (r *resolver) text(line string) bool {
	bare := quoteMarks.ReplaceAllString(line, "")
	switch {
	case r.fence != nil:
		if r.fence.closedBy(bare) {
			r.fence = nil
		}
		return false
	case r.html.in == "":
		if f, ok := openFence(bare); ok {
			r.fence = &f
			r.blank = false
			return false
		}
	}
	blank := strings.TrimSpace(line) == ""
	indent := indentOf(line)
	code := r.html.in == "" && !blank && indent >= 4 && (r.blank || r.code) && !r.list
	r.code = code || r.code && blank
	if !blank && indent < 4 {
		r.list = listItem.MatchString(line)
	}
	r.blank = blank
	return !code
}

// indentOf returns how many columns line is indented, a tab taking four.
func indentOf(line string) int {
	n := 0
	for _, c := range line {
		switch c {
		case ' ':
			n++
		case '\t':
			n += 4 - n%4
		default:
			return n
		}
	}
	return n
}

var (
	// quoteMarks matches the marks of the quotes a line is in.
	quoteMarks = regexp.MustCompile(`^(?: {0,3}> ?)+`)
	// listItem matches a line that starts an item of a list.
	listItem = regexp.MustCompile(`^ {0,3}(?:[-*+]|\d{1,9}[.)])(?:[ \t]|$)`)
	// refDef matches a link reference definition, but not a footnote's:
	// its address, in the second group or, without angle brackets, the
	// third, then at most a title before the end of the line.
	refDef = regexp.MustCompile(`^ {0,3}\[([^\]^][^\]]*)\]:[ \t]*(?:<([^>\n]*)>|([^\s<]+))(?:[ \t]+(?:"[^"]*"|'[^']*'|\([^)]*\)))?[ \t]*$`)
	// tagAddr matches the src or href of an HTML tag, its address in the
	// third, fourth or fifth group: in either quotes or none. An attribute
	// that only ends in src, such as data-src, isn't one.
	tagAddr = regexp.MustCompile(`(?i)(<[a-z][a-z0-9]*\b[^>]*?\s)(src|href)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>` + "`" + `]+))`)
	// tagStart matches the start of an HTML tag.
	tagStart = regexp.MustCompile(`<[a-zA-Z]`)
)

// openTag reports whether text ends within an HTML tag, which goes on on
// the next line.
func openTag(line string) bool {
	locs := tagStart.FindAllStringIndex(line, -1)
	if len(locs) == 0 {
		return false
	}
	return !strings.Contains(line[locs[len(locs)-1][0]:], ">")
}

// htmlCode is the HTML that Resolve leaves as it is, which may span lines:
// in names the tag or comment it is in, by what ends it, or is "".
type htmlCode struct {
	in string
}

// htmlOpeners are the starts of the HTML that holds code or a comment, and
// what ends each.
var htmlOpeners = [][2]string{{"<!--", "-->"}, {"<pre", "</pre>"}, {"<code", "</code>"}}

// outside returns s with f applied to what isn't in the HTML that holds
// code or a comment, and keeps where that ends for the lines after.
func (h *htmlCode) outside(s string, f func(string) string) string {
	var b strings.Builder
	lower := strings.ToLower(s)
	at := 0
	for at < len(s) {
		if h.in != "" {
			end := strings.Index(lower[at:], h.in)
			if end < 0 {
				b.WriteString(s[at:])
				return b.String()
			}
			end += at + len(h.in)
			b.WriteString(s[at:end])
			at, h.in = end, ""
			continue
		}
		next, closer := -1, ""
		for _, o := range htmlOpeners {
			k := strings.Index(lower[at:], o[0])
			if k < 0 || next >= 0 && at+k >= next {
				continue
			}
			// A tag's name ends there, as <code> or <pre class="x">.
			if e := at + k + len(o[0]); o[0] != "<!--" && e < len(s) && !strings.ContainsRune(" \t\n>/", rune(s[e])) {
				continue
			}
			next, closer = at+k, o[1]
		}
		if next < 0 {
			b.WriteString(f(s[at:]))
			return b.String()
		}
		b.WriteString(f(s[at:next]))
		at, h.in = next, closer
	}
	return b.String()
}

// resolveInline resolves the addresses of the inline links and images of
// s, each after the "](" that ends its text.
func resolveInline(s string, fn func(string, bool) string) string {
	if !strings.Contains(s, "](") {
		return s
	}
	var b strings.Builder
	at := 0
	for {
		k := strings.Index(s[at:], "](")
		if k < 0 {
			break
		}
		open := at + k + 2
		from, to := destination(s, open)
		if from == to {
			b.WriteString(s[at:open])
			at = open
			continue
		}
		b.WriteString(s[at:from])
		b.WriteString(fn(s[from:to], isImage(s, at+k)))
		at = to
	}
	b.WriteString(s[at:])
	return b.String()
}

// destination returns where the address of a link starts and ends in s,
// from i, just after its "(": past spaces, within angle brackets, or up to
// a space or the parenthesis that closes it.
func destination(s string, i int) (from, to int) {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i < len(s) && s[i] == '<' {
		end := strings.IndexByte(s[i+1:], '>')
		if end < 0 {
			return i, i
		}
		return i + 1, i + 1 + end
	}
	depth := 0
	j := i
loop:
	for ; j < len(s); j++ {
		switch s[j] {
		case ' ', '\t':
			break loop
		case '(':
			depth++
		case ')':
			if depth == 0 {
				break loop
			}
			depth--
		}
	}
	return i, j
}

// isImage reports whether the text that the bracket at end closes is an
// image's: the bracket that opens it follows a "!".
func isImage(s string, end int) bool {
	depth := 0
	for i := end - 1; i >= 0; i-- {
		switch s[i] {
		case ']':
			depth++
		case '[':
			if depth == 0 {
				return i > 0 && s[i-1] == '!'
			}
			depth--
		}
	}
	return false
}

// resolveTags resolves the src and href of the HTML tags of s; a src is an
// image's.
func resolveTags(s string, fn func(string, bool) string) string {
	if !strings.Contains(s, "=") {
		return s
	}
	return tagAddr.ReplaceAllStringFunc(s, func(m string) string {
		g := tagAddr.FindStringSubmatchIndex(m)
		from, to := g[6], g[7]
		for k := 8; from < 0 && k < len(g); k += 2 {
			from, to = g[k], g[k+1]
		}
		return m[:from] + fn(m[from:to], strings.EqualFold(m[g[4]:g[5]], "src")) + m[to:]
	})
}
