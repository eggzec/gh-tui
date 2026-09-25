package filterform

import (
	"slices"
	"strings"
	"unicode"
)

// Token is one word of a query: a qualifier such as label:"good first
// issue", or a free word.
type Token struct {
	// Raw is the token as written, quotes included.
	Raw string
	// Qualifier is the part before the colon, such as label or -label, or
	// "" for a free word.
	Qualifier string
	// Value is the part after the colon with its quotes removed, or the
	// whole word for a free word.
	Value string
	// rawValue is the part after the colon, quotes included, so a list can
	// be split on the commas outside them.
	rawValue string
}

// Values splits the token's value on the commas outside quotes, as in
// label:bug,"good first issue", and drops empty items.
func (t Token) Values() []string {
	var out []string
	for part := range splitOutside(t.rawValue, ',') {
		if v := unquote(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// Tokenize splits a query into tokens on the spaces outside quotes. An
// unclosed quote runs to the end.
func Tokenize(q string) []Token {
	var toks []Token
	for raw := range splitOutside(q, ' ') {
		if raw = strings.TrimSpace(raw); raw != "" {
			toks = append(toks, newToken(raw))
		}
	}
	return toks
}

func newToken(raw string) Token {
	t := Token{Raw: raw, Value: unquote(raw), rawValue: raw}
	i := strings.IndexByte(raw, ':')
	if i <= 0 || !isQualifier(raw[:i]) {
		return t
	}
	t.Qualifier, t.rawValue = raw[:i], raw[i+1:]
	t.Value = unquote(t.rawValue)
	return t
}

// isQualifier reports whether s can name a qualifier: letters, digits,
// dashes and underscores, after an optional minus that negates it.
func isQualifier(s string) bool {
	s = strings.TrimPrefix(s, "-")
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// splitOutside yields the parts of s between the seps outside double
// quotes. Whitespace counts as a separator when sep is a space.
func splitOutside(s string, sep rune) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		quoted, start := false, 0
		for i, r := range s {
			switch {
			case r == '"':
				quoted = !quoted
			case !quoted && (r == sep || sep == ' ' && unicode.IsSpace(r)):
				if !yield(s[start:i]) {
					return
				}
				start = i + len(string(r))
			}
		}
		yield(s[start:])
	}
}

// unquote drops the double quotes around, or in, a value. GitHub has no
// escapes, so a value can't hold a quote.
func unquote(s string) string { return strings.ReplaceAll(s, `"`, "") }

// quote quotes a value that holds a space or a comma, which would split it.
func quote(s string) string {
	s = unquote(s)
	if strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == ',' }) {
		return `"` + s + `"`
	}
	return s
}

// write returns the query text of f's value v, or "" for none.
func write(f *Field, v Value) string {
	if f.Format != nil {
		return f.Format(v)
	}
	switch f.Kind {
	case Toggle:
		if v.on {
			return f.Qualifier
		}
		return ""
	case Multi:
		if len(v.list) == 0 || f.Qualifier == "" {
			return ""
		}
		parts := make([]string, len(v.list))
		for i, s := range v.list {
			parts[i] = quote(s)
		}
		if !f.Each {
			return f.Qualifier + ":" + strings.Join(parts, ",")
		}
		for i := range parts {
			parts[i] = f.Qualifier + ":" + parts[i]
		}
		return strings.Join(parts, " ")
	default:
		if v.text == "" {
			return ""
		}
		if f.Qualifier == "" {
			return v.text
		}
		return f.Qualifier + ":" + quote(v.text)
	}
}

// claim returns f's value after tok, and whether f takes tok.
func claim(f *Field, tok Token, v Value) (Value, bool) {
	if f.Parse != nil {
		return f.Parse(tok, v)
	}
	if f.Kind == Toggle {
		if f.Qualifier != "" && strings.EqualFold(tok.Raw, f.Qualifier) {
			return BoolValue(true), true
		}
		return v, false
	}
	if f.Kind == Choice && f.Qualifier == "" {
		if opt, ok := findItem(f.Options, tok.Raw); ok {
			return TextValue(opt.Value), true
		}
		return v, false
	}
	if f.Qualifier == "" || !strings.EqualFold(tok.Qualifier, f.Qualifier) || tok.Value == "" {
		return v, false
	}
	switch f.Kind {
	case Choice:
		if opt, ok := findItem(f.Options, tok.Value); ok {
			return TextValue(opt.Value), true
		}
		return v, false
	case Multi:
		list := slices.Clone(v.list)
		for _, s := range tok.Values() {
			if !slices.Contains(list, s) {
				list = append(list, s)
			}
		}
		return Value{list: list}, true
	default:
		return TextValue(tok.Value), true
	}
}

// findItem returns the item whose non-empty value is s, ignoring case.
func findItem(items []Item, s string) (Item, bool) {
	for _, it := range items {
		if it.Value != "" && strings.EqualFold(it.Value, s) {
			return it, true
		}
	}
	return Item{}, false
}

// writeSort returns the sort token of s, or "" for none.
func writeSort(s Sort) string {
	if s.By == "" {
		return ""
	}
	if s.Desc {
		return "sort:" + s.By + "-desc"
	}
	return "sort:" + s.By + "-asc"
}

// claimSort returns the sort tok names, and whether it names one of sf's
// options. A sort without a direction is descending, as on GitHub.
func claimSort(sf *SortField, tok Token) (Sort, bool) {
	if !strings.EqualFold(tok.Qualifier, "sort") {
		return Sort{}, false
	}
	by, desc := tok.Value, true
	if i := strings.LastIndexByte(by, '-'); i > 0 {
		switch strings.ToLower(by[i+1:]) {
		case "desc":
			by = by[:i]
		case "asc":
			by, desc = by[:i], false
		}
	}
	opt, ok := findItem(sf.Options, by)
	if !ok {
		return Sort{}, false
	}
	return Sort{By: opt.Value, Desc: desc}, true
}

// state is what a form holds: a value per field, the sort, and the words no
// field claims.
type state struct {
	values []Value
	sort   Sort
	free   []string
}

// defaults returns the state the spec starts with.
func defaults(s *Spec) state {
	st := state{values: make([]Value, len(s.Fields))}
	for i := range s.Fields {
		st.values[i] = s.Fields[i].Default.clone()
	}
	if s.Sort != nil {
		st.sort = s.Sort.Default
	}
	return st
}

// cleared returns the state of an empty query: every field empty, except a
// Choice with no empty option and a sort with none, which keep their
// defaults since they can't be left out. A sort that can be left out keeps
// its default direction, which the option chosen next takes.
func cleared(s *Spec) state {
	st := state{values: make([]Value, len(s.Fields))}
	for i := range s.Fields {
		f := &s.Fields[i]
		if f.Kind == Choice && f.Parse == nil && !hasEmpty(f.Options) {
			st.values[i] = f.Default.clone()
		}
	}
	switch {
	case s.Sort == nil:
	case hasEmpty(s.Sort.Options):
		st.sort.Desc = s.Sort.Default.Desc
	default:
		st.sort = s.Sort.Default
	}
	return st
}

func hasEmpty(items []Item) bool {
	return slices.ContainsFunc(items, func(it Item) bool { return it.Value == "" })
}

// parse reads q into a state: each token goes to the first field that
// claims it, then to the sort, and is kept as free text otherwise.
func parse(s *Spec, q string) state {
	st := cleared(s)
	for _, tok := range Tokenize(q) {
		if st.take(s, tok) {
			continue
		}
		st.free = append(st.free, tok.Raw)
	}
	return st
}

func (st *state) take(s *Spec, tok Token) bool {
	for i := range s.Fields {
		if v, ok := claim(&s.Fields[i], tok, st.values[i]); ok {
			st.values[i] = v
			return true
		}
	}
	if s.Sort != nil {
		if so, ok := claimSort(s.Sort, tok); ok {
			st.sort = so
			return true
		}
	}
	return false
}

// query writes st as query text: the fields in order, the sort, then the
// free words in the order they were typed.
func (st *state) query(s *Spec) string {
	var b strings.Builder
	add := func(part string) {
		if part == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(part)
	}
	for i := range s.Fields {
		add(write(&s.Fields[i], st.values[i]))
	}
	add(writeSort(st.sort))
	for _, w := range st.free {
		add(w)
	}
	return b.String()
}
