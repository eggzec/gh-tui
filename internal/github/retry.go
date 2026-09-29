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
	"sync"
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
	// logs keeps an outage, when every request is sent again, from
	// logging each retry at info level.
	logs *obs.Throttle
}

// retryLogEvery is how often a retry, or giving up, is logged at info
// level or above for each reason; the others are logged at debug level.
const retryLogEvery = time.Minute

func newRetryTransport(base http.RoundTripper) *retryTransport {
	return &retryTransport{base: base, jitter: rand.Float64, wait: sleep, logs: obs.NewThrottle(retryLogEvery)}
}

type attemptKey struct{}

// withAttempt returns ctx carrying n, the number of the attempt at its
// request, which the log records from the second on.
func withAttempt(ctx context.Context, n int) context.Context {
	return context.WithValue(ctx, attemptKey{}, n)
}

type settleKey struct{}

// settle holds the record of an attempt that failed until the retry
// transport decides whether to send its request again: an attempt sent
// again is logged at debug level, since the retry's record says what
// became of it, and the last attempt keeps its level. Otherwise an outage
// would log each failure once per attempt.
type settle struct {
	mu            sync.Mutex
	decided, sent bool
	held          func()
}

// withSettle returns ctx carrying s for the attempt's log.
func withSettle(ctx context.Context, s *settle) context.Context {
	return context.WithValue(ctx, settleKey{}, s)
}

// hold keeps log, which logs the attempt, until decide, and reports true,
// unless the decision was made already.
func (s *settle) hold(log func()) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.decided {
		return false
	}
	s.held = log
	return true
}

// decide records whether the request is sent again, and logs the attempt
// if its record was held.
func (s *settle) decide(again bool) {
	s.mu.Lock()
	s.decided, s.sent = true, again
	log := s.held
	s.held = nil
	s.mu.Unlock()
	if log != nil {
		log()
	}
}

// sentAgain reports whether the request was sent again after the
// attempt.
func (s *settle) sentAgain() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sent
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
		s := new(settle)
		resp, err := t.base.RoundTrip(attempt.WithContext(withSettle(attempt.Context(), s)))
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
		if why == "" {
			s.decide(false)
			return resp, err
		}
		stop := ""
		switch {
		case n > maxRetries:
			stop = stopAttempts
		case time.Since(start) > retryWithin:
			stop = stopTime
		}
		if stop != "" {
			s.decide(false)
			t.logGiveUp(ctx, req, c, n, why, stop, time.Since(start), resp, err)
			return resp, err
		}
		s.decide(true)
		if resp != nil {
			discard(resp)
		}
		var wait time.Duration
		if why != retryLimit {
			wait = t.backoff(n)
		}
		obs.CountHTTPRetry(why)
		t.logRetry(ctx, req, c, n, why, wait, resp, err)
		if err := t.wait(ctx, wait); err != nil {
			return nil, err
		}
		if attempt, err = replay(req.WithContext(withAttempt(ctx, n+1))); err != nil {
			return nil, err
		}
	}
}

// Why a request that might have been sent again was not, as the record
// of giving up says.
const (
	// stopAttempts is a request sent as often as it may be.
	stopAttempts = "attempts"
	// stopTime is a request first sent longer than retryWithin ago.
	stopTime = "time"
)

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
// isn't trusted, which won't be trusted a moment later either, as is a
// request that can't be sent as it is (unsendable).
func transportRetry(ctx context.Context, err error, read bool) string {
	if ctx.Err() != nil || errors.Is(err, core.ErrRateLimited) || unsendable(err) {
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
// errors without a type. An error with a code, such as undefinedField,
// is GitHub refusing the query itself, which it would refuse again.
// Otherwise resp reads the same body as it came. A body that breaks off
// returns no response and the error.
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
			Type       string `json:"type"`
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	if json.Unmarshal(b, &body) != nil || body.Data != nil || len(body.Errors) == 0 {
		return resp, "", nil
	}
	for _, e := range body.Errors {
		if e.Type != "" || e.Extensions.Code != "" {
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
// resp or failed with err, at info level once a minute for each reason,
// and at debug level otherwise. The attempt itself is logged by the log
// transport.
func (t *retryTransport) logRetry(ctx context.Context, req *http.Request, c *call, n int, why string, wait time.Duration, resp *http.Response, err error) {
	level, held, ok := t.level(ctx, "retry "+why, slog.LevelInfo)
	if !ok {
		return
	}
	attrs := append(retryAttrs(req, c, n, why, resp, err), "delay_ms", obs.Millis(wait))
	slog.Log(ctx, level, "http retry", append(attrs, obs.Suppressed(held)...)...)
}

// logGiveUp logs that req, which attempt n failed for why, isn't sent
// again, since it was sent as often as it may be or too long ago, at warn
// level once a minute for each reason, and at debug level otherwise.
func (t *retryTransport) logGiveUp(ctx context.Context, req *http.Request, c *call, n int, why, stop string, elapsed time.Duration, resp *http.Response, err error) {
	level, held, ok := t.level(ctx, "give up "+why, slog.LevelWarn)
	if !ok {
		return
	}
	attrs := append(retryAttrs(req, c, n, why, resp, err), "stop", stop, "elapsed_ms", obs.Millis(elapsed))
	slog.Log(ctx, level, "http retry gave up", append(attrs, obs.Suppressed(held)...)...)
}

// level returns the level to log a record of key at, and how many like
// it the throttle held back before it: want, if the throttle lets it
// through, or else debug. It reports false if that level isn't enabled.
func (t *retryTransport) level(ctx context.Context, key string, want slog.Level) (slog.Level, int, bool) {
	if !obs.Enabled(ctx, want) {
		return 0, 0, false
	}
	if ok, held := t.logs.Allow(key); ok {
		return want, held, true
	}
	return slog.LevelDebug, 0, obs.Enabled(ctx, slog.LevelDebug)
}

// retryAttrs are what the records of retries say of attempt n at req,
// which got resp or failed with err, and was to be sent again for why.
func retryAttrs(req *http.Request, c *call, n int, why string, resp *http.Response, err error) []any {
	attrs := []any{"span", "http", "method", req.Method, "attempt", n, "reason", why}
	if c != nil && c.op != "" {
		attrs = append(attrs, "op", c.op)
	}
	if resp != nil {
		attrs = append(attrs, "status", resp.StatusCode)
	}
	if err != nil {
		attrs = append(attrs, "err", err.Error())
	}
	return attrs
}
