package core

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// DefaultHost is the host of github.com, which ParseTarget assumes when it
// is given no host.
const DefaultHost = "github.com"

// WebScheme returns the scheme of the web pages of host, as gh has it:
// http for github.localhost, a GitHub run locally for development, and
// https for any other.
func WebScheme(host string) string {
	if strings.EqualFold(host, "github.localhost") {
		return "http"
	}
	return "https"
}

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
// empty. Surrounding space is trimmed and case is kept. Its errors are
// *TargetError.
func ParseTarget(s, host string) (Target, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Target{}, &TargetError{Reason: "type owner/name, #number or a link", Err: errors.New("nothing to open: type owner/name, #number or a link")}
	}
	if host == "" {
		host = DefaultHost
	}
	parse := parseShort
	if isLink(s, host) {
		parse = func(s string) (Target, error) { return parseLink(s, host) }
	}
	t, err := parse(s)
	if e, ok := errors.AsType[*TargetError](err); ok {
		e.Input = s
	}
	return t, err
}

// TargetError reports that ParseTarget couldn't read Input, what the user
// typed. Reason says what is wrong without repeating Input, such as "want
// owner/name", for the user, and Err says it for the log.
type TargetError struct {
	Input  string
	Reason string
	Err    error
}

func (e *TargetError) Error() string {
	return e.Err.Error()
}

// Unwrap returns what the log says of the error.
func (e *TargetError) Unwrap() error {
	return e.Err
}

// badTarget returns a *TargetError whose reason is also its message.
func badTarget(reason string) error {
	return &TargetError{Reason: reason, Err: errors.New(reason)}
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
		r, err := parseRepo(repo)
		if err != nil {
			return Target{}, err
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
		return 0, badTarget("missing issue number after '#'")
	case s[0] == '-':
		return 0, badTarget("issue numbers are positive")
	case strings.Trim(s, "0123456789") != "":
		return 0, &TargetError{Reason: "not an issue number", Err: fmt.Errorf("not an issue number: %q", s)}
	}
	// GitHub's GraphQL Int is 32 bits, so larger numbers can't exist.
	n, err := strconv.ParseInt(s, 10, 32)
	if errors.Is(err, strconv.ErrRange) {
		return 0, &TargetError{
			Reason: "issue numbers are at most " + strconv.Itoa(math.MaxInt32),
			Err:    fmt.Errorf("issue number too large: %s (at most %d)", s, math.MaxInt32),
		}
	}
	if err != nil {
		return 0, &TargetError{Reason: "not an issue number", Err: fmt.Errorf("not an issue number: %q: %w", s, err)}
	}
	if n == 0 {
		return 0, badTarget("issue numbers are positive")
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
		return Target{}, &TargetError{Reason: "not a link", Err: fmt.Errorf("not a link: %q", s)}
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return Target{}, &TargetError{Reason: "not a web link", Err: fmt.Errorf("not a web link: %q", s)}
	}
	if path, ok := apiPath(u, host); ok {
		return parseAPILink(s, path)
	}
	if !sameHost(u.Host, u.Scheme, host) {
		return Target{}, &TargetError{Reason: "not a link to " + host, Err: fmt.Errorf("not a link to %s: %q", host, s)}
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || slices.Contains(parts[:2], "") ||
		slices.ContainsFunc(reservedOwners, func(o string) bool { return strings.EqualFold(o, parts[0]) }) {
		return Target{}, &TargetError{Reason: "not a link to a repository", Err: fmt.Errorf("not a repository link: %q", s)}
	}
	repo, err := parseRepo(parts[0] + "/" + parts[1])
	if err != nil {
		return Target{}, err
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
	if parts[3] == "" {
		// A link has no '#' before its number, which parseNumber would
		// ask for.
		return Target{}, &TargetError{
			Reason: "missing the number after /" + parts[2] + "/",
			Err:    fmt.Errorf("missing number after /%s/: %q", parts[2], s),
		}
	}
	n, err := parseNumber(parts[3])
	if err != nil {
		return Target{}, err
	}
	t.Number, t.Kind = n, kind
	return t, nil
}

// apiPath returns the path below the API root of u, a link to the REST
// API of host rather than to its pages: below /api/v3 on an Enterprise
// Server, whose API shares its host, and on the api. host of github.com
// or a GHE.com tenant.
func apiPath(u *url.URL, host string) (string, bool) {
	if sameHost(u.Host, u.Scheme, "api."+host) {
		return u.Path, true
	}
	// github.com's pages have no /api/v3; an owner named api has them.
	if !sameHost(u.Host, u.Scheme, host) || strings.EqualFold(host, DefaultHost) {
		return "", false
	}
	// Everything below /api is the API's, such as /api/graphql too.
	below := func(path, dir string) (string, bool) {
		rest, ok := strings.CutPrefix(path, dir)
		return rest, ok && (rest == "" || rest[0] == '/')
	}
	rest, ok := below(u.Path, "/api")
	if !ok {
		return "", false
	}
	if v3, ok := below(rest, "/v3"); ok {
		return v3, true
	}
	return rest, true
}

// apiKinds are the API paths below a repository whose next segment is a
// pull request or issue number.
var apiKinds = map[string]NumberKind{"pulls": KindPull, "issues": KindIssue}

// parseAPILink reads path, the part below the API root of s, a link to
// GitHub's REST API: /repos/owner/name for a repository, and
// /repos/owner/name/pulls/N or /issues/N, or a path below them, for a
// pull request or an issue. The API has no page for anything else.
func parseAPILink(s, path string) (Target, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 || parts[0] != "repos" {
		return Target{}, &TargetError{
			Reason: "an API link opens only a repository, pull request or issue",
			Err:    fmt.Errorf("not an API link to a repository, pull request or issue: %q", s),
		}
	}
	repo, err := parseRepo(parts[1] + "/" + parts[2])
	if err != nil {
		return Target{}, err
	}
	t := Target{Repo: repo}
	if len(parts) == 3 {
		return t, nil
	}
	kind := apiKinds[parts[3]]
	if !kind.Known() || len(parts) < 5 {
		return Target{}, &TargetError{
			Reason: "an API link opens only a repository, pull request or issue",
			Err:    fmt.Errorf("not an API link to a repository, pull request or issue: %q", s),
		}
	}
	n, err := parseNumber(parts[4])
	if err != nil {
		return Target{}, err
	}
	t.Number, t.Kind = n, kind
	return t, nil
}

// parseRepo parses the repository of a target, s, as "owner/name". A clone
// URL names the repository with ".git", which GitHub drops from names, and
// so does the short form.
func parseRepo(s string) (RepoRef, error) {
	r, err := parseRepoRef(strings.TrimSuffix(s, ".git"))
	if err != nil {
		return RepoRef{}, &TargetError{Reason: reasonOf(err), Err: fmt.Errorf("not a repository: %q: %w", s, err)}
	}
	return r, nil
}

// sameHost matches got, the host of a URL with scheme, to the user's
// host, which is served over https. A port that is the scheme's default
// is the same as none. The www. prefix is allowed on github.com, which
// redirects it.
func sameHost(got, scheme, host string) bool {
	got, host = defaultPort(got, scheme), defaultPort(host, "https")
	if strings.EqualFold(got, host) {
		return true
	}
	return strings.EqualFold(host, DefaultHost) && strings.EqualFold(got, "www."+DefaultHost)
}

// defaultPorts are the ports that URLs of each scheme leave out.
var defaultPorts = map[string]string{"https": "443", "http": "80"}

// defaultPort returns hostport without its port if that is the default of
// scheme, since github.com:443 is github.com.
func defaultPort(hostport, scheme string) string {
	h, port, err := net.SplitHostPort(hostport)
	if err != nil || port != defaultPorts[scheme] {
		return hostport
	}
	return h
}
