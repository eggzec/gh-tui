package search

import (
	"slices"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

// candidate is one way to complete the query: Text replaces the span of the
// line from Start to End.
type candidate struct {
	Text       string
	Start, End int
}

// qualifier is a qualifier a kind's search takes, with the values it offers.
type qualifier struct {
	name   string
	values []string
}

// qualifiers returns the qualifiers the filter of kind writes, with the
// values its choices and sorts hold, in the order the filter lists them.
func qualifiers(kind core.SearchKind) []qualifier {
	if kind == core.SearchTopics {
		// Topics take no qualifiers.
		return nil
	}
	var out []qualifier
	add := func(name, value string) {
		i := slices.IndexFunc(out, func(q qualifier) bool { return q.name == name })
		if i < 0 {
			out = append(out, qualifier{name: name})
			i = len(out) - 1
		}
		if value != "" && !slices.Contains(out[i].values, value) {
			out[i].values = append(out[i].values, value)
		}
	}
	sp := spec(kind, "")
	for i := range sp.Fields {
		f := &sp.Fields[i]
		name := f.Qualifier
		if name == "" {
			// Such a field's choices write whole qualifiers.
			for _, o := range f.Options {
				if n, v, ok := strings.Cut(o.Value, ":"); ok {
					add(n, v)
				}
			}
			continue
		}
		if n, v, ok := strings.Cut(name, ":"); ok {
			add(n, v)
			continue
		}
		add(name, "")
		for _, o := range f.Options {
			add(name, o.Value)
		}
	}
	if sp.Sort != nil {
		add("sort", "")
		for _, o := range sp.Sort.Options {
			add("sort", o.Value)
		}
	}
	return out
}

// wordSpan returns the byte span of the word of line the cursor touches, or
// an empty span at the cursor when it sits between words. Words split the
// way the filter form reads a query (filterform.Tokenize): at any white space
// outside double quotes, an unclosed quote running to the end of the line.
// Splitting on plain spaces would end a word inside a quoted value and let a
// completion rewrite it.
func wordSpan(line string, cursor int) (start, end int) {
	inQuote := false
	start = -1
	for i, r := range line {
		if !inQuote && unicode.IsSpace(r) {
			if start >= 0 {
				if start <= cursor && cursor <= i {
					return start, i
				}
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
		if r == '"' {
			inQuote = !inQuote
		}
	}
	if start >= 0 && start <= cursor {
		return start, len(line)
	}
	return cursor, cursor
}

// completeQuery returns the candidates that complete the word of line under
// the cursor, a byte offset, for the qualifiers of kind: a name such as is:
// for a word with no colon, and a value such as is:open after one. A minus
// that negates the word stays. A word that is empty or holds a quote has no
// candidates.
func completeQuery(kind core.SearchKind, line string, cursor int) []candidate {
	if cursor < 0 || cursor > len(line) {
		return nil
	}
	start, end := wordSpan(line, cursor)
	word := line[start:end]
	if word == "" || strings.Contains(word, `"`) {
		return nil
	}
	neg := ""
	if strings.HasPrefix(word, "-") {
		neg = "-"
	}
	body := strings.TrimPrefix(word, "-")
	name, val, hasValue := strings.Cut(body, ":")
	if !hasValue && body == "" {
		return nil
	}
	var out []candidate
	push := func(text string) {
		if text != word {
			out = append(out, candidate{Text: text, Start: start, End: end})
		}
	}
	for _, q := range qualifiers(kind) {
		if !hasValue {
			if hasPrefixFold(q.name, body) {
				push(neg + q.name + ":")
			}
			continue
		}
		if !strings.EqualFold(q.name, name) {
			continue
		}
		for _, v := range q.values {
			if hasPrefixFold(v, val) {
				push(neg + q.name + ":" + v)
			}
		}
	}
	return out
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// completion is the candidate row of the query while it types.
type completion struct {
	cands []candidate
	// sel is the candidate inserted in the line, or -1 while the line is as
	// typed.
	sel int
	// line and cursor are the line as typed before the user cycled, which
	// the spans of cands refer to.
	line   string
	cursor int
}

// refreshComplete asks for the candidates of the query as it is, which ends
// any cycling.
func (s *Section) refreshComplete() {
	line := s.input.Value()
	cursor := len(string([]rune(line)[:min(s.input.Position(), len([]rune(line)))]))
	s.comp = completion{sel: -1, line: line, cursor: cursor}
	if s.typing {
		s.comp.cands = completeQuery(s.kind, line, cursor)
	}
}

// cycleComplete inserts the candidate d steps after the selected one. Past
// either end, it puts back the line as typed.
func (s *Section) cycleComplete(d int) {
	c := &s.comp
	n := len(c.cands)
	if n == 0 {
		return
	}
	c.sel = (c.sel+1+d+n+1)%(n+1) - 1
	line, cursor := c.line, c.cursor
	if c.sel >= 0 {
		cd := c.cands[c.sel]
		line = c.line[:cd.Start] + cd.Text + c.line[cd.End:]
		cursor = cd.Start + len(cd.Text)
	}
	s.input.SetValue(line)
	s.input.SetCursor(len([]rune(line[:cursor])))
}

// completeRow draws the candidates in w cells, the selected one accented, and
// leaves out those that do not fit. It is empty without candidates.
func (s *Section) completeRow(w int) string {
	c := &s.comp
	if len(c.cands) == 0 || w <= 0 {
		return ""
	}
	// Start at the first candidate that shows the selected one.
	first := 0
	for first <= max(c.sel, 0) {
		width, fits := 0, true
		for i := first; i <= max(c.sel, 0); i++ {
			width += ansi.StringWidth(c.cands[i].Text) + 2
		}
		if width-2 > w {
			fits = false
		}
		if fits {
			break
		}
		first++
	}
	var b strings.Builder
	used := 0
	for i := first; i < len(c.cands); i++ {
		t := c.cands[i].Text
		need := ansi.StringWidth(t)
		if used > 0 {
			need += 2
		}
		if used+need > w {
			break
		}
		if used > 0 {
			b.WriteString("  ")
		}
		if i == c.sel {
			b.WriteString(s.st.selected.render(t))
		} else {
			b.WriteString(s.st.muted.render(t))
		}
		used += need
	}
	return b.String()
}
