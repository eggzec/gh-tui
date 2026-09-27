package github

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// maxRetries is how many times a request is sent again at most.
const maxRetries = 2

// retryBackoff is the wait before each retry, give or take retryJitter of
// it, so that clients that failed together don't come back together.
var retryBackoff = [maxRetries]time.Duration{250 * time.Millisecond, time.Second}

const retryJitter = 0.2

// retryWithin is how long after it was first sent a request may still be
// sent again. An attempt that ran into its timeout took longer, so the
// user never waits for the timeout more than once, while an attempt that
// failed fast is sent again.
const retryWithin = 5 * time.Second

// maxPeek is the largest GraphQL body that is read to see whether its
// query went wrong. A larger one has data, and passes through as it is.
const maxPeek = 8 << 20

// Why a request was sent again, as the stats count it.
const (
	retryDial        = "dial"
	retryTimeout     = "timeout"
	retryNetwork     = "network"
	retryUnavailable = "unavailable"
	retryGraphQL     = "graphql"
	retryLimit       = "secondary_limit"
)

// retryTransport sends a request again when an attempt failed in a way
// that is likely to pass at once, so that the user never hears of it.
//
// Reads, which a GET or a GraphQL query is, are sent again after an error
// of the connection, a 502, 503 or 504, or a GraphQL answer with no data
// and only errors without a type, which is how GitHub says that something
// went wrong on its side. A read that meets a secondary rate limit that
// lifts within maxSecondaryWait is sent again once, which the gate below
// holds until the limit lifts. A write is sent again
// only when its connection couldn't be made, since then GitHub never saw
// it; after that, GitHub may have done what it asked even if no answer
// came. No request is sent again once retryWithin has passed since it was
// first sent. A primary rate limit, reads ahead (obs.IsPrefetch) and the
// requests of background loops (obs.IsBackground), which come back to
// them anyway, are never sent again.
//
// It sits above the limit, so that each attempt takes and gives back a
// slot of its own and none is held during a wait, and above the log, so
// that each attempt is logged with a request_id of its own.
type retryTransport struct {
	base http.RoundTripper
	// jitter returns a number in [0, 1) that spreads the waits.
	jitter func() float64
	// wait waits for d, or until ctx is done.
	wait func(ctx context.Context, d time.Duration) error
}

func newRetryTransport(base http.RoundTripper) *retryTransport {
	return &retryTransport{base: base, jitter: rand.Float64, wait: sleep}
}

// RoundTrip sends req, and again as long as the policy allows.
func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if obs.IsPrefetch(ctx) || obs.IsBackground(ctx) || !replayable(req) {
		return t.base.RoundTrip(req)
	}
	c, _ := ctx.Value(callKey{}).(*call)
	query := c != nil && c.query
	read := req.Method == http.MethodGet || req.Method == http.MethodHead || query
	var limited bool
	start := time.Now()
	attempt := req
	for n := 1; ; n++ {
		resp, err := t.base.RoundTrip(attempt)
		if n > maxRetries {
			return resp, err
		}
		var why string
		if err == nil && query && resp.StatusCode == http.StatusOK && resp.Body != nil {
			resp, why, err = peekGraphQL(resp)
		}
		switch {
		case why != "":
		case err != nil:
			why = transportRetry(ctx, err, read)
		case read:
			why = statusRetry(resp, &limited)
		}
		if why == "" || time.Since(start) > retryWithin {
			return resp, err
		}
		if resp != nil {
			discard(resp)
		}
		var wait time.Duration
		if why != retryLimit {
			wait = t.backoff(n)
		}
		obs.CountHTTPRetry(why)
		if obs.Enabled(ctx, slog.LevelDebug) {
			logRetry(ctx, req, c, n, why, wait, resp, err)
		}
		if err := t.wait(ctx, wait); err != nil {
			return nil, err
		}
		if attempt, err = replay(req); err != nil {
			return nil, err
		}
	}
}

// backoff is the wait before retry n, 1 for the first.
func (t *retryTransport) backoff(n int) time.Duration {
	f := 1 - retryJitter + 2*retryJitter*t.jitter()
	return time.Duration(float64(retryBackoff[n-1]) * f)
}

// replayable reports whether the body of req, if it has one, can be sent
// again.
func replayable(req *http.Request) bool {
	return req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
}

// replay returns a copy of req to send again, with a new body.
func replay(req *http.Request) (*http.Request, error) {
	r := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		r.Body = body
	}
	return r, nil
}

// transportRetry returns why a request whose attempt failed with err,
// which got no response, may be sent again, or "" if it may not. A
// cancellation is final, and so is a rate limit, which the gate already
// held the attempt for as long as it may wait, and a certificate that
// isn't trusted, which won't be trusted a moment later either.
func transportRetry(ctx context.Context, err error, read bool) string {
	if ctx.Err() != nil || errors.Is(err, core.ErrRateLimited) {
		return ""
	}
	if dialFailed(err) {
		return retryDial
	}
	if !read {
		return ""
	}
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return ""
	}
	if e, ok := errors.AsType[interface {
		error
		Timeout() bool
	}](err); ok && e.Timeout() {
		return retryTimeout
	}
	return retryNetwork
}

// dialFailed reports whether err is of a connection that couldn't be
// made, so that nothing of the request was sent. It looks down the chain,
// since behind a proxy the dial's error comes wrapped in one of the
// CONNECT to the proxy.
func dialFailed(err error) bool {
	for op, ok := errors.AsType[*net.OpError](err); ok; op, ok = errors.AsType[*net.OpError](op.Err) {
		if op.Op == "dial" {
			return true
		}
	}
	return false
}

// statusRetry returns why a read that got resp may be sent again, or "" if
// it may not. limited says whether the read met a secondary rate limit
// already, which it sits out only once.
func statusRetry(resp *http.Response, limited *bool) string {
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return retryUnavailable
	case http.StatusForbidden, http.StatusTooManyRequests:
		if *limited || !shortSecondary(resp.Header) {
			return ""
		}
		*limited = true
		return retryLimit
	}
	return ""
}

// shortSecondary reports whether a secondary rate limit says, in
// Retry-After, that it lifts within maxSecondaryWait. A response whose
// quota is spent is a primary limit, which lasts until its reset.
func shortSecondary(h http.Header) bool {
	if h.Get("X-RateLimit-Remaining") == "0" {
		return false
	}
	s := h.Get("Retry-After")
	var wait time.Duration
	if secs, err := strconv.Atoi(s); err == nil {
		wait = time.Duration(secs) * time.Second
	} else if at, err := http.ParseTime(s); err == nil {
		wait = time.Until(at)
	} else {
		return false
	}
	return wait > 0 && wait <= maxSecondaryWait
}

// peekGraphQL reads the body of resp, the 200 of a GraphQL query, and
// returns why to send the query again if the answer has no data and only
// errors without a type. Otherwise resp reads the same body as it came.
// A body that breaks off returns no response and the error.
func peekGraphQL(resp *http.Response) (*http.Response, string, error) {
	if resp.ContentLength > maxPeek {
		return resp, "", nil
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxPeek+1))
	if err != nil {
		_ = resp.Body.Close()
		return nil, "", err
	}
	resp.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(b), resp.Body), resp.Body}
	if len(b) > maxPeek || !bytes.Contains(b, []byte(`"errors"`)) {
		return resp, "", nil
	}
	// Data is only looked at, not kept.
	var body struct {
		Data   *struct{} `json:"data"`
		Errors []struct {
			Type string `json:"type"`
		} `json:"errors"`
	}
	if json.Unmarshal(b, &body) != nil || body.Data != nil || len(body.Errors) == 0 {
		return resp, "", nil
	}
	for _, e := range body.Errors {
		if e.Type != "" {
			return resp, "", nil
		}
	}
	return resp, retryGraphQL, nil
}

// discard reads a little of the body of resp, so that its connection can
// be used again, and closes it, which gives its slot back.
func discard(resp *http.Response) {
	if resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}

// sleep waits for d, or until ctx is done.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// logRetry logs the decision to send req again after attempt n, which got
// resp or failed with err. The attempt itself is logged by the log
// transport.
func logRetry(ctx context.Context, req *http.Request, c *call, n int, why string, wait time.Duration, resp *http.Response, err error) {
	attrs := []any{"span", "http", "method", req.Method, "attempt", n, "reason", why, "delay_ms", obs.Millis(wait)}
	if c != nil && c.op != "" {
		attrs = append(attrs, "op", c.op)
	}
	if resp != nil {
		attrs = append(attrs, "status", resp.StatusCode)
	}
	if err != nil {
		attrs = append(attrs, "err", err.Error())
	}
	slog.DebugContext(ctx, "http retry", attrs...)
}
