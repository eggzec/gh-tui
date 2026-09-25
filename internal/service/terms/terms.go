// Package terms reads the words of a GitHub search query, so that a
// service can tell whether a list answers it without a search.
package terms

import (
	"strings"
	"unicode"
)

// Term is one word of a query: a qualifier such as label:"good first
// issue" or -is:draft, or a free word.
type Term struct {
	// Key is the qualifier in lower case, without the minus that negates
	// it, or "" for a free word.
	Key string
	// Not is set for a negated qualifier, such as -label:bug.
	Not bool
	// Value is what follows the colon without its quotes, or the whole
	// word for a free word.
	Value string
	// raw is what follows the colon, quotes included, so that a list is
	// split on the commas outside them.
	raw string
}

// Values splits the value on the commas outside quotes, as in
// label:bug,"good first issue", which GitHub reads as any of them, and
// drops empty items.
func (t Term) Values() []string {
	var out []string
	for part := range split(t.raw, ',') {
		if v := unquote(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// Parse splits q into its terms on the spaces outside quotes.
func Parse(q string) []Term {
	var out []Term
	for word := range split(q, ' ') {
		if word = strings.TrimSpace(word); word != "" {
			out = append(out, parse(word))
		}
	}
	return out
}

func parse(word string) Term {
	i := strings.IndexByte(word, ':')
	if i <= 0 || !isKey(word[:i]) {
		return Term{Value: unquote(word), raw: word}
	}
	key, not := strings.CutPrefix(word[:i], "-")
	return Term{Key: strings.ToLower(key), Not: not, Value: unquote(word[i+1:]), raw: word[i+1:]}
}

// isKey reports whether s names a qualifier: letters, digits, dashes and
// underscores, after an optional minus.
func isKey(s string) bool {
	s = strings.TrimPrefix(s, "-")
	return s != "" && !strings.ContainsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_'
	})
}

// split yields the parts of s between the seps outside double quotes. Any
// space separates when sep is a space.
func split(s string, sep rune) func(yield func(string) bool) {
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

// unquote drops the quotes of a value. GitHub has no escapes, so a value
// can't hold a quote.
func unquote(s string) string { return strings.ReplaceAll(s, `"`, "") }

// Sort reads the value of a sort: qualifier, such as created-asc, into
// what is sorted by and whether the order is ascending. A sort without a
// direction is descending, as on GitHub.
func Sort(value string) (by string, asc bool) {
	by = strings.ToLower(value)
	if b, ok := strings.CutSuffix(by, "-asc"); ok {
		return b, true
	}
	return strings.TrimSuffix(by, "-desc"), false
}
