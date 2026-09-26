// Package github is the transport to the GitHub REST and GraphQL APIs. It
// handles auth, conditional requests, pagination links, rate limits and
// error mapping so that the domain methods built on it stay short.
package github

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
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
	now        func() time.Time

	mu        sync.Mutex
	rateLimit RateLimit
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
	gql := graphqlEndpoint(base)
	// The client is copied so that logging leaves the caller's alone. The
	// limit comes first, so that a request's time waiting for a slot isn't
	// logged as its duration.
	hc := *o.http
	hc.Transport = newLimitTransport(&logTransport{
		base:        cmp.Or[http.RoundTripper](hc.Transport, http.DefaultTransport),
		restRoot:    base.EscapedPath(),
		graphqlPath: gql.Path,
	}, maxInFlight, foregroundSlots)
	return &Client{
		http:       &hc,
		token:      o.token,
		restURL:    base,
		graphqlURL: gql.String(),
		now:        time.Now,
	}, nil
}

func restRoot(host string) string {
	host = auth.NormalizeHostname(host)
	if auth.IsEnterprise(host) {
		return "https://" + host + "/api/v3/"
	}
	return "https://api." + host + "/"
}

func graphqlEndpoint(base *url.URL) *url.URL {
	u := *base
	if prefix, ok := strings.CutSuffix(u.Path, "/api/v3/"); ok {
		u.Path = prefix + "/api/graphql"
	} else {
		u.Path += "graphql"
	}
	u.RawPath = ""
	return &u
}

// Host returns the host of the API the client talks to, such as
// api.github.com, or a GitHub Enterprise Server hostname.
func (c *Client) Host() string {
	return c.restURL.Hostname()
}

// WebHost returns the host of the web pages of the GitHub the client talks
// to, with its port if it has one, as its links name it: an Enterprise
// Server's own host, whose API is below /api/v3, and otherwise the API
// host without its api. prefix, such as github.com for api.github.com or
// a GHE.com tenant for its api. host.
func (c *Client) WebHost() string {
	if strings.HasSuffix(strings.TrimSuffix(c.restURL.Path, "/"), "/api/v3") {
		return c.restURL.Host
	}
	if h, ok := strings.CutPrefix(c.restURL.Host, "api."); ok {
		return h
	}
	return c.restURL.Host
}

// Account returns a name for the account the client acts as: a hash of the
// host and the token, so that it can name what is kept for the account, such
// as a directory of cached responses, without giving the token away. Another
// token, even of the same user, has another name.
func (c *Client) Account() string {
	h := sha256.Sum256([]byte("gh-tui account\x00" + c.restURL.Host + "\x00" + c.token))
	return hex.EncodeToString(h[:16])
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

// send adds the headers every request needs, sends it, and records the rate
// limit of the response. A request that asks for another media type keeps
// its Accept header.
func (c *Client) send(req *http.Request) (*http.Response, error) {
	return c.sendWith(c.http, req)
}

// sendWith is send with hc, such as a copy of the client's that doesn't
// follow redirects.
func (c *Client) sendWith(hc *http.Client, req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", userAgent)
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	c.observe(resp.Header)
	return resp, nil
}
