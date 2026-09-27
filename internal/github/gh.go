package github

import (
	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/config"
)

// ghLookup is where New finds what gh knows: the default host, the token of
// a host and where it came from, and gh's config. Tests replace it so that
// they never read the user's.
type ghLookup struct {
	defaultHost func() (host, source string)
	token       func(host string) (token, source string)
	config      func() (*config.Config, error)
}

func ghDefaults() ghLookup {
	return ghLookup{
		defaultHost: auth.DefaultHost,
		token:       auth.TokenForHost,
		config:      func() (*config.Config, error) { return config.Read(nil) },
	}
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
