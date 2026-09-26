package core

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// DefaultHost is the host of github.com, which ParseTarget assumes when it
// is given no host.
const DefaultHost = "github.com"

// Target is what the user names after ':': a repository, a pull request or
// issue number, or both. A number without a repository refers to the
// current one, which the caller supplies. Kind is what a link said the
// number is: KindPull from /pull/ or /pulls/, KindIssue from /issues/.
// A number typed after '#' leaves it unknown, and it can be looked up as
// either.
type Target struct {
	Repo   RepoRef
	Number int
	Kind   NumberKind
}

// HasRepo reports whether t names a repository.
func (t Target) HasRepo() bool {
	return t.Repo != RepoRef{}
}

// HasNumber reports whether t names a pull request or issue.
func (t Target) HasNumber() bool {
	return t.Number > 0
}

// Same reports whether t and o name the same target, comparing
// repositories regardless of case as GitHub does. Kind is ignored: it
// can't make one number name two things.
func (t Target) Same(o Target) bool {
	return t.Repo.Same(o.Repo) && t.Number == o.Number
}

// String returns t in the short form ParseTarget reads: "owner/name",
// "owner/name#12" or "#12". The short form has no Kind.
func (t Target) String() string {
	var b strings.Builder
	if t.HasRepo() {
		b.WriteString(t.Repo.String())
	}
	if t.HasNumber() {
		b.WriteByte('#')
		b.WriteString(strconv.Itoa(t.Number))
	}
	return b.String()
}

// ParseTarget parses what the user typed after ':' to name a target:
// "owner/name", "owner/name#12", "#12", or a link to a repository, pull
// request or issue on host, such as https://github.com/owner/name/pull/12.
// A link to any other page of a repository names the repository. host is the user's GitHub host without a scheme, DefaultHost when
// empty. Surrounding space is trimmed and case is kept. Error messages can
// be shown to the user as they are.
func ParseTarget(s, host string) (Target, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Target{}, errors.New("nothing to open: type owner/name, #number or a link")
	}
	if host == "" {
		host = DefaultHost
	}
	if isLink(s, host) {
		return parseLink(s, host)
	}
	return parseShort(s)
}

// isLink reports whether s is a URL rather than the short form. Owners
// can't contain '.' or ':', so a first segment with either is a host,
// whether or not it is the user's. What follows '#' is a number or a
// fragment, never a host.
func isLink(s, host string) bool {
	if strings.HasPrefix(s, "#") {
		return false
	}
	if hasScheme(s) {
		return true
	}
	path, _, _ := strings.Cut(s, "#")
	first, rest, ok := strings.Cut(path, "/")
	if !ok {
		return false
	}
	if strings.ContainsAny(first, ".:") {
		return true
	}
	// A dotless Enterprise host such as "ghe" reads like an owner, so only
	// a path too long for owner/name makes it a link.
	return strings.EqualFold(first, host) && strings.Contains(rest, "/")
}

func hasScheme(s string) bool {
	scheme, _, ok := strings.Cut(s, "://")
	return ok && !strings.ContainsAny(scheme, "/#?")
}

func parseShort(s string) (Target, error) {
	repo, num, hasNum := strings.Cut(s, "#")
	var t Target
	if repo != "" {
		r, err := ParseRepoRef(repo)
		if err != nil {
			return Target{}, fmt.Errorf("not a repository: %w", err)
		}
		t.Repo = r
	}
	if !hasNum {
		return t, nil
	}
	n, err := parseNumber(num)
	if err != nil {
		return Target{}, err
	}
	t.Number = n
	return t, nil
}

// parseNumber reads a pull request or issue number. It accepts only
// digits, so "+3" and " 3" are refused rather than read as 3.
func parseNumber(s string) (int, error) {
	switch {
	case s == "":
		return 0, errors.New("missing issue number after '#'")
	case s[0] == '-':
		return 0, errors.New("issue numbers are positive")
	case strings.Trim(s, "0123456789") != "":
		return 0, fmt.Errorf("not an issue number: %q", s)
	}
	// GitHub's GraphQL Int is 32 bits, so larger numbers can't exist.
	n, err := strconv.ParseInt(s, 10, 32)
	if errors.Is(err, strconv.ErrRange) {
		return 0, fmt.Errorf("issue number too large: %s (at most %d)", s, math.MaxInt32)
	}
	if err != nil {
		return 0, fmt.Errorf("not an issue number: %q: %w", s, err)
	}
	if n == 0 {
		return 0, errors.New("issue numbers are positive")
	}
	return int(n), nil
}

// reservedOwners are first path segments GitHub uses for its own pages, so
// a link such as github.com/settings/profile isn't taken for a repository.
var reservedOwners = []string{
	"about", "account", "apps", "codespaces", "collections", "dashboard",
	"enterprise", "enterprises", "explore", "features", "gist", "issues",
	"login", "marketplace", "new", "notifications", "orgs", "organizations",
	"pricing", "pulls", "search", "security", "settings", "sponsors",
	"stars", "topics", "trending", "users",
}

// linkKinds are the pages below a repository whose next segment is a pull
// request or issue number; GitHub serves /pulls/12 as /pull/12.
var linkKinds = map[string]NumberKind{"pull": KindPull, "pulls": KindPull, "issues": KindIssue}

func parseLink(s, host string) (Target, error) {
	raw := s
	if !hasScheme(raw) {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Target{}, fmt.Errorf("not a link: %q", s)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return Target{}, fmt.Errorf("not a web link: %q", s)
	}
	if !sameHost(u.Host, host) {
		return Target{}, fmt.Errorf("not a link to %s: %q", host, s)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || slices.Contains(parts[:2], "") ||
		slices.ContainsFunc(reservedOwners, func(o string) bool { return strings.EqualFold(o, parts[0]) }) {
		return Target{}, fmt.Errorf("not a repository link: %q", s)
	}
	// A clone URL names the repository with ".git", which GitHub drops
	// from names, so a repository can't end in it.
	name := strings.TrimSuffix(parts[1], ".git")
	repo, err := ParseRepoRef(parts[0] + "/" + name)
	if err != nil {
		return Target{}, fmt.Errorf("not a repository: %w", err)
	}
	t := Target{Repo: repo}
	var kind NumberKind
	if len(parts) >= 4 {
		kind = linkKinds[parts[2]]
	}
	if !kind.Known() || strings.Trim(parts[3], "0123456789") != "" {
		// Any other page of a repository, such as its code, its actions or
		// a list, opens the repository.
		return t, nil
	}
	n, err := parseNumber(parts[3])
	if err != nil {
		return Target{}, err
	}
	t.Number, t.Kind = n, kind
	return t, nil
}

// sameHost matches got, a URL's host, to the user's host. The www. prefix
// is allowed on github.com, which redirects it.
func sameHost(got, host string) bool {
	if strings.EqualFold(got, host) {
		return true
	}
	return strings.EqualFold(host, DefaultHost) && strings.EqualFold(got, "www."+DefaultHost)
}
