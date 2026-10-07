package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/eggzec/gh-tui/internal/core"
)

// start is where the app starts. Host is the GitHub host of the session,
// which every repository of the session is on. Here is the repository of
// the current directory, which the dashboard shows first, or the zero
// RepoRef when there is none on Host. From says why Host was picked:
// --hostname, repo (the current repository's), or where defaultHost
// found it, such as GH_HOST.
type start struct {
	Host string
	Here core.RepoRef
	// HereHost is the host of the current directory's repository when it
	// is not Host, so Here is empty.
	HereHost string
	From     string
}

// startRepos picks the host of the session the way gh does: hostname, the
// value of --hostname, if set; else the host of the current repository;
// else defaultHost's, which is GH_HOST or the host gh is logged in to,
// with where it found it. The current repository is kept only when it is
// on that host.
func startRepos(hostname string, current func() (repository.Repository, bool), defaultHost func() (host, source string)) start {
	cur, ok := current()
	if !ok {
		cur = repository.Repository{}
	}
	host, from := normalizeHostname(hostname), "--hostname"
	if host == "" {
		host, from = normalizeHostname(cur.Host), "repo"
	}
	if host == "" {
		var source string
		host, source = defaultHost()
		host, from = normalizeHostname(host), source
	}
	s := start{Host: host, From: from}
	if ok && normalizeHostname(cur.Host) == host {
		s.Here = core.RepoRef{Owner: cur.Owner, Name: cur.Name}
	} else if ok {
		s.HereHost = normalizeHostname(cur.Host)
	}
	return s
}

// normalizeHostname reads a host as gh does: without a scheme or a
// trailing slash, in lowercase, and with the hosts of github.com's
// subdomains taken for github.com.
func normalizeHostname(h string) string {
	if _, rest, ok := strings.Cut(h, "://"); ok {
		h = rest
	}
	return auth.NormalizeHostname(strings.TrimRight(strings.TrimSpace(h), "/"))
}

// checkHostname returns why hostname, the value of --hostname, names no
// host, or nil if it does: a host with its port, if it has one, and a
// scheme and a trailing slash, which normalizeHostname drops, but no path,
// such as an Enterprise Server's /api/v3, nor a user or a query, which
// would otherwise fail later as a host gh has no token for.
func checkHostname(hostname string) error {
	host := normalizeHostname(hostname)
	u, err := url.Parse("//" + host)
	if host == "" || err != nil || u.Host != host || u.Path != "" || u.RawQuery != "" || u.Fragment != "" ||
		strings.ContainsAny(host, "@/ ") {
		return fmt.Errorf("--hostname %q names no host; give the host alone, such as github.com or ghe.example.com", hostname)
	}
	return nil
}

// currentRepo reads the repository of the current directory the way gh
// does: GH_REPO, else the git remotes. Outside a repository it reports
// false.
func currentRepo() (repository.Repository, bool) {
	r, err := repository.Current()
	if err != nil {
		return repository.Repository{}, false
	}
	return r, true
}

// defaultHost is the host gh uses when nothing else names one, and where
// it found it: GH_HOST, else the only host gh is logged in to (hosts),
// else github.com (default).
func defaultHost() (host, source string) {
	return auth.DefaultHost()
}
