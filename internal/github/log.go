package github

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

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
}

// call is what the client knows of a request that the transport can't see
// in it: the GraphQL operation, the repository it is about, and what the
// query reported of its rate limit. The client fills it in before it closes
// the body.
type call struct {
	op   string
	repo string
	rate *graphqlRate
	// external marks a request to a host outside the API, such as the
	// storage that a job log redirects to. Its URL holds a signed
	// credential, so it is logged by op alone, as API download.
	external bool
}

// apiDownload is the API of the requests that a call marks external.
const apiDownload = "download"

type callKey struct{}

// withCall returns ctx carrying c for the transport.
func withCall(ctx context.Context, c *call) context.Context {
	return context.WithValue(ctx, callKey{}, c)
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
// body that failed while it was read logs its error.
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
	// request, so it is logged as one.
	canceled := err != nil && (errors.Is(err, context.Canceled) || ctx.Err() != nil)
	switch {
	case canceled:
		level = slog.LevelInfo
	case err != nil:
		level = slog.LevelError
	}
	obs.CountHTTP(h)

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
	if resp != nil {
		if id := resp.Header.Get("X-GitHub-Request-Id"); id != "" {
			attrs = append(attrs, slog.String("gh_request_id", id))
		}
		attrs = append(attrs, slog.Int("status", resp.StatusCode), slog.Bool("not_modified", h.NotModified))
	} else {
		// No response came: the status is 0.
		attrs = append(attrs, slog.Int("status", 0))
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
	if err != nil {
		attrs = append(attrs, slog.String("err", err.Error()), slog.Bool("canceled", canceled))
	}
	if obs.Enabled(ctx, slog.LevelDebug) && (c == nil || !c.external) {
		attrs = append(attrs, slog.String("path", a.req.URL.EscapedPath()))
		if q := a.req.URL.RawQuery; q != "" {
			attrs = append(attrs, slog.String("query", q))
		}
		attrs = append(attrs, slog.Bool("conditional", a.req.Header.Get("If-None-Match") != "" || a.req.Header.Get("If-Modified-Since") != ""))
		if !a.headers.IsZero() {
			attrs = append(attrs, slog.Float64("ttfb_ms", obs.Millis(a.headers.Sub(a.start))))
		}
	}
	slog.LogAttrs(ctx, level, "http", attrs...)
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
	"compare": true, "files": true,
	"actions": true, "runs": true, "attempts": true, "jobs": true, "workflows": true, "logs": true,
	"rerun": true, "rerun-failed-jobs": true, "cancel": true, "check-runs": true, "annotations": true,
}
