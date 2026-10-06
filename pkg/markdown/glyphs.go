package markdown

import (
	"cmp"
	"regexp"
	"strings"

	"charm.land/glamour/v2/ansi"
	xansi "github.com/charmbracelet/x/ansi"
)

// Glyphs are the characters a render draws of its own, rather than of the
// text: list bullets, ticks, quote bars, table lines, the marks of images,
// collapsed sections, diagrams, alerts and footnotes, and the separators
// and ellipses of the lines it writes. An empty glyph keeps the default:
// the style's own for what the style draws, such as bullets, and the
// Unicode ones the renderer draws otherwise. The zero Glyphs draws what
// the renderer always drew; [ASCIIGlyphs] draws only ASCII.
type Glyphs struct {
	// Bullet starts an item of a list without numbers, and Tick fills the
	// box of a ticked task. They are drawn by the style: "•" and "✓" in
	// [DefaultStyle].
	Bullet, Tick string
	// Definition starts the description of a term in a definition list,
	// drawn by the style: "🠶" in [DefaultStyle].
	Definition string
	// TableColumn, TableRow and TableCross draw the lines of a table:
	// between its columns, under its header, and where they cross. Empty
	// ones draw them with lipgloss's normal border, "│", "─" and "┼".
	TableColumn, TableRow, TableCross string
	// Quote is the bar along a quote, "│".
	Quote string
	// Image marks an image shown as its text, "🖼". It goes into the text
	// of a link too, where glamour draws a backslash as it is, so it must
	// hold no backslash or bracket.
	Image string
	// Fold marks the summary of a collapsed section, "▸".
	Fold string
	// Diagram marks the line that stands for a diagram, "◆", and Link
	// ends its offer to view it, "↗".
	Diagram, Link string
	// Separator goes between the parts of a line, " · ", Ellipsis ends a
	// line cut to the width, "…", and More starts the note that ends a
	// source too long to show in full, "⋯".
	Separator, Ellipsis, More string
	// Note, Tip, Important, Warning and Caution mark GitHub's alerts:
	// "ℹ", "✓", "!", "⚠" and "✖".
	Note, Tip, Important, Warning, Caution string
	// ASCII keeps the rest of what the renderer adds ASCII too: emoji
	// shortcodes, such as :tada:, stay as they were written, a footnote
	// reference shows as [1] rather than ¹, a link in a table shows in
	// its cell rather than in a list under the table, which cuts a long
	// address with "…", the "…" that lipgloss ends a table's header cut
	// to its column with becomes "~", and every zero-width space goes and
	// every non-breaking one becomes a space, the text's own too, which
	// draws them the same.
	ASCII bool
}

// ASCIIGlyphs returns the glyphs that are all ASCII, for a terminal or a
// font that draws nothing else.
func ASCIIGlyphs() Glyphs {
	return Glyphs{
		Bullet: "*", Tick: "x", Definition: "->",
		TableColumn: "|", TableRow: "-", TableCross: "+",
		Quote: "|", Image: "Image:", Fold: "+",
		Diagram: "*", Link: "->",
		Separator: " - ", Ellipsis: "...", More: "...",
		Note: "i", Tip: "+", Important: "!", Warning: "!", Caution: "x",
		ASCII: true,
	}
}

// The renderer's own glyphs in Unicode, which it draws where Glyphs
// leaves them empty.
const (
	unicodeQuote     = "│"
	unicodeImage     = "🖼"
	unicodeFold      = "▸"
	unicodeDiagram   = "◆"
	unicodeLink      = "↗"
	unicodeSeparator = " · "
	unicodeEllipsis  = "…"
	unicodeMore      = "⋯"
)

// orDefault returns g with the renderer's own glyphs where it has none.
// The glyphs the style draws stay empty, which keeps the style's own.
func (g Glyphs) orDefault() Glyphs {
	g.Quote = cmp.Or(g.Quote, unicodeQuote)
	g.Image = cmp.Or(g.Image, unicodeImage)
	g.Fold = cmp.Or(g.Fold, unicodeFold)
	g.Diagram = cmp.Or(g.Diagram, unicodeDiagram)
	g.Link = cmp.Or(g.Link, unicodeLink)
	g.Separator = cmp.Or(g.Separator, unicodeSeparator)
	g.Ellipsis = cmp.Or(g.Ellipsis, unicodeEllipsis)
	g.More = cmp.Or(g.More, unicodeMore)
	g.Note = cmp.Or(g.Note, "ℹ")
	g.Tip = cmp.Or(g.Tip, "✓")
	g.Important = cmp.Or(g.Important, "!")
	g.Warning = cmp.Or(g.Warning, "⚠")
	g.Caution = cmp.Or(g.Caution, "✖")
	return g
}

// quoteToken is what glamour draws each cell of a quote's indent with in
// g: the bar, marked by a zero-width space as glamour's rather than the
// text's, so quoteBars changes only the indent.
func (g Glyphs) quoteToken() string { return g.Quote + "\u200b" }

// viewText is what the head of a diagram with a link offers.
func (g Glyphs) viewText() string { return "View diagram " + g.Link }

// alert returns the label of the GitHub alert named name, in upper case.
func (g Glyphs) alert(name string) string {
	switch name {
	case "NOTE":
		return g.Note + " Note"
	case "TIP":
		return g.Tip + " Tip"
	case "IMPORTANT":
		return g.Important + " Important"
	case "WARNING":
		return g.Warning + " Warning"
	default:
		return g.Caution + " Caution"
	}
}

// collapsed returns the line of markdown that shows b, which is
// collapsible, until the reader opens it.
func (g Glyphs) collapsed(b Block) string {
	return g.Diagram + " " + b.kind + g.Separator + b.size() + g.Separator + b.offer(g)
}

// style returns s with the glyphs of g that the style draws, where g has
// them. The bar of a quote changes only in a style that draws it as
// [DefaultStyle] does.
func (g Glyphs) style(s ansi.StyleConfig) ansi.StyleConfig {
	if g.Bullet != "" {
		s.Item.BlockPrefix = g.Bullet + " "
	}
	if g.Tick != "" {
		s.Task.Ticked = "[" + g.Tick + "] "
	}
	if g.Definition != "" {
		s.DefinitionDescription.BlockPrefix = "\n" + g.Definition + " "
	}
	if g.TableColumn != "" || g.TableRow != "" || g.TableCross != "" {
		s.Table.ColumnSeparator = new(cmp.Or(g.TableColumn, "│"))
		s.Table.RowSeparator = new(cmp.Or(g.TableRow, "─"))
		s.Table.CenterSeparator = new(cmp.Or(g.TableCross, "┼"))
	}
	if t := s.BlockQuote.IndentToken; t != nil && *t == quoteToken {
		s.BlockQuote.IndentToken = new(g.quoteToken())
	}
	if f := s.ImageText.Format; strings.HasPrefix(f, unicodeImage+" ") {
		s.ImageText.Format = g.Image + strings.TrimPrefix(f, unicodeImage)
	}
	return s
}

// asciiLines makes ASCII what glamour draws of its own in the rendered
// lines. Every non-breaking space becomes a space and every zero-width one
// goes, the text's own too, which draws them the same. The "…" that
// lipgloss always ends a table's header cut to its column with becomes
// "~", as wide: only one that ends a cell of a line drawn as the header of
// a table of two columns or more, over its rule, and not in the code that
// code reports. So a "…" the text ends a header cell with changes too, a
// header of one column keeps its "…", and code glamour draws itself,
// unhighlighted, changes only where it looks just like a table's header
// over its rule.
func asciiLines(lines []string, g Glyphs, code func(i int) bool) {
	var cellEnd *regexp.Regexp
	for i, l := range lines {
		l = asciiSpaces.Replace(l)
		if strings.Contains(l, "…") && i+1 < len(lines) && !code(i) && !code(i+1) && tableHeader(l, lines[i+1], g) {
			if cellEnd == nil {
				cellEnd = regexp.MustCompile(`…((?:\x1b\[[0-9;:]*m| )*(?:` + regexp.QuoteMeta(g.TableColumn) + `|$))`)
			}
			l = cellEnd.ReplaceAllString(l, "~$1")
		}
		lines[i] = l
	}
}

var asciiSpaces = strings.NewReplacer("\u00a0", " ", "\u200b", "")

// tableHeader reports whether header and rule are the header of a table
// drawn with g and the rule under it, maybe in a quote: the rule is runs
// of TableRow joined by TableCross, and the header has TableColumn
// over each TableCross.
func tableHeader(header, rule string, g Glyphs) bool {
	if g.TableRow == "" || g.TableCross == "" || g.TableColumn == "" {
		return false
	}
	r := xansi.Strip(rule)
	body := strings.TrimLeft(r, " "+g.Quote)
	if !strings.HasPrefix(body, g.TableRow) {
		return false
	}
	start := xansi.StringWidth(r[:len(r)-len(body)])
	cols := strings.Split(strings.TrimRight(body, " "), g.TableCross)
	if len(cols) < 2 {
		return false
	}
	h := xansi.Strip(header)
	at := start
	for i, c := range cols {
		if c == "" || strings.Trim(c, g.TableRow) != "" {
			return false
		}
		at += xansi.StringWidth(c)
		if i < len(cols)-1 {
			if xansi.Cut(h, at, at+xansi.StringWidth(g.TableCross)) != g.TableColumn {
				return false
			}
			at += xansi.StringWidth(g.TableCross)
		}
	}
	return true
}
