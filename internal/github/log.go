package github

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// logTransport logs each HTTP attempt as one record, once its body is
// closed, so that the record has the time and bytes that reading the
// body took too, and counts it in the stats of package obs. It never logs
// headers or bodies, which hold the token and private content.
type logTransport struct {
	base http.RoundTripper
	// restRoot and graphqlPath are the paths of the REST root, such as /
	// or /api/v3/, and of the GraphQL endpoint.
	restRoot    string
	graphqlPath string
	// deprecated holds the routes that GitHub said are deprecated, which
	// are told once.
	deprecated sync.Map
}

// call is what the client knows of a request that the transport can't see
// in it: the GraphQL operation, the repository it is about, and what the
// query reported of its rate limit. The client fills it in before it closes
// the body.
type call struct {
	op   string
	repo string
	rate *graphqlRate
	// query marks a GraphQL query, which only reads, so that it may be
	// sent again like a GET, unlike a mutation.
	query bool
	// shape names what a query costs alike: its operation, or the text
	// of a query without a name, since those all share the name query.
	shape string
	// external marks a request to a host outside the API, such as the
	// storage that a job log redirects to. Its URL holds a signed
	// credential, so it is logged by op alone, as API download.
	external bool
	// errors are those of a GraphQL answer, which came with an OK
	// status, and partial says that data came with them.
	errors  []GraphQLErrorItem
	partial bool
}

// apiDownload is the API of the requests that a call marks external.
const apiDownload = "download"

type callKey struct{}

// withCall returns ctx carrying c for the transport.
func withCall(ctx context.Context, c *call) context.Context {
	return context.WithValue(ctx, callKey{}, c)
}

type heldKey struct{}

// withHeld returns ctx carrying d, how long its attempt was held for its
// rate limit, which the log records.
func withHeld(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, heldKey{}, d)
}

// RoundTrip sends req with the base transport, and logs it once the
// response's body is closed, or at once if there is no response. A request
// that ends early, such as one canceled when the user navigates away, is
// logged too, with the error.
func (t *logTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	a := &attempt{t: t, req: req, id: obs.NewID(obs.RequestPrefix), start: time.Now()}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		a.done(nil, err)
		return nil, err
	}
	a.headers = time.Now()
	resp.Body = &loggedBody{ReadCloser: resp.Body, a: a, resp: resp}
	return resp, nil
}

// attempt is one HTTP request being logged.
type attempt struct {
	t              *logTransport
	req            *http.Request
	id             string
	start, headers time.Time
	bytes          int64
	// readErr is why reading the body stopped before its end, if it did.
	readErr error
	// held is set once the record of the attempt, which failed at
	// heldLevel, and timed out if heldTimedOut, was held until the retry
	// transport decided.
	held         bool
	heldLevel    slog.Level
	heldTimedOut bool
}

// loggedBody counts what is read of a response and logs the attempt when
// it is closed.
type loggedBody struct {
	io.ReadCloser
	a    *attempt
	resp *http.Response
	once sync.Once
}

func (b *loggedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.a.bytes += int64(n)
	if err != nil && !errors.Is(err, io.EOF) && b.a.readErr == nil {
		b.a.readErr = err
	}
	return n, err
}

func (b *loggedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() { b.a.done(b.resp, nil) })
	return err
}

// done logs the attempt and counts it. resp is nil when err is set, and a
// body that failed while it was read logs its error. A failed attempt
// that the retry transport sends again is logged at debug level.
func (a *attempt) done(resp *http.Response, err error) {
	ctx := a.req.Context()
	if err == nil {
		err = a.readErr
	}
	elapsed := time.Since(a.start)
	c, _ := ctx.Value(callKey{}).(*call)
	api, route, repo := a.t.route(a.req, c)

	h := obs.HTTP{API: api, Method: a.req.Method, Route: route, Duration: elapsed, Bytes: a.bytes}
	level := slog.LevelInfo
	var retryAfter string
	if resp != nil {
		h.Status = resp.StatusCode
		h.NotModified = resp.StatusCode == http.StatusNotModified
		h.Rate = headerRate(resp.Header)
		retryAfter = resp.Header.Get("Retry-After")
		switch {
		case resp.StatusCode >= http.StatusInternalServerError:
			level = slog.LevelError
		case resp.StatusCode >= http.StatusBadRequest:
			level = slog.LevelWarn
		}
	}
	if c != nil && c.rate != nil {
		h.Cost = c.rate.Cost
		if h.Rate.Resource == "" {
			h.Rate = c.rate.obs()
		}
	}
	// A canceled request is a decision, not a failure, but it cost a
	// request, so it is logged as one. One that ran out of time, its own
	// or its caller's, degrades what the user sees. ctx is the attempt's,
	// which the timeout ends, so its error tells the two apart.
	timedOut := err != nil && (errors.Is(ctx.Err(), context.DeadlineExceeded) || isTimeout(err))
	canceled := err != nil && !timedOut && (errors.Is(err, context.Canceled) || ctx.Err() != nil)
	switch {
	case canceled:
		level = slog.LevelInfo
	case timedOut:
		level = slog.LevelWarn
	case err != nil:
		level = slog.LevelError
	}
	// An answer with errors and no data failed, whatever its status.
	if err == nil && c != nil && len(c.errors) > 0 && !c.partial {
		level = max(level, slog.LevelWarn)
	}
	// The background loops, the polls and the revalidator, sum up what
	// their requests found in records of their own, so a request of
	// theirs that went well is detail.
	if level == slog.LevelInfo && err == nil && obs.IsBackground(ctx) && (c == nil || len(c.errors) == 0) {
		level = slog.LevelDebug
	}
	if a.held {
		// Logged once the retry transport decided, when the attempt's
		// context may be done: the attempt failed as it did when held.
		canceled, timedOut, level = false, a.heldTimedOut, a.heldLevel
	}
	if s, _ := ctx.Value(settleKey{}).(*settle); s != nil && level > slog.LevelInfo {
		a.heldLevel, a.heldTimedOut = level, timedOut
		if !a.held && s.hold(func() { a.held = true; a.done(resp, err) }) {
			return
		}
		if s.sentAgain() {
			level = slog.LevelDebug
		}
	}
	if resp != nil && (c == nil || !c.external) {
		a.t.deprecation(ctx, api, route, resp.Header)
	}
	obs.CountHTTP(h)
	switch {
	case resp == nil:
	case api == obs.GraphQL:
		obs.ChargeGraphQL(ctx, h.Cost)
	default:
		obs.ChargeREST(ctx, h.Rate.Resource, h.NotModified)
	}

	if !obs.Enabled(ctx, level) {
		return
	}
	attrs := make([]slog.Attr, 0, 16)
	attrs = append(attrs,
		slog.String("span", "http"),
		slog.String("request_id", a.id),
		slog.String("api", api),
		slog.String("method", a.req.Method),
		slog.String("route", route),
	)
	if repo != "" {
		attrs = append(attrs, slog.String("repo", repo))
	}
	attrs = append(attrs, a.t.hop(a.req, c)...)
	if resp != nil {
		if id := resp.Header.Get("X-GitHub-Request-Id"); id != "" {
			attrs = append(attrs, slog.String("gh_request_id", id))
		}
		attrs = append(attrs, slog.Int("status", resp.StatusCode), slog.Bool("not_modified", h.NotModified))
		if resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError && (c == nil || !c.external) {
			attrs = append(attrs, refusalAttrs(resp.Header)...)
		}
	} else {
		// No response came: the status is 0.
		attrs = append(attrs, slog.Int("status", 0))
	}
	// A conditional request that got a 200 tells a change, or validators
	// that didn't hold.
	if c == nil || !c.external {
		attrs = append(attrs, slog.Bool("conditional", a.req.Header.Get("If-None-Match") != "" || a.req.Header.Get("If-Modified-Since") != ""))
	}
	attrs = append(attrs, slog.Float64("duration_ms", obs.Millis(elapsed)), slog.Int64("bytes", a.bytes))
	if r := h.Rate; r.Resource != "" || h.Cost > 0 {
		rate := []any{
			slog.String("resource", r.Resource),
			slog.Int("limit", r.Limit),
			slog.Int("remaining", r.Remaining),
			slog.Int("used", r.Used),
			slog.Time("reset", r.Reset),
		}
		if c != nil && c.rate != nil {
			rate = append(rate, slog.Int("cost", h.Cost))
		}
		attrs = append(attrs, slog.Group("rate", rate...))
	}
	if retryAfter != "" {
		attrs = append(attrs, slog.String("retry_after", retryAfter))
	}
	if held, ok := ctx.Value(heldKey{}).(time.Duration); ok {
		attrs = append(attrs, slog.Float64("held_ms", obs.Millis(held)))
	}
	if n, ok := ctx.Value(attemptKey{}).(int); ok {
		attrs = append(attrs, slog.Int("attempt", n))
	}
	if err != nil {
		attrs = append(attrs, slog.String("err", err.Error()), slog.Bool("canceled", canceled), slog.Bool("timed_out", timedOut))
	}
	if c != nil && len(c.errors) > 0 {
		attrs = append(attrs, graphqlErrorAttrs(c.errors)...)
	}
	if obs.Enabled(ctx, slog.LevelDebug) && (c == nil || !c.external) {
		attrs = append(attrs, slog.String("path", a.req.URL.EscapedPath()))
		if q := a.req.URL.RawQuery; q != "" {
			attrs = append(attrs, slog.String("query", logQuery(q)))
		}
		if !a.headers.IsZero() {
			attrs = append(attrs, slog.Float64("ttfb_ms", obs.Millis(a.headers.Sub(a.start))))
		}
		if resp != nil {
			if mt := resp.Header.Get("X-GitHub-Media-Type"); mt != "" && len(mt) <= maxHeaderLogged {
				attrs = append(attrs, slog.String("media_type", mt))
			}
			if link := resp.Header.Get("Link"); link != "" {
				attrs = append(attrs, slog.Bool("has_next", parseLinks(link)["next"] != ""))
			}
		}
	}
	slog.LogAttrs(ctx, level, "http", attrs...)
}

// hop returns what a record says of where req came from: the host of a
// download outside the API, never its signed path or query, and for a
// request that follows a redirect, the status and route of the answer
// that redirected it, which ties the two records together.
func (t *logTransport) hop(req *http.Request, c *call) []slog.Attr {
	if c != nil && c.external {
		return []slog.Attr{slog.String("download_host", req.URL.Hostname())}
	}
	prev := req.Response
	if prev == nil || prev.Request == nil || prev.Request.URL == nil {
		return nil
	}
	_, route, _ := t.route(prev.Request, nil)
	return []slog.Attr{slog.String("redirected_from", strconv.Itoa(prev.StatusCode)+" "+route)}
}

// isTimeout reports whether err says that time ran out, as a dial or a
// TLS handshake that took too long does.
func isTimeout(err error) bool {
	ne, ok := errors.AsType[net.Error](err)
	return ok && ne.Timeout()
}

// maxLoggedScopes and maxLoggedPaths bound what a record lists, and
// maxHeaderLogged the value of a header it holds, far more than GitHub
// sends.
const (
	maxLoggedScopes = 16
	maxLoggedPaths  = 3
	maxHeaderLogged = 128
)

// deprecation warns, once per route, that GitHub said with the headers h
// of an answer that route of api is deprecated, or when it goes away.
func (t *logTransport) deprecation(ctx context.Context, api, route string, h http.Header) {
	dep, sunset := h.Get("Deprecation"), h.Get("Sunset")
	if dep == "" && sunset == "" {
		return
	}
	if _, told := t.deprecated.LoadOrStore(api+" "+route, true); told {
		return
	}
	attrs := []slog.Attr{slog.String("span", "http"), slog.String("api", api), slog.String("route", route)}
	if dep != "" && len(dep) <= maxHeaderLogged {
		attrs = append(attrs, slog.String("deprecation", dep))
	}
	if sunset != "" && len(sunset) <= maxHeaderLogged {
		attrs = append(attrs, slog.String("sunset", sunset))
	}
	slog.LogAttrs(ctx, slog.LevelWarn, "api deprecated", attrs...)
}

// refusalAttrs returns what the headers h of a 4xx answer say of why it
// was refused: the scopes of which the endpoint accepts one, the
// permissions it needs of a fine-grained or App token, and whether SSO
// stands in the way. The value of X-GitHub-SSO names an authorization
// request, so only its kind is logged.
func refusalAttrs(h http.Header) []slog.Attr {
	var attrs []slog.Attr
	if v, ok := h[http.CanonicalHeaderKey("X-Accepted-OAuth-Scopes")]; ok {
		scopes := core.ParseScopes(strings.Join(v, ","))
		attrs = append(attrs, slog.Any("accepted_scopes", append([]string{}, scopes[:min(len(scopes), maxLoggedScopes)]...)))
	}
	// Only a value of permissions GitHub's shape is logged.
	if v := h.Get("X-Accepted-GitHub-Permissions"); permissionsNeeded(v) != "" {
		attrs = append(attrs, slog.String("needs_permissions", v))
	}
	for _, v := range h.Values("X-GitHub-SSO") {
		kind, _, _ := strings.Cut(v, ";")
		if kind = strings.TrimSpace(kind); kind != "" && len(kind) <= 32 && strings.Trim(kind, "abcdefghijklmnopqrstuvwxyz-") == "" {
			attrs = append(attrs, slog.String("sso", kind))
			break
		}
	}
	return attrs
}

// graphqlErrorAttrs returns what a record says of the errors of a GraphQL
// answer: how many there are, their types and codes, each once, and the
// paths of the first few. Their messages may name private things, so
// they are left out.
func graphqlErrorAttrs(items []GraphQLErrorItem) []slog.Attr {
	var types, codes, paths []string
	for _, item := range items {
		if item.Type != "" && !slices.Contains(types, item.Type) {
			types = append(types, item.Type)
		}
		if e := item.Extensions; e != nil && e.Code != "" && !slices.Contains(codes, e.Code) {
			codes = append(codes, e.Code)
		}
		if len(item.Path) > 0 && len(paths) < maxLoggedPaths {
			paths = append(paths, graphqlPath(item.Path))
		}
	}
	attrs := []slog.Attr{slog.Int("gql_errors", len(items))}
	if len(types) > 0 {
		attrs = append(attrs, slog.Any("gql_types", types))
	}
	if len(codes) > 0 {
		attrs = append(attrs, slog.Any("gql_codes", codes))
	}
	if len(paths) > 0 {
		attrs = append(attrs, slog.Any("gql_paths", paths))
	}
	return attrs
}

// headerRate returns the rate limit that the headers of a response report,
// with the resource it counts against.
func headerRate(h http.Header) obs.Rate {
	rl, ok := parseRateLimit(h)
	if !ok {
		return obs.Rate{}
	}
	used, err := strconv.Atoi(h.Get("X-RateLimit-Used"))
	if err != nil {
		used = rl.Limit - rl.Remaining
	}
	return obs.Rate{Resource: rl.Resource, Limit: rl.Limit, Remaining: rl.Remaining, Used: used, Reset: rl.Reset}
}

// route returns the API of req, and its route and repository: for REST, the
// path below the root with its variable parts replaced; for GraphQL, the
// operation of the query.
func (t *logTransport) route(req *http.Request, c *call) (api, route, repo string) {
	if c != nil && c.external {
		return apiDownload, c.op, c.repo
	}
	if req.URL.Path == t.graphqlPath {
		if c == nil {
			return obs.GraphQL, "graphql", ""
		}
		return obs.GraphQL, c.op, c.repo
	}
	route, repo = restRoute(strings.TrimPrefix(req.URL.EscapedPath(), t.restRoot))
	return obs.REST, route, repo
}

// restRoute templates path, a REST path below the root such as
// repos/cli/cli/issues/42/comments, into a route that the records of every
// issue share, such as /repos/{owner}/{repo}/issues/{number}/comments, and
// returns the repository it names, if any, as owner/name.
func restRoute(path string) (route, repo string) {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	var b strings.Builder
	// prev is the segment written last, as a word or a placeholder.
	prev := ""
	for i := 0; i < len(segs); i++ {
		seg := segs[i]
		b.WriteByte('/')
		switch {
		case (prev == "repos" || prev == "starred") && i+1 < len(segs):
			repo = unescape(seg) + "/" + unescape(segs[i+1])
			prev = "{owner}/{repo}"
			i++
		case restWords[seg] && restValues[prev].name == "":
			prev = seg
		default:
			var rest bool
			prev, rest = placeholder(prev, seg)
			if rest {
				b.WriteString(prev)
				return b.String(), repo
			}
		}
		b.WriteString(prev)
	}
	return b.String(), repo
}

// placeholder names the variable segment seg that follows prev, and reports
// whether it takes the rest of the path, as a file path or a ref may.
func placeholder(prev, seg string) (name string, rest bool) {
	if v, ok := restValues[prev]; ok {
		return v.name, v.rest
	}
	if _, err := strconv.Atoi(seg); err == nil {
		return "{number}", false
	}
	return "{}", false
}

// restValues name what follows the words of REST paths that a value
// follows, such as the SHA after trees, and whether it takes the rest of
// the path.
var restValues = map[string]struct {
	name string
	rest bool
}{
	"contents": {"{path}", true},
	"ref":      {"{ref}", true},
	"refs":     {"{ref}", true},
	"compare":  {"{ref}", true},
	"trees":    {"{sha}", false},
	"blobs":    {"{sha}", false},
	"commits":  {"{sha}", false},
	"labels":   {"{name}", false},
	"threads":  {"{id}", false},
	"comments": {"{id}", false},
	"reviews":  {"{id}", false},

	"runs":       {"{run_id}", false},
	"attempts":   {"{attempt}", false},
	"jobs":       {"{job_id}", false},
	"workflows":  {"{workflow_id}", false},
	"check-runs": {"{check_run_id}", false},
}

func unescape(s string) string {
	if u, err := url.PathUnescape(s); err == nil {
		return u
	}
	return s
}

// restWords are the fixed segments of the REST paths.
var restWords = map[string]bool{
	"repos": true, "user": true, "users": true, "orgs": true, "starred": true,
	"issues": true, "pulls": true, "comments": true, "labels": true, "assignees": true,
	"events": true, "timeline": true, "reactions": true, "reviews": true, "merge": true,
	"git": true, "trees": true, "blobs": true, "commits": true, "ref": true, "refs": true,
	"contents": true, "readme": true, "branches": true, "tags": true, "releases": true,
	"notifications": true, "threads": true, "subscription": true,
	"search": true, "repositories": true, "code": true, "rate_limit": true,
	"compare": true, "files": true, "milestones": true,
	"actions": true, "runs": true, "attempts": true, "jobs": true, "workflows": true, "logs": true,
	"rerun": true, "rerun-failed-jobs": true, "cancel": true, "check-runs": true, "annotations": true,
}

// plainParams are the query parameters whose values the log keeps: pages,
// sorting and switches, which the app picks. The others, such as q or
// labels, hold what the user typed into a search or a filter.
var plainParams = map[string]bool{
	"page": true, "per_page": true, "sort": true, "direction": true, "state": true, "all": true,
	"participating": true, "recursive": true, "filter": true, "status": true, "event": true,
	"since": true, "before": true, "after": true,
}

// logQuery returns raw, the query of a request, as its record names it:
// every parameter in order, with the value of one that may hold what the
// user typed replaced by how long it is, as q=…12, so that a debug log
// pasted into an issue doesn't carry a search or a filter.
func logQuery(raw string) string {
	params := strings.Split(raw, "&")
	for i, p := range params {
		name, value, ok := strings.Cut(p, "=")
		if !ok || plainParams[name] {
			continue
		}
		if v, err := url.QueryUnescape(value); err == nil {
			value = v
		}
		params[i] = name + "=…" + strconv.Itoa(utf8.RuneCountInString(value))
	}
	return strings.Join(params, "&")
}
