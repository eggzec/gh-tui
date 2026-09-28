package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/auth"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	accesssvc "github.com/eggzec/gh-tui/internal/service/access"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// sessionInfo is who a session is: the host it talks to and the account it
// acts as, which the session record logs.
type sessionInfo struct {
	// Host is the host of the session, and From why it was picked.
	Host, From string
	// Here is the repository of the current directory, as owner/name, or
	// "" outside of one.
	Here string
	// WebHost is the host of the host's pages, which for github.com is
	// not that of its API.
	WebHost string
	// Login is the account gh stores the token for, or "" for a token
	// from elsewhere. TokenSource is where the token was found, such as
	// GH_TOKEN or oauth_token, and TokenKind the kind its prefix says.
	Login, TokenSource, TokenKind string
	// Account names the account in the cache, a hash.
	Account string
	// CacheDir is the account's directory of the disk cache, with the
	// home directory as ~, or "" when the disk cache is off.
	CacheDir string
	// GH is where gh is, with the home directory as ~, or "" when it
	// isn't installed.
	GH string
	// Proxy is the host of the proxy the API is reached through, if any,
	// never its credentials.
	Proxy string
	// GHHosts is how many hosts gh knows, and GHKnowsHost whether Host is
	// one of them. Their names may be other companies' servers, so they
	// aren't logged.
	GHHosts     int
	GHKnowsHost bool
}

// logHost makes every record logged from now on name host, the host of
// the session: several sessions, on several hosts, may log to one file.
func logHost(host string) {
	slog.SetDefault(slog.Default().With(slog.String("host", host)))
}

// logSession logs who the session is, once it is known, and then makes
// every later record name its account too. The host is on the records
// already, since logHost set it.
func logSession(s sessionInfo) {
	attrs := []slog.Attr{
		slog.String("span", "app"),
		slog.String("host_from", s.From),
		slog.String("web_host", s.WebHost),
	}
	if s.Here != "" {
		attrs = append(attrs, slog.String("here", s.Here))
	}
	attrs = append(attrs,
		slog.String("login", s.Login),
		slog.String("token_source", s.TokenSource),
		slog.String("token_kind", s.TokenKind),
		slog.String("account", s.Account),
		slog.String("cache_dir", s.CacheDir),
		slog.String("gh_path", s.GH),
	)
	if s.Proxy != "" {
		attrs = append(attrs, slog.String("proxy", s.Proxy))
	}
	attrs = append(attrs, slog.Int("gh_hosts", s.GHHosts), slog.Bool("gh_knows_host", s.GHKnowsHost))
	slog.LogAttrs(context.Background(), slog.LevelInfo, "session", attrs...)
	slog.SetDefault(slog.Default().With(slog.String("account", s.Account)))
}

// newSessionInfo returns who the session that st started, with token,
// through client is, and where it keeps its cache as cfg says.
func newSessionInfo(st start, token accesssvc.Token, client *github.Client, cfg config.Disk) sessionInfo {
	s := sessionInfo{
		Host:        st.Host,
		From:        st.From,
		WebHost:     client.WebHost(),
		Login:       token.Login,
		TokenSource: token.Source,
		TokenKind:   client.Access().Kind.String(),
		Account:     client.Account(),
		CacheDir:    accountDir(cfg, client.Host(), client.Account()),
		GH:          ui.ShortPath(accesssvc.GHPath()),
		Proxy:       proxyHost("https://"+client.Host()+"/", http.ProxyFromEnvironment),
	}
	hosts := auth.KnownHosts()
	s.GHHosts = len(hosts)
	s.GHKnowsHost = slices.ContainsFunc(hosts, func(h string) bool { return normalizeHostname(h) == st.Host })
	if st.Here != (core.RepoRef{}) {
		s.Here = st.Here.String()
	}
	return s
}

// ghVersionTimeout bounds gh --version, which only prints.
const ghVersionTimeout = 5 * time.Second

// logGHVersion logs the version of gh, the one at path, which reads and
// stores the token, in a gh record: it runs gh, so it runs in the
// background. Nothing is logged when gh isn't installed or doesn't say.
func logGHVersion(ctx context.Context, path string) {
	if path == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, ghVersionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Env = append(os.Environ(), "GH_NO_UPDATE_NOTIFIER=1")
	// A child of gh that holds the output open doesn't hold this past the
	// timeout.
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		slog.WarnContext(ctx, "gh version unknown", "span", "app", "err", err.Error())
		return
	}
	// The first line is the version, such as "gh version 2.63.0
	// (2024-11-27)", and the next its release page.
	line, _, _ := strings.Cut(string(out), "\n")
	if line = strings.TrimSpace(line); line != "" && len(line) <= 128 {
		slog.InfoContext(ctx, "gh", "span", "app", "version", line)
	}
}

// accountDir returns the directory of the disk cache that account keeps
// its entries in on host, with the home directory as ~, or "" when cfg
// keeps none.
func accountDir(cfg config.Disk, host, account string) string {
	if !cfg.Enabled {
		return ""
	}
	root, err := cfg.Path()
	if err != nil || root == "" {
		return ""
	}
	return ui.ShortPath(filepath.Join(root, hostDir(host), entryDir, account))
}

// proxyHost returns the host of the proxy that proxy, such as
// http.ProxyFromEnvironment, picks for a request to target, or "" for
// none. A proxy's URL may hold a user and password, so only its host is
// returned.
func proxyHost(target string, proxy func(*http.Request) (*url.URL, error)) string {
	u, err := url.Parse(target)
	if err != nil {
		return ""
	}
	p, err := proxy(&http.Request{URL: u})
	if err != nil || p == nil {
		return ""
	}
	return p.Host
}
