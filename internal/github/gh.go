package github

import (
	"context"
	"net"
	"net/http"

	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/config"
)

// ghLookup is where New finds what gh knows: the default host, the token of
// a host and where it came from, and gh's config. Tests replace it so that
// they never read the user's.
type ghLookup struct {
	defaultHost func() (host, source string)
	token       func(host string) (token, source string)
	// stored finds a token as token does, but only where it can without
	// running gh: the environment and gh's hosts file.
	stored func(host string) (token, source string)
	config func() (*config.Config, error)
}

func ghDefaults() ghLookup {
	return ghLookup{
		defaultHost: auth.DefaultHost,
		token:       auth.TokenForHost,
		stored:      auth.TokenFromEnvOrConfig,
		config:      func() (*config.Config, error) { return config.Read(nil) },
	}
}

// FindToken returns the token of host that gh would use, where it found
// it, as auth.TokenForHost names it, such as GH_TOKEN or gh, and the login
// of the account gh stores it for, or "" for a token from elsewhere. It
// reads the environment and gh's store each time, so it finds a token gh
// refreshed.
func FindToken(host string) (token, source, login string) {
	g := ghDefaults()
	token, source = g.token(host)
	return token, source, g.login(host, source)
}

// QuickToken returns what FindToken would, as far as it can without
// running gh, which FindToken does for a token gh keeps in the system
// keyring and which takes tens of milliseconds: a token from the
// environment or gh's hosts file, where it came from and the login; or no
// token, with the keyring as its source and the login gh is logged in as
// on host, when gh keeps the token there; or nothing when gh isn't logged
// in to host.
func QuickToken(host string) (token, source, login string) {
	return ghDefaults().quickToken(host)
}

func (g ghLookup) quickToken(host string) (token, source, login string) {
	if token, source = g.stored(host); token != "" {
		return token, source, g.login(host, source)
	}
	if login = g.login(host, sourceKeyring); login == "" {
		return "", "", ""
	}
	return "", sourceKeyring, login
}

// Where auth.TokenForHost found a token that gh itself stores: hosts.yml,
// or the keyring through gh auth token.
const (
	sourceHostsFile = "oauth_token"
	sourceKeyring   = "gh"
)

// login returns the login of the account gh is logged in as on host, when
// the token came from gh's own store, which holds that account's token. A
// token from the environment may be anyone's, so it has none.
func (g ghLookup) login(host, source string) string {
	if source != sourceHostsFile && source != sourceKeyring {
		return ""
	}
	cfg, err := g.config()
	if err != nil || cfg == nil {
		return ""
	}
	login, _ := cfg.Get([]string{"hosts", auth.NormalizeHostname(host), "user"})
	return login
}

// unixSocket returns the socket gh sends its HTTP requests through, from
// http_unix_socket in its config, or "" when it has none.
func (g ghLookup) unixSocket() string {
	cfg, err := g.config()
	if err != nil || cfg == nil {
		return ""
	}
	path, _ := cfg.Get([]string{"http_unix_socket"})
	return path
}

// throughSocket returns a copy of hc that sends every request through the
// unix socket at path, as gh does: the socket's server makes the
// connection, so TLS is left to it too. Without a path, it returns hc.
func throughSocket(hc *http.Client, path string) *http.Client {
	if path == "" {
		return hc
	}
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", path)
	}
	c := *hc
	c.Transport = &http.Transport{DialContext: dial, DialTLSContext: dial, DisableKeepAlives: true}
	return &c
}
