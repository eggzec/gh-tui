// Package github is the transport to the GitHub REST and GraphQL APIs. It
// handles auth, conditional requests, pagination links, rate limits and
// error mapping so that the domain methods built on it stay short.
package github

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/auth"

	"github.com/eggzec/gh-tui/internal/core"
)

const (
	apiVersion = "2022-11-28"
	userAgent  = "gh-tui"

	// defaultTimeout bounds requests whose context has no deadline.
	defaultTimeout = 30 * time.Second
)

// Client talks to the API of one GitHub host. It is safe for concurrent use.
type Client struct {
	http       *http.Client
	token      string
	restURL    *url.URL
	graphqlURL string
}

// Option configures a Client.
type Option func(*options)

type options struct {
	http    *http.Client
	host    string
	token   string
	baseURL string
}

// WithHTTPClient sets the HTTP client that sends requests. The default
// client times out after 30 seconds.
func WithHTTPClient(c *http.Client) Option {
	return func(o *options) { o.http = c }
}

// WithHost sets the GitHub host, such as github.com or a GitHub Enterprise
// Server hostname. By default it is the host gh is logged in to.
func WithHost(host string) Option {
	return func(o *options) { o.host = host }
}

// WithToken sets the token. By default it is the token gh uses for the host.
func WithToken(token string) Option {
	return func(o *options) { o.token = token }
}

// WithBaseURL sets the REST API root, such as https://api.github.com/. The
// GraphQL endpoint is derived from it: graphql below the root, or
// /api/graphql when the root ends in /api/v3 as on GitHub Enterprise Server.
func WithBaseURL(u string) Option {
	return func(o *options) { o.baseURL = u }
}

// New returns a client. Without options it finds the host and token the
// same way the gh CLI does.
func New(opts ...Option) (*Client, error) {
	o := options{http: &http.Client{Timeout: defaultTimeout}}
	for _, opt := range opts {
		opt(&o)
	}
	if o.host == "" && (o.token == "" || o.baseURL == "") {
		o.host, _ = auth.DefaultHost()
	}
	if o.token == "" {
		o.token, _ = auth.TokenForHost(o.host)
	}
	if o.token == "" {
		return nil, fmt.Errorf("no token for %s, run gh auth login: %w", o.host, core.ErrUnauthorized)
	}
	if o.baseURL == "" {
		o.baseURL = restRoot(o.host)
	}
	if !strings.HasSuffix(o.baseURL, "/") {
		o.baseURL += "/"
	}
	base, err := url.Parse(o.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	return &Client{
		http:       o.http,
		token:      o.token,
		restURL:    base,
		graphqlURL: graphqlEndpoint(base),
	}, nil
}

func restRoot(host string) string {
	host = auth.NormalizeHostname(host)
	if auth.IsEnterprise(host) {
		return "https://" + host + "/api/v3/"
	}
	return "https://api." + host + "/"
}

func graphqlEndpoint(base *url.URL) string {
	u := *base
	if prefix, ok := strings.CutSuffix(u.Path, "/api/v3/"); ok {
		u.Path = prefix + "/api/graphql"
	} else {
		u.Path += "graphql"
	}
	return u.String()
}

// resolve turns path into a URL. Path is relative to the REST root, or an
// absolute URL on the same host, as found in Link headers. Other hosts are
// refused so that the token never leaves the API host.
func (c *Client) resolve(path string) (string, error) {
	u, err := c.restURL.Parse(strings.TrimPrefix(path, "/"))
	if err != nil {
		return "", fmt.Errorf("parse path: %w", err)
	}
	if u.Scheme != c.restURL.Scheme || u.Host != c.restURL.Host {
		return "", fmt.Errorf("%s is not on %s", path, c.restURL.Host)
	}
	return u.String(), nil
}

// send adds the headers every request needs and sends it.
func (c *Client) send(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	return c.http.Do(req)
}
