package filterform

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// gutterWidth is the width of the mark before the row in focus.
const gutterWidth = 2

// labelGap is the space between the label column and the values.
const labelGap = 2

// View renders the form at exactly its width and height: the tabs, if it
// has them, the rows of the tab on view, with the open editor under its
// row, then a rule, the query and the help line.
func (m Model) View() string { return m.view }

// orderLabel names the row of the order on the Sort tab.
const orderLabel = "Order"

// labelWidth returns the width of the label column, which shrinks to a
// quarter of the width. Both tabs share it, so the values don't move when
// the tab changes.
func (m *Model) labelWidth() int {
	if m.lw.width == m.width && m.lw.set {
		return m.lw.label
	}
	w := 0
	for i := range m.spec.Fields {
		w = max(w, ansi.StringWidth(m.spec.Fields[i].Label))
	}
	if m.spec.Sort != nil {
		w = max(w, ansi.StringWidth(m.spec.Sort.Label), ansi.StringWidth(orderLabel))
	}
	m.lw.label, m.lw.width, m.lw.set = min(w, max((m.width-gutterWidth)/4, 4)), m.width, true
	return m.lw.label
}

// valueX returns the column where the values start.
func (m *Model) valueX() int { return gutterWidth + m.labelWidth() + labelGap }

// editorSize returns the size of a picker, which sits under the values, or
// under the labels too when the values are narrow.
func (m *Model) editorSize() (width, height int) {
	return m.width - m.editorX(), min(m.editorHeight, max(m.height-4, 3))
}

func (m *Model) editorX() int {
	if x := m.valueX(); m.width-x >= 24 {
		return x
	}
	return gutterWidth
}

// render renders the view for the current state.
func (m *Model) render() {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		m.view = ""
		return
	}
	m.layoutInputs()

	query := m.queryLines(w)
	var helpLine, tabLine string
	if m.helpLine {
		helpLine = m.helpView(w)
	}
	if m.tabBar && m.tabbed() {
		tabLine = m.tabLine(w)
	}
	if m.glyphs.ruleWidth != w {
		m.glyphs.rule, m.glyphs.ruleWidth = m.styles.Rule.Render(strings.Repeat(m.styles.Glyphs.Rule, w)), w
	}
	rule := m.glyphs.rule
	// Give the rows at least a line: drop the help, the tabs, the query's
	// second line, then the rule.
	bottomLen := func() int {
		n := 1 + len(query)
		if helpLine != "" {
			n++
		}
		if tabLine != "" {
			n++
		}
		return n
	}
	if h-bottomLen() < 1 {
		helpLine = ""
	}
	if h-bottomLen() < 1 {
		tabLine = ""
	}
	if h-bottomLen() < 1 && len(query) > 1 {
		query = query[:1]
	}
	if h-bottomLen() < 1 {
		rule = ""
	}
	n := h - len(query)
	if helpLine != "" {
		n--
	}
	if tabLine != "" {
		n--
	}
	if rule != "" {
		n--
	}

	lines := make([]string, 0, h)
	if tabLine != "" {
		lines = append(lines, tabLine)
	}
	lines = m.appendRows(lines, w, max(n, 0))
	if rule != "" {
		lines = append(lines, rule)
	}
	lines = append(lines, query...)
	if helpLine != "" {
		lines = append(lines, helpLine)
	}
	m.view = strings.Join(lines[:min(len(lines), h)], "\n")
}

// tabLine renders the names of the tabs, the one on view in the active
// style, the others muted.
func (m *Model) tabLine(w int) string {
	if c := &m.cache.tabs; c.ok && c.tab == m.tab && c.width == w {
		return c.line
	}
	var b strings.Builder
	b.WriteString("  ")
	for i, name := range tabNames {
		if i > 0 {
			b.WriteString(m.styles.Tab.Render(m.styles.Glyphs.Separator))
		}
		st := m.styles.Tab
		if Tab(i) == m.tab {
			st = m.styles.ActiveTab
		}
		b.WriteString(st.Render(name))
	}
	line := m.fit(b.String(), w)
	m.cache.tabs = tabCache{ok: true, tab: m.tab, width: w, line: line}
	return line
}

// layoutInputs sizes the text inputs and the picker to the current width.
func (m *Model) layoutInputs() {
	// An input draws one cell more than its width, for the cursor.
	if tw := max(m.width-m.valueX()-1, 1); m.text.Width() != tw {
		m.text.SetWidth(tw)
		m.text.SetCursor(m.text.Position())
	}
	if qw := max(m.width-gutterWidth-1, 1); m.query.Width() != qw {
		m.query.SetWidth(qw)
		m.query.SetCursor(m.query.Position())
	}
	if m.picking {
		if w, h := m.editorSize(); m.pick.Width() != w || m.pick.Height() != h {
			m.pick.SetSize(w, h)
		}
	}
}

// appendRows appends n lines of rows, scrolled so the row in focus and its
// editor show.
func (m *Model) appendRows(lines []string, w, n int) []string {
	if n <= 0 {
		return lines
	}
	var rows []string
	start, end := 0, 0
	for r := range m.queryRow() {
		if r == m.row {
			start = len(rows)
		}
		rows = append(rows, m.cachedRowLines(r, w)...)
		if r == m.row {
			rows = append(rows, m.editorLines(w)...)
			end = len(rows)
		}
	}
	if end > start {
		switch {
		case end-start > n || start < m.top:
			m.top = start
		case end > m.top+n:
			m.top = end - n
		}
	}
	m.top = max(min(m.top, len(rows)-n), 0)
	rows = rows[m.top:min(m.top+n, len(rows))]
	lines = append(lines, rows...)
	blank := strings.Repeat(" ", w)
	for range n - len(rows) {
		lines = append(lines, blank)
	}
	return lines
}

// rowLines renders row r: its label, then its value, on a second line too
// when the chips or choices don't fit on one.
func (m *Model) rowLines(r, w int) []string {
	focused := m.focused && r == m.row
	lw := m.labelWidth()
	var label string
	switch {
	case m.tab == FiltersTab:
		label = m.spec.Fields[r].Label
	case r == sortByRow:
		label = m.spec.Sort.Label
	default:
		label = orderLabel
	}
	var b strings.Builder
	if focused {
		b.WriteString(m.glyphs.gutter)
	} else {
		b.WriteString("  ")
	}
	st := m.styles.Label
	if focused {
		st = m.styles.FocusedLabel
	}
	b.WriteString(st.Render(m.fit(label, lw)))
	b.WriteString(strings.Repeat(" ", labelGap))
	first := b.String()

	vw := w - m.valueX()
	if vw < 1 {
		return []string{m.fit(first, w)}
	}
	var segs []string
	if m.tab == SortTab {
		segs = m.sortSegments(r, focused)
	} else {
		segs = m.fieldSegments(r, focused, vw)
	}
	vals := wrap(segs, segSep, vw, 2)
	out := make([]string, len(vals))
	indent := strings.Repeat(" ", m.valueX())
	for i, v := range vals {
		if i == 0 {
			out[i] = m.fit(first+v, w)
		} else {
			out[i] = m.fit(indent+v, w)
		}
	}
	return out
}

// fieldSegments renders the parts of field i's value, which wrap as words.
func (m *Model) fieldSegments(i int, focused bool, vw int) []string {
	f, v, fs := &m.spec.Fields[i], m.state.values[i], m.fields[i]
	s := m.styles
	switch f.Kind {
	case Choice:
		segs := make([]string, len(f.Options))
		for j, opt := range f.Options {
			segs[j] = m.radio(opt.Label, opt.Value == v.text, focused)
		}
		return segs
	case Toggle:
		box := s.Option.Render(boxOff)
		switch {
		case v.on && focused:
			box = s.Active.Render(boxOn)
		case v.on:
			box = s.Selected.Render(boxOn)
		}
		if f.Hint == "" {
			return []string{box}
		}
		return []string{box + " " + s.Value.Render(f.Hint)}
	case Multi:
		items := m.items(i)
		segs := make([]string, 0, len(v.list)+2)
		for j, val := range v.list {
			st := s.Chip
			if focused && !m.editing && fs.chip == j {
				st = s.ActiveChip
			}
			segs = append(segs, st.Render(labelOf(items, val))+m.glyphs.remove)
		}
		if len(v.list) == 0 && f.Hint != "" {
			segs = append(segs, s.Hint.Render(f.Hint))
		}
		add := s.Add
		if focused && !m.editing && fs.chip >= len(v.list) {
			add = s.Active
		}
		segs = append(segs, add.Render(addText))
		return append(segs, m.loadSegment(i)...)
	case Person:
		var seg string
		if v.text == "" {
			seg = s.Hint.Render(orDefault(f.Hint, "anyone"))
		} else {
			seg = s.Value.Render(labelOf(m.items(i), v.text))
		}
		return append([]string{seg + " " + s.Option.Render(m.styles.Glyphs.Drop)}, m.loadSegment(i)...)
	case Text:
		if m.editing && m.row == i {
			return []string{m.fit(m.text.View(), vw)}
		}
		if v.text == "" {
			return []string{s.Hint.Render(f.Hint)}
		}
		return []string{s.Value.Render(v.text)}
	default:
		return nil
	}
}

// loadSegment renders the state of field i's load, if it is loading or
// failed and its editor, which says more, is closed.
func (m *Model) loadSegment(i int) []string {
	if m.editing && m.row == i {
		return nil
	}
	switch m.fields[i].state {
	case loading:
		return []string{m.spin.View() + m.styles.Hint.Render("loading"+m.styles.Glyphs.Ellipsis)}
	case failed:
		if text, _ := m.errorWords(m.fields[i].err, ""); text == "" {
			return nil
		}
		return []string{m.styles.Error.Render(m.styles.ErrorGlyph + " couldn't load")}
	default:
		return nil
	}
}

// radio renders an option of a choice: marked when on, and in the active
// style when on in the row in focus.
func (m *Model) radio(label string, on, focused bool) string {
	switch {
	case on && focused:
		return m.styles.Active.Render(m.styles.Glyphs.On + " " + label)
	case on:
		return m.styles.Selected.Render(m.styles.Glyphs.On + " " + label)
	default:
		return m.styles.Option.Render(m.styles.Glyphs.Off + " " + label)
	}
}

// sortSegments renders the parts of row r of the Sort tab: the options
// sorted by, or the orders of the one chosen.
func (m *Model) sortSegments(r int, focused bool) []string {
	sf, so := m.spec.Sort, m.state.sort
	if r == sortByRow {
		segs := make([]string, len(sf.Options))
		for j, opt := range sf.Options {
			segs[j] = m.radio(opt.Label, opt.Value == so.By, focused)
		}
		return segs
	}
	i := sf.index(so.By)
	if so.By == "" || i < 0 {
		label := "this sort"
		if i >= 0 {
			label = sf.Options[i].Label
		}
		return []string{m.styles.Hint.Render("none for " + strings.ToLower(label))}
	}
	opt := sf.Options[i]
	return []string{
		m.radio(m.styles.Glyphs.Down+" "+opt.Desc, so.Desc, focused),
		m.radio(m.styles.Glyphs.Up+" "+opt.Asc, !so.Desc, focused),
	}
}

// editorLines renders the open editor of the row in focus: its picker, or
// the state of its load.
func (m *Model) editorLines(w int) []string {
	if !m.editing || m.kind() == Text {
		return nil
	}
	x := m.editorX()
	indent := strings.Repeat(" ", x)
	if m.picking {
		pv := strings.Split(m.pick.View(), "\n")
		out := make([]string, len(pv))
		for i, l := range pv {
			out[i] = m.fit(indent+l, w)
		}
		return out
	}
	fs, label := m.fields[m.row], strings.ToLower(m.spec.Fields[m.row].Label)
	if fs.state == failed {
		text, hint := m.errorWords(fs.err, label)
		if text == "" {
			return nil
		}
		back := m.keys.Cancel.Help().Key + " to go back"
		if hint != "" {
			back = hint + m.styles.ErrorSeparator + back
		}
		cut := m.styles.ErrorEllipsis
		return []string{
			fitCut(indent+m.styles.Error.Render(m.styles.ErrorGlyph+" "+text), w, cut),
			fitCut(indent+m.styles.Hint.Render(back), w, cut),
		}
	}
	return []string{m.fit(indent+m.spin.View()+m.styles.Hint.Render("Loading "+label+m.styles.Glyphs.Ellipsis), w)}
}

// errorWords returns what the editor says of err, the failed load of the
// options of the field called label, and the hint after it.
func (m *Model) errorWords(err error, label string) (text, hint string) {
	if m.errorText != nil && err != nil {
		return m.errorText(err)
	}
	msg := "unknown error"
	if err != nil {
		msg, _, _ = strings.Cut(err.Error(), "\n")
	}
	if k := m.keys.Edit.Help().Key; k != "" {
		hint = k + " to retry"
	}
	return "Couldn't load " + label + ": " + msg, hint
}

// queryLines renders the query: the input while it has focus, and the
// query wrapped onto at most two lines otherwise.
func (m *Model) queryLines(w int) []string {
	if m.focused && m.row == m.queryRow() {
		return []string{m.fit(m.glyphs.gutter+m.query.View(), w)}
	}
	q := m.Query()
	if c := &m.cache.query; c.ok && c.text == q && c.width == w {
		return slices.Clone(c.lines)
	}
	var lines []string
	if q == "" {
		lines = []string{m.fit("  "+m.styles.Hint.Render(m.query.Placeholder), w)}
	} else {
		toks := Tokenize(q)
		words := make([]string, len(toks))
		for i, tok := range toks {
			words[i] = tok.Raw
		}
		lines = wrap(words, " ", max(w-gutterWidth, 1), 2)
		for i, l := range lines {
			lines[i] = m.fit("  "+m.styles.Query.Render(l), w)
		}
	}
	m.cache.query = queryCache{ok: true, text: q, width: w, lines: lines}
	return slices.Clone(lines)
}

// helpView renders the help line for the row in focus.
func (m *Model) helpView(w int) string {
	bindings := m.shortHelp()
	var b strings.Builder
	for _, k := range bindings {
		b.WriteString(k.Help().Key)
		b.WriteByte(0)
		b.WriteString(k.Help().Desc)
		b.WriteByte(0)
	}
	if c := &m.cache.help; c.ok && c.key == b.String() && c.width == w {
		return c.line
	}
	m.help.SetWidth(w)
	line := m.fit(m.help.ShortHelpView(bindings), w)
	m.cache.help = helpCache{ok: true, key: b.String(), width: w, line: line}
	return line
}

// labelOf returns the label of the item with value v, or v itself, on one
// line, since items come from elsewhere, such as the labels of a
// repository.
func labelOf(items []Item, v string) string {
	for _, it := range items {
		if it.Value == v && it.Label != "" {
			return termtext.OneLine(it.Label)
		}
	}
	return termtext.OneLine(v)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// segSep separates the parts of a value.
const segSep = "  "

// wrap lays segs out on at most maxLines lines of width w, sep apart. What
// doesn't fit goes on the last line, to be truncated.
func wrap(segs []string, sep string, w, maxLines int) []string {
	lines := make([]string, 0, maxLines)
	var cur strings.Builder
	curW := 0
	for i, s := range segs {
		sw := ansi.StringWidth(s)
		if curW > 0 && curW+len(sep)+sw > w {
			if len(lines)+1 == maxLines {
				cur.WriteString(sep)
				cur.WriteString(strings.Join(segs[i:], sep))
				break
			}
			lines = append(lines, cur.String())
			cur.Reset()
			curW = 0
		}
		if curW > 0 {
			cur.WriteString(sep)
			curW += len(sep)
		}
		cur.WriteString(s)
		curW += sw
	}
	return append(lines, cur.String())
}

// fit truncates or pads styled text to exactly width cells.
// fit truncates styled text to exactly width cells, ending it with the
// ellipsis where it cuts, or pads it.
func (m *Model) fit(s string, width int) string { return fitCut(s, width, m.styles.Glyphs.Ellipsis) }

// fitCut is fit, ending text that is cut with tail.
func fitCut(s string, width int, tail string) string {
	w := ansi.StringWidth(s)
	if w > width {
		s = termtext.Truncate(s, width, tail)
		w = ansi.StringWidth(s)
	}
	if w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}

// renderCache holds rendered pieces by what they were rendered from, so a
// key only renders what it changed. Copies of the model share rows, which
// is safe since an entry is only used when its key matches; SetStyles
// replaces the whole cache.
type renderCache struct {
	// rows holds the rows of each tab.
	rows  [numTabs][]rowCache
	query queryCache
	help  helpCache
	tabs  tabCache
}

type rowCache struct {
	key   rowKey
	lines []string
}

// rowKey is everything a row's lines are rendered from, besides the spec
// and the styles.
type rowKey struct {
	ok      bool
	width   int
	focused bool
	text    string
	list    string
	on      bool
	chip    int
	items   int
	desc    bool
	editing bool
	state   loadState
}

type queryCache struct {
	ok    bool
	text  string
	width int
	lines []string
}

type tabCache struct {
	ok    bool
	tab   Tab
	width int
	line  string
}

type helpCache struct {
	ok    bool
	key   string
	width int
	line  string
}

// cachedRowLines returns rowLines(r, w), rendered again only when the row
// changed. A row that shows a text input or a spinner is always rendered.
func (m *Model) cachedRowLines(r, w int) []string {
	k := rowKey{ok: true, width: w, focused: m.focused && r == m.row, editing: m.editing && r == m.row}
	if m.tab == SortTab {
		k.text, k.desc = m.state.sort.By, m.state.sort.Desc
	} else {
		v, fs := m.state.values[r], m.fields[r]
		if fs.state == loading || k.editing && m.spec.Fields[r].Kind == Text {
			return m.rowLines(r, w)
		}
		k.text, k.list, k.on = v.text, strings.Join(v.list, "\x00"), v.on
		k.chip, k.items, k.state = fs.chip, len(fs.items), fs.state
	}
	rows := &m.cache.rows[m.tab]
	if len(*rows) != m.queryRow() {
		*rows = make([]rowCache, m.queryRow())
	}
	if c := (*rows)[r]; c.key == k {
		return c.lines
	}
	lines := m.rowLines(r, w)
	(*rows)[r] = rowCache{key: k, lines: lines}
	return lines
}
