// Package codeowners parses CODEOWNERS files and says who owns a path, by
// GitHub's rules. It does no I/O: the file's text comes from the caller.
//
// GitHub has no API that maps the files of a pull request to their owners,
// so the client works it out from the file itself. A line is a pattern
// followed by its owners, and the last line whose pattern matches a path
// decides its owners.
package codeowners

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// OwnerKind is what an owner names.
type OwnerKind int

// The kinds of owner a CODEOWNERS line may name.
const (
	// OwnerUser is a user, written @login.
	OwnerUser OwnerKind = iota
	// OwnerTeam is a team of an organization, written @org/team.
	OwnerTeam
	// OwnerEmail is an email address, which GitHub matches to a user.
	OwnerEmail
)

// Owner is who a line of a CODEOWNERS file gives a path to. It is
// comparable.
type Owner struct {
	Kind OwnerKind
	// Name is the login, "org/team" or the email address, without the @ that
	// starts a user or a team.
	Name string
}

// String returns the owner as the file writes it.
func (o Owner) String() string {
	if o.Kind == OwnerEmail {
		return o.Name
	}
	return "@" + o.Name
}

// Org returns the organization of a team, and "" for any other owner.
func (o Owner) Org() string {
	org, _ := o.team()
	return org
}

// Slug returns the slug of a team within its organization, and "" for any
// other owner.
func (o Owner) Slug() string {
	_, slug := o.team()
	return slug
}

func (o Owner) team() (org, slug string) {
	if o.Kind != OwnerTeam {
		return "", ""
	}
	org, slug, _ = strings.Cut(o.Name, "/")
	return org, slug
}

// Rule is a line of a CODEOWNERS file that names a pattern.
type Rule struct {
	// Pattern is the pattern as written, with escaped spaces and hashes
	// resolved.
	Pattern string
	// Owners are who the pattern gives its paths to. None clears the
	// ownership that an earlier rule gave.
	Owners []Owner
	// Line is the number of the line in the file, from 1.
	Line int

	re *regexp.Regexp
}

// Matches reports whether the rule's pattern matches path, a file's path
// relative to the root of the repository, with slashes.
func (r Rule) Matches(path string) bool {
	return r.re != nil && r.re.MatchString(strings.TrimPrefix(path, "/"))
}

// Invalid is a line that GitHub ignores, which Parse left out of the rules.
type Invalid struct {
	// Line is the number of the line in the file, from 1.
	Line int
	// Text is the line as it is.
	Text string
	// Reason says what is wrong with it.
	Reason string
}

// File is a parsed CODEOWNERS file.
type File struct {
	// Rules are the lines that name a pattern, in the order of the file.
	Rules []Rule
	// Invalid are the lines that were left out, so that a caller may show or
	// ignore them: the rest of the file is still good.
	Invalid []Invalid
}

// Parse reads the text of a CODEOWNERS file. A line it can't make sense of
// is reported in Invalid and the rest of the file is read as usual, as GitHub
// does.
func Parse(text string) File {
	var f File
	for i, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		fields := fields(line)
		if len(fields) == 0 {
			continue
		}
		rule, err := parseRule(fields)
		if err != nil {
			f.Invalid = append(f.Invalid, Invalid{Line: i + 1, Text: line, Reason: err.Error()})
			continue
		}
		rule.Line = i + 1
		f.Rules = append(f.Rules, rule)
	}
	return f
}

// fields splits a line at its unescaped white space, and drops a comment: a
// field that starts with an unescaped #, and what follows it. An escaped
// space or # belongs to the field, which holds it unescaped; any other
// escape is kept for the pattern's glob to read.
func fields(line string) []string {
	var (
		out []string
		cur strings.Builder
		in  bool
	)
	flush := func() {
		if in {
			out = append(out, cur.String())
			cur.Reset()
			in = false
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '\\' && i+1 < len(line):
			i++
			in = true
			if n := line[i]; n == ' ' || n == '#' || n == '\t' {
				cur.WriteByte(n)
			} else {
				cur.WriteByte(c)
				cur.WriteByte(n)
			}
		case c == ' ' || c == '\t':
			flush()
		case c == '#' && !in:
			flush()
			return out
		default:
			in = true
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}

// parseRule makes a rule of the fields of a line: a pattern, then owners.
func parseRule(fields []string) (Rule, error) {
	pattern := fields[0]
	if strings.HasPrefix(pattern, "!") {
		return Rule{}, fmt.Errorf("pattern %q negates, which CODEOWNERS doesn't support", pattern)
	}
	if hasRange(pattern) {
		return Rule{}, fmt.Errorf("pattern %q has a character range, which CODEOWNERS doesn't support", pattern)
	}
	re, err := compile(pattern)
	if err != nil {
		return Rule{}, fmt.Errorf("pattern %q: %w", pattern, err)
	}
	r := Rule{Pattern: pattern, re: re}
	for _, f := range fields[1:] {
		o, ok := parseOwner(f)
		if !ok {
			return Rule{}, fmt.Errorf("%q is no user, team or email address", f)
		}
		r.Owners = append(r.Owners, o)
	}
	return r, nil
}

// hasRange reports whether pattern has a [ or ] that isn't escaped.
func hasRange(pattern string) bool {
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '[', ']':
			return true
		}
	}
	return false
}

var (
	userName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	teamSlug = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	email    = regexp.MustCompile(`^[^@\s/]+@[^@\s/]+\.[^@\s/]+$`)
)

// parseOwner reads @login, @org/team or an email address.
func parseOwner(s string) (Owner, bool) {
	name, ok := strings.CutPrefix(s, "@")
	if !ok {
		return Owner{Kind: OwnerEmail, Name: s}, email.MatchString(s)
	}
	if org, slug, isTeam := strings.Cut(name, "/"); isTeam {
		return Owner{Kind: OwnerTeam, Name: name}, userName.MatchString(org) && teamSlug.MatchString(slug)
	}
	return Owner{Kind: OwnerUser, Name: name}, userName.MatchString(name)
}

// compile turns a CODEOWNERS pattern into a regular expression on a path.
//
// A pattern is anchored to the root if it starts with a slash or has one
// before its end, and otherwise matches at any depth. One that matches a
// directory gives its owners everything under it, and one that ends with a
// slash matches directories only. A * stays within a path segment and a **
// crosses them. The exception is a pattern that ends with /*: GitHub reads
// it as the files directly in the directory, though gitignore would take a
// subdirectory to match it and so everything under it.
func compile(pattern string) (*regexp.Regexp, error) {
	dirOnly := strings.HasSuffix(pattern, "/") && len(pattern) > 1
	body := strings.TrimSuffix(pattern, "/")
	if body == "" {
		body = "**"
	}
	rooted := strings.HasPrefix(body, "/")
	body = strings.TrimPrefix(body, "/")
	segments := strings.Split(body, "/")
	anchored := rooted || len(segments) > 1

	var b strings.Builder
	b.WriteString("(?s)^")
	if !anchored {
		b.WriteString("(?:.*/)?")
	}
	for i, seg := range segments {
		last := i == len(segments)-1
		switch {
		case seg == "**" && last:
			b.WriteString(".*")
		case seg == "**":
			b.WriteString("(?:.*/)?")
		default:
			b.WriteString(glob(seg))
			if !last {
				b.WriteByte('/')
			}
		}
	}
	switch {
	case dirOnly:
		b.WriteString("/.*")
	case len(segments) > 1 && segments[len(segments)-1] == "*":
		// Files only, as GitHub has it.
	default:
		// A bare /* has one segment, so it comes here: as in gitignore, it
		// matches every top-level entry, and a directory owns what is below
		// it, so it owns every file at every depth.
		b.WriteString("(?:/.*)?")
	}
	b.WriteByte('$')
	return regexp.Compile(b.String())
}

// glob translates one segment of a pattern: * and ? stay within it, and a
// backslash makes the next character literal.
func glob(seg string) string {
	var b strings.Builder
	for i := 0; i < len(seg); i++ {
		switch c := seg[i]; c {
		case '*':
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '\\':
			if i+1 < len(seg) {
				i++
				c = seg[i]
			}
			b.WriteString(regexp.QuoteMeta(string(c)))
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}

// RuleFor returns the rule that decides the owners of path: the last one
// whose pattern matches it. It reports false if no rule does.
func (f File) RuleFor(path string) (Rule, bool) {
	for _, r := range slices.Backward(f.Rules) {
		if r.Matches(path) {
			return r, true
		}
	}
	return Rule{}, false
}

// OwnersOf returns who owns path. It is empty if no rule matches, or if the
// last one that does names no owner, which clears the ownership.
func (f File) OwnersOf(path string) []Owner {
	r, ok := f.RuleFor(path)
	if !ok {
		return nil
	}
	return r.Owners
}

// Owned is an owner with the paths they own, of those that were asked.
type Owned struct {
	Owner Owner
	// Paths are sorted, without repeats.
	Paths []string
}

// OwnedPaths returns, for each owner of one of paths, the paths they own.
// Paths that nobody owns are left out. Owners are sorted by how the file
// writes them, ignoring case, and paths are sorted too, so the result
// doesn't depend on the order of paths.
func (f File) OwnedPaths(paths []string) []Owned {
	by := map[Owner][]string{}
	for _, p := range paths {
		for _, o := range f.OwnersOf(p) {
			by[o] = append(by[o], p)
		}
	}
	out := make([]Owned, 0, len(by))
	for o, ps := range by {
		slices.Sort(ps)
		out = append(out, Owned{Owner: o, Paths: slices.Compact(ps)})
	}
	slices.SortFunc(out, func(a, b Owned) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Owner.String()), strings.ToLower(b.Owner.String())),
			cmp.Compare(a.Owner.String(), b.Owner.String()),
		)
	})
	return out
}
