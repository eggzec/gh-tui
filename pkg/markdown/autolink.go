package markdown

import (
	"image/color"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	xansi "github.com/charmbracelet/x/ansi"
)

// RefKind is what a reference in a text points at.
type RefKind int

// The kinds of reference [Refs] finds.
const (
	// RefIssue is #N, an issue or a pull request of the repository.
	RefIssue RefKind = iota
	// RefUser is @login, a user or an organization.
	RefUser
	// RefCommit is a commit by its hash.
	RefCommit
)

// Ref is a reference found in a text. Its Ref is what follows the sign:
// the number for an issue, the login for a user and the hash, in lower
// case and as long as it was written, for a commit.
type Ref struct {
	Kind RefKind
	Ref  string
}

// maxRefs is how many references of a text are looked at, so a text full
// of them can't make a view ask about each.
const maxRefs = 100

// Autolink says where the reference ref of kind goes, an address, and
// whether it is known to exist. It is called while a text renders and must
// not do I/O: it answers from what is already known, and ok is false while
// a reference is unknown or doesn't exist.
type Autolink func(kind RefKind, ref string) (addr string, ok bool)

// SetAutolink makes the references of the texts it renders links, those
// that f says exist, while links are shown. A nil f links none. It forgets
// what was rendered; a kept render whose references f answers otherwise
// renders again when next asked for.
func (r *Renderer) SetAutolink(f Autolink) {
	r.autolink = f
	r.recent, r.older = nil, nil
}

// SetLinkColor sets the colour of the text of a link while links are shown,
// or none, for the colour of the style, if c is nil. It forgets what was
// rendered.
func (r *Renderer) SetLinkColor(c color.Color) {
	r.linkFg = ""
	if c != nil {
		r.linkFg = xansi.NewStyle().ForegroundColor(c).String()
	}
	r.recent, r.older = nil, nil
}

// answer is what the Autolink said of a reference, to know whether a kept
// render is still what it would say.
type answer struct {
	ref  Ref
	addr string
	ok   bool
}

// Refs returns the references of src, #N, @login and commit hashes, without
// repeats, in the order they come, at most [maxRefs]. Those in code, in
// links, in HTML and in addresses aren't.
func Refs(src string) []Ref {
	var all []Ref
	seen := map[Ref]bool{}
	var st refScan
	scan(src, "", Glyphs{}.orDefault(), 0, func(l string, text bool) {
		if !text {
			st = refScan{}
			return
		}
		for _, m := range st.find(unspan(l)) {
			if len(all) < maxRefs && !seen[m.ref] {
				seen[m.ref] = true
				all = append(all, m.ref)
			}
		}
	}, func(int, Block) { st = refScan{} })
	return all
}

// linkRefs returns line with each reference that r.autolink says exists as
// a markdown link, and records the answers in seen. st follows the code
// spans across the lines of a text.
func (r *Renderer) linkRefs(line string, st *refScan, seen *[]answer) string {
	ms := st.find(line)
	if len(ms) == 0 {
		return line
	}
	var b strings.Builder
	pos := 0
	for _, m := range ms {
		known := slices.ContainsFunc(*seen, func(a answer) bool { return a.ref == m.ref })
		if !known && len(*seen) >= maxRefs {
			continue
		}
		addr, ok := r.autolink(m.ref.Kind, m.ref.Ref)
		if !known {
			*seen = append(*seen, answer{m.ref, addr, ok})
		}
		if !ok || addr == "" {
			continue
		}
		text := line[m.start:m.end]
		if m.ref.Kind == RefCommit && len(text) > 7 {
			text = text[:7]
		}
		b.WriteString(line[pos:m.start])
		b.WriteString("[")
		b.WriteString(text)
		b.WriteString("](<")
		b.WriteString(addrSafe.Replace(addr))
		b.WriteString(">)")
		pos = m.end
	}
	b.WriteString(line[pos:])
	return b.String()
}

var addrSafe = strings.NewReplacer(" ", "%20", "<", "%3C", ">", "%3E", "\n", "%0A", "\r", "%0D")

// answered reports whether the Autolink says now what it said in seen.
func (r *Renderer) answered(seen []answer) bool {
	for _, a := range seen {
		if r.autolink == nil {
			return false
		}
		if addr, ok := r.autolink(a.ref.Kind, a.ref.Ref); addr != a.addr || ok != a.ok {
			return false
		}
	}
	return true
}

// refScan finds references in the lines of a text, following the code span
// that runs over lines.
type refScan struct {
	open  int
	depth int // the quote depth of the line the span opened in
}

// definition reports whether line starts with the definition of a link,
// [label]: address.
func definition(line string) bool {
	t := strings.TrimLeft(line, " ")
	if !strings.HasPrefix(t, "[") {
		return false
	}
	i := strings.Index(t, "]:")
	if i < 2 || strings.ContainsAny(t[1:i], "[]") {
		return false
	}
	// An address, and then nothing or a title.
	rest := strings.Fields(t[i+2:])
	return len(rest) == 1 || len(rest) > 1 && strings.ContainsAny(rest[1][:1], `"'(`)
}

// match is a reference at line[start:end].
type match struct {
	start, end int
	ref        Ref
}

func isWord(c rune) bool { return c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c) }

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// around returns the characters next to line[i:j].
func around(line string, i, j int) (before, after rune) {
	before, _ = utf8.DecodeLastRuneInString(line[:i])
	after, _ = utf8.DecodeRuneInString(line[j:])
	if i == 0 {
		before = 0
	}
	if j >= len(line) {
		after = 0
	}
	return before, after
}

// bounded reports whether the characters around a reference let it be one.
func bounded(before, after rune) bool {
	return !isWord(before) && before != '/' && !isWord(after) && after != '/'
}

// find returns the references of line.
func (st *refScan) find(line string) []match {
	if _, ok := continuation(line, st.depth); st.open > 0 && !ok {
		// A span never runs over a block boundary.
		st.open = 0
	}
	if st.open == 0 {
		st.depth = quotes(line)
	}
	if !strings.ContainsAny(line, "#@`0123456789") && st.open == 0 {
		return nil
	}
	if definition(line) {
		return nil
	}
	var out []match
	for i := 0; i < len(line); {
		if st.open > 0 {
			end := closeRun(line[i:], st.open)
			if end < 0 {
				return out
			}
			i += end
			st.open = 0
			continue
		}
		c := line[i]
		switch c {
		case '\\':
			i += 2
		case '`':
			if backslashes(line[:i])%2 == 1 {
				i++
				continue
			}
			n := run(line[i:])
			if end := closeRun(line[i+n:], n); end >= 0 {
				i += n + end
			} else {
				st.open = n
				return out
			}
		case '[':
			i = skipLink(line, i)
		case '<':
			i = skipTag(line, i)
		case '&':
			i = skipEntity(line, i)
		case '#', '@':
			j := i + 1
			for j < len(line) && (wordByte(line[j]) && (c == '@' || line[j] >= '0' && line[j] <= '9')) {
				j++
			}
			if m, ok := sign(line, i, j); ok {
				out = append(out, m)
			}
			i = j
		default:
			r, size := utf8.DecodeRuneInString(line[i:])
			if !isWord(r) {
				i += size
				continue
			}
			j := i
			for j < len(line) {
				r, size = utf8.DecodeRuneInString(line[j:])
				if !isWord(r) {
					break
				}
				j += size
			}
			before, after := around(line, i, j)
			switch tok := line[i:j]; {
			case after == ':' && strings.HasPrefix(line[j:], "://"), tok == "www" && after == '.':
				j += strings.IndexAny(line[j:]+" ", " \t<")
			case hash(tok) && bounded(before, after):
				out = append(out, match{i, j, Ref{RefCommit, strings.ToLower(tok)}})
			}
			i = j
		}
	}
	return out
}

// wordByte reports whether c may be in the word after a # or an @.
func wordByte(c byte) bool {
	return c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// sign returns the reference that starts with the # or @ at line[i] and
// whose word ends at j, if it is one.
func sign(line string, i, j int) (match, bool) {
	word := line[i+1 : j]
	before, after := around(line, i, j)
	if !bounded(before, after) || word == "" {
		return match{}, false
	}
	if line[i] == '@' && strings.ContainsRune("+.=%", before) {
		// The part of an address of email.
		return match{}, false
	}
	if line[i] == '#' {
		if len(word) > 9 || word[0] == '0' || strings.Trim(word, "0123456789") != "" {
			return match{}, false
		}
		return match{i, j, Ref{RefIssue, word}}, true
	}
	if len(word) > 39 || word[0] == '-' || word[len(word)-1] == '-' || strings.ContainsFunc(word, func(c rune) bool { return c == '_' || c >= 0x80 }) {
		return match{}, false
	}
	return match{i, j, Ref{RefUser, word}}, true
}

// hash reports whether tok is a commit hash: 7 to 40 hexadecimal digits,
// with a digit and a letter among them.
func hash(tok string) bool {
	if len(tok) < 7 || len(tok) > 40 {
		return false
	}
	digit, letter := false, false
	for i := range len(tok) {
		switch c := tok[i]; {
		case !isHex(c):
			return false
		case c <= '9':
			digit = true
		default:
			letter = true
		}
	}
	return digit && letter
}

// skipLink returns where to go on after the [ at line[i]: past the whole
// link if it is one, its text and address or label.
func skipLink(line string, i int) int {
	k := strings.IndexByte(line[i:], ']')
	if k < 0 {
		return i + 1
	}
	k += i
	if k+1 < len(line) {
		closer := map[byte]byte{'(': ')', '[': ']'}[line[k+1]]
		if closer != 0 {
			if e := strings.IndexByte(line[k+2:], closer); e >= 0 {
				return k + 2 + e + 1
			}
		}
	}
	return i + 1
}

// skipTag returns where to go on after the < at line[i]: past an HTML tag
// or an autolink, or just past the <.
func skipTag(line string, i int) int {
	if i+1 < len(line) {
		if c := line[i+1]; c == '/' || c == '!' || c == '?' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			if e := strings.IndexByte(line[i:], '>'); e >= 0 {
				return i + e + 1
			}
		}
	}
	return i + 1
}

// skipEntity returns where to go on after the & at line[i]: past a
// character reference such as &#123; or &amp;, or just past the &.
func skipEntity(line string, i int) int {
	j := i + 1
	if j < len(line) && line[j] == '#' {
		j++
	}
	for j < len(line) && j-i < 12 && (line[j] >= '0' && line[j] <= '9' || line[j] >= 'a' && line[j] <= 'z' || line[j] >= 'A' && line[j] <= 'Z') {
		j++
	}
	if j > i+1 && j < len(line) && line[j] == ';' {
		return j + 1
	}
	return i + 1
}

// recolor returns the text of a link in the colour fg, a sequence that sets
// it: it is set again after each sequence of the text, which may set its
// own, and ends with it.
func recolor(inner, fg string) string {
	var b strings.Builder
	b.WriteString(fg)
	for i := 0; i < len(inner); {
		if inner[i] == esc && i+1 < len(inner) && inner[i+1] == '[' {
			j := i + 2
			for j < len(inner) && (inner[j] < '@' || inner[j] > '~') {
				j++
			}
			if j < len(inner) {
				j++
				b.WriteString(inner[i:j])
				if inner[j-1] == 'm' {
					b.WriteString(fg)
				}
				i = j
				continue
			}
		}
		b.WriteByte(inner[i])
		i++
	}
	b.WriteString("\x1b[39m")
	return b.String()
}
