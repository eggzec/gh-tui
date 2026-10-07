package filterform

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/overlay"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// gutterWidth is the width of the mark before the row in focus.
const gutterWidth = 2

// labelGap is the space between the label column and the values.
const labelGap = 2

// View renders the form at exactly its width and height: the tabs, if it
// has them, the rows of the tab on view, then a rule, the query and the
// help line. An open dropdown floats over the rows and the query, under its
// row or above it, but never over the help line.
func (m Model) View() string { return m.view }

// insertLabel starts the help line in insert mode.
const insertLabel = "INSERT"

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

// DropdownExtra returns how many more lines the form needs for the
// dropdown it has open, beyond rows lines of rows and the rule and query
// under them, so that the dropdown fits under its row or above it. It is 0
// when none is open or one of the two has room enough. A parent that sizes
// the form to what it shows adds it to the rows; the lines go to the end of
// the rows, so they only add room under the dropdown's row.
func (m Model) DropdownExtra(rows, width int) int {
	if m.mode != listMode {
		return 0
	}
	want := m.dropHeight()
	// Lines under the row: the rows after it, the rule and the query; and
	// above it the rows before it.
	below := rows - m.row - 1 + 1 + m.QueryLines(width)
	above := m.row
	if below >= want || above >= want {
		return 0
	}
	return want - below
}

// editorX is the column a dropdown starts at: under the values, or under
// the labels too when the values are narrow.
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
	first := len(lines)
	lines, focusY := m.appendRows(lines, w, max(n, 0))
	if rule != "" {
		lines = append(lines, rule)
	}
	lines = append(lines, query...)
	lines = lines[:min(len(lines), h)]
	body := strings.Join(lines, "\n")
	// The dropdown floats over the lines above the help, so it never
	// covers the help.
	if box, x, y := m.dropdown(len(lines), first, focusY); box != "" {
		body = overlay.Place(body, box, x, y)
	}
	if helpLine != "" && len(lines) < h {
		body += "\n" + helpLine
	}
	m.view = body
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

// layoutInputs sizes the text inputs to the current width.
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
}

// appendRows appends n lines of rows, scrolled so the row in focus shows,
// and returns the lines and the index in them of the row in focus, or -1
// when it isn't one of them.
func (m *Model) appendRows(lines []string, w, n int) (out []string, focusY int) {
	focusY = -1
	if n <= 0 {
		return lines, focusY
	}
	rows := make([]string, 0, m.queryRow())
	for r := range m.queryRow() {
		rows = append(rows, m.cachedRowLines(r, w)...)
	}
	switch {
	case m.row >= len(rows):
	case m.row < m.top:
		m.top = m.row
	case m.row >= m.top+n:
		m.top = m.row - n + 1
	}
	m.top = max(min(m.top, len(rows)-n), 0)
	if m.row < len(rows) {
		focusY = len(lines) + m.row - m.top
	}
	rows = rows[m.top:min(m.top+n, len(rows))]
	lines = append(lines, rows...)
	blank := strings.Repeat(" ", w)
	for range n - len(rows) {
		lines = append(lines, blank)
	}
	return lines, focusY
}

// rowLines renders row r on one line: its label, then its value.
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
	switch {
	case focused && m.mode == insertMode:
		b.WriteString(m.glyphs.edge)
	case focused:
		b.WriteString(m.glyphs.gutter)
	default:
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
	return []string{m.fit(first+strings.Join(segs, segSep), w)}
}

// choiceText renders the value of a choice, which the row in focus wraps
// in the marks of the keys that change it.
func (m *Model) choiceText(label string, focused bool) string {
	if !focused {
		return m.styles.Selected.Render(label)
	}
	g := m.styles.Glyphs
	return m.styles.Active.Render(g.Prev + " " + label + " " + g.Next)
}

// fieldSegments renders the parts of field i's value.
func (m *Model) fieldSegments(i int, focused bool, vw int) []string {
	f, v := &m.spec.Fields[i], m.state.values[i]
	s := m.styles
	switch f.Kind {
	case Choice:
		// A value that is no option, such as one the query had, shows as
		// it is.
		j := slices.IndexFunc(f.Options, func(it Item) bool { return it.Value == v.text })
		switch {
		case j >= 0:
			return []string{m.choiceText(optionLabel(f.Options[j].Label, f.Options[j].Value), focused)}
		case v.text != "":
			return []string{m.choiceText(termtext.OneLine(v.text), focused)}
		}
		return []string{m.choiceText(orDefault(f.Hint, "any"), focused)}
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
		drop := " " + s.Option.Render(s.Glyphs.Drop)
		avail := vw - 1 - ansi.StringWidth(s.Glyphs.Drop)
		return append([]string{m.multiText(i, avail) + drop}, m.loadSegment(i)...)
	case Person:
		var seg string
		if v.text == "" {
			seg = s.Hint.Render(orDefault(f.Hint, "anyone"))
		} else {
			seg = s.Value.Render(labelOf(m.items(i), v.text))
		}
		return append([]string{seg + " " + s.Option.Render(m.styles.Glyphs.Drop)}, m.loadSegment(i)...)
	case Text:
		if m.mode == insertMode && m.row == i {
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

// multiText renders the labels of the Multi field i joined with commas, as
// many as fit in width cells, then how many more there are, as in "bug,
// docs +2". A field with none shows its hint.
func (m *Model) multiText(i, width int) string {
	s := m.styles
	list := m.state.values[i].list
	if len(list) == 0 {
		return s.Hint.Render(orDefault(m.spec.Fields[i].Hint, "any"))
	}
	items := m.items(i)
	names := make([]string, len(list))
	for j, val := range list {
		names[j] = labelOf(items, val)
	}
	more := func(n int) string {
		if n == 0 {
			return ""
		}
		return " +" + strconv.Itoa(n)
	}
	count := func(n int) string {
		if n == 0 {
			return ""
		}
		return s.Hint.Render(more(n))
	}
	for n := len(names); n > 1; n-- {
		if text := strings.Join(names[:n], ", "); ansi.StringWidth(text+more(len(names)-n)) <= width {
			return s.Value.Render(text) + count(len(names)-n)
		}
	}
	// Not even two fit: the first, cut to leave room for the count.
	rest := more(len(names) - 1)
	name := termtext.Truncate(names[0], max(width-ansi.StringWidth(rest), 1), s.Glyphs.Ellipsis)
	return s.Value.Render(name) + count(len(names)-1)
}

// optionLabel returns the label of an option, or its value if it has no
// label.
func optionLabel(label, value string) string {
	return termtext.OneLine(orDefault(label, value))
}

// loadSegment renders the state of field i's load, if it is loading or
// failed and its dropdown, which says more, is closed.
func (m *Model) loadSegment(i int) []string {
	if m.mode == listMode && m.row == i {
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

// sortSegments renders the parts of row r of the Sort tab: what is sorted
// by, or the order of the sort chosen.
func (m *Model) sortSegments(r int, focused bool) []string {
	sf, so := m.spec.Sort, m.state.sort
	i := sf.index(so.By)
	if r == sortByRow {
		if i < 0 {
			return []string{m.choiceText(termtext.OneLine(so.By), focused)}
		}
		return []string{m.choiceText(optionLabel(sf.Options[i].Label, sf.Options[i].Value), focused)}
	}
	if so.By == "" || i < 0 {
		label := "this sort"
		if i >= 0 {
			label = sf.Options[i].Label
		}
		return []string{m.styles.Hint.Render("none for " + strings.ToLower(label))}
	}
	opt, g := sf.Options[i], m.styles.Glyphs
	if so.Desc {
		return []string{m.choiceText(g.Down+" "+opt.Desc, focused)}
	}
	return []string{m.choiceText(g.Up+" "+opt.Asc, focused)}
}

// statusLines renders what the dropdown says while its options load or
// when that failed: the spinner, or the error and the keys that retry and
// close it.
func (m *Model) statusLines() []string {
	fs := m.fields[m.row]
	label := strings.ToLower(m.spec.Fields[m.row].Label)
	if fs.state != failed {
		return []string{m.spin.View() + m.styles.Hint.Render("Loading "+label+m.styles.Glyphs.Ellipsis)}
	}
	text, hint := m.errorWords(fs.err, label)
	var out []string
	if text != "" {
		out = append(out, m.styles.Error.Render(m.styles.ErrorGlyph+" "+text))
	}
	back := m.name(m.keys.List.Cancel.Help().Key) + " to close"
	if hint != "" {
		back = hint + m.styles.ErrorSeparator + back
	}
	return append(out, m.styles.Hint.Render(back))
}

// name writes a key as the parent wants it written.
func (m *Model) name(s string) string {
	if m.keyName == nil {
		return s
	}
	return m.keyName(s)
}

// errorWords returns what the dropdown says of err, the failed load of the
// options of the field called label, and the hint after it, which names the
// key that retries.
func (m *Model) errorWords(err error, label string) (text, hint string) {
	if m.errorText != nil && err != nil {
		return m.errorText(err)
	}
	msg := "unknown error"
	if err != nil {
		msg, _, _ = strings.Cut(err.Error(), "\n")
	}
	if k := m.keys.Retry.Help().Key; k != "" {
		hint = k + " to retry"
	}
	return "Couldn't load " + label + ": " + msg, hint
}

// queryLines renders the query: the input in insert mode, and the query
// wrapped onto at most two lines otherwise, with the mark of the cursor
// when it has focus.
func (m *Model) queryLines(w int) []string {
	focused := m.focused && m.row == m.queryRow()
	if focused && m.mode == insertMode {
		// The input is one line, but the rows keep the room the query
		// takes, so that they don't move while it is typed in.
		extra := len(m.shownQuery(w, false)) - 1
		lines := make([]string, 1, 1+max(extra, 0))
		lines[0] = m.fit(m.glyphs.edge+m.query.View(), w)
		for range extra {
			lines = append(lines, strings.Repeat(" ", w))
		}
		return lines
	}
	return m.shownQuery(w, focused)
}

// QueryLines returns the number of lines the query takes, one or two, in a
// form width cells wide.
func (m Model) QueryLines(width int) int { return len(m.shownQuery(width, false)) }

// shownQuery returns the query wrapped onto at most two lines, with the
// mark of the cursor on the first if focused.
func (m *Model) shownQuery(w int, focused bool) []string {
	q := m.Query()
	if c := &m.cache.query; c.ok && c.text == q && c.width == w && c.focused == focused {
		return slices.Clone(c.lines)
	}
	var lines []string
	if q == "" {
		lines = []string{m.fit(m.queryGutter(focused, true)+m.styles.Hint.Render(m.query.Placeholder), w)}
	} else {
		toks := Tokenize(q)
		words := make([]string, len(toks))
		for i, tok := range toks {
			words[i] = tok.Raw
		}
		lines = wrap(words, " ", max(w-gutterWidth, 1), 2)
		for i, l := range lines {
			lines[i] = m.fit(m.queryGutter(focused, i == 0)+m.styles.Query.Render(l), w)
		}
	}
	m.cache.query = queryCache{ok: true, text: q, width: w, focused: focused, lines: lines}
	return slices.Clone(lines)
}

// queryGutter returns what starts a line of the query: the cursor mark on
// the first line when the query has focus.
func (m *Model) queryGutter(focused, first bool) string {
	if focused && first {
		return m.glyphs.gutter
	}
	return "  "
}

// helpView renders the help line for the row in focus. When it is too wide
// the hints that matter least go first, then what is left is cut.
func (m *Model) helpView(w int) string {
	items := m.helpItems()
	name := m.keyName
	if name == nil {
		name = func(s string) string { return s }
	}
	var b strings.Builder
	b.WriteByte(byte('0' + m.mode))
	typing := m.mode == insertMode || m.mode == listMode && m.picking && m.pick.Typing()
	if typing {
		b.WriteByte('i')
	}
	type named struct{ key, desc string }
	ps := make([]named, len(items))
	for i, it := range items {
		h := it.Help()
		ps[i] = named{name(h.Key), name(h.Desc)}
		b.WriteString(ps[i].key + "\x00" + ps[i].desc + "\x00")
	}
	if c := &m.cache.help; c.ok && c.key == b.String() && c.width == w {
		return c.line
	}
	st := m.styles.Help
	sep := st.ShortSeparator.Render(m.styles.Glyphs.Separator)
	render := func() string {
		parts := make([]string, 0, len(ps)+1)
		if typing {
			parts = append(parts, m.styles.Mode.Render(insertLabel))
		}
		for _, p := range ps {
			parts = append(parts, st.ShortKey.Inline(true).Render(p.key)+" "+st.ShortDesc.Inline(true).Render(p.desc))
		}
		return strings.Join(parts, sep)
	}
	line := render()
	for ansi.StringWidth(line) > w {
		// Drop the last hint of the highest rank, unless all are kept.
		drop, top := -1, rankKeep
		for i, it := range items {
			if it.rank >= top && it.rank > rankKeep {
				drop, top = i, it.rank
			}
		}
		if drop < 0 {
			break
		}
		items = slices.Delete(items, drop, drop+1)
		ps = slices.Delete(ps, drop, drop+1)
		line = render()
	}
	line = m.fit(line, w)
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
	items   int
	desc    bool
	editing bool
	insert  bool
	state   loadState
}

type queryCache struct {
	ok      bool
	text    string
	width   int
	focused bool
	lines   []string
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
	k := rowKey{
		ok: true, width: w, focused: m.focused && r == m.row,
		editing: m.mode != rowsMode && r == m.row, insert: m.mode == insertMode && r == m.row,
	}
	if m.tab == SortTab {
		k.text, k.desc = m.state.sort.By, m.state.sort.Desc
	} else {
		v, fs := m.state.values[r], m.fields[r]
		if fs.state == loading || k.insert && m.spec.Fields[r].Kind == Text {
			return m.rowLines(r, w)
		}
		k.text, k.list, k.on = v.text, strings.Join(v.list, "\x00"), v.on
		k.items, k.state = len(fs.items), fs.state
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
