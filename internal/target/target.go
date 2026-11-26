// Package target reads where to switch to: a GitHub login, a host, or both.
package target

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/cli/go-gh/v2/pkg/auth"
)

// Usage is the error text for an argument that is none of the three forms.
const Usage = "usage: :switch user <name> | host <name> | <user>@<host>"

// Target is the identity to sign in as: a login on a host.
type Target struct {
	User string
	Host string
}

// String spells t as user@host.
func (t Target) String() string { return t.User + "@" + t.Host }

// NormalizeHost reads a host as gh does: without a scheme or a trailing
// slash, in lowercase, and with the hosts of github.com's subdomains taken
// for github.com.
func NormalizeHost(h string) string {
	if _, rest, ok := strings.Cut(h, "://"); ok {
		h = rest
	}
	return auth.NormalizeHostname(strings.TrimRight(strings.TrimSpace(h), "/"))
}

// ValidHost reports whether h, once normalized, is a host with its port, if
// it has one, and no path, such as an Enterprise Server's /api/v3, nor a
// user or a query.
func ValidHost(h string) bool {
	host := NormalizeHost(h)
	u, err := url.Parse("//" + host)
	return host != "" && err == nil && u.Host == host && u.Path == "" && u.RawQuery == "" && u.Fragment == "" &&
		!strings.ContainsAny(host, "@/ ")
}

// Parse reads the argument of :switch, which is one of "user NAME" (the
// current host), "host NAME" (the current user) or "USER@HOST". Nothing
// else is read.
func Parse(input, curUser, curHost string) (Target, error) {
	f := strings.Fields(input)
	t := Target{User: curUser, Host: curHost}
	var user, host string
	switch {
	case len(f) == 2 && f[0] == "user":
		user = f[1]
	case len(f) == 2 && f[0] == "host":
		host = f[1]
	case len(f) == 1 && strings.Count(f[0], "@") == 1:
		user, host, _ = strings.Cut(f[0], "@")
		if user == "" || host == "" {
			return Target{}, errors.New(Usage)
		}
	default:
		return Target{}, errors.New(Usage)
	}
	if user != "" {
		if !validUser(user) {
			return Target{}, fmt.Errorf("%q is not a GitHub login: letters, digits, - and _ only", user)
		}
		t.User = user
	}
	if host != "" {
		if !ValidHost(host) {
			return Target{}, fmt.Errorf("%q names no host; give the host alone, such as github.com or ghe.example.com", host)
		}
		t.Host = NormalizeHost(host)
	}
	return t, nil
}

func validUser(u string) bool {
	for _, r := range u {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		if !letter && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}
