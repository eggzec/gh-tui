package markdown

import (
	"regexp"
	"strings"
)

// Resolve returns src with the address of each link and image in it
// replaced by what fn returns for it, told whether it is an image's: the
// addresses of inline links and images, of link reference definitions,
// and the src and href of HTML tags. What fn returns goes in as it is,
// so it must not hold spaces or parentheses that would end the address.
// Code, fenced or in backticks, is left as it is. It lets a caller that
// knows what the relative addresses of a document are relative to, such
// as a README of a repository, make them absolute before rendering.
func Resolve(src string, fn func(addr string, image bool) string) string {
	if !strings.ContainsAny(src, "]=") {
		return src
	}
	lines := strings.Split(src, "\n")
	var in *fence
	for i, line := range lines {
		if in != nil {
			if in.closedBy(line) {
				in = nil
			}
			continue
		}
		if f, ok := openFence(line); ok {
			in = &f
			continue
		}
		if m := refDef.FindStringSubmatchIndex(line); m != nil {
			lines[i] = line[:m[2]] + fn(line[m[2]:m[3]], false) + line[m[3]:]
			continue
		}
		lines[i] = outsideCode(line, func(s string) string {
			return resolveTags(resolveInline(s, fn), fn)
		})
	}
	return strings.Join(lines, "\n")
}

// refDef matches a link reference definition, its address the first group.
var refDef = regexp.MustCompile(`^ {0,3}\[[^\]]+\]:[ \t]*<?([^\s>]+)>?`)

// tagAddr matches the src or href of an HTML tag, its address the third
// or the fourth group.
var tagAddr = regexp.MustCompile(`(?i)(<[a-z][a-z0-9]*\b[^>]*?\b)(src|href)\s*=\s*(?:"([^"]*)"|'([^']*)')`)

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
		if from < 0 {
			from, to = g[8], g[9]
		}
		return m[:from] + fn(m[from:to], strings.EqualFold(m[g[4]:g[5]], "src")) + m[to:]
	})
}
