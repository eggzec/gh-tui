package markdown

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	// footnoteDef matches the line that starts a footnote, "[^label]: text".
	footnoteDef = regexp.MustCompile(`^ {0,3}\[\^([^\]\s]{1,100})\]:[ \t]*(.*)$`)
	// footnoteRef matches a reference to a footnote, "[^label]".
	footnoteRef = regexp.MustCompile(`\[\^([^\]\s]{1,100})\]`)
)

// footnotes returns ps with its footnotes shown as GitHub shows them, which
// glamour can't: each reference to a footnote that has a definition
// becomes its number, superscript, in the order they are first referred
// to, and the definitions move to the end, numbered, after a rule. A
// definition takes the lines that follow it until a blank line, code, or
// a line that starts another block, such as a heading, a list item or a
// quote.
// As on GitHub, the first of two definitions with the same label, which
// matches in any case, wins, one that nothing refers to doesn't show, and
// a reference without a definition stays as it is.
func footnotes(ps []piece, g Glyphs) []piece {
	if !hasFootnote(ps) {
		return ps
	}
	defs := make(map[string]*[]string)
	out := make([]piece, 0, len(ps))
	// def holds the lines of the definition being read, or is nil.
	var def *[]string
	for _, p := range ps {
		switch m := footnoteDef.FindStringSubmatch(p.line); {
		case !p.text:
			def = nil
		case m != nil:
			def = &[]string{m[2]}
			if label := strings.ToLower(m[1]); defs[label] == nil {
				defs[label] = def
			}
			continue
		case def != nil && strings.TrimSpace(p.line) != "" && !blockStart(p.line):
			*def = append(*def, strings.TrimLeft(p.line, " "))
			continue
		default:
			def = nil
		}
		out = append(out, p)
	}
	if len(defs) == 0 {
		return ps
	}
	numbers := make(map[string]int)
	var order []string
	number := func(ref string) string {
		label := strings.ToLower(ref[2 : len(ref)-1])
		if defs[label] == nil {
			return ref
		}
		n, ok := numbers[label]
		if !ok {
			order = append(order, label)
			n = len(order)
			numbers[label] = n
		}
		if g.ASCII {
			// Escaped, so a link's definition can't take it.
			return `\[` + strconv.Itoa(n) + `\]`
		}
		return superscript(n)
	}
	for i := range out {
		if out[i].text && strings.Contains(out[i].line, "[^") {
			out[i].line = outsideCode(out[i].line, func(s string) string { return replaceRefs(s, number) })
		}
	}
	if len(order) == 0 {
		return out
	}
	out = append(out, piece{text: true}, piece{line: "---", text: true}, piece{text: true})
	// A footnote may refer to another, which then shows too, so order
	// may grow as the loop goes.
	i := 0
	for i < len(order) {
		lines := *defs[order[i]]
		for j, l := range lines {
			l = outsideCode(l, func(s string) string { return replaceRefs(s, number) })
			prefix := strings.Repeat(" ", len(strconv.Itoa(i+1))+2)
			if j == 0 {
				prefix = strconv.Itoa(i+1) + ". "
			}
			out = append(out, piece{line: prefix + l, text: true})
		}
		i++
	}
	return out
}

// hasFootnote reports whether the text of ps may hold a footnote.
func hasFootnote(ps []piece) bool {
	for _, p := range ps {
		if p.text && strings.Contains(p.line, "[^") {
			return true
		}
	}
	return false
}

// replaceRefs returns s with each reference to a footnote replaced by what
// f returns for it, except one escaped with a backslash.
func replaceRefs(s string, f func(ref string) string) string {
	var b strings.Builder
	last := 0
	for _, m := range footnoteRef.FindAllStringIndex(s, -1) {
		if escaped(s, m[0]) {
			continue
		}
		b.WriteString(s[last:m[0]])
		b.WriteString(f(s[m[0]:m[1]]))
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

// escaped reports whether the character at i of s is escaped: after an odd
// number of backslashes, since two backslashes are a backslash itself.
func escaped(s string, i int) bool {
	n := 0
	for i-n > 0 && s[i-n-1] == '\\' {
		n++
	}
	return n%2 == 1
}

// blockStart reports whether line starts a block that ends a footnote's
// definition: a heading, a quote, a list item or a fence.
func blockStart(line string) bool {
	if indent := len(line) - len(strings.TrimLeft(line, " ")); indent > 3 {
		return false
	}
	s := strings.TrimLeft(line, " ")
	if n, _ := marker(s); n > 0 {
		return true
	}
	if _, fenced := openFence(s); fenced {
		return true
	}
	hashes := len(s) - len(strings.TrimLeft(s, "#"))
	return hashes >= 1 && hashes <= 6 && (hashes == len(s) || s[hashes] == ' ')
}

// outsideCode returns line with f applied to what isn't in a code span.
func outsideCode(line string, f func(string) string) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		k := strings.IndexByte(line[i:], '`')
		if k < 0 {
			b.WriteString(f(line[i:]))
			break
		}
		k += i
		b.WriteString(f(line[i:k]))
		n := run(line[k:])
		end := closeRun(line[k+n:], n)
		if end < 0 {
			b.WriteString(line[k : k+n])
			i = k + n
			continue
		}
		end += k + n
		b.WriteString(line[k:end])
		i = end
	}
	return b.String()
}

// superscript returns n in superscript digits.
func superscript(n int) string {
	digits := []rune("⁰¹²³⁴⁵⁶⁷⁸⁹")
	var b strings.Builder
	for _, d := range strconv.Itoa(n) {
		b.WriteRune(digits[d-'0'])
	}
	return b.String()
}
