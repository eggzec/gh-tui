package markdown

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// tabWidth is how many columns a tab in the source moves to, as GitHub
// shows them.
const tabWidth = 4

// prepare makes src safe to draw and turns the HTML that GitHub renders,
// which glamour would drop or show as source, into markdown: the comments
// go, a collapsed section shows its summary and its content, and an image
// becomes a markdown image. Code in backticks is left as it is, and a
// fenced block shows as show has it, told its index and whether it is
// open, which a collapsible block is if its index is in open. A line of
// text that is an image alone shows as pic has it, told the image's alt
// text and address, unless pic is nil: an image on the web, or by a
// relative address too if relative is set. A source cut at maxLines ends
// with a note that offers hint, such as how to see the rest, if it isn't
// empty. What it adds of its own, such as the mark of a collapsed
// section, it draws with g.
func prepare(src string, open []int, hint string, g Glyphs, width int, show func(i int, b Block, open bool) string, pic func(alt, url string) string, relative bool) string {
	type out struct {
		line string
		text bool
	}
	var outs []out
	scan(src, hint, g, width, func(line string, text bool) { outs = append(outs, out{line, text}) }, func(i int, b Block) {
		outs = append(outs, out{show(i, b, b.Collapsed == "" || slices.Contains(open, i)), false})
	})
	lines := make([]string, len(outs))
	// first is the first line of the paragraph the line continues, if it
	// continues one, which puts it in a quote or a list item however many
	// lazy lines follow it. A block, such as code, a heading or a
	// thematic break, ends any paragraph, and a line with a marker starts
	// one.
	first := ""
	for i, o := range outs {
		lines[i] = o.line
		if pic == nil {
			continue
		}
		prev := first
		switch {
		case !o.text || strings.TrimSpace(o.line) == "" || heading(o.line) || rule(o.line):
			first = ""
		case first == "" || startsMarked(o.line):
			first = o.line
		}
		if !o.text {
			continue
		}
		alt, url, ok := alone(o.line, relative)
		if !ok {
			continue
		}
		next := ""
		if i+1 < len(outs) && outs[i+1].text {
			next = outs[i+1].line
		}
		if indent := leading(o.line); standsAlone(prev, next, indent) {
			lines[i] = indentLines(pic(alt, url), o.line[:indent])
		}
	}
	return strings.Join(lines, "\n")
}

var (
	// setext matches the line under a heading of the setext style, which
	// makes the line before it a heading, not a paragraph.
	setext = regexp.MustCompile(`^ {0,3}(?:=+|-+)\s*$`)
	// atxHeading matches a heading of the ATX style, which no line
	// continues.
	atxHeading = regexp.MustCompile(`^ {0,3}#{1,6}(?:\s|$)`)
)

// standsAlone reports whether a line that is an image alone, indented
// indent spaces, stands as a paragraph of its own, which a block of its
// own may take the place of: not under a quote or list item it continues
// lazily, which the block would end, and not the text of a heading. prev
// is the first line of the paragraph the image continues, or "" when it
// continues none, and next is the line after it.
func standsAlone(prev, next string, indent int) bool {
	if setext.MatchString(next) {
		return false
	}
	if strings.TrimSpace(prev) == "" {
		return true
	}
	// The content of prev starts after its indent and its markers.
	at := leading(prev)
	for {
		n, quote := marker(prev[at:])
		if n == 0 {
			break
		}
		if quote {
			return false
		}
		at += n
		at += leading(prev[at:])
	}
	return indent >= at
}

// heading reports whether line is a heading of the ATX style.
func heading(line string) bool {
	return strings.HasPrefix(strings.TrimLeft(line, " "), "#") && atxHeading.MatchString(line)
}

// rule reports whether line is a thematic break: three or more of one of
// -, * or _, with spaces between them if any, indented less than code is.
// A line of = is no break.
func rule(line string) bool {
	s := strings.TrimRight(line, " ")
	if leading(s) > 3 {
		return false
	}
	s = strings.ReplaceAll(s, " ", "")
	if len(s) < 3 || s[0] != '-' && s[0] != '*' && s[0] != '_' {
		return false
	}
	return strings.Trim(s, s[:1]) == ""
}

// startsMarked reports whether line starts with a quote or list marker.
func startsMarked(line string) bool {
	n, _ := marker(line[leading(line):])
	return n > 0
}

// leading returns how many spaces s starts with.
func leading(s string) int {
	return len(s) - len(strings.TrimLeft(s, " "))
}

// indentLines returns each line of s after indent.
func indentLines(s, indent string) string {
	if indent == "" {
		return s
	}
	return indent + strings.ReplaceAll(s, "\n", "\n"+indent)
}

// aloneImage matches a line that is an image alone, as html leaves an img
// tag or as markdown writes one, indented less than code is.
var aloneImage = regexp.MustCompile(`^ {0,3}!\[((?:\\.|[^\]\\])*)\]\(\s*(?:<([^>\s]+)>|([^)\s]+))(?:\s+"[^"]*")?\s*\)\s*$`)

// alone returns the alt text and the address of the image that line is
// alone, if it is one: one on the web, or with relative set, one by a
// relative address too, such as a file's beside the markdown.
func alone(line string, relative bool) (alt, url string, ok bool) {
	if !strings.HasPrefix(strings.TrimLeft(line, " "), "![") {
		return "", "", false
	}
	m := aloneImage.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	url = m[2] + m[3]
	web := strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://")
	if !web && (!relative || !relativeAddr(url)) {
		return "", "", false
	}
	alt = strings.NewReplacer(`\\`, `\`, `\[`, "[", `\]`, "]").Replace(m[1])
	return alt, url, true
}

// relativeAddr reports whether addr is a relative address, with no scheme
// and no host of its own, such as img/logo.png or /docs/a.png.
func relativeAddr(addr string) bool {
	if strings.HasPrefix(addr, "//") {
		return false
	}
	head, _, _ := strings.Cut(addr, "/")
	return !strings.Contains(head, ":")
}

// plain shows a block as markdown alone: a collapsible one as its
// collapsed line, in the renderer's glyphs, as code, which glamour puts on
// a line of its own even in a list item, with its code under it while it
// is open.
func (r *Renderer) plain(_ int, b Block, open bool) string {
	if b.Collapsed == "" {
		return b.Full
	}
	s := b.fence + "\n" + b.indent() + r.glyphs.collapsed(b) + "\n" + b.fence
	if open {
		s += "\n" + b.Full
	}
	return s
}

// scan makes src safe to draw and passes it on a line at a time, with the
// HTML of each line turned into markdown and its footnotes moved to its
// end, and whether the line is text rather than code, except that it
// passes each fenced code block whole, with its index among them. A
// source longer than maxLines is cut, and ends with cutNote(hint).
func scan(src, hint string, g Glyphs, width int, line func(l string, text bool), block func(int, Block)) {
	lines := strings.Split(termtext.Clean(src, tabWidth), "\n")
	cut := len(lines) > maxLines
	if cut {
		lines = lines[:maxLines]
	}
	var out []piece
	p := htmlState{g: g}
	c := codeState{blank: true}
	var lim bounds
	var sp spans
	n := 0
	for i := 0; i < len(lines); i++ {
		f, ok := openFence(lines[i])
		if !ok || p.inComment || c.holds(lines[i]) {
			l, end := lim.table(lim.nest(lines[i]))
			if end {
				out = append(out, piece{text: true})
			}
			l = p.line(refs(l))
			text := !c.in(l)
			if text {
				l = linkItem(alert(literal(l), g))
				if strings.TrimSpace(l) == "" {
					sp = spans{}
				}
				l = sp.bind(l, width)
			}
			if !text {
				sp = spans{}
			}
			out = append(out, piece{line: l, text: text})
			continue
		}
		end := len(lines) - 1
		for j := i + 1; j < len(lines); j++ {
			if f.closedBy(lines[j]) {
				end = j
				break
			}
		}
		sp = spans{}
		out = append(out, piece{block: new(showBlock(f.lang, strings.Join(lines[i:end+1], "\n"))), n: n})
		n++
		i = end
	}
	for _, pc := range footnotes(out, g) {
		if pc.block != nil {
			block(pc.n, *pc.block)
		} else {
			line(pc.line, pc.text)
		}
	}
	if cut {
		line("\n"+cutNote(hint, g), false)
	}
}

// piece is a line of a source that scan passes on, and whether it is text
// rather than code, or a fenced code block with its index.
type piece struct {
	line  string
	text  bool
	block *Block
	n     int
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
func cutNote(hint string, g Glyphs) string {
	note := verbatim(g.More) + " The rest is too long to show here"
	if hint != "" {
		note += verbatim(g.Separator) + verbatim(hint)
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

// holds reports whether line, which looks like a fence, is in such code
// instead.
func (c *codeState) holds(line string) bool {
	indent := len(line) - len(strings.TrimLeft(line, " "))
	return c.pre || indent >= 4 && (c.indented || c.blank) && !c.list
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
func badge(s string, g Glyphs) string {
	m := linkedImage.FindStringSubmatch(s)
	alt := strings.TrimSpace(m[1])
	if alt == "" {
		alt = "image"
	}
	// Glamour draws a link's text as written, escapes too, so the glyph
	// goes in as it is.
	return "[" + g.Image + " " + alt + "](" + m[2] + ")"
}

// alert turns the line that marks a GitHub alert, such as "> [!NOTE]",
// into the alert's label.
func alert(line string, g Glyphs) string {
	if !strings.Contains(line, "[!") {
		return line
	}
	m := alertLine.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	return m[1] + "**" + verbatim(g.alert(strings.ToUpper(m[2]))) + "**"
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
	// g are the glyphs that mark what the HTML turns into.
	g Glyphs
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
		b.WriteString(html(text.String(), p.g))
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
func html(s string, g Glyphs) string {
	if !hasHTML(s) {
		return s
	}
	s = imgTag.ReplaceAllStringFunc(s, image)
	s = linkedImage.ReplaceAllStringFunc(s, func(s string) string { return badge(s, g) })
	s = strings.ReplaceAll(s, "![](", "![image](")
	s = detailsTag.ReplaceAllString(s, "")
	s = wrapperTag.ReplaceAllString(s, "")
	open, end := summaryTag.FindStringIndex(s), summaryEnd.FindStringIndex(s)
	if open != nil && end != nil && open[1] <= end[0] {
		text := strings.TrimSpace(s[open[1]:end[0]])
		if text == "" {
			text = "Details"
		}
		s = s[:open[0]] + "**" + verbatim(g.Fold) + " " + text + "**" + body(s[end[1]:])
	} else {
		s = summaryTag.ReplaceAllLiteralString(s, verbatim(g.Fold)+" ")
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

// spanSpace stands for a space inside a code span while glamour wraps the
// text, which it would break at, so a span that fits on a line wraps as a
// unit. It is a private-use character of a supplementary plane, one cell
// wide, no space and no markdown, and unspan puts the spaces back. The
// planes' characters are rarer in text than the basic plane's, which icon
// fonts fill.
const spanSpace = '\U000F0000'

// spans follows the code spans of the lines of a paragraph, one line at a
// time, since a span may run over several.
type spans struct {
	// open is the length of the run of backticks of the span that is open at
	// the end of the last line, or 0.
	open int
}

// bind returns line with the spaces inside its code spans that are no wider
// than width, except those that start or end a span, which markdown drops,
// replaced with spanSpace. A span that is wider breaks at its spaces as it
// is. A backtick after an odd number of backslashes is text, and a span
// that closes on a later line is left alone, as is the end of one that
// opened on an earlier line. A width of 0 or less binds nothing.
func (p *spans) bind(line string, width int) string {
	if width <= 0 {
		return line
	}
	var b strings.Builder
	for i := 0; i < len(line); {
		if p.open > 0 {
			end := closeRun(line[i:], p.open)
			if end < 0 {
				b.WriteString(line[i:])
				return b.String()
			}
			b.WriteString(line[i : i+end])
			i += end
			p.open = 0
			continue
		}
		k := strings.IndexByte(line[i:], '`')
		if k < 0 {
			b.WriteString(line[i:])
			break
		}
		k += i
		if backslashes(line[:k])%2 == 1 {
			b.WriteString(line[i : k+1])
			i = k + 1
			continue
		}
		b.WriteString(line[i:k])
		n := run(line[k:])
		end := closeRun(line[k+n:], n)
		if end < 0 {
			p.open = n
			b.WriteString(line[k:])
			return b.String()
		}
		end += k + n
		b.WriteString(line[k : k+n])
		b.WriteString(bindSpaces(line[k+n:end-n], width-2*n))
		b.WriteString(line[end-n : end])
		i = end
	}
	return b.String()
}

// backslashes returns how many backslashes s ends with.
func backslashes(s string) int {
	return len(s) - len(strings.TrimRight(s, "\\"))
}

// bindSpaces returns the content of a code span with the spaces between its
// first and last character replaced with spanSpace, if it fits in room
// cells.
func bindSpaces(s string, room int) string {
	from := len(s) - len(strings.TrimLeft(s, " "))
	to := len(strings.TrimRight(s, " "))
	if from >= to || xansi.StringWidth(s) > room {
		return s
	}
	return s[:from] + strings.ReplaceAll(s[from:to], " ", string(spanSpace)) + s[to:]
}

// unspan puts back the spaces of the code spans that bind replaced.
func unspan(s string) string {
	return strings.ReplaceAll(s, string(spanSpace), " ")
}
