package github

import (
	"cmp"
	"errors"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// A GitHub Enterprise Server lags behind github.com, and GitHub refuses a
// whole query that selects a field its schema lacks, before it runs any of
// it. So when GitHub says that a field doesn't exist, the client sends the
// query once more without it, and sends it so from then on: the answer
// leaves the field out, which the decoders take for its zero value. A
// field that can't be left out, as the only one of its selection, stays,
// and the query fails with core.ErrUnsupported on an Enterprise Server.

// unsupported remembers, for the session, the queries that GitHub said
// select fields its schema lacks, and what is sent in their place. It is
// safe for concurrent use.
type unsupported struct {
	mu sync.Mutex
	// sent maps a query to the query sent in its place.
	sent map[string]string
}

// rewrite returns what to send for query: query itself, unless it selects
// fields GitHub said it lacks.
func (u *unsupported) rewrite(query string) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return cmp.Or(u.sent[query], query)
}

// remember sends sent in place of query from now on.
func (u *unsupported) remember(query, sent string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.sent == nil {
		u.sent = map[string]string{}
	}
	u.sent[query] = sent
}

// schemaCodes are the codes of GitHub's errors of a query that its schema
// can't answer, which it refuses before running any of it.
var schemaCodes = []string{
	"undefinedField", "undefinedType", "argumentNotAccepted", "argumentLiteralsIncompatible",
	"missingRequiredArguments", "variableRequiresValidType",
}

// schemaError reports whether item is GitHub's for a query its schema
// can't answer, such as one that selects a field it lacks.
func (item *GraphQLErrorItem) schemaError() bool {
	return item.Extensions != nil && slices.Contains(schemaCodes, item.Extensions.Code)
}

// missingField returns the field that item says the schema lacks, or
// false if it says something else.
func (item *GraphQLErrorItem) missingField() (string, bool) {
	e := item.Extensions
	if e == nil || e.Code != "undefinedField" || e.FieldName == "" || len(item.Locations) == 0 {
		return "", false
	}
	return e.FieldName, true
}

// withoutMissing returns query, which failed with err, without the fields
// that err says the schema lacks, and those fields as Type.field. It
// returns false when err isn't only of missing fields, or a field can't be
// left out: the only one of its selection, one GitHub didn't point to, one
// whose variable nothing else uses, or one whose selection alone spreads a
// fragment, which GitHub refuses to be left unused.
func withoutMissing(query string, err error) (fewer string, fields []string, ok bool) {
	e, ok := errors.AsType[*GraphQLError](err)
	if !ok || len(e.Errors) == 0 {
		return "", nil, false
	}
	type span struct{ from, to int }
	spans := make([]span, 0, len(e.Errors))
	fields = make([]string, 0, len(e.Errors))
	for i := range e.Errors {
		item := &e.Errors[i]
		name, ok := item.missingField()
		if !ok {
			return "", nil, false
		}
		at, ok := offset(query, item.Locations[0].Line, item.Locations[0].Column)
		if !ok {
			return "", nil, false
		}
		from, to, ok := fieldSpan(query, at, name)
		if !ok {
			return "", nil, false
		}
		// A field in a fragment that several places spread is reported
		// once for each.
		if !slices.Contains(spans, span{from, to}) {
			spans = append(spans, span{from, to})
		}
		fields = append(fields, item.Extensions.TypeName+"."+name)
	}
	// Cut from the end, so the offsets before stay put.
	slices.SortFunc(spans, func(a, b span) int { return b.from - a.from })
	out := query
	var cut strings.Builder
	for _, s := range spans {
		cut.WriteString(out[s.from:s.to])
		out = out[:s.from] + out[s.to:]
	}
	if emptySelection.MatchString(out) {
		return "", nil, false
	}
	for _, m := range fragmentDef.FindAllStringSubmatch(out, -1) {
		if !regexp.MustCompile(`\.\.\.\s*` + m[1] + `\b`).MatchString(out) {
			return "", nil, false
		}
	}
	for _, m := range variable.FindAllStringSubmatch(cut.String(), -1) {
		// The definition, and one use at least.
		if len(regexp.MustCompile(`\$`+m[1]+`\b`).FindAllStringIndex(out, 2)) < 2 {
			return "", nil, false
		}
	}
	slices.Sort(fields)
	return out, slices.Compact(fields), true
}

var (
	emptySelection = regexp.MustCompile(`\{\s*\}`)
	variable       = regexp.MustCompile(`\$(\w+)`)
	fragmentDef    = regexp.MustCompile(`\bfragment\s+(\w+)\s+on\b`)
)

// offset returns the byte offset of line and column, both from 1, in
// query, which is ASCII as all of the client's are.
func offset(query string, line, column int) (int, bool) {
	at := 0
	for range line - 1 {
		i := strings.IndexByte(query[at:], '\n')
		if i < 0 {
			return 0, false
		}
		at += i + 1
	}
	at += column - 1
	return at, column > 0 && at < len(query)
}

// fieldSpan returns where the selection of field name that starts at at,
// by its alias or its name, starts and ends in query: its alias, name,
// arguments, directives and selection. It reports false if the selection
// there isn't of name.
func fieldSpan(query string, at int, name string) (from, to int, ok bool) {
	from = at
	// GitHub may point to the name after an alias.
	if i := skipSpaceBack(query, at); i > 0 && query[i-1] == ':' {
		j := skipSpaceBack(query, i-1)
		k := j
		for k > 0 && isNameByte(query[k-1]) {
			k--
		}
		if k == j {
			return 0, 0, false
		}
		from = k
	}
	i := readName(query, at)
	got := query[at:i]
	if j := skipSpace(query, i); j < len(query) && query[j] == ':' {
		k := skipSpace(query, j+1)
		i = readName(query, k)
		got = query[k:i]
	}
	if got != name {
		return 0, 0, false
	}
	i = skipSpace(query, i)
	if i < len(query) && query[i] == '(' {
		if i, ok = skipBalanced(query, i, '(', ')'); !ok {
			return 0, 0, false
		}
		i = skipSpace(query, i)
	}
	for i < len(query) && query[i] == '@' {
		i = skipSpace(query, readName(query, i+1))
		if i < len(query) && query[i] == '(' {
			if i, ok = skipBalanced(query, i, '(', ')'); !ok {
				return 0, 0, false
			}
			i = skipSpace(query, i)
		}
	}
	end := i
	if i < len(query) && query[i] == '{' {
		if end, ok = skipBalanced(query, i, '{', '}'); !ok {
			return 0, 0, false
		}
	}
	return from, end, true
}

func isNameByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func readName(s string, i int) int {
	for i < len(s) && isNameByte(s[i]) {
		i++
	}
	return i
}

// skipSpace skips white space and commas, which GraphQL ignores.
func skipSpace(s string, i int) int {
	for i < len(s) && strings.IndexByte(" \t\r\n,", s[i]) >= 0 {
		i++
	}
	return i
}

func skipSpaceBack(s string, i int) int {
	for i > 0 && strings.IndexByte(" \t\r\n,", s[i-1]) >= 0 {
		i--
	}
	return i
}

// skipBalanced returns the offset after the close that matches the open
// at i, passing over strings.
func skipBalanced(s string, i int, opener, closer byte) (int, bool) {
	depth := 0
	for ; i < len(s); i++ {
		switch s[i] {
		case '"':
			for i++; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' {
					i++
				}
			}
		case opener:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}
