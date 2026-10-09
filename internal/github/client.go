// Package github is the transport to the GitHub REST and GraphQL APIs. It
// handles auth, retries, conditional requests, pagination links, rate
// limits and error mapping so that the domain methods built on it stay
// short.
package github

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cli/go-gh/v2/pkg/auth"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

const apiVersion = "2022-11-28"

// Client talks to the API of one GitHub host. It is safe for concurrent use.
type Client struct {
	http *http.Client
	// token is swapped by SetToken while requests read it.
	token atomic.Pointer[string]
	// authorized is closed once the client has its first token: at once,
	// or by Authorize for a client made WithTokenLater. authorize sets it
	// once.
	authorized chan struct{}
	authorize  sync.Once
	// tokenAccount is TokenAccount, of the token the client started with.
	// It is set before authorized is closed.
	tokenAccount string
	// host is the host as gh names it, for the error of a missing token.
	host       string
	login      string
	restURL    *url.URL
	graphqlURL string
	budget     *budget
	access     *tokenAccess
	// enterprise is set when the host is a GitHub Enterprise Server, and
	// unsupported keeps the queries it lacks fields of.
	enterprise  atomic.Bool
	unsupported unsupported
	// refusals holds the kinds of partial refusals logged so far.
	refusals sync.Map
	// onOld is told once of an Enterprise Server older than supported,
	// and toldOld is set once it was.
	onOld   func(version string)
	toldOld atomic.Bool
	// version is the Enterprise Server version the answers tell, and
	// toldServer is set once the server record is logged.
	version    atomic.Pointer[string]
	toldServer atomic.Bool
	// logURLs keeps the signed URLs of the logs of jobs in progress.
	logURLs logURLs
	// searchSize is the page size of a SearchQuery that sets none.
	searchSize int
}

// Option configures a Client.
type Option func(*options)

type options struct {
	http        *http.Client
	concurrency int
	host        string
	token       string
	source      string
	later       bool
	login       string
	baseURL     string
	gh          ghLookup
	notify      func()
	onAccess    func(core.Access)
	onOld       func(version string)
}

// WithHTTPClient sets the HTTP client that sends requests. Its Timeout
// bounds each attempt at a request, from sending it until its body is
// closed. The default client times out after github.timeout of the
// default config (config.Default).
func WithHTTPClient(c *http.Client) Option {
	return func(o *options) { o.http = c }
}

// WithConcurrency bounds the requests in flight at once to n, from the
// time each is sent until its body is closed, of which foregroundSlots are
// kept for what the user waits for. Without it, and for n below one, it
// is github.concurrency of the default config (config.Default).
func WithConcurrency(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.concurrency = n
		}
	}
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

// WithTokenSource sets the token that FindToken found, and where it
// found it, so that the client acts as one that found the token itself:
// it sends requests the way gh does, and names the account of a token gh
// stores by its login.
func WithTokenSource(token, source string) Option {
	return func(o *options) { o.token, o.source = token, source }
}

// WithTokenLater makes a client whose token comes later, by Authorize,
// from source, as FindToken names it: such as one that gh reads from the
// system keyring, which takes tens of milliseconds that the app can spend
// starting. Requests wait for it.
func WithTokenLater(source string) Option {
	return func(o *options) { o.token, o.source, o.later = "", source, true }
}

// WithLogin sets the login of the account the token is for, when gh
// doesn't store the token, such as one from GH_TOKEN, and the login is
// known from an earlier answer of GitHub. Account then names the account
// by its login. The login gh stores for its own token wins.
func WithLogin(login string) Option {
	return func(o *options) { o.login = login }
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
	def := config.Default()
	o := options{http: &http.Client{Timeout: def.GitHub.Timeout}, concurrency: def.GitHub.Concurrency, gh: ghDefaults()}
	for _, opt := range opts {
		opt(&o)
	}
	if o.host == "" && (o.token == "" || o.baseURL == "") {
		o.host, _ = o.gh.defaultHost()
	}
	source := o.source
	// A token looked for already, and not found, isn't looked for again,
	// which may run gh auth token again.
	if o.token == "" && source == "" && !o.later {
		o.token, source = o.gh.token(o.host)
	}
	if o.token == "" && !o.later {
		return nil, NoTokenError(o.host)
	}
	// A client found the way gh finds one also sends its requests the way
	// gh does, unless it was given a transport.
	if source != "" && o.http.Transport == nil {
		o.http = throughSocket(o.http, o.gh.unixSocket())
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
	// The client is copied so that its transports leave the caller's alone.
	// A request that failed for a moment is sent again, and its timeout
	// bounds each attempt instead of the call. The retries come before the
	// limit, so that no slot is held between attempts, and the limit
	// before the log, so that a request's time waiting for a slot isn't
	// logged as its duration. Each attempt passes the gate of its rate
	// limit before it waits for a slot, so that one held takes none.
	hc := *o.http
	b := newBudget(base.Host, base.EscapedPath(), gql.Path)
	if o.notify != nil {
		b.notifier = newRateNotifier(b, o.notify)
	}
	acc := newTokenAccess(base.Host, o.token, o.onAccess)
	hc.Transport = newRetryTransport(&rateTransport{
		budget: b,
		access: acc,
		base: &timeoutTransport{
			base: newLimitTransport(&logTransport{
				base:        cmp.Or[http.RoundTripper](hc.Transport, http.DefaultTransport),
				restRoot:    b.restRoot,
				graphqlPath: b.graphqlPath,
			}, o.concurrency, foregroundSlots),
			timeout: hc.Timeout,
		},
	})
	hc.Timeout = 0
	c := &Client{
		http:       &hc,
		authorized: make(chan struct{}),
		host:       o.host,
		login:      cmp.Or(o.gh.login(o.host, source), o.login),
		restURL:    base,
		graphqlURL: gql.String(),
		budget:     b,
		access:     acc,
		onOld:      o.onOld,
		searchSize: def.PageSize.Search,
	}
	c.token.Store(&o.token)
	if !o.later {
		c.Authorize(o.token)
	}
	// An Enterprise Server's API is below /api/v3.
	c.enterprise.Store(strings.HasSuffix(base.Path, "/api/v3/"))
	b.gate.probe = c.rateLimits
	return c, nil
}

// Close stops telling of changes to the rate limits, the function that
// WithRateNotify sets, and the timers that would, and the timers of the
// gate. Requests still work, but one that a rate limit holds goes on
// only once an answer lifts the limit, or its context ends.
func (c *Client) Close() {
	c.budget.notifier.stop()
	c.budget.close()
}

// NoTokenError is the error of a client that has no token for host, which
// says how to log in.
func NoTokenError(host string) error {
	return fmt.Errorf("no token for %s, run %s: %w", host, loginCommand(host), core.ErrUnauthorized)
}

// Authorize gives a client made WithTokenLater its token, and sends the
// requests that wait for it. An empty token fails them with
// NoTokenError. What the token may do starts from what its prefix says,
// as though New had it. Only the first call counts; SetToken replaces the
// token after. A token SetToken set meanwhile, as one an :auth refresh
// read while gh still read this one, is the newer, so it stays.
func (c *Client) Authorize(token string) {
	c.authorize.Do(func() {
		none := c.token.Load()
		if *none == "" && c.token.CompareAndSwap(none, &token) {
			c.access.start(token)
		} else {
			token = *c.token.Load()
		}
		c.tokenAccount = tokenAccount(c.restURL.Host, token)
		close(c.authorized)
	})
}

// waitToken waits until the client has its token, or ctx ends. A client
// that has it sends even a request whose context has ended, as it would
// have without waiting, so that the attempt is logged as canceled.
func (c *Client) waitToken(ctx context.Context) error {
	select {
	case <-c.authorized:
		return nil
	default:
	}
	select {
	case <-c.authorized:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// loginCommand is the gh command that logs in to host. gh logs in to
// github.com unless told otherwise.
func loginCommand(host string) string {
	if auth.NormalizeHostname(host) == "github.com" {
		return "gh auth login"
	}
	return "gh auth login --hostname " + host
}

func restRoot(host string) string {
	host = auth.NormalizeHostname(host)
	switch {
	case auth.IsEnterprise(host):
		return "https://" + host + "/api/v3/"
	case host == "github.localhost":
		// A GitHub run locally for development serves plain HTTP.
		return "http://api." + host + "/"
	}
	return "https://api." + host + "/"
}

// APIHost returns the host of the REST API of host, as Client.Host
// returns it for a client of host: api.github.com for github.com, and an
// Enterprise Server's own host. What the app keeps on disk for a host is
// below this name.
func APIHost(host string) string {
	u, err := url.Parse(restRoot(host))
	if err != nil {
		return host
	}
	return u.Hostname()
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

// Account returns a name for the account the client acts as, so that it can
// name what is kept for the account, such as a directory of cached
// responses, without giving the login or the token away. When the token is
// the one gh stores for its active account, the name is a hash of the host
// and that account's login, so it outlasts a refreshed token or a new
// login. A token from the environment may be anyone's, so its name is its
// TokenAccount, unless WithLogin named its login, and then it waits for
// the token as TokenAccount does: never call it from Update.
func (c *Client) Account() string {
	if c.login == "" {
		return c.TokenAccount()
	}
	h := sha256.Sum256([]byte("gh-tui login\x00" + c.restURL.Host + "\x00" + strings.ToLower(c.login)))
	return hex.EncodeToString(h[:16])
}

// TokenAccount returns a name for the token the client started with: a
// hash of the host and the token. Another token, even of the same user,
// has another name, but one set by SetToken keeps the first one's, so
// that what is kept for the account stays where it is for the session.
// Before Account named logins, it returned this. A client made
// WithTokenLater has no token until Authorize, so it waits until then:
// never call it from Update.
func (c *Client) TokenAccount() string {
	<-c.authorized
	return c.tokenAccount
}

func tokenAccount(host, token string) string {
	h := sha256.Sum256([]byte("gh-tui account\x00" + host + "\x00" + token))
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

// send adds the headers every request needs and sends it. A request that
// asks for another media type keeps its Accept header.
func (c *Client) send(req *http.Request) (*http.Response, error) {
	return c.sendWith(c.http, req)
}

// sendWith is send with hc, such as a copy of the client's that doesn't
// follow redirects.
func (c *Client) sendWith(hc *http.Client, req *http.Request) (*http.Response, error) {
	if err := c.waitToken(req.Context()); err != nil {
		return nil, err
	}
	token := *c.token.Load()
	if token == "" {
		return nil, NoTokenError(c.host)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", userAgent)
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, offline(req.Context(), err)
	}
	if req.URL.Host == c.restURL.Host {
		c.observeVersion(req.Context(), resp.Header.Get(enterpriseHeader))
		c.observeServer(req.Context(), resp.Header, time.Now())
	}
	return resp, nil
}
