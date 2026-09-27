package markdown

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// tabWidth is how many columns a tab in the source moves to, as GitHub
// shows them.
const tabWidth = 4

// prepare makes src safe to draw and turns the HTML that GitHub renders,
// which glamour would drop or show as source, into markdown: the comments
// go, a collapsed section shows its summary and its content, and an image
// becomes a markdown image. Code in backticks is left as it is, and a
// fenced block shows as its language has it, collapsed unless its index
// is in open. A source cut at maxLines ends with a note that offers hint,
// such as how to see the rest, if it isn't empty.
func prepare(src string, open []int, hint string) string {
	var out []string
	scan(src, hint, func(line string) { out = append(out, line) }, func(i int, b Block) {
		if b.Collapsed == "" || slices.Contains(open, i) {
			out = append(out, b.Full)
			return
		}
		// A paragraph of its own.
		out = append(out, "", b.Collapsed, "")
	})
	return strings.Join(out, "\n")
}

// scan makes src safe to draw and passes it on a line at a time, with the
// HTML of each line turned into markdown, except that it passes each
// fenced code block whole, with its index among them. A source longer
// than maxLines is cut, and ends with cutNote(hint).
func scan(src, hint string, line func(string), block func(int, Block)) {
	lines := strings.Split(termtext.Clean(src, tabWidth), "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		defer line("\n" + cutNote(hint))
	}
	var p htmlState
	c := codeState{blank: true}
	var lim bounds
	b := newBudget()
	n := 0
	for i := 0; i < len(lines); i++ {
		f, ok := openFence(lines[i])
		if !ok || p.inComment {
			l, end := lim.table(lim.nest(lines[i]))
			if end {
				line("")
			}
			l = p.line(refs(l))
			if !c.in(l) {
				l = linkItem(alert(literal(l)))
			}
			line(l)
			continue
		}
		end := len(lines) - 1
		for j := i + 1; j < len(lines); j++ {
			if f.closedBy(lines[j]) {
				end = j
				break
			}
		}
		block(n, showBlock(f.lang, strings.Join(lines[i:end+1], "\n"), b))
		n++
		i = end
	}
}

// fence is the opening fence of a code block.
type fence struct {
	char byte
	n    int
	lang string
}

// openFence reports whether line opens a fenced code block. It allows any
// indentation, since a fence in a list item is indented, and taking an
// indented line for a fence only leaves more of the source as it is.
func openFence(line string) (fence, bool) {
	s := strings.TrimLeft(line, " ")
	if len(s) < 3 || (s[0] != '`' && s[0] != '~') {
		return fence{}, false
	}
	c := s[0]
	n := len(s) - len(strings.TrimLeft(s, string(c)))
	if n < 3 {
		return fence{}, false
	}
	info := strings.TrimSpace(s[n:])
	if c == '`' && strings.Contains(info, "`") {
		return fence{}, false
	}
	lang, _, _ := strings.Cut(info, " ")
	return fence{char: c, n: n, lang: strings.ToLower(lang)}, true
}

// closedBy reports whether line closes the block f opened.
func (f fence) closedBy(line string) bool {
	s := strings.TrimSpace(line)
	return len(s) >= f.n && strings.Trim(s, string(f.char)) == ""
}

var (
	imgTag     = regexp.MustCompile(`(?i)<img\b[^>]*>`)
	attr       = regexp.MustCompile(`(?i)\b(src|alt)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)
	detailsTag = regexp.MustCompile(`(?i)</?details\b[^>]*>`)
	summaryTag = regexp.MustCompile(`(?i)<summary\b[^>]*>`)
	summaryEnd = regexp.MustCompile(`(?i)</summary\s*>`)
	// wrapperTag matches the tags that only lay out what they hold, which
	// would otherwise make glamour take the line for HTML and drop it.
	wrapperTag = regexp.MustCompile(`(?i)</?(?:p|div|center|picture)\b[^>]*>`)
	// linkedImage matches an image in a link, [![alt](src)](href).
	linkedImage = regexp.MustCompile(`\[!\[([^\]]*)\]\([^)]*\)\]\(\s*(<[^>]*>|[^)\s]+)[^)]*\)`)
	// taskLink matches a list item that starts with a link named x, X or
	// a space, which GitHub shows as a link but goldmark takes for a task.
	taskLink  = regexp.MustCompile(`^((?:\s*>)*\s*(?:[-*+]|\d+[.)])\s+)\[([ xX])\]\(`)
	alertLine = regexp.MustCompile(`(?i)^(\s*>\s*)\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*$`)
)

// maxQuotes and maxLists are how deep quotes and lists nest at most.
// Glamour's time grows fast with the depth, and a comment can nest
// hundreds deep; deeper ones show at these depths.
const (
	maxQuotes = 4
	maxLists  = 4
	// maxIndent is how far a line is indented at most, which is how
	// deep lists nest over several lines.
	maxIndent = 24
	// maxMarkers is how many quote and list markers a source keeps in
	// all; after them, each line keeps one of each. A thousand lines
	// nested eight deep would take glamour a quarter of a second.
	maxMarkers = 2000
)

// bounds keeps what a source spent of what glamour renders quickly: the
// quote and list markers kept, and the cells of its tables.
type bounds struct {
	markers, cells int
}

// nest returns line flattened to the depth the source has left.
func (b *bounds) nest(line string) string {
	quotes, lists := maxQuotes, maxLists
	if b.markers >= maxMarkers {
		quotes, lists = 1, 1
	}
	line, n := flatten(line, quotes, lists)
	b.markers += n
	return line
}

// flatten returns line with the quote and list markers that start it
// beyond quoteDepth quotes and listDepth lists dropped, and its indentation
// cut to maxIndent, and how many markers it kept.
func flatten(line string, quoteDepth, listDepth int) (flat string, kept int) {
	if indent := len(line) - len(strings.TrimLeft(line, " ")); indent > maxIndent {
		line = line[indent-maxIndent:]
	}
	quotes, lists := 0, 0
	var b strings.Builder
	i := 0
	for {
		j := i
		for j < len(line) && j-i < 4 && line[j] == ' ' {
			j++
		}
		n, quote := marker(line[j:])
		if n == 0 {
			break
		}
		keep := quote && quotes < quoteDepth || !quote && lists < listDepth
		if quote {
			quotes++
		} else {
			lists++
		}
		if keep {
			b.WriteString(line[i : j+n])
		} else if b.Len() == 0 && i == 0 {
			// Nothing kept yet: the line still starts where it did.
			b.WriteString(line[i:j])
		}
		i = j + n
	}
	kept = min(quotes, quoteDepth) + min(lists, listDepth)
	if quotes <= quoteDepth && lists <= listDepth {
		return line, kept
	}
	b.WriteString(line[i:])
	return b.String(), kept
}

// marker returns the length of the quote or list marker, with the space
// after it, that s starts with, and whether it is a quote, or 0.
func marker(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	if s[0] == '>' {
		if len(s) > 1 && s[1] == ' ' {
			return 2, true
		}
		return 1, true
	}
	n := 0
	switch s[0] {
	case '-', '*', '+':
		n = 1
	default:
		for n < len(s) && n < 9 && s[n] >= '0' && s[n] <= '9' {
			n++
		}
		if n == 0 || n == len(s) || s[n] != '.' && s[n] != ')' {
			return 0, false
		}
		n++
	}
	if n < len(s) && s[n] == ' ' {
		return n + 1, false
	}
	return 0, false
}

// maxLines is how many lines of a source show at most. Glamour takes
// about a tenth of a millisecond for each list item or other block, and
// renders in Update, so the rest of a longer source is left out, with a
// note in its place.
const maxLines = 1000

// cutNote returns the note that ends a source cut at maxLines, offering
// hint if it isn't empty.
func cutNote(hint string) string {
	note := "⋯ The rest is too long to show here"
	if hint != "" {
		note += " · " + verbatim(hint)
	}
	return "*" + note + "*"
}

// maxLine is how long a line's markdown is at most. Goldmark's time can
// grow with the square of a line's length, as with links or emphasis that
// never close, so a longer line shows as text.
const maxLine = 4 << 10

// literal returns line as it is, or, if it is longer than maxLine, with
// its markdown escaped after the quote and list markers that start it.
func literal(line string) string {
	if len(line) <= maxLine {
		return line
	}
	i := 0
	for {
		j := i + len(line[i:]) - len(strings.TrimLeft(line[i:], " "))
		n, _ := marker(line[j:])
		if n == 0 {
			break
		}
		i = j + n
	}
	return line[:i] + verbatim(line[i:])
}

// verbatim returns s with a backslash before each ASCII punctuation
// character, so markdown shows it as it is.
func verbatim(s string) string {
	var b strings.Builder
	b.Grow(len(s) + len(s)/4)
	for i := range len(s) {
		if strings.IndexByte(punctuation, s[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// punctuation is the characters markdown lets a backslash escape.
const punctuation = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

// maxCells is how many cells a table row has at most, and
// maxTableCells how many the tables of a source have in all. Glamour's
// time grows fast with the columns, and with the cells, and a row with
// more shows as text.
const (
	maxCells      = 64
	maxTableCells = 4096
)

// table returns line with its pipes escaped if it has more than maxCells
// of them, or once the tables before it had maxTableCells, so it can't
// make a table, and whether it is the first line past maxTableCells,
// which must follow a blank line to end the table it would go on.
func (b *bounds) table(line string) (string, bool) {
	n := strings.Count(line, "|")
	if n == 0 {
		return line, false
	}
	text := strings.ReplaceAll(line, "|", "\\|")
	if n > maxCells {
		return text, false
	}
	before := b.cells
	b.cells += n
	if b.cells <= maxTableCells {
		return line, false
	}
	return text, before <= maxTableCells
}

// charRef matches a numeric character reference, which glamour decodes
// after the source was cleaned. HTML takes one without its semicolon too.
var charRef = regexp.MustCompile(`&#(?:[xX]([0-9a-fA-F]{1,8})|(\d{1,10}));?|&(?:rlm|lrm);`)

// refs returns line with the character references to controls, and to the
// characters that reorder text, replaced by U+FFFD, so what glamour
// decodes is text. A control in the name of a link would otherwise break
// its hyperlink, whose address would then show.
func refs(line string) string {
	if !strings.Contains(line, "&") {
		return line
	}
	return charRef.ReplaceAllStringFunc(line, func(ref string) string {
		m := charRef.FindStringSubmatch(ref)
		var v int64 = -1
		switch {
		case m[1] != "":
			v, _ = strconv.ParseInt(m[1], 16, 64)
		case m[2] != "":
			v, _ = strconv.ParseInt(m[2], 10, 64)
		}
		r := rune(v)
		if v < 0 || v > utf8.MaxRune || v >= 0xd800 && v <= 0xdfff || v == 0 || termtext.Control(r) {
			return string(utf8.RuneError)
		}
		return ref
	})
}

// codeState follows the code that isn't fenced, in <pre> or indented,
// which the rewrites meant for text leave alone.
type codeState struct {
	pre, indented, list, blank bool
}

// in reports whether line, the next line that isn't fenced code, is in
// such code.
func (c *codeState) in(line string) bool {
	t := strings.TrimSpace(line)
	lower := strings.ToLower(t)
	switch {
	case c.pre:
		c.pre = !strings.Contains(lower, "</pre>")
		return true
	case strings.HasPrefix(lower, "<pre"):
		c.pre = !strings.Contains(lower, "</pre>")
		return true
	case t == "":
		c.blank = true
		return c.indented
	}
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent >= 4 && (c.indented || c.blank) && !c.list {
		c.indented, c.blank = true, false
		return true
	}
	if indent < 4 {
		n, quote := marker(t)
		c.list = n > 0 && !quote || c.list && !c.blank
	}
	c.indented, c.blank = false, false
	return false
}

// hasHTML reports whether s may hold what html converts.
func hasHTML(s string) bool {
	return strings.IndexByte(s, '<') >= 0 || strings.Contains(s, "![](") || strings.Contains(s, "[![")
}

// body returns what follows a summary on its line as a paragraph of its
// own, or "" when nothing does.
func body(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return "\n\n" + s
}

// badge turns an image that links somewhere, such as a badge, into a link
// named by the image, since the image can't show.
func badge(s string) string {
	m := linkedImage.FindStringSubmatch(s)
	alt := strings.TrimSpace(m[1])
	if alt == "" {
		alt = "image"
	}
	return "[🖼 " + alt + "](" + m[2] + ")"
}

// alerts are the labels of GitHub's alerts, by the name that marks them.
var alerts = map[string]string{
	"NOTE":      "ℹ Note",
	"TIP":       "✓ Tip",
	"IMPORTANT": "! Important",
	"WARNING":   "⚠ Warning",
	"CAUTION":   "✖ Caution",
}

// alert turns the line that marks a GitHub alert, such as "> [!NOTE]",
// into the alert's label.
func alert(line string) string {
	if !strings.Contains(line, "[!") {
		return line
	}
	m := alertLine.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	return m[1] + "**" + alerts[strings.ToUpper(m[2])] + "**"
}

// linkItem keeps a list item that starts with a link named x, as in
// "- [X](https://x.com)", from showing as a ticked task, with a zero-width
// space in the link's name.
func linkItem(line string) string {
	if !strings.Contains(line, "](") {
		return line
	}
	return taskLink.ReplaceAllString(line, "$1[\u200b$2](")
}

// htmlState carries an HTML comment from one line to the next.
type htmlState struct {
	inComment bool
}

// line returns line with its HTML turned into markdown and its HTML
// comments dropped, leaving code in backticks as it is. A line that held
// only HTML comes back empty.
func (p *htmlState) line(line string) string {
	if !p.inComment && !hasHTML(line) {
		return line
	}
	var b, text strings.Builder
	flush := func() {
		b.WriteString(html(text.String()))
		text.Reset()
	}
	for i := 0; i < len(line); {
		if p.inComment {
			end := strings.Index(line[i:], "-->")
			if end < 0 {
				break
			}
			p.inComment, i = false, i+end+3
			continue
		}
		next := strings.IndexAny(line[i:], "`<")
		if next < 0 {
			text.WriteString(line[i:])
			break
		}
		k := i + next
		text.WriteString(line[i:k])
		switch {
		case line[k] == '`':
			n := run(line[k:])
			end := closeRun(line[k+n:], n)
			if end < 0 {
				text.WriteString(line[k : k+n])
				i = k + n
				continue
			}
			flush()
			end += k + n
			b.WriteString(line[k:end])
			i = end
		case strings.HasPrefix(line[k:], "<!--"):
			p.inComment, i = true, k+4
		default:
			text.WriteByte('<')
			i = k + 1
		}
	}
	flush()
	if strings.TrimSpace(b.String()) == "" {
		return ""
	}
	return b.String()
}

// html converts the tags in s, which holds no code.
func html(s string) string {
	if !hasHTML(s) {
		return s
	}
	s = imgTag.ReplaceAllStringFunc(s, image)
	s = linkedImage.ReplaceAllStringFunc(s, badge)
	s = strings.ReplaceAll(s, "![](", "![image](")
	s = detailsTag.ReplaceAllString(s, "")
	s = wrapperTag.ReplaceAllString(s, "")
	open, end := summaryTag.FindStringIndex(s), summaryEnd.FindStringIndex(s)
	if open != nil && end != nil && open[1] <= end[0] {
		text := strings.TrimSpace(s[open[1]:end[0]])
		if text == "" {
			text = "Details"
		}
		s = s[:open[0]] + "**▸ " + text + "**" + body(s[end[1]:])
	} else {
		s = summaryTag.ReplaceAllString(s, "▸ ")
		s = summaryEnd.ReplaceAllString(s, "")
	}
	return s
}

// image turns an img tag into a markdown image.
func image(tag string) string {
	var src, alt string
	for _, m := range attr.FindAllStringSubmatch(tag, -1) {
		v := m[2] + m[3] + m[4]
		if strings.EqualFold(m[1], "src") {
			src = v
		} else {
			alt = v
		}
	}
	if src == "" {
		return ""
	}
	alt = strings.TrimSpace(alt)
	if alt == "" {
		alt = "image"
	}
	alt = strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`).Replace(alt)
	return "![" + alt + "](<" + strings.ReplaceAll(src, ">", "%3E") + ">)"
}

// run returns how many backticks s starts with.
func run(s string) int {
	return len(s) - len(strings.TrimLeft(s, "`"))
}

// closeRun returns where the run of exactly n backticks that closes a code
// span ends in s, or -1.
func closeRun(s string, n int) int {
	for i := 0; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		m := run(s[i:])
		if m == n {
			return i + m
		}
		i += m
	}
	return -1
}
