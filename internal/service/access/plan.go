package access

import (
	"slices"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// Where auth.TokenForHost finds a token that gh stores: its hosts file,
// or its keyring through gh auth token.
const (
	sourceHostsFile = "oauth_token"
	sourceKeyring   = "gh"
)

// Plan is what the user can do so that the token may do more.
type Plan struct {
	// Cmd is the command that grants it, the program first, or nil when
	// no command can.
	Cmd []string
	// URL is the page where the user changes the token themselves, or "".
	URL string
	// Why says in one line what Cmd does, or what the user can do instead.
	Why string
	// Scopes are the scopes to grant, sorted.
	Scopes []string
}

// Refresh returns what the user can do so that the token may do what
// needs each of needs, and be authorized for the SSO of the organizations
// that left their results out.
func (s *Service) Refresh(needs ...core.Need) Plan {
	s.mu.Lock()
	a, tok := s.cur, s.token
	s.mu.Unlock()
	st := situation{
		host:   s.host,
		source: tok.Source,
		kind:   a.Kind,
		known:  a.Known || a.Kind != core.TokenClassic,
		pat:    strings.HasPrefix(tok.Value, "ghp_"),
		sso:    len(a.SSO) > 0,
	}
	for _, n := range needs {
		if m := a.Missing(n); m != "" {
			st.missing = append(st.missing, m)
		} else if ok, known := a.Allows(n); !ok && known {
			st.refused = true
		}
	}
	slices.Sort(st.missing)
	st.missing = slices.Compact(st.missing)
	if stored(st.source) {
		st.gh = s.ghPath()
	}
	return plan(st)
}

// situation is what a plan depends on.
type situation struct {
	// host is the host gh names, such as github.com.
	host   string
	source string
	kind   core.TokenKind
	// known says that GitHub said what a classic token may do.
	known bool
	// pat says that the token is a classic personal access token, whose
	// scopes the user sets on GitHub.
	pat bool
	// gh is where gh is, or "" if it isn't installed.
	gh string
	// missing are the scopes to grant, and refused says that no token of
	// this kind may do something.
	missing []string
	refused bool
	sso     bool
}

// plan returns what the user can do in st.
func plan(st situation) Plan {
	scopes := phrase(st.missing)
	tokens := core.WebScheme(st.host) + "://" + st.host + "/settings/tokens"
	env := envSource(st.source)
	switch {
	case !st.known:
		return Plan{Why: "GitHub hasn't said yet what the token may do."}
	case len(st.missing) == 0 && !st.refused && !st.sso:
		return Plan{Why: "The token may do everything gh-tui does."}
	case !hostName(st.host):
		// The host goes into a command and a link, so one that could
		// be read as more goes into neither.
		return Plan{Why: "The host isn't a plain host name, so gh-tui can't refresh its token; run gh auth refresh yourself.", Scopes: st.missing}
	case st.kind == core.TokenFineGrained || st.kind == core.TokenApp:
		why := "A " + kindName(st.kind) + " token has no scopes to grant; "
		if env {
			return Plan{Why: why + "unset " + st.source + " and run gh auth login to use a classic token."}
		}
		return Plan{Why: why + "run gh auth login to use a classic token."}
	case env:
		p := Plan{URL: tokens, Scopes: st.missing}
		if len(st.missing) == 0 {
			p.Why = "The token comes from " + st.source + ": authorize it for the organizations' SSO on GitHub, or unset " + st.source + " and run gh auth login."
			return p
		}
		p.Why = "The token comes from " + st.source + ": update " + st.source + " to a token with " + scopes + ", or unset it and run gh auth login."
		return p
	case !stored(st.source) || st.pat:
		p := Plan{URL: tokens, Scopes: st.missing}
		if len(st.missing) == 0 {
			p.Why = "Authorize the token for the organizations' SSO on GitHub, then check again."
			return p
		}
		p.Why = "Give the token " + scopes + " on GitHub, then check again."
		return p
	case st.gh == "":
		if len(st.missing) == 0 {
			return Plan{Why: "Install GitHub CLI and run gh auth refresh to authorize the organizations' SSO."}
		}
		return Plan{Why: "Install GitHub CLI to grant the token " + scopes + ".", Scopes: st.missing}
	}
	cmd := []string{st.gh, "auth", "refresh", "--hostname=" + st.host}
	if len(st.missing) > 0 {
		cmd = append(cmd, "-s", strings.Join(st.missing, ","))
	}
	// gh keeps a refreshed token in the keyring unless told otherwise,
	// and this one was in its hosts file.
	if st.source == sourceHostsFile {
		cmd = append(cmd, "--insecure-storage")
	}
	if len(st.missing) == 0 {
		return Plan{Cmd: cmd, Why: "gh signs in again, where you can authorize the organizations' SSO."}
	}
	return Plan{Cmd: cmd, Why: "gh opens the browser to grant the token " + scopes + ".", Scopes: st.missing}
}

// hostName reports whether host is a host name, or an address, with an
// optional port, and nothing else, such as a flag or a path.
func hostName(host string) bool {
	name, port, hasPort := strings.Cut(host, ":")
	if name == "" || strings.HasPrefix(name, "-") || strings.HasPrefix(name, ".") {
		return false
	}
	if strings.ContainsFunc(name, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '.' && r != '-'
	}) {
		return false
	}
	if !hasPort {
		return true
	}
	return port != "" && len(port) <= 5 && !strings.ContainsFunc(port, func(r rune) bool { return r < '0' || r > '9' })
}

// stored reports whether source is gh's own store.
func stored(source string) bool {
	return source == sourceHostsFile || source == sourceKeyring
}

// envSource reports whether source is an environment variable, whose
// token gh can't change.
func envSource(source string) bool {
	switch source {
	case "GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN":
		return true
	}
	return false
}

// kindName names a kind of token that has no scopes.
func kindName(k core.TokenKind) string {
	if k == core.TokenApp {
		return "GitHub App"
	}
	return k.String()
}

// phrase names scopes, as "the workflow scope" or "the notifications and
// workflow scopes".
func phrase(scopes []string) string {
	switch len(scopes) {
	case 0:
		return ""
	case 1:
		return "the " + scopes[0] + " scope"
	}
	return "the " + strings.Join(scopes[:len(scopes)-1], ", ") + " and " + scopes[len(scopes)-1] + " scopes"
}
